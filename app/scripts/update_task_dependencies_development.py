"""Apply additive migration74 and reload only exact owned native loopback services."""
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import time

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/task-dependencies'


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / filename)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


def main():
    owner = APP / '.local/finance-reports-worker-owner.json'
    owned = module('dependency_process_owner', 'verify_notifications_sources.py').owned_process
    assert owner.exists() and owned(owner.name, 'worker-report-export'), 'Exact existing report worker ownership must be established before any update.'
    previous = json.loads(owner.read_text(encoding='utf-8'))
    update = module('dependency_api_update', 'update_project_templates_development.py')
    update.EVIDENCE = EVIDENCE
    if update.main():
        return 1
    source = module('dependency_backend_source', 'verify_backend_batches.py').digest()
    replaced = previous.get('backend_source_sha256') != source
    if replaced:
        assert owned(owner.name, 'worker-report-export'), 'Recorded worker changed; nothing was stopped.'
        result = subprocess.run(['taskkill', '/PID', str(previous['pid']), '/T', '/F'], capture_output=True, creationflags=subprocess.CREATE_NO_WINDOW)
        if result.returncode:
            raise SystemExit('Could not stop the exact owned export worker.')
        with (APP / '.local/finance-reports-worker.log').open('w', encoding='utf-8') as log:
            child = subprocess.Popen([sys.executable, 'app/scripts/run_backend.py', 'worker-report-export'], cwd=APP.parent,
                stdout=log, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW)
        actual = update.ps("$p=Get-CimInstance Win32_Process -Filter 'ProcessId = " + str(child.pid) + "';@{created_at=$p.CreationDate.ToUniversalTime().ToString('o')}|ConvertTo-Json -Compress")
        owner.write_text(json.dumps({'pid': child.pid, 'command': 'worker-report-export', 'started_at': actual['created_at'], 'backend_source_sha256': source}, indent=2) + '\n', encoding='utf-8')
        deadline = time.monotonic() + 150
        while time.monotonic() < deadline:
            if child.poll() is not None:
                raise SystemExit('Owned report worker exited; private diagnostics remain ignored.')
            if 'report export worker started' in (APP / '.local/finance-reports-worker.log').read_text(encoding='utf-8'):
                break
            time.sleep(.25)
        else:
            raise SystemExit('Owned report worker startup was not confirmed.')
    report = EVIDENCE / 'development-update.json'
    value = json.loads(report.read_text(encoding='utf-8'))
    value.update({'report_worker_replaced_verified_owner': replaced, 'report_worker_owned': owned(owner.name, 'worker-report-export'),
        'report_worker_source_sha256': source, 'previous_report_worker_source_sha256': previous.get('backend_source_sha256'),
        'external_integration_started': False, 'updater_sha256': __import__('hashlib').sha256(Path(__file__).read_bytes()).hexdigest()})
    value['passed'] = value['passed'] and value['report_worker_owned'] and source == module('dependency_backend_after', 'verify_backend_batches.py').digest()
    report.write_text(json.dumps(value, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': value['passed'], 'migration': 74, 'report_worker_replaced': replaced, 'external_integration_started': False}))
    return 0 if value['passed'] else 1


if __name__ == '__main__':
    main()
