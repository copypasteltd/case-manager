"""Current whole-wave evidence, limited local scope, immutable earlier delivery."""
import hashlib,json,subprocess,re
from pathlib import Path
from run_batch_round158 import ROOT,APP,source_names,source_digest,client_source_names
E=ROOT/'docs/quality/evidence/batch-round-158';BASE='cd0f4a96df103840a5a0bff71323109afbb28475'
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def read(p):return json.loads(p.read_text(encoding='utf-8'))
def digest(names=None):return source_digest(names or source_names())
def verify(include_scope=True):
 checks=[];records={};current=digest()
 def check(name,v):checks.append({'name':name,'passed':bool(v)})
 def record(p):records[p.relative_to(ROOT).as_posix()]=sha(p);return read(p)
 def latest(phase):
  values=[(read(p)['started_at'],p) for p in E.glob(phase+'-*.json') if read(p).get('passed') and read(p).get('source_sha256')==(source_digest(client_source_names()) if phase=='client-checks' else current)]
  if not values:raise ValueError('Passing current-source record missing: '+phase)
  return record(max(values)[1])
 freeze=latest('freeze-code');native=latest('native-tests');clients=record(E/'client-checks-batch-3.json');disposition=record(E/'client-scope-disposition.json');initial=record(E/'freeze-code-code-wave-1.json')
 check('All declared application/fixture code precedes first unified checks',not initial['tests_executed'] and initial['started_at']<native['started_at'] and initial['started_at']<clients['started_at'])
 check('All23 packages reviewed without invented acceptance',len(freeze['reviewed_package_ids'])==23 and freeze['reviewed_package_ids']==native['reviewed_package_ids']==clients['reviewed_package_ids'])
 result=native['test_results'];passed={i.get('Test') for i in result if i['Action']=='pass'}
 check('Affected current native journeys execute against disposable PG',native['source_unchanged'] and native['source_after_sha256']==current and not any(i['Action']=='fail' for i in result) and {'TestRound158AgentBusinessDrafts','TestRound158DerivedTextRetirement'}<=passed)
 retained=record(E/'native-tests-batch-3.json');inventory=record(E/'native-batch-3-source-inventory.json')
 changes={n for n in set(inventory['files'])|set(source_names()) if n not in inventory['files'] or not (ROOT/n).exists() or sha(ROOT/n)!=inventory['files'][n]}
 allowed={'app/backend/internal/httpapi/round158_business_drafts_integration_test.go','app/backend/migrations/0120_derived_text_retirement.sql','app/backend/internal/files/derived_retirement.go','app/backend/migrations/0119_agent_business_drafts.sql'}
 previous_failed={i['Test'] for i in retained['test_results'] if i['Action']=='fail' and i.get('Test') and '/' not in i['Test']}
 check('Prior selected checks remain source-disclosed; only two affected journeys are replayed',inventory['source_sha256']==retained['source_sha256'] and retained['source_unchanged'] and changes<=allowed and previous_failed=={'TestRound158AgentBusinessDrafts','TestRound158DerivedTextRetirement'} and previous_failed<=passed)
 required={'typecheck','web-tests','mobile-tests','mobile-new-contracts','runtime-tests','skill-catalog','web-build','weapp-build','douyin-build','android-bundle'}
 check('Nine current client steps pass; Android JS timeout remains explicitly failed and deferred',clients['source_unchanged'] and clients['source_after_sha256']==source_digest(client_source_names()) and clients['source_files']==client_source_names() and required=={i['step'] for i in clients['test_results']} and {i['step'] for i in clients['test_results'] if not i['passed']}=={'android-bundle'} and not clients['passed'] and disposition['record_sha256']==sha(E/'client-checks-batch-3.json') and disposition['source_sha256']==source_digest(client_source_names()) and not disposition['current_android_js_build_passed'] and not disposition['new_android_apk'] and not disposition['retry_executed'])
 for kind in ('docx-contract','fx-journals','business-drafts','derived-retirement'):
  candidates=[p for p in (E.parent).glob('batch-round-158-'+kind+'-*/result.json') if read(p).get('passed')]
  if not candidates:raise ValueError('Current actual browser check missing: '+kind)
  p=max(candidates,key=lambda p:read(p)['finished_at']);v=record(p);capture=record(p.parent/'encoded-media.json');manifest=record(p.parent/'source-manifest.json');h=hashlib.sha256()
  controller=record(E/'browser-fixes-3.json');archived=ROOT/controller['retained_controller'];old_controller=archived.read_bytes();new_controller=(APP/'scripts/check_batch_round158.mjs').read_bytes()
  compatible=old_controller.count(controller['old_statement'].encode())==1 and old_controller.replace(controller['old_statement'].encode(),controller['new_statement'].encode(),1)==new_controller
  use_archived=p.parent.name in controller['retained_passed_capture_groups'] and kind!='derived-retirement'
  if use_archived:
   records[archived.relative_to(ROOT).as_posix()]=sha(archived)
   check(kind+' retained exact controller differs only in the unrelated derived-version selector',compatible)
  for name in manifest['files']:
   data=old_controller if use_archived and name=='scripts/check_batch_round158.mjs' else (APP/name).read_bytes()
   h.update(name.encode()+b'\0'+data+b'\0')
  check(kind+' actual UI and source binding',h.hexdigest()==manifest['sha256']==v['source_sha256'] and v['source_unchanged'] and v['build_source_unchanged'] and v['owned_browser_closed'] and v['owned_review_server_stopped'] and v['isolated_database_cleanup_passed'] and v['native_fixture_exit_code']==0 and bool(v['checks']))
  check(kind+' screenshot/recording/GIF hashes match',capture['passed'] and not capture['production_acceptance'] and all(sha(ROOT/i['file'])==i['sha256'] for i in capture['screenshots']+capture['artifacts']))
 old_rules=re.search(r'ADD CONSTRAINT agent_tool_receipts_tool_check CHECK\(tool IN \(([^;]+)\)\);',(APP/'backend/migrations/0114_skill_release_governance.sql').read_text()).group(1)
 new_rules=re.search(r'ADD CONSTRAINT agent_tool_receipts_tool_check CHECK\(tool IN \(([^;]+)\)\);',(APP/'backend/migrations/0119_agent_business_drafts.sql').read_text()).group(1)
 check('New tool whitelist preserves every earlier tool and adds exactly the fixed draft command',set(re.findall("'([^']+)'",new_rules))==set(re.findall("'([^']+)'",old_rules))|{'create_business_draft'})
 visual=record(E/'visual-review.json');check('Inspected current screenshots and GIFs match bytes',visual['passed'] and len(visual['files'])>=4 and not visual['production_acceptance'] and all(sha(ROOT/i['file'])==i['sha256'] for i in visual['files']))
 changed=subprocess.check_output(['git','diff','--name-only',BASE,'--','app/backend/migrations','docs/quality/evidence','docs/media'],cwd=ROOT).decode().splitlines();forbidden=[]
 for name in changed:
  if name.startswith('app/backend/migrations/'):
   if int(Path(name).name.split('_')[0])<=117:forbidden.append(name)
  elif name!='docs/media/README.md' and not any(name.startswith(p) for p in ('docs/quality/evidence/batch-round-158','docs/media/batch-round-158/')):forbidden.append(name)
 check('Migrations1-117 and all earlier evidence/media stay immutable',not forbidden)
 tags=subprocess.check_output(['git','tag','--list','v0.1.0-rc.*'],cwd=ROOT).decode().splitlines();rc8=subprocess.check_output(['git','rev-parse','v0.1.0-rc.8^{commit}'],cwd=ROOT).decode().strip()
 check('Eight previous freezes stay fixed',len(tags)==8 and rc8=='160d78f8032be21c9eab72fc28415fda598fb7df')
 scope=read(ROOT/'docs/delivery/04-current-scope.json');check('23 active packages and five exclusions retained',len([i for i in scope['items'] if i['status']=='active'])==23 and len([i for i in scope['items'] if i['status']!='active'])==5)
 return {'passed':all(i['passed'] for i in checks),'checks':checks,'records':records,'interaction_source_sha256':current,'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False,'virtualization_used':False,'new_android_apk':False,'native_result_records':len(result),'current_four_client_builds':False,'current_client_builds':['Web','WeChat','Douyin'],'current_android_js_build_passed':False,'deferred_checks':['android-bundle'],'limited_release_scope':True,'retained':{'full_backend_increment':121,'development_android_increment':154,'development_android_source_increment':153,'development_android_schema':113}}
if __name__=='__main__':
 v=verify();(E/'final-source-check.json').write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');print(json.dumps({'passed':v['passed'],'checks':len(v['checks']),'failed':[i['name'] for i in v['checks'] if not i['passed']]}));raise SystemExit(0 if v['passed'] else 1)
