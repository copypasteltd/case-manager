"""Verify the complete audited wave and actual selected checks, without replay."""
import hashlib
import json
from pathlib import Path
import zipfile
from development_wave import Wave, ROOT, sha, write_once

E = ROOT / 'docs/quality/evidence/batch-round-168'
def read(path): return json.loads(path.read_text(encoding='utf-8'))
def current(files): return all((ROOT/name).is_file() and sha(ROOT/name)==value for name,value in files.items())
def digest():
    wave=Wave(168)
    return wave.digest(wave.binding(wave.sources()))

def verify():
    wave=Wave(168);plan=wave.plan();inputs=read(E/'verification-inputs.json');checks=[];records=[]
    def check(name,ok): checks.append({'name':name,'passed':bool(ok)})
    def record(path): records.append({'file':path.relative_to(ROOT).as_posix(),'sha256':sha(path)});return read(path)
    check('whole23_requirement_contexts_and_open_formal_acceptance',len(plan['packages'])==23 and all(row['audit_state']=='reviewed' and row['acceptance_state']=='open' and row['current_remaining'] for row in plan['packages']))
    check('every_confirmed_code_task_implemented_before_checks',bool(plan['code_tasks']) and all(row['state']=='implemented' and all((ROOT/name).exists() for name in row['files']) for row in plan['code_tasks']))
    check('independent_refutations_preserved',all((ROOT/name).is_file() for name in plan['independent_refutations']))
    first=record(E/'freeze-code-initial.json')
    with zipfile.ZipFile(ROOT/first['archive']['file']) as saved: original={name:hashlib.sha256(saved.read(name)).hexdigest() for name in saved.namelist()}
    check('whole_code_tools_and_fixtures_preserved_before_any_test',first['passed'] and not first['tests_executed'] and len(first['reviewed_package_ids'])==23 and sha(ROOT/first['archive']['file'])==first['archive']['sha256'] and original==first['source_file_sha256'])
    for phase in ('native','clients','runtime','release'):
        result=record(E/(phase+'-'+inputs[phase]+'.json'))
        expected=set(plan['validation']['steps'][phase]);actual={row['step'] for row in result['steps']}
        check(phase+'_all_selected_current_consumers_pass',result['passed'] and result['source_unchanged'] and actual==expected and all(row['passed'] and row['exit_code']==0 and current(row['source_file_sha256']) for row in result['steps']) and not result['formal_acceptance'] and not result['real_integrations_executed'])
    for group in inputs['browser_groups']:
        result=record(ROOT/'docs/quality/evidence'/group/'result.json');media=record(ROOT/'docs/quality/evidence'/group/'encoded-media.json')
        check(group+'_actual_views_closed_and_current',result['passed'] and result['source_unchanged'] and result['owned_browser_closed'] and result['owned_review_server_stopped'] and result['owned_build_process_stopped'] and result['owned_browser_runner_stopped'] and result['isolated_database_cleanup_passed'] and result['native_fixture_exit_code']==0 and result['build_source_unchanged'] and result['build_source_after_sha256']==wave.digest(wave.binding(wave.sources())) and len(result['checks'])>=5)
        check(group+'_actual_redacted_media',media['passed'] and len(media['screenshots'])>=4 and all((ROOT/row['file']).is_file() and sha(ROOT/row['file'])==row['sha256'] for row in media['screenshots']+media['artifacts']))
    package=record(E/('production-'+inputs['production']+'.json'))
    check('native_server_artifact_binds_current_sources_and136_migrations',package['passed'] and package['schema']==136 and current(package['source_file_sha256']) and sha(ROOT/package['build']['archive'])==package['build']['archive_sha256'] and not package['native_linux_executed'] and not package['production_deployed'] and not package['apk_built'])
    check('historical133_migrations_immutable',all(sha(ROOT/name)==value for name,value in inputs['historical_migration_sha256'].items()) and len(inputs['historical_migration_sha256'])==133)
    check('eight_skills_stay_draft',len(list((ROOT/'app/skills').glob('*/manifest.json')))==8 and all(read(path)['state']=='draft' for path in (ROOT/'app/skills').glob('*/manifest.json')))
    check('development_and_frozen_artifacts_remain_retained',inputs['development_postgres_schema']==100 and inputs['frozen_business_increment']==163 and inputs['frozen_schema']==129 and inputs['development_apk_increment']==164 and not inputs['new_android_bundle_or_apk_built'])
    return {'passed':all(row['passed'] for row in checks),'increment':168,'checks':checks,'records':records,
            'retained':{'full_backend_regression':'increment121 retained, not replayed',
                        'native_jvm':'increment167 retained, not replayed',
                        'frozen_release':'v0.1.0-rc.9/business163/schema129/APK164; nine freezes immutable',
                        'development_database':'schema100/API unchanged',
                        'real_configuration':'user deferred; all23 formal packages open; eight Skills draft'},
            'interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),
            'full_backend_suite_executed':False,'formal_android_acceptance':False,
            'business_tests_replayed':False,'full_goal_complete':False,'production_ready':False,'formal_acceptance':False}
if __name__=='__main__':
    out=verify();target=E/'final-source-check.json'
    if not target.exists(): write_once(target,out)
    print(json.dumps({'passed':out['passed'],'checks':len(out['checks']),'failed':[c['name'] for c in out['checks'] if not c['passed']]}));raise SystemExit(0 if out['passed'] else 1)
