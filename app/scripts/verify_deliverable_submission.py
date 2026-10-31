"""Exact focused151 source/evidence check; frozen and production acceptance stay distinct."""
import hashlib,json,subprocess
from pathlib import Path
from deliverable_submission_sources import APP,ROOT,digest,sources
E=ROOT/'docs/quality/evidence/deliverable-submission'
BASE='fd304e1d3e23f84b13fd3a3142703e9300206a4f'
STEPS=('web-deliverable-submission-tests','mobile-deliverable-submission-compile','mobile-deliverable-submission-tests','runtime-tests','typecheck','web-build','weapp-build','douyin-build')
read=lambda p:json.loads(p.read_text(encoding='utf-8'))
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def git(*args):return subprocess.check_output(['git',*args],cwd=ROOT)
def verify(include_scope=True):
 checks=[];paths=[]
 def check(name,v):checks.append({'name':name,'passed':bool(v)})
 def record(p):paths.append(p);return read(p)
 n=record(E/'native-final.json')
 check('Selected current native HTTP/PG suites pass with fixed sources',n['passed'] and n['source_sha256']==n['source_after_sha256']==digest(n['source_files']) and all(v['Action']=='pass' for v in n['test_results']))
 names={v['Test'] for v in n['test_results']}
 check('All four actual session channels submit once and replay the original receipt',all('TestAgentDeliverableSubmission/'+c+'_commits_one_fixed_revision_and_replays_once' in names for c in ('web','wechat','douyin','android')))
 check('Exact fields, old basis, revoked session and independent human acceptance covered','TestAgentDeliverableSubmission/stale_head_and_revoked_original_session_never_write' in names and 'TestProjectDeliverables/automatic_actors_cannot_record_a_human_acceptance' in names and 'TestAgentDeliverableUpdates' in names)
 for step in STEPS:
  p=sorted(E.glob(step+'-*.json'),key=lambda p:int(p.stem.rsplit('-',1)[1]))[-1];r=record(p)
  check(step+' matches current declared client sources',r['passed'] and r['source_unchanged'] and r['source_sha256']==r['source_after_sha256']==digest(r['source_files']))
 browser=record(E.parent/'deliverable-submission-web/result.json');manifest=record(E.parent/'deliverable-submission-web/source-manifest.json')
 check('Actual Web input and queued intent close owned contexts and native fixture',browser['passed'] and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'] and browser['source_sha256']==manifest['sha256']==digest(manifest['files']))
 media=record(E/'media.json');review=record(E/'visual-review.json')
 check('Current screenshots, actual-pixel MP4 and disclosed screenshot GIF match hashes',media['passed'] and all(sha(ROOT/a['file'])==a['sha256'] for a in media['artifacts']) and review['passed'] and review['artifacts']==media['artifacts'])
 releases=record(ROOT/'docs/releases/v0.1.0-rc.6/release-manifest.json')['prior_freezes_unchanged']+[{'tag':'v0.1.0-rc.6','source_commit':'c49177e6f9f26731ce1a2f2183e2c9020b2c4d3d','artifact_commit':'4a35ed903559438501153bcdbc6374ed30b528d8'}]
 check('All six prior annotated source and artifact refs remain unchanged',all(git('rev-parse',r['tag']+'^{}').decode().strip()==r['source_commit'] and git('rev-parse','codex/release-'+r['tag']).decode().strip()==r['artifact_commit'] for r in releases))
 changed=git('diff',BASE,'--name-only').decode().splitlines()
 protected=[p for p in changed if p.startswith(('docs/quality/evidence/','docs/media/screenshots/','docs/media/videos/','docs/media/gifs/','docs/releases/','app/backend/migrations/')) and 'deliverable-submission' not in p and '0112_agent_deliverable_submission.sql' not in p]
 check('Prior proof/media/releases and migrations1-111 keep their exact bytes',not protected)
 check('Eight Skills remain draft and ordinary evidence cleanup is complete',len(list((APP/'skills').glob('*/manifest.json')))==8 and all(read(p)['state']=='draft' for p in (APP/'skills').glob('*/manifest.json')) and not any((APP/'.local'/p).exists() for p in ('business-browser-fixture.json','deliverable-submission-native-cluster.json','deliverable-submission-browser-cluster.json')))
 for name in (('scope-check.json','acceptance-applicability-check.json') if include_scope else ()):
  r=record(E/name);check(name+' passes without counting business cases',r['passed'] and (name!='acceptance-applicability-check.json' or r['business_cases_passed']==0))
 attempts=record(E/'attempts.json');check('Failed development attempts remain disclosed',attempts['passed'] and attempts['failed_attempts_count']>=4)
 return {'passed':all(c['passed'] for c in checks),'checks':checks,'records':{p.relative_to(ROOT).as_posix():sha(p) for p in paths},'retained':{'base_commit':BASE,'full_backend_increment':121,'full_client_increment':136,'android_artifact_increment':150,'android_business_increment':149,'android_schema':111,'previous_proof_and_media_reexecuted':False},'interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False,'virtualization_used':False,'new_android_apk':False}
if __name__=='__main__':
 import sys
 r=verify('--prepare' not in sys.argv);(E/'final-source-check.json').write_text(json.dumps(r,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');print(json.dumps({'passed':r['passed'],'checks':len(r['checks']),'failed':[c['name'] for c in r['checks'] if not c['passed']]}));raise SystemExit(0 if r['passed'] else 1)
