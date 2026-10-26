"""Clean only the exact empty fixture left during the interrupted migration.

No live connections are terminated. The two applied migration hashes and empty
organization/user tables must match this task's native integration fixture.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
from urllib.parse import urlsplit

APP = Path(__file__).resolve().parents[1]
TARGET = 'cp_test_521c1eb70f4a41c0b900acdb9bd545f2'
EVIDENCE = APP.parent / 'docs/quality/evidence/project-templates'


def main():
    config = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    db = urlsplit(config['CP_DATABASE_URL'])
    interruption = json.loads((EVIDENCE / 'interruption.json').read_text(encoding='utf-8'))
    if (os.name != 'nt' or config.get('CP_ENV') not in ('local', 'development')
            or db.hostname != '127.0.0.1' or db.port != 14660
            or db.path == '/' + TARGET or not re.fullmatch(r'cp_test_[a-f0-9]{32}', TARGET)
            or not interruption['restoration_required']
            or interruption['incomplete_batch_counted_as_pass']):
        raise SystemExit('Exact owned interrupted native fixture required.')
    env = {**os.environ, 'PGHOST': db.hostname, 'PGPORT': str(db.port),
           'PGUSER': 'postgres', 'PGPASSWORD': config['_superuser_password'],
           'PGCONNECT_TIMEOUT': '5', 'PGOPTIONS': '-c statement_timeout=55000'}

    def sql(database, query):
        result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q',
                                 '-v', 'ON_ERROR_STOP=1', '-c', query],
                                env={**env, 'PGDATABASE': database}, capture_output=True,
                                text=True, encoding='utf-8', timeout=60,
                                creationflags=subprocess.CREATE_NO_WINDOW)
        if result.returncode:
            raise SystemExit('Fixture operation failed; private diagnostics withheld. Inspect actual state before retrying.')
        return result.stdout.strip()

    raw = sql('postgres', "SELECT json_build_object('owner',pg_get_userbyid(datdba),"
              "'template',datistemplate,'connections',(SELECT count(*) FROM pg_stat_activity "
              "WHERE datid=d.oid)) FROM pg_database d WHERE datname='" + TARGET + "'")
    before = json.loads(raw) if raw else None
    applied = []
    if before is not None:
        if before != {'owner': 'postgres', 'template': False, 'connections': 0}:
            raise SystemExit('Refusing a connected, template or unexpected-owner database.')
        shape = json.loads(sql(TARGET, "SELECT json_build_object('organizations',(SELECT count(*) FROM organizations),"
                              "'users',(SELECT count(*) FROM users))"))
        applied = json.loads(sql(TARGET, "SELECT json_agg(json_build_object('name',name,'sha256',sha256) ORDER BY name) FROM schema_migrations"))
        expected = [{'name': name, 'sha256': hashlib.sha256((APP / 'backend/migrations' / name).read_bytes()).hexdigest()}
                    for name in ('0001_identity_platform.sql', '0002_projects_knowledge.sql')]
        if shape != {'organizations': 0, 'users': 0} or applied != expected:
            raise SystemExit('Empty partial fixture and exact first-two migration hashes required.')
        # The connection used for inspection is already closed. No FORCE.
        sql('postgres', 'DROP DATABASE "' + TARGET + '"')
    absent = sql('postgres', "SELECT count(*) FROM pg_database WHERE datname='" + TARGET + "'") == '0'
    report = {'passed': absent, 'checked_at': datetime.now(timezone.utc).isoformat(),
              'database': TARGET, 'before': before, 'migration_hashes': applied,
              'empty_fixture_verified': before is not None, 'already_absent': before is None,
              'forced_disconnect': False, 'application_database_selected': False,
              'scope': 'Exact disconnected empty partial fixture from interrupted increment67; current running tests unaffected.'}
    (EVIDENCE / 'interrupted-fixture-cleanup.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': absent, 'removed_owned_empty_fixture': before is not None, 'forced_disconnect': False}))
    return 0 if absent else 1


if __name__ == '__main__':
    raise SystemExit(main())
