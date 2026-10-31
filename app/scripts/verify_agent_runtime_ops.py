"""Read-only proof inspection; CLI writes only its own explicit final report."""
import hashlib,json,subprocess
from agent_runtime_ops_sources import APP,ROOT,sources,digest,CLIENT_SCOPES
E=ROOT/'docs/quality/evidence/agent-runtime-operations'
def read(p):return json.loads(p.read_text(encoding='utf-8'))
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def verify():
 checks=[]
 def check(n,v):checks.append({'name':n,'passed':bool(v)})
 native=read(E/'native-final.json')
 check('selected native HTTP and execution suites actually ran at current backend sources',native['passed'] and native['exit_code']==0 and native['source_sha256']==native['source_after_sha256']==digest(native['source_files']) and any(t['Test']=='TestAgentRuntimeOperations' and t['Action']=='pass' for t in native['test_results']) and any(t['Test']=='TestAgentExecutionController' and t['Action']=='pass' for t in native['test_results']) and not any(t['Action'] in ('fail','skip') for t in native['test_results']))
 for step in ('typecheck','web-build','weapp-build','douyin-build'):
  candidates=sorted(E.glob(step+'-*.json'),key=lambda p:int(p.stem.rsplit('-',1)[1]));r=read(candidates[-1]);check(step+' current portable client sources',r['passed'] and r['source_unchanged'] and r['source_sha256']==digest(r['source_files']) and not r['new_android_apk'])
 browser=read(E.parent/'agent-runtime-operations-web/result.json');manifest=read(E.parent/'agent-runtime-operations-web/source-manifest.json')
 check('actual Web inventory retirement readback with all owned resources closed',browser['passed'] and len(browser['checks'])==3 and browser['source_unchanged'] and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'] and browser['native_fixture_exit_code']==0 and browser['source_sha256']==digest(manifest['files']) and browser['build_source_sha256']==digest())
 media=read(E/'media.json');check('current screenshot video GIF hashes',media['passed'] and all(sha(ROOT/v['file'])==v['sha256'] for v in media['artifacts']))
 check('visual review matches current media',read(E/'visual-review.json')['artifacts']==media['artifacts'])
 check('acceptance applicability remains explicit',read(E/'acceptance-applicability-check.json')['passed'])
 changed=subprocess.check_output(['git','diff','85c0bc407b5769b6c81e0275df5ff84a799f933c','--name-only'],cwd=ROOT).decode().splitlines()
 check('older evidence media and applied migrations remain unchanged',not any(n.startswith(('docs/quality/evidence/','docs/media/screenshots/','docs/media/videos/','docs/media/gifs/')) and 'agent-runtime-operations' not in n for n in changed) and not any(n.startswith('app/backend/migrations/') and not n.endswith('0107_agent_runtime_operations.sql') for n in changed))
 check('no private fixture or native cluster remains owned',not (APP/'.local/business-browser-fixture.json').exists() and not (APP/'.local/agent-runtime-operations-native-cluster.json').exists() and not (APP/'.local/agent-runtime-operations-browser-cluster.json').exists())
 check('frozen candidates retain exact business commits',subprocess.check_output(['git','rev-parse','v0.1.0-rc.1^{}','v0.1.0-rc.2^{}'],cwd=ROOT).decode().splitlines()==['a95c6afbd0e76684a64dc641c6abe598ad625277','40cd118634117204e2e310395fac32b59af5cecc'])
 check('eight Skill manifests remain draft',all(json.loads(p.read_text(encoding='utf-8'))['state']=='draft' for p in (APP/'skills').glob('*/manifest.json')) and len(list((APP/'skills').glob('*/manifest.json')))==8)
 record_names=['native-final.json','media.json','visual-review.json','native-cluster-closed.json','acceptance-applicability-check.json']
 record_names += [sorted(E.glob(step+'-*.json'),key=lambda p:int(p.stem.rsplit('-',1)[1]))[-1].name for step in ('typecheck','web-build','weapp-build','douyin-build')]
 paths=[E/n for n in record_names]+[E.parent/'agent-runtime-operations-web'/n for n in ('result.json','source-manifest.json','business-proof.json')]+[E.parent/'telegram-inbox/final-source-check.json']
 records={p.relative_to(ROOT).as_posix():sha(p) for p in paths}
 retained={'base_commit':'85c0bc407b5769b6c81e0275df5ff84a799f933c','full_backend_increment':121,'runtime_increment':121,'business_full_client_increment':136,'android_artifact_increment':137,'android_business_source_increment':136,'android_schema':103,'previous_proof_and_media_reexecuted':False,'current_android_apk_built':False}
 return {'passed':all(c['passed'] for c in checks),'checks':checks,'records':records,'retained':retained,'interaction_source_sha256':digest(),'verification_script_sha256':sha(__import__('pathlib').Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False,'new_android_apk':False,'full_regression':False,'virtualization_used':False}
if __name__=='__main__':
 r=verify();(E/'final-source-check.json').write_text(json.dumps(r,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');print(json.dumps(r,ensure_ascii=False));raise SystemExit(0 if r['passed'] else 1)
