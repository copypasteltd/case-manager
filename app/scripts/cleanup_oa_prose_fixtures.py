"""Remove only three inspected, invalid, inactive fixtures tied to failed native DROPs.

The inspection is private. Exact names, owners, connection state, prior failed
DROP statements and preserved first/final test outcomes must all match.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/oa-prose'
TARGETS = {
    'cp_test_4ecfa96f7539453cb53c1d78d36b4f1a': 'TestHistoryRestoration/waived_history_preserves_its_original_report_and_a_newer_disabled_policy_blocks_another_publication',
    'cp_test_ae891aff96514918b69bcabaf5c3e58d': 'TestHistoryRestoration/old_lease_cannot_persist_a_new_receipt_and_exhausted_unstarted_work_gets_a_domain_failure',
    'cp_test_fdb818926ea84aba9e1fd9ab8b64dc8d': 'TestPlatformIntegration/archived_projects_remain_read_only_and_cannot_close_with_open_tasks',
}


def main():
    values = json.loads((APP / '.local/environment.json').read_text())
    assert values['CP_ENV'] == 'development' and values['_postgres_port'] == 14660
    inspection = APP / '.local/oa-prose-orphan-inspection.json'
    rows = json.loads(inspection.read_text())
    assert len(rows) == 3 and {row['name'] for row in rows} == set(TARGETS)
    assert (APP / '.local/postgres/PG_VERSION').read_text().strip() == '18'
    log = (APP / '.local/postgres.log').read_text(encoding='utf-8', errors='replace')
    first = json.loads((E.parent / 'oa-prose-backend/attempt01/result.json').read_text())
    final = json.loads((E.parent / 'oa-prose-backend/result.json').read_text())
    assert not first['passed'] and first['source_unchanged_during_tests']
    assert final['passed'] and final['source_sha256'] == first['source_sha256']
    assert final['all_discovered_tests_executed']
    failed = {row['Test'].split('/')[0] for row in first['checks'] if row['Action'] == 'fail'}
    assert failed == {'TestHistoryRestoration', 'TestPlatformIntegration'}
    env = {**os.environ, 'PGHOST': '127.0.0.1', 'PGPORT': '14660', 'PGUSER': 'postgres',
        'PGPASSWORD': values['_superuser_password'], 'PGDATABASE': 'postgres', 'PGCONNECT_TIMEOUT': '5'}

    def query(sql):
        result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', sql],
            env=env, capture_output=True, text=True, encoding='utf-8', timeout=180, creationflags=subprocess.CREATE_NO_WINDOW)
        if result.returncode:
            (APP / '.local/oa-prose-cleanup-error.log').write_text(result.stderr, encoding='utf-8')
            raise SystemExit('Owned invalid fixture cleanup failed; diagnostics remain ignored.')
        return result.stdout.strip()

    outcomes = []
    for row in rows:
        name = row['name']
        assert re.fullmatch(r'cp_test_[a-f0-9]{32}', name) and len(name) == 40
        assert row['owner'] == 'postgres' and row['active_connections'] == 0 and row['connection_limit'] == -2
        assert row['drop_statement_in_owned_postgres_log']
        assert re.search(r'DROP DATABASE\s+"?' + re.escape(name) + r'"?\s+WITH\s*\(FORCE\)', log)
        assert row['failed_cleanup_test'] == TARGETS[name]
        assert row['failed_cleanup_test'] in {item['Test'] for item in first['checks'] if item['Action'] == 'fail'}
        times = re.findall(r'^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d+)[^\n]*DROP DATABASE\s+"?'
            + re.escape(name) + r'"?\s+WITH\s*\(FORCE\)', log, re.MULTILINE)
        failed_at = datetime.fromisoformat(row['cleanup_error_time']).replace(tzinfo=None)
        assert any(abs((datetime.strptime(value, '%Y-%m-%d %H:%M:%S.%f') - failed_at).total_seconds()) < 1 for value in times)
        current = json.loads(query("SELECT json_build_object('owner',pg_get_userbyid(datdba),'invalid',datconnlimit=-2,'connections',(SELECT count(*) FROM pg_stat_activity WHERE datid=d.oid)) FROM pg_database d WHERE datname='" + name + "'"))
        assert current == {'owner': 'postgres', 'invalid': True, 'connections': 0}
        query('DROP DATABASE "' + name + '" WITH (FORCE)')
        assert json.loads(query("SELECT count(*) FROM pg_database WHERE datname='" + name + "'")) == 0
        outcomes.append({'database': name, 'invalid_before': True, 'connections_before': 0,
            'exact_failed_drop_logged': True, 'failed_cleanup_test': row['failed_cleanup_test'],
            'cleanup_error_time': row['cleanup_error_time'], 'drop_log_time_matches': True, 'removed': True})
    record = {'passed': len(outcomes) == 3, 'checked_at': datetime.now(timezone.utc).isoformat(), 'targets': outcomes,
        'inspection_sha256': hashlib.sha256(inspection.read_bytes()).hexdigest(),
        'script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'production_database_touched': False, 'postgres_restarted': False, 'unrelated_database_touched': False}
    (E / 'fixture-cleanup.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': record['passed'], 'attested_invalid_fixtures_removed': len(outcomes), 'production_database_touched': False}))


if __name__ == '__main__':
    main()
