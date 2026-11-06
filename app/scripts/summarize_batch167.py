"""Summarize selected current executions; retain every original failed attempt."""
import json,re
from run_batch_round167 import EVIDENCE,PRIVATE,write_once
inputs=json.loads((EVIDENCE/'verification-inputs.json').read_text('utf-8'))
n=json.loads((EVIDENCE/('native-tests-'+inputs['native']+'.json')).read_text('utf-8'))
steps={}
for step,attempt in inputs['clients'].items():
    record=json.loads((EVIDENCE/('client-checks-'+attempt+'.json')).read_text('utf-8'))
    steps[step]=next(row for row in record['test_results'] if row['step']==step)
def log(step):return re.sub(r'\x1b\[[0-9;]*m','',(PRIVATE/(step+'-'+inputs['clients'][step]+'.log')).read_text('utf-8',errors='replace'))
web=re.findall(r'Tests\s+(\d+) passed',log('web-tests'))
mobile=[int(v) for step in ('mobile-tests','mobile-new-contracts') for v in re.findall(r'# tests (\d+)',log(step))]
result={'passed':n['passed'] and all(v['passed'] for v in steps.values()) and len(web)==1 and len(mobile)>=2,
 'native_record':'native-tests-'+inputs['native']+'.json','client_steps':steps,
 'native_result_records':sum(v['Action']=='pass' for v in n['test_results']),
 'native_top_level_tests':sum(1 for v in n['test_results'] if v['Action']=='pass' and v.get('Test') and '/' not in v['Test']),
 'web_tests':int(web[0]) if web else 0,'shared_mobile_tests':sum(mobile),'shared_mobile_parts':mobile,
 'runtime_tests_retained':66,'runtime_reexecuted':False,'formal_acceptance':False}
write_once(EVIDENCE/'test-summary.json',result)
print(json.dumps({k:v for k,v in result.items() if k!='client_steps'}))
