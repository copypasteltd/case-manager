"""Apply additive77 and reload only verified owned native API/in-app/export workers."""
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import time

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/file-imports'


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / filename)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def main():
    owned = module('mention_process_owner', 'verify_notifications_sources.py').owned_process
    workers = [('project-templates-worker-owner.json', 'worker-notification', 'project-templates-worker.log', 'notification worker started'),
        ('finance-reports-worker-owner.json', 'worker-report-export', 'finance-reports-worker.log', 'report export worker started')]
    for owner, command, _, _ in workers:
        assert owned(owner, command), 'Exact existing worker ownership is required; nothing was stopped.'
    update = module('mention_api_update', 'update_project_templates_development.py')
    update.EVIDENCE = EVIDENCE
    if update.main():
        return 1
    backend = module('mention_backend_bytes', 'verify_backend_batches.py')
    source = backend.digest()
    records = []
    for owner, command, logfile, marker in workers:
        path = APP / '.local' / owner
        previous = json.loads(path.read_text(encoding='utf-8'))
        replaced = previous.get('backend_source_sha256') != source
        if replaced:
            assert owned(owner, command), 'Recorded worker identity changed; nothing was stopped.'
            status = subprocess.run(['taskkill', '/PID', str(previous['pid']), '/T', '/F'], capture_output=True, creationflags=subprocess.CREATE_NO_WINDOW)
            if status.returncode:
                raise SystemExit('The exact owned worker could not be stopped.')
            logpath = APP / '.local' / logfile
            with logpath.open('w', encoding='utf-8') as log:
                child = subprocess.Popen([sys.executable, 'app/scripts/run_backend.py', command], cwd=APP.parent,
                    stdout=log, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW)
            actual = update.ps("$p=Get-CimInstance Win32_Process -Filter 'ProcessId = " + str(child.pid) + "';@{created_at=$p.CreationDate.ToUniversalTime().ToString('o')}|ConvertTo-Json -Compress")
            path.write_text(json.dumps({'pid': child.pid, 'command': command, 'started_at': actual['created_at'], 'backend_source_sha256': source}, indent=2) + '\n', encoding='utf-8')
            deadline = time.monotonic() + 150
            while time.monotonic() < deadline:
                if child.poll() is not None:
                    raise SystemExit('Owned worker exited during startup; private diagnostics remain ignored.')
                if marker in logpath.read_text(encoding='utf-8'):
                    break
                time.sleep(.25)
            else:
                raise SystemExit('Owned worker startup could not be confirmed.')
        records.append({'command': command, 'replaced_verified_owner': replaced, 'owned': owned(owner, command), 'backend_source_sha256': source,
            'previous_backend_source_sha256': previous.get('backend_source_sha256')})
    report = EVIDENCE / 'development-update.json'
    result = json.loads(report.read_text())
    result.update({'workers': records, 'notification_worker_restarted': records[0]['replaced_verified_owner'], 'migration': 77,
        'source_unchanged': source == backend.digest(), 'updater_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'external_integration_started': False, 'production_deployment': False})
    result['passed'] = result['passed'] and result['source_unchanged'] and all(r['owned'] for r in records)
    report.write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'migration': 77, 'workers_owned': all(r['owned'] for r in records), 'external_integration_started': False}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
