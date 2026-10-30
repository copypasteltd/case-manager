"""Verify current source-bound execution, actual UI/media, retained evidence and cleanup."""
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
E = ROOT / 'docs/quality/evidence/payment-fee'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def module(name):
    spec = importlib.util.spec_from_file_location('fee_final_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def main():
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    source = module('verify_backend_batches').digest()
    backend = read(E.parent / 'payment_fee-backend/result.json')
    check('Every discovered current backend test passes without source changes', backend['passed'] and backend['source_sha256'] == source and backend['source_unchanged_during_tests'] and backend['all_discovered_tests_executed'] and backend['discovered_top_level_tests'] == 240 and len(backend['checks']) == 1492 and all(v['Action'] == 'pass' for v in backend['checks']))
    raw = read(E.parent / 'payment_fee-backend/raw-log-archive.json')
    check('All12 native backend batches retain exact private logs', raw['passed'] and raw['source_sha256'] == source and raw['records'] == len(backend['checks']) and len(raw['logs']) == 12 and all(sha(APP / v['raw_log']) == v['sha256'] and not v['excluded_test_families'] for v in raw['logs']))
    client = read(E / 'client-checks.json')
    check('Eight full client commands pass against unchanged complete inputs', client['passed'] and client['source_unchanged'] and client['source_sha256'] == client['source_after_sha256'] == digest(client['source_files']) and len(client['checks']) == 8 and all(v['passed'] and v['exit_code'] == 0 and sha(APP / '.local' / Path(v['raw_log']).name) == v['raw_log_sha256'] for v in client['checks']))
    web, mobile, runtime = [read(E / n) for n in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    mobile_source = module('verify_mobile').digest()[0]
    check('Web311 mobile407 runtime56 are actual current-source command counts', web['passed'] and web['tests']['numPassedTests'] == 311 and web['source_sha256'] == digest(web['sources']) and mobile['passed'] and mobile['tests'] == {'pass': 407, 'fail': 0, 'cancelled': 0, 'skipped': 0} and mobile['source_sha256'] == mobile_source and runtime['status'] == 'passed' and runtime['tests']['pass'] == 56 and runtime['source_sha256'] == module('verify_agent_runtime').digest()[0] and all(v['derived_from']['sha256'] == sha(E / 'client-checks.json') for v in (web, mobile, runtime)))
    cases = re.findall(r"\['([^']+)',", (APP / 'packages/contracts/test/payment-fees-cases.ts').read_text(encoding='utf-8'))
    log = (APP / '.local/payment_fee-client-mobile-tests.log').read_text(encoding='utf-8')
    check('All eight shared fee cases are selected and executed by the mobile test command', len(cases) == 8 and all(v in log for v in cases) and 'payment-fees.test.js' in read(APP / 'mobile/package.json')['scripts']['test'])
    check('Both actual miniapp builds retain20 routes each', set(mobile['targets']) == {'weapp', 'tt'} and all(v['page_count'] == 20 and sha(APP / 'mobile/dist' / k / 'app.json') == v['app_json_sha256'] for k, v in mobile['targets'].items()))
    api = read(ROOT / 'docs/development/openapi.json')
    paths = api['paths']
    operations = sum(method in ('get', 'post', 'patch', 'delete', 'put', 'head', 'options') for path in paths.values() for method in path)
    check('Canonical and embedded contracts contain all337 paths414 operations748 schemas', len(paths) == 337 and operations == 414 and len(api['components']['schemas']) == 748 and (ROOT / 'docs/development/openapi.json').read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes())
    previous = json.loads(subprocess.check_output(['git', 'show', 'e4a66e32cc4a2bf1a663506381a2be6f3ebeb15c:docs/development/openapi.json'], cwd=ROOT))
    check('All334 existing paths and736 schemas preserve their complete published contracts', all(v == paths.get(k) for k, v in previous['paths'].items()) and all(v == api['components']['schemas'].get(k) for k, v in previous['components']['schemas'].items()))
    build, inspection, content, jvm = [read(E / n) for n in ('build.json', 'apk-inspection.json', 'android-feature-content.json', 'android-test-scope.json')]
    check('Current development APK binds complete native and shared sources and18 static checks', build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android').sources()[0] and all(sha(APP / v['path']) == v['sha256'] for v in build['artifacts'].values()) and inspection['passed'] and len(inspection['checks']) == 18 and not build['device_executed'] and not build['virtualization_used'])
    check('Exact APK bundle includes fee evidence review balance and human revocation', content['passed'] and len(content['checks']) == 3 and content['source_sha256'] == build['source_sha256'] and not content['artifact_executed'] and not content['real_device'] and content['verification_script_sha256'] == sha(APP / 'scripts/verify_payment_fee_android_content.py'))
    check('JVM26 preserves actual Gradle cache status and four exact XMLs', jvm['passed'] and jvm['tests'] == 26 and jvm['new_test_execution'] == (not jvm['gradle_cached']) and not jvm['device_executed'] and sha(E / 'native-build.log') == jvm['native_log_sha256'] and len(jvm['xml_sha256']) == 4 and all(sha(E / 'android-unit-results' / n) == v for n, v in jvm['xml_sha256'].items()))
    linux = read(E / 'linux-build.json')
    check('Two current Linux ELF targets cross-build without execution claims', linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == source and len(linux['artifacts']) == 2 and all(sha(APP / v['file']) == v['sha256'] for v in linux['artifacts']) and not linux['artifact_executed'] and not linux['kernel_boundary_executed'] and not linux['deployed'] and linux['verification_script_sha256'] == sha(APP / 'scripts/build_payment_fee_linux.py'))
    upgrade = read(E / 'schema-upgrade.json')
    check('Published117 populated finance journey verifies normal90to91 upgrade with18 original fact tables unchanged', upgrade['passed'] and upgrade['source_sha256'] == source and upgrade['before_schema'] == 90 and upgrade['after_schema'] == 91 and upgrade['original_facts_unchanged'] and upgrade['new_tables_empty_and_guards_ready'] and len(upgrade['facts']) == 18 and upgrade['facts']['payment_allocations']['rows'] == 26 and not upgrade['counts_as_current_backend_tests'] and upgrade['verification_script_sha256'] == sha(APP / 'scripts/verify_payment_fee_upgrade.py'))
    update, development = [read(E / n) for n in ('development-update.json', 'development-project-resources.json')]
    check('Attested native development services upgrade91 without changing original facts or restarting PG', update['passed'] and update['backend_source_sha256'] == source and update['previous_migration_count'] == 90 and update['migration_count'] == 91 and update['history_before'] == update['history_after'] and not update['postgres_restarted'] and not update['external_integration_started'] and update['updater_sha256'] == sha(APP / 'scripts/update_payment_fee_development.py'))
    check('Current read-only development inspection preserves worker ownership and disabled external integrations', development['passed'] and development['migration_count'] == 91 and development['backend_source_sha256'] == source and development['worker_sources_match'] and all(development['ownership'].values()) and development['integrations_disabled'] and development['database']['disposable_test_databases'] == 0 and development['verification_script_sha256'] == sha(APP / 'scripts/verify_payment_fee_development.py'))
    interaction = module('payment_fee_sources').digest()
    expected_ui = re.findall(r"checks.push\('([^']+)'\)", (APP / 'scripts/check_payment_fee.mjs').read_text(encoding='utf-8'))
    assert len(expected_ui) == 14
    shots = []
    for target in ('web', 'mobile'):
        folder = E.parent / ('payment-fee-' + target)
        browser, manifest, capture = [read(folder / n) for n in ('result.json', 'source-manifest.json', 'media.json')]
        check('Actual ' + target + ' selection linking replay and revocation pass with all owned resources closed', browser['passed'] and browser['checks'] == expected_ui and browser['source_unchanged'] and browser['source_sha256'] == manifest['sha256'] == digest(manifest['files']) and browser['build_source_sha256'] == browser['build_source_after_sha256'] == interaction and browser['build_source_unchanged'] and all(browser[n] for n in ('owned_browser_closed', 'owned_review_server_stopped', 'owned_api_proxy_stopped', 'isolated_database_cleanup_passed')) and browser['native_fixture_exit_code'] == 0 and not browser['console_errors'] and not browser['server_errors'] and browser['h5_prebuilt_before_fixture'] == (target == 'mobile') and not capture['api_responses_mocked'])
        shots.extend(capture['screenshots'])
    check('Six new actual screenshots retain exact hashes', len(shots) == 6 and all(sha(ROOT / v['file']) == v['sha256'] for v in shots))
    media = read(E / 'media.json')
    raw_pixels = True
    for clip in media['recordings']:
        capture = read(E.parent / ('payment-fee-' + clip['client']) / 'media.json')['recording']
        raw_path = APP / capture['manifest']
        frames = read(raw_path)
        raw_pixels = raw_pixels and sha(raw_path) == capture['manifest_sha256'] == clip['capture_manifest_sha256'] and len(frames['frames']) == clip['capture_frame_count'] and all(sha(raw_path.parent / v['file']) == v['sha256'] for v in frames['frames'])
    check('Two silent MP4 recordings and two labelled screenshot GIF tours bind exact retained real pixels', media['passed'] and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_payment_fee_media.py') and len(media['recordings']) == 2 and raw_pixels and all(not v['gif_is_screen_recording'] and v['gif_mode'] == 'actual_screenshot_tour' and len(v['gif_screenshots']) == 3 and all(sha(ROOT / a['file']) == a['sha256'] for a in v['artifacts']) for v in media['recordings']))
    visual = read(E / 'visual-review.json')
    check('Explicit visual review binds all six screenshots and decoded MP4 GIF samples', visual['reviewed'] and len(visual['screenshots']) == 6 and all(sha(ROOT / v['file']) == v['sha256'] for v in visual['screenshots'] + visual['gifs']) and set(visual['recording_frames_reviewed']) == {'web', 'mobile'} and not visual['findings'])
    retained = read(E / 'reused-evidence.json')
    check('All306 older published media and original proof records remain exact', retained['passed'] and len(retained['media']) == 306 and all(sha(ROOT / v['file']) == v['sha256'] for v in retained['media'] + retained['records']))
    audit = read(E / 'dependency-audit.json')
    check('Unchanged dependency lock explicitly reuses prior zero-advisory evidence without claiming a new query', audit['passed'] and not audit['executed_now'] and audit['package_lock_sha256'] == sha(APP / 'package-lock.json') and audit['vulnerabilities']['total'] == 0 and sha(ROOT / audit['reused_from']) == audit['reused_record_sha256'] and not audit['production_security_gate_passed'])
    prior = read(E / 'preliminary-checks.json')
    preserved = all(sha(ROOT / v['file']) == v['sha256'] for v in prior['records'])
    attempts = sorted((E.parent / 'payment-fee-attempts').glob('*/preservation.json'))
    for path in attempts:
        v = read(path)
        preserved = preserved and not v['counts_as_current_pass'] and sha(ROOT / v['source_archive']) == v['source_archive_sha256'] and all(sha(ROOT / r['file']) == r['sha256'] for r in v['public_records'] + v['private_records'])
    check('Preliminary and failed browser attempts retain exact sources and logs without final pass credit', preserved and not prior['counts_as_final_regression'] and len(attempts) >= 1)
    archive = read(E / 'source-archive.json')
    exact = archive['passed'] and sha(ROOT / archive['private_archive']) == archive['archive_sha256']
    with zipfile.ZipFile(ROOT / archive['private_archive']) as saved:
        exact = exact and all(hashlib.sha256(saved.read(v['file'])).hexdigest() == v['sha256'] == sha(ROOT / v['file']) for v in archive['files'])
    check('Private final archive preserves every declared current execution source', exact and archive['all_declared_inputs_included'] and archive['interaction_source_sha256'] == interaction and not archive['counts_as_test_execution'])
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('23 active packages five exclusions and eight draft Skills retain the corrected AI fee-only exclusion', scope['counts']['active'] == 23 and scope['counts']['excluded'] == 5 and scope['flags']['ai_secretary_required'] and scope['flags']['ai_authorization_required'] and scope['flags']['ai_execution_isolation_required'] and not scope['flags']['ai_cost_control_required'] and len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    closed = read(E / 'native-cluster-closed.json')
    ports = True
    for port in (5189, 5190):
        with socket.socket() as probe:
            probe.settimeout(1)
            ports = ports and probe.connect_ex(('127.0.0.1', port)) != 0
    check('Owned durable test PG and temporary browser servers close without stopping development PG', closed['passed'] and closed['remaining_disposable_databases'] == 0 and closed['owned_cluster_stopped'] and closed['development_cluster_preserved'] and closed['verification_script_sha256'] == sha(APP / 'scripts/payment_fee_native_cluster.py') and ports and not (APP / '.local/business-browser-fixture.json').exists())
    proof = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
        'interaction_kind': 'actual_payment_fee_evidence', 'interaction_source_sha256': interaction,
        'mobile_source_sha256': mobile_source, 'backend_source_sha256': source, 'verification_script_sha256': sha(Path(__file__)),
        'new_browser_capture': True, 'new_backend_test_execution': True, 'full_goal_complete': False, 'production_ready': False}
    (E / 'final-source-check.json').write_text(json.dumps(proof, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': proof['passed'], 'checks': len(checks), 'failed': [v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
