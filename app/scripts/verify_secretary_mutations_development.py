"""Read-only evidence for the owned native L1 update schema and API."""
from datetime import datetime, timezone
import importlib.util
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlparse

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/secretary-mutations'


def main():
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    spec = importlib.util.spec_from_file_location('mutation_runtime_checks', APP / 'scripts/verify_agent_execution_development.py')
    baseline = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(baseline)
    baseline.EVIDENCE = EVIDENCE
    if baseline.main():
        return 1
    common = json.loads((EVIDENCE / 'development-runtime.json').read_text(encoding='utf-8'))
    values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    db = urlparse(values['CP_DATABASE_URL'])
    if values.get('CP_ENV') not in ('development', 'local') or db.hostname != '127.0.0.1' or db.port != 14660 or values.get('CP_HTTP_ADDR') != '127.0.0.1:18080':
        raise SystemExit('Only the owned native loopback development environment is supported.')
    environment = {**os.environ, 'PGHOST': db.hostname, 'PGPORT': str(db.port), 'PGUSER': unquote(db.username or ''),
                   'PGPASSWORD': unquote(db.password or ''), 'PGDATABASE': unquote(db.path.lstrip('/')), 'PGCONNECT_TIMEOUT': '5'}
    query = """SELECT json_build_object(
      'can_read_intents',has_table_privilege(current_user,'agent_run_mutations','SELECT'),
      'can_insert_intents',has_table_privilege(current_user,'agent_run_mutations','INSERT'),
      'can_rewrite_intents',has_table_privilege(current_user,'agent_run_mutations','UPDATE') OR has_table_privilege(current_user,'agent_run_mutations','DELETE'),
      'can_read_commits',has_table_privilege(current_user,'agent_mutation_commits','SELECT'),
      'can_insert_commits',has_table_privilege(current_user,'agent_mutation_commits','INSERT'),
      'can_rewrite_commits',has_table_privilege(current_user,'agent_mutation_commits','UPDATE') OR has_table_privilege(current_user,'agent_mutation_commits','DELETE'),
      'mutation_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN
        ('agent_mutation_immutable','agent_mutation_commit_immutable','agent_mutation_binding','agent_mutation_commit_binding','agent_mutation_sender_evidence','agent_mutation_tool_evidence')),
      'deferred_mutation_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgdeferrable AND tginitdeferred
        AND tgname IN ('agent_mutation_sender_evidence','agent_mutation_tool_evidence')),
      'active_runtime_profiles',(SELECT count(*) FROM agent_runtime_profiles WHERE state='active'),
      'disposable_test_databases',(SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[0-9a-f]{32}$'))"""
    checked = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
                             env=environment, capture_output=True, text=True, encoding='utf-8', timeout=15,
                             creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if checked.returncode:
        raise SystemExit('Owned mutation database checks failed; private diagnostics withheld.')
    database = json.loads(checked.stdout)
    skill_states = [json.loads(p.read_text(encoding='utf-8'))['state'] for p in sorted((APP / 'skills').glob('*/manifest.json'))]
    passed = bool(common['passed'] and common['database']['migration_count'] == 50
                  and database['mutation_guards'] == 6 and database['deferred_mutation_guards'] == 2
                  and all(database[k] for k in ('can_read_intents', 'can_insert_intents', 'can_read_commits', 'can_insert_commits'))
                  and not database['can_rewrite_intents'] and not database['can_rewrite_commits']
                  and database['active_runtime_profiles'] == 0 and database['disposable_test_databases'] == 0
                  and skill_states == ['draft'] * 8)
    report = {'passed': passed, 'checked_at': datetime.now(timezone.utc).isoformat(), 'database': database,
              'migration_count': common['database']['migration_count'], 'actual_skill_states': skill_states,
              'environment': 'owned native Windows Go/PostgreSQL; no virtualization',
              'real_model_invoked': False, 'production_deployed': False}
    (EVIDENCE / 'development-mutations.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': passed, 'migration_count': common['database']['migration_count'], 'mutation_guards': database['mutation_guards']}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
