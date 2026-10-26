"""Update only the recorded native loopback development API and its schema.

No PostgreSQL restart, fixture seed, external integration, model, cloud or worker
activation. The existing notification worker and Vite process remain running.
"""
from datetime import datetime, timezone
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import time
from urllib.parse import urlsplit
from urllib.request import urlopen

APP=Path(__file__).resolve().parents[1]
EVIDENCE=APP.parent/'docs/quality/evidence/project-templates'
FLAGS=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0


def ps(code):
    result=subprocess.run(['powershell.exe','-NoProfile','-NonInteractive','-Command',code],capture_output=True,text=True,timeout=20,creationflags=FLAGS)
    if result.returncode: raise RuntimeError('Owned native process inspection failed; raw output withheld.')
    return json.loads(result.stdout)


def main():
    if os.name!='nt':raise SystemExit('This updater is for the owned Windows development environment only.')
    values=json.loads((APP/'.local/environment.json').read_text(encoding='utf-8'))
    db=urlsplit(values['CP_DATABASE_URL'])
    migration=urlsplit(values['CP_MIGRATION_DATABASE_URL'])
    if values.get('CP_ENV') not in ('local','development') or db.hostname!='127.0.0.1' or db.port!=14660 or migration.hostname!='127.0.0.1' or migration.port!=14660 or values.get('CP_HTTP_ADDR')!='127.0.0.1:18080':
        raise SystemExit('Refusing any environment other than the exact owned loopback development services.')
    if values.get('CP_STORAGE_DRIVER')!='disabled' or values.get('CP_AGENT_RUNTIME_ENABLED','false') not in ('false','disabled'):
        raise SystemExit('Development integrations must stay explicitly disabled.')
    previous_path=APP/'.local/telegram-delivery-api-owner.json'
    current_path=APP/'.local/project-templates-api-owner.json'
    record=json.loads((current_path if current_path.exists() else previous_path).read_text(encoding='utf-8'))
    pid=int(record['pid'])
    process=ps("$p=Get-CimInstance Win32_Process -Filter 'ProcessId = "+str(pid)+"'; if($p -and $p.Name -eq 'python.exe' -and $p.CommandLine -like '*app/scripts/run_backend.py api*') { @{valid=$true;created_at=$p.CreationDate.ToUniversalTime().ToString('o')} | ConvertTo-Json -Compress } else { '{\"valid\":false}' }")
    if not process['valid'] or abs((datetime.fromisoformat(process['created_at'])-datetime.fromisoformat(record['started_at'])).total_seconds())>=10:
        raise SystemExit('Recorded API process identity is no longer exact; nothing was stopped.')
    spec=importlib.util.spec_from_file_location('template_backend',APP/'scripts/verify_backend_batches.py');backend=importlib.util.module_from_spec(spec);spec.loader.exec_module(backend)
    before=backend.digest();EVIDENCE.mkdir(parents=True,exist_ok=True)
    # Migrations are additive. Apply while old API still runs, then replace only
    # its verified process tree; startup independently verifies the manifest.
    log=APP/'.local/project-templates-development-migration.log'
    with log.open('w',encoding='utf-8') as output:
        status=subprocess.call([sys.executable,'app/scripts/run_backend.py','migrate'],cwd=APP.parent,stdout=output,stderr=subprocess.STDOUT,creationflags=FLAGS)
    if status:raise SystemExit('Development migration failed. Existing API was retained; raw diagnostics withheld.')
    if backend.digest()!=before:raise SystemExit('Backend source changed during migration; existing process retained.')
    # Revalidate PID and creation time immediately before terminating its tree.
    verify=ps("$p=Get-CimInstance Win32_Process -Filter 'ProcessId = "+str(pid)+"'; if($p){@{created_at=$p.CreationDate.ToUniversalTime().ToString('o')}|ConvertTo-Json -Compress}else{'{}'}")
    if verify.get('created_at')!=process['created_at']:raise SystemExit('Recorded process changed; refusing to stop it.')
    result=subprocess.run(['taskkill','/PID',str(pid),'/T','/F'],capture_output=True,creationflags=FLAGS)
    if result.returncode:raise SystemExit('Could not stop the exact owned API process tree.')
    with (APP/'.local/project-templates-api.log').open('w',encoding='utf-8') as output:
        child=subprocess.Popen([sys.executable,'app/scripts/run_backend.py','api'],cwd=APP.parent,stdout=output,stderr=subprocess.STDOUT,creationflags=FLAGS)
    observed=ps("$p=Get-CimInstance Win32_Process -Filter 'ProcessId = "+str(child.pid)+"';@{created_at=$p.CreationDate.ToUniversalTime().ToString('o')}|ConvertTo-Json -Compress")
    current_path.write_text(json.dumps({'pid':child.pid,'command':'api','started_at':observed['created_at']},indent=2)+'\n',encoding='utf-8')
    ready=False;deadline=time.monotonic()+150
    while time.monotonic()<deadline:
        if child.poll() is not None:break
        try:
            with urlopen('http://127.0.0.1:18080/health/ready',timeout=3) as response:ready=response.status==200
        except OSError:pass
        if ready:break
        time.sleep(0.25)
    report={'passed':ready and before==backend.digest(),'checked_at':datetime.now(timezone.utc).isoformat(),'source_sha256':before,
        'migration_exit_code':status,'replaced_verified_api_pid':pid,'api_pid':child.pid,'api_ready':ready,
        'postgres_restarted':False,'notification_worker_restarted':False,'vite_restarted':False,
        'external_integration_started':False,'production_deployment':False}
    (EVIDENCE/'development-update.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':report['passed'],'api_ready':ready,'postgres_restarted':False,'external_integration_started':False}))
    return 0 if report['passed'] else 1


if __name__=='__main__':raise SystemExit(main())
