"""Lock a verified, committed whole-wave source before fresh native packaging."""
import argparse,hashlib,json,subprocess
from datetime import datetime,timezone
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
E=ROOT/'docs/quality/evidence/release-candidate-164'
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def git(*args):return subprocess.check_output(['git',*args],cwd=ROOT)
def main():
 p=argparse.ArgumentParser();p.add_argument('--native-record',required=True);p.add_argument('--client-record',required=True);a=p.parse_args()
 import verify_batch_round163 as business
 proof=business.verify();assert proof['passed']
 source=git('rev-parse','HEAD').decode().strip()
 files={'retained-functional-proof.json':'docs/quality/evidence/batch-round-163/final-source-check.json','retained-native-proof.json':a.native_record,'retained-client-proof.json':a.client_record,'retained-test-summary.json':'docs/quality/evidence/batch-round-163/test-summary.json'}
 E.mkdir(parents=True,exist_ok=True);assert not (E/'source-lock.json').exists()
 for target,name in files.items():
  raw=(ROOT/name).read_bytes();assert git('show',source+':'+name)==raw
  assert not (E/target).exists();(E/target).write_bytes(raw)
 prior=[]
 for n in range(1,9):
  tag='v0.1.0-rc.'+str(n)
  prior.append({'tag':tag,'source_commit':git('rev-parse',tag+'^{}').decode().strip(),'artifact_commit':git('rev-parse','codex/release-'+tag).decode().strip()})
 lock={'source_commit':source,'business_increment':163,'schema':129,'version':'v0.1.0-rc.9','created_at':datetime.now(timezone.utc).isoformat(),'prior_freezes':prior,'retained_files':files,'business_proof_sha256':sha(E/'retained-functional-proof.json'),'real_integrations_executed':False}
 (E/'source-lock.json').write_text(json.dumps(lock,indent=2)+'\n',encoding='utf8')
 print(json.dumps({'source_commit':source,'schema':129,'prior_freezes':len(prior)}))
 return 0
if __name__=='__main__':raise SystemExit(main())
