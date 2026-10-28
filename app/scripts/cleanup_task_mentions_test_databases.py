"""Dispose exactly the two idle synthetic databases left by interrupted95 checks.

One-shot recovery, not a general database cleanup command. Never uses FORCE.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/task-mentions'
report = E / 'disposable-cleanup.json'
assert not report.exists(), 'This one-shot recovery has already been recorded.'
values = json.loads((APP / '.local/environment.json').read_text())
assert values.get('CP_ENV') in ('local', 'development') and values.get('_postgres_port') == 14660
previous = json.loads((E.parent / 'mobile-files/development-project-resources.json').read_text())
assert previous['passed'] and previous['database']['disposable_test_databases'] == 0
environment = {**os.environ, 'PGHOST': '127.0.0.1', 'PGPORT': '14660', 'PGUSER': 'postgres',
    'PGPASSWORD': values['_superuser_password'], 'PGDATABASE': 'postgres', 'PGCONNECT_TIMEOUT': '5'}


def sql(query):
    result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
        env=environment, capture_output=True, text=True, encoding='utf-8', timeout=20, creationflags=subprocess.CREATE_NO_WINDOW)
    if result.returncode:
        raise SystemExit('Owned disposable-database operation failed; no private connection details are printed.')
    return result.stdout.strip()


query = """SELECT COALESCE(json_agg(row_to_json(x)),'[]'::json) FROM (
 SELECT d.datname,(SELECT count(*) FROM pg_stat_activity a WHERE a.datname=d.datname) connections,
 pg_get_userbyid(d.datdba) owner FROM pg_database d WHERE d.datname ~ '^cp_test_[0-9a-f]{32}$' ORDER BY d.datname) x"""
databases = json.loads(sql(query))
assert len(databases) == 2 and all(re.fullmatch(r'cp_test_[0-9a-f]{32}', d['datname']) and d['connections'] == 0 and d['owner'] == 'postgres' for d in databases), 'Recovery scope differs; nothing was dropped.'
for item in databases:
    # Validated generated identifiers only; no paths, wildcards or FORCE.
    assert sql("SELECT count(*) FROM pg_stat_activity WHERE datname='" + item['datname'] + "'") == '0'
    sql('DROP DATABASE "' + item['datname'] + '"')
remaining = json.loads(sql(query))
result = {'passed': not remaining, 'checked_at': datetime.now(timezone.utc).isoformat(), 'previous_published94_disposable_count': 0,
    'inspected_idle_synthetic_databases': len(databases), 'dropped': len(databases), 'remaining': len(remaining),
    'database_identity_sha256': [hashlib.sha256(d['datname'].encode()).hexdigest() for d in databases],
    'reason': 'Two databases remained after explicitly interrupted95 regression preparation. All current owned checks completed before cleanup.',
    'native_loopback_port': 14660, 'force_used': False, 'development_database_touched': False,
    'cleanup_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
report.write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': result['passed'], 'dropped_idle_synthetic_databases': len(databases), 'remaining': len(remaining)}))
