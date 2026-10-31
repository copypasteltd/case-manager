"""Read-only source/evidence inspection; does not replay deferred functional tests."""
from datetime import datetime,timezone
import hashlib,json,socket,subprocess
from pathlib import Path
from telegram_operations_sources import APP,ROOT,CLIENT_SCOPES,sources,digest
E=ROOT/'docs/quality/evidence/telegram-operations'
BASE='63b08c87c06f1797166586f7c8a1610c1a9dad1b'
read=lambda p:json.loads(p.read_text(encoding='utf-8'))
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def closed(port):
 with socket.socket() as sock:sock.settimeout(.2);return sock.connect_ex(('127.0.0.1',port))!=0
def verify(save=False):
 checks=[];records=[]
 def check(name,passed):checks.append({'name':name,'passed':bool(passed)})
 def record(p):records.append({'file':p.relative_to(ROOT).as_posix(),'sha256':sha(p)});return read(p)
 native=record(E/'native-final.json');events=native['test_results']
 check('Two selected native HTTP/PG suites pass; distinguish26 executed business scenarios from parents and the unenabled browser hold',native['passed'] and native['source_unchanged'] and len(events)==29 and all(v['Action']=='pass' for v in events) and len([v for v in events if '/' not in v['Test']])==2 and len([v for v in events if v['Test'].startswith('TestTelegramOperations/') and not v['Test'].endswith('/browser_workbench')])==12)
 check('Final native execution binds complete current backend, migrations, API builder and native harness',native['source_files']==sources(['backend','scripts/build_openapi.py','scripts/test_telegram_operations_native.py','scripts/contract_body_native_cluster.py']) and native['source_sha256']==native['source_after_sha256']==digest(native['source_files']))
 for name in ('typecheck','web-build','weapp-build','douyin-build'):
  r=record(E/(name+'-2.json'))
  check('Fresh complete client source binding: '+name,r['passed'] and r['exit_code']==0 and r['source_unchanged'] and r['source_files']==sources(CLIENT_SCOPES) and r['source_sha256']==r['source_after_sha256']==digest(r['source_files']) and r['node_version']=='v22.23.4' and not r['new_android_apk'])
 group=E.parent/'telegram-operations-web';browser=record(group/'result.json');manifest=record(group/'source-manifest.json');business=record(group/'business-proof.json')
 adjustment=record(E/'browser-fixture-adjustment.json');test=APP/adjustment['file'];current=test.read_bytes()
 old=adjustment['before'].encode();new=adjustment['after'].encode()
 reconstructed=current.replace(new,old)
 check('The sole post-capture fixture adjustment is exactly412-to404; application behavior is unchanged',adjustment['file']=='backend/internal/httpapi/telegram_operations_integration_test.go' and adjustment['replacement_count']==1 and not adjustment['application_behavior_changed'] and current.count(new)==1 and old not in current and sha(test)==adjustment['after_sha256'] and hashlib.sha256(reconstructed).hexdigest()==adjustment['before_sha256'])
 def captured_digest(names):
  h=hashlib.sha256()
  for n in names:h.update(n.encode()+b'\0'+(reconstructed if n==adjustment['file'] else (APP/n).read_bytes())+b'\0')
  return h.hexdigest()
 # This inspector was added after execution. No original capture claims to
 # have executed its future verifier, and its bytes bind the final proof below.
 capture_build_files=[n for n in sources() if n!='scripts/verify_telegram_operations.py']
 inspection={'scope':'Only this new read-only final verifier was absent from the original complete capture-build source list.','added_source':'scripts/verify_telegram_operations.py','current_sha256':sha(Path(__file__)),'capture_reexecuted':False}
 check('Actual Web execution retains its exact original sources through the disclosed fixture-only adjustment',browser['passed'] and browser['source_unchanged'] and browser['build_source_unchanged'] and browser['source_sha256']==browser['source_after_sha256']==manifest['sha256']==captured_digest(manifest['files']) and browser['build_source_sha256']==browser['build_source_after_sha256']==captured_digest(capture_build_files) and len(browser['checks'])==3 and business['durable_readback'] and business['metadata_allowlist'] and business['journal_events']==4 and business['manual_confirmation'] and not business['bot_api_contacted'] and not business['real_group'])
 cluster=record(E/'native-cluster-closed.json');webcluster=record(E/'native-telegram-operations-web/native-cluster-closed.json')
 check('Owned durable native clusters, browser, temporary service and fixture all close',cluster['passed'] and webcluster['passed'] and cluster['owned_cluster_stopped'] and webcluster['owned_cluster_stopped'] and browser['owned_browser_closed'] and browser['native_fixture_exit_code']==0 and browser['isolated_database_cleanup_passed'] and browser['owned_review_server_stopped'] and not (APP/'.local/business-browser-fixture.json').exists() and not (APP/'.local/telegram-operations-native-cluster.json').exists() and not (APP/'.local/telegram-operations-browser-cluster.json').exists() and closed(5189))
 media=record(E/'media.json');visual=record(E/'visual-review.json')
 check('Three inspected screenshots, actual authenticated MP4 and labelled screenshot-tour GIF retain exact bytes',media['passed'] and len(media['artifacts'])==5 and all(sha(ROOT/v['file'])==v['sha256'] for v in media['artifacts']) and visual['passed'] and {v['file'] for v in visual['artifacts']}=={v['file'] for v in media['artifacts']} and all(v['reviewed'] and sha(ROOT/v['file'])==v['sha256'] for v in visual['artifacts']))
 changes=subprocess.check_output(['git','diff','--name-only',BASE,'--','docs/quality/evidence','docs/media'],cwd=ROOT).decode().splitlines()
 changes+=subprocess.check_output(['git','ls-files','--others','--exclude-standard','--','docs/quality/evidence','docs/media'],cwd=ROOT).decode().splitlines()
 allowed=('docs/quality/evidence/telegram-operations/','docs/quality/evidence/telegram-operations-web/','docs/media/screenshots/telegram-operations-','docs/media/videos/telegram-operations-','docs/media/gifs/telegram-operations-')
 unchanged=all(n=='docs/media/README.md' or n.startswith(allowed) for n in changes)
 migrations=subprocess.check_output(['git','diff','--name-only',BASE,'--','app/backend/migrations'],cwd=ROOT).decode().splitlines()+subprocess.check_output(['git','ls-files','--others','--exclude-standard','--','app/backend/migrations'],cwd=ROOT).decode().splitlines()
 check('Historical proof/media and all installed migrations1–104 remain immutable',unchanged and sorted(set(migrations))==['app/backend/migrations/0105_telegram_operations.sql'])
 tags={'v0.1.0-rc.1^{}':'a95c6afbd0e76684a64dc641c6abe598ad625277','codex/release-v0.1.0-rc.1':'8ddb3f7a98a24910af49ede2a6c8b8c744d50973','v0.1.0-rc.2^{}':'40cd118634117204e2e310395fac32b59af5cecc','codex/release-v0.1.0-rc.2':'467b2276c0d890691d1c06238e79bb60d5f0c665'}
 check('Both published frozen candidates and artifact branches stay pinned',all(subprocess.check_output(['git','rev-parse',name],cwd=ROOT).decode().strip()==v for name,v in tags.items()))
 previous=record(E.parent/'completed-upload-retirement/final-source-check.json')
 check('Previous proof remains historical and all eight Skills remain draft',previous['passed'] and previous['increment']==138 and len(list((APP/'skills').glob('*/manifest.json')))==8 and all(read(p)['state']=='draft' for p in (APP/'skills').glob('*/manifest.json')))
 failures=[record(p) for p in sorted(E.glob('native-*.json')) if p.name not in ('native-final.json','native-cluster.json','native-cluster-closed.json')]
 check('Failed attempts are preserved with exact execution source/log hashes',len(failures)==5 and all(not r['passed'] and r['source_unchanged'] and r['private_log_sha256'] for r in failures))
 retained={'passed':unchanged,'base_commit':BASE,'full_backend_increment':121,'runtime_increment':121,'business_full_client_increment':136,'business_full_web_tests':452,'business_full_mobile_tests':537,'android_artifact_increment':137,'android_business_source_increment':136,'android_schema':103,'freeze_tags':['v0.1.0-rc.1','v0.1.0-rc.2'],'previous_proof_and_media_reexecuted':False,'current_android_apk_built':False}
 result={'passed':all(v['passed'] for v in checks),'checked_at':datetime.now(timezone.utc).isoformat(),'increment':139,'verification_mode':'scoped_increment','checks':checks,'records':records,'retained':retained,'interaction_kind':'telegram-operations','interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'fixture_only_adjustment':{'record':'docs/quality/evidence/telegram-operations/browser-fixture-adjustment.json','sha256':sha(E/'browser-fixture-adjustment.json')},'inspection_only_adjustment':inspection,'full_backend_suite_executed':False,'formal_android_acceptance':False,'current_android_apk_built':False,'current_development_database_schema':100,'schema_migration':105,'executed_new_business_scenarios':12,'executed_existing_delivery_scenarios':14,'unexecuted_browser_hold_in_normal_native_run':True,'skipped_checks':['Full backend/runtime and Web/mobile regression','Complete mobile browser journeys and new Android JS/JVM/APK','Persistent development PG100-to105 upgrade','Real Bot/model/Skills/Linux/cloud/platform/production acceptance'],'skip_authority':'User authorized selected validation deferral to accelerate frozen delivery; both frozen candidates stay immutable.','full_goal_complete':False,'production_ready':False}
 if save:(E/'final-source-check.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
 return result
if __name__=='__main__':
 r=verify(save=True);print(json.dumps({'passed':r['passed'],'checks':len(r['checks']),'failed':[v['name'] for v in r['checks'] if not v['passed']]}));raise SystemExit(0 if r['passed'] else 1)
