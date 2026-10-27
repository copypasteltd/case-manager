"""Migrate and reload the recorded loopback API; start one owned DB-only export worker."""
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import time

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/finance-reports'


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / filename)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def main():
    update = module('report_api_update', 'update_project_templates_development.py')
    update.EVIDENCE = EVIDENCE
    if update.main():
        return 1
    source = module('report_backend_source', 'verify_backend_batches.py').digest()
    owner = APP / '.local/finance-reports-worker-owner.json'
    owned = module('report_owner_check', 'verify_notifications_sources.py').owned_process
    if owner.exists():
        record = json.loads(owner.read_text(encoding='utf-8'))
        if not owned(owner.name, 'worker-report-export') or record.get('backend_source_sha256') != source:
            raise SystemExit('Existing report worker needs explicit ownership/source reconciliation; no process was stopped.')
        started = False
    else:
        log = APP / '.local/finance-reports-worker.log'
        with log.open('w', encoding='utf-8') as output:
            child = subprocess.Popen([sys.executable, 'app/scripts/run_backend.py', 'worker-report-export'], cwd=APP.parent,
                stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW)
        actual = update.ps("$p=Get-CimInstance Win32_Process -Filter 'ProcessId = " + str(child.pid) + "';@{created_at=$p.CreationDate.ToUniversalTime().ToString('o')}|ConvertTo-Json -Compress")
        owner.write_text(json.dumps({'pid': child.pid, 'command': 'worker-report-export', 'started_at': actual['created_at'], 'backend_source_sha256': source}, indent=2) + '\n', encoding='utf-8')
        deadline = time.monotonic() + 150
        while time.monotonic() < deadline:
            if child.poll() is not None:
                raise SystemExit('Owned report worker stopped; private diagnostics remain ignored.')
            if 'report export worker started' in log.read_text(encoding='utf-8'):
                break
            time.sleep(.25)
        else:
            raise SystemExit('Owned report worker startup was not confirmed.')
        started = True
    report_path = EVIDENCE / 'development-update.json'
    report = json.loads(report_path.read_text(encoding='utf-8'))
    report.update({'report_worker_started': started, 'report_worker_owned': owned(owner.name, 'worker-report-export'),
        'report_worker_source_sha256': source, 'external_integration_started': False})
    report['passed'] = report['passed'] and report['report_worker_owned']
    report_path.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'report_worker_started': started, 'external_integration_started': False}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
