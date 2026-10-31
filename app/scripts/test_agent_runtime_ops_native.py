"""Focused native HTTP/PG proof. No Docker, WSL, emulator or real cloud writes."""
from datetime import datetime, timezone
import hashlib, json, os, subprocess, sys, time
from pathlib import Path
import contract_body_native_cluster as cluster

APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent
E=ROOT/'docs/quality/evidence/agent-runtime-operations'
cluster.OWNER=APP/'.local/agent-runtime-operations-native-cluster.json'
cluster.E=E

def names():
 return sorted(n for n in subprocess.check_output(['git','ls-files','-z','--cached','--others','--exclude-standard','--','backend','scripts/build_openapi.py','scripts/test_agent_runtime_ops_native.py','scripts/contract_body_native_cluster.py'],cwd=APP).decode('utf-8').split('\0') if n and (APP/n).is_file())

def digest(files):
 h=hashlib.sha256()
 for n in files:h.update(n.encode()+b'\0'+(APP/n).read_bytes()+b'\0')
 return h.hexdigest()

def main():
 attempt=sys.argv[1] if len(sys.argv)>1 else 'focused'
 regex=sys.argv[2] if len(sys.argv)>2 else '^(TestAgentRuntimeOperations|TestAgentExecutionController)$'
 assert attempt.replace('-','').isalnum()
 E.mkdir(parents=True,exist_ok=True)
 local=APP/'.local/agent-runtime-operations';local.mkdir(parents=True,exist_ok=True)
 report=E/('native-'+attempt+'.json');assert not report.exists(),'Preserve previous attempt.'
 log=local/('native-'+attempt+'.jsonl');assert not log.exists()
 files=names();before=digest(files);start=datetime.now(timezone.utc).isoformat();clock=time.monotonic()
 command=['go','test','./internal/httpapi','-run',regex,'-p=1','-timeout=12m','-count=1','-json']
 code=None
 try:
  with log.open('wb') as out:
   code=subprocess.run([str(APP/'.tools/go/bin/go.exe'),*command[1:]],cwd=APP/'backend',env=cluster.test_environment(),stdout=out,stderr=subprocess.STDOUT,creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0).returncode
 finally:
  if cluster.OWNER.exists():
   cluster.stop();closed_id=hashlib.sha256(cluster.OWNER.read_bytes()).hexdigest()[:16];cluster.OWNER.rename(cluster.OWNER.with_name('agent-runtime-operations-closed-'+attempt+'-'+closed_id+'.json'))
 events=[]
 for line in log.read_text(encoding='utf-8',errors='replace').splitlines():
  try:
   v=json.loads(line)
   if v.get('Action') in ('pass','fail','skip') and v.get('Test'):events.append({k:v[k] for k in ('Action','Test','Elapsed') if k in v})
  except ValueError:pass
 after=digest(files)
 record={'passed':code==0 and before==after,'started_at':start,'elapsed_seconds':round(time.monotonic()-clock,3),'command':command,'exit_code':code,'source_files':files,'source_sha256':before,'source_after_sha256':after,'source_unchanged':before==after,'test_results':events,'private_log_sha256':hashlib.sha256(log.read_bytes()).hexdigest(),'native_pg_durable':True,'provider_transport':'no provider or model invoked','retention_and_observation_clock':'isolated synthetic fixture only','virtualization_used':False,'production_ready':False}
 report.write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8')
 print(json.dumps({k:record[k] for k in ('passed','exit_code','elapsed_seconds','test_results')}))
 return 0 if record['passed'] else 1

if __name__=='__main__':raise SystemExit(main())
