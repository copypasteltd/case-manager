"""Reload attested native services; only file-batch permits normal schema94-to95 upgrade.

The existing cluster is preserved. No provider, TG, cloud, emulator or container
is enabled. A stale PID belonging to another process is never terminated.
"""
from datetime import datetime, timezone
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import time
import psutil
from urllib.parse import unquote, urlsplit
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/project-business'
FLAGS = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0


def inspect(record, command):
    if not record:
        return {'exists': False, 'valid': False}
    try:
        process=psutil.Process(int(record['pid']))
        args=process.cmdline()
        created=datetime.fromtimestamp(process.create_time(), timezone.utc)
        valid=(process.name().lower()=='python.exe' and len(args)>=3
            and args[-2].replace('\\','/').endswith('app/scripts/run_backend.py') and args[-1]==command
            and abs((created-datetime.fromisoformat(record['started_at'])).total_seconds())<10)
        return {'exists':True,'valid':valid,'created_at':created.isoformat()}
    except psutil.NoSuchProcess:
        return {'exists':False,'valid':False}


def main():
    global E
    parser = argparse.ArgumentParser()
    parser.add_argument('--feature', choices=('project-business', 'business-cost', 'cost-payment', 'file-batch', 'document-text', 'office-authoring'), default='project-business')
    args = parser.parse_args()
    E = APP.parent / 'docs/quality/evidence' / args.feature
    expected_count = 96 if args.feature in ('document-text', 'office-authoring') else 95 if args.feature == 'file-batch' else 94
    assert not (E / 'development-update.json').exists(), 'Preserve an existing completed service update.'
    values = json.loads((APP / '.local/environment.json').read_text())
    db = urlsplit(values['CP_DATABASE_URL'])
    assert values['CP_ENV'] == 'development' and db.hostname == '127.0.0.1' and db.port == 14660
    assert values['CP_HTTP_ADDR'] == '127.0.0.1:18080' and values['CP_STORAGE_DRIVER'] == 'disabled'
    effective = {**os.environ, **values}
    assert effective.get('CP_AGENT_RUNTIME_ENABLED', 'false') in ('false', 'disabled')
    assert not any(effective.get(key) for key in ('CP_TELEGRAM_BOTS', 'CP_TELEGRAM_DISPATCH_BOTS', 'CP_TELEGRAM_DELIVERY_BOTS',
        'CP_MULTIPART_CLEANUP_PROVIDERS', 'CP_WECHAT_APP_ID', 'CP_WECHAT_APP_SECRET', 'CP_DOUYIN_APP_ID', 'CP_DOUYIN_APP_SECRET', 'CP_MINIAPP_IDENTITY_KEY'))
    spec = importlib.util.spec_from_file_location('cost_scope_backend', APP / 'scripts/verify_backend_batches.py')
    backend = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(backend)
    source = backend.digest()
    env = {**os.environ, 'PGHOST': db.hostname, 'PGPORT': str(db.port), 'PGUSER': unquote(db.username),
        'PGPASSWORD': unquote(db.password), 'PGDATABASE': unquote(db.path[1:]), 'PGCONNECT_TIMEOUT': '5'}

    def sql(query):
        result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
            env=env, capture_output=True, text=True, encoding='utf-8', timeout=20, creationflags=FLAGS)
        if result.returncode:
            (APP / '.local/project_business-metadata-error.log').write_text(result.stderr, encoding='utf-8')
            raise SystemExit('Native metadata query failed; connection details withheld.')
        return json.loads(result.stdout)

    history_query = "SELECT json_build_object(" + ','.join(
        "'" + table + "',(SELECT json_build_object('rows',count(*),'content_md5',md5(coalesce(string_agg((to_jsonb(t)-'refund_original_amount'-'docx_text_acknowledged'-'engine_sha256'-'page_count'-'ocr_page_count')::text,'' ORDER BY (to_jsonb(t)-'refund_original_amount'-'docx_text_acknowledged'-'engine_sha256'-'page_count'-'ocr_page_count')::text),''))) FROM " + table + " t)"
        for table in ('agent_budget_policies', 'agent_budget_reservations', 'agent_budget_usage', 'agent_budget_reconciliations', 'payment_records', 'payment_revisions', 'payment_events', 'actual_cost_entries', 'cost_revisions', 'cost_allocations', 'contracts', 'contract_revisions', 'contract_lifecycle_events', 'file_version_references', 'file_text_derivations', 'files', 'file_versions', 'folders', 'storage_entries')) + ')'
    before = sql(history_query)
    previous_count = sql('SELECT count(*) FROM application_schema_manifest()')
    E.mkdir(parents=True, exist_ok=True)
    assert previous_count in ((96,) if args.feature == 'office-authoring' else (95, 96) if args.feature == 'document-text' else (94, 95) if args.feature == 'file-batch' else (94,)), 'Only the declared schema transition is allowed.'
    upgraded = previous_count != expected_count
    if upgraded:
        with (APP / ('.local/' + args.feature + '-schema-upgrade.log')).open('w', encoding='utf-8') as log:
            result = subprocess.run([sys.executable, str(APP / 'scripts/run_backend.py'), 'migrate'], cwd=APP.parent, stdout=log, stderr=subprocess.STDOUT, timeout=150, creationflags=FLAGS)
        assert result.returncode == 0, 'Normal schema upgrade failed; ignored local diagnostics retained.'
    after = sql(history_query)
    assert before == after, 'Schema upgrade must preserve all original cash, incurred and legacy/usage bytes.'
    assert sql('SELECT count(*) FROM application_schema_manifest()') == expected_count
    services = [('project-templates-api-owner.json', 'api', 'http server started'),
        ('project-templates-worker-owner.json', 'worker-notification', 'notification worker started'),
        ('finance-reports-worker-owner.json', 'worker-report-export', 'report export worker started')]
    records = []
    for name, command, marker in services:
        path = APP / '.local' / name
        old = json.loads(path.read_text()) if path.exists() else None
        actual = inspect(old, command)
        replaced = False
        stop_code = None
        if actual['valid'] and old.get('backend_source_sha256') != source:
            assert inspect(old, command) == actual
            stopped = subprocess.run(['taskkill', '/PID', str(int(old['pid'])), '/T', '/F'], capture_output=True, creationflags=FLAGS)
            stop_code = stopped.returncode
            # A child may exit while taskkill enumerates a process tree. Require
            # actual owned-process exit; a command status alone cannot prove it.
            deadline = time.monotonic() + 15
            while inspect(old, command)['valid'] and time.monotonic() < deadline:
                time.sleep(.2)
            assert not inspect(old, command)['valid'], 'The attested service is still live; do not start a duplicate.'
            replaced = True
            actual = {'exists': False, 'valid': False}
            if command == 'api':
                # The attested process tree has exited, but Windows can need
                # a short interval to release its listening socket.
                deadline = time.monotonic() + 10
                while time.monotonic() < deadline:
                    with socket.socket() as sock:
                        sock.settimeout(.3)
                        if sock.connect_ex(('127.0.0.1', 18080)) != 0:
                            break
                    time.sleep(.1)
        started = not actual['valid']
        if started:
            if command == 'api':
                with socket.socket() as sock:
                    sock.settimeout(.3)
                    assert sock.connect_ex(('127.0.0.1', 18080)) != 0, 'Another API owns this port; it was preserved.'
            logpath = APP / '.local' / ('project_business-development-current-' + command + '.log')
            with logpath.open('w', encoding='utf-8') as output:
                child = subprocess.Popen([sys.executable, 'app/scripts/run_backend.py', command], cwd=APP.parent,
                    stdout=output, stderr=subprocess.STDOUT, creationflags=FLAGS)
            creation = {'created_at': datetime.fromtimestamp(psutil.Process(child.pid).create_time(),timezone.utc).isoformat()}
            record = {'pid': child.pid, 'command': command, 'started_at': creation['created_at'], 'backend_source_sha256': source}
            path.write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
            deadline = time.monotonic() + 150
            while time.monotonic() < deadline:
                assert child.poll() is None, 'Owned service exited; diagnostics remain ignored.'
                ready = False
                if command == 'api':
                    try:
                        with urlopen('http://127.0.0.1:18080/health/ready', timeout=3) as response:
                            ready = response.status == 200
                    except OSError:
                        pass
                else:
                    ready = marker in logpath.read_text(encoding='utf-8')
                if ready:
                    break
                time.sleep(.25)
            else:
                raise SystemExit('Owned native service readiness was not established.')
        record = json.loads(path.read_text())
        assert inspect(record, command)['valid'] and record['backend_source_sha256'] == source
        records.append({'command': command, 'owned': True, 'started': started, 'replaced_attested_owner': replaced,
            'stop_command_exit_code': stop_code, 'owned_process_exit_confirmed': replaced,
            'stale_unrelated_pid_preserved': bool(actual['exists'] and not actual['valid'])})
    result = {'passed': before == after and source == backend.digest(), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'backend_source_sha256': source, 'previous_migration_count': previous_count, 'migration_count': expected_count,
        'history_before': before, 'history_after': after, 'legacy_and_usage_rows_preserved': before == after,
        'services': records, 'updater_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'schema_upgrade_executed': upgraded, 'postgres_restarted': False, 'external_integration_started': False, 'production_deployment': False}
    (E / 'development-update.json').write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'migration_count': expected_count, 'services_owned': True, 'history_preserved': before == after}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
