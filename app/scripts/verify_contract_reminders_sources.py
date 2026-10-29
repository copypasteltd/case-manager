"""Verify current source, actual native journeys, artifacts and retained boundaries."""
from datetime import datetime,timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess

APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent
E=ROOT/'docs/quality/evidence/contract-reminders'
PUBLISHED='8699b7d3401f2215bf37d2004870996fbfc8d5cf'
def read(p):return json.loads(p.read_text(encoding='utf-8'))
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def module(name):
    spec=importlib.util.spec_from_file_location('contract_reminders_proof_'+name,APP/'scripts'/(name+'.py'))
    value=importlib.util.module_from_spec(spec);spec.loader.exec_module(value);return value
def digest(names):
    value=hashlib.sha256()
    for name in names:value.update(name.encode()+b'\0'+(APP/name).read_bytes()+b'\0')
    return value.hexdigest()

def main():
    checks=[]
    def check(name,value):checks.append({'name':name,'passed':bool(value)})
    backend_source=module('verify_backend_batches').digest()
    group=E.parent/'contract-reminders-backend'
    backend,progress,raw=[read(group/n) for n in ('result.json','batch-progress.json','raw-log-archive.json')]
    expected={tuple(v) for v in progress['expected_tests']}
    observed={(v['Package'],v['Test']) for v in backend['checks'] if '/' not in v['Test']}
    check('Every discovered current backend test executes without skips',backend['passed'] and backend['source_sha256']==backend_source and backend['source_unchanged_during_tests'] and backend['all_discovered_tests_executed'] and expected==observed and backend['discovered_top_level_tests']==len(expected) and backend['parallel_batches']==1 and all(v['Action']=='pass' for v in backend['checks']) and backend['verification_script_sha256']==sha(APP/'scripts/verify_contract_reminders_backend.py'))
    check('Each complete successful native batch retains its exact raw log',raw['passed'] and raw['source_sha256']==backend_source and len(raw['logs'])==len(progress['batches']) and raw['records']==len(backend['checks']) and raw['freezer_sha256']==sha(APP/'scripts/freeze_contract_reminders_backend.py') and all(sha(APP/v['raw_log'])==v['sha256'] and not v['excluded_test_families'] for v in raw['logs']))
    feature=[v for v in backend['checks'] if v['Test'].startswith('TestContractReminderPolicies/')]
    check('All12 actual policy, signed-contract, scheduler, worker and transaction groups pass',len(feature)==12 and all(v['Action']=='pass' for v in feature))
    failed_fixtures=[read(E/'attempts'/name/'attempt.json') for name in ('01-mfa-fixture-reenrollment','02-expired-binding-restoration')]
    def private_path(name):
        value=Path(name)
        return value if value.is_absolute() else (ROOT/value if name.startswith('app/') else APP/value)
    check('Both failed fixture attempts retain exact private inputs and remain failed',all(not v['passed'] and any(c['Action']=='fail' for c in v['checks']) and sha(private_path(v['raw_log']))==v['raw_log_sha256'] and sha(private_path(v['source_archive']))==v['source_archive_sha256'] for v in failed_fixtures))
    discovery=read(E/'attempts/03-discovery-adapter/attempt.json')
    check('The refused empty discovery attempt is retained without executed test claims',not discovery['passed'] and discovery['test_batches_executed']==0 and sha(E/'attempts/03-discovery-adapter/verification-script.py')==discovery['script_sha256'])
    cluster,closed=[read(E/n) for n in ('native-cluster.json','native-cluster-closed.json')]
    check('Owned temporary native PG retains full durability and is closed cleanly',cluster['passed'] and closed['passed'] and all(cluster[n] for n in ('fsync','synchronous_commit','full_page_writes')) and not cluster['original_development_cluster_modified'] and not cluster['fixture_cleanup_deadline_changed'] and not cluster['virtualization_used'] and closed['owned_cluster_stopped'] and closed['remaining_disposable_databases']==0 and cluster['verification_script_sha256']==closed['verification_script_sha256']==sha(APP/'scripts/contract_reminders_native_cluster.py'))
    contract=read(APP/'backend/internal/contract/openapi.json')
    original=json.loads(subprocess.check_output(['git','show',PUBLISHED+':docs/development/openapi.json'],cwd=ROOT))
    check('All prior323 API paths and705 schemas retain exact semantics',all(contract['paths'][k]==v for k,v in original['paths'].items()) and all(contract['components']['schemas'][k]==v for k,v in original['components']['schemas'].items()) and len(contract['paths'])==325 and len(contract['components']['schemas'])==711 and contract==read(ROOT/'docs/development/openapi.json'))
    client=read(E/'client-checks.json')
    check('All eight final client commands rerun and bind unchanged current input',client['passed'] and client['source_unchanged'] and client['source_sha256']==client['source_after_sha256']==digest(client['source_files']) and len(client['checks'])==8 and all(c['passed'] and c['exit_code']==0 and sha(APP/'.local'/Path(c['raw_log']).name)==c['raw_log_sha256'] for c in client['checks']))
    web,mobile,runtime=[read(E/n) for n in ('web-build.json','mobile-build.json','runtime-build.json')]
    mobile_source=module('verify_mobile').digest()[0]
    check('Fresh Web205, mobile299 and runtime55 tests match their actual component sources',web['passed'] and web['tests']['numPassedTests']==205 and web['source_sha256']==digest(web['sources']) and mobile['passed'] and mobile['tests']=={'pass':299,'fail':0,'cancelled':0,'skipped':0} and mobile['source_sha256']==mobile_source and runtime['status']=='passed' and runtime['tests']['pass']==55 and runtime['source_sha256']==module('verify_agent_runtime').digest()[0] and all(v['derived_from']['sha256']==sha(E/'client-checks.json') for v in (web,mobile,runtime)))
    check('Both built miniapps retain all20 configured routes',set(mobile['targets'])=={'weapp','tt'} and all(v['page_count']==20 and sha(APP/'mobile/dist'/k/'app.json')==v['app_json_sha256'] for k,v in mobile['targets'].items()))
    selection=read(E/'attempts/04-mobile-test-selection/attempt.json');old=read(E/'attempts/04-mobile-test-selection/client-checks.json')
    check('Earlier client pass is preserved as incomplete new-case selection, never substituted for final299',not selection['passed'] and selection['original_commands_passed'] and not selection['new_mobile_policy_cases_executed'] and old['passed'] and sha(E/'attempts/04-mobile-test-selection/client-checks.json')==selection['original_record_sha256'] and all(sha(APP/v['file'])==v['sha256'] for v in selection['raw_logs']) and sha(APP/selection['source_archive'])==selection['source_archive_sha256'])
    feedback_folder=E/'attempts/05-mobile-validation-feedback'
    feedback,feedback_browser,feedback_clients=[read(feedback_folder/n) for n in ('attempt.json','result.json','client-checks.json')]
    check('The actual mobile feedback failure and passing pre-repair commands remain exact and distinct',not feedback['passed'] and not feedback_browser['passed'] and feedback_clients['passed'] and feedback['original_mobile_tests']==299 and feedback['owned_browser_closed'] and feedback['isolated_database_cleanup_passed'] and sha(feedback_folder/'result.json')==feedback['browser_record_sha256'] and sha(feedback_folder/'client-checks.json')==feedback['client_record_sha256'] and sha(APP/feedback['source_archive'])==feedback['source_archive_sha256'] and all(sha(APP/v['file'])==v['sha256'] for v in feedback['raw_logs']))
    layout_folder=E/'attempts/06-web-layout-adapter'
    layout,layout_web,layout_mobile=[read(layout_folder/n) for n in ('attempt.json','web/result.json','mobile/result.json')]
    check('The Web DOM-check failure and original passing mobile capture retain exact provenance',not layout['passed'] and not layout['application_source_changed'] and not layout_web['passed'] and len(layout_web['checks'])==4 and layout_mobile['passed'] and len(layout_mobile['checks'])==5 and sha(layout_folder/'web/result.json')==layout['web_record_sha256'] and sha(layout_folder/'mobile/result.json')==layout['mobile_record_sha256'] and sha(APP/layout['source_archive'])==layout['source_archive_sha256'] and all(sha(APP/v['file'])==v['sha256'] for v in layout['private_logs_and_manifests']) and all(sha(ROOT/v['file'])==v['sha256'] for v in layout['screenshots']))
    build,inspection,feature_apk,jvm=[read(E/n) for n in ('build.json','apk-inspection.json','android-feature-content.json','android-test-scope.json')]
    check('Actual new native development APK binds current source and byte-identical bundle',build['passed'] and build['source_unchanged_during_build'] and build['source_sha256']==module('build_android').sources()[0] and all(sha(APP/v['path'])==v['sha256'] for v in build['artifacts'].values()) and inspection['passed'] and feature_apk['passed'] and len(inspection['checks'])==18 and len(feature_apk['checks'])==4 and not feature_apk['artifact_executed'] and not feature_apk['real_device'] and feature_apk['verification_script_sha256']==sha(APP/'scripts/verify_contract_reminders_android_content.py'))
    check('Android JVM26 records preserve actual cache disclosure and four original XMLs',jvm['passed'] and jvm['tests']==26 and jvm['new_test_execution']==(not jvm['gradle_cached']) and not jvm['device_executed'] and sha(E/'native-build.log')==jvm['native_log_sha256'] and len(jvm['xml_sha256'])==4 and all(sha(E/'android-unit-results'/name)==value for name,value in jvm['xml_sha256'].items()))
    linux=read(E/'linux-build.json')
    check('Two current Linux ELF artifacts cross-compile without executing Linux',linux['passed'] and linux['source_sha256']==linux['source_after_sha256']==backend_source and len(linux['artifacts'])==2 and all(sha(APP/v['file'])==v['sha256'] for v in linux['artifacts']) and not linux['artifact_executed'] and not linux['kernel_boundary_executed'] and not linux['deployed'] and linux['verification_script_sha256']==sha(APP/'scripts/build_contract_reminders_linux.py'))
    interaction=module('contract_reminders_sources').digest()
    shots=[]
    for target in ('web','mobile'):
        folder=E.parent/('contract-reminders-'+target)
        browser,manifest,capture=[read(folder/n) for n in ('result.json','source-manifest.json','capture.json')]
        check('Actual '+target+' policy journey passes five groups and closes owned resources',browser['passed'] and len(browser['checks'])==5 and browser['source_unchanged'] and browser['source_sha256']==manifest['sha256']==digest(manifest['files']) and browser['build_source_sha256']==browser['build_source_after_sha256']==interaction and browser['build_source_unchanged'] and all(browser[n] for n in ('owned_browser_closed','owned_review_server_stopped','owned_api_proxy_stopped','isolated_database_cleanup_passed')) and browser['native_fixture_exit_code']==0 and not browser['console_errors'] and not browser['server_errors'] and browser['h5_prebuilt_before_fixture']==(target=='mobile') and browser['real_api_forwarded_without_response_substitution']==(target=='mobile') and all(sha(ROOT/v['file'])==v['sha256'] for v in capture['screenshots']))
        shots.extend(capture['screenshots'])
    media=read(E/'media.json');visual=read(E/'visual-review.json')
    check('Both silent MP4 and GIF pairs retain exact authenticated synthetic frame provenance',media['passed'] and len(media['recordings'])==2 and media['encoder_script_sha256']==sha(APP/'scripts/encode_contract_reminders_media.py') and all(sha(ROOT/v['capture_manifest'])==v['capture_manifest_sha256'] and all(sha(ROOT/a['file'])==a['sha256'] for a in v['artifacts']) for v in media['recordings']))
    check('Visual review binds every new screenshot and both decoded GIF samples',visual['reviewed'] and len(shots)==7 and len(visual['screenshots'])==7 and len(visual['gifs'])==2 and all(sha(ROOT/v['file'])==v['sha256'] for v in visual['screenshots']+visual['gifs']) and set(visual['gif_frames_reviewed'])=={'web','mobile'})
    retained=read(E/'reused-evidence.json')
    check('All233 previous media and dependency records retain original published bytes and dates',retained['passed'] and retained['published_commit']==PUBLISHED and len(retained['media'])==233 and all(sha(ROOT/v['file'])==v['sha256'] for v in retained['media']+retained['records']) and retained['package_lock_sha256']==sha(APP/'package-lock.json') and retained['workspace_dependency_fields_unchanged'] and retained['mobile_test_selection_updated'] and not retained['new_dependency_audit'])
    development,update=[read(E/n) for n in ('development-project-resources.json','development-update.json')]
    check('Ordinary native schema85-to86 upgrade preserves historical fee rows and owned services',development['passed'] and update['passed'] and development['migration_count']==update['migration_count']==86 and update['previous_migration_count']==85 and update['legacy_and_usage_rows_preserved'] and development['backend_source_sha256']==update['backend_source_sha256']==backend_source and all(development['ownership'].values()) and development['worker_sources_match'] and development['integrations_disabled'] and development['database']['disposable_test_databases']==0 and development['verification_script_sha256']==sha(APP/'scripts/verify_contract_reminders_development.py') and update['updater_sha256']==sha(APP/'scripts/update_contract_reminders_development.py') and not update['production_deployment'])
    scope=read(ROOT/'docs/delivery/04-current-scope.json')
    skills=[read(p) for p in (APP/'skills').glob('*/manifest.json')]
    check('All23 active packages, five exclusions and eight draft Skills retain the corrected AI scope',scope['counts']['active']==23 and scope['counts']['excluded']==5 and scope['flags']['ai_secretary_required'] and scope['flags']['ai_authorization_required'] and scope['flags']['ai_execution_isolation_required'] and not scope['flags']['ai_cost_control_required'] and len(skills)==8 and all(s['state']=='draft' for s in skills))
    ports_closed=True
    for port in (5189,5190):
        with socket.socket() as probe:probe.settimeout(1);ports_closed=ports_closed and probe.connect_ex(('127.0.0.1',port))!=0
    check('Owned browser fixture and both temporary HTTP ports are released',ports_closed and not (APP/'.local/business-browser-fixture.json').exists())
    proof={'passed':all(v['passed'] for v in checks),'checked_at':datetime.now(timezone.utc).isoformat(),'checks':checks,
        'interaction_kind':'organization_contract_reminder_policy','interaction_source_sha256':interaction,
        'mobile_source_sha256':mobile_source,'backend_source_sha256':backend_source,'verification_script_sha256':sha(Path(__file__)),
        'historical_evidence_commit':PUBLISHED,'new_browser_capture':True,'new_backend_test_execution':True,
        'full_goal_complete':False,'production_ready':False}
    (E/'final-source-check.json').write_text(json.dumps(proof,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':proof['passed'],'checks':len(checks),'failed':[v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1
if __name__=='__main__':raise SystemExit(main())
