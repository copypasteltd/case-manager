"""Read-only evidence for the owned native notification API, worker and schema."""
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
EVIDENCE = APP.parent / 'docs/quality/evidence/notifications'


def main():
    values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    db = urlparse(values['CP_DATABASE_URL'])
    if (values.get('CP_ENV') not in ('development', 'local') or db.hostname != '127.0.0.1'
            or db.port != 14660 or values.get('CP_HTTP_ADDR') != '127.0.0.1:18080'):
        raise SystemExit('Only this task\'s owned native development environment is supported.')
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    spec = importlib.util.spec_from_file_location('notification_runtime_checks', APP / 'scripts/verify_agent_execution_development.py')
    baseline = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(baseline)
    baseline.EVIDENCE = EVIDENCE
    if baseline.main():
        return 1
    common = json.loads((EVIDENCE / 'development-runtime.json').read_text(encoding='utf-8'))
    environment = {**os.environ, 'PGHOST': db.hostname, 'PGPORT': str(db.port), 'PGUSER': unquote(db.username or ''),
                   'PGPASSWORD': unquote(db.password or ''), 'PGDATABASE': unquote(db.path.lstrip('/')), 'PGCONNECT_TIMEOUT': '5'}
    query = """SELECT json_build_object(
      'append_only_delivery_tables',(SELECT bool_and(has_table_privilege(current_user,t,'SELECT') AND has_table_privilege(current_user,t,'INSERT') AND NOT has_table_privilege(current_user,t,'UPDATE') AND NOT has_table_privilege(current_user,t,'DELETE')) FROM unnest(ARRAY['notification_deliveries','notification_job_results','notification_due_events'])t),
      'head_privileges',has_table_privilege(current_user,'notification_threads','SELECT') AND has_table_privilege(current_user,'notification_threads','INSERT') AND has_table_privilege(current_user,'notification_threads','UPDATE') AND NOT has_table_privilege(current_user,'notification_threads','DELETE'),
      'notification_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O' AND tgname LIKE 'notification_%'),
      'deferred_notification_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O' AND tgdeferrable AND tginitdeferred AND tgname LIKE 'notification_%'),
      'operator_role_bindings',(SELECT count(*) FROM role_bindings b JOIN roles r ON r.org_id=b.org_id AND r.id=b.role_id WHERE r.key='notification_operator' AND b.revoked_at IS NULL),
      'actual_in_app_deliveries',(SELECT count(*) FROM notification_deliveries),
      'actual_successful_delivery_jobs',(SELECT count(*) FROM notification_job_results r JOIN jobs j ON j.org_id=r.org_id AND j.id=r.job_id AND j.state='succeeded' AND j.kind='notification.deliver.v1'),
      'invalid_job_results',(SELECT count(*) FROM notification_job_results r LEFT JOIN jobs j ON j.org_id=r.org_id AND j.id=r.job_id WHERE j.id IS NULL OR j.state<>'succeeded' OR j.kind<>'notification.deliver.v1' OR r.delivered<>(SELECT count(*) FROM notification_deliveries d WHERE d.org_id=r.org_id AND d.job_id=r.job_id)),
      'active_runtime_profiles',(SELECT count(*) FROM agent_runtime_profiles WHERE state='active'),
      'disposable_test_databases',(SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[0-9a-f]{32}$'))"""
    result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
                            env=environment, capture_output=True, text=True, encoding='utf-8', timeout=15,
                            creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if result.returncode:
        raise SystemExit('Owned notification database checks failed; private diagnostics withheld.')
    database = json.loads(result.stdout)
    identifier = '00000000-0000-4000-8000-000000000000'
    probes = []
    for method, route in [('GET', '/v1/notifications'), ('GET', '/v1/notifications/summary'), ('GET', '/v1/notifications/' + identifier), ('POST', '/v1/notifications/' + identifier + '/read'), ('GET', '/v1/notification-delivery-failures')]:
        try:
            request = Request('http://127.0.0.1:18080' + route, method=method, data=b'{"delivery_seq":1}' if method == 'POST' else None, headers={'Content-Type': 'application/json'})
            with urlopen(request, timeout=5) as response:
                status = response.status
        except HTTPError as error:
            status = error.code
            error.close()
        probes.append({'method': method, 'path': route, 'status': status, 'expected': 401, 'passed': status == 401})
    effective = {**os.environ, **values}
    contract_disabled = not any(effective.get(key) for key in ('CP_NOTIFICATION_CONTRACT_ORG_ID', 'CP_NOTIFICATION_CONTRACT_TIMEZONE', 'CP_NOTIFICATION_CONTRACT_DAYS_BEFORE'))
    skills = [json.loads(path.read_text(encoding='utf-8'))['state'] for path in sorted((APP / 'skills').glob('*/manifest.json'))]
    no_model_or_cloud = effective.get('CP_AGENT_FILE_PROVIDERS', '') in ('', 'disabled') and effective.get('CP_AGENT_DRAFT_DRIVER', '') in ('', 'disabled') and database['active_runtime_profiles'] == 0
    passed = bool(common['passed'] and common['database']['migration_count'] == 52 and database['append_only_delivery_tables'] and database['head_privileges'] and database['notification_guards'] == 10 and database['deferred_notification_guards'] == 3 and database['operator_role_bindings'] == 0 and database['invalid_job_results'] == 0 and database['actual_in_app_deliveries'] > 0 and database['actual_successful_delivery_jobs'] > 0 and database['disposable_test_databases'] == 0 and all(probe['passed'] for probe in probes) and contract_disabled and no_model_or_cloud and skills == ['draft'] * 8)
    report = {'passed': passed, 'checked_at': datetime.now(timezone.utc).isoformat(), 'database': database, 'migration_count': common['database']['migration_count'], 'probes': probes,
              'contract_reminders_explicitly_unconfigured': contract_disabled, 'actual_skill_states': skills, 'real_model_and_cloud_disabled': no_model_or_cloud,
              'environment': 'owned native Windows Go/PostgreSQL with actual in-app worker receipts; no virtualization', 'production_deployed': False, 'actual_model_or_cloud_invoked': False,
              'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (EVIDENCE / 'development-notifications.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': passed, 'migration_count': report['migration_count'], 'notification_guards': database['notification_guards'], 'actual_in_app_deliveries': database['actual_in_app_deliveries'], 'probe_count': len(probes)}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
