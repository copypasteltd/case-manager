"""Read-only checks of the owned development cost schema/API; never prints credentials."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
from urllib.error import HTTPError
from urllib.parse import unquote, urlparse
from urllib.request import Request, urlopen

APP = Path(__file__).resolve().parents[1]
values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
connection = urlparse(values['CP_DATABASE_URL'])
address = values.get('CP_HTTP_ADDR', '127.0.0.1:8080')
if values.get('CP_ENV') not in ('development', 'local') or connection.hostname != '127.0.0.1' or not address.startswith('127.0.0.1:'):
    raise SystemExit('Only the owned loopback development environment is supported.')
env = {**os.environ, 'PGHOST': connection.hostname, 'PGPORT': str(connection.port or 5432),
       'PGUSER': unquote(connection.username or ''), 'PGPASSWORD': unquote(connection.password or ''),
       'PGDATABASE': unquote(connection.path.lstrip('/')), 'PGCONNECT_TIMEOUT': '5'}


def sql(statement):
    result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', statement],
                            env=env, capture_output=True, text=True, encoding='utf-8', creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if result.returncode:
        raise SystemExit('Development database probe failed; credentials and raw diagnostics withheld.')
    return json.loads(result.stdout)


database = sql("""SELECT json_build_object('role',current_user,'is_superuser',r.rolsuper,
    'can_create_database',r.rolcreatedb,'can_create_roles',r.rolcreaterole,
    'can_read_migration_ledger',has_table_privilege(current_user,'schema_migrations','SELECT'),
    'can_create_public_objects',has_schema_privilege(current_user,'public','CREATE'),
    'can_insert_cost_heads',has_table_privilege(current_user,'actual_cost_entries','INSERT'),
    'can_update_cost_heads',has_table_privilege(current_user,'actual_cost_entries','UPDATE'),
    'can_delete_cost_heads',has_table_privilege(current_user,'actual_cost_entries','DELETE'),
    'can_insert_cost_events',has_table_privilege(current_user,'cost_events','INSERT'),
    'can_update_cost_revisions',has_table_privilege(current_user,'cost_revisions','UPDATE'),
    'can_delete_cost_allocations',has_table_privilege(current_user,'cost_allocations','DELETE'),
    'financial_role_count',(SELECT count(*) FROM roles WHERE key IN ('cost_recorder','cost_confirmer','cost_reverser','cost_reader','finance_reporter')),
    'financial_role_bindings',(SELECT count(*) FROM role_bindings b JOIN roles x ON x.org_id=b.org_id AND x.id=b.role_id WHERE x.key IN ('cost_recorder','cost_confirmer','cost_reverser','cost_reader','finance_reporter')))
    FROM pg_roles r WHERE r.rolname=current_user""")
manifest = sql("SELECT json_agg(json_build_object('name',name,'sha256',sha256) ORDER BY name) FROM application_schema_manifest()")
expected = [{'name': path.name, 'sha256': hashlib.sha256(path.read_bytes()).hexdigest()} for path in sorted((APP / 'backend/migrations').glob('*.sql'))]
database.update(migration_count=len(manifest), latest_migration=manifest[-1]['name'], migration_manifest_matches_source=manifest == expected)
identifier = '00000000-0000-4000-8000-000000000000'
routes = [('GET', '/health/live', 200), ('GET', '/health/ready', 200), ('GET', '/v1/costs', 401),
          ('POST', '/v1/costs', 401), ('GET', '/v1/costs/' + identifier, 401), ('PATCH', '/v1/costs/' + identifier, 401),
          ('GET', '/v1/reports/expenses?kind=incurred', 401)]
routes += [('POST', '/v1/costs/' + identifier + '/' + part, 401) for part in ('confirm', 'reverse', 'links')]
routes += [('GET', '/v1/costs/' + identifier + '/' + part, 401) for part in ('revisions', 'revisions/' + identifier, 'events')]
probes = []
for method, route, expected_status in routes:
    request = Request('http://' + address + route, method=method,
                      data=b'{}' if method in ('POST', 'PATCH') else None,
                      headers={'Content-Type': 'application/json'})
    try:
        with urlopen(request, timeout=5) as response:
            status = response.status
    except HTTPError as error:
        status = error.code
        error.close()
    probes.append({'method': method, 'path': route, 'status': status, 'expected_status': expected_status})
required = ('can_insert_cost_heads', 'can_update_cost_heads', 'can_insert_cost_events', 'migration_manifest_matches_source')
forbidden = ('is_superuser', 'can_create_database', 'can_create_roles', 'can_read_migration_ledger', 'can_create_public_objects',
             'can_delete_cost_heads', 'can_update_cost_revisions', 'can_delete_cost_allocations')
passed = all(database[key] for key in required) and not any(database[key] for key in forbidden)
passed = passed and database['financial_role_count'] == 5 and database['financial_role_bindings'] == 0 and all(p['status'] == p['expected_status'] for p in probes)
report = {'passed': passed, 'executed_at': datetime.now(timezone.utc).isoformat(),
          'scope': 'Owned local development database and API; not a production deployment',
          'native_database': True, 'database': database, 'probes': probes}
target = APP.parent / 'docs/quality/evidence/actual-costs/development-runtime.json'
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': passed, 'migration_count': database['migration_count'], 'api_probes': len(probes), 'financial_role_bindings': database['financial_role_bindings']}))
raise SystemExit(0 if passed else 1)
