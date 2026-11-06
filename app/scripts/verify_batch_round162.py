"""Check the final wave against executed, immutable local proof records."""
from pathlib import Path
import argparse, hashlib, json, subprocess, zipfile
from run_batch_round162 import APP, ROOT, EVIDENCE, load_plan, source_names, source_digest, write_once

def sha(p): return hashlib.sha256(p.read_bytes()).hexdigest()

def digest(): return source_digest(source_names())

def verify():
    inputs=json.loads((EVIDENCE/'verification-inputs.json').read_text(encoding='utf-8'))
    plan=load_plan();checks=[];records={}
    def check(name,value): checks.append({'name':name,'passed':bool(value)})
    def read(path):
        records[path.relative_to(ROOT).as_posix()]=sha(path)
        return json.loads(path.read_text(encoding='utf-8'))
    initial=read(EVIDENCE/'freeze-code-initial.json')
    check('declared_wave_finished_before_checks',initial['tests_executed'] is False and len(initial['reviewed_package_ids'])==23 and all(i['state']=='implemented' for i in plan['round_tasks']))
    archive=read(EVIDENCE/'initial-source-archive.json')
    with zipfile.ZipFile(ROOT/archive['file']) as frozen:
        archived={name:hashlib.sha256(frozen.read(name)).hexdigest() for name in frozen.namelist()}
    check('initial_archive_matches_all_pretest_source_bytes',sha(ROOT/archive['file'])==archive['sha256'] and archived==initial['source_file_sha256'])

    native=read(EVIDENCE/('native-tests-'+inputs['native']+'.json'))
    clients=read(EVIDENCE/('client-checks-'+inputs['clients']+'.json'))
    for label,proof,prefixes in [('native',native,('app/backend/',)),('client',clients,('app/web/','app/mobile/','app/packages/','app/agent-runtime/','app/skills/','docs/skills/'))]:
        check(label+'_execution_passed',proof['passed'] and proof['tests_executed'] and proof['source_unchanged'])
        selected={n:v for n,v in proof['source_file_sha256'].items() if n.startswith(prefixes)}
        actual={n:sha(ROOT/n) for n in source_names() if n.startswith(prefixes)}
        check(label+'_actual_consumer_bytes_current',selected==actual)
        check(label+'_starts_after_whole_code_wave',proof['started_at']>=initial['started_at'])

    retained_clients=read(EVIDENCE/'client-checks-repair-3.json')
    ui_time_files={'app/web/src/telegram-grant-time.ts','app/web/src/TelegramWriteGrant.tsx','app/web/src/TelegramGrantInventory.tsx'}
    retained_actual={n:v for n,v in retained_clients['source_file_sha256'].items() if n.startswith(('app/web/','app/mobile/','app/packages/','app/agent-runtime/','app/skills/','docs/skills/')) and n not in ui_time_files}
    current_actual={n:sha(ROOT/n) for n in source_names() if n.startswith(('app/web/','app/mobile/','app/packages/','app/agent-runtime/','app/skills/','docs/skills/')) and n not in ui_time_files}
    check('retained_unaffected_units_runtime_mobile_and_builds_have_exact_inputs',retained_clients['passed'] and len(retained_clients['test_results'])==9 and retained_actual==current_actual)
    check('current_web_time_repair_typechecks_and_builds',{v['step'] for v in clients['test_results']}=={'typecheck','web-build'})
    for label,group in [('web',inputs['web']),('mobile',inputs['mobile'])]:
        folder=ROOT/'docs/quality/evidence'/group
        report=read(folder/'result.json')
        check(label+'_capture_and_cleanup',all(report[k] for k in ('passed','source_unchanged','build_source_unchanged','owned_browser_closed','owned_review_server_stopped','isolated_database_cleanup_passed')) and report['native_fixture_exit_code']==0)
        manifest=read(folder/'source-manifest.json')
        capture_hash=hashlib.sha256()
        for name in manifest['files']:
            capture_hash.update(name.encode('utf-8')+b'\0'+(APP/name).read_bytes()+b'\0')
        check(label+'_actual_capture_inputs_current',capture_hash.hexdigest()==manifest['sha256']==report['source_sha256'])
        encoded=read(folder/'encoded-media.json')
        interaction=read(folder/'interaction-media.json')
        artifacts=encoded['screenshots']+encoded['artifacts']+[interaction]
        check(label+'_primary_media_bytes',encoded['passed'] and interaction['passed'] and all((ROOT/v['file']).is_file() and sha(ROOT/v['file'])==v['sha256'] for v in artifacts))
    original=read(EVIDENCE/'native-tests-initial.json')
    fixed_catalog=['app/backend/internal/agent/execution_tools.go','app/backend/internal/agent/mutations.go','app/backend/internal/agentworker/mcp.go','app/backend/internal/agentworker/bridge.go','app/backend/internal/agentworker/round162_grants_test.go']
    check('retained_initial_MCP_catalog_pass_has_unchanged_actual_inputs',any(v.get('Test')=='TestRound162TelegramProjectCatalog' and v['Action']=='pass' for v in original['test_results']) and all(original['source_file_sha256'][n]==sha(ROOT/n) for n in fixed_catalog))
    scope=json.loads((ROOT/'docs/delivery/04-current-scope.json').read_text(encoding='utf-8'))
    check('all_23_packages_still_open',sum(v['status']=='active' for v in scope['items'])==23 and all(v['acceptance_state']=='open' for v in plan['packages']))
    check('eight_skills_remain_draft',all(json.loads(p.read_text(encoding='utf-8'))['state']=='draft' for p in (APP/'skills').glob('*/manifest.json')))
    check('schema128_is_append_only',len(list((APP/'backend/migrations').glob('*.sql')))==128)
    old_changed=subprocess.check_output(['git','diff','--name-only','--diff-filter=MD','42e8540db539cb6a97f62a2d442742f3206a5aca','--','docs/quality/evidence','docs/media','app/backend/migrations'],cwd=ROOT,text=True).splitlines()
    # The current media navigation is maintained with every published update.
    # It is the only permitted edit in these historical artifact directories.
    old_changed=[name for name in old_changed if name!='docs/media/README.md']
    check('prior_evidence_media_migrations_preserved',not old_changed)
    review=read(EVIDENCE/'visual-review.json')
    check('primary_images_reviewed',review['reviewed'] and len(review['images'])==8 and all(sha(ROOT/v['file'])==v['sha256'] for v in review['images']))
    return {'passed':all(c['passed'] for c in checks),'checks':checks,'increment':162,'schema':128,'source_sha256':digest(),'interaction_source_sha256':digest(),'records':records,'retained':{'prior_source':'42e8540db539cb6a97f62a2d442742f3206a5aca','apk':'154/business153/schema113','full_regression':'121'},'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'production_ready':False,'formal_acceptance':False,'real_integrations_executed':False,'full_goal_complete':False}

def main():
    parser=argparse.ArgumentParser()
    for name in ('native','clients','web','mobile'): parser.add_argument('--'+name)
    parser.add_argument('--attempt',default='final')
    args=parser.parse_args()
    values={name:getattr(args,name) for name in ('native','clients','web','mobile')}
    config=EVIDENCE/'verification-inputs.json'
    if any(values.values()):
        if not all(values.values()): raise ValueError('Supply all selected executed records together.')
        if not config.exists(): write_once(config,values)
        elif json.loads(config.read_text(encoding='utf-8'))!=values: raise ValueError('Preserve the previously selected proof inputs.')
    result=verify()
    write_once(EVIDENCE/('scope-check-'+args.attempt+'.json'),result)
    if result['passed'] and not (EVIDENCE/'final-source-check.json').exists(): write_once(EVIDENCE/'final-source-check.json',result)
    print(json.dumps({'passed':result['passed'],'checks':len(result['checks']),'failed':[c['name'] for c in result['checks'] if not c['passed']]}))
    return 0 if result['passed'] else 1
if __name__=='__main__':raise SystemExit(main())
