"""Derive selected test counts from preserved native and client execution records."""
import argparse,hashlib,json,re
from pathlib import Path
from run_batch_round163 import ROOT,PRIVATE,EVIDENCE,write_once
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def main():
 p=argparse.ArgumentParser();p.add_argument('--native',required=True);p.add_argument('--clients',required=True);a=p.parse_args()
 native=json.loads((EVIDENCE/('native-tests-'+a.native+'.json')).read_text(encoding='utf8'))
 clients=json.loads((EVIDENCE/('client-checks-'+a.clients+'.json')).read_text(encoding='utf8'))
 assert native['passed'] and clients['passed']
 logs={}
 def content(step):
  path=PRIVATE/(step+'-'+a.clients+'.log');row=next(v for v in clients['test_results'] if v['step']==step)
  assert sha(path)==row['private_log_sha256']
  logs[step]={'sha256':sha(path)}
  return re.sub(r'\x1b\[[0-9;]*m','',path.read_text(encoding='utf8',errors='replace'))
 web=[int(n) for n in re.findall(r'Tests\s+(\d+)\s+passed',content('web-tests'))]
 mobile=[int(n) for n in re.findall(r'^# tests\s+(\d+)',content('mobile-tests'),re.M)]
 extra=[int(n) for n in re.findall(r'^# tests\s+(\d+)',content('mobile-new-contracts'),re.M)]
 runtime=[int(n) for n in re.findall(r'^# tests\s+(\d+)',content('runtime-tests'),re.M)]
 assert len(web)==1 and len(mobile)>=2 and extra and len(runtime)==1
 tests=[r for r in native['test_results'] if 'Test' in r]
 assert tests and all(v['Action']=='pass' for v in tests)
 record={'passed':True,'native_record':'native-tests-'+a.native+'.json','client_record':'client-checks-'+a.clients+'.json','native_top_level_tests':sum('/' not in v['Test'] for v in tests),'native_test_entries':len(tests),'native_result_records':len(native['test_results']),'web_tests':web[0],'shared_mobile_tests':sum(mobile+extra),'shared_mobile_breakdown':mobile+extra,'runtime_tests':runtime[0],'draft_skills':8,'private_log_hashes':logs,'script_sha256':sha(Path(__file__)),'full_backend_regression':False,'full_client_regression':False,'production_ready':False}
 write_once(EVIDENCE/'test-summary.json',record);print(json.dumps(record))
 return 0
if __name__=='__main__':raise SystemExit(main())
