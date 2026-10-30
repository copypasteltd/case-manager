"""Verify actual current full-body drafting, explicit human adoption and bounded evidence scope."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import socket
import subprocess
import zipfile

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/contract-body'
PUBLISHED = '30c364a7794c3fe7049c6ab04c4ba5b41d306ec3'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def module(name):
    spec = importlib.util.spec_from_file_location('body_proof_' + name, APP / 'scripts' / (name + '.py'))
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
    group = E.parent / 'contract_body-backend'
    backend, progress, raw = [read(group / n) for n in ('result.json', 'batch-progress.json', 'raw-log-archive.json')]
    expected = {tuple(v) for v in progress['expected_tests']}
    observed = {(v['Package'], v['Test']) for v in backend['checks'] if '/' not in v['Test']}
    check('Every discovered current native backend test runs without skips or substituted cases', backend['passed'] and backend['source_sha256'] == backend_source and backend['source_unchanged_during_tests'] and backend['all_discovered_tests_executed'] and expected == observed and backend['discovered_top_level_tests'] == len(expected) == 239 and backend['parallel_batches'] == 1 and all(v['Action'] == 'pass' for v in backend['checks']) and backend['verification_script_sha256'] == sha(APP / 'scripts/verify_contract_body_backend.py'))
    check('Complete successful serial batches retain exact private raw logs', raw['passed'] and raw['source_sha256'] == backend_source and len(raw['logs']) == len(progress['batches']) == 12 and raw['records'] == len(backend['checks']) and raw['freezer_sha256'] == sha(APP / 'scripts/freeze_contract_body_backend.py') and all(sha(APP / v['raw_log']) == v['sha256'] and not v['excluded_test_families'] for v in raw['logs']))
    feature = [v for v in backend['checks'] if v['Test'].startswith('TestAgentContractBodies/')]
    check('Seven full-body API/PG groups cover exact files, event-log views, human adoption, current rights and atomic failure', len(feature) == 7 and all(v['Action'] == 'pass' for v in feature))
    cluster, closed = [read(E / n) for n in ('native-cluster.json', 'native-cluster-closed.json')]
    check('Final owned fully durable native test PG closes with zero disposable databases', cluster['passed'] and closed['passed'] and all(cluster[n] for n in ('fsync', 'synchronous_commit', 'full_page_writes')) and not cluster['original_development_cluster_modified'] and not cluster['fixture_cleanup_deadline_changed'] and not cluster['virtualization_used'] and closed['owned_cluster_stopped'] and closed['remaining_disposable_databases'] == 0 and cluster['verification_script_sha256'] == closed['verification_script_sha256'] == sha(APP / 'scripts/contract_body_native_cluster.py'))
    contract = read(APP / 'backend/internal/contract/openapi.json')
    original = json.loads(subprocess.check_output(['git', 'show', PUBLISHED + ':docs/development/openapi.json'], cwd=ROOT))
    changed = {k for k, v in original['components']['schemas'].items() if contract['components']['schemas'][k] != v}
    check('334 paths/410 operations/736 schemas preserve original paths and disclose three extensions/five new schemas', all(contract['paths'][k] == v for k, v in original['paths'].items()) and set(contract['paths']) - set(original['paths']) == {'/runs/{id}/contract-body-adoption'} and changed == {'AgentSourceSnapshot', 'RunCreate', 'Run'} and set(contract['components']['schemas']) - set(original['components']['schemas']) == {'AgentContractBodyInput', 'AgentContractBodyFile', 'AgentContractBodyView', 'AgentContractBodyAdopt', 'AgentContractBodyViewResponse'} and len(contract['paths']) == 334 and sum(len(v) for v in contract['paths'].values()) == 410 and len(contract['components']['schemas']) == 736 and contract == read(ROOT / 'docs/development/openapi.json'))
    upgrade = read(E / 'schema-upgrade.json')
    check('Published116 populated contract journey upgrades normally from89 to90 without changing18 original fact tables', upgrade['passed'] and upgrade['source_sha256'] == backend_source and upgrade['published_fixture_commit'] == PUBLISHED and upgrade['before_schema'] == 89 and upgrade['after_schema'] == 90 and upgrade['original_facts_unchanged'] and upgrade['new_tables_empty_and_guards_ready'] and len(upgrade['facts']) == 18 and upgrade['facts']['contract_revisions']['rows'] >= 2 and not upgrade['counts_as_current_backend_tests'] and upgrade['verification_script_sha256'] == sha(APP / 'scripts/verify_contract_body_upgrade.py') and sha(APP / '.local/contract-body-upgrade-v3.log') == upgrade['raw_log_sha256'])
    client = read(E / 'client-checks.json')
    check('Eight complete client commands pass against unchanged exact inputs and byte-matching logs', client['passed'] and client['source_unchanged'] and client['source_sha256'] == client['source_after_sha256'] == digest(client['source_files']) and len(client['checks']) == 8 and all(v['passed'] and v['exit_code'] == 0 and sha(APP / '.local' / Path(v['raw_log']).name) == v['raw_log_sha256'] for v in client['checks']))
    web, mobile, runtime = [read(E / n) for n in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    mobile_source = module('verify_mobile').digest()[0]
    check('Web303/mobile399/runtime56 are actual current-source command counts', web['passed'] and web['tests']['numPassedTests'] == 303 and web['source_sha256'] == digest(web['sources']) and mobile['passed'] and mobile['tests'] == {'pass': 399, 'fail': 0, 'cancelled': 0, 'skipped': 0} and mobile['source_sha256'] == mobile_source and runtime['status'] == 'passed' and runtime['tests']['pass'] == 56 and runtime['source_sha256'] == module('verify_agent_runtime').digest()[0] and all(v['derived_from']['sha256'] == sha(E / 'client-checks.json') for v in (web, mobile, runtime)))
    check('Both actual miniapp builds retain all20 routes', set(mobile['targets']) == {'weapp', 'tt'} and all(v['page_count'] == 20 and sha(APP / 'mobile/dist' / k / 'app.json') == v['app_json_sha256'] for k, v in mobile['targets'].items()))
    build, inspection, content, jvm = [read(E / n) for n in ('build.json', 'apk-inspection.json', 'android-feature-content.json', 'android-test-scope.json')]
    check('Current development APK binds complete native/portable inputs and passes18+3 static checks without device execution', build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android').sources()[0] and all(sha(APP / v['path']) == v['sha256'] for v in build['artifacts'].values()) and inspection['passed'] and len(inspection['checks']) == 18 and content['passed'] and len(content['checks']) == 3 and not content['artifact_executed'] and not content['real_device'] and content['verification_script_sha256'] == sha(APP / 'scripts/verify_contract_body_android_content.py'))
    check('JVM26 retains actual Gradle cache status and four exact XMLs', jvm['passed'] and jvm['tests'] == 26 and jvm['new_test_execution'] == (not jvm['gradle_cached']) and not jvm['device_executed'] and sha(E / 'native-build.log') == jvm['native_log_sha256'] and len(jvm['xml_sha256']) == 4 and all(sha(E / 'android-unit-results' / n) == v for n, v in jvm['xml_sha256'].items()))
    linux = read(E / 'linux-build.json')
    check('Two current Linux ELF targets cross-build without execution, isolation or deployment claims', linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_source and len(linux['artifacts']) == 2 and all(sha(APP / v['file']) == v['sha256'] for v in linux['artifacts']) and not linux['artifact_executed'] and not linux['kernel_boundary_executed'] and not linux['deployed'] and linux['verification_script_sha256'] == sha(APP / 'scripts/build_contract_body_linux.py'))
    interaction = module('contract_body_sources').digest()
    expected_ui = re.findall(r"checks.push\('([^']+)'\)", (APP / 'scripts/check_contract_body.mjs').read_text(encoding='utf-8'))
    assert len(expected_ui) >= 25
    shots = []
    for target in ('web', 'mobile'):
        folder = E.parent / ('contract-body-' + target)
        browser, manifest, capture = [read(folder / n) for n in ('result.json', 'source-manifest.json', 'media.json')]
        check('Actual ' + target + ' full-body request, review, human adoption, replay and history pass with all owned resources closed', browser['passed'] and browser['checks'] == expected_ui and browser['source_unchanged'] and browser['source_sha256'] == manifest['sha256'] == digest(manifest['files']) and browser['build_source_sha256'] == browser['build_source_after_sha256'] == interaction and browser['build_source_unchanged'] and all(browser[n] for n in ('owned_browser_closed', 'owned_review_server_stopped', 'owned_api_proxy_stopped', 'isolated_database_cleanup_passed')) and browser['native_fixture_exit_code'] == 0 and not browser['console_errors'] and not browser['server_errors'] and browser['h5_prebuilt_before_fixture'] == (target == 'mobile') and browser['real_api_forwarded_without_response_substitution'] == (target == 'mobile') and not capture['api_responses_mocked'] and not capture['real_device'] and all(sha(ROOT / v['file']) == v['sha256'] for v in capture['screenshots']))
        shots.extend(capture['screenshots'])
    media, visual = [read(E / n) for n in ('media.json', 'visual-review.json')]
    check('Two actual silent viewport recordings and labelled screenshot-tour GIFs bind their real synthetic pixels', media['passed'] and len(media['recordings']) == 2 and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_contract_body_media.py') and all(sha(ROOT / v['capture_manifest']) == v['capture_manifest_sha256'] and v['gif_mode'] == 'actual_screenshot_tour' and not v['gif_is_screen_recording'] and v['gif_frame_duration_ms'] == 2000 and len(v['gif_screenshots']) == 3 and all(sha(ROOT / s['file']) == s['sha256'] for s in v['gif_screenshots']) and all(sha(ROOT / a['file']) == a['sha256'] for a in v['artifacts']) for v in media['recordings']))
    check('Actual visual review binds six screenshots and decoded MP4/GIF samples', visual['reviewed'] and len(shots) == len(visual['screenshots']) == 6 and len(visual['gifs']) == 2 and all(sha(ROOT / v['file']) == v['sha256'] for v in visual['screenshots'] + visual['gifs']) and set(visual['gif_frames_reviewed']) == set(visual['recording_frames_reviewed']) == {'web', 'mobile'} and all(sha(ROOT / v['file']) == v['sha256'] and len(v['sampled_seconds']) == 2 and not v['credential_fields_visible'] for v in visual['recording_frames_reviewed'].values()))
    retained, audit = [read(E / n) for n in ('reused-evidence.json', 'dependency-audit.json')]
    check('296 published media and old proofs retain exact bytes; resolved dependency packages stay unchanged', retained['passed'] and retained['published_commit'] == PUBLISHED and len(retained['media']) == 296 and all(sha(ROOT / v['file']) == v['sha256'] for v in retained['media'] + retained['records']) and retained['package_lock_sha256'] == sha(APP / 'package-lock.json') and retained['resolved_dependency_packages_unchanged'])
    check('Fresh current-lock audit includes all scopes and reports zero known advisories without production security signoff', audit['passed'] and audit['source_unchanged'] and audit['exit_code'] == 0 and audit['package_lock_sha256'] == sha(APP / 'package-lock.json') and audit['vulnerabilities']['total'] == 0 and audit['all_dependency_scopes_included'] and not audit['advisories_suppressed'] and not audit['production_security_gate_passed'] and audit['raw_report_sha256'] == sha(APP / '.local/contract-body-dependency-audit.json'))
    development, update = [read(E / n) for n in ('development-project-resources.json', 'development-update.json')]
    check('Only attested development services reload; schema89→90 preserves original contract/cash/legacy facts', development['passed'] and update['passed'] and development['migration_count'] == update['migration_count'] == 90 and update['previous_migration_count'] == 89 and update['schema_upgrade_executed'] and update['legacy_and_usage_rows_preserved'] and development['backend_source_sha256'] == update['backend_source_sha256'] == backend_source and all(development['ownership'].values()) and development['worker_sources_match'] and development['integrations_disabled'] and development['database']['disposable_test_databases'] == 0 and development['verification_script_sha256'] == sha(APP / 'scripts/verify_contract_body_development.py') and update['updater_sha256'] == sha(APP / 'scripts/update_contract_body_development.py') and not update['production_deployment'])
    archive = read(E / 'source-archive.json')
    with zipfile.ZipFile(ROOT / archive['private_archive']) as saved:
        exact = all(hashlib.sha256(saved.read(v['file'])).hexdigest() == v['sha256'] == sha(ROOT / v['file']) for v in archive['files'])
    check('Complete final private source union covers every client/native/browser/runtime input with exact original bytes', archive['passed'] and not archive['counts_as_test_execution'] and sha(ROOT / archive['private_archive']) == archive['archive_sha256'] and archive['interaction_source_sha256'] == interaction and exact and archive['all_declared_inputs_included'])
    attempts = read(E / 'preliminary-checks.json')
    check('Preliminary and interrupted checks retain private logs with no final regression credit', not attempts['counts_as_final_regression'] and all(sha(ROOT / v['file']) == v['sha256'] for v in attempts['records']))
    loss = read(E.parent / 'contract-body-attempts/recording-retention-v1/disclosure.json')
    unavailable = {(v['preservation_file'], v['file'], v['expected_sha256']): v for v in loss['missing_records']}
    preserved, observed_losses = [], set()
    for path in sorted((E.parent / 'contract-body-attempts').rglob('preservation.json')):
        prior = read(path)
        valid = not prior.get('counts_as_current_pass', prior.get('counts_as_current_source_pass', False))
        for entry in prior.get('records', []) + prior.get('public_records', []) + prior.get('private_records', []):
            exact = sha(ROOT / entry['file']) == entry['sha256']
            key = (path.relative_to(ROOT).as_posix(), entry['file'], entry['sha256'])
            if not exact and key in unavailable:
                disclosed = unavailable[key]
                valid = valid and sha(path) == disclosed['preservation_sha256'] and not disclosed['original_bytes_recovered'] and sha(ROOT / entry['file']) == disclosed['observed_reused_path_sha256']
                observed_losses.add(key)
            else:
                valid = valid and exact
        if prior.get('source_archive'):
            valid = valid and sha(ROOT / prior['source_archive']) == prior['source_archive_sha256']
        preserved.append(valid)
    old_types = read(E.parent / 'contract-body-attempts/client-types-v1.json')
    check('Earlier sources, screenshots, logs and results remain separate from final passes; two obsolete recordings are explicitly unavailable', preserved and all(preserved) and not old_types['preservation']['counted_as_current_pass'] and sha(APP / '.local/contract-body-attempts/client-types-v1/source.zip') == old_types['preservation']['source_zip_sha256'])
    loss_labels = {Path(v['preservation_file']).parent.name for v in loss['missing_records']}
    check('Original failed retention check and two precise historical recording losses remain disclosed without recovery or pass credit', loss['status'] == 'historical_recording_retention_failed' and not loss['historical_raw_recordings_retained'] and not loss['original_bytes_recovered'] and not loss['counts_as_current_pass'] and len(unavailable) == 2 and observed_losses == set(unavailable) and loss_labels == {'03-before-mobile-field-fix-web', '04-before-mobile-field-repair-web'} and all(v['file'] == 'app/.local/repository-showcase/contract-body-web/frames.json' for v in loss['missing_records']) and all(sha(ROOT / v['file']) == v['sha256'] for v in loss['records']) and loss['corrected_archiver_sha256'] == sha(APP / 'scripts/archive_contract_body_passed_browser.py') and loss['disclosure_script_sha256'] == sha(APP / 'scripts/disclose_contract_body_recording_loss.py'))
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('23 active packages, five exclusions and eight draft Skills retain the corrected AI scope', scope['counts']['active'] == 23 and scope['counts']['excluded'] == 5 and scope['flags']['ai_secretary_required'] and scope['flags']['ai_authorization_required'] and scope['flags']['ai_execution_isolation_required'] and not scope['flags']['ai_cost_control_required'] and len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    ports_closed = True
    for port in (5189, 5190):
        with socket.socket() as probe:
            probe.settimeout(1)
            ports_closed = ports_closed and probe.connect_ex(('127.0.0.1', port)) != 0
    check('All owned temporary HTTP ports and browser fixture capabilities are released', ports_closed and not (APP / '.local/business-browser-fixture.json').exists())
    proof = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
        'interaction_kind': 'human_contract_body_adoption', 'interaction_source_sha256': interaction,
        'mobile_source_sha256': mobile_source, 'backend_source_sha256': backend_source, 'verification_script_sha256': sha(Path(__file__)),
        'historical_evidence_commit': PUBLISHED, 'new_browser_capture': True, 'new_backend_test_execution': True,
        'full_goal_complete': False, 'production_ready': False}
    (E / 'final-source-check.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': proof['passed'], 'checks': len(checks), 'failed': [v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
