"""Bind current task views to builds, actual browser chains and owned cleanup."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/project-task-views'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / filename)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


def main():
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    previous = EVIDENCE.parent / 'project-templates'
    backend = module('task_view_backend', 'verify_backend_batches.py').digest()
    result = read(previous / 'result.json')
    check('Unchanged backend reuses complete 141-test 861-result native regression',
          result['passed'] and result['source_sha256'] == backend
          and result['source_unchanged_during_tests'] and result['all_discovered_tests_executed']
          and result['discovered_top_level_tests'] == 141 and len(result['checks']) == 861
          and all(item['Action'] == 'pass' for item in result['checks']))
    linux = read(previous / 'linux-build.json')
    artifact = APP / linux['artifact']
    check('Unchanged backend retains verified Linux artifact without executing or deploying it',
          linux['passed'] and linux['source_sha256'] == backend and artifact.exists()
          and hashlib.sha256(artifact.read_bytes()).hexdigest() == linux['artifact_sha256']
          and not linux['artifact_executed'] and not linux['deployed'])
    web = read(EVIDENCE / 'web-build.json')
    check('Current Web source passes 68 tests, typecheck and production build',
          web['passed'] and web['source_unchanged_during_run'] and web['source_sha256'] == digest(web['sources'])
          and web['tests']['numPassedTests'] == 68
          and web['tests']['numFailedTests'] == web['tests']['numPendingTests'] == 0)
    folder = EVIDENCE.parent / 'project-task-views-web'
    browser, manifest = read(folder / 'result.json'), read(folder / 'source-manifest.json')
    check('Four actual project/task/dependency/permission browser chains match current sources',
          browser['passed'] and browser['real_web_api'] and browser['isolated_native_database']
          and browser['source_unchanged_during_tests'] and browser['source_sha256'] == digest(manifest['files'])
          and len(browser['checks']) == 4 and all(item['passed'] for item in browser['checks'])
          and not browser['failure'] and not browser['errors'] and not browser['server_errors'])
    check('Board and timeline pass all six settled responsive layouts',
          len(browser['layouts']) == 6 and all(item['passed'] for item in browser['layouts']))
    check('Owned browser, tabs, native fixture database and temporary Vite closed',
          browser['owned_browser_closed'] and browser['all_owned_browser_tabs_closed']
          and browser['native_fixture_exit_code'] == 0 and browser['isolated_database_cleanup_passed']
          and browser['owned_vite_stopped'] and not browser['cleanup_errors'])
    mobile = read(previous / 'mobile-build.json')
    check('Unchanged mobile/shared sources reuse passing 79-test and both-mini-program evidence',
          mobile['passed'] and mobile['source_unchanged_during_run']
          and mobile['source_sha256'] == module('task_view_mobile', 'verify_mobile.py').digest()[0]
          and mobile['tests']['pass'] == 79 and mobile['tests']['fail'] == mobile['tests']['skipped'] == mobile['tests']['cancelled'] == 0)
    runtime = read(EVIDENCE.parent / 'android-native/runtime-build.json')
    check('Unchanged controlled runtime retains passing source-bound 34-test evidence',
          runtime['status'] == 'passed' and runtime['source_sha256'] == runtime['source_after_sha256']
          == module('task_view_runtime', 'verify_agent_runtime.py').digest()[0] and runtime['tests']['pass'] == 34)
    apk_build = read(EVIDENCE.parent / 'android-upload/build.json')
    apk = APP / apk_build['artifacts']['apk']['path']
    check('Previous frozen native upload APK remains intact without a new build/device claim',
          apk_build['passed'] and apk.exists() and hashlib.sha256(apk.read_bytes()).hexdigest() == apk_build['artifacts']['apk']['sha256']
          and not apk_build['production_release'] and not apk_build['device_executed'])
    dev, base = read(EVIDENCE / 'development-project-templates.json'), read(EVIDENCE / 'development-runtime.json')
    check('Owned development uses matching 64-migration manifest with no disposable DB residue',
          dev['passed'] and base['passed'] and base['database']['migration_count'] == 64
          and base['database']['migration_manifest_matches_source'] and dev['database']['disposable_test_databases'] == 0)
    check('Development cloud/model/TG integrations remain disabled',
          dev['integrations_disabled'] and dev['database']['active_runtime_profiles'] == dev['database']['enabled_telegram_channels'] == 0)
    owner = module('task_view_owner', 'verify_notifications_sources.py').owned_process
    check('Owned API and notification-worker creation times still match increment67 records',
          owner('project-templates-api-owner.json', 'api') and owner('project-templates-worker-owner.json', 'worker-notification'))
    check('Embedded and documented OpenAPI remain identical',
          (APP / 'backend/internal/contract/openapi.json').read_bytes() == (APP.parent / 'docs/development/openapi.json').read_bytes())
    with socket.socket() as probe:
        probe.settimeout(1)
        check('Private browser fixture removed and temporary review port released',
              probe.connect_ex(('127.0.0.1', 5189)) != 0 and not (APP / '.local/business-browser-fixture.json').exists())
    for address in ('http://127.0.0.1:18080/health/ready', 'http://localhost:5178/health/ready', 'http://localhost:5178/'):
        try:
            with urlopen(address, timeout=5) as response:
                ready = response.status == 200
        except OSError:
            ready = False
        check('Owned daily development service remains ready: ' + address, ready)
    audit = read(EVIDENCE.parent / 'android-native/dependency-audit.json')
    check('Dependency lock unchanged; previous unresolved audit findings remain open',
          hashlib.sha256((APP / 'package-lock.json').read_bytes()).hexdigest() == audit['package_lock_sha256']
          and not audit['production_security_gate_passed'])
    report = {'passed': all(item['passed'] for item in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
              'backend_source_sha256': backend, 'checks': checks,
              'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'full_goal_complete': False, 'production_deployed': False, 'real_cloud_verified': False,
              'real_model_verified': False, 'real_device_verified': False,
              'reused_evidence': {'backend': 'increment67 unchanged', 'mobile': 'increment67 unchanged',
                                  'runtime': 'increment65 unchanged', 'android': 'increment66 frozen APK'},
              'scope': 'Web project task board, basic task timeline and actual existing business controls; full project aggregation, mobile parity and production first release remain open.'}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [item['name'] for item in checks if not item['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
