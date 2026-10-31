"""Independent153 selected-source proof; never relabel retained/full acceptance."""
import hashlib,json,subprocess
from pathlib import Path
from notification_telegram_sources import APP,ROOT,digest,sources
E=ROOT/'docs/quality/evidence/notification-telegram'
BASE='884f5afe2d160f7286c14569e4009a975d1264d1'
STEPS=('web-notification-telegram-tests','mobile-notification-telegram-compile','mobile-notification-telegram-tests','typecheck','web-build','weapp-build','douyin-build')
read=lambda p:json.loads(p.read_text(encoding='utf-8'))
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def git(*args):return subprocess.check_output(['git',*args],cwd=ROOT)

def verify(include_scope=True):
 checks=[];paths=[]
 def check(name,value):checks.append({'name':name,'passed':bool(value)})
 def record(p):paths.append(p);return read(p)
 native=record(E/'native-focused3.json');events=native['test_results'];names={r['Test'] for r in events}
 check('Actual native HTTP/PG selected14 subcases plus one suite pass with unchanged current backend sources',native['passed'] and len(events)==15 and all(r['Action']=='pass' for r in events) and native['source_sha256']==native['source_after_sha256']==digest(native['source_files']))
 check('Current binding, business authority, unknown outcome, manual reconciliation and client session boundaries executed',all('TestTelegramBusinessNotices/'+name in names for name in ('real_two_way_binding_is_required_and_does_not_enable_delivery','unknown_outcome_never_automatically_resends','current_business_authority_is_rechecked_before_send','authority_and_binding_are_rechecked_before_manual_retry','four_human_clients_read_own_settings_and_web_requires_CSRF','human_confirmed_delivery_does_not_read_or_complete_business')))
 transport=record(E/'telegram-transport.json')
 check('Private notice and retained secretary transport18 result records pass without real Telegram sends',transport['passed'] and not transport['real_telegram_delivery'] and len(transport['test_results'])==18 and all(r['Action']=='pass' for r in transport['test_results']) and transport['source_sha256']==transport['source_after_sha256']==digest(transport['source_files']))
 for step in STEPS:
  p=sorted(E.glob(step+'-*.json'),key=lambda p:int(p.stem.rsplit('-',1)[1]))[-1];r=record(p)
  check(step+' passes for exact declared current client source',r['passed'] and r['source_unchanged'] and r['source_sha256']==r['source_after_sha256']==digest(r['source_files']) and not r['new_android_apk'] and not r['full_regression'])
 browser=record(E.parent/'notification-telegram-web/result.json');manifest=record(E.parent/'notification-telegram-web/source-manifest.json');capture=record(E.parent/'notification-telegram-web/media.json')
 check('Actual Web review, manual reconciliation and disable close owned contexts and native servers',browser['passed'] and len(browser['checks'])==3 and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'] and browser['build_source_unchanged'] and browser['source_sha256']==manifest['sha256']==digest(manifest['files']))
 media=record(E/'media.json');review=record(E/'visual-review.json')
 check('Three reviewed screenshots and real-pixel video/disclosed screenshot GIF match exact artifact hashes',media['passed'] and review['passed'] and review['artifacts']==media['artifacts'] and review['reviewed_screenshots']==capture['screenshots'] and len(capture['screenshots'])==3 and all(sha(ROOT/a['file'])==a['sha256'] for a in [*media['artifacts'],*capture['screenshots']]))
 release=record(ROOT/'docs/releases/v0.1.0-rc.7/release-manifest.json')
 refs=release['prior_freezes_unchanged']+[{'tag':'v0.1.0-rc.7','source_commit':'59102225bc0c82bba0d838fdb8a219d06f958bbf','artifact_commit':'ced0adc94a187908d5e13ae4e45599ba5d3f3999'}]
 check('All seven annotated source and artifact freezes remain unchanged',all(git('rev-parse',r['tag']+'^{}').decode().strip()==r['source_commit'] and git('rev-parse','codex/release-'+r['tag']).decode().strip()==r['artifact_commit'] for r in refs))
 old=git('ls-tree','-r','--name-only',BASE,'--','app/backend/migrations').decode().splitlines()
 check('Migration1-112 retain original bytes and independent normal migration113 exists',len([n for n in old if n.endswith('.sql')])==112 and all((ROOT/n).read_bytes()==git('show',BASE+':'+n) for n in old) and (APP/'backend/migrations/0113_notification_telegram.sql').is_file())
 changed=git('diff',BASE,'--name-only').decode().splitlines()
 old_scripts=set(git('ls-tree','-r','--name-only',BASE,'--','app/scripts').decode().splitlines())
 check('Prior evidence, media, frozen guides and existing proof helpers are not rewritten',not any(n.startswith(('docs/quality/evidence/','docs/media/','docs/releases/')) and n!='docs/media/README.md' and not n.startswith(('docs/quality/evidence/notification-telegram','docs/media/screenshots/notification-telegram','docs/media/videos/notification-telegram','docs/media/gifs/notification-telegram')) for n in changed) and not any(n in old_scripts and n!='app/scripts/build_openapi.py' for n in changed))
 retained=record(E/'retained-functional-proof.json')
 check('Prior full backend121, clients136, runtime151 and APK152 remain explicitly retained',retained['passed'] and all(sha(ROOT/n)==v for n,v in retained['records'].items()) and retained['android_business_source_increment']==151 and retained['android_schema']==112 and not retained['new_android_apk'] and not retained['runtime_reexecuted'])
 api=record(E/'api-additive.json')
 check('Only five new API paths and14 new schemas are added; all prior contracts stay equivalent',api['passed'] and api['old_paths_changed']==[] and api['old_schemas_changed']==[] and api['new_paths']==5 and api['new_schemas']==14 and sha(ROOT/'docs/development/openapi.json')==api['sha256'] and (ROOT/'docs/development/openapi.json').read_bytes()==(APP/'backend/internal/contract/openapi.json').read_bytes())
 attempts=record(E/'attempts.json')
 check('Failed development and capture attempts remain explicitly retained',attempts['passed'] and attempts['failed_attempts_count']==4 and all(sha(ROOT/n)==v for n,v in attempts['records'].items()))
 check('All eight Skills remain draft',len(list((APP/'skills').glob('*/manifest.json')))==8 and all(read(p)['state']=='draft' for p in (APP/'skills').glob('*/manifest.json')))
 deployment=record(E/'deployment-sources.json')
 check('Additional native service template and transport runner match explicitly recorded source bytes',deployment['passed'] and all(sha(ROOT/n)==v for n,v in deployment['records'].items()))
 cleanup=record(E/'resource-cleanup.json')
 check('Owned browser ports and fixture capabilities are closed; original PG/API owners unchanged',cleanup['passed'] and cleanup['original_listeners']=={'14660':3888,'18080':2600} and cleanup['owned_listener_ports_open']==[] and not cleanup['private_fixture_present'])
 for name in (('scope-check.json','acceptance-applicability-check.json') if include_scope else ()):
  r=record(E/name);check(name+' confirms applicability only',r['passed'] and (name!='acceptance-applicability-check.json' or r['business_cases_passed']==0))
 baseline=record(ROOT/'docs/quality/current-functional-baseline.json')
 check('Current113 source and retained151/112 APK are distinguished; scope and production remain open',baseline['increment']==153 and baseline['schema_migration']==113 and baseline['current_development_database_schema']==100 and not baseline['current_android_apk_built'] and baseline['retained_android_artifact_increment']==152 and not baseline['full_goal_complete'] and not baseline['production_ready'] and retained['remaining_work_packages']==23)
 return {'passed':all(c['passed'] for c in checks),'checks':checks,'records':{p.relative_to(ROOT).as_posix():sha(p) for p in paths},'retained':{'backend_full_increment':121,'client_full_increment':136,'runtime_increment':151,'runtime_tests':58,'development_android_increment':152,'development_android_source_increment':151,'development_android_schema':112,'new_android_apk':False},'interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False,'new_android_apk':False,'virtualization_used':False}

if __name__=='__main__':
 import sys
 r=verify('--prepare' not in sys.argv);(E/'final-source-check.json').write_text(json.dumps(r,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');print(json.dumps({'passed':r['passed'],'checks':len(r['checks']),'failed':[c['name'] for c in r['checks'] if not c['passed']]}));raise SystemExit(0 if r['passed'] else 1)
