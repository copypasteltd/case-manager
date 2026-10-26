"""Read-only checks of the owned native purchase variance schema and API."""
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
    'can_delete_variance_heads',has_table_privilege(current_user,'purchase_variances','DELETE'),
    'can_insert_immutable_facts',(SELECT bool_and(has_table_privilege(current_user,t,'INSERT')) FROM unnest(ARRAY['purchase_variance_revisions','purchase_variance_files','purchase_plan_activations']) t),
    'can_rewrite_immutable_facts',(SELECT bool_or(has_table_privilege(current_user,t,'UPDATE') OR has_table_privilege(current_user,t,'DELETE')) FROM unnest(ARRAY['purchase_variance_revisions','purchase_variance_files','purchase_plan_activations']) t),
    'variance_role_count',(SELECT count(*) FROM roles WHERE key IN ('purchase_variance_manager','purchase_variance_reader','purchase_plan_activator')),
    'variance_role_bindings',(SELECT count(*) FROM role_bindings b JOIN roles x ON x.org_id=b.org_id AND x.id=b.role_id WHERE x.key IN ('purchase_variance_manager','purchase_variance_reader','purchase_plan_activator')),
    'activator_requires_mfa',(SELECT bool_and(requires_mfa) FROM roles WHERE key='purchase_plan_activator'),
    'variance_consistency_constraints',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN ('purchase_variance_head_valid','purchase_variance_revision_valid','purchase_variance_files_valid','purchase_variance_request_valid','request_variance_valid') AND tgdeferrable AND tginitdeferred),
    'variance_immutable_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN ('immutable_purchase_variance_revisions','immutable_purchase_variance_files','immutable_purchase_plan_activations')))
    FROM pg_roles r WHERE r.rolname=current_user""")
manifest = sql("SELECT json_agg(json_build_object('name',name,'sha256',sha256) ORDER BY name) FROM application_schema_manifest()")
expected = [{'name': p.name, 'sha256': hashlib.sha256(p.read_bytes()).hexdigest()} for p in sorted((APP / 'backend/migrations').glob('*.sql'))]
database.update(migration_count=len(manifest), latest_migration=manifest[-1]['name'], migration_manifest_matches_source=manifest == expected)
identifier = '00000000-0000-4000-8000-000000000000'
purchase = '/v1/purchases/' + identifier
variance = '/v1/purchase-variances/' + identifier
routes = [('GET', '/health/live', 200), ('GET', '/health/ready', 200),
          ('GET', purchase + '/execution-plan', 401), ('GET', purchase + '/variances', 401), ('POST', purchase + '/variances', 401),
          ('GET', variance, 401), ('PATCH', variance, 401), ('GET', variance + '/revisions', 401),
          ('GET', variance + '/revisions/' + identifier, 401), ('POST', variance + '/activate', 401),
          ('GET', purchase, 401), ('GET', '/v1/requests', 401), ('GET', '/v1/costs', 401), ('GET', '/v1/workflow-definitions', 401)]
probes = []
for method, route, expected_status in routes:
    request = Request('http://' + address + route, method=method, data=b'{}' if method in ('POST', 'PATCH') else None, headers={'Content-Type': 'application/json'})
    try:
        with urlopen(request, timeout=5) as response:
            status = response.status
    except HTTPError as error:
        status = error.code
    probes.append({'method': method, 'path': route, 'status': status, 'expected': expected_status, 'passed': status == expected_status})
forbidden = ('is_superuser', 'can_bypass_row_security', 'can_create_database', 'can_create_roles', 'can_read_migration_ledger', 'can_create_public_objects', 'can_delete_variance_heads', 'can_rewrite_immutable_facts')
passed = all(probe['passed'] for probe in probes) and not any(database[key] for key in forbidden)
passed = passed and all(database[key] for key in ('can_insert_immutable_facts', 'activator_requires_mfa', 'migration_manifest_matches_source'))
passed = passed and database['variance_role_count'] == 3 and database['variance_role_bindings'] == 0 and database['variance_consistency_constraints'] == 5 and database['variance_immutable_guards'] == 3
report = {'checked_at': datetime.now(timezone.utc).isoformat(), 'passed': passed, 'environment': 'owned native loopback development only', 'database': database, 'probes': probes, 'production_deployment': False, 'worker_started': False}
target = APP.parent / 'docs/quality/evidence/purchase-variance/development-runtime.json'
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': passed, 'migration_count': len(manifest), 'probe_count': len(probes)}))
raise SystemExit(0 if passed else 1)

