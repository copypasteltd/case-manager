"""Independent152 native frozen-artifact check; original151 proof remains retained."""
import hashlib,json,subprocess
from pathlib import Path
import build_production_release as server
from build_candidate_152 import APP,ROOT,E,SOURCE,VERSION,sha
from deliverable_submission_sources import digest as business_digest
RELEASE=ROOT/'docs/releases'/VERSION
read=lambda p:json.loads(p.read_text(encoding='utf-8'))
def git(*args):return subprocess.check_output(['git',*args],cwd=ROOT)
def digest():
 h=hashlib.sha256();h.update(business_digest().encode())
 for name in ('build_candidate_152.py','build_candidate_152_server.py','inspect_candidate_152.py','seal_candidate_152.py','publish_candidate_152.py','verify_candidate_152.py'):
  h.update(name.encode()+b'\0'+(APP/'scripts'/name).read_bytes()+b'\0')
 return h.hexdigest()
def verify(include_scope=True):
 checks=[];paths=[]
 def check(name,v):checks.append({'name':name,'passed':bool(v)})
 def record(p):paths.append(p);return read(p)
 prior=record(E/'retained-functional-proof.json');original=ROOT/'docs/quality/evidence/deliverable-submission/final-source-check.json'
 check('Exact151 selected business/runtime/client/browser/media proof retained without replay',prior==read(original) and prior['passed'] and prior['interaction_source_sha256']==business_digest() and all(sha(ROOT/n)==v for n,v in prior['records'].items()) and git('show',SOURCE+':'+original.relative_to(ROOT).as_posix())==original.read_bytes())
 smoke=record(E/'retained-native-proof.json');check('Three current151 native suites and36 records retain exact source binding',smoke==read(ROOT/'docs/quality/evidence/deliverable-submission/native-final.json') and smoke['passed'] and len(smoke['test_results'])==36 and all(r['Action']=='pass' for r in smoke['test_results']) and smoke['source_sha256']==business_digest(smoke['source_files']))
 m=record(RELEASE/'release-manifest.json');s=record(RELEASE/'server-build.json');seal=record(E/'seal.json');scope=record(E/'server-build-scope.json')
 check('Fresh Linux/Web/runtime inputs match frozen source151 and schema112',m['source']['commit']==SOURCE and m['schema_version']==112 and s['passed'] and s['migrations']==112 and scope['frozen_input_bytes_verified'] and s['source']['inputs_sha256']==server.digest(server.canonical(server.inputs())) and all(r['exit_code']==0 for r in s['commands']))
 mini=record(E/'miniapps-build.json');mi=record(E/'miniapps-inspection.json');android=record(E/'build.json');ai=record(E/'apk-inspection.json');native=record(E/'android-build-scope.json')
 check('Fresh WeChat and Douyin outputs contain submission and replica recovery',mini['passed'] and mi['passed'] and len(mi['checks'])==2 and all(c['passed'] for c in mi['checks']) and mini['source_sha256']==business_digest(mini['source_files']))
 check('Fresh development APK has current mapped sources and valid native inspection',android['passed'] and ai['passed'] and len(ai['checks'])==10 and all(c['passed'] for c in ai['checks']) and native['passed'] and ai['apk_sha256']==android['artifacts']['apk']['sha256'])
 check('Actual native JVM task outcome and cached28 XML results remain explicit',native['test_count']==28 and m['android']['native_test_cached']==native['cached'] and m['android']['new_native_test_execution']==(not native['cached']) and not m['android']['standalone_jvm_rerun'])
 check('Five archive assets reconstruct complete application Git blobs with expanded private scan',len(m['assets'])==5 and seal['passed'] and seal['manifest_sha256']==sha(RELEASE/'release-manifest.json') and m['source_archive']['complete_application_source'] and m['source_archive']['all_packaged_blob_bytes_match_git_source'] and s['private_material_check']['passed'] and s['private_material_check']['matches']==[])
 pub=record(RELEASE/'git-publication.json')
 check('New annotated source and artifact refs fix sealed manifests and asset hashes',git('rev-parse',VERSION+'^{}').decode().strip()==SOURCE and git('rev-parse','codex/release-'+VERSION).decode().strip()==pub['artifact_commit'] and pub['manifest_sha256']==sha(RELEASE/'release-manifest.json') and all(git('rev-parse',pub['artifact_commit']+':docs/releases/'+VERSION+'/'+a['name']).decode().strip()==a['git_blob'] for a in m['assets']))
 check('All six earlier source/artifact refs remain immutable',len(m['prior_freezes_unchanged'])==6 and all(git('rev-parse',r['tag']+'^{}').decode().strip()==r['source_commit'] and git('rev-parse','codex/release-'+r['tag']).decode().strip()==r['artifact_commit'] for r in m['prior_freezes_unchanged']))
 protected=[p for p in git('diff',SOURCE,'--name-only').decode().splitlines() if p.startswith(('app/backend/migrations/','docs/quality/evidence/','docs/media/screenshots/','docs/media/videos/','docs/media/gifs/','docs/releases/')) and not p.startswith(('docs/quality/evidence/release-candidate-152/','docs/releases/'+VERSION+'/'))]
 check('Prior proof/media and112 installed migration files preserve exact bytes',not protected)
 for name in (('scope-check.json','acceptance-applicability-check.json') if include_scope else ()):
  r=record(E/name);check(name+' passes without business-case claims',r['passed'] and (name!='acceptance-applicability-check.json' or r['business_cases_passed']==0))
 check('Reduced validation and unresolved production scope stay disclosed',m['remaining_work_packages']==23 and m['eight_skills_status']=='draft' and not m['production_ready'] and not m['deployed'] and not m['github_release_page_created'])
 return {'passed':all(c['passed'] for c in checks),'checks':checks,'records':{p.relative_to(ROOT).as_posix():sha(p) for p in paths},'retained':{'business_source_increment':151,'business_source_commit':SOURCE,'business_tests_reexecuted':False,'full_backend_increment':121,'full_client_increment':136,'runtime_tests_executed_increment':151,'runtime_tests':58},'interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False,'new_android_apk':True,'virtualization_used':False}
if __name__=='__main__':
 import sys
 r=verify('--prepare' not in sys.argv);(E/'final-source-check.json').write_text(json.dumps(r,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');print(json.dumps({'passed':r['passed'],'checks':len(r['checks']),'failed':[c['name'] for c in r['checks'] if not c['passed']]}));raise SystemExit(0 if r['passed'] else 1)
