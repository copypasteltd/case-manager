"""Finish one exact synthetic fixture abandoned by the preserved Go timeout.

Requires its creation window, exact migration ledger, standard synthetic seed,
empty commercial records and zero live connections. No FORCE or PG restart.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/oa-prose'
TARGET = 'cp_test_773b40dd46dd46c79dc05dfdb0f322e7'
OID = '88587012'
CASE = 'TestMiniappProofBoundaries'


def main():
    config = json.loads((APP / '.local/environment.json').read_text())
    assert os.name == 'nt' and config['CP_ENV'] == 'development' and config['_postgres_port'] == 14660
    assert '/' + TARGET not in config['CP_DATABASE_URL']
    first = json.loads((E.parent / 'oa-prose-backend/attempt01/result.json').read_text())
    final = json.loads((E.parent / 'oa-prose-backend/result.json').read_text())
    assert not first['passed'] and final['passed'] and final['source_sha256'] == first['source_sha256']
    assert CASE in {name for _, name in final['first_attempt_missing_tests']}
    progress = json.loads((E.parent / 'oa-prose-backend/attempt01/batch-progress.json').read_text())
    batch = progress['batches'][4]
    raw = APP / '.local' / Path(batch['raw_output']).name
    assert hashlib.sha256(raw.read_bytes()).hexdigest() == batch['raw_output_sha256']
    events = []
    for line in raw.read_text(encoding='utf-8', errors='replace').splitlines():
        try:
            events.append(json.loads(line))
        except ValueError:
            pass
    starts = [row['Time'] for row in events if row.get('Action') == 'run' and row.get('Test') == CASE]
    panics = [row['Time'] for row in events if 'panic: test timed out after 20m0s' in row.get('Output', '')]
    assert len(starts) == len(panics) == 1
    started, interrupted = [datetime.fromisoformat(value) for value in (starts[0], panics[0])]
    created = datetime.fromtimestamp((APP / '.local/postgres/base' / OID).stat().st_ctime, timezone.utc)
    assert started <= created <= interrupted
    env = {**os.environ, 'PGHOST': '127.0.0.1', 'PGPORT': '14660', 'PGUSER': 'postgres',
        'PGPASSWORD': config['_superuser_password'], 'PGCONNECT_TIMEOUT': '5'}

    def sql(database, query):
        result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q',
            '-v', 'ON_ERROR_STOP=1', '-c', query], env={**env, 'PGDATABASE': database},
            capture_output=True, text=True, encoding='utf-8', timeout=60, creationflags=subprocess.CREATE_NO_WINDOW)
        assert result.returncode == 0, 'Exact fixture operation failed; private details withheld.'
        return result.stdout.strip()

    before = json.loads(sql('postgres', "SELECT json_build_object('oid',oid::text,'owner',pg_get_userbyid(datdba),"
        "'template',datistemplate,'connections',(SELECT count(*) FROM pg_stat_activity WHERE datid=d.oid)) FROM pg_database d WHERE datname='" + TARGET + "'"))
    assert before == {'oid': OID, 'owner': 'postgres', 'template': False, 'connections': 0}
    shape = json.loads(sql(TARGET, "SELECT json_build_object('organizations',(SELECT json_agg(name) FROM organizations),"
        "'users',(SELECT json_agg(login_key ORDER BY login_key) FROM users),"
        "'projects',(SELECT count(*) FROM projects),'files',(SELECT count(*) FROM files),"
        "'contracts',(SELECT count(*) FROM contracts),'spend',(SELECT count(*) FROM spend_documents),"
        "'requests',(SELECT count(*) FROM requests),'agent_runs',(SELECT count(*) FROM agent_runs),"
        "'proofs',(SELECT count(*) FROM miniapp_binding_proofs),'bindings',(SELECT count(*) FROM miniapp_bindings))"))
    assert shape == {'organizations': ['Test CopyPaste'], 'users': ['alice', 'bob', 'member', 'sysadmin'],
        'projects': 0, 'files': 0, 'contracts': 0, 'spend': 0, 'requests': 0, 'agent_runs': 0, 'proofs': 1, 'bindings': 0}
    applied = json.loads(sql(TARGET, "SELECT json_agg(json_build_object('name',name,'sha256',sha256) ORDER BY name) FROM schema_migrations"))
    expected = [{'name': path.name, 'sha256': hashlib.sha256(path.read_bytes()).hexdigest()}
        for path in sorted((APP / 'backend/migrations').glob('*.sql'))]
    assert len(applied) == 80 and applied == expected
    assert sql('postgres', "SELECT count(*) FROM pg_stat_activity WHERE datid='" + OID + "'::oid") == '0'
    sql('postgres', 'DROP DATABASE "' + TARGET + '"')
    absent = sql('postgres', "SELECT count(*) FROM pg_database WHERE datname='" + TARGET + "'") == '0'
    record = {'passed': absent, 'checked_at': datetime.now(timezone.utc).isoformat(), 'database': TARGET,
        'test': CASE, 'test_started_at': starts[0], 'test_interrupted_at': panics[0],
        'database_directory_created_at': created.isoformat(), 'creation_within_interrupted_case': True,
        'original_batch_log_sha256': batch['raw_output_sha256'], 'before': before,
        'standard_synthetic_seed_verified': True, 'commercial_records_empty': True,
        'source_migration_ledger_matched': True, 'migration_count': 80, 'removed': absent,
        'forced_disconnect': False, 'postgres_restarted': False, 'production_database_touched': False,
        'script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (E / 'interrupted-fixture-cleanup.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': absent, 'exact_interrupted_fixture_removed': absent, 'forced_disconnect': False}))


if __name__ == '__main__':
    main()
