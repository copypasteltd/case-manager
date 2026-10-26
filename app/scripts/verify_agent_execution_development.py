"""Read-only checks of the owned native secretary execution development services."""
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
EVIDENCE = APP.parent / 'docs/quality/evidence/secretary-execution'


def main():
    values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    connection = urlparse(values['CP_DATABASE_URL'])
    address = values.get('CP_HTTP_ADDR', '')
    if (values.get('CP_ENV') not in ('development', 'local') or connection.hostname != '127.0.0.1'
            or not address.startswith('127.0.0.1:')):
        raise SystemExit('Only owned native loopback development services are supported.')
    environment = {**os.environ, 'PGHOST': connection.hostname, 'PGPORT': str(connection.port or 5432),
                   'PGUSER': unquote(connection.username or ''), 'PGPASSWORD': unquote(connection.password or ''),
                   'PGDATABASE': unquote(connection.path.lstrip('/')), 'PGCONNECT_TIMEOUT': '5'}

    def sql(query):
        result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q',
                                 '-v', 'ON_ERROR_STOP=1', '-c', query], env=environment,
                                capture_output=True, text=True, encoding='utf-8', timeout=15,
                                creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
        if result.returncode:
            raise SystemExit('Development probe failed; private diagnostics withheld.')
        return json.loads(result.stdout)

    database = sql("""SELECT json_build_object('role',current_user,'superuser',r.rolsuper,
      'bypass_rls',r.rolbypassrls,'create_database',r.rolcreatedb,'create_roles',r.rolcreaterole,
      'create_public_objects',has_schema_privilege(current_user,'public','CREATE'),
      'read_private_migration_ledger',has_table_privilege(current_user,'schema_migrations','SELECT'),
      'write_runtime_profiles',has_table_privilege(current_user,'agent_runtime_profiles','INSERT')
        OR has_table_privilege(current_user,'agent_runtime_profiles','UPDATE')
        OR has_table_privilege(current_user,'agent_runtime_profiles','DELETE'),
      'insert_execution_evidence',(SELECT bool_and(has_table_privilege(current_user,t,'INSERT'))
        FROM unnest(ARRAY['agent_run_jobs','agent_execution_leases','agent_execution_verifications',
          'agent_tool_receipts','agent_run_results','agent_execution_observations'])t),
      'rewrite_execution_evidence',(SELECT bool_or(has_table_privilege(current_user,t,'UPDATE')
        OR has_table_privilege(current_user,t,'DELETE'))
        FROM unnest(ARRAY['agent_run_jobs','agent_execution_leases','agent_execution_verifications',
          'agent_tool_receipts','agent_run_results','agent_execution_observations'])t),
      'immutable_execution_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN
        ('agent_job_immutable','agent_execution_lease_immutable','agent_tool_receipt_immutable',
         'agent_run_result_immutable','agent_observation_immutable','agent_verification_immutable')),
      'deferred_execution_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal
        AND tgdeferrable AND tginitdeferred AND tgname IN
        ('agent_tool_evidence','agent_result_evidence','agent_observation_evidence',
         'agent_verification_evidence','agent_response_head','agent_response_message')),
      'active_runtime_profiles',(SELECT count(*) FROM agent_runtime_profiles WHERE state='active'))
      FROM pg_roles r WHERE r.rolname=current_user""")
    manifest = sql("SELECT json_agg(json_build_object('name',name,'sha256',sha256) ORDER BY name) FROM application_schema_manifest()")
    expected = [{'name': path.name, 'sha256': hashlib.sha256(path.read_bytes()).hexdigest()}
                for path in sorted((APP / 'backend/migrations').glob('*.sql'))]
    database.update(migration_count=len(manifest), latest_migration=manifest[-1]['name'],
                    migration_manifest_matches_source=manifest == expected)
    identifier = '00000000-0000-4000-8000-000000000000'
    routes = [('GET', '/health/live', 200), ('GET', '/health/ready', 200),
              ('GET', '/v1/conversations', 401),
              ('GET', '/v1/conversations/' + identifier + '/runtime-options', 401),
              ('GET', '/v1/conversations/' + identifier + '/messages', 401),
              ('POST', '/v1/conversations/' + identifier + '/runs', 401),
              ('GET', '/v1/runs/' + identifier, 401),
              ('GET', '/v1/runs/' + identifier + '/event-log', 401),
              ('GET', '/v1/runs/' + identifier + '/events', 401),
              ('POST', '/v1/runs/' + identifier + '/cancel', 401)]
    probes = []
    for method, route, expected_status in routes:
        # Authentication precedes body decoding. Empty POST bodies avoid unread-data
        # reset races in short-lived native Windows HTTP connections.
        request = Request('http://' + address + route, method=method,
                          data=b'' if method == 'POST' else None,
                          headers={'Content-Type': 'application/json'})
        try:
            with urlopen(request, timeout=5) as response:
                status = response.status
        except HTTPError as error:
            status = error.code
            error.close()
        probes.append({'method': method, 'path': route, 'status': status,
                       'expected': expected_status, 'passed': status == expected_status,
                       'request_body': 'empty' if method == 'POST' else None})
    forbidden = ('superuser', 'bypass_rls', 'create_database', 'create_roles', 'create_public_objects',
                 'read_private_migration_ledger', 'write_runtime_profiles', 'rewrite_execution_evidence')
    passed = (not any(database[key] for key in forbidden) and database['insert_execution_evidence']
              and database['migration_manifest_matches_source'] and database['immutable_execution_guards'] == 6
              and database['deferred_execution_guards'] == 6 and all(probe['passed'] for probe in probes))
    report = {'passed': bool(passed), 'checked_at': datetime.now(timezone.utc).isoformat(),
              'environment': 'owned native loopback development only', 'database': database,
              'probes': probes, 'production_deployment': False, 'worker_started': False,
              'actual_model_or_cloud': False,
              'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    (EVIDENCE / 'development-runtime.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': bool(passed), 'migration_count': len(manifest),
                      'probe_count': len(probes), 'active_runtime_profiles': database['active_runtime_profiles']}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
