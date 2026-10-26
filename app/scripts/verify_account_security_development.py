"""Read-only schema/privilege checks and unauthenticated owned API probes."""
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
EVIDENCE = APP.parent / 'docs/quality/evidence/account-security'


def main():
    values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    db = urlparse(values['CP_DATABASE_URL'])
    if (values.get('CP_ENV') not in ('development', 'local') or db.hostname != '127.0.0.1'
            or db.port != 14660 or values.get('CP_HTTP_ADDR') != '127.0.0.1:18080'):
        raise SystemExit('Only the owned native loopback development environment is supported.')
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    spec = importlib.util.spec_from_file_location('account_runtime', APP / 'scripts/verify_agent_execution_development.py')
    baseline = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(baseline)
    baseline.EVIDENCE = EVIDENCE
    if baseline.main():
        return 1
    common = json.loads((EVIDENCE / 'development-runtime.json').read_text(encoding='utf-8'))
    environment = {**os.environ, 'PGHOST': db.hostname, 'PGPORT': str(db.port), 'PGUSER': unquote(db.username or ''),
                   'PGPASSWORD': unquote(db.password or ''), 'PGDATABASE': unquote(db.path.lstrip('/')), 'PGCONNECT_TIMEOUT': '5'}
    query = """SELECT json_build_object(
      'reset_privileges',has_table_privilege(current_user,'password_resets','SELECT')
        AND has_table_privilege(current_user,'password_resets','INSERT')
        AND has_table_privilege(current_user,'password_resets','UPDATE')
        AND NOT has_table_privilege(current_user,'password_resets','DELETE'),
      'credential_indexes',(SELECT count(*) FROM pg_indexes WHERE schemaname='public'
        AND indexname IN ('password_resets_one_pending','sessions_family_history')),
      'scoped_reset_authority',EXISTS(SELECT 1 FROM roles r JOIN role_actions a ON a.org_id=r.org_id AND a.role_id=r.id
        WHERE r.key='system_admin' AND r.requires_mfa AND a.action='user.password_reset'),
      'notification_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O'
        AND tgname LIKE 'notification_%'),
      'successful_delivery_jobs',(SELECT count(*) FROM notification_job_results r JOIN jobs j
        ON j.org_id=r.org_id AND j.id=r.job_id WHERE j.state='succeeded' AND j.kind='notification.deliver.v1'),
      'active_runtime_profiles',(SELECT count(*) FROM agent_runtime_profiles WHERE state='active'),
      'disposable_test_databases',(SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[0-9a-f]{32}$'))"""
    result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
                            env=environment, capture_output=True, text=True, encoding='utf-8', timeout=15,
                            creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if result.returncode:
        raise SystemExit('Owned account-security database checks failed; private diagnostics withheld.')
    database = json.loads(result.stdout)
    identifier = '00000000-0000-4000-8000-000000000000'
    probes = []
    for method, route, expected in [('GET', '/v1/me/sessions', 401), ('POST', '/v1/me/sessions/' + identifier + '/revoke', 401),
            ('POST', '/v1/me/password', 401), ('POST', '/v1/users/' + identifier + '/password-resets', 401), ('POST', '/v1/auth/password/reset', 422)]:
        try:
            request = Request('http://127.0.0.1:18080' + route, method=method, data=b'{}' if method == 'POST' else None, headers={'Content-Type': 'application/json'})
            with urlopen(request, timeout=5) as response:
                status = response.status
        except HTTPError as error:
            status = error.code
            error.close()
        probes.append({'method': method, 'path': route, 'status': status, 'expected': expected, 'passed': status == expected})
    effective = {**os.environ, **values}
    skills = [json.loads(path.read_text(encoding='utf-8'))['state'] for path in sorted((APP / 'skills').glob('*/manifest.json'))]
    external_disabled = effective.get('CP_AGENT_FILE_PROVIDERS', '') in ('', 'disabled') and effective.get('CP_AGENT_DRAFT_DRIVER', '') in ('', 'disabled') and database['active_runtime_profiles'] == 0
    passed = bool(common['passed'] and common['database']['migration_count'] == 53 and database['reset_privileges']
        and database['credential_indexes'] == 2 and database['scoped_reset_authority'] and database['notification_guards'] == 10
        and database['successful_delivery_jobs'] > 0 and database['disposable_test_databases'] == 0
        and all(probe['passed'] for probe in probes) and external_disabled and skills == ['draft'] * 8)
    report = {'passed': passed, 'checked_at': datetime.now(timezone.utc).isoformat(), 'database': database,
              'migration_count': common['database']['migration_count'], 'probes': probes, 'actual_skill_states': skills,
              'real_model_and_cloud_disabled': external_disabled, 'production_deployed': False, 'external_messages_sent': False,
              'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (EVIDENCE / 'development-account-security.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': passed, 'migration_count': report['migration_count'], 'probe_count': len(probes)}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
