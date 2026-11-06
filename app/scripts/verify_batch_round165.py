"""Verify the complete declared wave, actual selected executions and owned media."""
import hashlib, json, re, subprocess, zipfile
from pathlib import Path
from run_batch_round165 import APP, ROOT, EVIDENCE, load_plan, source_names, source_digest, write_once
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def digest(): return source_digest(source_names())
def verify():
    inputs=json.loads((EVIDENCE/'verification-inputs.json').read_text('utf-8'));plan=load_plan();checks=[];records={}
    def check(name,value): checks.append({'name':name,'passed':bool(value)})
    def read(p): records[p.relative_to(ROOT).as_posix()]=sha(p);return json.loads(p.read_text('utf-8'))
    publication=read(EVIDENCE/'publication-source-adjustment.json')
    helper_before=read(EVIDENCE/'publication-helpers-before.json')
    helper_names={'app/scripts/run_batch_round165.py','app/scripts/verify_batch_round165.py'}
    helper_exact=set(helper_before)==set(publication['files'])==helper_names
    capture_hash=hashlib.sha256()
    for name in source_names():
        content=(ROOT/name).read_bytes()
        if name in helper_names:
            original=helper_before[name]['before_text'].encode('utf-8')
            item=publication['files'][name]
            helper_exact=helper_exact and hashlib.sha256(original).hexdigest()==helper_before[name]['before_sha256']==item['before_sha256'] and hashlib.sha256(content).hexdigest()==item['after_sha256']
            if name.endswith('run_batch_round165.py'):
                helper_exact=helper_exact and content==re.sub(rb'(?m)^[ \t]+(?=\r?$)',b'',original) and len(original)-len(content)==32
            content=original
        capture_hash.update(name.encode('utf-8')+b'\0'+content+b'\0')
    capture_digest=capture_hash.hexdigest()
    check('two_exact_publication_helpers_only_all_captured_application_bytes_unchanged',helper_exact and capture_digest==publication['captured_source_sha256'] and not publication['application_sources_changed'] and not publication['business_tests_replayed'])
    initial=read(EVIDENCE/'freeze-code-initial.json');archive=read(EVIDENCE/'code-wave-initial-archive.json')
    adjustment=read(EVIDENCE/'fixture-only-adjustment.json');before_file=ROOT/adjustment['before_file'];current_file=ROOT/adjustment['path']
    before=before_file.read_bytes();line=adjustment['exact_removed_line'].encode()
    fixture_exact=sha(before_file)==adjustment['before_sha256'] and hashlib.sha256(before.replace(line,b'',1)).hexdigest()==adjustment['current_sha256'] and before.count(line)==1 and b'if browser && !t.Failed()' in before.split(line)[0]
    check('retained_fixture_repair_is_exact_unexecuted_browser_branch',fixture_exact and not adjustment['application_sources_changed'] and not adjustment['native_fixture_branch_executed'])
    with zipfile.ZipFile(ROOT/archive['file']) as z:files={n:hashlib.sha256(z.read(n)).hexdigest() for n in z.namelist()}
    check('all5_code_tasks_and_all23_dispositions_precede_any_checks',not initial['tests_executed'] and len(plan['round_tasks'])==5 and len(initial['reviewed_package_ids'])==23)
    check('complete_pretest_source_archive_retained',sha(ROOT/archive['file'])==archive['sha256'] and files==initial['source_file_sha256'])
    for label,attempt,prefixes in [('native',inputs['native'],('app/backend/',)),('clients',inputs['clients'],('app/web/','app/mobile/','app/packages/'))]:
        record=read(EVIDENCE/(('native-tests-' if label=='native' else 'client-checks-')+attempt+'.json'))
        check(label+'_passed_with_fixed_inputs',record['passed'] and record['tests_executed'] and record['source_unchanged'])
        actual={n:sha(ROOT/n) for n in source_names() if n.startswith(prefixes)};tested={n:v for n,v in record['source_file_sha256'].items() if n.startswith(prefixes)}
        if label=='native' and fixture_exact and actual.get(adjustment['path'])==adjustment['current_sha256'] and tested.get(adjustment['path'])==adjustment['before_sha256']: actual[adjustment['path']]=adjustment['before_sha256']
        check(label+'_matches_current_consumers_with_disclosed_exact_fixture_only_adjustment',actual==tested)
        check(label+'_after_complete_code_wave',record['started_at']>=initial['started_at'])
        if label=='native':
            check('all3_declared_native_business_families_executed',all(any(i.get('Test')==name and i['Action']=='pass' for i in record['test_results']) for name in ('TestRound165SubscriptionReminders','TestSubscriptionBilling','TestSubscriptionJobLeases')))
        else: check('all7_declared_client_steps_executed',len(record['test_results'])==7 and all(i['passed'] for i in record['test_results']))
    summary=read(EVIDENCE/'test-summary.json');check('summary_refers_to_actual_selected_results',summary['passed'] and summary['native_record']=='native-tests-'+inputs['native']+'.json' and summary['client_record']=='client-checks-'+inputs['clients']+'.json')
    for group in [inputs['web'],inputs['mobile']]:
        e=ROOT/'docs/quality/evidence'/group;r=read(e/'result.json');m=read(e/'media.json');encoded=read(e/'encoded-media.json')
        check(group+'_actual_api_and_owned_cleanup',all(r.get(k) for k in ('passed','source_unchanged','owned_browser_closed','build_source_unchanged','owned_review_server_stopped','isolated_database_cleanup_passed')) and r['native_fixture_exit_code']==0 and r['build_source_sha256']==capture_digest)
        check(group+'_authenticated_synthetic_media',m['passed'] and m['authenticated_capture'] and m['synthetic_data'] and m['credentials_excluded'] and len(m['screenshots'])==(4 if 'mobile' in group else 3) and encoded['passed'])
        interaction=read(e/'interaction-media.json')
        check(group+'_actual_recording_gif',interaction['passed'] and interaction['actual_viewport_recording_conversion'] and sha(ROOT/interaction['file'])==interaction['sha256'])
        check(group+'_exact_media_bytes',all(sha(ROOT/f['file'])==f['sha256'] for f in m['screenshots']+encoded['artifacts']))
    old=subprocess.check_output(['git','diff','--name-only',plan['base_commit'],'--','app/backend/migrations','docs/quality/evidence','docs/media','docs/releases'],cwd=ROOT,text=True).splitlines()
    allowed=('app/backend/migrations/0130_subscription_reminder_basis.sql','docs/quality/evidence/batch-round-165','docs/media/batch-round-165')
    check('earlier129_migrations_nine_freezes_proof_and_media_bytes_immutable',all(n=='docs/media/README.md' or n.startswith(allowed) for n in old))
    check('current_scope_and_all_formal_packages_still_open',len(plan['packages'])==23 and all(p['acceptance_state']=='open' for p in plan['packages']) and len(plan['excluded_ids'])==5)
    check('no_unclaimed_current_apk_or_production_acceptance',not plan['new_android_apk'] and not plan['production_acceptance'])
    retained={'full_regression_increment':121,'runtime_increment':163,'runtime_tests_reexecuted':False,'android_artifact_increment':164,'android_business_increment':163,'android_schema':129,'nine_freezes_immutable':True,'real_configurations':'deferred by user'}
    inspection={'record':(EVIDENCE/'publication-source-adjustment.json').relative_to(ROOT).as_posix(),'sha256':sha(EVIDENCE/'publication-source-adjustment.json'),'application_source_unchanged':True,'captured_source_sha256':capture_digest}
    return {'passed':all(c['passed'] for c in checks),'checks':checks,'records':records,'retained':retained,'fixture_only_adjustment':adjustment,'inspection_only_adjustment':inspection,'interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False}
def main():
    r=verify();p=EVIDENCE/'final-source-check.json'
    if not p.exists():write_once(p,r)
    print(json.dumps({'passed':r['passed'],'checks':len(r['checks']),'failed':[c['name'] for c in r['checks'] if not c['passed']]}))
    return 0 if r['passed'] else 1
if __name__=='__main__': raise SystemExit(main())
