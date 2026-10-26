"""Compile the fixed backend for Linux; never execute or deploy that artifact."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys

APP=Path(__file__).resolve().parents[1]
if len(sys.argv)!=2 or not sys.argv[1].replace('-','').isalnum():
    raise SystemExit('Usage: python app/scripts/verify_linux.py <evidence-group>')
group=sys.argv[1]
spec=importlib.util.spec_from_file_location('backend_manifest',APP/'scripts/verify_backend_batches.py')
module=importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
before=module.digest()
artifact=APP/'.local'/f'{group}-platform-linux-amd64'
log=APP/'.local'/f'{group}-linux-build.log'
started=datetime.now(timezone.utc).isoformat()
environment={**os.environ,'GOOS':'linux','GOARCH':'amd64','CGO_ENABLED':'0','GOFLAGS':'-p=1','GOMAXPROCS':'2'}
with log.open('w',encoding='utf-8') as output:
    result=subprocess.run([str(APP/'.tools/go/bin/go.exe'),'build','-o',str(artifact),'./cmd/platform'],cwd=APP/'backend',env=environment,stdout=output,stderr=subprocess.STDOUT,
                          creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0)
after=module.digest()
report={'passed':result.returncode==0 and before==after and artifact.is_file(),'started_at':started,'finished_at':datetime.now(timezone.utc).isoformat(),
        'source_sha256':before,'source_after_sha256':after,'exit_code':result.returncode,'target':'linux/amd64','cgo_enabled':False,
        'artifact_sha256':hashlib.sha256(artifact.read_bytes()).hexdigest() if artifact.is_file() else None,
        'artifact_executed':False,'deployed':False}
target=APP.parent/'docs/quality/evidence'/group/'linux-build.json'
target.parent.mkdir(parents=True,exist_ok=True)
target.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'passed':report['passed'],'target':report['target'],'artifact_executed':False}))
raise SystemExit(0 if report['passed'] else 1)
