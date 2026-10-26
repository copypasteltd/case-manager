"""Recompute this increment's evidence against current source and local cleanup."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import sys
from urllib.request import urlopen

APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent
GROUP=sys.argv[1] if __name__=='__main__' and len(sys.argv)==2 else 'secretary-plans'
if __name__=='__main__' and (len(sys.argv)>2 or GROUP not in ('secretary-plans','secretary-discovery')):
    raise SystemExit('Usage: python app/scripts/verify_secretary_plans_sources.py [secretary-plans|secretary-discovery]')
EVIDENCE=ROOT/'docs/quality/evidence'/GROUP

def read(path):
    return json.loads(path.read_text(encoding='utf-8'))

def module(name,file):
    spec=importlib.util.spec_from_file_location(name,APP/'scripts'/file)
    value=importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value

def hash_files(paths,base,separator=b'\0',trailing=True):
    h=hashlib.sha256()
    for p in paths:
        h.update(p.relative_to(base).as_posix().encode()+separator+p.read_bytes())
        if trailing:
            h.update(b'\0')
    return h.hexdigest()

def main():
    backend=module('plans_backend_manifest','verify_backend_batches.py').digest()
    runtime=module('plans_runtime_manifest','verify_agent_runtime.py').digest()[0]
    mobile=module('plans_mobile_manifest','verify_mobile.py').digest()[0]
    web_paths=sorted(set([APP/'scripts/verify_web.py',APP/'package.json',APP/'package-lock.json']+[
        p for folder in ('web','packages/design','packages/contracts') for p in (APP/folder).rglob('*')
        if p.is_file() and not any(part in ('node_modules','dist','.taro','.swc') for part in p.relative_to(APP).parts)
        and p.suffix in ('.ts','.tsx','.css','.json','.html')]),key=lambda p:p.relative_to(APP).as_posix())
    web=hash_files(web_paths,APP)
    checks=[]
    for file,expected,success_key in [('backend-result.json',backend,'passed'),('linux-build.json',backend,'passed'),('runtime-build.json',runtime,'status'),('mobile-build.json',mobile,'passed'),('web-build.json',web,'passed')]:
        report=read(EVIDENCE/file)
        success=report[success_key]=='passed' if success_key=='status' else report[success_key] is True
        checks.append({'name':file,'passed':success and report['source_sha256']==expected,'source_sha256':expected})
    for group,separator,trailing in [(GROUP+'-web',b'\n',False),(GROUP+'-mobile',b'\0',True)]:
        report=read(ROOT/'docs/quality/evidence'/group/'result.json')
        paths=[APP/name for name in report['source_files']]
        current=hash_files(paths,APP,separator,trailing)
        closed=all(report.get(key) is True for key in ('owned_contexts_closed','owned_browser_closed','isolated_database_cleanup_passed'))
        checks.append({'name':group,'passed':report['passed'] is True and closed and current==report['source_sha256'],'source_sha256':current,'checks':len(report['checks'])})
    development=read(EVIDENCE/'development-plans.json')
    checks.append({'name':'development plans and unauthorized route probes','passed':development['passed'] is True and development['database']['disposable_test_databases']==0})
    checks.append({'name':'OpenAPI embedded and documented copies','passed':(APP/'backend/internal/contract/openapi.json').read_bytes()==(ROOT/'docs/development/openapi.json').read_bytes()})
    checks.append({'name':'Private browser fixture removed','passed':not (APP/'.local/business-browser-fixture.json').exists()})
    with socket.socket() as probe:
        probe.settimeout(1)
        checks.append({'name':'Owned temporary browser review port released','passed':probe.connect_ex(('127.0.0.1',5189))!=0})
    for url in ('http://localhost:5178/','http://localhost:5178/health/ready','http://127.0.0.1:18080/health/ready'):
        try:
            with urlopen(url,timeout=5) as response:
                passed=response.status==200
        except OSError:
            passed=False
        checks.append({'name':url,'passed':passed})
    report={'passed':all(c['passed'] for c in checks),'checked_at':datetime.now(timezone.utc).isoformat(),'checks':checks,
            'scope':'Fixed source verification, local readiness and owned cleanup; no production, real model, cloud, device or capacity acceptance.'}
    (EVIDENCE/'final-source-check.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':report['passed'],'checks':len(checks),'failed':[c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1

if __name__=='__main__':
    raise SystemExit(main())
