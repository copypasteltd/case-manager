"""Bind notification recovery to executed checks, exact artifacts and declared retained evidence."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess
import zipfile

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/notification-retry'
PUBLISHED = 'b860e36a104ae215f3fd79eac5ac09cb977f37a4'


def read(p):
    return json.loads(p.read_text(encoding='utf-8'))


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def digest(names, previous=None):
    h = hashlib.sha256()
    for name in names:
        file = previous.get(name, APP / name) if previous else APP / name
        h.update(name.encode() + b'\0' + file.read_bytes() + b'\0')
    return h.hexdigest()


def module(name):
    spec = importlib.util.spec_from_file_location('audit_proof_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec); spec.loader.exec_module(value)
    return value


def main():
    checks = []
    def check(name, value):
        checks.append({'name':name, 'passed':bool(value)})
    backend_source = module('verify_backend_batches').digest()
    group = E.parent / 'notification-retry-backend'
    backend, progress, raw = [read(group / name) for name in ('result.json', 'batch-progress.json', 'raw-log-archive.json')]
    expected = {tuple(v) for v in progress['expected_tests']}
    observed = {(v['Package'],v['Test']) for v in backend['checks'] if '/' not in v['Test']}
    check('Every discovered current Go/SQL/contract test passes without skips', backend['passed'] and backend['verification_script_sha256'] == sha(APP / 'scripts/verify_notification_retry_backend.py') and backend['parallel_batches'] == 1 and backend['source_unchanged_during_tests'] and backend['source_sha256'] == backend_source and expected == observed and backend['all_discovered_tests_executed'] and all(v['Action'] == 'pass' for v in backend['checks']))
    check('Final batch records retain exact run-specific private logs', raw['passed'] and raw['source_sha256'] == backend_source and raw['records'] == len(backend['checks']) and len(raw['logs']) == sum(len(v.get('evidence_segments',[None])) for v in progress['batches']) and raw['freezer_sha256'] == sha(APP / 'scripts/freeze_notification_retry_backend.py') and all(sha(APP / v['raw_log']) == v['sha256'] and (APP / v['raw_log']).stat().st_size == v['bytes'] for v in raw['logs']))
    retry = backend.get('cleanup_retry')
    if retry:
        initial_path = ROOT / retry['initial_record']
        initial, attempt = read(initial_path), read(initial_path.parent / 'attempt.json')
        check('Only archived cleanup-timeout families rerun at the same source; successful original batches retain provenance', not initial['passed'] and initial['source_unchanged_during_tests'] and initial['source_sha256'] == backend_source and sha(initial_path) == retry['initial_record_sha256'] and sha(initial_path.parent / 'attempt.json') == retry['attempt_record_sha256'] and retry['verification_script_sha256'] == sha(APP / 'scripts/merge_notification_retry_native_backend.py') and retry['same_source_sha256'] == backend_source and attempt['failed_batches'] == retry['serial_retried_batches'] and retry['retained_successful_batches'] + len(retry['serial_retried_batches']) == len(progress['batches']) and all(sha(APP / v['file']) == v['sha256'] for v in attempt['raw_logs']))
        additional_path = ROOT / retry['additional_attempt_record']
        additional = read(additional_path)
        native_path = ROOT / retry['native_result_record']
        native = read(native_path)
        cluster = read(E / 'native-cluster.json')
        closed_cluster = read(E / 'native-cluster-closed.json')
        check('Both failed native attempts remain failed and the actual unchanged integration family passes on a fully durable owned native PG cluster', sha(additional_path) == retry['additional_attempt_sha256'] and not additional['passed'] and additional['complete'] and additional['source_sha256'] == backend_source and additional['first_retry_script_sha256'] == sha(APP / 'scripts/retry_notification_retry_backend_cleanup.py') and all(sha(APP / v['file']) == v['sha256'] for v in additional['raw_logs']) and sha(native_path) == retry['native_result_sha256'] and native['passed'] and native['exit_code'] == 0 and native['source_sha256'] == backend_source and native['source_unchanged_during_tests'] and native['verification_script_sha256'] == sha(APP / 'scripts/run_notification_retry_native_family.py') and sha(APP / native['raw_log']) == native['raw_log_sha256'] and native['native_cluster_record_sha256'] == sha(E / 'native-cluster.json') and len(native['checks']) == 11 and all(v['Action'] == 'pass' for v in native['checks']))
        check('Temporary native PG keeps all durability flags and deadlines, then closes with zero disposable databases', cluster['passed'] and all(cluster[k] for k in ('loopback_only','temporary_cluster','fsync','synchronous_commit','full_page_writes')) and not any(cluster[k] for k in ('original_development_cluster_modified','fixture_cleanup_deadline_changed','virtualization_used')) and cluster['verification_script_sha256'] == sha(APP / 'scripts/notification_retry_native_cluster.py') == closed_cluster['verification_script_sha256'] and closed_cluster['passed'] and closed_cluster['owned_cluster_stopped'] and closed_cluster['development_cluster_preserved'] and closed_cluster['remaining_disposable_databases'] == 0)
        diagnostic = read(E / 'native-cleanup-diagnostic.json')
        check('Cleanup diagnostic preserves only the exact two-line checkpoint excerpt and does not claim a benchmark or altered deadline', diagnostic['passed'] and diagnostic['verification_script_sha256'] == sha(APP / 'scripts/retain_notification_retry_cleanup_diagnostic.py') and sha(ROOT / diagnostic['private_excerpt']) == diagnostic['private_excerpt_sha256'] and diagnostic['checkpoint_total_seconds'] == 132.018 and diagnostic['checkpoint_sync_seconds'] == 128.318 and diagnostic['unchanged_drop_context_seconds'] == 120 and not any(diagnostic[k] for k in ('full_server_log_published','database_durability_changed','test_deadline_changed','network_performance_test')))
    names = {v['Test'] for v in backend['checks']}
    required = {'TestNotificationManualRetries'}
    check('Fourteen real authority, fixed-input, worker, deduplication, current recipient, concurrency, database and rollback scenarios execute', required <= names and len([v for v in names if v.startswith('TestNotificationManualRetries/')]) == 14)
    attempts = E / 'attempts'
    focused = [read(attempts / folder / 'result.json') for folder in ('01-fresh-install-role','02-cursor-status','04-focused-before-final-list')]
    expected_results = [False,False,True]
    archive_paths = [APP / '.local/notification-retry-attempts' / folder / 'source.zip' for folder in ('focused01','focused02','focused03')]
    focus_logs = [APP / '.local/notification-retry-attempts/focused01/notification-retry-focused-01.log', APP / '.local/notification-retry-attempts/focused02/notification-retry-focused-02.log', APP / '.local/notification-retry-attempts/focused03/notification-retry-focused-03.log']
    check('Earlier focused failures and pre-final success retain their distinct results, exact source archives and raw logs', [v['passed'] for v in focused] == expected_results and all(sha(p) == v['archive_sha256'] and sha(log) == v['raw_log_sha256'] for v,p,log in zip(focused,archive_paths,focus_logs)))
    previous_clients = []
    for folder,private in [('03-client-composite-include','clients01'),('05-clients-before-live-status','clients02-before-live-status')]:
        attempt = read(attempts / folder / 'attempt.json'); record = read(attempts / folder / 'client-checks.json')
        directory = APP / '.local/notification-retry-attempts' / private
        valid = sha(directory / 'source.zip') == attempt['archive_sha256'] and all(sha(directory / Path(v['raw_log']).name) == v['raw_log_sha256'] for v in record['checks'])
        previous_clients.append(valid and attempt['passed'] == record['passed'])
    check('Composite-project failure and pre-polling client pass remain exact earlier attempts; final clients rerun all eight commands', all(previous_clients))
    scheduling = attempts / '06-browser-prebuild-scheduling'
    archived = read(scheduling / 'attempt.json')
    previous_web = read(scheduling / 'web-before-runner-change/result.json')
    previous_manifest = read(scheduling / 'web-before-runner-change/source-manifest.json')
    previous_digest = hashlib.sha256()
    with zipfile.ZipFile(ROOT / archived['source_archive']) as archive:
        for name in previous_manifest['files']:
            previous_digest.update(name.encode() + b'\0' + archive.read(name) + b'\0')
    check('H5 fixture timeout remains failed and prior Web evidence retains exact pre-runner sources and logs', not archived['passed'] and archived['native_fixture_exit_code'] == 1 and not archived['browser_context_created'] and archived['owned_review_server_stopped'] and sha(ROOT / archived['source_archive']) == archived['archive_sha256'] and all(sha(ROOT / v['file']) == v['sha256'] for v in archived['raw_logs']) and previous_web['passed'] and previous_web['source_sha256'] == previous_manifest['sha256'] == previous_digest.hexdigest())
    cleanup_attempt = read(attempts / '07-web-cleanup-timeout/attempt.json')
    failed_web = read(attempts / '07-web-cleanup-timeout/web-browser-result/result.json')
    check('The browser pass with a failed native cleanup remains a failed complete attempt with exact source and raw logs', not cleanup_attempt['passed'] and not failed_web['passed'] and failed_web['native_fixture_exit_code'] == 1 and len(failed_web['checks']) == 5 and failed_web['owned_browser_closed'] and sha(ROOT / cleanup_attempt['source_archive']) == cleanup_attempt['archive_sha256'] and all(sha(ROOT / v['file']) == v['sha256'] for v in cleanup_attempt['raw_logs']))
    interrupted = read(attempts / '08-interrupted-native-build/attempt.json')
    check('Interrupted concurrent native packaging retains its exact inputs and log without a pass claim', not interrupted['passed'] and interrupted['status'] == 'interrupted' and not interrupted['new_artifact_claimed'] and interrupted['other_processes_preserved'] and sha(ROOT / interrupted['source_archive']) == interrupted['archive_sha256'] and sha(ROOT / interrupted['raw_log']) == interrupted['raw_log_sha256'])
    init = read(attempts / '09-native-init-path/attempt.json')
    check('Initial Unicode-path initdb failure retains exact helpers and private diagnostics without a started-service claim', not init['passed'] and not init['native_service_started'] and all(sha(ROOT / v['file']) == v['sha256'] for v in init['source_files']) and sha(ROOT / init['private_log']) == init['private_log_sha256'])
    locator_path = attempts / '11-h5-locator'
    locator = read(locator_path / 'attempt.json')
    locator_sources = []
    with zipfile.ZipFile(ROOT / locator['source_archive']) as archive:
        for client in ('web','mobile'):
            directory = locator_path / (client + '-before-locator-change')
            result, manifest = read(directory / 'result.json'), read(directory / 'source-manifest.json')
            old_digest = hashlib.sha256()
            for name in manifest['files']:
                old_digest.update(name.encode() + b'\0' + archive.read(name) + b'\0')
            locator_sources.append(result['source_sha256'] == manifest['sha256'] == old_digest.hexdigest() and result['passed'] == (client == 'web') and result['native_fixture_exit_code'] == 0 and result['owned_browser_closed'] and result['owned_review_server_stopped'] and result['isolated_database_cleanup_passed'])
    check('H5 locator failure and pre-helper Web pass retain exact closed-native attempts; both final clients rerun', not locator['passed'] and not locator['application_source_changed'] and locator['native_fixture_cleanups_passed'] and locator['browser_contexts_closed'] and all(locator_sources) and sha(ROOT / locator['source_archive']) == locator['archive_sha256'] and all(sha(ROOT / v['file']) == v['sha256'] for v in locator['raw_logs']) and locator['verification_script_sha256'] == sha(APP / 'scripts/archive_notification_retry_locator_attempt.py'))
    predicate_directory = attempts / '12-android-content-predicate'
    predicate = read(predicate_directory / 'attempt.json')
    failed_predicate = read(predicate_directory / 'android-feature-content.json')
    check('Incorrect unused-client-literal APK predicate retains its exact failure and helper without an application-change or rebuild claim', not predicate['passed'] and not failed_predicate['passed'] and not predicate['application_source_changed'] and not predicate['apk_rebuilt_for_predicate_change'] and sha(ROOT / predicate['source_file']) == predicate['source_sha256'] == failed_predicate['verification_script_sha256'] and sha(predicate_directory / 'android-feature-content.json') == predicate['failed_record_sha256'] and [v['passed'] for v in failed_predicate['checks']] == [True,False,True,True])
    current = read(ROOT / 'docs/development/openapi.json')
    old = json.loads(subprocess.check_output(['git','show',PUBLISHED + ':docs/development/openapi.json'], cwd=ROOT))
    check('All prior320 API paths and698 models remain exact; three recovery paths and seven models are added', (ROOT / 'docs/development/openapi.json').read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes() and len(current['paths']) == 323 and sum(k in ('get','post','put','patch','delete','head','options','trace') for p in current['paths'].values() for k in p) == 398 and len(current['components']['schemas']) == 705 and all(current['paths'][k] == v for k,v in old['paths'].items()) and all(current['components']['schemas'][k] == v for k,v in old['components']['schemas'].items()))
    clients = read(E / 'client-checks.json')
    check('Final client command summaries bind current inputs and exact original logs', clients['passed'] and clients['source_unchanged'] and clients['source_sha256'] == digest(clients['source_files']) and len(clients['checks']) == 8 and all(v['passed'] and v['exit_code'] == 0 and sha(APP / '.local' / Path(v['raw_log']).name) == v['raw_log_sha256'] for v in clients['checks']))
    web, mobile, runtime = [read(E / n) for n in ('web-build.json','mobile-build.json','runtime-build.json')]
    mobile_source = module('verify_mobile').digest()[0]
    check('Web195, mobile289 and runtime55 results match their current implementation scopes', web['passed'] and all(v['derived_from']['sha256'] == sha(E / 'client-checks.json') and v['derived_from']['summary_adapter_sha256'] == sha(APP / 'scripts/freeze_notification_retry_clients.py') for v in (web,mobile,runtime)) and web['source_sha256'] == digest(web['sources']) and web['tests']['numPassedTests'] == 195 and mobile['passed'] and mobile['source_sha256'] == mobile_source and mobile['tests'] == {'pass':289,'fail':0,'cancelled':0,'skipped':0} and runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime').digest()[0] and runtime['tests']['pass'] == 55)
    check('Both actual mini-program builds retain20 configured pages', all(v['page_count'] == 20 and sha(APP / 'mobile/dist' / target / 'app.json') == v['app_json_sha256'] for target,v in mobile['targets'].items()) and set(mobile['targets']) == {'weapp','tt'})
    build, inspection, feature, jvm = [read(E / n) for n in ('build.json','apk-inspection.json','android-feature-content.json','android-test-scope.json')]
    check('Fresh native APK binds exact current source and bundle bytes with18 plus4 static checks', build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android').sources()[0] and inspection['passed'] and feature['passed'] and len(inspection['checks']) == 18 and len(feature['checks']) == 4 and not feature['artifact_executed'] and not feature['real_device'] and feature['verification_script_sha256'] == sha(APP / 'scripts/verify_notification_retry_android_content.py') and all(sha(APP / a['path']) == a['sha256'] for a in build['artifacts'].values()))
    check('JVM26 results disclose actual Gradle cache status and retain original XML bytes', jvm['passed'] and jvm['tests'] == 26 and jvm['new_test_execution'] == (not jvm['gradle_cached']) and not jvm['device_executed'] and sha(E / 'native-build.log') == jvm['native_log_sha256'] and len(jvm['xml_sha256']) == 4 and all(sha(E / 'android-unit-results' / k) == v for k,v in jvm['xml_sha256'].items()))
    linux = read(E / 'linux-build.json')
    check('Both current Linux artifacts cross-compile without claiming Linux execution', linux['passed'] and linux['verification_script_sha256'] == sha(APP / 'scripts/build_notification_retry_linux.py') and linux['source_sha256'] == linux['source_after_sha256'] == backend_source and len(linux['artifacts']) == 2 and all(sha(APP / a['file']) == a['sha256'] for a in linux['artifacts']) and not linux['artifact_executed'] and not linux['kernel_boundary_executed'] and not linux['deployed'])
    browser_sources = module('notification_retry_sources').digest()
    for client in ('web','mobile'):
        directory = E.parent / ('notification-retry-' + client)
        browser, manifest, capture = [read(directory / n) for n in ('result.json','source-manifest.json','capture.json')]
        check('Actual ' + client + ' browser/API journey passes and all owned resources close', browser['passed'] and len(browser['checks']) == 5 and browser['source_unchanged'] and browser['source_sha256'] == manifest['sha256'] == digest(manifest['files']) and browser['build_source_sha256'] == browser['build_source_after_sha256'] == browser_sources and browser['build_source_unchanged'] and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['owned_api_proxy_stopped'] and browser['h5_prebuilt_before_fixture'] == (client == 'mobile') and browser['real_api_forwarded_without_response_substitution'] == (client == 'mobile') and browser['native_fixture_exit_code'] == 0 and browser['isolated_database_cleanup_passed'] and not browser['console_errors'] and not browser['server_errors'] and all(sha(ROOT / s['file']) == s['sha256'] for s in capture['screenshots']))
    media = read(E / 'media.json')
    check('Both silent MP4/GIF pairs encode exact authenticated synthetic frame manifests', media['passed'] and len(media['recordings']) == 2 and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_notification_retry_media.py') and all(sha(ROOT / r['capture_manifest']) == r['capture_manifest_sha256'] and all(sha(ROOT / a['file']) == a['sha256'] for a in r['artifacts']) for r in media['recordings']))
    visual = read(E / 'visual-review.json')
    check('Visual review binds all seven final screenshots and both decoded GIF samples', visual['reviewed'] and len(visual['screenshots']) == 7 and len(visual['gifs']) == 2 and all(sha(ROOT / v['file']) == v['sha256'] for v in visual['screenshots'] + visual['gifs']) and set(visual['gif_frames_reviewed']) == {'web','mobile'})
    retained = read(E / 'reused-evidence.json')
    check('222 previous media and unchanged dependency audit retain exact published bytes and original date', retained['passed'] and retained['published_commit'] == PUBLISHED and len(retained['media']) == 222 and not retained['new_dependency_audit'] and retained['package_lock_sha256'] == sha(APP / 'package-lock.json') and all(sha(ROOT / v['file']) == v['sha256'] for v in [*retained['media'],*retained['records']]) and not subprocess.check_output(['git','diff','--name-only',PUBLISHED,'--','docs/quality/evidence/dependency-integrity'], cwd=ROOT).strip())
    cleanup = read(E / 'fixture-cleanup.json'); plan = read(E / 'fixture-cleanup-plan.json')
    check('One exact inactive invalid synthetic remnant is attributed and removed without FORCE; original failed input remains exact', cleanup['passed'] and len(cleanup['removed']) == 1 and cleanup['remaining_disposable_databases'] == 0 and not cleanup['force_used'] and cleanup['development_database_preserved'] and not cleanup['database_cluster_restarted'] and cleanup['plan_sha256'] == sha(E / 'fixture-cleanup-plan.json') and cleanup['verification_script_sha256'] == sha(APP / 'scripts/cleanup_notification_retry_fixtures.py') and plan['backend_record_sha256'] == sha(E / 'fixture-cleanup-input.json') == sha(group / 'attempts/01-native-cleanup-timeout/result.json') and all(sha(ROOT / v['file']) == v['sha256'] for v in plan['raw_logs']) and all(v['invalid_drop_marker'] and v['active_connections'] == 0 and not v['contents_inspected'] and v['matched_failed_tests'] for v in cleanup['removed']))
    second_directory = attempts / '10-second-owned-fixture-cleanup'
    second_cleanup = read(second_directory / 'fixture-cleanup.json'); second_plan = read(second_directory / 'fixture-cleanup-plan.json')
    check('The second separately attributed invalid remnant is removed without FORCE or development restart', second_cleanup['passed'] and len(second_cleanup['removed']) == 1 and second_cleanup['remaining_disposable_databases'] == 0 and not second_cleanup['force_used'] and second_cleanup['development_database_preserved'] and not second_cleanup['database_cluster_restarted'] and second_cleanup['plan_sha256'] == sha(second_directory / 'fixture-cleanup-plan.json') and second_cleanup['verification_script_sha256'] == sha(APP / 'scripts/cleanup_notification_retry_second_fixture.py') and second_plan['backend_record_sha256'] == sha(second_directory / 'fixture-cleanup-input.json') and second_plan['browser_attempt_sha256'] == sha(attempts / '07-web-cleanup-timeout/attempt.json') and all(sha(ROOT / v['file']) == v['sha256'] for v in second_plan['raw_logs']) and all(v['invalid_drop_marker'] and v['active_connections'] == 0 and not v['contents_inspected'] and v['matched_failed_tests'] for v in second_cleanup['removed']))
    dev = read(E / 'development-project-resources.json'); update = read(E / 'development-update.json')
    check('Native schema85 upgrade preserves historical rows and current owned services, with no fixture databases or integrations enabled', dev['passed'] and dev['migration_count'] == 85 and dev['backend_source_sha256'] == backend_source and dev['database']['disposable_test_databases'] == 0 and dev['integrations_disabled'] and all(dev['ownership'].values()) and dev['worker_sources_match'] and update['passed'] and update['legacy_and_usage_rows_preserved'] and update['backend_source_sha256'] == backend_source and update['previous_migration_count'] == 84 and update['migration_count'] == 85 and not update['production_deployment'] and update['updater_sha256'] == sha(APP / 'scripts/update_notification_retry_development.py') and dev['verification_script_sha256'] == sha(APP / 'scripts/verify_notification_retry_development.py'))
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('23 active packages,5 exclusions and8 draft Skills retain AI authorization and isolation without fee controls', scope['counts']['active'] == 23 and scope['counts']['excluded'] == 5 and not scope['flags']['ai_cost_control_required'] and scope['flags']['ai_execution_isolation_required'] and scope['flags']['ai_authorization_required'] and len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    closed = True
    for port in (5189,5190):
        with socket.socket() as probe:
            probe.settimeout(1)
            closed = closed and probe.connect_ex(('127.0.0.1',port)) != 0
    check('Owned browser fixture file, review port and API forwarding port are released', closed and not (APP / '.local/business-browser-fixture.json').exists())
    proof = {'passed':all(v['passed'] for v in checks), 'checked_at':datetime.now(timezone.utc).isoformat(), 'checks':checks,
        'interaction_kind':'organization_notification_retry', 'interaction_source_sha256':browser_sources,
        'mobile_source_sha256':mobile_source, 'backend_source_sha256':backend_source,
        'verification_script_sha256':sha(Path(__file__)), 'supporting_scripts':{name:sha(APP / 'scripts' / name) for name in ('verify_notification_retry_backend.py','freeze_notification_retry_backend.py','verify_notification_retry_clients.py','freeze_notification_retry_clients.py','build_notification_retry_linux.py','freeze_notification_retry_android.py','retain_notification_retry_evidence.py')}, 'historical_evidence_commit':PUBLISHED,
        'new_browser_capture':True, 'new_backend_test_execution':True, 'full_goal_complete':False, 'production_ready':False}
    (E / 'final-source-check.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed':proof['passed'], 'checks':len(checks), 'failed':[v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
