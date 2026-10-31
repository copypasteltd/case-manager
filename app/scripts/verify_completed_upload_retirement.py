"""Verify the bounded increment and historical provenance without replaying tests."""
from datetime import datetime,timezone
import hashlib,json,socket,subprocess
from pathlib import Path
from completed_upload_retirement_sources import APP,ROOT,CLIENT_SCOPES,sources,digest
E=ROOT/'docs/quality/evidence/completed-upload-retirement'
BASE='c320aaff6cd0292bbb10b5217aa97ca8b1a2ed3b'
read=lambda p:json.loads(p.read_text(encoding='utf-8'))
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()

def port_closed(port):
 with socket.socket() as sock:
  sock.settimeout(.2);return sock.connect_ex(('127.0.0.1',port))!=0

def verify(save=False):
 checks=[];records=[]
 def check(name,passed):checks.append({'name':name,'passed':bool(passed)})
 def record(p):records.append({'file':p.relative_to(ROOT).as_posix(),'sha256':sha(p)});return read(p)
 backend=record(E/'native-final.json');events=backend['test_results']
 check('Five selected actual HTTP/PG suites pass: eight client journeys, all known candidates, permanent fences, renewal, rollback and legacy cleanup',backend['passed'] and backend['exit_code']==0 and len(events)==30 and all(v['Action']=='pass' for v in events) and len([v for v in events if '/' not in v['Test']])==5)
 check('Native execution binds current complete backend, migrations and its actual harness',backend['source_files']==sources(['backend','scripts/build_openapi.py','scripts/test_completed_upload_retirement_native.py','scripts/contract_body_native_cluster.py']) and backend['source_unchanged'] and backend['source_sha256']==backend['source_after_sha256']==digest(backend['source_files']))
 latest={name:name+'-1.json' for name in ('web-tests','mobile-concern-compile','mobile-tests','mobile-transfer-compile','mobile-transfer-tests','web-build','weapp-build','douyin-build')};latest['typecheck']='typecheck-2.json'
 executions={}
 for name,file in latest.items():
  r=record(E/file);executions[name]=r
  check('Selected current client execution and complete source binding: '+name,r['passed'] and r['exit_code']==0 and r['source_unchanged'] and r['source_files']==sources(CLIENT_SCOPES) and r['source_sha256']==r['source_after_sha256']==digest(r['source_files']) and r['node_version']=='v22.23.4')
 web=record(E/'web-test-results.json')
 check('Actual selected client consumers pass, with no invented full-regression count',web['passed'] and web['execution_record_sha256']==sha(E/latest['web-tests']) and web['source_sha256']==executions['web-tests']['source_sha256'] and web['tests']=={'numPassedTests':32,'numFailedTests':0,'numPendingTests':0} and len(web['events'])==32 and all(v['status']=='passed' for v in web['events']) and executions['mobile-tests']['tests']=={'pass':23,'fail':0,'skip':0} and executions['mobile-transfer-tests']['tests']=={'pass':13,'fail':0,'skip':0})
 closed=record(E/'native-cluster-closed.json')
 check('Owned durable native PG closes; persistent development services are preserved',closed['passed'] and closed['owned_cluster_stopped'] and closed['development_cluster_preserved'] and not (APP/'.local/completed-upload-retirement-native-cluster.json').exists())
 group=E.parent/'completed-upload-retirement-web'
 browser=record(group/'result.json');manifest=record(group/'source-manifest.json');business=record(group/'business-proof.json')
 check('Authenticated Web inventories a new upload, creates one exact folder plan and durably reads it back',browser['passed'] and browser['source_unchanged'] and browser['build_source_unchanged'] and browser['source_sha256']==browser['source_after_sha256']==manifest['sha256']==digest(manifest['files']) and browser['build_source_sha256']==browser['build_source_after_sha256']==digest() and len(browser['checks'])>=3 and business['durable_readback'] and business['object_count']==2 and business['new_file_without_formal_identity'] and not business['real_cloud'])
 check('Every owned browser context, review server and disposable fixture is closed',browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'] and browser['native_fixture_exit_code']==0 and not (APP/'.local/business-browser-fixture.json').exists() and port_closed(5189))
 media=record(E/'media.json');visual=record(E/'visual-review.json')
 check('Inspected screenshots, actual post-authentication MP4 and explicit screenshot-tour GIF retain exact bytes',media['passed'] and len(media['artifacts'])==5 and all(sha(ROOT/v['file'])==v['sha256'] for v in media['artifacts']) and visual['passed'] and {v['file'] for v in visual['artifacts']}=={v['file'] for v in media['artifacts']} and all(v['reviewed'] and sha(ROOT/v['file'])==v['sha256'] for v in visual['artifacts']))
 changes=subprocess.check_output(['git','diff','--name-only',BASE,'--','docs/quality/evidence','docs/media'],cwd=ROOT).decode().splitlines()
 allowed=('docs/quality/evidence/completed-upload-retirement/','docs/quality/evidence/completed-upload-retirement-web/','docs/media/screenshots/completed-upload-retirement-','docs/media/videos/completed-upload-retirement-','docs/media/gifs/completed-upload-retirement-')
 unchanged=all(n=='docs/media/README.md' or n.startswith(allowed) for n in changes)
 migration_changes=subprocess.check_output(['git','diff','--name-only',BASE,'--','app/backend/migrations'],cwd=ROOT).decode().splitlines()
 migration_changes+=subprocess.check_output(['git','ls-files','--others','--exclude-standard','--','app/backend/migrations'],cwd=ROOT).decode().splitlines()
 check('Historical proof/media and all applied migrations1–103 remain immutable',unchanged and sorted(set(migration_changes))==['app/backend/migrations/0104_completed_upload_retirements.sql'])
 tags={'v0.1.0-rc.1^{}':'a95c6afbd0e76684a64dc641c6abe598ad625277','codex/release-v0.1.0-rc.1':'8ddb3f7a98a24910af49ede2a6c8b8c744d50973','v0.1.0-rc.2^{}':'40cd118634117204e2e310395fac32b59af5cecc','codex/release-v0.1.0-rc.2':'467b2276c0d890691d1c06238e79bb60d5f0c665'}
 check('Both published freeze tags and artifact branches remain pinned',all(subprocess.check_output(['git','rev-parse',name],cwd=ROOT).decode().strip()==value for name,value in tags.items()))
 previous=record(E.parent/'release-candidate-137/final-source-check.json')
 check('Eight Skills stay draft and older complete tests/APK are explicitly retained',len(list((APP/'skills').glob('*/manifest.json')))==8 and all(read(p)['state']=='draft' for p in (APP/'skills').glob('*/manifest.json')) and previous['passed'] and previous['schema_migration']==103)
 retained={'passed':unchanged,'base_commit':BASE,'full_backend_increment':121,'runtime_increment':121,'business_full_client_increment':136,'business_full_web_tests':452,'business_full_mobile_tests':537,'android_artifact_increment':137,'android_business_source_increment':136,'android_schema':103,'freeze_tags':['v0.1.0-rc.1','v0.1.0-rc.2'],'previous_proof_and_media_reexecuted':False,'current_android_apk_built':False}
 result={'passed':all(v['passed'] for v in checks),'checked_at':datetime.now(timezone.utc).isoformat(),'increment':138,'verification_mode':'scoped_increment','checks':checks,'records':records,'retained':retained,'interaction_kind':'completed-upload-retirement','interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'fixture_only_adjustment':None,'inspection_only_adjustment':None,'full_backend_suite_executed':False,'formal_android_acceptance':False,'current_android_apk_built':False,'current_development_database_schema':100,'schema_migration':104,'skipped_checks':['Full backend/runtime regression','Additional mobile browser journeys and fresh Android JS/JVM/APK','Persistent development PG100-to104 upgrade','Real cloud/model/Skills/TG/Linux/platform/production acceptance'],'skip_authority':'User explicitly authorized selected validation deferral to accelerate the first frozen release; this focused post-freeze increment preserves both frozen deliveries.','full_goal_complete':False,'production_ready':False}
 if save:(E/'final-source-check.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
 return result

if __name__=='__main__':
 r=verify(save=True);print(json.dumps({'passed':r['passed'],'checks':len(r['checks']),'failed':[v['name'] for v in r['checks'] if not v['passed']]}));raise SystemExit(0 if r['passed'] else 1)
