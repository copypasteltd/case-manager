"""Bind complete current dual-currency settlement evidence; retained evidence keeps its actual scope."""
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
E = ROOT / 'docs/quality/evidence/payment-fx'
PUBLISHED = 'd1a31a03d1487776920b61aa946e5cd8c49e9f45'


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
    group = E.parent / 'payment_fx-backend'
    backend, progress, raw = [read(group / name) for name in ('result.json', 'batch-progress.json', 'raw-log-archive.json')]
    expected = {tuple(v) for v in progress['expected_tests']}
    observed = {(v['Package'], v['Test']) for v in backend['checks'] if '/' not in v['Test']}
    check('Every discovered current-source native backend test executes without skips', backend['passed'] and backend['source_sha256'] == backend_source and backend['source_unchanged_during_tests'] and backend['all_discovered_tests_executed'] and expected == observed and backend['discovered_top_level_tests'] == len(expected) and backend['parallel_batches'] == 1 and all(v['Action'] == 'pass' for v in backend['checks']) and backend['verification_script_sha256'] == sha(APP / 'scripts/verify_payment_fx_backend.py'))
    check('Complete successful batches retain exact private raw logs without substituted cases', raw['passed'] and raw['source_sha256'] == backend_source and len(raw['logs']) == len(progress['batches']) and raw['records'] == len(backend['checks']) and raw['freezer_sha256'] == sha(APP / 'scripts/freeze_payment_fx_backend.py') and all(sha(APP / v['raw_log']) == v['sha256'] and not v['excluded_test_families'] for v in raw['logs']))
    feature = [v for v in backend['checks'] if v['Test'].startswith('TestCrossCurrencySettlement/')]
    check('Fifteen dual-currency API and native PG groups pass', len(feature) == 15 and all(v['Action'] == 'pass' for v in feature) and backend['discovered_top_level_tests'] == 238)
    cluster, closed = [read(E / n) for n in ('native-cluster.json', 'native-cluster-closed.json')]
    check('Owned full-durability native test PG closes with no disposable databases', cluster['passed'] and closed['passed'] and all(cluster[n] for n in ('fsync', 'synchronous_commit', 'full_page_writes')) and not cluster['original_development_cluster_modified'] and not cluster['fixture_cleanup_deadline_changed'] and not cluster['virtualization_used'] and closed['owned_cluster_stopped'] and closed['remaining_disposable_databases'] == 0 and cluster['verification_script_sha256'] == closed['verification_script_sha256'] == sha(APP / 'scripts/payment_fx_native_cluster.py'))
    contract = read(APP / 'backend/internal/contract/openapi.json')
    original = json.loads(subprocess.check_output(['git', 'show', PUBLISHED + ':docs/development/openapi.json'], cwd=ROOT))
    expected_changed = {'PaymentAllocate', 'PaymentRelease', 'PaymentRefundAllocate', 'PaymentRecord', 'PaymentAllocation'}
    changed = {key for key, value in original['components']['schemas'].items() if contract['components']['schemas'][key] != value}
    restored = json.loads(json.dumps(contract['components']['schemas']))
    for name, keys in {'PaymentAllocate':['fx'], 'PaymentRelease':['cost_amount'], 'PaymentRefundAllocate':['cost_amount'], 'PaymentAllocation':['cost_amount','cost_currency','payment_currency','cost_exponent','payment_exponent','cost_shares','remaining_cost_amount','fx']}.items():
        for key in keys: restored[name]['properties'].pop(key)
    restored['PaymentRecord']['properties']['refund_allocations']['items']['properties'].pop('cost_amount')
    check('Existing333 paths and409 operations remain exact; only five disclosed schemas extend and two quotation schemas are added', contract['paths'] == original['paths'] and changed == expected_changed and all(restored[key] == value for key,value in original['components']['schemas'].items()) and set(contract['components']['schemas']) - set(original['components']['schemas']) == {'PaymentFXInput','PaymentFXQuote'} and len(contract['paths']) == 333 and sum(len(v) for v in contract['paths'].values()) == 409 and len(contract['components']['schemas']) == 731 and contract == read(ROOT / 'docs/development/openapi.json'))
    upgrade = read(E / 'schema-upgrade.json')
    check('Actual published schema88 cash journey upgrades through current89 migrator without altering original facts', upgrade['passed'] and upgrade['source_sha256'] == backend_source and upgrade['before_schema'] == 88 and upgrade['after_schema'] == 89 and upgrade['original_facts_unchanged'] and upgrade['legacy_dual_units_exact'] and len(upgrade['facts']) == 14 and upgrade['facts']['payment_allocations']['rows'] >= 10 and not upgrade['counts_as_current_backend_tests'] and upgrade['verification_script_sha256'] == sha(APP / 'scripts/verify_payment_fx_upgrade.py') and sha(APP / '.local/payment-fx-upgrade.log') == upgrade['raw_log_sha256'])
    client = read(E / 'client-checks.json')
    check('Eight client commands run against unchanged exact source with retained raw hashes', client['passed'] and client['source_unchanged'] and client['source_sha256'] == client['source_after_sha256'] == digest(client['source_files']) and len(client['checks']) == 8 and all(v['passed'] and v['exit_code'] == 0 and sha(APP / '.local' / Path(v['raw_log']).name) == v['raw_log_sha256'] for v in client['checks']))
    web, mobile, runtime = [read(E / n) for n in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    mobile_source = module('verify_mobile').digest()[0]
    check('Web294 mobile390 runtime55 match current complete sources and actual command records', web['passed'] and web['tests']['numPassedTests'] == 294 and web['source_sha256'] == digest(web['sources']) and mobile['passed'] and mobile['tests'] == {'pass': 390, 'fail': 0, 'cancelled': 0, 'skipped': 0} and mobile['source_sha256'] == mobile_source and runtime['status'] == 'passed' and runtime['tests']['pass'] == 55 and runtime['source_sha256'] == module('verify_agent_runtime').digest()[0] and all(v['derived_from']['sha256'] == sha(E / 'client-checks.json') for v in (web, mobile, runtime)))
    check('Both built miniapps retain all20 routes', set(mobile['targets']) == {'weapp', 'tt'} and all(v['page_count'] == 20 and sha(APP / 'mobile/dist' / k / 'app.json') == v['app_json_sha256'] for k, v in mobile['targets'].items()))
    build, inspection, content, jvm = [read(E / n) for n in ('build.json', 'apk-inspection.json', 'android-feature-content.json', 'android-test-scope.json')]
    check('New development APK matches current exact native and portable inputs without device execution', build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android').sources()[0] and all(sha(APP / v['path']) == v['sha256'] for v in build['artifacts'].values()) and inspection['passed'] and len(inspection['checks']) == 18 and content['passed'] and len(content['checks']) == 3 and not content['artifact_executed'] and not content['real_device'] and content['verification_script_sha256'] == sha(APP / 'scripts/verify_payment_fx_android_content.py'))
    check('JVM26 retains actual Gradle cache status and four exact XMLs', jvm['passed'] and jvm['tests'] == 26 and jvm['new_test_execution'] == (not jvm['gradle_cached']) and not jvm['device_executed'] and sha(E / 'native-build.log') == jvm['native_log_sha256'] and len(jvm['xml_sha256']) == 4 and all(sha(E / 'android-unit-results' / name) == value for name, value in jvm['xml_sha256'].items()))
    linux = read(E / 'linux-build.json')
    check('Two current Linux ELF targets cross-compile without execution or deployment', linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_source and len(linux['artifacts']) == 2 and all(sha(APP / v['file']) == v['sha256'] for v in linux['artifacts']) and not linux['artifact_executed'] and not linux['kernel_boundary_executed'] and not linux['deployed'] and linux['verification_script_sha256'] == sha(APP / 'scripts/build_payment_fx_linux.py'))
    interaction = module('payment_fx_sources').digest()
    shots = []
    for target in ('web', 'mobile'):
        folder = E.parent / ('payment-fx-' + target)
        browser, manifest, capture = [read(folder / n) for n in ('result.json', 'source-manifest.json', 'media.json')]
        check('Actual ' + target + ' payment_fx passes six groups and releases owned resources', browser['passed'] and len(browser['checks']) == 6 and browser['source_unchanged'] and browser['source_sha256'] == manifest['sha256'] == digest(manifest['files']) and browser['build_source_sha256'] == browser['build_source_after_sha256'] == interaction and browser['build_source_unchanged'] and all(browser[n] for n in ('owned_browser_closed', 'owned_review_server_stopped', 'owned_api_proxy_stopped', 'isolated_database_cleanup_passed')) and browser['native_fixture_exit_code'] == 0 and not browser['console_errors'] and not browser['server_errors'] and browser['h5_prebuilt_before_fixture'] == (target == 'mobile') and browser['real_api_forwarded_without_response_substitution'] == (target == 'mobile') and not capture['api_responses_mocked'] and not capture['real_device'] and all(sha(ROOT / v['file']) == v['sha256'] for v in capture['screenshots']))
        shots.extend(capture['screenshots'])
    media, visual = [read(E / n) for n in ('media.json', 'visual-review.json')]
    check('Two silent MP4 recordings and labelled screenshot-tour GIFs bind their actual synthetic pixels', media['passed'] and len(media['recordings']) == 2 and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_payment_fx_media.py') and all(sha(ROOT / v['capture_manifest']) == v['capture_manifest_sha256'] and v['gif_mode'] == 'actual_screenshot_tour' and not v['gif_is_screen_recording'] and v['gif_frame_duration_ms'] == 2000 and len(v['gif_screenshots']) == 3 and all(sha(ROOT / s['file']) == s['sha256'] for s in v['gif_screenshots']) and all(sha(ROOT / a['file']) == a['sha256'] for a in v['artifacts']) for v in media['recordings']))
    check('Visual review binds six screenshots and decoded MP4/GIF samples from both clients', visual['reviewed'] and len(shots) == len(visual['screenshots']) == 6 and len(visual['gifs']) == 2 and all(sha(ROOT / v['file']) == v['sha256'] for v in visual['screenshots'] + visual['gifs']) and set(visual['gif_frames_reviewed']) == set(visual['recording_frames_reviewed']) == {'web', 'mobile'} and all(sha(ROOT / v['file']) == v['sha256'] and len(v['sampled_seconds']) == 2 and not v['credential_fields_visible'] for v in visual['recording_frames_reviewed'].values()))
    retained = read(E / 'reused-evidence.json')
    check('All previously published media and dependency evidence retain exact published bytes and dates', retained['passed'] and retained['published_commit'] == PUBLISHED and len(retained['media']) >= 286 and all(sha(ROOT / v['file']) == v['sha256'] for v in retained['media'] + retained['records']) and retained['package_lock_sha256'] == sha(APP / 'package-lock.json') and retained['workspace_dependency_fields_unchanged'] and retained['mobile_test_selection_updated'] and retained['contracts_source_export_added'] and not retained['new_dependency_audit'])
    development, update = [read(E / n) for n in ('development-project-resources.json', 'development-update.json')]
    check('Only attested development services reload while schema89 upgrades once from88 and fee history remain fixed', development['passed'] and update['passed'] and development['migration_count'] == update['migration_count'] == 89 and update['previous_migration_count'] == 88 and update['schema_upgrade_executed'] and update['legacy_and_usage_rows_preserved'] and development['backend_source_sha256'] == update['backend_source_sha256'] == backend_source and all(development['ownership'].values()) and development['worker_sources_match'] and development['integrations_disabled'] and development['database']['disposable_test_databases'] == 0 and development['verification_script_sha256'] == sha(APP / 'scripts/verify_payment_fx_development.py') and update['updater_sha256'] == sha(APP / 'scripts/update_payment_fx_development.py') and not update['production_deployment'])
    attempts = read(E / 'attempts/early-checks.json')
    check('Failed and interrupted preliminary checks retain private raw hashes without final regression credit', attempts['records'] and all(sha(ROOT / v['file']) == v['sha256'] and not v['counts_as_final_full_regression'] for v in attempts['records']))
    upgrade_attempt = read(E / 'attempts/upgrade-output-chunks.json')
    check('Initial upgrade output-collection failure retains exact private logs, migrator and collector without passing credit', not upgrade_attempt['passed'] and not upgrade_attempt['counts_as_final_full_regression'] and all(sha(ROOT / v['file']) == v['sha256'] for v in upgrade_attempt['records']))
    earlier = E / 'attempts/03-before-mobile-validation-fix'
    preserved = read(earlier / 'preservation.json')
    supplement = read(earlier / 'source-archive-supplement.json')
    old_client, old_build = [read(earlier / name) for name in ('client-checks.json', 'build.json')]
    with zipfile.ZipFile(ROOT / preserved['private_source_archive']) as archived, zipfile.ZipFile(ROOT / supplement['source_archive']) as extra:
        available = set(archived.namelist())
        def old_bytes(name):
            return archived.read(name) if name in available else extra.read(name)
        old_source = hashlib.sha256()
        for name in old_client['source_files']:
            old_source.update(name.encode() + b'\0' + old_bytes(name) + b'\0')
        old_web = hashlib.sha256()
        for name in read(earlier / 'web/source-manifest.json')['files']:
            old_web.update(name.encode() + b'\0' + old_bytes(name) + b'\0')
        old_android_source = hashlib.sha256()
        for name in old_build['sources']:
            old_android_source.update(name.encode() + b'\0' + old_bytes(name) + b'\0')
        supplement_exact = supplement['passed'] and not supplement['counts_as_new_test_execution'] and sha(ROOT / supplement['source_archive']) == supplement['source_archive_sha256'] and all(hashlib.sha256(extra.read(v['file'])).hexdigest() == v['sha256'] for v in supplement['files'])
    with zipfile.ZipFile(APP / old_build['artifacts']['apk']['path']) as apk:
        old_apk_bundle = hashlib.sha256(apk.read('assets/index.android.bundle')).hexdigest()
    check('Earlier passed client/APK/Web executions retain their original sources and artifacts before the mobile notice correction', preserved['passed_original_checks'] and not preserved['counts_as_final_current_source'] and sha(ROOT / preserved['private_source_archive']) == preserved['private_source_archive_sha256'] and all(sha(ROOT / v['file']) == v['sha256'] for v in preserved['private_files']) and supplement_exact and old_client['passed'] and old_source.hexdigest() == old_client['source_sha256'] and old_web.hexdigest() == preserved['web_source_sha256'] and old_android_source.hexdigest() == old_build['source_sha256'] and old_apk_bundle == old_build['artifacts']['bundle']['sha256'] and sha(APP / old_build['artifacts']['apk']['path']) == old_build['artifacts']['apk']['sha256'])
    eof_folder = E / 'attempts/06-web-eof-whitespace'
    eof = read(eof_folder / 'preservation.json')
    eof_supplement = read(eof_folder / 'source-archive-supplement.json')
    navigation_folder = E / 'attempts/07-mobile-navigation-race'
    navigation = read(navigation_folder / 'navigation-preservation.json')
    with zipfile.ZipFile(ROOT / navigation['source_archive']) as navigation_archive:
        original_checker = navigation_archive.read('scripts/check_payment_fx.mjs')
    prior_client = read(eof_folder / 'client-checks.json')
    prior_publication = read(eof_folder / 'repository-check.json')
    with zipfile.ZipFile(ROOT / eof['source_archive']) as source, zipfile.ZipFile(ROOT / eof_supplement['source_archive']) as extra:
        available = set(source.namelist())
        def eof_bytes(name):
            return source.read(name) if name in available else extra.read(name)
        def archived_digest(names):
            value = hashlib.sha256()
            for name in names:
                value.update(name.encode() + b'\0' + eof_bytes(name) + b'\0')
            return value.hexdigest()
        prior_exact = archived_digest(module('payment_fx_sources').sources()) == eof['old_interaction_source_sha256']
        prior_exact = prior_exact and archived_digest(prior_client['source_files']) == prior_client['source_sha256']
        prior_exact = prior_exact and all(source.read(name) == (original_checker if name == 'scripts/check_payment_fx.mjs' else (APP / name).read_bytes() + (b'\r\n' if name == eof['corrected_file'] else b'')) for name in source.namelist())
        prior_exact = prior_exact and eof_supplement['passed'] and not eof_supplement['counts_as_new_test_execution'] and eof_supplement['original_archive_preserved'] and eof_supplement['original_archive_sha256'] == eof['source_archive_sha256'] and eof_supplement['byte_source_archive_sha256'] == navigation['source_archive_sha256'] and sha(ROOT / eof_supplement['source_archive']) == eof_supplement['source_archive_sha256'] and len(eof_supplement['files']) == 2 and all(hashlib.sha256(extra.read(v['file'])).hexdigest() == v['sha256'] for v in eof_supplement['files']) and sha(ROOT / eof_supplement['previous_verifier']['file']) == eof_supplement['previous_verifier']['sha256']
        for target in ('web', 'mobile'):
            prior_browser = read(eof_folder / target / 'result.json')
            prior_manifest = read(eof_folder / target / 'source-manifest.json')
            prior_exact = prior_exact and prior_browser['passed'] and prior_browser['owned_browser_closed'] and archived_digest(prior_manifest['files']) == prior_browser['source_sha256'] == prior_manifest['sha256'] and prior_browser['build_source_sha256'] == eof['old_interaction_source_sha256']
    check('The publication EOF failure and prior successful executions retain exact sources, logs and media before fresh clients and UI reruns', eof['passed_original_checks'] and not eof['counts_as_final_current_source'] and eof['removed_bytes'] == 2 and not eof['code_tokens_changed'] and eof['backend_inputs_unchanged'] and eof['android_inputs_unchanged'] and prior_exact and eof['new_interaction_source_sha256'] == navigation['interaction_source_sha256'] and sha(ROOT / eof['source_archive']) == eof['source_archive_sha256'] and sha(APP / 'scripts/archive_payment_fx_whitespace.py') == eof['archive_script_sha256'] and all(sha(ROOT / v['file']) == v['sha256'] for v in eof['public_records'] + eof['private_records']) and not prior_publication['passed'] and prior_publication['publication_whitespace_findings'] == ['app/web/src/Payments.tsx'])
    with zipfile.ZipFile(ROOT / navigation['source_archive']) as source:
        old_interaction, old_web = hashlib.sha256(), hashlib.sha256()
        for name in module('payment_fx_sources').sources():
            old_interaction.update(name.encode() + b'\0' + source.read(name) + b'\0')
        for name in read(navigation_folder / 'passed-web/source-manifest.json')['files']:
            old_web.update(name.encode() + b'\0' + source.read(name) + b'\0')
        unchanged_app = all(source.read(name) == (APP / name).read_bytes() for name in source.namelist() if name != 'scripts/check_payment_fx.mjs')
    original_text = original_checker.decode().replace('\r\n', '\n')
    added_helper = '''  async function waitMobilePaymentDetail() {
    // The command validates its ledger receipt before redirecting. Wait for
    // that real navigation; forcing another hash navigation races Taro.
    await page.waitForURL(url => url.hash === '#/pages/finance/index?kind=payment&id=' + payment.id);
    await root().getByText('资金记录', { exact: true }).waitFor();
    await root().getByText('当前可读分配流水', { exact: true }).waitFor();
  }
'''
    wanted_checker = original_text.replace('  async function confirmMFA() {', added_helper + '  async function confirmMFA() {', 1).replace("  await open('release');", "  if (mobile) await waitMobilePaymentDetail();\n  await open('release');", 1).replace('  await open(); await root().getByText(/费用原币/)', '  if (mobile) await waitMobilePaymentDetail(); else await open();\n  await root().getByText(/费用原币/)', 1)
    check('The navigation repair only waits for real command completion while preserving assertions, deadlines and original failed/passed records', navigation['passed_original_web'] and not navigation['mobile_passed'] and not navigation['counts_as_final_current_source'] and unchanged_app and old_interaction.hexdigest() == navigation['interaction_source_sha256'] and old_web.hexdigest() == read(navigation_folder / 'passed-web/result.json')['source_sha256'] and sha(ROOT / navigation['source_archive']) == navigation['source_archive_sha256'] and hashlib.sha256(original_checker).hexdigest() == navigation['checker_sha256'] and wanted_checker == (APP / 'scripts/check_payment_fx.mjs').read_text(encoding='utf-8') and sha(APP / 'scripts/archive_payment_fx_navigation.py') == navigation['archive_script_sha256'] and all(sha(ROOT / v['file']) == v['sha256'] for v in navigation['public_records'] + navigation['private_records']))
    browser_attempts = []
    for path in sorted((E / 'attempts').glob('*/attempt.json')):
        attempt = read(path)
        archived = ROOT / attempt['private_source_archive']
        valid = not attempt['passed'] and not attempt['counts_as_passed'] and attempt['source_binding_verified'] and sha(archived) == attempt['archive_sha256']
        for folder in ('web', 'mobile'):
            if not (path.parent / folder).exists():
                continue
            result = read(path.parent / folder / 'result.json')
            manifest = read(path.parent / folder / 'source-manifest.json')
            h = hashlib.sha256()
            with zipfile.ZipFile(archived) as source:
                for file in manifest['files']:
                    h.update(file.encode() + b'\0' + source.read(file) + b'\0')
            valid = valid and not result['passed'] and result['source_unchanged'] and result['owned_browser_closed'] and result['owned_review_server_stopped'] and result['isolated_database_cleanup_passed'] and h.hexdigest() == manifest['sha256'] == attempt['source_sha256']
        valid = valid and all(sha(ROOT / v['path']) == v['sha256'] for v in attempt['private_logs'])
        browser_attempts.append(valid)
    check('Earlier browser attempts retain exact prior source archives and closed resources without passing credit', all(browser_attempts))
    lowering = read(E / 'attempts/05-taro-bigint-lowering/diagnosis.json')
    check('Observed Taro BigInt lowering retains original compiled chunks and the prior static inspection scope', not lowering['counts_as_final_acceptance'] and len(lowering['private_records']) >= 2 and all(sha(ROOT / v['file']) == v['sha256'] for v in lowering['private_records']))
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
        'interaction_kind': 'human_cross_currency_settlement', 'interaction_source_sha256': interaction,
        'mobile_source_sha256': mobile_source, 'backend_source_sha256': backend_source, 'verification_script_sha256': sha(Path(__file__)),
        'historical_evidence_commit': PUBLISHED, 'new_browser_capture': True, 'new_backend_test_execution': True,
        'full_goal_complete': False, 'production_ready': False}
    (E / 'final-source-check.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': proof['passed'], 'checks': len(checks), 'failed': [v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
