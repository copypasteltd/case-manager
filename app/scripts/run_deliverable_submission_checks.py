"""Selected source-bound checks. Full regression and fresh Android are deferred."""
from datetime import datetime,timezone
import hashlib,json,os,subprocess,sys,time
from deliverable_submission_sources import APP,ROOT,CLIENT_SCOPES,sources,digest
E=ROOT/'docs/quality/evidence/deliverable-submission'
NODE=APP/'.tools/node-v22.23.4-win-x64/node.exe'
NPM=NODE.parent/'node_modules/npm/bin/npm-cli.js'
COMMANDS=[('web-deliverable-submission-tests',APP/'web',[NODE,APP/'node_modules/vitest/vitest.mjs','run','src/deliverable-submission.test.ts']),('mobile-deliverable-submission-compile',APP/'mobile',[NODE,APP/'node_modules/typescript/bin/tsc','--project','tsconfig.concern-test.json']),('mobile-deliverable-submission-tests',APP,[NODE,'--test','.local/mobile-concern-tests/mobile/src/platform/deliverable-submission.test.js']),('runtime-tests',APP,[NODE,NPM,'run','test','--workspace','@copypaste/agent-runtime']),('typecheck',APP,[NODE,NPM,'run','typecheck']),('web-build',APP,[NODE,NPM,'run','build:web']),
 ('weapp-build',APP,[NODE,NPM,'run','build:weapp']),('douyin-build',APP,[NODE,NPM,'run','build:tt'])]
def main():
 E.mkdir(parents=True,exist_ok=True);local=APP/'.local/deliverable-submission';local.mkdir(parents=True,exist_ok=True)
 selected=set(sys.argv[1:]);env={**os.environ,'PATH':str(NODE.parent)+os.pathsep+os.environ['PATH'],'CI':'true','TARO_CHECK_UPDATE':'false','CP_MOBILE_BUILD_PROFILE':'local','CP_MOBILE_API_ORIGIN':'http://127.0.0.1:18080'}
 for name,cwd,args in COMMANDS:
  if selected and name not in selected:continue
  index=1
  while (E/(name+'-'+str(index)+'.json')).exists():index+=1
  report=E/(name+'-'+str(index)+'.json');log=local/(name+'-'+str(index)+'.log')
  files=sources(CLIENT_SCOPES);before=digest(files);start=datetime.now(timezone.utc).isoformat();clock=time.monotonic()
  with log.open('wb') as out:p=subprocess.run(list(map(str,args)),cwd=cwd,env=env,stdout=out,stderr=subprocess.STDOUT,creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0)
  after=digest(files)
  record={'passed':p.returncode==0 and before==after,'started_at':start,'elapsed_seconds':round(time.monotonic()-clock,3),'step':name,'exit_code':p.returncode,'source_files':files,'source_sha256':before,'source_after_sha256':after,'source_unchanged':before==after,'runner_sha256':hashlib.sha256(__import__('pathlib').Path(__file__).read_bytes()).hexdigest(),'private_log_sha256':hashlib.sha256(log.read_bytes()).hexdigest(),'node_version':subprocess.check_output([str(NODE),'--version'],text=True).strip(),'build_profile':'local','new_android_apk':False,'full_regression':False,'virtualization_used':False,'production_ready':False}
  report.write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8');print(json.dumps({k:record[k] for k in ('passed','step','exit_code','elapsed_seconds')}),flush=True)
  if not record['passed']:return 1
 return 0
if __name__=='__main__':raise SystemExit(main())
