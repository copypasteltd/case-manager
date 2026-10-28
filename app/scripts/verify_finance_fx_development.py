"""Read-only current checks of unchanged schema81 and exact owned native development services."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlsplit

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/finance-fx'


def module(name):
    spec = importlib.util.spec_from_file_location('mention_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def main():
    values = json.loads((APP / '.local/environment.json').read_text())
    db = urlsplit(values['CP_DATABASE_URL'])
    assert db.hostname == '127.0.0.1' and db.port == 14660 and values.get('CP_HTTP_ADDR') == '127.0.0.1:18080'
    base = module('verify_agent_execution_development')
    base.EVIDENCE = E
    base_status = base.main()
    common = json.loads((E / 'development-runtime.json').read_text())
    environment = {**os.environ, 'PGHOST': db.hostname, 'PGPORT': str(db.port), 'PGUSER': unquote(db.username),
        'PGPASSWORD': unquote(db.password), 'PGDATABASE': unquote(db.path[1:]), 'PGCONNECT_TIMEOUT': '5'}
    query = """SELECT json_build_object(
      'fx_books',to_regclass('finance_fx_books') IS NOT NULL,
      'fx_immutable',NOT has_table_privilege(current_user,'finance_fx_books','UPDATE') AND NOT has_table_privilege(current_user,'finance_fx_books','DELETE'),
      'fx_guards',(SELECT count(*)=3 FROM pg_trigger WHERE tgname IN ('finance_fx_book_binding','finance_fx_book_immutable','finance_base_currency_policy') AND tgenabled='O' AND NOT tgisinternal),
      'oa_text_drafts',to_regclass('agent_oa_text_drafts') IS NOT NULL,
      'oa_text_adoptions',to_regclass('agent_oa_text_adoptions') IS NOT NULL,
      'oa_text_immutable',(SELECT count(*)=2 FROM pg_trigger WHERE tgname IN ('agent_oa_text_draft_immutable','agent_oa_text_adoption_immutable') AND tgenabled='O' AND NOT tgisinternal),
      'oa_text_no_rewrite',NOT has_table_privilege(current_user,'agent_oa_text_drafts','UPDATE') AND NOT has_table_privilege(current_user,'agent_oa_text_drafts','DELETE')
        AND NOT has_table_privilege(current_user,'agent_oa_text_adoptions','UPDATE') AND NOT has_table_privilege(current_user,'agent_oa_text_adoptions','DELETE'),
      'telemetry_run_fk',EXISTS(SELECT 1 FROM pg_constraint WHERE conname='agent_execution_usage_run_fkey' AND contype='f' AND convalidated),
      'no_cost_policy_insert',NOT has_table_privilege(current_user,'agent_budget_policies','INSERT'),
      'no_cost_reservation_insert',NOT has_table_privilege(current_user,'agent_budget_reservations','INSERT'),
      'no_cost_reconciliation_insert',NOT has_table_privilege(current_user,'agent_budget_reconciliations','INSERT'),
      'bindings',to_regclass('miniapp_bindings') IS NOT NULL,
      'proofs',to_regclass('miniapp_binding_proofs') IS NOT NULL,
      'single_use_codes',to_regclass('miniapp_code_claims') IS NOT NULL,
      'session_source',EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='sessions' AND column_name='miniapp_binding_id'),
      'challenge_source',EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='auth_challenges' AND column_name='miniapp_binding_id'),
      'history_guards',(SELECT count(*)=2 FROM pg_trigger WHERE tgname IN ('miniapp_binding_guard','miniapp_claim_immutable') AND tgenabled='O' AND NOT tgisinternal),
      'identity_digest_immutable',NOT has_column_privilege(current_user,'miniapp_bindings','subject_hash','UPDATE'),
      'no_binding_delete',NOT has_table_privilege(current_user,'miniapp_bindings','DELETE'),
      'no_claim_delete',NOT has_table_privilege(current_user,'miniapp_code_claims','DELETE'),
      'disposable_test_databases',(SELECT count(*) FROM pg_database WHERE datname~'^cp_(test|suite|batch|fixture)_'))"""
    result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
        env=environment, capture_output=True, text=True, encoding='utf-8', timeout=20, creationflags=subprocess.CREATE_NO_WINDOW)
    if result.returncode:
        raise SystemExit('Native mini-program metadata inspection failed; private connection details withheld.')
    actual = json.loads(result.stdout)
    common['database']['disposable_test_databases'] = actual.pop('disposable_test_databases')
    owners = module('verify_notifications_sources').owned_process
    ownership = {'api': owners('project-templates-api-owner.json', 'api'),
        'notification_worker': owners('project-templates-worker-owner.json', 'worker-notification'),
        'report_export_worker': owners('finance-reports-worker-owner.json', 'worker-report-export')}
    backend = module('verify_backend_batches').digest()
    worker_sources = all(json.loads((APP / '.local' / path).read_text()).get('backend_source_sha256') == backend
        for path in ('project-templates-worker-owner.json', 'finance-reports-worker-owner.json'))
    effective = {**os.environ, **values}
    disabled = values.get('CP_STORAGE_DRIVER') == 'disabled' and effective.get('CP_AGENT_RUNTIME_ENABLED', 'false') in ('false', 'disabled') and not effective.get('CP_TELEGRAM_BOTS') and not effective.get('CP_TELEGRAM_DISPATCH_BOTS') and not any(effective.get(k) for k in ('CP_WECHAT_APP_ID','CP_WECHAT_APP_SECRET','CP_DOUYIN_APP_ID','CP_DOUYIN_APP_SECRET','CP_MINIAPP_IDENTITY_KEY'))
    passed = base_status == 0 and common['passed'] and all(actual.values()) and all(ownership.values()) and worker_sources and disabled and common['database']['disposable_test_databases'] == 0
    report = {'passed': bool(passed), 'checked_at': datetime.now(timezone.utc).isoformat(), 'database': common['database'], 'miniapp_metadata': actual,
        'migration_count': 81, 'ownership': ownership, 'worker_sources_match': worker_sources, 'backend_source_sha256': backend,
        'integrations_disabled': disabled, 'read_only_inspection': True, 'production_deployment': False,
        'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (E / 'development-project-resources.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'migration_count': 81, 'disposable_databases': common['database']['disposable_test_databases'], 'integrations_disabled': disabled}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
