"""Upgrade owned native development storage and start/reload only attested services.

The existing cluster is preserved. No provider, TG, cloud, emulator or container
is enabled. A stale PID belonging to another process is never terminated.
"""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import time
from urllib.parse import unquote, urlsplit
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/contract-reminders'
FLAGS = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0


def ps(code):
    result = subprocess.run(['powershell.exe', '-NoProfile', '-NonInteractive', '-Command', code],
        capture_output=True, text=True, encoding='utf-8', timeout=20, creationflags=FLAGS)
    if result.returncode:
        raise SystemExit('Native process inspection failed; nothing was terminated.')
    return json.loads(result.stdout)


def inspect(record, command):
    if not record:
        return {'exists': False, 'valid': False}
    pid = int(record['pid'])
    actual = ps("$p=Get-CimInstance Win32_Process -Filter 'ProcessId = " + str(pid) +
        "';if($p){@{exists=$true;valid=($p.Name -eq 'python.exe' -and $p.CommandLine -like '*app/scripts/run_backend.py " + command +
        "*');created_at=$p.CreationDate.ToUniversalTime().ToString('o')}|ConvertTo-Json -Compress}else{'{\"exists\":false,\"valid\":false}'}")
    if actual['valid']:
        actual['valid'] = abs((datetime.fromisoformat(actual['created_at']) - datetime.fromisoformat(record['started_at'])).total_seconds()) < 10
    return actual


def main():
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
            (APP / '.local/contract-reminders-metadata-error.log').write_text(result.stderr, encoding='utf-8')
            raise SystemExit('Native metadata query failed; connection details withheld.')
        return json.loads(result.stdout)

    history_query = "SELECT json_build_object(" + ','.join(
        "'" + table + "',(SELECT json_build_object('rows',count(*),'content_md5',md5(coalesce(string_agg(to_jsonb(t)::text,'' ORDER BY to_jsonb(t)::text),''))) FROM " + table + " t)"
        for table in ('agent_budget_policies', 'agent_budget_reservations', 'agent_budget_usage', 'agent_budget_reconciliations')) + ')'
    before = sql(history_query)
    previous_count = sql('SELECT count(*) FROM application_schema_manifest()')
    E.mkdir(parents=True, exist_ok=True)
    log = APP / '.local/contract-reminders-development-migration.log'
    with log.open('w', encoding='utf-8') as output:
        status = subprocess.run([sys.executable, 'app/scripts/run_backend.py', 'migrate'], cwd=APP.parent,
            stdout=output, stderr=subprocess.STDOUT, creationflags=FLAGS).returncode
    assert status == 0 and source == backend.digest()
    after = sql(history_query)
    assert before == after, 'Schema upgrade must preserve all legacy/usage bytes.'
    assert sql('SELECT count(*) FROM application_schema_manifest()') == 86
    services = [('project-templates-api-owner.json', 'api', 'http server started'),
        ('project-templates-worker-owner.json', 'worker-notification', 'notification worker started'),
        ('finance-reports-worker-owner.json', 'worker-report-export', 'report export worker started')]
    records = []
    for name, command, marker in services:
        path = APP / '.local' / name
        old = json.loads(path.read_text()) if path.exists() else None
        actual = inspect(old, command)
        replaced = False
        if actual['valid'] and old.get('backend_source_sha256') != source:
            assert inspect(old, command) == actual
            stopped = subprocess.run(['taskkill', '/PID', str(int(old['pid'])), '/T', '/F'], capture_output=True, creationflags=FLAGS)
            assert stopped.returncode == 0
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
            logpath = APP / '.local' / ('contract-reminders-development-' + command + '.log')
            with logpath.open('w', encoding='utf-8') as output:
                child = subprocess.Popen([sys.executable, 'app/scripts/run_backend.py', command], cwd=APP.parent,
                    stdout=output, stderr=subprocess.STDOUT, creationflags=FLAGS)
            creation = ps("$p=Get-CimInstance Win32_Process -Filter 'ProcessId = " + str(child.pid) + "';@{created_at=$p.CreationDate.ToUniversalTime().ToString('o')}|ConvertTo-Json -Compress")
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
            'stale_unrelated_pid_preserved': bool(actual['exists'] and not actual['valid'])})
    result = {'passed': before == after and source == backend.digest(), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'backend_source_sha256': source, 'previous_migration_count': previous_count, 'migration_count': 86,
        'history_before': before, 'history_after': after, 'legacy_and_usage_rows_preserved': before == after,
        'services': records, 'updater_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'postgres_restarted': False, 'external_integration_started': False, 'production_deployment': False}
    (E / 'development-update.json').write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'migration_count': 86, 'services_owned': True, 'history_preserved': before == after}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
