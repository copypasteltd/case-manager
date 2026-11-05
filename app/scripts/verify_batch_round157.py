"""Independent current batch evidence, with retained artifacts kept explicit."""
import hashlib,json,subprocess
from pathlib import Path
from run_batch_round import ROOT,APP,source_names,source_digest
from record_batch_client_inputs import client_names,client_digest
E=ROOT/'docs/quality/evidence/batch-round-157'
BASE='fb81cbbf829d9dc2b233fb38ebb28940520e4cff'
def digest(names=None):return source_digest(names or source_names())
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def read(p):return json.loads(p.read_text(encoding='utf-8'))
def verify(include_scope=True):
    checks=[];records={};current=digest()
    def check(name,value):checks.append({'name':name,'passed':bool(value)})
    def record(p):records[p.relative_to(ROOT).as_posix()]=sha(p);return read(p)
    def latest(phase):
        values=[]
        for p in E.glob(phase+'-*.json'):
            value=read(p)
            if value.get('passed') and value.get('source_sha256')==current:values.append((value['started_at'],p))
        if not values:raise ValueError('No passing current-source batch: '+phase)
        return record(max(values)[1])
    freeze=latest('freeze-code');native=latest('native-tests')
    candidates=[]
    for p in E.glob('client-checks-*.json'):
        value=read(p);attempt=value.get('attempt','')
        before=E/('client-inputs-'+attempt+'-before.json');after=E/('client-inputs-'+attempt+'-after.json')
        if not before.exists() or not after.exists() or not all(i.get('exit_code')==0 for i in value.get('test_results',[]) if i['step']!='android-bundle'):continue
        b,a=read(before),read(after)
        if b['source_files']==a['source_files']==client_names() and b['source_sha256']==a['source_sha256']==client_digest() and b['product_source_sha256']==value['source_sha256']:candidates.append((value['started_at'],p,before,after))
    if not candidates:raise ValueError('Current unchanged client inputs and completed checks are required.')
    _,p,before,after=max(candidates);clients=record(p);record(before);record(after)
    initial=record(E/'freeze-code-code-wave-1.json')
    check('Complete initial code wave precedes checks and current backend repairs precede rechecks',not freeze['tests_executed'] and freeze['started_at']<native['started_at'] and initial['started_at']<clients['started_at'])
    check('All23 numbered packages were reviewed without closing acceptance',len(freeze['reviewed_package_ids'])==23 and freeze['reviewed_package_ids']==native['reviewed_package_ids']==clients['reviewed_package_ids'])
    result=native['test_results'];passed={i.get('Test') for i in result if i['Action']=='pass'}
    check('Current native Office, release governance and TG handoff/disclosure run successfully',native['source_unchanged'] and native['source_after_sha256']==current and not any(i['Action']=='fail' for i in result) and {'TestRound157OfficeUpdateAndAbandonment','TestRound157SkillGovernance','TestRound157TelegramHandoffAndDisclosure','TestRound157OfficeEdits','TestRound157ToolListing','TestCompletedUploadRetirementFourClients'}<=passed)
    required={'typecheck','web-tests','mobile-tests','mobile-new-contracts','runtime-tests','skill-catalog','web-build','weapp-build','douyin-build','android-bundle'}
    android=record(E/('android-js-supplement.json' if clients['attempt']=='batch-3' else 'android-js-supplement-'+clients['attempt']+'.json'))
    check('Unified client types/tests/runtime and four builds pass for unchanged complete client dependencies',{i['step'] for i in clients['test_results']}==required and all(i['passed'] and i['exit_code']==0 for i in clients['test_results'] if i['step']!='android-bundle') and android['passed'] and android['client_source_sha256']==client_digest() and android['client_source_files']==client_names() and not android['new_apk'])
    check('Final repaired product source completes all ten client steps together',clients['attempt']=='batch-4' and clients['passed'] and clients['source_unchanged'] and clients['source_after_sha256']==current==clients['source_sha256'] and all(i['passed'] and i['exit_code']==0 for i in clients['test_results']))
    check('Android supplement matches its recorder and actual current application modules',android['recorder_sha256']==sha(APP/'scripts/record_batch_android157.py') and android['build']['native_invocation']['exit_code']==0 and android['build']['no_expo_map_guard'] and len(android['modules'])>=190 and all(sha(APP/i['file'])==i['sha256'] for i in android['modules']))
    closed=record(E/('native-'+native['attempt'])/'native-cluster-closed.json')
    check('Current native fixture cluster closes without disposable databases and preserves development services',closed['passed'] and closed['owned_cluster_stopped'] and closed['remaining_disposable_databases']==0 and closed['development_cluster_preserved'])
    media=[]
    for kind in ('office','telegram'):
        candidates=[]
        for p in E.parent.glob('batch-round-157-'+kind+'-*/result.json'):
            v=read(p)
            if v.get('passed') and v.get('build_source_sha256')==current:candidates.append((v['started_at'],p))
        if not candidates:raise ValueError('Current-source browser capture missing: '+kind)
        p=max(candidates)[1];v=record(p);capture=record(p.parent/'encoded-media.json');record(p.parent/'media.json');record(p.parent/'capture-manifest.json')
        manifest=record(p.parent/'source-manifest.json');h=hashlib.sha256()
        for file in manifest['files']:h.update(file.encode()+b'\0'+(APP/file).read_bytes()+b'\0')
        check(kind+' browser instructions and exact capture sources still match',h.hexdigest()==manifest['sha256']==v['source_sha256'])
        check(kind+' actual UI/capture uses synthetic native records and closes owned resources',v['source_unchanged'] and v['build_source_unchanged'] and v['owned_browser_closed'] and v['owned_review_server_stopped'] and v['isolated_database_cleanup_passed'] and v['native_fixture_exit_code']==0 and len(v['checks'])>=1 and capture['passed'] and not capture['production_acceptance'])
        media.extend(capture['screenshots']+capture['artifacts'])
    check('Current screenshots, real silent recordings and disclosed GIF tours match bytes',bool(media) and all(sha(ROOT/i['file'])==i['sha256'] for i in media))
    visual=record(E/'visual-review.json')
    check('Visual review binds four inspected screenshots and two GIF tours',visual['passed'] and len(visual['files'])==6 and not visual['production_acceptance'] and all(sha(ROOT/i['file'])==i['sha256'] for i in visual['files']))
    changed=subprocess.check_output(['git','diff','--name-only',BASE,'--','app/backend/migrations','docs/media','docs/quality/evidence'],cwd=ROOT).decode().splitlines()
    forbidden=[]
    for p in changed:
        if p.startswith('app/backend/migrations/'):
            if int(Path(p).name.split('_')[0])<=113:forbidden.append(p)
        elif p!='docs/media/README.md' and not any(p.startswith(prefix) for prefix in ('docs/media/batch-round-157/','docs/quality/evidence/batch-round-157')):forbidden.append(p)
    check('Old migrations/proofs/media remain unchanged',not forbidden)
    tags=subprocess.check_output(['git','tag','--list','v0.1.0-rc.*'],cwd=ROOT).decode().splitlines()
    rc8=subprocess.check_output(['git','rev-parse','v0.1.0-rc.8^{commit}'],cwd=ROOT).decode().strip()
    check('All eight freezes and exact latest source stay immutable',len(tags)==8 and rc8=='160d78f8032be21c9eab72fc28415fda598fb7df')
    scope=read(ROOT/'docs/delivery/04-current-scope.json')
    check('All23 acceptance packages and five exclusions remain explicit',len([i for i in scope['items'] if i['status']=='active'])==23 and len([i for i in scope['items'] if i['status']!='active'])==5)
    return {'passed':all(i['passed'] for i in checks),'checks':checks,'records':records,'retained':{'full_backend_increment':121,'development_android_increment':154,'development_android_source_increment':153,'development_android_schema':113},'interaction_source_sha256':current,'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False,'virtualization_used':False,'new_android_apk':False,'native_result_records':len(result),'current_four_client_builds':True}
if __name__=='__main__':
    value=verify();(E/'final-source-check.json').write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':value['passed'],'checks':len(value['checks']),'failed':[i['name'] for i in value['checks'] if not i['passed']]}))
    raise SystemExit(0 if value['passed'] else 1)
