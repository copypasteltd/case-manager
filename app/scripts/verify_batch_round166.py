"""Current application, full pretest wave, actual selected checks and media."""
import hashlib
import json
from pathlib import Path
import subprocess
import zipfile

from run_batch_round166 import APP, ROOT, EVIDENCE, load_plan, source_names, source_digest, write_once

sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def digest():return source_digest(source_names())


def verify():
    plan=load_plan();inputs=json.loads((EVIDENCE/'verification-inputs.json').read_text('utf-8'));checks=[];records={}
    def check(name,value):checks.append({'name':name,'passed':bool(value)})
    def read(path):records[path.relative_to(ROOT).as_posix()]=sha(path);return json.loads(path.read_text('utf-8'))
    inspection=read(EVIDENCE/'post-capture-verifier-repair.json')
    capture_archive=read(EVIDENCE/'code-wave-browser-repair-archive.json')
    helper='app/scripts/verify_batch_round166.py'
    with zipfile.ZipFile(ROOT/capture_archive['file']) as saved:
        archived={n:hashlib.sha256(saved.read(n)).hexdigest() for n in saved.namelist()}
        original=saved.read(helper)
    h=hashlib.sha256()
    actual={n:sha(ROOT/n) for n in source_names()}
    exact=archived.keys()==actual.keys() and all(actual[n]==v for n,v in archived.items() if n!=helper)
    for name in source_names():
        content=original if name==helper else (ROOT/name).read_bytes()
        h.update(name.encode('utf-8')+b'\0'+content+b'\0')
    captured=h.hexdigest()
    check('post_capture_verifier_only_all_application_inputs_unchanged',exact and sha(ROOT/capture_archive['file'])==capture_archive['sha256'] and inspection['before_text'].encode('utf-8')==original and inspection['before_sha256']==hashlib.sha256(original).hexdigest() and inspection['after_sha256']==sha(Path(__file__)) and not inspection['application_sources_changed'] and not inspection['business_tests_replayed'])
    initial=read(EVIDENCE/'freeze-code-initial.json');archive=read(EVIDENCE/'code-wave-initial-archive.json')
    with zipfile.ZipFile(ROOT/archive['file']) as saved:files={n:hashlib.sha256(saved.read(n)).hexdigest() for n in saved.namelist()}
    check('complete_all7_tasks_all23_dispositions_pretest_archive',len(plan['round_tasks'])==7 and len(plan['packages'])==23 and not initial['tests_executed'] and files==initial['source_file_sha256'] and sha(ROOT/archive['file'])==archive['sha256'])
    check('all_current_declared_tasks_implemented_before_tests',plan['implementation_completed_before_tests'] and all(t['state']=='implemented' for t in plan['round_tasks']))
    for label,prefixes,filename in [('native',('app/backend/',),'native-tests-'),('clients',('app/web/','app/mobile/','app/packages/'),'client-checks-'),('engineering',('app/deploy/','app/backend/internal/telegram/','app/backend/internal/project/'),'engineering-')]:
        record=read(EVIDENCE/(filename+inputs[label]+'.json'))
        current={n:sha(ROOT/n) for n in source_names() if n.startswith(prefixes)}
        tested={n:v for n,v in record['source_file_sha256'].items() if n.startswith(prefixes)}
        check(label+'_actual_selected_pass_and_current_sources',record['passed'] and record['tests_executed'] and record['source_unchanged'] and current==tested and record['started_at']>=initial['started_at'])
        if label=='native':
            check('all4_native_business_families_executed',all(any(i.get('Test')==name and i['Action']=='pass' for i in record['test_results']) for name in ('TestRound166CloseoutCalendar','TestRound166TelegramGrantDelivery','TestTelegramDelivery','TestProjectCloseoutAndArchive')))
        if label=='clients':check('all7_declared_client_steps_pass',len(record['test_results'])==7 and all(i['passed'] for i in record['test_results']))
        if label=='engineering':
            earlier=read(EVIDENCE/'engineering-initial.json')
            selected={i['step']:i for i in earlier['steps'] if i['passed']}
            selected.update({i['step']:i for i in record['steps']})
            unchanged={n:sha(ROOT/n) for n in source_names() if n.startswith(prefixes) and n!='app/deploy/release/test_native_release.py'}
            original={n:v for n,v in earlier['source_file_sha256'].items() if n.startswith(prefixes) and n!='app/deploy/release/test_native_release.py'}
            check('all4_engineering_steps_pass_with_exact_unchanged_consumer_reuse',set(selected)=={'release','monitor-plan','go-private','monitor-tools'} and all(i['passed'] for i in selected.values()) and unchanged==original)
            repair=read(EVIDENCE/'unified-repair.json')
            with zipfile.ZipFile(ROOT/archive['file']) as saved: old_test=saved.read('app/deploy/release/test_native_release.py').decode('utf-8')
            expected=old_test.replace('\n    def test_personal_telegram_fixed_credentials_and_complete_catalog(self):','\n\nclass PrivateTelegramReleaseTests(Fixture):\n    def test_personal_telegram_fixed_credentials_and_complete_catalog(self):',1)
            check('only_two_fixture_cases_moved_to_their_own_fixture_class',expected==(APP/'deploy/release/test_native_release.py').read_text('utf-8') and repair['application_sources_changed'] is False)
    tools=read(EVIDENCE/('monitor-tools-'+inputs.get('monitor_tools',inputs['engineering'])+'.json'))
    check('actual_pinned_native_prometheus_alertmanager_blackbox_validation',tools['passed'] and len(tools['checks'])==5 and not tools['services_started'] and not tools['real_receiver_contacted'] and tools['component_lock_sha256']==sha(APP/'deploy/monitoring/components.lock.json'))
    summary=read(EVIDENCE/'test-summary.json');check('summary_from_actual_native_and_client_records',summary['passed'] and summary['native_record']=='native-tests-'+inputs['native']+'.json' and summary['client_record']=='client-checks-'+inputs['clients']+'.json')
    for group in [inputs['web'],inputs['mobile']]:
        e=ROOT/'docs/quality/evidence'/group;r=read(e/'result.json');m=read(e/'media.json');encoded=read(e/'encoded-media.json');interaction=read(e/'interaction-media.json')
        check(group+'_actual_api_and_owned_cleanup',all(r.get(k) for k in ('passed','source_unchanged','owned_browser_closed','build_source_unchanged','owned_review_server_stopped','isolated_database_cleanup_passed')) and r['native_fixture_exit_code']==0 and r['build_source_sha256']==captured)
        check(group+'_authenticated_synthetic_media',m['passed'] and m['authenticated_capture'] and m['synthetic_data'] and m['credentials_excluded'] and len(m['screenshots'])==2 and encoded['passed'])
        check(group+'_exact_media_and_actual_recording_gif',interaction['passed'] and interaction['actual_viewport_recording_conversion'] and sha(ROOT/interaction['file'])==interaction['sha256'] and all(sha(ROOT/f['file'])==f['sha256'] for f in m['screenshots']+encoded['artifacts']))
    old=subprocess.check_output(['git','diff','--name-only',plan['base_commit'],'--','app/backend/migrations','docs/quality/evidence','docs/media','docs/releases'],cwd=ROOT,text=True).splitlines()
    allowed=('app/backend/migrations/0131_','app/backend/migrations/0132_','docs/quality/evidence/batch-round-166','docs/media/batch-round-166')
    check('earlier130_migrations_nine_freezes_proof_media_immutable',all(n=='docs/media/README.md' or n.startswith(allowed) for n in old))
    check('all23_formal_packages_and_5_exclusions_preserved',len(plan['packages'])==23 and all(p['acceptance_state']=='open' for p in plan['packages']) and len(plan['excluded_ids'])==5)
    check('no_unclaimed_new_apk_real_services_or_production_acceptance',not plan['new_android_apk'] and not plan['production_acceptance'])
    retained={'full_regression_increment':121,'runtime_increment':163,'runtime_tests_reexecuted':False,'android_artifact_increment':164,'android_business_increment':163,'android_schema':129,'nine_freezes_immutable':True,'real_configurations':'deferred by user'}
    return {'passed':all(c['passed'] for c in checks),'checks':checks,'records':records,'retained':retained,'inspection_only_adjustment':{'record':(EVIDENCE/'post-capture-verifier-repair.json').relative_to(ROOT).as_posix(),'sha256':sha(EVIDENCE/'post-capture-verifier-repair.json'),'application_source_unchanged':True,'captured_source_sha256':captured},'interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False}


def main():
    result=verify();target=EVIDENCE/'final-source-check.json'
    if not target.exists():write_once(target,result)
    print(json.dumps({'passed':result['passed'],'checks':len(result['checks']),'failed':[c['name'] for c in result['checks'] if not c['passed']]}))
    return 0 if result['passed'] else 1


if __name__=='__main__':raise SystemExit(main())
