"""Remove only the two confirmed, disconnected 0042 disposable test databases.

This one-off recovery never terminates sessions or selects application databases.
The cancelled cleanup statements must exist in this task's native PostgreSQL log.
"""
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import subprocess
from urllib.parse import urlparse

APP = Path(__file__).resolve().parents[1]
TARGETS = ('cp_test_4b119cc30eb14919a8fec3bb7e794151', 'cp_test_3ca630e6f0164b548368685d068f13d8')
values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
source = urlparse(values['CP_DATABASE_URL'])
if values.get('CP_ENV') not in ('development', 'local') or source.hostname != '127.0.0.1':
    raise SystemExit('Only this owned native loopback development cluster is supported.')
log = (APP / '.local/postgres.log').read_text(encoding='utf-8', errors='replace').splitlines()
for name in TARGETS:
    if not re.fullmatch(r'cp_test_[a-f0-9]{32}', name) or source.path == '/' + name:
        raise SystemExit('Refusing an unsafe target.')
    found = any('canceling statement due to user request' in log[index - 1]
                and 'DROP DATABASE "' + name + '" WITH (FORCE)' in line
                for index, line in enumerate(log) if index > 0)
    if not found:
        raise SystemExit('A target lacks the cancelled fixture cleanup evidence.')
env = {**os.environ, 'PGHOST': '127.0.0.1', 'PGPORT': str(source.port or 5432),
       'PGUSER': 'postgres', 'PGPASSWORD': values['_superuser_password'],
       'PGDATABASE': 'postgres', 'PGCONNECT_TIMEOUT': '5', 'PGOPTIONS': '-c statement_timeout=55000'}


def sql(query):
    result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q',
                             '-v', 'ON_ERROR_STOP=1', '-c', query], env=env, capture_output=True,
                            text=True, encoding='utf-8', timeout=60,
                            creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if result.returncode:
        raise SystemExit('Native cleanup failed; private diagnostics withheld.')
    return result.stdout.strip()


checks = []
for name in TARGETS:
    raw = sql("SELECT json_build_object('owner',pg_get_userbyid(datdba),'template',datistemplate,"
              "'connections',(SELECT count(*) FROM pg_stat_activity WHERE datid=d.oid)) "
              "FROM pg_database d WHERE datname='" + name + "'")
    if not raw:
        checks.append({'database': name, 'already_absent': True, 'passed': True})
        continue
    before = json.loads(raw)
    if before != {'owner': 'postgres', 'template': False, 'connections': 0}:
        raise SystemExit('Refusing a connected, template, or unexpected-owner database.')
    # The identifier is an exact, regex-checked allowlist entry. No FORCE is used.
    sql('DROP DATABASE "' + name + '"')
    gone = sql("SELECT count(*) FROM pg_database WHERE datname='" + name + "'") == '0'
    checks.append({'database': name, 'before': before, 'cancelled_fixture_cleanup_found': True,
                   'forced_disconnect': False, 'removed': gone, 'passed': gone})
report = {'passed': all(item['passed'] for item in checks),
          'executed_at': datetime.now(timezone.utc).isoformat(),
          'scope': 'Exact two owned native 0042 disposable fixture databases; no application database',
          'application_database_selected': False, 'checks': checks}
target = APP.parent / 'docs/quality/evidence/purchase-completion/test-database-cleanup.json'
target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': report['passed'], 'target_count': len(checks)}))
raise SystemExit(0 if report['passed'] else 1)
