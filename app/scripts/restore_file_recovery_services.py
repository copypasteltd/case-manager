"""Restore confirmed stopped owned development services without rerunning schema upgrades."""
from datetime import datetime, timezone
import hashlib
import json
import os
import socket
import subprocess
import sys
import time
from urllib.request import urlopen
import update_file_recovery_development as owner_tools
from verify_backend_batches import digest

APP, E, FLAGS = owner_tools.APP, owner_tools.E, owner_tools.FLAGS

def main():
    target = E / 'development-service-restoration.json'
    assert not target.exists()
    configuration = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    effective = {**os.environ, **configuration}
    assert configuration['CP_ENV'] == 'development' and configuration['CP_HTTP_ADDR'] == '127.0.0.1:18080'
    assert configuration['CP_STORAGE_DRIVER'] == 'disabled' and effective.get('CP_AGENT_RUNTIME_ENABLED', 'false') in ('false', 'disabled')
    assert not any(effective.get(k) for k in ('CP_TELEGRAM_BOTS', 'CP_TELEGRAM_DISPATCH_BOTS', 'CP_TELEGRAM_DELIVERY_BOTS', 'CP_MULTIPART_CLEANUP_PROVIDERS', 'CP_WECHAT_APP_ID', 'CP_WECHAT_APP_SECRET', 'CP_DOUYIN_APP_ID', 'CP_DOUYIN_APP_SECRET', 'CP_MINIAPP_IDENTITY_KEY'))
    upgrade = E / 'development-update.json'
    before = upgrade.read_bytes()
    previous = json.loads(before)
    source = digest()
    assert previous['passed'] and previous['previous_migration_count'] == 86 and previous['migration_count'] == 87 and previous['backend_source_sha256'] == source
    records = []
    children = []
    success = False
    try:
        for name, command, marker in [('project-templates-api-owner.json', 'api', 'http server started'),
            ('project-templates-worker-owner.json', 'worker-notification', 'notification worker started'),
            ('finance-reports-worker-owner.json', 'worker-report-export', 'report export worker started')]:
            path = APP / '.local' / name
            old = json.loads(path.read_text(encoding='utf-8'))
            actual = owner_tools.inspect(old, command)
            assert not actual['valid'], 'An original owned service is live; inspect before starting a duplicate.'
            if command == 'api':
                with socket.socket() as probe:
                    probe.settimeout(.5)
                    assert probe.connect_ex(('127.0.0.1', 18080)) != 0, 'Existing listener was preserved.'
            log = APP / '.local' / ('file-recovery-restored-' + command + '.log')
            with log.open('x', encoding='utf-8') as output:
                child = subprocess.Popen([sys.executable, 'app/scripts/run_backend.py', command], cwd=APP.parent,
                    stdout=output, stderr=subprocess.STDOUT, creationflags=FLAGS)
            children.append(child)
            creation = owner_tools.ps("$p=Get-CimInstance Win32_Process -Filter 'ProcessId = " + str(child.pid) + "';@{created_at=$p.CreationDate.ToUniversalTime().ToString('o')}|ConvertTo-Json -Compress")
            new = {'pid': child.pid, 'command': command, 'started_at': creation['created_at'], 'backend_source_sha256': source}
            path.write_text(json.dumps(new, indent=2) + '\n', encoding='utf-8')
            deadline = time.monotonic() + 150
            while time.monotonic() < deadline:
                assert child.poll() is None, 'Owned service exited; inspect its private output.'
                ready = marker in log.read_text(encoding='utf-8')
                if command == 'api':
                    try:
                        with urlopen('http://127.0.0.1:18080/health/ready', timeout=3) as response:
                            ready = response.status == 200
                    except OSError:
                        ready = False
                if ready:
                    break
                time.sleep(.25)
            else:
                raise RuntimeError('Owned service did not become ready.')
            assert owner_tools.inspect(new, command)['valid']
            records.append({'command': command, 'started_after_confirmed_exit': True, 'current_source_attested': True,
                'stale_unrelated_pid_preserved': bool(actual['exists'] and not actual['valid'])})
        assert before == upgrade.read_bytes() and source == digest()
        report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'services': records,
            'backend_source_sha256': source, 'original_upgrade_record_sha256': hashlib.sha256(before).hexdigest(),
            'schema_upgrade_rerun': False, 'live_process_terminated': False, 'external_integrations_started': False,
            'production_deployment': False, 'restoration_script_sha256': hashlib.sha256(__import__('pathlib').Path(__file__).read_bytes()).hexdigest()}
        target.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
        success = True
        print(json.dumps({'passed': True, 'owned_services_restored': len(records), 'schema_upgrade_rerun': False}))
    finally:
        if not success:
            for child in children:
                if child.poll() is None:
                    subprocess.run(['taskkill', '/PID', str(child.pid), '/T', '/F'], capture_output=True, creationflags=FLAGS, timeout=20)

if __name__ == '__main__':
    main()
