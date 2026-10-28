"""Verify fresh dependency/client evidence and explicitly retained published UI/backend evidence."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import socket
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/dependency-maintenance'
PUBLISHED = '269d9004af32a7fa90c5002fa27167eec875dfdc'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def module(name):
    spec = importlib.util.spec_from_file_location('maintenance_' + name, APP / 'scripts' / name)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def main():
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    security = read(E / 'security-checks.json')
    check('Fourteen actual dependency security/compatibility tests and two native JS tests pass on current inputs',
        security['passed'] and security['source_unchanged']
        and security['source_sha256'] == module('verify_dependency_maintenance_security.py').digest()
        and [(row['name'], row['tests']['pass']) for row in security['checks']] == [('security-tests', 14), ('native-js-tests', 2)]
        and all(row['passed'] and sha(E / (row['name'] + '.log')) == row['log_sha256'] for row in security['checks']))
    check('Selected installed dependency graph is valid and has exact retained bytes',
        security['selected_dependency_tree_valid'] and sha(E / 'selected-dependency-tree.json') == security['selected_dependency_tree_sha256'])
    audit = read(E / 'dependency-audit.json')
    before, after = [read(E / name) for name in ('audit-before.json', 'audit-after.json')]
    check('Full current audit includes development, optional and peer scopes without suppressed advisories',
        audit['passed'] and audit['source_unchanged'] and audit['all_dependency_scopes_included']
        and not audit['advisories_suppressed'] and not audit['production_security_gate_passed']
        and audit['command'] == ['npm', 'audit', '--json', '--include=dev', '--include=optional', '--include=peer']
        and sha(APP / 'package-lock.json') == audit['package_lock_sha256']
        and sha(E / audit['raw_report']) == audit['raw_report_sha256']
        and before['metadata']['vulnerabilities'] == audit['before_vulnerabilities']
        and after['metadata']['vulnerabilities'] == audit['vulnerabilities'])
    check('Fresh before/after counts are91/80; no glob, matcher, braces or RN runner finding is hidden',
        audit['before_vulnerabilities']['total'] == 91 and audit['before_vulnerabilities']['high'] == 20
        and audit['vulnerabilities'] == {'info': 0, 'low': 7, 'moderate': 60, 'high': 13, 'critical': 0, 'total': 80}
        and not {'glob', 'micromatch', 'braces', '@tarojs/rn-runner'} & set(after['vulnerabilities']))
    remaining = read(E / 'remaining-findings.json')
    check('Every remaining package finding stays open, with exact original advisory data',
        remaining['audit_sha256'] == sha(E / 'audit-after.json') and len(remaining['findings']) == 80
        and {row['name'] for row in remaining['findings']} == set(after['vulnerabilities'])
        and all(row['status'].startswith('open;') for row in remaining['findings']))
    lock = read(APP / 'package-lock.json')['packages']
    check('Native runner is removed while all four client targets and supported native entry remain',
        '@tarojs/rn-runner' not in read(APP / 'mobile/package.json')['devDependencies']
        and not any(name.endswith('/@tarojs/rn-runner') for name in lock)
        and read(APP / 'package.json')['overrides']['micromatch'] == '4.0.8'
        and read(APP / 'package.json')['overrides']['glob@10'] == '10.5.0'
        and all(name in read(APP / 'package.json')['scripts'] for name in ('build:web', 'build:weapp', 'build:tt', 'build:rn'))
        and (APP / 'mobile/native/build-bundle.cjs').read_bytes() == subprocess.check_output(['git', 'show', PUBLISHED + ':app/mobile/native/build-bundle.cjs'], cwd=ROOT))
    client = read(E / 'client-checks.json')
    check('Eight fresh client commands bind unchanged complete inputs and byte-matching native logs',
        client['passed'] and client['source_unchanged'] and len(client['checks']) == 8
        and client['source_sha256'] == digest(client['source_files'])
        and all(row['passed'] and row['exit_code'] == 0 and sha(APP / '.local' / Path(row['raw_log']).name) == row['raw_log_sha256'] for row in client['checks']))
    web, mobile, runtime = [read(E / name) for name in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    check('Fresh Web135, mobile248 and runtime51 results remain source-bound',
        web['passed'] and web['source_sha256'] == digest(web['sources']) and web['tests']['numPassedTests'] == 135
        and mobile['passed'] and mobile['source_sha256'] == module('verify_mobile.py').digest()[0] and mobile['tests']['pass'] == 248
        and runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime.py').digest()[0] and runtime['tests']['pass'] == 51)
    check('Both real local mini-program outputs contain20 pages',
        all(row['page_count'] == 20 and sha(APP / 'mobile/dist' / target / 'app.json') == row['app_json_sha256'] for target, row in mobile['targets'].items()))
    build, inspection, scope = [read(E / name) for name in ('build.json', 'apk-inspection.json', 'android-test-scope.json')]
    check('Current-source Android build and18 fresh package checks bind complete sources',
        build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android.py').sources()[0]
        and sha(APP / build['artifacts']['apk']['path']) == build['artifacts']['apk']['sha256']
        and inspection['passed'] and len(inspection['checks']) == 18 and all(row['passed'] for row in inspection['checks'])
        and not build['device_executed'] and not build['emulator_started'] and not build['virtualization_used'] and not build['production_release'])
    check('JVM26 results state real Gradle execution/cache scope without claiming a device',
        scope['passed'] and scope['tests'] == 26 and scope['new_test_execution'] == (not scope['gradle_cached'])
        and not scope['device_executed'] and sha(E / 'native-build.log') == scope['native_log_sha256'])
    retained = read(E / 'reused-evidence.json')
    check('Media are explicitly reused from97 with exact original97 PNG/35 MP4/35 GIF bytes',
        retained['passed'] and retained['published_commit'] == PUBLISHED and not retained['new_browser_capture']
        and retained['media_counts'] == {'png': 97, 'mp4': 35, 'gif': 35}
        and all(sha(ROOT / row['file']) == row['sha256'] for row in retained['media']))
    actual_blobs = subprocess.check_output(['git', 'hash-object', '--', *[row['file'] for row in retained['media']]], cwd=ROOT).decode().splitlines()
    check('Retained media agree with the published Git objects and unchanged UI sources',
        actual_blobs == [row['git_blob'] for row in retained['media']]
        and retained['ui_sources_unchanged'] and subprocess.check_output(['git', 'diff', '--name-only', PUBLISHED, '--', 'app/web', 'app/mobile'], cwd=ROOT).decode().splitlines() == ['app/mobile/package.json'])
    check('Original backend, Linux and browser records retain exact published bytes and historical scope',
        not retained['new_backend_test_execution'] and not retained['new_linux_cross_compile']
        and all(sha(ROOT / row['file']) == row['sha256'] and (ROOT / row['file']).read_bytes() == subprocess.check_output(['git', 'show', PUBLISHED + ':' + row['file']], cwd=ROOT) for row in retained['retained_records']))
    backend = read(E.parent / 'miniapp-identity-backend/result.json')
    backend_source = module('verify_backend_batches.py').digest()
    check('Reused native backend189/1183 results still match exact unchanged Go sources',
        backend['passed'] and backend['source_sha256'] == backend_source and backend['all_discovered_tests_executed']
        and backend['discovered_top_level_tests'] == 189 and len(backend['checks']) == 1183 and all(row['Action'] == 'pass' for row in backend['checks']))
    linux = read(E.parent / 'miniapp-identity/linux-build.json')
    check('Reused Linux cross-build matches current Go tree; no Linux execution or deployment claimed',
        linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_source
        and not linux['artifact_executed'] and not linux['deployed'])
    previous = read(E / 'publication-before.json')
    check('All nine prior bilingual guide/baseline snapshots match their published original bytes',
        previous['published_commit'] == PUBLISHED and len(previous['files']) == 9
        and all(sha(ROOT / row['snapshot']) == row['sha256'] and (ROOT / row['snapshot']).read_bytes() == subprocess.check_output(['git', 'show', PUBLISHED + ':' + row['file']], cwd=ROOT) for row in previous['files']))
    dev = read(E / 'development-project-resources.json')
    check('Fresh read-only native inspection confirms schema78, owned services, no test DBs or external activation',
        dev['passed'] and dev['read_only_inspection'] and not dev['production_deployment'] and dev['migration_count'] == 78
        and dev['backend_source_sha256'] == backend_source and dev['worker_sources_match'] and all(dev['ownership'].values())
        and dev['database']['disposable_test_databases'] == 0 and dev['integrations_disabled'])
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    gaps = (ROOT / 'docs/delivery/03-goal-gap-assessment.md').read_text(encoding='utf-8-sig')
    check('All eight Skills remain draft and all28 original gap IDs remain open for complete goal acceptance',
        len(skills) == 8 and all(row['state'] == 'draft' for row in skills)
        and set(re.findall(r'\| (GAP-\d{2}) \|', gaps)) == {f'GAP-{n:02d}' for n in range(1, 29)})
    with socket.socket() as sock:
        sock.settimeout(.2)
        unused = sock.connect_ex(('127.0.0.1', 5189)) != 0
    check('No review server or credential-bearing browser fixture is left behind',
        unused and not (APP / '.local/business-browser-fixture.json').exists())
    result = {'passed': all(row['passed'] for row in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
        'interaction_kind': 'dependency_compatibility_and_retained_media',
        'interaction_source_sha256': module('dependency_maintenance_sources.py').digest(),
        'mobile_source_sha256': module('verify_mobile.py').digest()[0], 'backend_source_sha256': backend_source,
        'verification_script_sha256': sha(Path(__file__)), 'new_browser_capture': False,
        'full_goal_complete': False, 'production_ready': False, 'real_device': False, 'real_cloud': False,
        'real_model': False, 'real_telegram_network': False, 'real_wechat_or_douyin_exchange': False}
    (E / 'final-source-check.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'checks': len(checks), 'failed': [row['name'] for row in checks if not row['passed']]}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
