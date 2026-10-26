"""Bind notification evidence to current code and verify owned-resource cleanup."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess
import os
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/notifications'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def module(name, file):
    spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / file)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode('utf-8') + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def owned_process(name, command):
    record = read(APP / '.local' / name)
    pid = int(record['pid'])
    code = "$p=Get-CimInstance Win32_Process -Filter 'ProcessId = " + str(pid) + "'; if($p -and $p.Name -eq 'python.exe' -and $p.CommandLine -like '*app/scripts/run_backend.py " + command + "*') { @{valid=$true;created_at=$p.CreationDate.ToUniversalTime().ToString('o')} | ConvertTo-Json -Compress } else { '{\"valid\":false}' }"
    result = subprocess.run(['powershell.exe', '-NoProfile', '-NonInteractive', '-Command', code], capture_output=True, text=True, timeout=15, creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if result.returncode:
        return False
    actual = json.loads(result.stdout)
    return actual['valid'] and abs((datetime.fromisoformat(actual['created_at']) - datetime.fromisoformat(record['started_at'])).total_seconds()) < 10


def main():
    checks = []
    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})
    backend = module('notifications_backend', 'verify_backend_batches.py').digest()
    runtime = module('notifications_runtime', 'verify_agent_runtime.py').digest()[0]
    mobile = module('notifications_mobile', 'verify_mobile.py').digest()[0]
    b = read(EVIDENCE / 'backend-result.json')
    check('Every discovered backend test passes against current sources', b['passed'] and b['source_sha256'] == backend and b['source_unchanged_during_tests'] and b['all_discovered_tests_executed'] and all(c['Action'] == 'pass' for c in b['checks']))
    linux = read(EVIDENCE / 'linux-build.json')
    check('Linux compilation matches current backend; artifact was not executed or deployed', linux['passed'] and linux['source_sha256'] == backend and not linux['artifact_executed'] and not linux['deployed'])
    w = read(EVIDENCE / 'web-build.json')
    check('Web tests and production build match current source manifest', w['passed'] and w['source_unchanged_during_run'] and w['source_sha256'] == digest(w['sources']))
    m = read(EVIDENCE / 'mobile-build.json')
    check('Mobile tests and both mini-program compilation targets match current sources', m['passed'] and m['source_unchanged_during_run'] and m['source_sha256'] == mobile and all(c['exit_code'] == 0 for c in m['commands']))
    r = read(EVIDENCE / 'runtime-build.json')
    check('Runtime tests and eight draft skills match current sources', r['status'] == 'passed' and r['source_sha256'] == runtime and r['source_after_sha256'] == runtime)
    for group in ('notifications-web', 'notifications-mobile'):
        folder = EVIDENCE.parent / group
        result, manifest = read(folder / 'result.json'), read(folder / 'source-manifest.json')
        current = digest(manifest['files'])
        check(group + ' real browser evidence matches current source manifest', result['passed'] and result['source_unchanged'] and result['source_sha256'] == current == manifest['sha256'] and not result['console_errors'] and not result['server_errors'])
        check(group + ' owned browser, fixture and temporary server closed', result['owned_browser_closed'] and result['browser_contexts_closed'] > 0 and result['isolated_database_cleanup_passed'] and result['native_fixture_exit_code'] == 0 and (result.get('owned_vite_stopped') is True if group.endswith('-web') else result.get('owned_review_server_stopped') is True))
        if group.endswith('-mobile'):
            check('Mobile fixture, H5 build and browser used the same sources', result['build_and_browser_sources_match'] and result['source_sha256_before_fixture_and_build'] == result['source_sha256_after_build'] == result['source_sha256'])
    development = read(EVIDENCE / 'development-notifications.json')
    check('Owned development schema, actual worker receipts, live routes and database cleanup', development['passed'] and development['migration_count'] == 52 and development['database']['disposable_test_databases'] == 0 and development['database']['invalid_job_results'] == 0 and development['database']['notification_guards'] == 10 and development['database']['actual_in_app_deliveries'] > 0 and development['database']['append_only_delivery_tables'])
    check('Development verification script matches its recorded source', development['verification_script_sha256'] == hashlib.sha256((APP / 'scripts/verify_notifications_development.py').read_bytes()).hexdigest())
    check('Documented and embedded OpenAPI copies match', (APP / 'backend/internal/contract/openapi.json').read_bytes() == (APP.parent / 'docs/development/openapi.json').read_bytes())
    check('Private browser fixture removed', not (APP / '.local/business-browser-fixture.json').exists())
    check('Owned API and notification worker processes match recorded identity and start time', owned_process('notifications-api-owner.json', 'api') and owned_process('notifications-worker-owner.json', 'worker-notification'))
    with socket.socket() as probe:
        probe.settimeout(1)
        check('Owned temporary review port released', probe.connect_ex(('127.0.0.1', 5189)) != 0)
    for url in ('http://localhost:5178/', 'http://localhost:5178/health/ready', 'http://127.0.0.1:18080/health/ready'):
        try:
            with urlopen(url, timeout=5) as response:
                passed = response.status == 200
        except OSError:
            passed = False
        check(url, passed)
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
              'backend_source_sha256': backend, 'scope': 'Current-source native implementation, actual business-event notifications, controlled browser operation and owned cleanup; excludes production, devices, external messaging, real cloud/model and capacity acceptance.'}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
