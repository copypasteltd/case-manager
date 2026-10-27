"""Read-only inspection of the exact owned native financial report development API and additive migration 73."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlsplit

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/finance-reports'


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / filename)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


def main():
    values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    db = urlsplit(values['CP_DATABASE_URL'])
    if db.hostname != '127.0.0.1' or db.port != 14660 or values.get('CP_HTTP_ADDR') != '127.0.0.1:18080':
        raise SystemExit('Only the exact owned native loopback services may be inspected.')
    base = load('review_runtime_probe', 'verify_agent_execution_development.py')
    base.EVIDENCE = EVIDENCE
    if base.main():
        return 1
    common = json.loads((EVIDENCE / 'development-runtime.json').read_text(encoding='utf-8'))
    environment = {**os.environ, 'PGHOST': db.hostname, 'PGPORT': str(db.port), 'PGUSER': unquote(db.username),
                   'PGPASSWORD': unquote(db.password), 'PGDATABASE': unquote(db.path[1:]), 'PGCONNECT_TIMEOUT': '5'}
    query = """SELECT json_build_object(
      'report_tables',(SELECT count(*) FROM pg_class WHERE relkind='r' AND relname IN ('finance_report_exports','finance_report_snapshots')),
      'report_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O' AND tgname IN ('guard_finance_report_export','immutable_finance_report_export','guard_finance_report_snapshot','immutable_finance_report_snapshot')),
      'report_update_denied',NOT has_table_privilege(current_user,'finance_report_exports','UPDATE') AND NOT has_table_privilege(current_user,'finance_report_exports','DELETE') AND NOT has_table_privilege(current_user,'finance_report_snapshots','UPDATE'),
      'budget_tables',(SELECT count(*) FROM pg_class WHERE relkind='r' AND relname IN ('agent_budget_policies','agent_budget_reservations','agent_budget_usage','agent_budget_reconciliations')),
      'budget_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O' AND tgname='budget_binding'),
      'budget_immutable',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O' AND tgname='budget_immutable'),
      'budget_update_denied',NOT has_table_privilege(current_user,'agent_budget_reservations','UPDATE') AND NOT has_table_privilege(current_user,'agent_budget_usage','UPDATE'),
      'budget_policies',(SELECT count(*) FROM agent_budget_policies),
      'budget_reservations',(SELECT count(*) FROM agent_budget_reservations),
      'mutation_append_only',has_table_privilege(current_user,'agent_run_mutations','SELECT')
        AND has_table_privilege(current_user,'agent_run_mutations','INSERT')
        AND NOT has_table_privilege(current_user,'agent_run_mutations','UPDATE')
        AND NOT has_table_privilege(current_user,'agent_run_mutations','DELETE')
        AND has_table_privilege(current_user,'agent_mutation_commits','SELECT')
        AND has_table_privilege(current_user,'agent_mutation_commits','INSERT')
        AND NOT has_table_privilege(current_user,'agent_mutation_commits','UPDATE')
        AND NOT has_table_privilege(current_user,'agent_mutation_commits','DELETE'),
      'deliverable_tool_constraints',(SELECT count(*) FROM pg_constraint WHERE convalidated AND conname IN
        ('agent_run_mutations_tool_check','agent_tool_receipts_tool_check','agent_execution_observations_tool_check')
        AND pg_get_constraintdef(oid) LIKE '%create_deliverable%'),
      'deliverable_scope_constraint',(SELECT count(*) FROM pg_constraint WHERE convalidated AND conname='agent_run_mutations_scope_check'
        AND pg_get_constraintdef(oid) LIKE '%create_deliverable%'),
      'deliverable_receipt_function',pg_get_functiondef('validate_agent_mutation_binding()'::regprocedure) LIKE '%create_deliverable%'
        AND pg_get_functiondef('validate_agent_mutation_session()'::regprocedure) LIKE '%create_deliverable%',
      'milestone_unicode_whitespace',pg_get_functiondef('validate_agent_mutation_session()'::regprocedure)
        LIKE '%btrim(m.arguments->>''title'',trim_chars)%'
        AND pg_get_functiondef('validate_agent_mutation_session()'::regprocedure)
        LIKE '%btrim(expected#>>''{}'',trim_chars)%',
      'deferred_evidence',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O' AND tgdeferrable AND tginitdeferred
        AND tgname IN ('agent_mutation_sender_evidence','agent_mutation_tool_evidence')),
      'concern_tool_constraints',(SELECT count(*) FROM pg_constraint WHERE convalidated AND conname IN
        ('agent_run_mutations_tool_check','agent_tool_receipts_tool_check','agent_execution_observations_tool_check')
        AND pg_get_constraintdef(oid) LIKE '%create_risk%' AND pg_get_constraintdef(oid) LIKE '%update_risk%'
        AND pg_get_constraintdef(oid) LIKE '%create_issue%' AND pg_get_constraintdef(oid) LIKE '%update_issue%'),
      'concern_session_function',pg_get_functiondef('validate_agent_mutation_session()'::regprocedure) LIKE '%create_risk%'
        AND pg_get_functiondef('validate_agent_mutation_session()'::regprocedure) LIKE '%child%',
      'task_creation_tool_constraints',(SELECT count(*) FROM pg_constraint WHERE convalidated AND conname IN ('agent_run_mutations_tool_check','agent_tool_receipts_tool_check','agent_execution_observations_tool_check') AND pg_get_constraintdef(oid) LIKE '%create_task%'),
      'task_creation_session_function',pg_get_functiondef('validate_agent_mutation_session()'::regprocedure) LIKE '%task creation requires complete exact fields%',
      'active_profiles',(SELECT count(*) FROM agent_runtime_profiles WHERE state='active'),
      'enabled_telegram_channels',(SELECT count(*) FROM telegram_channels WHERE delivery_enabled),
      'disposable_test_databases',(SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[0-9a-f]{32}$'))"""
    result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
                            env=environment, capture_output=True, text=True, encoding='utf-8', timeout=15,
                            creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if result.returncode:
        raise SystemExit('Owned schema inspection failed; private diagnostics withheld.')
    actual = json.loads(result.stdout)
    manifests = [json.loads(path.read_text(encoding='utf-8')) for path in sorted((APP / 'skills').glob('*/manifest.json'))]
    effective = {**os.environ, **values}
    disabled = (values.get('CP_STORAGE_DRIVER') == 'disabled'
                and effective.get('CP_AGENT_RUNTIME_ENABLED', 'false') in ('false', 'disabled')
                and not effective.get('CP_TELEGRAM_BOTS') and not effective.get('CP_TELEGRAM_DISPATCH_BOTS'))
    owned = load('review_process_owner', 'verify_notifications_sources.py').owned_process
    ownership = {'api': owned('project-templates-api-owner.json', 'api'),
                 'notification_worker': owned('project-templates-worker-owner.json', 'worker-notification'),
                 'report_export_worker': owned('finance-reports-worker-owner.json', 'worker-report-export')}
    draft = len(manifests) == 8 and all(m['state'] == 'draft' for m in manifests)
    project_update = next(m for m in manifests if m['skill_id'] == 'project-update')
    worker_record=json.loads((APP/'.local/finance-reports-worker-owner.json').read_text(encoding='utf-8'))
    worker_matches=worker_record['backend_source_sha256']==load('report_backend_bytes','verify_backend_batches.py').digest()
    passed = (worker_matches and actual['report_tables']==2 and actual['report_guards']==4 and actual['report_update_denied'] and actual['budget_tables']==4 and actual['budget_guards']==4 and actual['budget_immutable']==4 and actual['budget_update_denied'] and actual['budget_policies']==0 and actual['budget_reservations']==0 and common['passed'] and common['database']['migration_count'] == 73 and actual['mutation_append_only'] and actual['task_creation_tool_constraints'] == 3 and actual['task_creation_session_function']
              and actual['deliverable_tool_constraints'] == 3 and actual['deliverable_scope_constraint'] == 1
              and actual['concern_tool_constraints'] == 3 and actual['concern_session_function'] and actual['deliverable_receipt_function'] and actual['milestone_unicode_whitespace'] and actual['deferred_evidence'] == 2
              and actual['active_profiles'] == actual['enabled_telegram_channels'] == actual['disposable_test_databases'] == 0
              and project_update['version'] == '1.6.0' and all(tool in project_update['allowed_tools'] for tool in ('create_task','create_risk','update_risk','create_issue','update_issue'))
              and draft and disabled and all(ownership.values()))
    report = {'passed': bool(passed), 'checked_at': datetime.now(timezone.utc).isoformat(), 'database': actual,
              'migration_count': common['database']['migration_count'], 'all_eight_skills_draft': draft,
              'integrations_disabled': disabled, 'ownership': ownership, 'production_deployment': False,
              'real_cloud_or_model': False, 'report_worker_source_matches':worker_matches, 'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (EVIDENCE / 'development-project-resources.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': bool(passed), 'migration_count': report['migration_count'], 'integrations_disabled': disabled,
                      'disposable_test_databases': actual['disposable_test_databases']}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
