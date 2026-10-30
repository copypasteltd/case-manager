"""Reuse only completed current-source cases; split remaining cases without extending deadlines."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
from verify_backend_batches import APP, digest

ROOT=APP.parent
E=ROOT/'docs/quality/evidence/refund-fx-backend'
P=ROOT/'docs/quality/evidence/refund-fx-attempts/14-backend-package-deadline'
PRIVATE=APP/'.local/refund-fx-attempts/14-backend-package-deadline'
assert not P.exists() and not PRIVATE.exists()
state=json.loads((E/'batch-progress.json').read_text())
assert state['source_sha256']==digest() and state['batches'][0]['status']=='passed' and state['batches'][1]['status']=='failed'
assert all(b['status']=='pending' for b in state['batches'][2:])
P.mkdir(parents=True);PRIVATE.mkdir(parents=True)
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
records=[]
for source,target in [(E/'batch-progress.json',P/'batch-progress.json'),(E/'result.json',P/'result.json'),(APP/'.local/backend-batch-2.log',PRIVATE/'backend-batch-2.log')]:
    if source.exists():
        shutil.copyfile(source,target)
        records.append({'file':target.relative_to(ROOT).as_posix(),'sha256':sha(target)})
events=[]
for line in (PRIVATE/'backend-batch-2.log').read_text(encoding='utf-8').splitlines():
    try:value=json.loads(line)
    except ValueError:continue
    if value.get('Test') and value.get('Action') in ('pass','fail','skip'):
        events.append({k:value[k] for k in ('Action','Package','Test','Elapsed') if k in value})
passed={v['Test'] for v in events if '/' not in v['Test'] and v['Action']=='pass'}
assert len(passed)==19 and passed <= set(state['batches'][1]['names'])
selected=[v for v in events if v['Test'].split('/')[0] in passed]
assert selected and all(v['Action']=='pass' for v in selected)
partial={'names':sorted(passed),'status':'passed','checks':selected,'source_unchanged_during_tests':True,
    'source_sha256':digest(),'completed_at':datetime.now(timezone.utc).isoformat(),
    'execution_scope':'Nineteen completed current-source top-level cases and their passed children, selected from an interrupted package. Original failed package and subcase remain archived; no package pass claimed.',
    'selected_from_failed_package':True,'original_raw_log':(PRIVATE/'backend-batch-2.log').relative_to(APP).as_posix()}
done=set(state['batches'][0]['names'])|passed
remaining=sorted({n for _,n in state['expected_tests']}-done)
state['batches']=[state['batches'][0],partial]+[{'names':remaining[i:i+4],'status':'pending','checks':[]} for i in range(0,len(remaining),4)]
(E/'batch-progress.json').write_text(json.dumps(state,indent=2)+'\n',encoding='utf-8')
report={'counts_as_current_pass':False,'backend_attempt_completed':False,'source_sha256':digest(),
    'reason':'The cumulative20-minute Go package deadline expired. Preserve its failure; reuse only19 completed current-source cases with passing children, and rerun the incomplete case plus all remaining names in groups of4. Test, lease, session and business deadlines unchanged.',
    'completed_case_names':sorted(passed),'selected_pass_records':len(selected),'remaining_top_level_names':len(remaining),
    'records':records,'verification_script_sha256':sha(Path(__file__))}
(P/'preservation.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'current_completed_cases_reused':len(passed),'remaining':len(remaining),'deadline_seconds_unchanged':1200}))
