"""Verify current source-bound local evidence without changing formal acceptance."""
import hashlib,json,re,subprocess,zipfile
from pathlib import Path
from run_batch_round167 import APP,ROOT,EVIDENCE,load_plan,source_names,source_digest,write_once

def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def read(path):return json.loads(path.read_text(encoding='utf-8'))
def digest():return source_digest(source_names())
def current(files):return bool(files) and all((ROOT/name).is_file() and sha(ROOT/name)==value for name,value in files.items())

def verify():
    checks=[];records=[]
    def check(name,condition):checks.append({'name':name,'passed':bool(condition)})
    def record(path):records.append({'file':path.relative_to(ROOT).as_posix(),'sha256':sha(path)});return read(path)
    plan=load_plan();inputs=record(EVIDENCE/'verification-inputs.json')
    scope=read(ROOT/'docs/delivery/04-current-scope.json')
    check('complete23_code_inventory_and_five_exclusions',len(plan['packages'])==23 and all(p['acceptance_state']=='open' and p['current_remaining'] for p in plan['packages']) and len([v for v in scope['items'] if v['status']=='active'])==23)
    frozen=record(EVIDENCE/'freeze-code-initial.json');archive=record(EVIDENCE/'code-wave-initial-archive.json')
    with zipfile.ZipFile(ROOT/archive['file']) as saved:
        original={n:hashlib.sha256(saved.read(n)).hexdigest() for n in saved.namelist()}
    check('full_code_and_tools_archived_before_testing',not frozen['tests_executed'] and frozen['passed'] and sha(ROOT/archive['file'])==archive['sha256'] and original==frozen['source_file_sha256'])
    native=record(EVIDENCE/('native-tests-'+inputs['native']+'.json'))
    adjustment=record(EVIDENCE/'fixture-only-repair.json')
    with zipfile.ZipFile(ROOT/adjustment['initial_source_archive']) as saved:
        old_fixture=hashlib.sha256(saved.read(adjustment['file'])).hexdigest()
    fixture_exact=(adjustment['file']=='app/backend/internal/httpapi/round167_browser_integration_test.go' and old_fixture==adjustment['before_sha256']==native['source_file_sha256'][adjustment['file']] and sha(ROOT/adjustment['file'])==adjustment['after_sha256'] and not adjustment['application_sources_changed'])
    native_current=fixture_exact and all(sha(ROOT/name)==value for name,value in native['source_file_sha256'].items() if name!=adjustment['file'])
    check('native_business_passes_with_exact_disclosed_browser_fixture_only_repair',native['passed'] and native['source_unchanged'] and native_current and native['started_at']>=frozen['started_at'] and all(v['Action']=='pass' for v in native['test_results']) and not native['full_backend_suite_executed'])
    summary=record(EVIDENCE/'test-summary.json')
    client_steps=summary['client_steps'];expected={'typecheck','web-tests','mobile-tests','mobile-new-contracts','web-build','weapp-build','douyin-build'}
    check('all_seven_current_client_consumers_pass',summary['passed'] and set(client_steps)==expected and all(v['passed'] and current(v['source_file_sha256']) for v in client_steps.values()))
    engineering={}
    for name,attempt in inputs['engineering'].items():
        evidence=record(EVIDENCE/('engineering-'+attempt+'.json'))
        step=next(v for v in evidence['steps'] if v['step']==name);engineering[name]=step
    check('current_controlled_release_and_jvm_and_packager_pass',set(engineering)=={'release','jvm','production'} and all(v['passed'] and current(v['source_file_sha256']) for v in engineering.values()))
    jvm=record(EVIDENCE/('jvm-'+inputs['engineering']['jvm']+'.json'))
    check('native_java_full_source_tests_are_fresh_and_bounded',jvm['passed'] and current({'app/'+k:v for k,v in jvm['source_files'].items()}) and jvm['test_count']>0 and not jvm['android_runtime_executed'] and not jvm['apk_built'])
    package=record(EVIDENCE/('production-'+inputs['engineering']['production']+'.json'))
    check('actual_native_artifact_has_all_four_standalone_modules',package['passed'] and package['installed_cli_exit_code']==0 and set(package['installed_modules'])=={'bundle.py','catalog.py','controller.py','telegram_config.py'} and all(sha(APP/'deploy/release'/k)==v for k,v in package['installed_modules'].items()) and package['all_controlled_units_match'] and package['schema']==133 and not package['native_linux_executed'])
    check('artifact_source_and_archive_are_exact',current({row['path']:row['sha256'] for row in package['build']['source_files']}) and sha(ROOT/package['build']['archive'])==package['build']['archive_sha256'])
    for group in inputs['browsers']:
        base=ROOT/'docs/quality/evidence'/group;browser=record(base/'result.json');manifest=record(base/'source-manifest.json')
        h=hashlib.sha256()
        for name in manifest['files']:h.update(name.encode()+b'\0'+(APP/name).read_bytes()+b'\0')
        check(group+'_current_real_browser_checks',browser['passed'] and browser['source_unchanged'] and browser['source_sha256']==manifest['sha256']==h.hexdigest() and len(browser['checks'])==3)
        check(group+'_owned_resources_closed',browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'] and browser['native_fixture_exit_code']==0 and browser['owned_build_process_stopped'] and browser['owned_browser_runner_stopped'])
        media=record(base/'encoded-media.json');interaction=record(base/'interaction-media.json');business=record(base/'business-proof.json')
        check(group+'_actual_media_and_no_credentials',media['passed'] and business['passed'] and business['plaintext_excluded'] and len(media['screenshots'])==2 and all(sha(ROOT/v['file'])==v['sha256'] for v in media['screenshots']+media['artifacts']) and interaction['passed'] and sha(ROOT/interaction['file'])==interaction['sha256'] and sha(APP/'scripts/render_batch_round167_media.py')==media['encoder_sha256'] and sha(APP/'scripts/derive_batch_round167_interaction.py')==interaction['source_script_sha256'])
    base='e36ec01a7b78eed742a7c1f6c831a4114b96bcde'
    old=subprocess.check_output(['git','ls-tree','-r','--name-only',base,'--','app/backend/migrations'],cwd=ROOT).decode().splitlines()
    check('all132_prior_sql_migrations_immutable',all(subprocess.check_output(['git','show',base+':'+n],cwd=ROOT)==(ROOT/n).read_bytes() for n in old if n.endswith('.sql')) and (APP/'backend/migrations/0133_invitation_reissues.sql').is_file())
    skills=list((APP/'skills').glob('*/manifest.json'))
    check('all_eight_skills_remain_draft_and_no_new_android_claim',len(skills)==8 and all(read(p)['state']=='draft' for p in skills) and not jvm['apk_built'] and not package['apk_built'])
    return {'passed':all(v['passed'] for v in checks),'checks':checks,'records':records,'retained':{'runtime':'increment163/66 retained, not replayed','full_backend_regression':'increment121 retained, not replayed','frozen_release':'v0.1.0-rc.9/business163/schema129/APK164; nine freezes immutable','real_configuration':'user deferred; all23 formal packages open'},'fixture_only_adjustment':adjustment,'interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False}

def main():
    result=verify();target=EVIDENCE/'final-source-check.json'
    if not target.exists():write_once(target,result)
    print(json.dumps({'passed':result['passed'],'checks':len(result['checks']),'failed':[v['name'] for v in result['checks'] if not v['passed']]}))
    return 0 if result['passed'] else 1

if __name__=='__main__':raise SystemExit(main())
