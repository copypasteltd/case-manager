"""Remove only exact inactive synthetic fixtures attributable to this failed run."""
from datetime import datetime, timedelta, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
from urllib.parse import urlsplit

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/notification-retry'
B = E.parent / 'notification-retry-backend'
# Filled only after the completed-run catalog inspection; never select by prefix alone.
EXPECTED_DATABASES = {'cp_test_ff929bf581e4405db43ab073f396aaf4'}


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    assert EXPECTED_DATABASES and not (E / 'fixture-cleanup.json').exists()
    assert not (APP / '.local/business-browser-fixture.json').exists()
    flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
    probe = subprocess.run(['powershell.exe', '-NoProfile', '-NonInteractive', '-Command',
        r"@(Get-CimInstance Win32_Process | Where-Object {$_.Name -eq 'python.exe' -and $_.CommandLine -match 'app[\\/]scripts[\\/]test_backend.py'}).Count | ConvertTo-Json -Compress"],
        capture_output=True, text=True, timeout=20, creationflags=flags)
    assert probe.returncode == 0 and json.loads(probe.stdout) == 0, 'Preserve every running native test phase.'
    original, state = read(B / 'result.json'), read(B / 'batch-progress.json')
    assert original['all_discovered_tests_executed'] and original['source_unchanged_during_tests']
    assert all(v['status'] in ('passed', 'failed') for v in state['batches'])
    values = read(APP / '.local/environment.json')
    target = urlsplit(values['CP_DATABASE_URL'])
    assert values['CP_ENV'] == 'development' and target.hostname == '127.0.0.1' and target.port == 14660
    environment = {**os.environ, 'PGHOST': '127.0.0.1', 'PGPORT': '14660', 'PGUSER': 'postgres',
        'PGPASSWORD': values['_superuser_password'], 'PGCONNECT_TIMEOUT': '5'}

    def sql(query, database='postgres', parse=True):
        result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
            env={**environment, 'PGDATABASE': database}, capture_output=True, text=True, encoding='utf-8', timeout=300, creationflags=flags)
        if result.returncode:
            (APP / '.local/notification-retry-cleanup-error.log').write_text(query + '\n' + result.stderr, encoding='utf-8')
            raise SystemExit('Fixture cleanup stopped; private database diagnostics withheld.')
        return json.loads(result.stdout) if parse else None

    windows, raw_records = [], []
    private = APP / '.local/notification-retry-fixture-cleanup'
    private.mkdir(parents=True, exist_ok=False)
    for number, batch in enumerate(state['batches'], 1):
        if batch['status'] != 'failed':
            continue
        raw = APP / '.local' / ('notification-retry-backend-batch-' + str(number) + '.log')
        assert sha(raw) == batch['raw_output_sha256']
        events = []
        for line in raw.read_text(encoding='utf-8').splitlines():
            try:
                events.append(json.loads(line))
            except json.JSONDecodeError:
                pass
        errors = [v.get('Output', '') for v in events if re.search(r'\.go:\d+:', v.get('Output', ''))]
        assert errors and all('server_integration_test.go:86: timeout: context deadline exceeded' in v for v in errors)
        for failure in (v for v in events if v.get('Action') == 'fail' and v.get('Test')):
            start = next(v['Time'] for v in events if v.get('Action') == 'run' and v.get('Test') == failure['Test'])
            windows.append({'test': failure['Test'], 'from': datetime.fromisoformat(start) - timedelta(seconds=2), 'to': datetime.fromisoformat(failure['Time']) + timedelta(seconds=2)})
        saved = private / raw.name
        shutil.copyfile(raw, saved)
        raw_records.append({'file': saved.relative_to(ROOT).as_posix(), 'sha256': sha(saved)})
    browser_attempt = E / 'attempts/07-web-cleanup-timeout/attempt.json'
    browser = read(browser_attempt)
    assert not browser['passed'] and browser['native_fixture_exit_code'] == 1
    assert all(sha(ROOT / v['file']) == v['sha256'] for v in browser['raw_logs'])
    windows.append({'test': 'TestNotificationManualRetries/browser-cleanup',
        'from': datetime.fromisoformat(browser['fixture_window']['from']), 'to': datetime.fromisoformat(browser['fixture_window']['to'])})
    query = "SELECT coalesce(json_agg(json_build_object('oid',d.oid::text,'name',datname,'owner',pg_get_userbyid(datdba),'invalid_drop_marker',datconnlimit=-2,'template',datistemplate,'active_connections',(SELECT count(*) FROM pg_stat_activity WHERE datid=d.oid),'created',(pg_stat_file('base/'||d.oid,true)).creation)),'[]'::json) FROM pg_database d WHERE datname ~ '^cp_test_[a-f0-9]{32}$'"
    candidates = sql(query)
    assert {v['name'] for v in candidates} == EXPECTED_DATABASES
    for row in candidates:
        assert re.fullmatch(r'cp_test_[a-f0-9]{32}', row['name']) and re.fullmatch(r'[0-9]+', row['oid'])
        assert row['owner'] == 'postgres' and not row['template'] and row['active_connections'] == 0 and row['created']
        created = datetime.fromisoformat(row['created'])
        matched = [v['test'] for v in windows if v['from'] <= created <= v['to']]
        assert matched, 'Unknown fixture ownership; leave it intact.'
        row.update(matched_failed_tests=matched, contents_inspected=False)
        if not row['invalid_drop_marker']:
            metadata = sql("SELECT json_build_object('schema85',(SELECT count(*)=85 FROM application_schema_manifest()),'synthetic_browser_retry',EXISTS(SELECT 1 FROM notification_retries WHERE reason='Synthetic diagnosed notification retry'))", database=row['name'])
            assert all(metadata.values()), 'A normal database must independently match this synthetic browser fixture.'
            row.update(contents_inspected=True, synthetic_metadata=metadata)
    initial = E / 'fixture-cleanup-input.json'
    shutil.copyfile(B / 'result.json', initial)
    plan = {'candidates': candidates, 'raw_logs': raw_records, 'backend_record_sha256': sha(initial),
        'browser_attempt_sha256': sha(browser_attempt), 'force': False, 'production_database_selected': False}
    (E / 'fixture-cleanup-plan.json').write_text(json.dumps(plan, indent=2) + '\n', encoding='utf-8')
    for row in candidates:
        current = sql("SELECT json_build_object('name',datname,'invalid_drop_marker',datconnlimit=-2,'active_connections',(SELECT count(*) FROM pg_stat_activity WHERE datid=d.oid)) FROM pg_database d WHERE oid=" + row['oid'])
        assert current['name'] == row['name'] and current['active_connections'] == 0 and current['invalid_drop_marker'] == row['invalid_drop_marker']
        sql('DROP DATABASE "' + row['name'] + '"', parse=False)
        print('Removed one individually verified inactive synthetic fixture.', flush=True)
    remaining = sql(query)
    report = {'passed': not remaining, 'checked_at': datetime.now(timezone.utc).isoformat(), 'removed': candidates,
        'remaining_disposable_databases': len(remaining), 'force_used': False, 'development_database_preserved': True,
        'database_cluster_restarted': False, 'plan_sha256': sha(E / 'fixture-cleanup-plan.json'), 'verification_script_sha256': sha(Path(__file__))}
    (E / 'fixture-cleanup.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'removed': len(candidates), 'remaining': len(remaining)}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
