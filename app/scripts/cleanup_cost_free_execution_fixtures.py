"""Finish only exact invalid native fixture databases from attested failed DROPs.

Never selects arbitrary databases for removal: every target must have an earlier
read-only inspection, an exact DROP statement in this owned cluster's log, the
fixture name/owner, PostgreSQL's invalid flag, and zero live connections.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/cost-free-execution'


def main():
    values = json.loads((APP / '.local/environment.json').read_text())
    assert values['CP_ENV'] == 'development' and values['_postgres_port'] == 14660
    inspected = APP / '.local/cost-free-orphan-inspection.json'
    rows = json.loads(inspected.read_text())
    assert len(rows) == 2
    assert (APP / '.local/postgres/PG_VERSION').read_text().strip() == '18'
    log = (APP / '.local/postgres.log').read_text(encoding='utf-8', errors='replace')
    first = json.loads((E.parent / 'cost-free-execution-backend/attempt01/result.json').read_text())
    assert not first['passed'] and first['source_unchanged_during_tests']
    assert {r['Test'] for r in first['checks'] if r['Action'] == 'fail'} == {
        'TestCommentAttachmentPreservation', 'TestDownloadPermitExpiryAndClassificationChanges'}
    env = {**os.environ, 'PGHOST': '127.0.0.1', 'PGPORT': '14660', 'PGUSER': 'postgres',
        'PGPASSWORD': values['_superuser_password'], 'PGDATABASE': 'postgres', 'PGCONNECT_TIMEOUT': '5'}

    def query(sql):
        result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', sql],
            env=env, capture_output=True, text=True, encoding='utf-8', timeout=180, creationflags=subprocess.CREATE_NO_WINDOW)
        if result.returncode:
            (APP / '.local/cost-free-cleanup-error.log').write_text(result.stderr, encoding='utf-8')
            raise SystemExit('Owned invalid fixture cleanup failed; diagnostics remain ignored.')
        return result.stdout.strip()

    outcomes = []
    for row in rows:
        name = row['name']
        assert re.fullmatch(r'cp_test_[a-f0-9]{32}', name) and len(name) == 40
        assert row['owner'] == 'postgres' and row['active_connections'] == 0 and row['connection_limit'] == -2
        assert row['drop_statement_in_owned_postgres_log'] and re.search(r'DROP DATABASE\s+"?' + re.escape(name) + r'"?\s+WITH\s*\(FORCE\)', log)
        current = json.loads(query("SELECT json_build_object('owner',pg_get_userbyid(datdba),'invalid',datconnlimit=-2,'connections',(SELECT count(*) FROM pg_stat_activity WHERE datid=d.oid)) FROM pg_database d WHERE datname='" + name + "'"))
        assert current == {'owner': 'postgres', 'invalid': True, 'connections': 0}
        query('DROP DATABASE "' + name + '" WITH (FORCE)')
        assert json.loads(query("SELECT count(*) FROM pg_database WHERE datname='" + name + "'")) == 0
        outcomes.append({'database': name, 'invalid_before': True, 'connections_before': 0, 'exact_failed_drop_logged': True, 'removed': True})
    result = {'passed': len(outcomes) == 2, 'checked_at': datetime.now(timezone.utc).isoformat(), 'targets': outcomes,
        'inspection_sha256': hashlib.sha256(inspected.read_bytes()).hexdigest(), 'script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'production_database_touched': False, 'postgres_restarted': False, 'unrelated_database_touched': False}
    (E / 'fixture-cleanup.json').write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'attested_invalid_fixtures_removed': len(outcomes), 'production_database_touched': False}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
