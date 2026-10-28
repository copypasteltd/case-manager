"""Read-only checks of additive76 and exact owned native development services."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlsplit

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/task-mentions'


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
      'mention_array',(SELECT data_type='ARRAY' AND is_nullable='NO' FROM information_schema.columns WHERE table_schema='public' AND table_name='comments' AND column_name='mentioned_user_ids'),
      'mention_guard',(SELECT count(*) FROM pg_trigger WHERE tgname='task_comment_mentions_guard' AND tgenabled='O' AND NOT tgisinternal),
      'comment_immutable_guard',(SELECT count(*) FROM pg_trigger WHERE tgname='comment_revision_immutable' AND tgenabled='O' AND NOT tgisinternal),
      'mention_function_stable',(SELECT provolatile='s' FROM pg_proc WHERE oid='task_comment_mention_eligible(uuid,uuid,uuid,uuid)'::regprocedure),
      'mention_bounds',(SELECT count(*) FROM pg_constraint WHERE conrelid='comments'::regclass AND conname='comments_task_mentions'),
      'notification_context',(SELECT count(*) FROM pg_constraint WHERE conrelid='notification_threads'::regclass AND conname='notification_task_mention_context'),
      'notification_category',(SELECT strpos(pg_get_constraintdef(oid),'mention')>0 FROM pg_constraint WHERE conrelid='notification_threads'::regclass AND conname='notification_threads_category_check'),
      'single_successor',to_regclass('comments_single_successor') IS NOT NULL
    )"""
    result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
        env=environment, capture_output=True, text=True, encoding='utf-8', timeout=20, creationflags=subprocess.CREATE_NO_WINDOW)
    if result.returncode:
        raise SystemExit('Native mention metadata inspection failed; private connection details withheld.')
    actual = json.loads(result.stdout)
    owners = module('verify_notifications_sources').owned_process
    ownership = {'api': owners('project-templates-api-owner.json', 'api'),
        'notification_worker': owners('project-templates-worker-owner.json', 'worker-notification'),
        'report_export_worker': owners('finance-reports-worker-owner.json', 'worker-report-export')}
    backend = module('verify_backend_batches').digest()
    worker_sources = all(json.loads((APP / '.local' / path).read_text()).get('backend_source_sha256') == backend
        for path in ('project-templates-worker-owner.json', 'finance-reports-worker-owner.json'))
    effective = {**os.environ, **values}
    disabled = values.get('CP_STORAGE_DRIVER') == 'disabled' and effective.get('CP_AGENT_RUNTIME_ENABLED', 'false') in ('false', 'disabled') and not effective.get('CP_TELEGRAM_BOTS') and not effective.get('CP_TELEGRAM_DISPATCH_BOTS')
    passed = base_status == 0 and common['passed'] and all(actual.values()) and all(ownership.values()) and worker_sources and disabled and common['database']['disposable_test_databases'] == 0
    report = {'passed': bool(passed), 'checked_at': datetime.now(timezone.utc).isoformat(), 'database': common['database'], 'mention_metadata': actual,
        'migration_count': 76, 'ownership': ownership, 'worker_sources_match': worker_sources, 'backend_source_sha256': backend,
        'integrations_disabled': disabled, 'read_only_inspection': True, 'production_deployment': False,
        'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (E / 'development-project-resources.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'migration_count': 76, 'disposable_databases': common['database']['disposable_test_databases'], 'integrations_disabled': disabled}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
