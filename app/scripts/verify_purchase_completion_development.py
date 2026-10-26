"""Read-only probes of the owned native purchase completion schema and API."""
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
        raise SystemExit('Development database probe failed; raw diagnostics and credentials withheld.')
    return json.loads(result.stdout)


database = sql("""SELECT json_build_object('role',current_user,'is_superuser',r.rolsuper,
    'can_bypass_row_security',r.rolbypassrls,'can_create_database',r.rolcreatedb,'can_create_roles',r.rolcreaterole,
    'can_read_migration_ledger',has_table_privilege(current_user,'schema_migrations','SELECT'),
    'can_create_public_objects',has_schema_privilege(current_user,'public','CREATE'),
    'can_insert_completion_facts',(SELECT bool_and(has_table_privilege(current_user,t,'INSERT')) FROM unnest(ARRAY['purchase_completions','purchase_completion_files'])t),
    'can_rewrite_completion_facts',(SELECT bool_or(has_table_privilege(current_user,t,'UPDATE') OR has_table_privilege(current_user,t,'DELETE')) FROM unnest(ARRAY['purchase_completions','purchase_completion_files'])t),
    'completer_role_count',(SELECT count(*) FROM roles WHERE key='purchase_completer'),
    'completer_role_bindings',(SELECT count(*) FROM role_bindings b JOIN roles x ON x.org_id=b.org_id AND x.id=b.role_id WHERE x.key='purchase_completer'),
    'completer_requires_mfa',(SELECT bool_and(requires_mfa) FROM roles WHERE key='purchase_completer'),
    'completion_consistency_constraints',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN ('purchase_completion_head','purchase_completion_event','purchase_completion_valid','purchase_completion_files_valid') AND tgdeferrable AND tginitdeferred),
    'completion_immutable_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN ('immutable_purchase_completions','immutable_purchase_completion_files')),
    'completed_purchase_variance_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN ('completed_purchase_variance','completed_purchase_activation')),
    'completion_reference_guard',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname='purchase_completion_file_reference'))
    FROM pg_roles r WHERE r.rolname=current_user""")
manifest = sql("SELECT json_agg(json_build_object('name',name,'sha256',sha256) ORDER BY name) FROM application_schema_manifest()")
expected = [{'name': p.name, 'sha256': hashlib.sha256(p.read_bytes()).hexdigest()} for p in sorted((APP / 'backend/migrations').glob('*.sql'))]
database.update(migration_count=len(manifest), latest_migration=manifest[-1]['name'], migration_manifest_matches_source=manifest == expected)
identifier = '00000000-0000-4000-8000-000000000000'
purchase = '/v1/purchases/' + identifier
routes = [('GET', '/health/live', 200), ('GET', '/health/ready', 200),
          ('GET', purchase + '/completion-preview', 401), ('GET', purchase + '/completion', 401), ('POST', purchase + '/completion', 401),
          ('GET', purchase, 401), ('GET', purchase + '/fulfillment', 401), ('GET', purchase + '/execution-plan', 401), ('GET', purchase + '/variances', 401),
          ('POST', purchase + '/orders', 401), ('POST', purchase + '/receipts', 401),
          ('GET', '/v1/requests', 401), ('GET', '/v1/costs', 401), ('GET', '/v1/payments', 401), ('GET', '/v1/workflow-definitions', 401)]
probes = []
for method, route, expected_status in routes:
    request = Request('http://' + address + route, method=method, data=b'{}' if method == 'POST' else None, headers={'Content-Type': 'application/json'})
    try:
        with urlopen(request, timeout=5) as response:
            status = response.status
    except HTTPError as error:
        status = error.code
    probes.append({'method': method, 'path': route, 'status': status, 'expected': expected_status, 'passed': status == expected_status})
forbidden = ('is_superuser', 'can_bypass_row_security', 'can_create_database', 'can_create_roles', 'can_read_migration_ledger', 'can_create_public_objects', 'can_rewrite_completion_facts')
passed = all(probe['passed'] for probe in probes) and not any(database[key] for key in forbidden)
passed = passed and all(database[key] for key in ('can_insert_completion_facts', 'completer_requires_mfa', 'migration_manifest_matches_source'))
passed = passed and database['migration_count'] == 43 and database['completer_role_count'] == 1 and database['completer_role_bindings'] == 0
passed = passed and database['completion_consistency_constraints'] == 4 and database['completion_immutable_guards'] == 2 and database['completed_purchase_variance_guards'] == 2 and database['completion_reference_guard'] == 1
target = APP.parent / 'docs/quality/evidence/purchase-completion/development-runtime.json'
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(json.dumps({'checked_at': datetime.now(timezone.utc).isoformat(), 'passed': passed,
    'environment': 'owned native loopback development only', 'database': database, 'probes': probes,
    'production_deployment': False, 'worker_started': False}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': passed, 'migration_count': len(manifest), 'probe_count': len(probes)}))
raise SystemExit(0 if passed else 1)
