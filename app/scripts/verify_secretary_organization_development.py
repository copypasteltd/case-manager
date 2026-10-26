"""Read-only native schema, privilege and authenticated-route boundary checks."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
from urllib.error import HTTPError
from urllib.parse import unquote, urlparse
from urllib.request import Request, urlopen

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/secretary-organization'


def main():
    values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    db = urlparse(values['CP_DATABASE_URL'])
    if (values.get('CP_ENV') not in ('development', 'local') or db.hostname != '127.0.0.1'
            or db.port != 14660 or values.get('CP_HTTP_ADDR') != '127.0.0.1:18080'):
        raise SystemExit('Only the owned native loopback development environment is supported.')
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    spec = importlib.util.spec_from_file_location('organization_runtime', APP / 'scripts/verify_agent_execution_development.py')
    baseline = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(baseline)
    baseline.EVIDENCE = EVIDENCE
    if baseline.main():
        return 1
    common = json.loads((EVIDENCE / 'development-runtime.json').read_text(encoding='utf-8'))
    environment = {**os.environ, 'PGHOST': db.hostname, 'PGPORT': str(db.port), 'PGUSER': unquote(db.username or ''),
                   'PGPASSWORD': unquote(db.password or ''), 'PGDATABASE': unquote(db.path.lstrip('/')), 'PGCONNECT_TIMEOUT': '5'}
    query = """SELECT json_build_object(
      'plan_privileges',has_table_privilege(current_user,'agent_organization_plans','SELECT')
        AND has_table_privilege(current_user,'agent_organization_plans','INSERT')
        AND has_table_privilege(current_user,'agent_organization_plans','UPDATE')
        AND NOT has_table_privilege(current_user,'agent_organization_plans','DELETE'),
      'immutable_evidence_privileges',(SELECT bool_and(has_table_privilege(current_user,t,'SELECT')
        AND has_table_privilege(current_user,t,'INSERT') AND NOT has_table_privilege(current_user,t,'UPDATE')
        AND NOT has_table_privilege(current_user,t,'DELETE')) FROM unnest(ARRAY['agent_organization_events','agent_directory_snapshots'])t),
      'organization_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O'
        AND tgname IN ('agent_organization_event_immutable','agent_organization_no_delete','agent_organization_binding',
         'agent_organization_event_binding','agent_organization_history','agent_directory_immutable','agent_directory_binding')),
      'plan_index',EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='public' AND indexname='agent_organization_plans_conversation'),
      'active_runtime_profiles',(SELECT count(*) FROM agent_runtime_profiles WHERE state='active'),
      'disposable_test_databases',(SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[0-9a-f]{32}$'))"""
    result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
                            env=environment, capture_output=True, text=True, encoding='utf-8', timeout=15,
                            creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if result.returncode:
        raise SystemExit('Owned organization schema checks failed; private diagnostics withheld.')
    database = json.loads(result.stdout)
    identifier = '00000000-0000-4000-8000-000000000000'
    probes = []
    routes = [('GET', '/v1/conversations/' + identifier + '/directories/' + identifier),
              ('GET', '/v1/conversations/' + identifier + '/organization-plans'),
              ('POST', '/v1/conversations/' + identifier + '/organization-plans'),
              ('GET', '/v1/organization-plans/' + identifier)]
    routes += [('POST', '/v1/organization-plans/' + identifier + '/' + action) for action in ('approve', 'revoke', 'commit')]
    for method, route in routes:
        try:
            request = Request('http://127.0.0.1:18080' + route, method=method, data=b'{}' if method == 'POST' else None, headers={'Content-Type': 'application/json'})
            with urlopen(request, timeout=5) as response:
                status = response.status
        except HTTPError as error:
            status = error.code
            error.close()
        probes.append({'method': method, 'path': route, 'status': status, 'expected': 401, 'passed': status == 401})
    effective = {**os.environ, **values}
    skills = [json.loads(path.read_text(encoding='utf-8'))['state'] for path in sorted((APP / 'skills').glob('*/manifest.json'))]
    external_disabled = (effective.get('CP_AGENT_FILE_PROVIDERS', '') in ('', 'disabled')
                         and effective.get('CP_AGENT_DRAFT_DRIVER', '') in ('', 'disabled') and database['active_runtime_profiles'] == 0)
    passed = bool(common['passed'] and common['database']['migration_count'] == 54 and database['plan_privileges']
        and database['immutable_evidence_privileges'] and database['organization_guards'] == 7 and database['plan_index']
        and database['disposable_test_databases'] == 0 and all(probe['passed'] for probe in probes)
        and external_disabled and skills == ['draft'] * 8)
    report = {'passed': passed, 'checked_at': datetime.now(timezone.utc).isoformat(), 'database': database,
              'migration_count': common['database']['migration_count'], 'probes': probes, 'actual_skill_states': skills,
              'real_model_and_cloud_disabled': external_disabled, 'production_deployed': False, 'external_messages_sent': False,
              'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (EVIDENCE / 'development-organization.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': passed, 'migration_count': report['migration_count'], 'probe_count': len(probes)}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
