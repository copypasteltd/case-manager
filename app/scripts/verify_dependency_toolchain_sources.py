"""Verify current remediation and distinguish fresh checks from retained evidence."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import subprocess
import zipfile

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/dependency-toolchain'


def module(name):
    spec = importlib.util.spec_from_file_location('proof_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def main():
    checks = []
    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})
    package = read(APP / 'package.json')
    lock = read(APP / 'package-lock.json')
    check('All six CJS replacements use disclosed local package identities and matching overrides',
          all(package['overrides'][name] == '$' + name and package['devDependencies'][name].startswith('file:packages/')
              for name in ['decompress', 'download-git-repo', 'latest-version', 'image-size', 'html-minifier', 'webpack-dev-server']))
    paths = lock['packages']
    check('No old got, git-clone, download or image-size parser remains in the lock graph',
          not any(name.split('node_modules/')[-1] in ['got', 'git-clone', 'download'] for name in paths)
          and not any(name.endswith('/image-size') and row.get('version') for name, row in paths.items())
          and paths['node_modules/@copypaste/image-size-parser']['version'] == '2.0.4'
          and paths['node_modules/@copypaste/latest-version-upstream']['version'] == '9.0.0')
    install = read(E / 'clean-install.json')
    check('Standard npm ci actually completed with scripts enabled and exact committed lock bytes',
          install['passed'] and install['exit_code'] == 0 and install['lock_unchanged']
          and install['package_lock_sha256'] == sha(APP / 'package-lock.json')
          and install['command'] == ['npm', 'ci', '--no-audit', '--no-fund']
          and install['install_scripts_enabled'] and not install['force_or_legacy_peer_flags'])
    configuration = read(E / 'install-config.json')
    check('Effective npm config enables scripts and includes dependencies without force or legacy peer bypasses',
          configuration['passed'] and configuration['exit_code'] == 0
          and configuration['effective_values'] == {'ignore-scripts': 'false', 'force': 'false',
              'legacy-peer-deps': 'false', 'omit': '', 'include': ''})
    security = read(E / 'security-checks.json')
    check('Actual Metro/Taro, archive, malformed image and Git/HTTP security checks pass on current code',
          security['passed'] and security['source_sha256'] == module('verify_dependency_toolchain_security').digest()
          and security['source_unchanged'] and security['selected_dependency_tree_valid']
          and security['selected_dependency_tree_exit_code'] == 0
          and [v['tests']['pass'] for v in security['checks']] == [40, 2]
          and all(v['passed'] for v in security['checks'])
          and security['selected_dependency_tree_sha256'] == sha(E / 'selected-dependency-tree.json')
          and all(v['public_log_sha256'] == sha(E / (v['name'] + '.log')) for v in security['checks']))
    audit = read(E / 'dependency-audit.json'); before = read(E / 'audit-before.json'); after = read(E / 'audit-after.json')
    check('Full fresh before/after audit is source-bound with no suppressed scope and an open release gate',
          audit['passed'] and audit['all_dependency_scopes_included'] and not audit['advisories_suppressed']
          and not audit['production_security_gate_passed'] and audit['package_lock_sha256'] == sha(APP / 'package-lock.json')
          and audit['raw_report_sha256'] == sha(E / 'audit-after.json')
          and audit['before_vulnerabilities'] == before['metadata']['vulnerabilities']
          and audit['vulnerabilities'] == after['metadata']['vulnerabilities']
          and before['metadata']['vulnerabilities']['total'] == 70)
    pending = read(E / 'remaining-findings.json')
    check('Every remaining npm package finding is individually retained as open',
          len(pending['items']) == audit['vulnerabilities']['total']
          and {v['name'] for v in pending['items']} == set(after['vulnerabilities'])
          and all(v['status'] == 'open' and v['via'] == after['vulnerabilities'][v['name']]['via'] for v in pending['items']))
    clients = read(E / 'client-checks.json')
    client_counts = {v['name']: v['test_count'] for v in clients['checks'] if v['test_count'] is not None}
    check('Eight client commands pass with unchanged complete portable source inputs',
          clients['passed'] and clients['source_unchanged'] and len(clients['checks']) == 8
          and clients['source_sha256'] == digest(clients['source_files'])
          and client_counts == {'web-tests': 172, 'mobile-tests': 273, 'runtime-tests': 55}
          and all(v['passed'] and v['exit_code'] == 0 for v in clients['checks']))
    mobile = read(E / 'mobile-build.json')
    check('Both miniapp builds retain all20 pages and current source input digests',
          mobile['passed'] and mobile['source_sha256'] == module('verify_mobile').digest()[0]
          and all(v['page_count'] == 20 for v in mobile['targets'].values()))
    android = read(E / 'build.json'); inspected = read(E / 'apk-inspection.json')
    check('Native Android build and18 actual APK checks bind local compatibility source as well as the lock',
          android['passed'] and android['source_unchanged_during_build']
          and android['source_sha256'] == module('build_android').sources()[0]
          and any(v.startswith('packages/image-size-compat/') for v in android['sources'])
          and sha(APP / android['artifacts']['apk']['path']) == android['artifacts']['apk']['sha256']
          and inspected['passed'] and len(inspected['checks']) == 18 and all(v['passed'] for v in inspected['checks'])
          and not android['device_executed'] and not android['emulator_started'] and not android['production_release'])
    jvm = read(E / 'android-test-scope.json')
    check('JVM26 accurately records new execution or Gradle cache scope',
          jvm['passed'] and jvm['tests'] == 26 and jvm['new_test_execution'] == (not jvm['gradle_cached'])
          and not jvm['device_executed'] and jvm['native_log_sha256'] == sha(E / 'native-build.log')
          and len(jvm['xml_sha256']) == 4
          and all(sha(E / 'android-unit-results' / name) == value for name, value in jvm['xml_sha256'].items()))
    retained = read(E / 'reused-evidence.json')
    check('All211 published media remain byte-identical and retain their original capture scope',
          retained['passed'] and sum(retained['media_counts'].values()) == 211
          and not retained['new_browser_capture'] and retained['ui_sources_unchanged']
          and all(sha(ROOT / v['file']) == v['sha256'] for v in retained['media']))
    ui_changed = subprocess.check_output(['git', 'diff', '--name-only', retained['published_commit'], '--', *retained['ui_source_paths']], cwd=ROOT).decode().splitlines()
    check('Retained UI source and original Go/browser records remain unchanged',
          not ui_changed and all(sha(ROOT / v['file']) == v['sha256'] for v in retained['retained_records']))
    backend = read(E.parent / 'cost-amortization-backend/result.json')
    backend_digest = module('verify_backend_batches').digest()
    check('Retained224/1355 backend evidence still matches all unchanged current Go/SQL sources',
          not retained['new_backend_test_execution'] and backend['passed']
          and backend['source_sha256'] == backend_digest and backend['all_discovered_tests_executed']
          and backend['discovered_top_level_tests'] == 224 and len(backend['checks']) == 1355
          and all(v['Action'] == 'pass' for v in backend['checks']))
    linux = read(E.parent / 'cost-amortization/linux-build.json')
    check('Retained Linux cross-build agrees with unchanged Go source; no deployment claimed',
          linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_digest
          and not linux['artifact_executed'] and not linux['deployed'])
    dev = read(E / 'development-project-resources.json')
    check('Native development inspection confirms schema83, current owned processes and no test databases',
          dev['passed'] and dev['migration_count'] == 83 and dev['backend_source_sha256'] == backend_digest
          and dev['database']['disposable_test_databases'] == 0 and dev['integrations_disabled']
          and dev['worker_sources_match'] and all(dev['ownership'].values()))
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    check('Scope retains23 open work packages and excludes AI fees only',
          scope['counts']['active'] == 23 and scope['counts']['excluded'] == 5
          and not scope['flags']['ai_cost_control_required'] and scope['flags']['ai_execution_isolation_required'])
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('Eight Skills remain drafts awaiting real model acceptance', len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    leftovers = [p.name for p in (APP / '.local').iterdir() if p.is_dir() and re.fullmatch(r'dependency-(?:remediation|security|maintenance|toolchain)-[a-zA-Z0-9]{6}', p.name)]
    check('Owned synthetic security workspaces are removed after completed checks', leftovers == [])
    check('Installed graph resolves modern template/server, PostCSS and optional Vite consumers only',
          package['engines']['node'] == '>=22.15.0 <23'
          and paths['node_modules/@copypaste/dev-server-upstream']['version'] == '6.1.0'
          and paths['node_modules/@copypaste/html-minifier-upstream']['version'] == '7.2.0'
          and paths['node_modules/@pmmmwh/react-refresh-webpack-plugin']['version'] == '0.6.3'
          and all(row.get('version') == '8.5.32' for name, row in paths.items() if name.endswith('node_modules/postcss'))
          and all(row.get('version') == '8.3.5' for name, row in paths.items() if name.endswith('node_modules/vite')))
    watch = read(E / 'h5-server-check.json'); client = read(E / 'h5-client-check.json')
    check('Actual current Taro watch/login smoke passes with closed browser, context, process and port',
          watch['passed'] and watch['source_unchanged'] and watch['source_sha256'] == module('dependency_toolchain_sources').digest()
          and watch['owned_process_terminal'] and watch['owned_port_released'] and len(watch['checks']) == 5
          and watch['client_record_sha256'] == sha(E / 'h5-client-check.json')
          and client['passed'] and client['context_closed'] and client['browser_closed'] and len(client['checks']) == 3
          and not client['recorded_credentials'] and not client['new_media'])
    attempt_dir = E / 'attempts/01-initial-consumers'
    attempt = read(attempt_dir / 'result.json'); original = read(attempt_dir / 'source-manifest.json')
    value = hashlib.sha256(); original_ok = True
    with zipfile.ZipFile(attempt_dir / 'changed-source-inputs.zip') as archive:
        archived = set(archive.namelist())
        for row in original['source_files']:
            data = archive.read(row['file']) if row['file'] in archived else (APP / row['file']).read_bytes()
            original_ok = original_ok and hashlib.sha256(data).hexdigest() == row['sha256']
            value.update(row['file'].encode() + b'\0' + data + b'\0')
    failed_log = (APP / '.local/dependency-toolchain-initial-consumers.log')
    check('Initial failed consumer inputs and exact failure provenance are retained without being accepted as a pass',
          original_ok and not attempt['passed'] and attempt['exit_code'] == 1 and attempt['source_unchanged']
          and value.hexdigest() == original['source_sha256'] == attempt['source_sha256']
          and original['changed_archive_sha256'] == sha(attempt_dir / 'changed-source-inputs.zip')
          and original['public_log_sha256'] == sha(attempt_dir / 'consumer-tests.log')
          and (not failed_log.exists() or attempt['raw_log_sha256'] == sha(failed_log)))
    interrupted_ok = True
    for label, raw_name in [('03-security-deadline', 'dependency-toolchain-security-attempt03.log'),
                            ('04-security-cold-start', 'dependency-toolchain-security-attempt04.log')]:
        folder = E / 'attempts' / label
        interrupted = read(folder / 'result.json')
        original_digest = hashlib.sha256()
        with zipfile.ZipFile(folder / 'source-inputs.zip') as archive:
            interrupted_ok = interrupted_ok and archive.namelist() == interrupted['sources']
            for name in interrupted['sources']:
                original_digest.update(name.encode() + b'\0' + archive.read(name) + b'\0')
        raw = APP / '.local' / raw_name
        interrupted_ok = interrupted_ok and not interrupted['passed'] and interrupted['exit_code'] == 1
        interrupted_ok = interrupted_ok and original_digest.hexdigest() == interrupted['source_sha256']
        interrupted_ok = interrupted_ok and interrupted['archive_sha256'] == sha(folder / 'source-inputs.zip')
        interrupted_ok = interrupted_ok and (not raw.exists() or sha(raw) == interrupted['raw_log_sha256'])
    check('Interrupted security suites retain complete original inputs and remain failed attempts', interrupted_ok)
    later_attempts_ok = True
    for label in ['05-server-and-tree-deadlines', '06-optional-peer-tree']:
        folder = E / 'attempts' / label
        attempted = read(folder / 'result.json')
        original_digest = hashlib.sha256()
        with zipfile.ZipFile(folder / 'source-inputs.zip') as archive:
            later_attempts_ok = later_attempts_ok and archive.namelist() == attempted['sources']
            for name in attempted['sources']:
                original_digest.update(name.encode() + b'\0' + archive.read(name) + b'\0')
        later_attempts_ok = later_attempts_ok and not attempted['passed'] and attempted['exit_code'] == 1
        later_attempts_ok = later_attempts_ok and original_digest.hexdigest() == attempted['source_sha256']
        later_attempts_ok = later_attempts_ok and attempted['archive_sha256'] == sha(folder / 'source-inputs.zip')
    check('Server-deadline and invalid-peer outcomes retain their original full source archives', later_attempts_ok)
    applicability = read(E / 'acceptance-applicability-check.json')
    applicability_module = module('verify_acceptance_applicability')
    check('All original AC/TC IDs have current applicability without inventing business test passes',
          applicability['passed'] and len(applicability['checks']) == 12
          and all(row['passed'] for row in applicability['checks'])
          and applicability['business_cases_executed'] == applicability['business_cases_passed'] == 0
          and applicability['manifest_sha256'] == sha(applicability_module.MANIFEST)
          and applicability['document_sha256'] == sha(applicability_module.DOCUMENT)
          and applicability['authority_scope_sha256'] == sha(applicability_module.SCOPE)
          and applicability['verification_script_sha256'] == sha(APP / 'scripts/verify_acceptance_applicability.py'))
    encoding_attempt = E / 'attempts/07-native-log-encoding'
    encoding_failure = read(encoding_attempt / 'result.json')
    check('Native evidence encoding repair preserves the original failed helper and raw log bytes',
          not encoding_failure['passed'] and encoding_failure['exit_code'] == 1
          and encoding_failure['helper_sha256'] == sha(encoding_attempt / 'freeze_dependency_toolchain_android.py.source')
          and encoding_failure['native_log_sha256'] == sha(encoding_attempt / 'native-build.log')
          and encoding_failure['build_record_sha256'] == sha(E / 'attempts/08-watch-development-peer/previous-candidate/build.json'))
    failed_watch_ok = True
    for label in ['08-watch-development-peer', '09-browser-executable', '10-watch-socket-and-input']:
        folder = E / 'attempts' / label
        failed = read(folder / 'result.json'); original = read(folder / 'source-manifest.json')
        original_digest = hashlib.sha256()
        with zipfile.ZipFile(folder / 'changed-source-inputs.zip') as archive:
            for row in original['source_files']:
                data = archive.read(row['file']) if row['file'] in archive.namelist() else (APP / row['file']).read_bytes()
                failed_watch_ok = failed_watch_ok and hashlib.sha256(data).hexdigest() == row['sha256']
                original_digest.update(row['file'].encode() + b'\0' + data + b'\0')
        failed_watch_ok = failed_watch_ok and not failed['passed'] and failed['owned_port_released']
        failed_watch_ok = failed_watch_ok and original_digest.hexdigest() == original['source_sha256'] == failed['source_sha256']
        failed_watch_ok = failed_watch_ok and original['changed_archive_sha256'] == sha(folder / 'changed-source-inputs.zip')
        if failed['client_record_sha256']:
            failed_watch_ok = failed_watch_ok and failed['client_record_sha256'] == sha(folder / 'client.json')
    check('Failed watch, browser and socket attempts retain their full original provenance and freed ports', failed_watch_ok)
    post_ci = E / 'attempts/11-post-ci-timeouts'
    post_ci_result = read(post_ci / 'result.json')
    post_ci_security = read(post_ci / 'security-checks.json')
    post_ci_digest = hashlib.sha256()
    with zipfile.ZipFile(post_ci / 'source-inputs.zip') as archive:
        post_ci_inputs_ok = set(archive.namelist()) == set(post_ci_security['sources'])
        for name in post_ci_security['sources']:
            post_ci_digest.update(name.encode() + b'\0' + archive.read(name) + b'\0')
    post_ci_log = (post_ci / 'security-tests.log').read_text(encoding='utf-8')
    check('The unchanged-code post-CI timeout attempt retains full inputs and its original38/2 outcome',
          not post_ci_result['passed'] and not post_ci_security['passed'] and post_ci_inputs_ok
          and post_ci_digest.hexdigest() == post_ci_result['source_sha256'] == post_ci_security['source_sha256'] == security['source_sha256']
          and post_ci_result['archive_sha256'] == sha(post_ci / 'source-inputs.zip')
          and post_ci_result['original_record_sha256'] == sha(post_ci / 'security-checks.json')
          and post_ci_security['checks'][0]['tests'] == {'tests': 40, 'pass': 38, 'fail': 2, 'skipped': 0, 'cancelled': 0}
          and post_ci_security['checks'][1]['tests'] == {'tests': 2, 'pass': 2, 'fail': 0, 'skipped': 0, 'cancelled': 0}
          and post_ci_security['selected_dependency_tree_valid']
          and all(row['public_log_sha256'] == sha(post_ci / (row['name'] + '.log')) for row in post_ci_security['checks'])
          and post_ci_security['selected_dependency_tree_sha256'] == sha(post_ci / 'selected-dependency-tree.json')
          and 'ETIMEDOUT' in post_ci_log and 'Synthetic HTTP request timed out.' in post_ci_log)
    proof = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
             'interaction_kind': 'dependency_toolchain_and_retained_media',
             'interaction_source_sha256': module('dependency_toolchain_sources').digest(),
             'mobile_source_sha256': module('verify_mobile').digest()[0], 'backend_source_sha256': backend_digest,
             'verification_script_sha256': sha(Path(__file__)), 'new_browser_capture': False,
             'new_backend_test_execution': False, 'full_goal_complete': False, 'production_ready': False}
    (E / 'final-source-check.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': proof['passed'], 'checks': len(checks), 'failed': [v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
