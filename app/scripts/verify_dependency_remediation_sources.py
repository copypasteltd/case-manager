"""Verify current remediation and distinguish fresh checks from retained evidence."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/dependency-remediation'


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
    check('All three CJS replacements use disclosed local package identities and matching overrides',
          all(package['overrides'][name] == '$' + name and package['devDependencies'][name].startswith('file:packages/')
              for name in ['download-git-repo', 'latest-version', 'image-size']))
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
          and not install['force_or_legacy_peer_flags'])
    security = read(E / 'security-checks.json')
    check('Actual Metro/Taro, archive, malformed image and Git/HTTP security checks pass on current code',
          security['passed'] and security['source_sha256'] == module('verify_dependency_remediation_security').digest()
          and security['source_unchanged'] and security['selected_dependency_tree_valid']
          and security['selected_dependency_tree_exit_code'] == 0
          and [v['tests']['pass'] for v in security['checks']] == [31, 2]
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
          and before['metadata']['vulnerabilities']['total'] == 80)
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
          and client_counts == {'web-tests': 151, 'mobile-tests': 255, 'runtime-tests': 52}
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
    check('All189 published media remain byte-identical and retain their original capture scope',
          retained['passed'] and retained['media_counts'] == {'png': 107, 'mp4': 41, 'gif': 41}
          and not retained['new_browser_capture'] and retained['ui_sources_unchanged']
          and all(sha(ROOT / v['file']) == v['sha256'] for v in retained['media']))
    ui_changed = subprocess.check_output(['git', 'diff', '--name-only', retained['published_commit'], '--', *retained['ui_source_paths']], cwd=ROOT).decode().splitlines()
    check('Retained UI source and original Go/browser records remain unchanged',
          not ui_changed and all(sha(ROOT / v['file']) == v['sha256'] for v in retained['retained_records']))
    backend = read(E.parent / 'finance-fx-backend/result.json')
    backend_digest = module('verify_backend_batches').digest()
    check('Retained191/1201 backend evidence still matches all unchanged current Go/SQL sources',
          not retained['new_backend_test_execution'] and backend['passed']
          and backend['source_sha256'] == backend_digest and backend['all_discovered_tests_executed']
          and backend['discovered_top_level_tests'] == 191 and len(backend['checks']) == 1201
          and all(v['Action'] == 'pass' for v in backend['checks']))
    linux = read(E.parent / 'finance-fx/linux-build.json')
    check('Retained Linux cross-build agrees with unchanged Go source; no deployment claimed',
          linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_digest
          and not linux['artifact_executed'] and not linux['deployed'])
    dev = read(E / 'development-project-resources.json')
    check('Native development inspection confirms schema81, current owned processes and no test databases',
          dev['passed'] and dev['migration_count'] == 81 and dev['backend_source_sha256'] == backend_digest
          and dev['database']['disposable_test_databases'] == 0 and dev['integrations_disabled']
          and dev['worker_sources_match'] and all(dev['ownership'].values()))
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    check('Scope retains23 open work packages and excludes AI fees only',
          scope['counts']['active'] == 23 and scope['counts']['excluded'] == 5
          and not scope['flags']['ai_cost_control_required'] and scope['flags']['ai_execution_isolation_required'])
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('Eight Skills remain drafts awaiting real model acceptance', len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    leftovers = [p.name for p in (APP / '.local').iterdir() if p.is_dir() and re.fullmatch(r'dependency-(?:remediation|security|maintenance)-[a-zA-Z0-9]{6}', p.name)]
    check('Owned synthetic security workspaces are removed after completed checks', leftovers == [])
    proof = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
             'interaction_kind': 'dependency_consumer_remediation_and_retained_media',
             'interaction_source_sha256': module('dependency_remediation_sources').digest(),
             'mobile_source_sha256': module('verify_mobile').digest()[0], 'backend_source_sha256': backend_digest,
             'verification_script_sha256': sha(Path(__file__)), 'new_browser_capture': False,
             'new_backend_test_execution': False, 'full_goal_complete': False, 'production_ready': False}
    (E / 'final-source-check.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': proof['passed'], 'checks': len(checks), 'failed': [v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
