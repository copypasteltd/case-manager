"""Verify implementation, actual executions, media, retained history and open scope."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess

APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent
E=ROOT/'docs/quality/evidence/finance-fx'
PUBLISHED='624c95d86f737e679e11af4fbb505dedf8e27b71'

def read(path):return json.loads(path.read_text(encoding='utf-8-sig'))
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def module(name):
    spec=importlib.util.spec_from_file_location('finance_fx_check_'+name,APP/'scripts'/name)
    out=importlib.util.module_from_spec(spec);spec.loader.exec_module(out);return out
def digest(names):
    h=hashlib.sha256()
    for name in names:h.update(name.encode()+b'\0'+(APP/name).read_bytes()+b'\0')
    return h.hexdigest()

def main():
    checks=[]
    def check(name,passed):checks.append({'name':name,'passed':bool(passed)})
    migrations=sorted((APP/'backend/migrations').glob('*.sql'))
    check('Migrations1-80 retain published bytes;81 is an append-only upgrade',len(migrations)==81 and all(
        p.read_bytes()==subprocess.check_output(['git','show',PUBLISHED+':app/backend/migrations/'+p.name],cwd=ROOT) for p in migrations[:-1]))
    sql=migrations[-1].read_text()
    check('Database binds immutable quote content, intervals and explicit company base policy',all(v in sql for v in ['finance_fx_book_immutable','finance_fx_book_binding','finance_base_currency_policy','content_sha256','previous_end','scope_line_half_up']))
    api=read(ROOT/'docs/development/openapi.json')
    check('Generated API exposes explicit book selection and human publishing with embedded parity',
        all(v in api['paths'] for v in ['/finance/fx-policy','/finance/fx-books','/finance/fx-books/{id}'])
        and 'fx_book_id' in api['components']['schemas']['FinancialReportQuery']['properties']
        and (ROOT/'docs/development/openapi.json').read_bytes()==(APP/'backend/internal/contract/openapi.json').read_bytes())
    backend=read(E.parent/'finance-fx-backend/result.json');backend_source=module('verify_backend_batches.py').digest()
    discovered=set(module('verify_backend_batches.py').discover())
    observed={(v['Package'],v['Test']) for v in backend['checks'] if '/' not in v['Test']}
    check('Every discovered current-source backend test actually passed on native PostgreSQL',backend['passed'] and backend['source_sha256']==backend_source and backend['source_unchanged_during_tests'] and backend['all_discovered_tests_executed'] and observed==discovered and len(discovered)==backend['discovered_top_level_tests'] and all(v['Action']=='pass' for v in backend['checks']))
    retry_ok=True
    if backend.get('retries'):
        folder=E.parent/'finance-fx-backend';first=backend['first_attempt'];original=read(folder/first['record'])
        retried={(v['package'],v['test']) for v in backend['retries']}
        failed={(v['Package'],v['Test'].split('/')[0]) for v in original['checks'] if v['Action']!='pass'}
        old_observed={(v['Package'],v['Test']) for v in original['checks'] if '/' not in v['Test']}
        retained=[v for v in original['checks'] if (v['Package'],v['Test'].split('/')[0]) not in retried]
        retry_ok=(not original['passed'] and original['source_sha256']==backend_source and original['source_unchanged_during_tests'] and sha(folder/first['record'])==first['sha256'] and retried==failed|(discovered-old_observed) and len(retained)==backend['retained_passed_result_records'] and backend['checks']==retained+[v for r in backend['retries'] for v in r['checks']] and backend['verification_script_sha256']==sha(APP/'scripts/retry_finance_fx_backend.py') and all(r['passed'] and r['exit_code']==0 and r['source_sha256']==backend_source and r['source_unchanged'] and sha(APP/'.local'/Path(r['raw_output']).name)==r['raw_output_sha256'] for r in backend['retries']))
    check('Any failed first attempt and exact serial retry selection remain inspectable on the same backend source',retry_ok)
    fx=[v for v in backend['checks'] if v['Test'].startswith('TestFinancialReportsAndExports/fx_')]
    check('Ten FX authority, arithmetic, history, concurrency, reversal and CSV journeys plus exact math pass',len(fx)==10 and any(v['Test']=='TestExactFXArithmetic' for v in backend['checks']))
    clients=read(E/'client-checks.json')
    check('Eight fresh client commands retain exact input digests and private original logs',clients['passed'] and clients['source_unchanged'] and clients['source_sha256']==digest(clients['source_files']) and len(clients['checks'])==8 and all(v['passed'] and sha(APP/'.local'/Path(v['raw_log']).name)==v['raw_log_sha256'] for v in clients['checks']))
    web,mobile,runtime=[read(E/name) for name in ['web-build.json','mobile-build.json','runtime-build.json']]
    mobile_source=module('verify_mobile.py').digest()[0]
    check('Web151/mobile255/runtime52 proofs bind current inputs',web['passed'] and web['source_sha256']==digest(web['sources']) and web['tests']['numPassedTests']==151 and mobile['passed'] and mobile['source_sha256']==mobile_source and mobile['tests']['pass']==255 and runtime['status']=='passed' and runtime['source_sha256']==module('verify_agent_runtime.py').digest()[0] and runtime['tests']['pass']==52)
    check('Both mini-program targets compile20 actual registered pages with current route output',all(v['page_count']==20 and sha(APP/'mobile/dist'/target/'app.json')==v['app_json_sha256'] for target,v in mobile['targets'].items()))
    build,apk,jvm,feature=[read(E/name) for name in ['build.json','apk-inspection.json','android-test-scope.json','android-feature-content.json']]
    check('New Android bundle and APK bind current sources; package and FX content inspections pass',build['passed'] and build['source_unchanged_during_build'] and build['source_sha256']==module('build_android.py').sources()[0] and all(sha(APP/v['path'])==v['sha256'] for v in build['artifacts'].values()) and apk['passed'] and len(apk['checks'])==18 and feature['passed'] and len(feature['checks'])==4 and not feature['artifact_executed'])
    check('JVM scope preserves actual task/cache status and original XML bytes without claiming a device run',jvm['passed'] and jvm['freezer_sha256']==sha(APP/'scripts/freeze_finance_fx_android.py') and (not jvm['gradle_cached'] or jvm['xml_sha256']==read(E.parent/'oa-prose/android-test-scope.json')['xml_sha256']) and jvm['tests']==26 and jvm['new_test_execution']==(not jvm['gradle_cached']) and not jvm['device_executed'] and sha(E/'native-build.log')==jvm['native_log_sha256'] and all(sha(E/jvm['xml_artifact_directory']/name)==value for name,value in jvm['xml_sha256'].items()))
    linux=read(E/'linux-build.json')
    check('Linux binary is a current source-bound cross-build without deployment',linux['passed'] and linux['source_sha256']==linux['source_after_sha256']==backend_source and sha(APP/'.local/finance-fx-platform-linux-amd64')==linux['artifact']['sha256'] and not linux['artifact_executed'] and not linux['deployed'])
    interaction=module('finance_fx_sources.py').digest()
    browser_ok=[]
    for client in ['web','mobile']:
        folder=E.parent/('finance-fx-'+client)
        result,manifest,capture=[read(folder/name) for name in ['result.json','source-manifest.json','capture.json']]
        browser_ok.append(result['passed'] and result['source_unchanged'] and result['source_sha256']==manifest['sha256']==digest(manifest['files']) and result['build_source_unchanged'] and result['build_source_sha256']==result['build_source_after_sha256']==interaction and result['owned_build_process_stopped'] and result['owned_browser_runner_stopped'] and result['owned_browser_closed'] and result['browser_contexts_closed']==1 and result['owned_review_server_stopped'] and result['isolated_database_cleanup_passed'] and result['native_fixture_exit_code']==0 and len(result['checks'])==3 and not result['console_errors'] and not result['server_errors'] and capture['synthetic_data'] and {v['width'] for v in capture['layouts']}=={390,768,1440})
    check('Web and shared Taro H5 perform actual selected-book, publication and pinned export journeys and close owned resources',all(browser_ok))
    media=read(E/'media.json')
    check('Two real-pixel capture manifests bind actual MP4 and GIF outputs',media['passed'] and media['encoder_script_sha256']==sha(APP/'scripts/encode_finance_fx_media.py') and len(media['recordings'])==2 and all(sha(ROOT/v['capture_manifest'])==v['capture_manifest_sha256'] and v['build_source_sha256']==interaction and v['capture_frame_count']>=5 and all(sha(ROOT/a['file'])==a['sha256'] and (ROOT/a['file']).stat().st_size==a['bytes'] for a in v['artifacts']) for v in media['recordings']))
    visual=read(E/'visual-review.json')
    check('Four reviewed screenshots are actual synthetic business captures with matching bytes',visual['passed'] and visual['synthetic_data'] and len(visual['screenshots'])==4 and all(sha(ROOT/v['file'])==v['sha256'] for v in visual['screenshots']))
    reused=read(E/'reused-evidence.json')
    blobs=subprocess.check_output(['git','hash-object','--',*[v['file'] for v in reused['media']]],cwd=ROOT).decode().splitlines()
    check('All181 prior media and retained audit/JVM records remain exact published bytes',reused['passed'] and reused['published_commit']==PUBLISHED and len(reused['media'])==181 and blobs==[v['git_blob'] for v in reused['media']] and all(sha(ROOT/v['file'])==v['sha256'] for v in reused['media']+reused['records']))
    development=read(E/'development-project-resources.json');upgrade=read(E/'development-update.json')
    check('Owned native development schema81 and services are current, integrations disabled and test databases removed',development['passed'] and development['migration_count']==81 and development['backend_source_sha256']==backend_source and development['database']['disposable_test_databases']==0 and development['integrations_disabled'] and upgrade['passed'] and upgrade['legacy_and_usage_rows_preserved'])
    cleanup=read(E/'interrupted-fixture-cleanup.json')
    check('The exact interrupted synthetic fixture was removed after ledger, seed, creation-window and connection checks',cleanup['passed'] and cleanup['removed'] and cleanup['creation_within_interrupted_case'] and cleanup['source_migration_ledger_matched'] and cleanup['restoration_fixture_shape_verified'] and cleanup['before']['connections']==0 and not cleanup['forced_disconnect'] and not cleanup['postgres_restarted'] and not cleanup['production_database_touched'] and cleanup['script_sha256']==sha(APP/'scripts/cleanup_finance_fx_interrupted_fixture.py'))
    timed=read(E/'timeout-fixture-cleanup.json')
    check('The invalid synthetic remnant matches its original DROP log and failed case, with no contents-read claim',timed['passed'] and timed['removed'] and timed['remaining_test_databases']==0 and timed['creation_within_failed_case'] and timed['invalid_database_marker_verified'] and timed['before']['connection_limit']==-2 and timed['before']['connections']==0 and timed['drop_statement_time_matches_failure'] and timed['published_fixture_source_matched'] and timed['suite_source_sha256']==backend_source and timed['exact_original_drop_statement'] in (APP/'.local/postgres.log').read_text(encoding='utf-8',errors='replace') and not timed['contents_accessible'] and not timed['ledger_read_claimed'] and not timed['forced_disconnect'] and not timed['postgres_restarted'] and not timed['production_database_touched'] and timed['script_sha256']==sha(APP/'scripts/cleanup_finance_fx_timeout_fixture.py'))
    skills=[read(p) for p in sorted((APP/'skills').glob('*/manifest.json'))]
    check('Eight Skills remain draft; complete goal and production acceptance remain open',len(skills)==8 and all(v['state']=='draft' for v in skills))
    with socket.socket() as probe:
        probe.settimeout(1);closed=probe.connect_ex(('127.0.0.1',5189))!=0
    check('Owned review server port5189 is released',closed)
    report={'passed':all(v['passed'] for v in checks),'checked_at':datetime.now(timezone.utc).isoformat(),'checks':checks,'verification_script_sha256':sha(Path(__file__)),'backend_source_sha256':backend_source,'mobile_source_sha256':mobile_source,'interaction_kind':'explicit_finance_fx_valuation','interaction_source_sha256':interaction,'full_goal_complete':False,'production_ready':False}
    (E/'final-source-check.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':report['passed'],'checks':len(checks),'failed':[v['name'] for v in checks if not v['passed']]}))
    return 0 if report['passed'] else 1

if __name__=='__main__':raise SystemExit(main())
