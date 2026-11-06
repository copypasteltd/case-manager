"""Verify the completed whole code wave against exact executed consumer inputs."""
import argparse,hashlib,json,subprocess,zipfile
from pathlib import Path
from run_batch_round163 import APP,ROOT,EVIDENCE,load_plan,source_names,source_digest,write_once
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def digest():return source_digest(source_names())
def verify():
 inputs=json.loads((EVIDENCE/'verification-inputs.json').read_text(encoding='utf8'));plan=load_plan();checks=[];records={}
 def check(n,v):checks.append({'name':n,'passed':bool(v)})
 def read(p):records[p.relative_to(ROOT).as_posix()]=sha(p);return json.loads(p.read_text(encoding='utf8'))
 initial=read(EVIDENCE/'freeze-code-initial.json');archive=read(EVIDENCE/'code-wave-initial-archive.json')
 with zipfile.ZipFile(ROOT/archive['file']) as z:files={n:hashlib.sha256(z.read(n)).hexdigest() for n in z.namelist()}
 check('whole_declared_code_and_all23_dispositions_precede_checks',not initial['tests_executed'] and len(initial['reviewed_package_ids'])==23 and len(plan['round_tasks'])==7 and all(i['state']=='implemented' for i in plan['round_tasks']))
 check('pretest_archive_preserves_exact_whole_wave',sha(ROOT/archive['file'])==archive['sha256'] and files==initial['source_file_sha256'])
 for label,attempt,prefixes in [('native',inputs['native'],('app/backend/',)),('client',inputs['clients'],('app/web/','app/mobile/','app/packages/','app/agent-runtime/','app/skills/','docs/skills/'))]:
  record=read(EVIDENCE/(('native-tests-' if label=='native' else 'client-checks-')+attempt+'.json'))
  actual={n:sha(ROOT/n) for n in source_names() if n.startswith(prefixes)}
  tested={n:v for n,v in record['source_file_sha256'].items() if n.startswith(prefixes)}
  check(label+'_selected_execution_passed',record['passed'] and record['tests_executed'] and record['source_unchanged'])
  check(label+'_actual_consumer_bytes_current',actual==tested)
  check(label+'_starts_after_whole_code_wave',record['started_at']>=initial['started_at'])
  if label=='native':check('file_original_session_family_executed',any(v.get('Test')=='TestRound163FileOriginalSessions' and v['Action']=='pass' for v in record['test_results']))
  else:check('complete_selected_client_wave_executed',len(record['test_results'])==9 and all(v['passed'] for v in record['test_results']))
 summary=read(EVIDENCE/'test-summary.json');check('counts_refer_to_selected_passed_records',summary['passed'] and summary['native_record']=='native-tests-'+inputs['native']+'.json' and summary['client_record']=='client-checks-'+inputs['clients']+'.json' and summary['web_tests']>0 and summary['shared_mobile_tests']>0 and summary['runtime_tests']>0)
 for client,group in [('web',inputs['web']),('mobile',inputs['mobile'])]:
  e=ROOT/'docs/quality/evidence'/group;report=read(e/'result.json');manifest=read(e/'source-manifest.json')
  check(client+'_capture_and_owned_cleanup',all(report[k] for k in ('passed','source_unchanged','build_source_unchanged','owned_browser_closed','owned_review_server_stopped','isolated_database_cleanup_passed')) and report['native_fixture_exit_code']==0)
  h=hashlib.sha256()
  for name in manifest['files']:h.update(name.encode()+b'\0'+(APP/name).read_bytes()+b'\0')
  check(client+'_actual_capture_inputs_current',h.hexdigest()==manifest['sha256']==report['source_sha256'])
  encoded=read(e/'encoded-media.json');interaction=read(e/'interaction-media.json')
  check(client+'_actual_media_bytes',encoded['passed'] and interaction['passed'] and len(encoded['screenshots'])==2 and all(sha(ROOT/v['file'])==v['sha256'] for v in encoded['screenshots']+encoded['artifacts']+[interaction]))
 review=read(EVIDENCE/'visual-review.json');check('primary_screenshots_and_gif_frames_reviewed',review['reviewed'] and len(review['images'])==6 and all(sha(ROOT/v['file'])==v['sha256'] for v in review['images']))
 parts=read(EVIDENCE/'media-parts-check.json');check('lossless_media_parts_and_rejection_checks_passed',parts['passed'] and len(parts['checks'])==7 and parts['helper_sha256']==sha(APP/'scripts/release_archive_parts164.py') and parts['script_sha256']==sha(APP/'scripts/test_release_archive_parts164.py'))
 scope=json.loads((ROOT/'docs/delivery/04-current-scope.json').read_text(encoding='utf8'))
 check('all23_formal_packages_and_eight_draft_skills_remain_open',sum(i['status']=='active' for i in scope['items'])==23 and all(i['acceptance_state']=='open' for i in plan['packages']) and all(json.loads(p.read_text(encoding='utf8'))['state']=='draft' for p in (APP/'skills').glob('*/manifest.json')))
 changed=subprocess.check_output(['git','diff','--name-only','--diff-filter=MD','3ad8684a3574237c497f8a8ba470cd15f2a27e73','--','docs/quality/evidence','docs/media','app/backend/migrations'],cwd=ROOT,text=True).splitlines()
 check('prior_proofs_media_and_migrations_preserved',not [n for n in changed if n!='docs/media/README.md'] and len(list((APP/'backend/migrations').glob('*.sql')))==129)
 return {'passed':all(v['passed'] for v in checks),'checks':checks,'records':records,'retained':{'prior_source':'3ad8684a3574237c497f8a8ba470cd15f2a27e73','apk':'154/business153/schema113','full_regression':'121'},'increment':163,'schema':129,'interaction_source_sha256':digest(),'source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False,'real_integrations_executed':False}
def main():
 p=argparse.ArgumentParser()
 for n in ('native','clients','web','mobile'):p.add_argument('--'+n)
 p.add_argument('--attempt',default='final');a=p.parse_args();values={n:getattr(a,n) for n in ('native','clients','web','mobile')};target=EVIDENCE/'verification-inputs.json'
 if any(values.values()):
  assert all(values.values())
  if target.exists():assert json.loads(target.read_text(encoding='utf8'))==values
  else:write_once(target,values)
 result=verify();write_once(EVIDENCE/('scope-check-'+a.attempt+'.json'),result)
 if result['passed'] and not (EVIDENCE/'final-source-check.json').exists():write_once(EVIDENCE/'final-source-check.json',result)
 print(json.dumps({'passed':result['passed'],'checks':len(result['checks']),'failed':[v['name'] for v in result['checks'] if not v['passed']]}));return 0 if result['passed'] else 1
if __name__=='__main__':raise SystemExit(main())
