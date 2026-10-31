"""Execute bounded client checks; preserve logs privately and exact source hashes."""
from datetime import datetime, timezone
import hashlib,json,os,re,shutil,subprocess,sys,time
from foreign_expense_refunds_sources import APP,ROOT,CLIENT_SCOPES,sources,digest
E=ROOT/'docs/quality/evidence/foreign-expense-refunds'
COMMANDS=[('typecheck',['run','typecheck']),('web-tests',['run','test','--workspace','@copypaste/web','--','--reporter=json']),
 ('mobile-tests',['run','test','--workspace','@copypaste/mobile']),('web-build',['run','build:web']),
 ('weapp-build',['run','build:weapp']),('douyin-build',['run','build:tt'])]
def main():
 E.mkdir(parents=True,exist_ok=True); local=APP/'.local/foreign-expense-refunds';local.mkdir(parents=True,exist_ok=True)
 npm=shutil.which('npm.cmd' if os.name=='nt' else 'npm');assert npm,'Native Node22 required'
 env={**os.environ,'CI':'true','TARO_CHECK_UPDATE':'false','CP_MOBILE_BUILD_PROFILE':'local','CP_MOBILE_API_ORIGIN':'http://127.0.0.1:18080'}
 selected=set(sys.argv[1:]);failed=False
 for name,args in COMMANDS:
  if selected and name not in selected: continue
  target=E/(name+'.json');assert not target.exists(),'Archive an existing attempt before rerunning: '+name
  names=sources(CLIENT_SCOPES);before=digest(names);start=datetime.now(timezone.utc).isoformat();t=time.monotonic();log=local/(name+'.log')
  with log.open('wb') as out: p=subprocess.run([npm,*args],cwd=APP,env=env,stdout=out,stderr=subprocess.STDOUT,creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0)
  after=digest(names);text=log.read_text(encoding='utf-8',errors='replace');tests={}
  if name=='web-tests':
   for line in text.splitlines():
    try:
     x=json.loads(line)
     if 'numPassedTests' in x: tests={k:x[k] for k in ('numPassedTests','numFailedTests','numPendingTests')}
    except ValueError: pass
  if name=='web-tests' and not tests:
   x=json.loads((APP/'web/.vitest/json/output.json').read_text(encoding='utf-8')); tests={k:x[k] for k in ('numPassedTests','numFailedTests','numPendingTests')}
  if name=='mobile-tests':
   for key,pattern in [('pass',r'# pass (\d+)'),('fail',r'# fail (\d+)'),('skip',r'# skipped (\d+)')]: tests[key]=sum(map(int,re.findall(pattern,text)))
  passed=p.returncode==0 and before==after
  record={'passed':passed,'started_at':start,'elapsed_seconds':round(time.monotonic()-t,3),'command':['npm',*args],'exit_code':p.returncode,'source_files':names,'source_sha256':before,'source_after_sha256':after,'source_unchanged':before==after,'tests':tests,'private_log_sha256':hashlib.sha256(log.read_bytes()).hexdigest(),'build_profile':'local','android_apk_built':False,'virtualization_used':False,'production_ready':False}
  target.write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8');print(json.dumps({k:record[k] for k in ('passed','command','elapsed_seconds','exit_code','tests')}),flush=True)
  failed|=not passed
  if failed:break
 return 1 if failed else 0
if __name__=='__main__':raise SystemExit(main())
