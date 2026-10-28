"""Finish the exact synthetic DB cleanup whose first attempt exceeded 120s.

The failed result and serial retry remain intact. Require the exact original
DROP statement, invalid-database marker, OID, creation window and no connections.
Invalid database contents cannot be inspected; no ledger/seed-read claim is made.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/finance-fx'
TARGET = 'cp_test_cf057a05af29475e8c9d092fb228a322'
OID = '91871926'
CASE = 'TestMiniappBrowserFixture'


def main():
    config = json.loads((APP / '.local/environment.json').read_text())
    assert os.name == 'nt' and config['CP_ENV'] == 'development' and config['_postgres_port'] == 14660
    assert '/' + TARGET not in config['CP_DATABASE_URL']
    folder = E.parent / 'finance-fx-backend'
    first = json.loads((folder / 'attempt01/result.json').read_text())
    final = json.loads((folder / 'result.json').read_text())
    assert not first['passed'] and final['passed'] and first['source_sha256'] == final['source_sha256']
    assert {v['test'] for v in final['retries']} == {CASE} and all(v['passed'] for v in final['retries'])
    progress = json.loads((folder / 'attempt01/batch-progress.json').read_text())
    batch = progress['batches'][4]
    raw = (APP / '.local' / Path(batch['raw_output']).name).read_bytes()
    assert hashlib.sha256(raw).hexdigest() == batch['raw_output_sha256']
    events = []
    for line in raw.decode('utf-8', errors='replace').splitlines():
        try:
            events.append(json.loads(line))
        except ValueError:
            pass
    starts = [v['Time'] for v in events if v.get('Test') == CASE and v.get('Action') == 'run']
    failures = [v['Time'] for v in events if v.get('Test') == CASE and v.get('Action') == 'fail']
    assert len(starts) == len(failures) == 1
    assert any('server_integration_test.go:86: timeout: context deadline exceeded' in v.get('Output', '') for v in events if v.get('Test') == CASE)
    started, failed = [datetime.fromisoformat(v) for v in (starts[0], failures[0])]
    created = datetime.fromtimestamp((APP / '.local/postgres/base' / OID).stat().st_birthtime, timezone.utc)
    assert started <= created <= failed
    fixture = 'backend/internal/httpapi/miniapp_fixture_test.go'
    assert (APP / fixture).read_bytes() == subprocess.check_output(['git', 'show', '624c95d86f737e679e11af4fbb505dedf8e27b71:app/' + fixture], cwd=APP)
    env = {**os.environ, 'PGHOST': '127.0.0.1', 'PGPORT': '14660', 'PGUSER': 'postgres',
        'PGPASSWORD': config['_superuser_password'], 'PGCONNECT_TIMEOUT': '5'}

    def sql(database, query):
        result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
            env={**env, 'PGDATABASE': database}, capture_output=True, text=True, encoding='utf-8', timeout=90,
            creationflags=subprocess.CREATE_NO_WINDOW)
        if result.returncode:
            (APP / '.local/finance-fx-timeout-cleanup-error.log').write_text(result.stderr, encoding='utf-8')
        assert result.returncode == 0, 'Exact synthetic cleanup failed; private diagnostics withheld.'
        return result.stdout.strip()

    before = json.loads(sql('postgres', "SELECT json_build_object('oid',oid::text,'owner',pg_get_userbyid(datdba),'template',datistemplate,"
        "'connection_limit',datconnlimit,'connections',(SELECT count(*) FROM pg_stat_activity WHERE datid=d.oid)) FROM pg_database d WHERE datname='" + TARGET + "'"))
    assert before == {'oid': OID, 'owner': 'postgres', 'template': False, 'connection_limit': -2, 'connections': 0}
    statement = 'STATEMENT:  DROP DATABASE "' + TARGET + '" WITH (FORCE)'
    lines = [v for v in (APP / '.local/postgres.log').read_text(encoding='utf-8', errors='replace').splitlines() if statement in v]
    assert len(lines) == 1
    # Native log and Go timestamps both use this host's +08:00 clock.
    logged = datetime.strptime(lines[0][:23], '%Y-%m-%d %H:%M:%S.%f').replace(tzinfo=failed.tzinfo)
    assert 0 <= (logged - failed).total_seconds() <= 5
    assert sql('postgres', "SELECT count(*) FROM pg_stat_activity WHERE datid='" + OID + "'::oid") == '0'
    sql('postgres', 'DROP DATABASE "' + TARGET + '"')
    absent = sql('postgres', "SELECT count(*) FROM pg_database WHERE datname='" + TARGET + "'") == '0'
    remaining = int(sql('postgres', "SELECT count(*) FROM pg_database WHERE datname~'^cp_(test|suite|batch|fixture)_'"))
    record = {'passed': absent and remaining == 0, 'checked_at': datetime.now(timezone.utc).isoformat(), 'database': TARGET,
        'test': CASE, 'test_started_at': starts[0], 'test_failed_at': failures[0], 'database_directory_created_at': created.isoformat(),
        'creation_within_failed_case': True, 'original_batch_log_sha256': batch['raw_output_sha256'], 'before': before,
        'invalid_database_marker_verified': True, 'exact_original_drop_statement': lines[0],
        'drop_statement_time_matches_failure': True, 'contents_accessible': False, 'ledger_read_claimed': False,
        'published_fixture_source_matched': True, 'suite_source_sha256': final['source_sha256'],
        'removed': absent, 'remaining_test_databases': remaining,
        'forced_disconnect': False, 'postgres_restarted': False, 'production_database_touched': False,
        'script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (E / 'timeout-fixture-cleanup.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': record['passed'], 'exact_timed_out_fixture_removed': absent, 'remaining_test_databases': remaining}))


if __name__ == '__main__':
    main()
