"""Inspect only current selected OA proofs; never rewrite historical evidence."""
import hashlib,json,subprocess
from pathlib import Path
from oa_line_prose_sources import APP,ROOT,sources,digest,CLIENT_SCOPES
E=ROOT/'docs/quality/evidence/oa-line-prose'
BASE='cea9ec622ad96b3520d6f36b466658de9e1e34f9'
STEPS=('web-oa-tests','mobile-oa-compile','mobile-oa-tests','runtime-tests','typecheck','web-build','weapp-build','douyin-build')
def read(p):return json.loads(p.read_text(encoding='utf-8'))
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def latest(step):return sorted(E.glob(step+'-*.json'),key=lambda p:int(p.stem.rsplit('-',1)[1]))[-1]
def verify():
 checks=[]
 def check(n,v):checks.append({'name':n,'passed':bool(v)})
 native=read(E/'native-final.json')
 check('current native HTTP/PG and MCP groups ran without skipped or failed cases',native['passed'] and native['exit_code']==0 and native['source_sha256']==native['source_after_sha256']==digest(native['source_files']) and all(v['Action']=='pass' for v in native['test_results']) and {v['Test'] for v in native['test_results'] if '/' not in v['Test']}=={'TestAgentOARequests','TestOAProseMCPBoundary'})
 names={v['Test'] for v in native['test_results']}
 check('both business kinds actually adopted through all four authenticated channel types',all('TestAgentOARequests/'+channel+'_'+kind+'_selected_line_human_adoption_preserves_all_facts' in names for channel in ('web','wechat','douyin','android') for kind in ('purchase','claim')))
 paths=[E/'native-final.json']
 for step in STEPS:
  p=latest(step);r=read(p);paths.append(p)
  check(step+' binds current portable source and Skill instructions',r['passed'] and r['source_unchanged'] and r['source_sha256']==digest(r['source_files']) and not r['new_android_apk'])
 browser=read(E.parent/'oa-line-prose-web/result.json');manifest=read(E.parent/'oa-line-prose-web/source-manifest.json')
 check('actual Web exact selection and reviewed adoption closed every owned resource',browser['passed'] and len(browser['checks'])==3 and browser['source_unchanged'] and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'] and browser['native_fixture_exit_code']==0 and browser['source_sha256']==digest(manifest['files']) and browser['build_source_sha256']==digest())
 media=read(E/'media.json');check('current actual screenshots, silent MP4 and disclosed screenshot GIF match',media['passed'] and len(media['artifacts'])==5 and all(sha(ROOT/v['file'])==v['sha256'] for v in media['artifacts']))
 check('recorded visual review matches exact current artifacts',read(E/'visual-review.json')['artifacts']==media['artifacts'])
 check('acceptance applicability remains classification only',read(E/'acceptance-applicability-check.json')['passed'])
 changed=subprocess.check_output(['git','diff',BASE,'--name-only'],cwd=ROOT).decode().splitlines()
 check('older evidence, media, migrations and frozen release metadata are unchanged',not any(n.startswith(('docs/quality/evidence/','docs/media/screenshots/','docs/media/videos/','docs/media/gifs/')) and 'oa-line-prose' not in n for n in changed) and not any(n.startswith('app/backend/migrations/') and not n.endswith('0110_oa_line_prose.sql') for n in changed) and not any(n.startswith('docs/releases/') for n in changed))
 check('owned fixtures/clusters have closed and their private handoff is removed',not (APP/'.local/business-browser-fixture.json').exists() and not (APP/'.local/oa-line-prose-native-cluster.json').exists() and not (APP/'.local/oa-line-prose-browser-cluster.json').exists())
 check('all three frozen source tags retain exact business commits',subprocess.check_output(['git','rev-parse','v0.1.0-rc.1^{}','v0.1.0-rc.2^{}','v0.1.0-rc.3^{}'],cwd=ROOT).decode().splitlines()==['a95c6afbd0e76684a64dc641c6abe598ad625277','40cd118634117204e2e310395fac32b59af5cecc','e1c7a671cc4c7f52888a1e6ed72d7ce1a9c042d3'])
 check('oa-assist1.4.0 and all eight packages remain draft',len(list((APP/'skills').glob('*/manifest.json')))==8 and all(read(p)['state']=='draft' for p in (APP/'skills').glob('*/manifest.json')) and read(APP/'skills/oa-assist/manifest.json')['version']=='1.4.0')
 retained_paths=[ROOT/'docs/quality/evidence/release-candidate-144'/n for n in ('final-source-check.json','build.json','apk-inspection.json','github-release-api.json')]
 check('retained144 APK/source/API rejection facts preserve their exact published bytes',all(sha(p)==hashlib.sha256(subprocess.check_output(['git','show',BASE+':'+p.relative_to(ROOT).as_posix()],cwd=ROOT)).hexdigest() for p in retained_paths))
 paths += [E/n for n in ('native-cluster-closed.json','media.json','visual-review.json','selected-test-counts.json','acceptance-applicability-check.json','attempts/01-schema-strict-types/reason.json','runtime-tests-1.json','native-reviewed.json')]
 paths += [E.parent/'oa-line-prose-web'/n for n in ('result.json','source-manifest.json','media.json')]+retained_paths
 records={p.relative_to(ROOT).as_posix():sha(p) for p in paths}
 retained={'base_commit':BASE,'full_backend_increment':121,'full_client_increment':136,'android_artifact_increment':144,'android_business_increment':143,'android_schema':109,'current_apk_built':False,'previous_proof_and_media_reexecuted':False,'first_native_attempt_passed_at_recorded_sources':True,'first_runtime_schema_attempt_failed_and_retained':True}
 return {'passed':all(v['passed'] for v in checks),'checks':checks,'records':records,'retained':retained,'interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False,'new_android_apk':False,'full_regression':False,'virtualization_used':False}
if __name__=='__main__':
 r=verify();(E/'final-source-check.json').write_text(json.dumps(r,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');print(json.dumps({'passed':r['passed'],'checks':r['checks']},ensure_ascii=True));raise SystemExit(0 if r['passed'] else 1)
