"""Read-only verification of native knowledge delegation, inherited file guards and disabled external runtime."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
from urllib.error import HTTPError
from urllib.parse import unquote, urlparse
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/secretary-post-writes'


def main():
    values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    db = urlparse(values['CP_DATABASE_URL'])
    if (values.get('CP_ENV') not in ('development', 'local') or db.hostname != '127.0.0.1'
            or db.port != 14660 or values.get('CP_HTTP_ADDR') != '127.0.0.1:18080'):
        raise SystemExit('Only the owned native loopback development environment is supported.')
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    spec = importlib.util.spec_from_file_location('file_runtime_checks', APP / 'scripts/verify_agent_execution_development.py')
    baseline = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(baseline)
    baseline.EVIDENCE = EVIDENCE
    if baseline.main():
        return 1
    common = json.loads((EVIDENCE / 'development-runtime.json').read_text(encoding='utf-8'))
    environment = {**os.environ, 'PGHOST': db.hostname, 'PGPORT': str(db.port), 'PGUSER': unquote(db.username or ''),
                   'PGPASSWORD': unquote(db.password or ''), 'PGDATABASE': unquote(db.path.lstrip('/')), 'PGCONNECT_TIMEOUT': '5'}
    query = """SELECT json_build_object(
      'can_read_file_evidence',(SELECT bool_and(has_table_privilege(current_user,t,'SELECT'))
        FROM unnest(ARRAY['files','folders','file_versions','file_replicas','storage_targets'])t),
      'historical_creator_nullable',(SELECT NOT attnotnull FROM pg_attribute WHERE attrelid='files'::regclass AND attname='creator_id'),
      'historical_original_name_nullable',(SELECT NOT attnotnull FROM pg_attribute WHERE attrelid='file_versions'::regclass AND attname='original_name'),
      'append_only_file_tables',(SELECT bool_and(has_table_privilege(current_user,t,'SELECT') AND has_table_privilege(current_user,t,'INSERT') AND NOT has_table_privilege(current_user,t,'UPDATE') AND NOT has_table_privilege(current_user,t,'DELETE')) FROM unnest(ARRAY['agent_file_intents','agent_file_drafts','agent_file_stages','agent_file_proposals','agent_file_commits'])t),
      'file_update_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O' AND tgname LIKE 'agent_file_%'),
      'original_name_immutable_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O'
        AND tgname='file_version_original_name_immutable'),
      'creation_ids_nullable',(SELECT count(*)=2 FROM pg_attribute WHERE attrelid='agent_file_intents'::regclass AND attname IN ('file_id','base_version_id') AND NOT attnotnull),
      'legacy_update_default',(SELECT position('update' in pg_get_expr(d.adbin,d.adrelid))>0 FROM pg_attrdef d JOIN pg_attribute a ON a.attrelid=d.adrelid AND a.attnum=d.adnum WHERE a.attrelid='agent_file_intents'::regclass AND a.attname='mode'),
      'mode_scope_enforced',(SELECT position('create' in pg_get_constraintdef(oid))>0 AND position('update' in pg_get_constraintdef(oid))>0 AND position('file_version = 0' in pg_get_constraintdef(oid))>0 FROM pg_constraint WHERE conrelid='agent_file_intents'::regclass AND conname='agent_file_intent_mode_scope'),
      'exact_creation_binding',(SELECT position('creation intent requires its exact current directory' in prosrc)>0 AND position('IS DISTINCT FROM i.file_id' in prosrc)>0 AND position('/internal/agent-files/create' in prosrc)>0 FROM pg_proc WHERE proname='validate_agent_file_update_binding'),
      'creation_source_and_tool_evidence',(SELECT position('agent_file_create' in prosrc)>0 AND position('commit_file_create' in prosrc)>0 FROM pg_proc WHERE proname='validate_agent_file_update_evidence'),
      'oa_append_only_tables',(SELECT bool_and(has_table_privilege(current_user,t,'SELECT') AND has_table_privilege(current_user,t,'INSERT') AND NOT has_table_privilege(current_user,t,'UPDATE') AND NOT has_table_privilege(current_user,t,'DELETE')) FROM unnest(ARRAY['agent_oa_intents','agent_oa_commits'])t),
      'oa_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O' AND tgname LIKE 'agent_oa_%'),
      'oa_original_session_binding',(SELECT position('sessions s JOIN users' in prosrc)>0 AND position('s.id=i.session_id' in prosrc)>0 AND position('s.device_type=x.channel' in prosrc)>0 FROM pg_proc WHERE proname='validate_agent_oa_binding'),
      'oa_exact_arguments_and_receipt',(SELECT position('expected_business_version' in prosrc)>0 AND position('preview_hash' in prosrc)>0 AND position('o.response_data=NEW.result_data' in prosrc)>0 AND position('approval_steps' in prosrc)>0 FROM pg_proc WHERE proname='validate_agent_oa_binding'),
      'oa_sender_and_tool_evidence',(SELECT position('conversation_messages' in prosrc)>0 AND position('agent_oa_request' in prosrc)>0 AND position('agent_tool_receipts' in prosrc)>0 AND position('operation_id' in prosrc)>0 FROM pg_proc WHERE proname='validate_agent_oa_evidence'),
      'oa_tool_allowlists',(SELECT count(*) FROM pg_constraint WHERE conname IN ('agent_tool_receipts_tool_check','agent_execution_observations_tool_check') AND position('draft_request' in pg_get_constraintdef(oid))>0 AND position('submit_request' in pg_get_constraintdef(oid))>0),
      'post_append_only_tables',(SELECT bool_and(has_table_privilege(current_user,t,'SELECT') AND has_table_privilege(current_user,t,'INSERT') AND NOT has_table_privilege(current_user,t,'UPDATE') AND NOT has_table_privilege(current_user,t,'DELETE')) FROM unnest(ARRAY['agent_post_intents','agent_post_commits'])t),
      'post_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O' AND tgname LIKE 'agent_post_%'),
      'post_original_session_binding',(SELECT position('sessions s JOIN users' in prosrc)>0 AND position('s.id=i.session_id' in prosrc)>0 AND position('s.device_type=x.channel' in prosrc)>0 FROM pg_proc WHERE proname='validate_agent_post_binding'),
      'post_exact_content_and_receipt',(SELECT position('r.body=NEW.arguments' in prosrc)>0 AND position('p.published_revision_id<>p.current_revision_id' in prosrc)>0 AND position('o.response_data=NEW.result_data' in prosrc)>0 AND position('base_revision_id' in prosrc)>0 FROM pg_proc WHERE proname='validate_agent_post_binding'),
      'post_sender_and_tool_evidence',(SELECT position('conversation_messages' in prosrc)>0 AND position('agent_post_write' in prosrc)>0 AND position('agent_tool_receipts' in prosrc)>0 AND position('operation_id' in prosrc)>0 FROM pg_proc WHERE proname='validate_agent_post_evidence'),
      'post_tool_allowlists',(SELECT count(*) FROM pg_constraint WHERE conname IN ('agent_tool_receipts_tool_check','agent_execution_observations_tool_check') AND position('draft_post' in pg_get_constraintdef(oid))>0 AND position('publish_post' in pg_get_constraintdef(oid))>0),
      'can_rewrite_tool_receipts',has_table_privilege(current_user,'agent_tool_receipts','UPDATE')
        OR has_table_privilege(current_user,'agent_tool_receipts','DELETE'),
      'active_runtime_profiles',(SELECT count(*) FROM agent_runtime_profiles WHERE state='active'),
      'disposable_test_databases',(SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[0-9a-f]{32}$'))"""
    result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
                            env=environment, capture_output=True, text=True, encoding='utf-8', timeout=15,
                            creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if result.returncode:
        raise SystemExit('Owned file-creation database checks failed; private diagnostics withheld.')
    database = json.loads(result.stdout)
    identifier = '00000000-0000-4000-8000-000000000000'
    route = '/v1/conversations/' + identifier + '/context-sources/file/' + identifier + '?context_security_version=1&version_id=' + identifier
    try:
        with urlopen('http://127.0.0.1:18080' + route, timeout=5) as response:
            status = response.status
    except HTTPError as error:
        status = error.code
        error.close()
    skill_states = [json.loads(p.read_text(encoding='utf-8'))['state'] for p in sorted((APP / 'skills').glob('*/manifest.json'))]
    effective = {**os.environ, **values}
    disabled = effective.get('CP_AGENT_FILE_PROVIDERS', '') in ('', 'disabled')
    writing_disabled = effective.get('CP_AGENT_DRAFT_DRIVER', '') in ('', 'disabled')
    passed = bool(common['passed'] and common['database']['migration_count'] == 57
                  and database['post_append_only_tables'] and database['post_guards'] == 6 and database['post_tool_allowlists'] == 2
                  and all(database[k] for k in ('post_original_session_binding','post_exact_content_and_receipt','post_sender_and_tool_evidence'))
                  and database['oa_append_only_tables'] and database['oa_guards'] == 6 and database['oa_tool_allowlists'] == 2
                  and all(database[k] for k in ('oa_original_session_binding','oa_exact_arguments_and_receipt','oa_sender_and_tool_evidence'))
                  and database['append_only_file_tables'] and database['file_update_guards'] == 13
                  and all(database[key] for key in ('creation_ids_nullable','legacy_update_default','mode_scope_enforced','exact_creation_binding','creation_source_and_tool_evidence'))
                  and database['can_read_file_evidence'] and database['historical_creator_nullable']
                  and database['historical_original_name_nullable'] and database['original_name_immutable_guards'] == 1
                  and not database['can_rewrite_tool_receipts'] and database['active_runtime_profiles'] == 0
                  and database['disposable_test_databases'] == 0 and skill_states == ['draft'] * 8 and status == 401 and disabled and writing_disabled)
    report = {'passed': passed, 'checked_at': datetime.now(timezone.utc).isoformat(), 'database': database,
              'migration_count': common['database']['migration_count'], 'actual_skill_states': skill_states,
              'file_context_probe': {'method': 'GET', 'path': route, 'status': status, 'expected': 401, 'passed': status == 401},
              'agent_file_reading_disabled_in_development_configuration': disabled,
              'agent_file_writing_disabled_in_development_configuration': writing_disabled,
              'environment': 'owned native Windows Go/PostgreSQL; no virtualization',
              'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'real_model_invoked': False, 'actual_cloud_invoked': False, 'production_deployed': False}
    (EVIDENCE / 'development-post-writes.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': passed, 'migration_count': report['migration_count'], 'unauthenticated_file_context_status': status}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
