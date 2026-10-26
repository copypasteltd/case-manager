"""Restore already-stopped owned development services after a turn interruption.

Never kills or replaces a live process. The native PG cluster must already be
ready. No credentials, model/cloud integration or Telegram sender are activated.
"""
from datetime import datetime, timezone
import importlib.util
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import time
from urllib.parse import urlsplit
from urllib.request import urlopen

APP=Path(__file__).resolve().parents[1]
FLAGS=subprocess.CREATE_NO_WINDOW


def module(name,file):
    spec=importlib.util.spec_from_file_location(name,APP/'scripts'/file);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def listening(port):
    with socket.socket() as s:s.settimeout(1);return s.connect_ex(('127.0.0.1',port))==0
def ps(code):
    r=subprocess.run(['powershell.exe','-NoProfile','-NonInteractive','-Command',code],capture_output=True,text=True,timeout=20,creationflags=FLAGS)
    if r.returncode:raise RuntimeError('Native process inspection failed; diagnostics withheld.')
    return json.loads(r.stdout)


def main():
    if os.name!='nt':raise SystemExit('Native Windows development only.')
    config=json.loads((APP/'.local/environment.json').read_text(encoding='utf-8'));db=urlsplit(config['CP_DATABASE_URL'])
    if config.get('CP_ENV') not in ('development','local') or db.hostname!='127.0.0.1' or db.port!=14660 or config.get('CP_HTTP_ADDR')!='127.0.0.1:18080' or config.get('CP_STORAGE_DRIVER')!='disabled':raise SystemExit('Exact disabled-integration loopback development configuration required.')
    if not listening(14660):raise SystemExit('Owned PG is not ready; inspect its real process and recovery state first.')
    if listening(18080) or listening(5178):raise SystemExit('A service is already listening. Nothing was started or stopped; inspect its owner.')
    for name in ('project-templates-api-owner.json','telegram-delivery-worker-owner.json','project-templates-worker-owner.json'):
        file=APP/'.local'/name
        if not file.exists():continue
        record=json.loads(file.read_text());pid=int(record['pid'])
        actual=ps("$p=Get-CimInstance Win32_Process -Filter 'ProcessId = "+str(pid)+"';if($p){@{exists=$true;created_at=$p.CreationDate.ToUniversalTime().ToString('o')}|ConvertTo-Json -Compress}else{'{\"exists\":false}'}")
        if actual['exists'] and abs((datetime.fromisoformat(actual['created_at'])-datetime.fromisoformat(record['started_at'])).total_seconds())<10:raise SystemExit('Recorded service is still live; no duplicate will be started.')
    started=[]
    for command in ('api','worker-notification'):
        log=APP/'.local'/('project-templates-restored-'+command+'.log')
        with log.open('w',encoding='utf-8') as output:
            child=subprocess.Popen([sys.executable,'app/scripts/run_backend.py',command],cwd=APP.parent,stdout=output,stderr=subprocess.STDOUT,creationflags=FLAGS)
        observed=ps("$p=Get-CimInstance Win32_Process -Filter 'ProcessId = "+str(child.pid)+"';@{created_at=$p.CreationDate.ToUniversalTime().ToString('o')}|ConvertTo-Json -Compress")
        owner=APP/'.local'/('project-templates-api-owner.json' if command=='api' else 'project-templates-worker-owner.json')
        owner.write_text(json.dumps({'pid':child.pid,'command':command,'started_at':observed['created_at']},indent=2)+'\n',encoding='utf-8');started.append((command,child))
    node=shutil.which('node')
    if not node:raise SystemExit('Native Node is required.')
    with (APP/'.local/project-templates-restored-vite.log').open('w',encoding='utf-8') as output:
        vite=subprocess.Popen([node,str(APP/'node_modules/vite/bin/vite.js'),'--host','127.0.0.1','--port','5178','--strictPort'],cwd=APP/'web',env={**os.environ,'CP_DEV_API_ORIGIN':'http://127.0.0.1:18080'},stdout=output,stderr=subprocess.STDOUT,creationflags=FLAGS)
    started.append(('vite',vite));ready=False;deadline=time.monotonic()+150
    while time.monotonic()<deadline:
        if any(p.poll() is not None for _,p in started):break
        try:
            with urlopen('http://127.0.0.1:18080/health/ready',timeout=3) as r:api_ready=r.status==200
            with urlopen('http://localhost:5178/health/ready',timeout=3) as r:web_ready=r.status==200
            ready=api_ready and web_ready
        except OSError:pass
        if ready:break
        time.sleep(0.25)
    source=module('restore_template_source','verify_backend_batches.py').digest()
    report={'passed':ready,'checked_at':datetime.now(timezone.utc).isoformat(),'source_sha256':source,'restored_after_confirmed_process_exit':True,'live_process_terminated':False,'new_processes':[{'kind':kind,'pid':p.pid,'live':p.poll() is None}for kind,p in started],'pg_cluster_reinitialized':False,'external_integration_started':False,'production_deployed':False}
    output=APP.parent/'docs/quality/evidence/project-templates/development-restoration.json';output.write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':ready,'restored_services':len(started),'live_process_terminated':False,'external_integration_started':False}))
    return 0 if ready else 1


if __name__=='__main__':raise SystemExit(main())
