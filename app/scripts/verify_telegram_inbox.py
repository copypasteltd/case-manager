"""Read-only integrity inspection of focused checks; no deferred tests are replayed."""
from datetime import datetime,timezone
import hashlib,json,socket,subprocess
from pathlib import Path
from telegram_inbox_sources import APP,ROOT,CLIENT_SCOPES,sources,digest
E=ROOT/'docs/quality/evidence/telegram-inbox'
BASE='d891be5957c7affc7c7648691cc4355305d91b7f'
read=lambda p:json.loads(p.read_text(encoding='utf-8'))
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()

def verify(save=False):
 checks=[];records={}
 def check(name,passed):checks.append({'name':name,'passed':bool(passed)})
 def record(path):
  records[path.relative_to(ROOT).as_posix()]=sha(path)
  return read(path)
 native=record(E/'native-final.json')
 results=native['test_results'];top={v['Test'] for v in results if v['Action']=='pass' and '/' not in v['Test']}
 check('Selected actual HTTP/PG suites execute on the complete current backend inputs',native['passed'] and native['source_unchanged'] and native['source_sha256']==native['source_after_sha256']==digest(native['source_files']) and top=={'TestTelegramInboxOperations','TestTelegramDispatch'} and all(v['Action']=='pass' for v in results))
 required=['web_metadata_withdrawal_and_replay','wechat_metadata_withdrawal_and_replay','douyin_metadata_withdrawal_and_replay','android_metadata_withdrawal_and_replay','actual_dispatcher_wait_invalidates_stale_withdrawal','audit_failure_rolls_back_request_journal_and_receipt','database_journal_cannot_be_forged_mutated_or_bypassed','dispatcher_withdrawal_race_produces_one_coherent_outcome','automatic_channel_withdrawal_is_journaled_without_invented_human_decision','current_role_is_checked_before_successful_replay']
 check('Current business checks cover four channels, real dispatcher ordering, immutable journals and atomic failure',all(any(v['Test']=='TestTelegramInboxOperations/'+name and v['Action']=='pass' for v in results) for name in required))
 for step in ['typecheck','web-build','weapp-build','douyin-build']:
  files=sorted(E.glob(step+'-*.json'),key=lambda p:int(p.stem.rsplit('-',1)[1]));r=record(files[-1])
  check('Current '+step+' binds exact portable client inputs',r['passed'] and r['source_unchanged'] and r['source_files']==sources(CLIENT_SCOPES) and r['source_sha256']==r['source_after_sha256']==digest(r['source_files']) and r['node_version'].startswith('v22.') and not r['new_android_apk'])
 B=E.parent/'telegram-inbox-web';browser=record(B/'result.json');manifest=record(B/'source-manifest.json');business=record(B/'business-proof.json')
 check('Actual authenticated Web withdrawal persists exact source-bound metadata and history',browser['passed'] and browser['source_unchanged'] and browser['build_source_unchanged'] and browser['source_sha256']==browser['source_after_sha256']==manifest['sha256']==digest(manifest['files']) and browser['build_source_sha256']==browser['build_source_after_sha256']==digest() and len(browser['checks'])==3 and business['manual_withdrawal'] and business['durable_readback'] and business['metadata_allowlist'] and business['journal_events']==2 and not business['secretary_run_created'] and not business['bot_api_contacted'])
 native_closed=record(E/'native-cluster-closed.json');browser_closed=record(E/'native-telegram-inbox-web/native-cluster-closed.json')
 with socket.socket() as sock:port_closed=sock.connect_ex(('127.0.0.1',5189))!=0
 check('Every owned browser, fixture, review service and durable native PG cluster closes',native_closed['passed'] and browser_closed['passed'] and native_closed['owned_cluster_stopped'] and browser_closed['owned_cluster_stopped'] and browser['owned_browser_closed'] and browser['native_fixture_exit_code']==0 and browser['isolated_database_cleanup_passed'] and browser['owned_review_server_stopped'] and port_closed and not (APP/'.local/business-browser-fixture.json').exists() and not (APP/'.local/telegram-inbox-native-cluster.json').exists() and not (APP/'.local/telegram-inbox-browser-cluster.json').exists())
 media=record(E/'media.json');visual=record(E/'visual-review.json')
 check('Inspected screenshots, actual viewport MP4 and labelled screenshot-tour GIF retain exact bytes',media['passed'] and len(media['artifacts'])==5 and all(sha(ROOT/v['file'])==v['sha256'] for v in media['artifacts']) and visual['passed'] and {v['file'] for v in visual['artifacts']}=={v['file'] for v in media['artifacts']} and all(v['reviewed'] and sha(ROOT/v['file'])==v['sha256'] for v in visual['artifacts']))
 changes=subprocess.check_output(['git','diff','--name-only',BASE,'--','docs/quality/evidence','docs/media'],cwd=ROOT).decode().splitlines()+subprocess.check_output(['git','ls-files','--others','--exclude-standard','--','docs/quality/evidence','docs/media'],cwd=ROOT).decode().splitlines()
 allowed=('docs/quality/evidence/telegram-inbox/','docs/quality/evidence/telegram-inbox-web/','docs/media/screenshots/telegram-inbox-','docs/media/videos/telegram-inbox-','docs/media/gifs/telegram-inbox-')
 unchanged=all(n=='docs/media/README.md' or n.startswith(allowed) for n in changes)
 migrations=subprocess.check_output(['git','diff','--name-only',BASE,'--','app/backend/migrations'],cwd=ROOT).decode().splitlines()+subprocess.check_output(['git','ls-files','--others','--exclude-standard','--','app/backend/migrations'],cwd=ROOT).decode().splitlines()
 check('Prior proof/media and installed migrations1–105 remain immutable',unchanged and sorted(set(migrations))==['app/backend/migrations/0106_telegram_inbox_operations.sql'])
 tags={'v0.1.0-rc.1^{}':'a95c6afbd0e76684a64dc641c6abe598ad625277','codex/release-v0.1.0-rc.1':'8ddb3f7a98a24910af49ede2a6c8b8c744d50973','v0.1.0-rc.2^{}':'40cd118634117204e2e310395fac32b59af5cecc','codex/release-v0.1.0-rc.2':'467b2276c0d890691d1c06238e79bb60d5f0c665'}
 check('Both frozen candidates stay pinned; all eight Skills remain draft',all(subprocess.check_output(['git','rev-parse',k],cwd=ROOT).decode().strip()==v for k,v in tags.items()) and len(list((APP/'skills').glob('*/manifest.json')))==8 and all(read(p)['state']=='draft' for p in (APP/'skills').glob('*/manifest.json')))
 previous=record(E.parent/'telegram-operations/final-source-check.json')
 check('Previous functional evidence stays historical with explicit retained artifact provenance',previous['passed'] and previous['increment']==139)
 retained={'passed':unchanged,'base_commit':BASE,'full_backend_increment':121,'runtime_increment':121,'business_full_client_increment':136,'business_full_web_tests':452,'business_full_mobile_tests':537,'android_artifact_increment':137,'android_business_source_increment':136,'android_schema':103,'freeze_tags':['v0.1.0-rc.1','v0.1.0-rc.2'],'previous_proof_and_media_reexecuted':False,'current_android_apk_built':False}
 value={'passed':all(v['passed'] for v in checks),'checked_at':datetime.now(timezone.utc).isoformat(),'increment':140,'verification_mode':'scoped_increment','checks':checks,'records':records,'retained':retained,'interaction_kind':'telegram-inbox','interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'current_android_apk_built':False,'current_development_database_schema':100,'schema_migration':106,'skipped_checks':['Full backend/runtime and Web/mobile regression','Complete mobile UI journeys and new Android JS/JVM/APK','Existing persistent development PG100-to106 upgrade','Real Bot/model/Skills/Linux/cloud/platform/production acceptance'],'skip_authority':'User authorized selected validation deferral for accelerated frozen delivery. Both frozen candidates stay immutable.','full_goal_complete':False,'production_ready':False}
 if save:(E/'final-source-check.json').write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
 return value
if __name__=='__main__':
 r=verify(save=True);print(json.dumps({'passed':r['passed'],'checks':len(r['checks']),'failed':[v['name'] for v in r['checks'] if not v['passed']]}));raise SystemExit(0 if r['passed'] else 1)
