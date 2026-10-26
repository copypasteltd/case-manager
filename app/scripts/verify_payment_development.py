"""Read-only checks of the owned development payment schema and API."""
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
    'can_bypass_row_security',r.rolbypassrls,'can_create_database',r.rolcreatedb,'can_create_roles',r.rolcreaterole,
    'can_read_migration_ledger',has_table_privilege(current_user,'schema_migrations','SELECT'),
    'can_create_public_objects',has_schema_privilege(current_user,'public','CREATE'),
    'can_insert_payment_heads',has_table_privilege(current_user,'payment_records','INSERT'),
    'can_update_payment_heads',has_table_privilege(current_user,'payment_records','UPDATE'),
    'can_delete_payment_heads',has_table_privilege(current_user,'payment_records','DELETE'),
    'can_insert_immutable_facts',(SELECT bool_and(has_table_privilege(current_user,t,'INSERT')) FROM unnest(ARRAY['payment_revisions','payment_revision_files','payment_allocations','payment_allocation_shares','payment_events']) t),
    'can_rewrite_immutable_facts',(SELECT bool_or(has_table_privilege(current_user,t,'UPDATE') OR has_table_privilege(current_user,t,'DELETE')) FROM unnest(ARRAY['payment_revisions','payment_revision_files','payment_allocations','payment_allocation_shares','payment_events']) t),
    'financial_role_count',(SELECT count(*) FROM roles WHERE key IN ('payment_recorder','payment_reverser','payment_reader','cash_reporter')),
    'financial_role_bindings',(SELECT count(*) FROM role_bindings b JOIN roles x ON x.org_id=b.org_id AND x.id=b.role_id WHERE x.key IN ('payment_recorder','payment_reverser','payment_reader','cash_reporter')))
    FROM pg_roles r WHERE r.rolname=current_user""")
manifest = sql("SELECT json_agg(json_build_object('name',name,'sha256',sha256) ORDER BY name) FROM application_schema_manifest()")
expected = [{'name': path.name, 'sha256': hashlib.sha256(path.read_bytes()).hexdigest()} for path in sorted((APP / 'backend/migrations').glob('*.sql'))]
database.update(migration_count=len(manifest), latest_migration=manifest[-1]['name'], migration_manifest_matches_source=manifest == expected)

identifier = '00000000-0000-4000-8000-000000000000'
payment = '/v1/payments/' + identifier
routes = [('GET', '/health/live', 200), ('GET', '/health/ready', 200),
          ('GET', '/v1/payments', 401), ('POST', '/v1/payments', 401),
          ('GET', payment, 401), ('PATCH', payment, 401)]
routes += [('POST', payment + '/' + part, 401) for part in ('record', 'reverse', 'allocations', 'refund-allocations', 'allocations/' + identifier + '/release')]
routes += [('GET', payment + '/' + part, 401) for part in ('allocations', 'revisions', 'revisions/' + identifier, 'events')]
routes += [('GET', '/v1/claims/' + identifier + '/settlement', 401), ('GET', '/v1/reports/cash', 401)]
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

required = ('can_insert_payment_heads', 'can_update_payment_heads', 'can_insert_immutable_facts', 'migration_manifest_matches_source')
forbidden = ('is_superuser', 'can_bypass_row_security', 'can_create_database', 'can_create_roles', 'can_read_migration_ledger', 'can_create_public_objects',
             'can_delete_payment_heads', 'can_rewrite_immutable_facts')
passed = all(database[key] for key in required) and not any(database[key] for key in forbidden)
passed = passed and database['financial_role_count'] == 4 and database['financial_role_bindings'] == 0 and all(p['status'] == p['expected_status'] for p in probes)
report = {'passed': passed, 'executed_at': datetime.now(timezone.utc).isoformat(),
          'scope': 'Owned local development database and API; not a production deployment',
          'native_database': True, 'database': database, 'probes': probes}
target = APP.parent / 'docs/quality/evidence/payments-settlement/development-runtime.json'
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': passed, 'migration_count': database['migration_count'], 'api_probes': len(probes), 'financial_role_bindings': database['financial_role_bindings']}))
raise SystemExit(0 if passed else 1)
