"""Execute exact-source release tests, then inspect actual built archive bytes."""
import argparse
from datetime import datetime, timezone
import importlib.util
import io
import json
from pathlib import Path
import re
import subprocess
import sys
import unittest
import zipfile

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
sys.path.insert(0, str(APP / 'deploy/release'))
from bundle import canonical, digest, extract_archive, file_sha, read_json, require
from catalog import SERVICES, render, unit
from build_production_release import inputs

EVIDENCE = ROOT / 'docs/quality/evidence/native-release'


def module(name):
    spec = importlib.util.spec_from_file_location('release_proof_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def sources():
    files = [*sorted((APP / 'deploy/release').glob('*.py')),
             APP / 'scripts/build_production_release.py', APP / 'scripts/native_ci_release.py',
             APP / 'scripts/verify_native_release_engineering.py', ROOT / '.github/workflows/native-release.yml']
    return [{'file': p.relative_to(ROOT).as_posix(), 'sha256': file_sha(p)} for p in files]


def run_tests(attempt):
    require(re.fullmatch(r'[a-z0-9-]{1,40}', attempt), 'Use a unique test attempt name.')
    private = APP / '.local/native-release119' / attempt
    require(not private.exists(), 'Preserve prior attempts; choose a new name.')
    private.mkdir(parents=True)
    before = sources()
    with zipfile.ZipFile(private / 'source.zip', 'x', zipfile.ZIP_DEFLATED) as archive:
        for row in before:
            archive.write(ROOT / row['file'], row['file'])
    output = io.StringIO()
    records = []
    class Result(unittest.TextTestResult):
        def addSuccess(self, test):
            records.append({'name': test.id(), 'passed': True})
            super().addSuccess(test)
        def addError(self, test, error):
            records.append({'name': test.id(), 'passed': False, 'kind': 'error'})
            super().addError(test, error)
        def addFailure(self, test, error):
            records.append({'name': test.id(), 'passed': False, 'kind': 'failure'})
            super().addFailure(test, error)
    suite = unittest.defaultTestLoader.discover(str(APP / 'deploy/release'), pattern='test_native_release.py')
    result = unittest.TextTestRunner(stream=output, resultclass=Result, verbosity=2).run(suite)
    (private / 'tests.log').write_text(output.getvalue(), encoding='utf-8')
    report = {'passed': result.wasSuccessful() and not result.skipped and before == sources(),
              'checked_at': datetime.now(timezone.utc).isoformat(), 'tests': result.testsRun, 'checks': records,
              'skipped': len(result.skipped), 'source_unchanged': before == sources(), 'source_files': before,
              'source_sha256': digest(canonical(before)), 'raw_log': (private / 'tests.log').relative_to(ROOT).as_posix(),
              'raw_log_sha256': file_sha(private / 'tests.log'), 'private_source_archive': (private / 'source.zip').relative_to(ROOT).as_posix(),
              'source_archive_sha256': file_sha(private / 'source.zip'), 'native_host': sys.platform,
              'native_symlinks_executed': sys.platform == 'linux', 'process_effects_injected': True,
              'native_posix_permissions_executed': sys.platform == 'linux',
              'windows_file_pointer_double': sys.platform != 'linux', 'systemd_executed': False,
              'production_database_used': False, 'virtualization_used': False}
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    (EVIDENCE / ('tests-' + attempt + '.json')).write_bytes(canonical(report))
    print(json.dumps({k: report[k] for k in ('passed', 'tests', 'skipped', 'source_unchanged', 'native_symlinks_executed')}))
    return 0 if report['passed'] else 1


def verify(attempt, build_folder):
    build_folder = Path(build_folder).resolve(strict=True)
    require((APP / '.local').resolve() in build_folder.parents and ROOT in build_folder.parents, 'Inspect an owned private build folder.')
    test = read_json((EVIDENCE / ('tests-' + attempt + '.json')).read_bytes())
    build = read_json((build_folder / 'build.json').read_bytes())
    checks = []
    def check(name, value):
        checks.append({'name': name, 'passed': bool(value)})
    check('All exact current release tests pass without skips or source edits', test['passed'] and test['source_files'] == sources()
          and len(test['checks']) == test['tests'] and all(r['passed'] for r in test['checks']) and not test['skipped'])
    check('Exact raw test log and tested source archive remain private and retained', file_sha(ROOT / test['raw_log']) == test['raw_log_sha256']
          and file_sha(ROOT / test['private_source_archive']) == test['source_archive_sha256'])
    with zipfile.ZipFile(ROOT / test['private_source_archive']) as archive:
        check('Test source archive matches every declared current engineering input', all(digest(archive.read(r['file'])) == r['sha256'] for r in sources()))
    check('Native build passes and keeps complete declared build inputs unchanged', build['passed'] and build['source_files'] == inputs()
          and build['source']['inputs_sha256'] == digest(canonical(inputs())) and len(build['commands']) == 5 and all(r['exit_code'] == 0 for r in build['commands']))
    with zipfile.ZipFile(ROOT / build['private_source_archive']) as saved:
        check('Actual build input archive and all five raw compiler logs retain exact bytes',
              file_sha(ROOT / build['private_source_archive']) == build['source_archive_sha256']
              and all(digest(saved.read(row['path'])) == row['sha256'] for row in build['source_files'])
              and all(file_sha(build_folder / 'private-logs' / (name + '.log')) == row['raw_log_sha256']
                      for name, row in zip(('platform', 'agent-native', 'status-bridge', 'web', 'runtime'), build['commands'])))
    archive = ROOT / build['archive']
    check('Frozen archive retains the exact sealed digest', file_sha(archive) == build['archive_sha256'])
    inspection = APP / '.local/native-release119' / ('archive-inspection-' + attempt)
    manifest = extract_archive(archive, inspection, build['manifest_sha256'])
    check('Actual archive extracts only exact regular inventory members', len(manifest['files']) == build['files'] and manifest['source'] == build['source'])
    artifacts = [inspection / 'bin' / n for n in ('platform', 'agent-native', 'status-bridge')]
    check('All three actual server executables are Linux ELF64 x86-64', all(p.read_bytes()[:5] == b'\x7fELF\x02' and p.read_bytes()[18:20] == b'\x3e\x00' for p in artifacts))
    schema = [{'name': p.name, 'sha256': file_sha(p)} for p in sorted((APP / 'backend/migrations').glob('*.sql'))]
    check('All91 exact current normal schema upgrades are pinned', manifest['schema'] == schema and len(schema) == 91)
    runtime = read_json((inspection / 'metadata/runtime.json').read_bytes())
    packaging = module('build_native_runtime_release')
    check('Runtime dependency closure matches its actual packaged graph', runtime['release_sha256'] == packaging.release_digest(inspection / 'runtime')
          and all(file_sha(inspection / 'runtime' / row['file']) == row['sha256'] for row in runtime['files']))
    skills = [read_json(p.read_bytes()) for p in (inspection / 'runtime/app/skills').glob('*/manifest.json')]
    check('All eight packaged Skills remain draft with no model activation', len(skills) == 8 and all(v['state'] == 'draft' for v in skills) and build['all_skills_remain_draft'])
    check('Every server unit uses the fixed current service catalog', all((inspection / 'systemd' / unit(name)).read_text(encoding='utf-8') == render(name) for name in SERVICES))
    check('Packaged deployment tools match current source bytes', all(file_sha(inspection / 'metadata/release-tools' / n) == file_sha(APP / 'deploy/release' / n) for n in ('bundle.py', 'catalog.py', 'controller.py')))
    # Parse real YAML using the installed dependency, not a hand-written text matcher.
    yaml_log = subprocess.check_output([str(APP / '.tools/node-v22.23.4-win-x64/node.exe') if sys.platform == 'win32' else 'node', '--input-type=module', '-e',
        "import fs from 'node:fs'; import YAML from 'yaml'; const w=YAML.parse(fs.readFileSync('../.github/workflows/native-release.yml','utf8')); console.log(JSON.stringify(w));"], cwd=APP)
    workflow = read_json(yaml_log)
    job = workflow['jobs']['package']
    check('Actual CI YAML is manual native self-hosted only with pinned actions and scoped uploads', set(workflow['on']) == {'workflow_dispatch'}
          and job['runs-on'] == ['self-hosted', 'linux', 'x64', 'copypaste-release'] and 'container' not in job and 'services' not in job
          and workflow['permissions'] == {'contents': 'read'} and not workflow['concurrency']['cancel-in-progress']
          and all(re.fullmatch(r'actions/[a-z-]+@[a-f0-9]{40}', step['uses']) for step in job['steps'] if 'uses' in step)
          and all('artifacts/' in line for step in job['steps'] if step.get('uses', '').startswith('actions/upload-artifact@') for line in step['with']['path'].strip().splitlines()))
    baseline = read_json((ROOT / 'docs/quality/current-functional-baseline.json').read_bytes())
    retained = read_json((ROOT / 'docs/quality/evidence/payment-fee/final-source-check.json').read_bytes())
    check('The complete application remains at its exact published118 functional source', baseline['increment'] == 118 and baseline['evidence_group'] == 'payment-fee'
          and retained['passed'] and retained['interaction_source_sha256'] == module('payment_fee_sources').digest())
    previous_commit = '86fa0a5c7d856070abddac5791c6a862ad7129c8'
    old = []
    rows = subprocess.check_output(['git', 'ls-tree', '-r', '--name-only', previous_commit, '--', 'docs/media', 'docs/quality/evidence/payment-fee', 'docs/quality/evidence/payment_fee-backend'], cwd=ROOT).decode().splitlines()
    for name in rows:
        if name.startswith('docs/media/') and Path(name).suffix not in ('.png', '.mp4', '.gif'):
            continue
        prior = subprocess.check_output(['git', 'show', previous_commit + ':' + name], cwd=ROOT)
        old.append({'file': name, 'sha256': digest(prior)})
    check('All316 published media and original118 proof bytes are retained', sum(r['file'].startswith('docs/media/') for r in old) == 316
          and all(file_sha(ROOT / r['file']) == r['sha256'] for r in old))
    private_values = module('verify_repository_publication').private_values()
    matches = [r['path'] for r in manifest['files'] if any(value in (inspection / r['path']).read_bytes() for value in private_values)]
    check('Actual artifact bytes contain none of the known private development values', not matches)
    preliminary = read_json((EVIDENCE / 'preliminary-attempts.json').read_bytes())
    check('Superseded and failed attempts retain declared sources and logs without final pass credit', not preliminary['counts_as_final_pass']
          and all(file_sha(ROOT / row['file']) == row['sha256'] for row in preliminary['records']))
    guard = None
    if sys.platform == 'win32':
        result = subprocess.run([sys.executable, '-B', str(APP / 'deploy/release/controller.py'), 'activate', '--release-id', 'r-' + '0' * 24,
            '--manifest-sha256', '0' * 64, '--configuration', 'guard-only', '--expected-current', 'none'], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        guard = result.returncode == 1 and not result.stdout and b'Controlled release failed' in result.stderr
        check('Actual Windows deployment invocation refuses before host actions', guard)
    proof = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
             'engineering_increment': 119, 'functional_increment': 118, 'source_files': sources(), 'source_sha256': digest(canonical(sources())),
             'tests_record': 'docs/quality/evidence/native-release/tests-' + attempt + '.json', 'test_count': test['tests'],
             'build_record': (build_folder / 'build.json').relative_to(ROOT).as_posix(), 'build_record_sha256': file_sha(build_folder / 'build.json'),
             'archive_sha256': build['archive_sha256'], 'manifest_sha256': build['manifest_sha256'], 'release_id': build['release_id'],
             'retained': old, 'windows_deployment_guard_executed': guard, 'native_symlinks_executed': test['native_symlinks_executed'],
             'native_posix_permissions_executed': test['native_posix_permissions_executed'],
             'process_effects_injected': True, 'new_linux_ci_run': False, 'systemd_executed': False, 'production_deployed': False,
             'new_application_regression': False, 'new_browser_capture': False, 'full_goal_complete': False, 'production_ready': False}
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    (EVIDENCE / 'build.json').write_bytes(canonical(build))
    (EVIDENCE / 'verification.json').write_bytes(canonical(proof))
    print(json.dumps({'passed': proof['passed'], 'checks': len(checks), 'tests': test['tests'], 'failed': [v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=('test', 'verify'))
    parser.add_argument('--attempt', required=True)
    parser.add_argument('--build', type=Path)
    args = parser.parse_args()
    return run_tests(args.attempt) if args.mode == 'test' else verify(args.attempt, args.build)


if __name__ == '__main__':
    raise SystemExit(main())
