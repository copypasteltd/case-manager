"""Read-only checks of the owned native project change schema and running API."""
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
 'can_insert_change_history',(SELECT bool_and(has_table_privilege(current_user,t,'INSERT')) FROM unnest(ARRAY['project_change_revisions','project_change_files','project_change_implementations'])t),
 'can_rewrite_change_history',(SELECT bool_or(has_table_privilege(current_user,t,'UPDATE') OR has_table_privilege(current_user,t,'DELETE')) FROM unnest(ARRAY['project_change_revisions','project_change_files','project_change_implementations','budget_lines'])t),
 'can_delete_change_or_budget',(SELECT bool_or(has_table_privilege(current_user,t,'DELETE')) FROM unnest(ARRAY['change_requests','project_budgets'])t),
 'special_role_count',(SELECT count(*) FROM roles WHERE key IN('project_change_reader','project_change_implementer')),
 'special_role_bindings',(SELECT count(*) FROM role_bindings b JOIN roles x ON x.org_id=b.org_id AND x.id=b.role_id WHERE x.key IN('project_change_reader','project_change_implementer')),
 'implementer_requires_mfa',(SELECT bool_and(requires_mfa) FROM roles WHERE key='project_change_implementer'),
 'reader_has_only_narrow_actions',(SELECT count(*)=2 AND bool_and(action IN('change.read','budget.read')) FROM role_actions a JOIN roles x ON x.org_id=a.org_id AND x.id=a.role_id WHERE x.key='project_change_reader'),
 'implementer_has_only_narrow_actions',(SELECT count(*)=5 AND bool_and(action IN('project.read','project.update','change.read','budget.read','change.implement')) FROM role_actions a JOIN roles x ON x.org_id=a.org_id AND x.id=a.role_id WHERE x.key='project_change_implementer'),
 'change_consistency_constraints',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN('project_change_head_consistency','project_change_revision_consistency','project_change_files_consistency','project_change_request_consistency') AND tgdeferrable AND tginitdeferred),
 'change_immutable_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN('project_change_revision_immutable','project_budget_lines_immutable','project_change_files_immutable','project_change_implementation_immutable')),
 'change_write_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN('project_change_head_guard','project_change_revision_guard','project_change_snapshot_guard','project_budget_guard','project_budget_line_guard','project_change_implementation_guard','project_controlled_scope')),
 'change_apply_and_reference_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN('project_change_budget_draft','project_change_apply','project_change_file_reference')))
 FROM pg_roles r WHERE r.rolname=current_user""")
manifest = sql("SELECT json_agg(json_build_object('name',name,'sha256',sha256) ORDER BY name) FROM application_schema_manifest()")
expected = [{'name': p.name, 'sha256': hashlib.sha256(p.read_bytes()).hexdigest()} for p in sorted((APP / 'backend/migrations').glob('*.sql'))]
database.update(migration_count=len(manifest), latest_migration=manifest[-1]['name'], migration_manifest_matches_source=manifest == expected)
identifier = '00000000-0000-4000-8000-000000000000'
project = '/v1/projects/' + identifier
change = '/v1/changes/' + identifier
routes = [('GET', '/health/live', 200), ('GET', '/health/ready', 200),
          ('GET', project + '/changes', 401), ('POST', project + '/changes', 401),
          ('GET', project + '/change-basis?currency=HKD', 401), ('GET', project + '/budgets', 401),
          ('GET', change, 401), ('PATCH', change, 401), ('GET', change + '/revisions', 401),
          ('GET', change + '/revisions/' + identifier, 401), ('POST', change + '/submit', 401),
          ('POST', change + '/implement', 401), ('GET', '/v1/requests', 401),
          ('GET', '/v1/costs', 401), ('GET', '/v1/payments', 401)]
probes = []
for method, route, expected_status in routes:
    request = Request('http://' + address + route, method=method, data=b'{}' if method in ('POST', 'PATCH') else None, headers={'Content-Type': 'application/json'})
    try:
        with urlopen(request, timeout=5) as response:
            status = response.status
    except HTTPError as error:
        status = error.code
    probes.append({'method': method, 'path': route, 'status': status, 'expected': expected_status, 'passed': status == expected_status})
forbidden = ('is_superuser', 'can_bypass_row_security', 'can_create_database', 'can_create_roles', 'can_read_migration_ledger', 'can_create_public_objects', 'can_rewrite_change_history', 'can_delete_change_or_budget')
required = ('can_insert_change_history', 'implementer_requires_mfa', 'reader_has_only_narrow_actions', 'implementer_has_only_narrow_actions', 'migration_manifest_matches_source')
passed = all(probe['passed'] for probe in probes) and not any(database[key] for key in forbidden) and all(database[key] for key in required)
passed = passed and database['migration_count'] == 45 and database['special_role_count'] == 2 and database['special_role_bindings'] == 0
passed = passed and database['change_consistency_constraints'] == 4 and database['change_immutable_guards'] == 4 and database['change_write_guards'] == 7 and database['change_apply_and_reference_guards'] == 3
target = APP.parent / 'docs/quality/evidence/project-changes/development-runtime.json'
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(json.dumps({'checked_at': datetime.now(timezone.utc).isoformat(), 'passed': passed,
    'environment': 'owned native loopback development only', 'database': database, 'probes': probes,
    'production_deployment': False, 'worker_started': False}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': passed, 'migration_count': len(manifest), 'probe_count': len(probes)}))
raise SystemExit(0 if passed else 1)
