"""Read-only current checks of schema94 and exact owned native development services."""
from datetime import datetime, timezone
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlsplit

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/project-business'


def module(name):
    spec = importlib.util.spec_from_file_location('mention_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def main():
    global E
    parser = argparse.ArgumentParser()
    parser.add_argument('--feature', choices=('project-business', 'business-cost', 'cost-payment', 'file-batch', 'document-text', 'office-authoring'), default='project-business')
    args = parser.parse_args()
    E = APP.parent / 'docs/quality/evidence' / args.feature
    values = json.loads((APP / '.local/environment.json').read_text())
    db = urlsplit(values['CP_DATABASE_URL'])
    assert db.hostname == '127.0.0.1' and db.port == 14660 and values.get('CP_HTTP_ADDR') == '127.0.0.1:18080'
    base = module('verify_agent_execution_development')
    base.EVIDENCE = E
    base_status = base.main()
    common = json.loads((E / 'development-runtime.json').read_text())
    environment = {**os.environ, 'PGHOST': db.hostname, 'PGPORT': str(db.port), 'PGUSER': unquote(db.username),
        'PGPASSWORD': unquote(db.password), 'PGDATABASE': unquote(db.path[1:]), 'PGCONNECT_TIMEOUT': '5'}
    query = """SELECT jsonb_build_object(
      'refund_principal_column',EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='payment_revisions' AND column_name='refund_original_amount' AND is_generated='ALWAYS'),
      'refund_quote_guards',(SELECT count(*)=3 FROM pg_trigger WHERE tgname IN ('payment_refund_quote','payment_foreign_refund_no_attribution','contract_foreign_refund_no_attribution') AND tgenabled='O'),
      'docx_selector',file_text_extractor('application/vnd.openxmlformats-officedocument.wordprocessingml.document')='docx-v2',
      'docx_source_guard',EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='docx_text_source' AND tgenabled='O' AND NOT tgisinternal),
      'office_selector',file_text_extractor('application/vnd.openxmlformats-officedocument.spreadsheetml.sheet')='office-v1' AND file_text_extractor('text/plain')='text-v1',
      'office_source_guard',EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='office_text_source' AND tgenabled='O' AND NOT tgisinternal),
      'fee_links',to_regclass('payment_fee_links') IS NOT NULL,
      'fee_revocations',to_regclass('payment_fee_revocations') IS NOT NULL,
      'fee_guards',(SELECT count(*)=4 FROM pg_trigger WHERE tgname IN ('payment_fee_link_evidence','payment_fee_revocation_evidence','payment_fee_link_immutable','payment_fee_revocation_immutable') AND tgenabled='O' AND NOT tgisinternal),
      'fee_no_rewrite',NOT has_table_privilege(current_user,'payment_fee_links','UPDATE') AND NOT has_table_privilege(current_user,'payment_fee_links','DELETE') AND NOT has_table_privilege(current_user,'payment_fee_revocations','UPDATE') AND NOT has_table_privilege(current_user,'payment_fee_revocations','DELETE'),
      'contract_body_intents',to_regclass('agent_contract_body_intents') IS NOT NULL,
      'contract_body_adoptions',to_regclass('agent_contract_body_adoptions') IS NOT NULL,
      'contract_body_guards',(SELECT count(*)=4 FROM pg_trigger WHERE tgname IN ('agent_contract_body_intent_binding','agent_contract_body_adoption_binding','agent_contract_body_intent_immutable','agent_contract_body_adoption_immutable') AND tgenabled='O' AND NOT tgisinternal),
      'contract_body_no_rewrite',NOT has_table_privilege(current_user,'agent_contract_body_intents','UPDATE') AND NOT has_table_privilege(current_user,'agent_contract_body_intents','DELETE') AND NOT has_table_privilege(current_user,'agent_contract_body_adoptions','UPDATE') AND NOT has_table_privilege(current_user,'agent_contract_body_adoptions','DELETE')
      ) || jsonb_build_object(
      'payment_fx_table',to_regclass('file_scope_transfers') IS NOT NULL,
      'payment_fx_guard',EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='file_scope_transfer_guard' AND tgenabled='O' AND NOT tgisinternal),
      'payment_fx_no_delete',NOT has_table_privilege(current_user,'file_scope_transfers','DELETE'),
      'payment_dual_columns',(SELECT count(*)=5 FROM information_schema.columns WHERE table_name='payment_allocations' AND column_name IN ('cost_amount','fx_rate','fx_source','fx_quoted_on','amount')),
      'payment_fx_guards',(SELECT count(*)=4 FROM pg_trigger WHERE tgname IN ('payment_allocation_z_fx_basis','payment_share_fx_basis','validate_payment_fx_allocation','validate_payment_fx_share') AND tgenabled='O' AND NOT tgisinternal),
      'payment_ledger_append_only',NOT has_table_privilege(current_user,'payment_allocations','UPDATE') AND NOT has_table_privilege(current_user,'payment_allocations','DELETE'),
      'contract_policy',to_regclass('notification_contract_policies') IS NOT NULL,
      'contract_policy_append_only',has_table_privilege(current_user,'notification_contract_policies','SELECT') AND has_table_privilege(current_user,'notification_contract_policies','INSERT') AND NOT has_table_privilege(current_user,'notification_contract_policies','UPDATE') AND NOT has_table_privilege(current_user,'notification_contract_policies','DELETE'),
      'contract_policy_guards',(SELECT count(*)=4 FROM pg_trigger WHERE tgname IN ('notification_contract_policy_immutable','notification_contract_policy_input','notification_contract_policy_evidence','notification_due_policy') AND tgenabled='O' AND NOT tgisinternal),
      'contract_policy_roles',(SELECT count(*)=2 FROM roles WHERE key IN ('notification_policy_reader','notification_policy_manager')) AND NOT EXISTS(SELECT 1 FROM role_actions a JOIN roles r ON r.org_id=a.org_id AND r.id=a.role_id WHERE a.action='notification.policy.manage' AND r.key IN ('notification_operator','notification_retry_operator','notification_policy_reader')),
      'notification_retries',to_regclass('notification_retries') IS NOT NULL,
      'notification_retry_append_only',has_table_privilege(current_user,'notification_retries','SELECT') AND has_table_privilege(current_user,'notification_retries','INSERT') AND NOT has_table_privilege(current_user,'notification_retries','UPDATE') AND NOT has_table_privilege(current_user,'notification_retries','DELETE'),
      'notification_retry_guards',(SELECT count(*)=3 FROM pg_trigger WHERE tgname IN ('notification_retry_immutable','notification_retry_input','notification_retry_evidence') AND tgenabled='O' AND NOT tgisinternal),
      'notification_retry_role',EXISTS(SELECT 1 FROM roles r JOIN role_actions a ON a.org_id=r.org_id AND a.role_id=r.id WHERE r.key='notification_retry_operator' AND r.requires_mfa AND a.action='notification.delivery.retry') AND NOT EXISTS(SELECT 1 FROM roles r JOIN role_actions a ON a.org_id=r.org_id AND a.role_id=r.id WHERE r.key='notification_operator' AND a.action='notification.delivery.retry'),
      'audit_exports',to_regclass('audit_exports') IS NOT NULL AND to_regclass('audit_export_events') IS NOT NULL,
      'audit_no_rewrite',NOT has_table_privilege(current_user,'audit_exports','UPDATE') AND NOT has_table_privilege(current_user,'audit_exports','DELETE') AND NOT has_table_privilege(current_user,'audit_export_events','UPDATE') AND NOT has_table_privilege(current_user,'audit_export_events','DELETE'),
      'schema94',(SELECT count(*)=94 FROM application_schema_manifest()),
      'amortization_versions',to_regclass('cost_amortization_versions') IS NOT NULL,
      'amortization_children',to_regclass('cost_amortization_entries') IS NOT NULL AND to_regclass('cost_amortization_shares') IS NOT NULL,
      'amortization_guards',(SELECT count(*)=7 FROM pg_trigger WHERE tgname LIKE 'cost_amortization_%' AND tgenabled='O' AND NOT tgisinternal),
      'file_recovery_table',to_regclass('agent_file_recoveries') IS NOT NULL,
      'file_recovery_immutable',NOT has_table_privilege(current_user,'agent_file_recoveries','UPDATE') AND NOT has_table_privilege(current_user,'agent_file_recoveries','DELETE'),
      'file_recovery_guards',(SELECT count(*)=5 FROM pg_trigger WHERE tgname IN ('agent_file_recovery_immutable','agent_file_recovery_binding','agent_file_recovery_draft','agent_file_recovery_intent_evidence','agent_file_recovery_stage_evidence') AND tgenabled='O' AND NOT tgisinternal),
      'amortization_no_update',NOT has_table_privilege(current_user,'cost_amortization_versions','UPDATE') AND NOT has_table_privilege(current_user,'cost_amortization_entries','UPDATE') AND NOT has_table_privilege(current_user,'cost_amortization_shares','UPDATE'),
      'amortization_no_delete',NOT has_table_privilege(current_user,'cost_amortization_versions','DELETE') AND NOT has_table_privilege(current_user,'cost_amortization_entries','DELETE') AND NOT has_table_privilege(current_user,'cost_amortization_shares','DELETE'),
      'cleanup_plans',to_regclass('upload_cleanup_plans') IS NOT NULL,
      'cleanup_receipts',to_regclass('upload_cleanup_receipts') IS NOT NULL,
      'cleanup_no_rewrite',NOT has_table_privilege(current_user,'upload_cleanup_receipts','UPDATE') AND NOT has_table_privilege(current_user,'upload_cleanup_receipts','DELETE'),
      'cleanup_guards',(SELECT count(*)=3 FROM pg_trigger WHERE tgname IN ('upload_cleanup_plan_guard','upload_cleanup_item_guard','upload_cleanup_receipt_immutable') AND tgenabled='O' AND NOT tgisinternal),
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
    if args.feature in ('file-batch', 'document-text', 'office-authoring'):
        query = query.replace("'schema94',(SELECT count(*)=94", "'schema95',(SELECT count(*)=95")
        query = query.replace("'refund_principal_column'", "'file_batch_table',to_regclass('file_batch_plans') IS NOT NULL,'file_batch_guard',EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='file_batch_plan_guard' AND tgenabled='O' AND NOT tgisinternal),'file_batch_privileges',has_table_privilege(current_user,'file_batch_plans','SELECT') AND has_table_privilege(current_user,'file_batch_plans','INSERT') AND has_table_privilege(current_user,'file_batch_plans','UPDATE') AND NOT has_table_privilege(current_user,'file_batch_plans','DELETE'),'refund_principal_column'")
    if args.feature in ('document-text', 'office-authoring'):
        query = query.replace("'schema95',(SELECT count(*)=95", "'schema96',(SELECT count(*)=96")
        query = query.replace("'refund_principal_column'", "'document_selector',file_text_extractor('application/pdf')='document-ocr-v1' AND file_text_extractor('image/png')='document-ocr-v1' AND file_text_extractor('image/jpeg')='document-ocr-v1','document_guard',EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='document_text_source' AND tgenabled='O' AND NOT tgisinternal),'document_columns',(SELECT count(*)=3 FROM information_schema.columns WHERE table_name='file_text_derivations' AND column_name IN ('engine_sha256','page_count','ocr_page_count')),'refund_principal_column'")
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
    disabled = values.get('CP_STORAGE_DRIVER') == 'disabled' and effective.get('CP_AGENT_RUNTIME_ENABLED', 'false') in ('false', 'disabled') and not effective.get('CP_TELEGRAM_BOTS') and not effective.get('CP_TELEGRAM_DISPATCH_BOTS') and not any(effective.get(k) for k in ('CP_MULTIPART_CLEANUP_PROVIDERS', 'CP_WECHAT_APP_ID','CP_WECHAT_APP_SECRET','CP_DOUYIN_APP_ID','CP_DOUYIN_APP_SECRET','CP_MINIAPP_IDENTITY_KEY'))
    passed = base_status == 0 and common['passed'] and all(actual.values()) and all(ownership.values()) and worker_sources and disabled and common['database']['disposable_test_databases'] == 0
    report = {'passed': bool(passed), 'checked_at': datetime.now(timezone.utc).isoformat(), 'database': common['database'], 'miniapp_metadata': actual,
        'migration_count': (96 if args.feature in ('document-text', 'office-authoring') else 95 if args.feature == 'file-batch' else 94), 'ownership': ownership, 'worker_sources_match': worker_sources, 'backend_source_sha256': backend,
        'integrations_disabled': disabled, 'read_only_inspection': True, 'production_deployment': False,
        'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (E / 'development-project-resources.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'migration_count': (96 if args.feature in ('document-text', 'office-authoring') else 95 if args.feature == 'file-batch' else 94), 'disposable_databases': common['database']['disposable_test_databases'], 'integrations_disabled': disabled}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
