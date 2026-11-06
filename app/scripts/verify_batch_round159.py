"""Current complete-wave evidence with explicit deferred and retained acceptance."""
import hashlib,json,re,subprocess
from pathlib import Path
from run_batch_round159 import ROOT,APP,EVIDENCE as E,source_names,source_digest,client_source_names,load_plan
BASE='53f4b3fdf421351a87e6878322094eed70182fa7'
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def read(p):return json.loads(p.read_text(encoding='utf-8'))
def digest(names=None):return source_digest(names or source_names())
def verify(include_scope=True):
 checks=[];records={};current=digest();plan=load_plan()
 def check(name,value):checks.append({'name':name,'passed':bool(value)})
 def record(p):records[p.relative_to(ROOT).as_posix()]=sha(p);return read(p)
 def latest(phase):
  target=source_digest(client_source_names()) if phase=='client-checks' else current
  values=[p for p in E.glob(phase+'-*.json') if read(p).get('passed') and read(p).get('source_sha256')==target]
  if not values:raise ValueError('Passing current-source evidence missing: '+phase)
  return record(max(values,key=lambda p:read(p)['started_at']))
 freeze=latest('freeze-code');native=record(E/'native-tests-batch-5.json');clients=latest('client-checks');initial=record(E/'freeze-code-initial.json');first_native=record(E/'native-tests-initial.json');first_clients=record(E/'client-checks-initial.json');repairs=record(E/'batch-repairs.json')
 check('Complete declared code wave precedes first unified validation',not initial['tests_executed'] and initial['started_at']<first_native['started_at'] and initial['started_at']<first_clients['started_at'] and plan['implementation_completed_before_tests'])
 check('Initial failures and concentrated repair ledger retained',not first_native['passed'] and not first_clients['passed'] and bool(repairs['repairs']))
 check('All23 packages and six implementation tasks are reviewed',len(plan['packages'])==23 and len(plan['round_tasks'])==6 and all(i['state']=='implemented' for i in plan['round_tasks']) and all(i['acceptance_state']=='open' for i in plan['packages']) and freeze['reviewed_package_ids']==native['reviewed_package_ids']==clients['reviewed_package_ids'])
 webfix=record(E/'web-only-6.json');oldweb=ROOT/webfix['retained_original'];records[webfix['retained_original']]=sha(oldweb)
 def reconstructed(names,overrides):
  h=hashlib.sha256()
  for name in names:h.update(name.encode()+b'\0'+(overrides[name].read_bytes() if name in overrides else (ROOT/name).read_bytes())+b'\0')
  return h.hexdigest()
 results=native['test_results'];passed={i.get('Test') for i in results if i['Action']=='pass'}
 check('Selected native159 suites pass; only unrelated Web form source changed later',native['source_unchanged'] and reconstructed(native['source_files'],{webfix['affected_file']:oldweb})==native['source_sha256'] and sha(oldweb)==webfix['retained_original_sha256'] and webfix['web_only'] and not webfix['backend_mobile_runtime_changed'] and not any(i['Action']=='fail' for i in results) and 'TestRound159AgentBusinessRevisions' in passed)
 prior=record(E/'native-tests-batch-2.json');adjustment=record(E/'fixture-only-3.json');archived=ROOT/adjustment['retained_original'];records[adjustment['retained_original']]=sha(archived);h=hashlib.sha256()
 for name in prior['source_files']:h.update(name.encode()+b'\0'+(archived.read_bytes() if name==adjustment['affected_file'] else oldweb.read_bytes() if name==webfix['affected_file'] else (ROOT/name).read_bytes())+b'\0')
 retained_passed={i.get('Test') for i in prior['test_results'] if i['Action']=='pass'}
 check('Unaffected native suites retained with exact fixture-only source reconstruction',prior['source_unchanged'] and h.hexdigest()==prior['source_sha256'] and sha(archived)==adjustment['retained_original_sha256'] and adjustment['fixture_only'] and not adjustment['production_mfa_behavior_changed'] and {'TestAgentOARequests','TestAgentRuntimeOperations','TestRound158AgentBusinessDrafts'}<=retained_passed)
 required={'typecheck','web-tests','mobile-tests','mobile-new-contracts','runtime-tests','skill-catalog','web-build','weapp-build','douyin-build'}
 previous_clients=record(E/'client-checks-batch-2.json');fresh_steps={i['step'] for i in clients['test_results']};old_steps={i['step'] for i in previous_clients['test_results']}
 check('Three affected Web checks pass with six unchanged client results explicitly retained',clients['source_unchanged'] and clients['source_after_sha256']==source_digest(client_source_names()) and clients['source_files']==client_source_names() and fresh_steps=={'typecheck','web-tests','web-build'} and old_steps==required and all(i['passed'] for i in clients['test_results']+previous_clients['test_results']) and reconstructed(previous_clients['source_files'],{webfix['affected_file']:oldweb})==previous_clients['source_sha256'])
 retained=record(ROOT/'docs/quality/evidence/batch-round-158/final-source-check.json');old_android=record(ROOT/'docs/quality/evidence/batch-round-158/client-scope-disposition.json')
 check('Prior full regression and failed Android JS remain explicit retained evidence',retained['passed'] and not retained['full_backend_suite_executed'] and not old_android['current_android_js_build_passed'] and not old_android['new_android_apk'] and plan['current_android_js_rebuild_deferred'] and not plan['new_android_apk'])
 candidates=[p for p in E.parent.glob('batch-round-159-business-revisions-*/result.json') if read(p).get('passed')]
 if not candidates:raise ValueError('Current actual Web evidence missing')
 p=max(candidates,key=lambda p:read(p)['finished_at']);v=record(p);manifest=record(p.parent/'source-manifest.json');capture=record(p.parent/'encoded-media.json');h=hashlib.sha256()
 for name in manifest['files']:h.update(name.encode()+b'\0'+(APP/name).read_bytes()+b'\0')
 check('Actual Web full forms/review/zero premature save bind current source',h.hexdigest()==manifest['sha256']==v['source_sha256'] and v['source_unchanged'] and v['build_source_unchanged'] and len(v['checks'])>=5 and all(v[k] for k in ['owned_browser_closed','owned_review_server_stopped','isolated_database_cleanup_passed']) and v['native_fixture_exit_code']==0)
 check('Actual screenshots/recording/tour match output hashes',capture['passed'] and not capture['production_acceptance'] and all(sha(ROOT/i['file'])==i['sha256'] for i in capture['screenshots']+capture['artifacts']))
 interaction=record(p.parent/'interaction-media.json');check('Readable interaction GIF binds immutable actual recording',interaction['passed'] and interaction['actual_viewport_recording_conversion'] and sha(ROOT/interaction['file'])==interaction['sha256'] and sha(ROOT/interaction['source_video'])==interaction['source_video_sha256'] and sha(APP/'scripts/derive_batch_round159_interaction.py')==interaction['source_script_sha256'])
 visual=record(E/'visual-review.json');check('Current screenshots and GIF inspected',visual['passed'] and len(visual['files'])>=2 and not visual['production_acceptance'] and all(sha(ROOT/i['file'])==i['sha256'] for i in visual['files']))
 changed=subprocess.check_output(['git','diff','--name-only',BASE,'--','app/backend/migrations','docs/quality/evidence','docs/media'],cwd=ROOT,text=True).splitlines();forbidden=[]
 for name in changed:
  if name.startswith('app/backend/migrations/'):
   if int(Path(name).name.split('_')[0])<=120:forbidden.append(name)
  elif name!='docs/media/README.md' and not any(name.startswith(s) for s in ['docs/quality/evidence/batch-round-159','docs/media/batch-round-159/']):forbidden.append(name)
 check('Prior120 migrations and all earlier proofs/media unchanged',not forbidden)
 old=(APP/'backend/migrations/0119_agent_business_drafts.sql').read_text(encoding='utf-8');new=(APP/'backend/migrations/0121_agent_business_revisions.sql').read_text(encoding='utf-8');pattern=r'ADD CONSTRAINT agent_tool_receipts_tool_check CHECK\(tool IN \(([^;]+)\)\);'
 values=lambda s:set(re.findall("'([^']+)'",re.search(pattern,s).group(1)))
 check('Tool constraint preserves old commands and adds only fixed revision',values(new)==values(old)|{'revise_business_draft'})
 tags=subprocess.check_output(['git','tag','--list','v0.1.0-rc.*'],cwd=ROOT,text=True).splitlines();rc8=subprocess.check_output(['git','rev-parse','v0.1.0-rc.8^{}'],cwd=ROOT,text=True).strip()
 check('All8 older freezes remain fixed',len(tags)==8 and rc8=='160d78f8032be21c9eab72fc28415fda598fb7df')
 skills=[read(p) for p in (APP/'skills').glob('*/manifest.json')];check('Eight Skills remain draft with exact L2 OA version',len(skills)==8 and all(p['state']=='draft' for p in skills) and any(p['skill_id']=='oa-assist' and p['version']=='1.6.0' and p['authorization_level']=='L2' for p in skills))
 scope=read(ROOT/'docs/delivery/04-current-scope.json');check('23 active packages and five exclusions preserved',len([i for i in scope['items'] if i['status']=='active'])==23 and len([i for i in scope['items'] if i['status']!='active'])==5)
 return {'passed':all(i['passed'] for i in checks),'checks':checks,'records':records,'interaction_source_sha256':current,'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False,'virtualization_used':False,'new_android_apk':False,'native_result_records':len(results),'current_four_client_builds':False,'current_client_builds':['Web','WeChat','Douyin'],'current_android_js_build_passed':False,'deferred_checks':['android-bundle'],'limited_release_scope':True,'retained':{'full_backend_increment':121,'prior_selected_increment':158,'development_android_increment':154,'development_android_source_increment':153,'development_android_schema':113}}
if __name__=='__main__':
 v=verify();(E/'final-source-check.json').write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');print(json.dumps({'passed':v['passed'],'checks':len(v['checks']),'failed':[i['name'] for i in v['checks'] if not i['passed']]}));raise SystemExit(0 if v['passed'] else 1)
