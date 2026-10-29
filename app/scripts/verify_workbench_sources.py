"""Bind complete current business-workbench evidence; retained evidence keeps its actual scope."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/workbench'
PUBLISHED = '094efc76bc3ef48336fdfa149affb8e49c932c75'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def module(name):
    spec = importlib.util.spec_from_file_location('comparison_proof_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def main():
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    backend_source = module('verify_backend_batches').digest()
    group = E.parent / 'workbench-backend'
    backend, progress, raw = [read(group / name) for name in ('result.json', 'batch-progress.json', 'raw-log-archive.json')]
    expected = {tuple(v) for v in progress['expected_tests']}
    observed = {(v['Package'], v['Test']) for v in backend['checks'] if '/' not in v['Test']}
    check('Every discovered current-source native backend test executes without skips', backend['passed'] and backend['source_sha256'] == backend_source and backend['source_unchanged_during_tests'] and backend['all_discovered_tests_executed'] and expected == observed and backend['discovered_top_level_tests'] == len(expected) and backend['parallel_batches'] == 1 and all(v['Action'] == 'pass' for v in backend['checks']) and backend['verification_script_sha256'] == sha(APP / 'scripts/verify_workbench_backend.py'))
    check('Complete successful batches retain exact private raw logs without substituted cases', raw['passed'] and raw['source_sha256'] == backend_source and len(raw['logs']) == len(progress['batches']) and raw['records'] == len(backend['checks']) and raw['freezer_sha256'] == sha(APP / 'scripts/freeze_workbench_backend.py') and all(sha(APP / v['raw_log']) == v['sha256'] and not v['excluded_test_families'] for v in raw['logs']))
    feature = [v for v in backend['checks'] if v['Test'].startswith('TestBusinessWorkbench/')]
    check('Ten actual business summary API and native PG groups pass', len(feature) == 10 and all(v['Action'] == 'pass' for v in feature) and backend['discovered_top_level_tests'] == 235)
    cluster, closed = [read(E / n) for n in ('native-cluster.json', 'native-cluster-closed.json')]
    check('Owned full-durability native test PG closes with no disposable databases', cluster['passed'] and closed['passed'] and all(cluster[n] for n in ('fsync', 'synchronous_commit', 'full_page_writes')) and not cluster['original_development_cluster_modified'] and not cluster['fixture_cleanup_deadline_changed'] and not cluster['virtualization_used'] and closed['owned_cluster_stopped'] and closed['remaining_disposable_databases'] == 0 and cluster['verification_script_sha256'] == closed['verification_script_sha256'] == sha(APP / 'scripts/workbench_native_cluster.py'))
    contract = read(APP / 'backend/internal/contract/openapi.json')
    original = json.loads(subprocess.check_output(['git', 'show', PUBLISHED + ':docs/development/openapi.json'], cwd=ROOT))
    check('All prior API paths and schemas retain semantics; business workbench adds one path and four schemas', all(contract['paths'][k] == v for k, v in original['paths'].items()) and all(contract['components']['schemas'][k] == v for k, v in original['components']['schemas'].items()) and len(contract['paths']) == 327 and sum(len(v) for v in contract['paths'].values()) == 403 and len(contract['components']['schemas']) == 721 and contract == read(ROOT / 'docs/development/openapi.json'))
    client = read(E / 'client-checks.json')
    check('Eight client commands run against unchanged exact source with retained raw hashes', client['passed'] and client['source_unchanged'] and client['source_sha256'] == client['source_after_sha256'] == digest(client['source_files']) and len(client['checks']) == 8 and all(v['passed'] and v['exit_code'] == 0 and sha(APP / '.local' / Path(v['raw_log']).name) == v['raw_log_sha256'] for v in client['checks']))
    web, mobile, runtime = [read(E / n) for n in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    mobile_source = module('verify_mobile').digest()[0]
    check('Web257 mobile351 runtime55 match current complete sources and actual command records', web['passed'] and web['tests']['numPassedTests'] == 257 and web['source_sha256'] == digest(web['sources']) and mobile['passed'] and mobile['tests'] == {'pass': 351, 'fail': 0, 'cancelled': 0, 'skipped': 0} and mobile['source_sha256'] == mobile_source and runtime['status'] == 'passed' and runtime['tests']['pass'] == 55 and runtime['source_sha256'] == module('verify_agent_runtime').digest()[0] and all(v['derived_from']['sha256'] == sha(E / 'client-checks.json') for v in (web, mobile, runtime)))
    check('Both built miniapps retain all20 routes', set(mobile['targets']) == {'weapp', 'tt'} and all(v['page_count'] == 20 and sha(APP / 'mobile/dist' / k / 'app.json') == v['app_json_sha256'] for k, v in mobile['targets'].items()))
    build, inspection, content, jvm = [read(E / n) for n in ('build.json', 'apk-inspection.json', 'android-feature-content.json', 'android-test-scope.json')]
    check('New development APK matches current exact native and portable inputs without device execution', build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android').sources()[0] and all(sha(APP / v['path']) == v['sha256'] for v in build['artifacts'].values()) and inspection['passed'] and len(inspection['checks']) == 18 and content['passed'] and len(content['checks']) == 3 and not content['artifact_executed'] and not content['real_device'] and content['verification_script_sha256'] == sha(APP / 'scripts/verify_workbench_android_content.py'))
    check('JVM26 retains actual Gradle cache status and four exact XMLs', jvm['passed'] and jvm['tests'] == 26 and jvm['new_test_execution'] == (not jvm['gradle_cached']) and not jvm['device_executed'] and sha(E / 'native-build.log') == jvm['native_log_sha256'] and len(jvm['xml_sha256']) == 4 and all(sha(E / 'android-unit-results' / name) == value for name, value in jvm['xml_sha256'].items()))
    linux = read(E / 'linux-build.json')
    check('Two current Linux ELF targets cross-compile without execution or deployment', linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_source and len(linux['artifacts']) == 2 and all(sha(APP / v['file']) == v['sha256'] for v in linux['artifacts']) and not linux['artifact_executed'] and not linux['kernel_boundary_executed'] and not linux['deployed'] and linux['verification_script_sha256'] == sha(APP / 'scripts/build_workbench_linux.py'))
    interaction = module('workbench_sources').digest()
    shots = []
    for target in ('web', 'mobile'):
        folder = E.parent / ('workbench-' + target)
        browser, manifest, capture = [read(folder / n) for n in ('result.json', 'source-manifest.json', 'media.json')]
        check('Actual ' + target + ' workbench passes six groups and releases owned resources', browser['passed'] and len(browser['checks']) == 6 and browser['source_unchanged'] and browser['source_sha256'] == manifest['sha256'] == digest(manifest['files']) and browser['build_source_sha256'] == browser['build_source_after_sha256'] == interaction and browser['build_source_unchanged'] and all(browser[n] for n in ('owned_browser_closed', 'owned_review_server_stopped', 'owned_api_proxy_stopped', 'isolated_database_cleanup_passed')) and browser['native_fixture_exit_code'] == 0 and not browser['console_errors'] and not browser['server_errors'] and browser['h5_prebuilt_before_fixture'] == (target == 'mobile') and browser['real_api_forwarded_without_response_substitution'] == (target == 'mobile') and not capture['api_responses_mocked'] and not capture['real_device'] and all(sha(ROOT / v['file']) == v['sha256'] for v in capture['screenshots']))
        shots.extend(capture['screenshots'])
    media, visual = [read(E / n) for n in ('media.json', 'visual-review.json')]
    check('Two silent MP4 recordings and labelled screenshot-tour GIFs bind their actual synthetic pixels', media['passed'] and len(media['recordings']) == 2 and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_workbench_media.py') and all(sha(ROOT / v['capture_manifest']) == v['capture_manifest_sha256'] and v['gif_mode'] == 'actual_screenshot_tour' and not v['gif_is_screen_recording'] and v['gif_frame_duration_ms'] == 2000 and len(v['gif_screenshots']) == 3 and all(sha(ROOT / s['file']) == s['sha256'] for s in v['gif_screenshots']) and all(sha(ROOT / a['file']) == a['sha256'] for a in v['artifacts']) for v in media['recordings']))
    check('Visual review binds six screenshots and decoded GIF samples from both clients', visual['reviewed'] and len(shots) == len(visual['screenshots']) == 6 and len(visual['gifs']) == 2 and all(sha(ROOT / v['file']) == v['sha256'] for v in visual['screenshots'] + visual['gifs']) and set(visual['gif_frames_reviewed']) == {'web', 'mobile'})
    retained = read(E / 'reused-evidence.json')
    check('Prior266 media and dependency evidence retain exact published bytes and dates', retained['passed'] and retained['published_commit'] == PUBLISHED and len(retained['media']) == 266 and all(sha(ROOT / v['file']) == v['sha256'] for v in retained['media'] + retained['records']) and retained['package_lock_sha256'] == sha(APP / 'package-lock.json') and retained['workspace_dependency_fields_unchanged'] and retained['mobile_test_selection_updated'] and retained['contracts_source_export_added'] and not retained['new_dependency_audit'])
    development, update = [read(E / n) for n in ('development-project-resources.json', 'development-update.json')]
    check('Only attested development services reload while unchanged schema87 and fee history remain fixed', development['passed'] and update['passed'] and development['migration_count'] == update['migration_count'] == update['previous_migration_count'] == 87 and not update['schema_upgrade_executed'] and update['legacy_and_usage_rows_preserved'] and development['backend_source_sha256'] == update['backend_source_sha256'] == backend_source and all(development['ownership'].values()) and development['worker_sources_match'] and development['integrations_disabled'] and development['database']['disposable_test_databases'] == 0 and development['verification_script_sha256'] == sha(APP / 'scripts/verify_workbench_development.py') and update['updater_sha256'] == sha(APP / 'scripts/update_workbench_development.py') and not update['production_deployment'])
    initial = read(E / 'attempts/01-existing-mfa-fixture/attempt.json')
    browser_attempt = read(E / 'attempts/02-browser-mfa-fixture/source.json')
    invalidated = read(E / 'attempts/02-browser-mfa-fixture/invalidated-backend/result.json')
    initial_web = read(E / 'attempts/02-browser-mfa-fixture/browser-web/result.json')
    initial_typecheck = read(E / 'attempts/03-web-composite-includes/attempt.json')
    refresh_attempt = read(E / 'attempts/04-mobile-refresh-selector/attempt.json')
    refresh_mobile = read(E / 'attempts/04-mobile-refresh-selector/browser-mobile/result.json')
    refresh_web = read(E / 'attempts/04-mobile-refresh-selector/browser-web/result.json')
    environment_attempt = read(E / 'attempts/05-native-attestation-timeout/attempt.json')
    environment_progress = read(E / 'attempts/05-native-attestation-timeout/batch-progress.json')
    recording_attempt = read(E / 'attempts/06-recording-readable-holds/attempt.json')
    navigation_attempt = read(E / 'attempts/07-mobile-home-navigation/attempt.json')
    navigation_result = read(E / 'attempts/07-mobile-home-navigation/browser-mobile/result.json')
    build_attempt = read(E / 'attempts/08-h5-build-timeout/attempt.json')
    build_attempt_result = read(E / 'attempts/08-h5-build-timeout/result.json')
    memory_attempt = read(E / 'attempts/09-h5-memory-session-reload/attempt.json')
    memory_result = read(E / 'attempts/09-h5-memory-session-reload/browser-mobile/result.json')
    late_attempt = read(E / 'attempts/10-web-late-fixture/attempt.json')
    late_cleanup = read(E / 'attempts/10-web-late-fixture/controlled-cleanup.json')
    check('Earlier fixture/typecheck failures and invalidated backend source remain separate from final passes',
        all(not v['passed'] and not v['counts_as_passed'] for v in (initial, browser_attempt, initial_typecheck, refresh_attempt, environment_attempt))
        and not invalidated['passed'] and not invalidated['source_unchanged_during_tests']
        and initial['raw_log_sha256'] == sha(ROOT / initial['private_log'])
        and browser_attempt['archive_sha256'] == sha(ROOT / browser_attempt['private_source_archive'])
        and initial_typecheck['raw_log_sha256'] == sha(ROOT / initial_typecheck['private_log'])
        and not initial_web['passed'] and initial_web['owned_review_server_stopped']
        and not refresh_mobile['passed'] and refresh_mobile['isolated_database_cleanup_passed'] and refresh_mobile['owned_review_server_stopped']
        and refresh_web['passed'] and refresh_attempt['previous_web_result_passed_for_previous_sources'] and not refresh_attempt['current_source_pass_credit']
        and refresh_attempt['archive_sha256'] == sha(ROOT / refresh_attempt['private_source_archive'])
        and refresh_attempt['interaction_source_sha256'] != interaction
        and all(sha(ROOT / v['path']) == v['sha256'] for v in refresh_attempt['private_files'])
        and environment_attempt['raw_log_sha256'] == sha(ROOT / environment_attempt['private_log'])
        and environment_progress['source_sha256'] == backend_source
        and environment_progress['batches'][8]['status'] == 'failed' and not environment_progress['batches'][8]['checks']
        and any(v['retained_complete_batches'] == list(range(1, 9)) for v in progress['resumes'])
        and any(v['batch'] == 9 and not v['counts_as_passed'] for v in progress['prior_attempts'])
        and recording_attempt['superseded'] and not recording_attempt['counts_as_current_source_passed']
        and recording_attempt['previous_web_and_mobile_checks_passed'] and recording_attempt['source_binding_verified_from_exact_archived_helper']
        and recording_attempt['interaction_source_sha256'] != interaction
        and all(sha(ROOT / v['path']) == v['sha256'] for v in recording_attempt['private_files'])
        and not navigation_attempt['passed'] and not navigation_attempt['counts_as_passed'] and navigation_attempt['source_binding_verified_from_reconstructed_helpers']
        and navigation_attempt['interaction_source_sha256'] == navigation_result['build_source_sha256'] and navigation_attempt['interaction_source_sha256'] != interaction
        and not navigation_result['passed'] and navigation_result['owned_browser_closed'] and navigation_result['owned_review_server_stopped']
        and all(sha(ROOT / v['path']) == v['sha256'] for v in navigation_attempt['private_files'])
        and not build_attempt['passed'] and not build_attempt['counts_as_passed'] and not build_attempt['fixture_started'] and not build_attempt['browser_started'] and not build_attempt['private_fixture_present']
        and build_attempt['raw_log_sha256'] == sha(ROOT / build_attempt['private_log'])
        and not build_attempt_result['passed'] and build_attempt_result['runner_failure_code'] == 'TimeoutExpired' and build_attempt_result['owned_review_server_stopped']
        and not memory_attempt['passed'] and not memory_attempt['counts_as_passed'] and memory_attempt['h5_in_memory_session_expected'] and memory_attempt['previous_web_passed']
        and not memory_result['passed'] and memory_result['failure_context']['hash'] == '#/pages/login/index' and memory_result['owned_browser_closed'] and memory_result['owned_review_server_stopped']
        and memory_attempt['interaction_source_sha256'] != interaction and all(sha(ROOT / v['path']) == v['sha256'] for v in memory_attempt['private_files'])
        and not late_attempt['passed'] and not late_attempt['counts_as_passed'] and late_attempt['fixture_started_late'] and not late_attempt['browser_started'] and late_attempt['previous_mobile_passed_for_prior_harness']
        and late_cleanup['passed'] and late_cleanup['remaining_test_databases'] == 0 and late_cleanup['original_fixture_preserved_private'] and late_cleanup['no_active_connection_terminated'] and not late_cleanup['development_cluster_modified']
        and all(sha(ROOT / v['path']) == v['sha256'] for v in late_attempt['private_files']))
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('All23 packages, five exclusions and eight draft Skills preserve the corrected AI scope', scope['counts']['active'] == 23 and scope['counts']['excluded'] == 5 and scope['flags']['ai_secretary_required'] and scope['flags']['ai_authorization_required'] and scope['flags']['ai_execution_isolation_required'] and not scope['flags']['ai_cost_control_required'] and len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    ports_closed = True
    for port in (5189, 5190):
        with socket.socket() as probe:
            probe.settimeout(1)
            ports_closed = ports_closed and probe.connect_ex(('127.0.0.1', port)) != 0
    check('All owned temporary HTTP ports and private browser fixtures are released', ports_closed and not (APP / '.local/business-browser-fixture.json').exists())
    proof = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
        'interaction_kind': 'human_business_workbench', 'interaction_source_sha256': interaction,
        'mobile_source_sha256': mobile_source, 'backend_source_sha256': backend_source, 'verification_script_sha256': sha(Path(__file__)),
        'historical_evidence_commit': PUBLISHED, 'new_browser_capture': True, 'new_backend_test_execution': True,
        'full_goal_complete': False, 'production_ready': False}
    (E / 'final-source-check.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': proof['passed'], 'checks': len(checks), 'failed': [v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
