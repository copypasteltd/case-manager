"""Preserve the completed earlier client run before including the new mobile cases."""
from datetime import datetime,timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP=Path(__file__).resolve().parents[1]
E=APP.parent/'docs/quality/evidence/contract-reminders'
record=E/'client-checks.json'
read=lambda p:json.loads(p.read_text(encoding='utf-8'))
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
run=read(record)
assert run['passed'] and run['source_unchanged'] and len(run['checks'])==8
assert next(c for c in run['checks'] if c['name']=='mobile-tests')['test_count']==289
attempt=E/'attempts/04-mobile-test-selection';attempt.mkdir(parents=True,exist_ok=False)
private=APP/'.local/contract-reminders-before-mobile-test-selection';private.mkdir(exist_ok=False)
archive=private/'client-source.zip'
with zipfile.ZipFile(archive,'x',compression=zipfile.ZIP_DEFLATED) as output:
    for name in run['source_files']:output.write(APP/name,name)
logs=[]
for check in run['checks']:
    source=APP/'.local'/Path(check['raw_log']).name
    assert sha(source)==check['raw_log_sha256']
    target=private/source.name
    source.rename(target)
    logs.append({'file':target.relative_to(APP).as_posix(),'sha256':sha(target)})
record.rename(attempt/'client-checks.json')
preserved={'passed':False,'recorded_at':datetime.now(timezone.utc).isoformat(),
    'reason':'Eight original client commands passed, but their explicit mobile test list did not select the new10 policy cases. Preserve that execution; wire the cases and execute a fresh complete client run.',
    'original_commands_passed':True,'new_mobile_policy_cases_executed':False,
    'original_record_sha256':sha(attempt/'client-checks.json'),'source_archive':archive.relative_to(APP).as_posix(),
    'source_archive_sha256':sha(archive),'raw_logs':logs,'script_sha256':sha(Path(__file__))}
(attempt/'attempt.json').write_text(json.dumps(preserved,indent=2)+'\n',encoding='utf-8')
for name in ('mobile/package.json','mobile/tsconfig.concern-test.json'):
    path=APP/name;raw=path.read_bytes();value=json.loads(raw)
    if name.endswith('package.json'):
        script=value['scripts']['test'];assert 'contract-reminders.test.js' not in script
        value['scripts']['test']=script+' ../.local/mobile-concern-tests/mobile/src/platform/contract-reminders.test.js'
    else:
        assert 'src/platform/contract-reminders.test.ts' not in value['include']
        value['include'].append('src/platform/contract-reminders.test.ts')
    text=json.dumps(value,ensure_ascii=False,indent=2)+'\n'
    path.write_bytes((text.replace('\n','\r\n') if b'\r\n' in raw else text).encode('utf-8'))
print(json.dumps({'wired_new_cases':10,'earlier_commands_preserved':8,'fresh_client_run_required':True}))
