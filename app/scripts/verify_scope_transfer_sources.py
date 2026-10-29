"""Bind complete current business-scope_transfer evidence; retained evidence keeps its actual scope."""
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
E = ROOT / 'docs/quality/evidence/scope-transfer'
PUBLISHED = '33e3d52f9a2bddd6be6fc5fba97a20f99eca20fd'


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
    group = E.parent / 'scope_transfer-backend'
    backend, progress, raw = [read(group / name) for name in ('result.json', 'batch-progress.json', 'raw-log-archive.json')]
    expected = {tuple(v) for v in progress['expected_tests']}
    observed = {(v['Package'], v['Test']) for v in backend['checks'] if '/' not in v['Test']}
    check('Every discovered current-source native backend test executes without skips', backend['passed'] and backend['source_sha256'] == backend_source and backend['source_unchanged_during_tests'] and backend['all_discovered_tests_executed'] and expected == observed and backend['discovered_top_level_tests'] == len(expected) and backend['parallel_batches'] == 1 and all(v['Action'] == 'pass' for v in backend['checks']) and backend['verification_script_sha256'] == sha(APP / 'scripts/verify_scope_transfer_backend.py'))
    check('Complete successful batches retain exact private raw logs without substituted cases', raw['passed'] and raw['source_sha256'] == backend_source and len(raw['logs']) == len(progress['batches']) and raw['records'] == len(backend['checks']) and raw['freezer_sha256'] == sha(APP / 'scripts/freeze_scope_transfer_backend.py') and all(sha(APP / v['raw_log']) == v['sha256'] and not v['excluded_test_families'] for v in raw['logs']))
    feature = [v for v in backend['checks'] if v['Test'].startswith('TestFileScopeTransfers/')]
    check('Thirteen actual file-scope transfer API and native PG groups pass', len(feature) == 13 and all(v['Action'] == 'pass' for v in feature) and backend['discovered_top_level_tests'] == 236)
    cluster, closed = [read(E / n) for n in ('native-cluster.json', 'native-cluster-closed.json')]
    check('Owned full-durability native test PG closes with no disposable databases', cluster['passed'] and closed['passed'] and all(cluster[n] for n in ('fsync', 'synchronous_commit', 'full_page_writes')) and not cluster['original_development_cluster_modified'] and not cluster['fixture_cleanup_deadline_changed'] and not cluster['virtualization_used'] and closed['owned_cluster_stopped'] and closed['remaining_disposable_databases'] == 0 and cluster['verification_script_sha256'] == closed['verification_script_sha256'] == sha(APP / 'scripts/scope_transfer_native_cluster.py'))
    contract = read(APP / 'backend/internal/contract/openapi.json')
    original = json.loads(subprocess.check_output(['git', 'show', PUBLISHED + ':docs/development/openapi.json'], cwd=ROOT))
    check('All prior API paths and schemas retain semantics; human scope transfers add six paths and eight schemas', all(contract['paths'][k] == v for k, v in original['paths'].items()) and all(contract['components']['schemas'][k] == v for k, v in original['components']['schemas'].items()) and len(contract['paths']) == 333 and sum(len(v) for v in contract['paths'].values()) == 409 and len(contract['components']['schemas']) == 729 and contract == read(ROOT / 'docs/development/openapi.json'))
    client = read(E / 'client-checks.json')
    check('Eight client commands run against unchanged exact source with retained raw hashes', client['passed'] and client['source_unchanged'] and client['source_sha256'] == client['source_after_sha256'] == digest(client['source_files']) and len(client['checks']) == 8 and all(v['passed'] and v['exit_code'] == 0 and sha(APP / '.local' / Path(v['raw_log']).name) == v['raw_log_sha256'] for v in client['checks']))
    web, mobile, runtime = [read(E / n) for n in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    mobile_source = module('verify_mobile').digest()[0]
    check('Web281 mobile375 runtime55 match current complete sources and actual command records', web['passed'] and web['tests']['numPassedTests'] == 281 and web['source_sha256'] == digest(web['sources']) and mobile['passed'] and mobile['tests'] == {'pass': 375, 'fail': 0, 'cancelled': 0, 'skipped': 0} and mobile['source_sha256'] == mobile_source and runtime['status'] == 'passed' and runtime['tests']['pass'] == 55 and runtime['source_sha256'] == module('verify_agent_runtime').digest()[0] and all(v['derived_from']['sha256'] == sha(E / 'client-checks.json') for v in (web, mobile, runtime)))
    check('Both built miniapps retain all20 routes', set(mobile['targets']) == {'weapp', 'tt'} and all(v['page_count'] == 20 and sha(APP / 'mobile/dist' / k / 'app.json') == v['app_json_sha256'] for k, v in mobile['targets'].items()))
    build, inspection, content, jvm = [read(E / n) for n in ('build.json', 'apk-inspection.json', 'android-feature-content.json', 'android-test-scope.json')]
    check('New development APK matches current exact native and portable inputs without device execution', build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android').sources()[0] and all(sha(APP / v['path']) == v['sha256'] for v in build['artifacts'].values()) and inspection['passed'] and len(inspection['checks']) == 18 and content['passed'] and len(content['checks']) == 3 and not content['artifact_executed'] and not content['real_device'] and content['verification_script_sha256'] == sha(APP / 'scripts/verify_scope_transfer_android_content.py'))
    check('JVM26 retains actual Gradle cache status and four exact XMLs', jvm['passed'] and jvm['tests'] == 26 and jvm['new_test_execution'] == (not jvm['gradle_cached']) and not jvm['device_executed'] and sha(E / 'native-build.log') == jvm['native_log_sha256'] and len(jvm['xml_sha256']) == 4 and all(sha(E / 'android-unit-results' / name) == value for name, value in jvm['xml_sha256'].items()))
    linux = read(E / 'linux-build.json')
    check('Two current Linux ELF targets cross-compile without execution or deployment', linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_source and len(linux['artifacts']) == 2 and all(sha(APP / v['file']) == v['sha256'] for v in linux['artifacts']) and not linux['artifact_executed'] and not linux['kernel_boundary_executed'] and not linux['deployed'] and linux['verification_script_sha256'] == sha(APP / 'scripts/build_scope_transfer_linux.py'))
    interaction = module('scope_transfer_sources').digest()
    shots = []
    for target in ('web', 'mobile'):
        folder = E.parent / ('scope-transfer-' + target)
        browser, manifest, capture = [read(folder / n) for n in ('result.json', 'source-manifest.json', 'media.json')]
        check('Actual ' + target + ' scope_transfer passes six groups and releases owned resources', browser['passed'] and len(browser['checks']) == 6 and browser['source_unchanged'] and browser['source_sha256'] == manifest['sha256'] == digest(manifest['files']) and browser['build_source_sha256'] == browser['build_source_after_sha256'] == interaction and browser['build_source_unchanged'] and all(browser[n] for n in ('owned_browser_closed', 'owned_review_server_stopped', 'owned_api_proxy_stopped', 'isolated_database_cleanup_passed')) and browser['native_fixture_exit_code'] == 0 and not browser['console_errors'] and not browser['server_errors'] and browser['h5_prebuilt_before_fixture'] == (target == 'mobile') and browser['real_api_forwarded_without_response_substitution'] == (target == 'mobile') and not capture['api_responses_mocked'] and not capture['real_device'] and all(sha(ROOT / v['file']) == v['sha256'] for v in capture['screenshots']))
        shots.extend(capture['screenshots'])
    media, visual = [read(E / n) for n in ('media.json', 'visual-review.json')]
    check('Two silent MP4 recordings and labelled screenshot-tour GIFs bind their actual synthetic pixels', media['passed'] and len(media['recordings']) == 2 and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_scope_transfer_media.py') and all(sha(ROOT / v['capture_manifest']) == v['capture_manifest_sha256'] and v['gif_mode'] == 'actual_screenshot_tour' and not v['gif_is_screen_recording'] and v['gif_frame_duration_ms'] == 2000 and len(v['gif_screenshots']) == 3 and all(sha(ROOT / s['file']) == s['sha256'] for s in v['gif_screenshots']) and all(sha(ROOT / a['file']) == a['sha256'] for a in v['artifacts']) for v in media['recordings']))
    check('Visual review binds six screenshots and decoded MP4/GIF samples from both clients', visual['reviewed'] and len(shots) == len(visual['screenshots']) == 6 and len(visual['gifs']) == 2 and all(sha(ROOT / v['file']) == v['sha256'] for v in visual['screenshots'] + visual['gifs']) and set(visual['gif_frames_reviewed']) == set(visual['recording_frames_reviewed']) == {'web', 'mobile'} and all(sha(ROOT / v['file']) == v['sha256'] and len(v['sampled_seconds']) == 2 and not v['credential_fields_visible'] for v in visual['recording_frames_reviewed'].values()))
    retained = read(E / 'reused-evidence.json')
    check('Prior276 media and dependency evidence retain exact published bytes and dates', retained['passed'] and retained['published_commit'] == PUBLISHED and len(retained['media']) == 276 and all(sha(ROOT / v['file']) == v['sha256'] for v in retained['media'] + retained['records']) and retained['package_lock_sha256'] == sha(APP / 'package-lock.json') and retained['workspace_dependency_fields_unchanged'] and retained['mobile_test_selection_updated'] and retained['contracts_source_export_added'] and not retained['new_dependency_audit'])
    development, update = [read(E / n) for n in ('development-project-resources.json', 'development-update.json')]
    check('Only attested development services reload while schema88 upgrades once from87 and fee history remain fixed', development['passed'] and update['passed'] and development['migration_count'] == update['migration_count'] == 88 and update['previous_migration_count'] == 87 and update['schema_upgrade_executed'] and update['legacy_and_usage_rows_preserved'] and development['backend_source_sha256'] == update['backend_source_sha256'] == backend_source and all(development['ownership'].values()) and development['worker_sources_match'] and development['integrations_disabled'] and development['database']['disposable_test_databases'] == 0 and development['verification_script_sha256'] == sha(APP / 'scripts/verify_scope_transfer_development.py') and update['updater_sha256'] == sha(APP / 'scripts/update_scope_transfer_development.py') and not update['production_deployment'])
    attempts = sorted((E / 'attempts').glob('*/attempt.json'))
    attempt_ok = len(attempts) >= 2
    for path in attempts:
        attempt = read(path)
        saved = ROOT / attempt['private_source_archive']
        attempt_ok = attempt_ok and not attempt['passed'] and not attempt['counts_as_passed'] and attempt['source_binding_verified'] and sha(saved) == attempt['archive_sha256'] and all(sha(ROOT / v['path']) == v['sha256'] for v in attempt['private_logs'])
        folder = next(p for p in path.parent.iterdir() if p.is_dir() and (p/'source-manifest.json').exists())
        manifest, old_result = [read(folder/n) for n in ('source-manifest.json','result.json')]
        h = hashlib.sha256()
        with zipfile.ZipFile(saved) as archive:
            for name in manifest['files']:
                h.update(name.encode()+b'\0'+archive.read(name)+b'\0')
        attempt_ok = attempt_ok and h.hexdigest() == manifest['sha256'] == attempt['source_sha256'] and not old_result['passed'] and old_result['owned_browser_closed'] and old_result['owned_review_server_stopped'] and old_result['isolated_database_cleanup_passed']
    targeted = read(E/'attempts/targeted-checks.json')
    attempt_ok = attempt_ok and all(sha(ROOT/v['file']) == v['sha256'] and not v['counts_as_final_full_regression'] for v in targeted['records'])
    check('Earlier fixture/typecheck/browser failures retain exact private hashes and verified browser sources without pass credit', attempt_ok)
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
        'interaction_kind': 'human_file_scope_transfer', 'interaction_source_sha256': interaction,
        'mobile_source_sha256': mobile_source, 'backend_source_sha256': backend_source, 'verification_script_sha256': sha(Path(__file__)),
        'historical_evidence_commit': PUBLISHED, 'new_browser_capture': True, 'new_backend_test_execution': True,
        'full_goal_complete': False, 'production_ready': False}
    (E / 'final-source-check.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': proof['passed'], 'checks': len(checks), 'failed': [v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
