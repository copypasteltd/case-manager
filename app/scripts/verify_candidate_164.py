"""Verify fresh matching artifacts, exact retained whole-wave proof and immutable refs."""
import hashlib,json,subprocess
from pathlib import Path
import build_production_release as server
import verify_batch_round163 as business
from notification_telegram_sources import digest as client_digest
from seal_candidate_164 import OUT
from release_archive_parts164 import verify_parts
from build_candidate_164 import APP,ROOT,E,SOURCE,VERSION,sha
RELEASE=ROOT/'docs/releases'/VERSION
read=lambda p:json.loads(p.read_text(encoding='utf8'))
def git(*args):return subprocess.check_output(['git',*args],cwd=ROOT)
def digest():
 h=hashlib.sha256();h.update(business.digest().encode())
 for n in ('build_candidate_164.py','build_candidate_164_server.py','inspect_candidate_164.py','seal_candidate_164.py','publish_candidate_164.py','prepare_candidate_164.py','verify_candidate_164.py'):h.update(n.encode()+b'\0'+(APP/'scripts'/n).read_bytes()+b'\0')
 return h.hexdigest()
def verify():
 checks=[];records={}
 def check(n,v):checks.append({'name':n,'passed':bool(v)})
 def record(p):records[p.relative_to(ROOT).as_posix()]=sha(p);return read(p)
 lock=record(E/'source-lock.json');prior=record(E/'retained-functional-proof.json');current=business.verify()
 original=ROOT/'docs/quality/evidence/batch-round-163/final-source-check.json'
 adjustment=record(E/'inspection-only-source-adjustment.json');allowed={'app/scripts/inspect_candidate_164.py','app/scripts/verify_candidate_164.py'}
 assert {r['file'] for r in adjustment['files']}==allowed
 normalized=hashlib.sha256()
 for name in business.source_names():
  data=(ROOT/name).read_bytes()
  if name in allowed:
   row=next(r for r in adjustment['files'] if r['file']==name);frozen=git('show',SOURCE+':'+name)
   assert hashlib.sha256(data).hexdigest()==row['current_sha256'] and hashlib.sha256(frozen).hexdigest()==row['frozen_sha256']
   data=frozen
  normalized.update(name.encode()+b'\0'+data+b'\0')
 check('exact163_business_proof_retained_without_replay',prior==read(original) and current['passed'] and current['checks']==prior['checks'] and current['records']==prior['records'] and normalized.hexdigest()==prior['interaction_source_sha256'] and git('show',SOURCE+':'+original.relative_to(ROOT).as_posix())==original.read_bytes())
 smoke=record(E/'retained-native-proof.json');clients=record(E/'retained-client-proof.json');summary=record(E/'retained-test-summary.json')
 check('selected_native_client_results_and_counts_retained_exactly',smoke['passed'] and clients['passed'] and summary['passed'] and all((ROOT/name).read_bytes()==(E/target).read_bytes() for target,name in lock['retained_files'].items()))
 m=record(RELEASE/'release-manifest.json');s=record(RELEASE/'server-build.json');scope=record(E/'server-build-scope.json');seal=record(E/'seal.json')
 check('fresh_native_server_web_runtime_match_frozen_schema129_source',m['source']['commit']==SOURCE==lock['source_commit'] and m['schema_version']==s['migrations']==129 and s['passed'] and scope['frozen_input_bytes_verified'] and s['source']['inputs_sha256']==server.digest(server.canonical(server.inputs())) and all(r['exit_code']==0 for r in s['commands']))
 mini=record(E/'miniapps-build.json');mi=record(E/'miniapps-inspection.json');android=record(E/'build.json');ai=record(E/'apk-inspection.json');native=record(E/'android-build-scope.json')
 check('fresh_miniapps_and_current_android_sources_pass_actual_artifact_inspection',mini['passed'] and mini['source_commit']==SOURCE and mini['source_unchanged'] and mini['source_sha256']==client_digest(mini['source_files']) and mi['passed'] and len(mi['checks'])==2 and all(v['passed'] for v in mi['checks']) and android['passed'] and android['source_unchanged_during_build'] and ai['passed'] and all(v['passed'] for v in ai['checks']) and native['passed'] and ai['apk_sha256']==android['artifacts']['apk']['sha256'])
 check('actual_cached_or_executed_JVM_outcome_remains_explicit',native['test_count']==28 and m['android']['native_test_cached']==native['cached'] and m['android']['new_native_test_execution']==(not native['cached']) and not m['android']['standalone_jvm_rerun'])
 check('all_five_archives_sealed_and_complete_source_reconstructed',len(m['assets'])==4+len(m['media_archive']['parts']) and verify_parts(OUT,m['media_archive']) and m['assets']==seal['assets'] and all(sha(OUT/v['name'])==v['sha256'] and (OUT/v['name']).stat().st_size==v['bytes'] and git('hash-object','--no-filters',str(OUT/v['name'])).decode().strip()==v['git_blob'] for v in m['assets']) and seal['passed'] and seal['manifest_sha256']==sha(RELEASE/'release-manifest.json') and m['source_archive']['complete_application_source'] and m['source_archive']['all_packaged_blob_bytes_match_git_source'] and s['private_material_check']['passed'] and not s['private_material_check']['matches'])
 pub=record(RELEASE/'git-publication.json')
 check('new_immutable_source_and_artifact_refs_fix_the_sealed_assets',git('rev-parse',VERSION+'^{}').decode().strip()==SOURCE and git('rev-parse','codex/release-'+VERSION).decode().strip()==pub['artifact_commit'] and pub['manifest_sha256']==sha(RELEASE/'release-manifest.json') and all(git('rev-parse',pub['artifact_commit']+':docs/releases/'+VERSION+'/'+v['name']).decode().strip()==v['git_blob'] for v in m['assets']))
 check('all_eight_earlier_freezes_preserve_exact_refs',len(m['prior_freezes_unchanged'])==8 and m['prior_freezes_unchanged']==lock['prior_freezes'] and all(git('rev-parse',v['tag']+'^{}').decode().strip()==v['source_commit'] and git('rev-parse','codex/release-'+v['tag']).decode().strip()==v['artifact_commit'] for v in lock['prior_freezes']))
 protected=git('diff',SOURCE,'--name-only','--diff-filter=MD','--','docs/quality/evidence','docs/media','app/backend/migrations').decode().splitlines()
 check('all163_and_earlier_proof_media_migration_bytes_retained',not protected)
 check('honest_formal_boundaries_and_all23_open_packages',m['remaining_work_packages']==23 and m['eight_skills_status']=='draft' and not m['production_ready'] and not m['deployed'] and not m['github_release_page_created'])
 return {'passed':all(v['passed'] for v in checks),'checks':checks,'records':records,'retained':{'business_source_increment':163,'business_source_commit':SOURCE,'business_tests_reexecuted':False,'full_backend_increment':121,'full_client_increment':136,'test_summary_sha256':sha(E/'retained-test-summary.json')},'interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False,'new_android_apk':True,'virtualization_used':False,'inspection_only_adjustment':{'record':(E/'inspection-only-source-adjustment.json').relative_to(ROOT).as_posix(),'sha256':sha(E/'inspection-only-source-adjustment.json'),'application_source_unchanged':True}}
if __name__=='__main__':
 r=verify();p=E/'final-source-check.json';assert not p.exists();p.write_text(json.dumps(r,ensure_ascii=False,indent=2)+'\n',encoding='utf8');print(json.dumps({'passed':r['passed'],'checks':len(r['checks']),'failed':[v['name'] for v in r['checks'] if not v['passed']]}));raise SystemExit(0 if r['passed'] else 1)
