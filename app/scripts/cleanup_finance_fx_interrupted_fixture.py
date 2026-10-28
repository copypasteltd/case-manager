"""Remove one proven synthetic fixture left by the retained native interruption.

Match its exact OID, creation window, immutable migration ledger, synthetic seed
and restoration-fixture shape. Refuse live connections; never FORCE or restart.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/finance-fx'
TARGET = 'cp_test_eacc81fb0e6e4328a063bc6e14494e02'
OID = '89465660'
CASE = 'TestAgentFinanceSummary'


def main():
    values = json.loads((APP / '.local/environment.json').read_text())
    assert os.name == 'nt' and values['CP_ENV'] == 'development' and values['_postgres_port'] == 14660
    assert '/' + TARGET not in values['CP_DATABASE_URL']
    interruption = json.loads((E.parent / 'finance-fx-backend/attempts/native-process-interruption/interruption.json').read_text())
    assert interruption['classification'] == 'interrupted' and not interruption['passed']
    raw = (APP / '.local/finance-fx-interrupted-batch-1.log').read_bytes()
    assert hashlib.sha256(raw).hexdigest() == interruption['raw_log_sha256']
    events = []
    for line in raw.decode('utf-8', errors='replace').splitlines():
        try:
            events.append(json.loads(line))
        except ValueError:
            pass
    starts = [v['Time'] for v in events if v.get('Action') == 'run' and v.get('Test') == CASE]
    assert len(starts) == 1 and not any(v.get('Test') == CASE and v.get('Action') in ('pass', 'fail') for v in events)
    started, observed = [datetime.fromisoformat(v) for v in (starts[0], interruption['observed_at'])]
    created = datetime.fromtimestamp((APP / '.local/postgres/base' / OID).stat().st_birthtime, timezone.utc)
    assert started <= created <= observed
    original_test = subprocess.check_output(['git', 'show', '624c95d86f737e679e11af4fbb505dedf8e27b71:app/backend/internal/httpapi/agent_finance_summary_integration_test.go'], cwd=APP)
    assert original_test == (APP / 'backend/internal/httpapi/agent_finance_summary_integration_test.go').read_bytes()
    env = {**os.environ, 'PGHOST': '127.0.0.1', 'PGPORT': '14660', 'PGUSER': 'postgres',
           'PGPASSWORD': values['_superuser_password'], 'PGCONNECT_TIMEOUT': '5'}

    def sql(database, query):
        result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q',
            '-v', 'ON_ERROR_STOP=1', '-c', query], env={**env, 'PGDATABASE': database},
            capture_output=True, text=True, encoding='utf-8', timeout=60, creationflags=subprocess.CREATE_NO_WINDOW)
        assert result.returncode == 0, 'Exact synthetic fixture operation failed; private diagnostics withheld.'
        return result.stdout.strip()

    before = json.loads(sql('postgres', "SELECT json_build_object('oid',oid::text,'owner',pg_get_userbyid(datdba),"
        "'template',datistemplate,'connections',(SELECT count(*) FROM pg_stat_activity WHERE datid=d.oid)) FROM pg_database d WHERE datname='" + TARGET + "'"))
    assert before == {'oid': OID, 'owner': 'postgres', 'template': False, 'connections': 0}
    shape = json.loads(sql(TARGET, "SELECT json_build_object('organizations',(SELECT json_agg(name) FROM organizations),"
        "'users',(SELECT json_agg(login_key ORDER BY login_key) FROM users),"
        "'projects',(SELECT count(*) FROM projects),'files',(SELECT count(*) FROM files),"
        "'versions',(SELECT count(*) FROM file_versions),'uploads',(SELECT count(*) FROM upload_sessions),"
        "'contracts',(SELECT count(*) FROM contracts),'spend',(SELECT count(*) FROM spend_documents),"
        "'parties',(SELECT count(*) FROM parties),'costs',(SELECT count(*) FROM actual_cost_entries),"
        "'payments',(SELECT count(*) FROM payment_records),'subscriptions',(SELECT count(*) FROM subscriptions),"
        "'requests',(SELECT count(*) FROM requests),'agent_runs',(SELECT count(*) FROM agent_runs))"))
    assert shape == {'organizations': ['Test CopyPaste'], 'users': ['alice', 'bob', 'member', 'sysadmin'],
        'projects': 1, 'files': 1, 'versions': 2, 'uploads': 2, 'contracts': 0, 'spend': 0,
        'parties': 0, 'costs': 0, 'payments': 0, 'subscriptions': 0, 'requests': 0, 'agent_runs': 0}
    applied = json.loads(sql(TARGET, "SELECT json_agg(json_build_object('name',name,'sha256',sha256) ORDER BY name) FROM schema_migrations"))
    expected = [{'name': p.name, 'sha256': hashlib.sha256(p.read_bytes()).hexdigest()}
                for p in sorted((APP / 'backend/migrations').glob('*.sql'))]
    assert len(applied) == 81 and applied == expected
    assert sql('postgres', "SELECT count(*) FROM pg_stat_activity WHERE datid='" + OID + "'::oid") == '0'
    sql('postgres', 'DROP DATABASE "' + TARGET + '"')
    absent = sql('postgres', "SELECT count(*) FROM pg_database WHERE datname='" + TARGET + "'") == '0'
    record = {'passed': absent, 'checked_at': datetime.now(timezone.utc).isoformat(), 'database': TARGET,
        'test': CASE, 'test_started_at': starts[0], 'interruption_observed_at': interruption['observed_at'],
        'database_directory_created_at': created.isoformat(), 'creation_within_interrupted_case': True,
        'original_batch_log_sha256': interruption['raw_log_sha256'], 'before': before,
        'published_fixture_source_matched': True, 'standard_synthetic_seed_verified': True,
        'restoration_fixture_shape_verified': True, 'commercial_records_empty': True,
        'source_migration_ledger_matched': True, 'migration_count': 81, 'removed': absent,
        'forced_disconnect': False, 'postgres_restarted': False, 'production_database_touched': False,
        'script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (E / 'interrupted-fixture-cleanup.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': absent, 'exact_interrupted_fixture_removed': absent, 'forced_disconnect': False}))


if __name__ == '__main__':
    main()
