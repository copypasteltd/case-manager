"""Bind actual current proofs; keep bounded cash refunds distinct from full acceptance."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import zipfile
from refund_fx_sources import APP, digest

ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/refund-fx'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
read = lambda p: json.loads(p.read_text(encoding='utf-8'))


def module(name):
    spec = importlib.util.spec_from_file_location('refund_verify_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def app_digest(names):
    h = hashlib.sha256()
    for name in names:
        h.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return h.hexdigest()


def main():
    checks = []
    def check(name, value): checks.append({'name': name, 'passed': bool(value)})
    backend_source = module('verify_backend_batches').digest()
    bdir = E.parent / 'refund-fx-backend'
    backend, state, raw = [read(bdir / n) for n in ('result.json', 'batch-progress.json', 'raw-log-archive.json')]
    check('All discovered Go cases pass against complete current Go SQL and API sources', backend['passed']
        and backend['source_sha256'] == state['source_sha256'] == backend_source
        and backend['all_discovered_tests_executed'] and backend['source_unchanged_during_tests']
        and all(v['Action'] == 'pass' for v in backend['checks'])
        and sorted((v['Package'], v['Test']) for v in backend['checks'] if '/' not in v['Test']) == sorted(map(tuple, state['expected_tests']))
        and all(v['status'] == 'passed' for v in state['batches']))
    check('Exact successful raw logs remain private and match final backend results', raw['passed'] and raw['source_sha256'] == backend_source
        and raw['records'] == len(backend['checks']) and all(sha(APP / v['file']) == v['sha256'] for v in raw['logs']))
    plan = read(bdir / 'package-execution-plan.json')
    check('Remaining batches select every actual package without removing discovered cases', plan['passed']
        and plan['source_sha256'] == plan['source_after_sha256'] == backend_source
        and plan['runner_sha256'] == sha(APP / 'scripts/verify_refund_fx_backend.py')
        and plan['all_discovered_test_names_preserved'] and plan['count_flag'] == 1
        and plan['package_parallelism'] == 1 and plan['package_deadline'] == '20m'
        and sorted(plan['retained_completed_batches'] + [v['batch'] for v in plan['invocations']]) == list(range(1,len(state['batches'])+1))
        and all(v['names'] == state['batches'][v['batch']-1]['names']
            and v['packages'] == sorted({p for p,n in state['expected_tests'] if n in v['names']})
            and '-count=1' in v['arguments'] and '-timeout=20m' in v['arguments']
            for v in plan['invocations']))
    names = {v['Test'] for v in backend['checks']}
    check('Actual foreign cash dual settlement fees and legacy payment paths all executed',
        {'TestActualRefundQuote', 'TestActualForeignRefunds', 'TestCrossCurrencySettlement', 'TestPaymentFeeEvidence'} <= names
        and any(n.startswith('TestActualForeignRefunds/concurrent_') for n in names)
        and any(n.startswith('TestActualForeignRefunds/database_quote_') for n in names))
    upgrade = read(E / 'schema-upgrade.json')
    check('Published schema92 finance journey upgrades to93 preserving original facts', upgrade['passed']
        and upgrade['before_schema'] == 92 and upgrade['after_schema'] == 93 and upgrade['source_sha256'] == backend_source
        and upgrade['original_facts_unchanged'] and upgrade['new_principal_matches_legacy_cash'] and upgrade['new_confirmation_defaults_false']
        and len(upgrade['facts']) == 15 and upgrade['facts']['payment_allocations']['rows'] >= 5
        and not upgrade['counts_as_current_backend_tests'] and sha(APP / 'scripts/verify_refund_fx_upgrade.py') == upgrade['verification_script_sha256'])
    clients = read(E / 'client-checks.json')
    cc = {v['name']: v for v in clients['checks']}
    check('All eight client commands completed against unchanged inputs and exact raw logs', clients['passed'] and len(cc) == 8
        and clients['source_sha256'] == clients['source_after_sha256'] == app_digest(clients['source_files'])
        and all(v['passed'] and v['exit_code'] == 0 and sha(APP / '.local' / Path(v['raw_log']).name) == v['raw_log_sha256'] for v in cc.values()))
    web, mobile, runtime = [read(E / n) for n in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    mobile_source = module('verify_mobile').digest()[0]
    check('Client summaries bind complete current sources actual counts and both miniapp route sets', web['passed']
        and web['source_sha256'] == app_digest(web['sources']) and web['tests']['numPassedTests'] == cc['web-tests']['test_count'] >= 317
        and mobile['passed'] and mobile['source_sha256'] == mobile_source and mobile['tests'] == {'pass':cc['mobile-tests']['test_count'],'fail':0,'cancelled':0,'skipped':0}
        and cc['mobile-tests']['test_count'] >= 414 and set(mobile['targets']) == {'weapp','tt'}
        and all(v['page_count'] == 20 and sha(APP / 'mobile/dist' / k / 'app.json') == v['app_json_sha256'] for k,v in mobile['targets'].items())
        and runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime').digest()[0]
        and runtime['tests']['pass'] == cc['runtime-tests']['test_count'] == 56)
    build, inspection, content, jvm = [read(E / n) for n in ('build.json','apk-inspection.json','android-feature-content.json','android-test-scope.json')]
    enumeration=read(E/'android-source-enumeration.json')
    check('Early directory pruning preserves all292 previously selected native source paths',enumeration['passed']
        and enumeration['same_source_paths_as_published120'] and enumeration['source_count']==292
        and not enumeration['added_paths'] and not enumeration['removed_paths']
        and enumeration['build_script_sha256']==sha(APP/'scripts/build_android.py'))
    check('Current development APK binds actual full sources and inspected foreign refund features', build['passed']
        and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android').sources()[0]
        and build['scoped_bundle_invocation']['max_workers'] == 4 and build['scoped_bundle_invocation']['exit_code'] == 0
        and build['scoped_build_script_sha256'] == sha(APP / 'scripts/build_refund_fx_android.py') and build['bundle_source_map_guard_applied']
        and all(sha(APP / v['path']) == v['sha256'] for v in build['artifacts'].values())
        and inspection['passed'] and all(v['passed'] for v in inspection['checks']) and content['passed'] and all(v['passed'] for v in content['checks'])
        and not content['artifact_executed'] and not build['production_release'] and not build['device_executed'])
    check('JVM test cache status is preserved separately from device execution', jvm['passed'] and jvm['tests'] == 26
        and jvm['new_test_execution'] != jvm['gradle_cached'] and not jvm['device_executed']
        and all(sha(E / 'android-unit-results' / n) == v for n,v in jvm['xml_sha256'].items()) and sha(E / 'native-build.log') == jvm['native_log_sha256'])
    linux = read(E / 'linux-build.json')
    check('Current Linux cross builds are intact without a Linux execution claim', linux['passed'] and linux['source_sha256'] == backend_source
        and len(linux['artifacts']) == 2 and all(sha(APP / v['file']) == v['sha256'] for v in linux['artifacts'])
        and not linux['artifact_executed'] and not linux['kernel_boundary_executed'] and not linux['deployed'])
    interaction = digest()
    for client in ('web', 'mobile'):
        browser = read(E.parent / ('refund-fx-' + client) / 'result.json')
        manifest = read(E.parent / ('refund-fx-' + client) / 'source-manifest.json')
        check('Actual '+client+' cash editing confirmation replay and reversal close owned resources', browser['passed']
            and len(browser['checks']) >= 6 and browser['source_sha256'] == browser['source_after_sha256'] == app_digest(manifest['files'])
            and browser['build_source_sha256'] == browser['build_source_after_sha256'] == interaction
            and browser['source_unchanged'] and browser['build_source_unchanged'] and browser['owned_browser_closed']
            and browser['owned_review_server_stopped'] and browser['owned_api_proxy_stopped'] and browser['owned_build_process_stopped']
            and browser['isolated_database_cleanup_passed'] and not browser['console_errors'] and not browser['server_errors'])
    media = read(E / 'media-encoding.json')
    check('Actual screenshots recordings and labelled screenshot tours bind exact reviewed pixels', media['passed'] and len(media['recordings']) == 2
        and all(not v['gif_is_screen_recording'] and len(v['gif_screenshots']) == 3 and all(sha(ROOT / a['file']) == a['sha256'] for a in v['artifacts']) for v in media['recordings'])
        and all(sha(ROOT / v['capture_manifest']) == v['capture_manifest_sha256'] for v in media['recordings']))
    visual = read(E / 'visual-review.json')
    check('Visual review binds all six screenshots and decoded MP4 GIF samples', visual['reviewed'] and len(visual['screenshots']) == 6
        and all(sha(ROOT / v['file']) == v['sha256'] for v in visual['screenshots'] + visual['gifs'])
        and set(visual['recording_frames_reviewed']) == {'web','mobile'} and not visual['findings'])
    retained = read(E / 'reused-evidence.json')
    check('Every prior published proof and all327 prior media retain original bytes', retained['passed'] and retained['prior_media_count'] == 327
        and all(sha(ROOT / v['file']) == v['sha256'] for v in retained['records']))
    archive = read(E / 'source-archive.json')
    check('Current source archive and retained dependency evidence stay honest', archive['passed'] and archive['source_sha256'] == interaction
        and all(sha(APP / v['file']) == v['sha256'] for v in archive['files'])
        and read(E / 'dependency-audit.json')['package_lock_sha256'] == sha(APP / 'package-lock.json')
        and not read(E / 'dependency-audit.json')['production_security_gate_passed'])
    frozen = APP / '.local/refund-fx-final-sources.zip'
    with zipfile.ZipFile(frozen) as z:
        check('Private source archive contains exactly the declared current bytes', sha(frozen) == archive['private_archive_sha256']
            and all(hashlib.sha256(z.read(v['file'])).hexdigest() == v['sha256'] for v in archive['files']))
    attempt_records = [read(p) for p in (E.parent / 'refund-fx-attempts').glob('*/preservation.json')]
    attempts = [a for a in attempt_records if 'source_archive' in a]
    check('Earlier browser attempts preserve exact bytes and original status without final pass credit', len(attempts) >= 8
        and all(not a['counts_as_current_pass'] and sha(ROOT / a['source_archive']) == a['source_archive_sha256']
            and all(sha(ROOT / v['file']) == v['sha256'] for v in a['public_records'] + a['private_records']) for a in attempts))
    check('Incomplete native diagnostics receive no current pass credit and retain exact logs',
        all(not a['counts_as_current_pass'] and not a['android_build_completed']
            and all(sha(ROOT/v['file']) == v['sha256'] for v in a['records'])
            for a in attempt_records if 'android_build_completed' in a))
    check('Cumulative package deadline failure is retained without credit for incomplete cases',
        all(not a['counts_as_current_pass'] and not a['backend_attempt_completed'] and a['source_sha256']==backend_source
            and all(sha(ROOT/v['file'])==v['sha256'] for v in a['records']) for a in attempt_records if 'backend_attempt_completed' in a))
    preliminary = [read(p) for p in (E.parent / 'refund-fx-preliminary').glob('**/preservation.json')]
    check('Earlier client proofs and interrupted Metro logs retain exact source archives without final credit', len(preliminary) == 2
        and all(not a.get('counts_as_final_regression', a.get('counts_as_current_pass', True))
            and sha(ROOT / a.get('source_archive', a.get('private_source_archive'))) == a.get('source_archive_sha256', a.get('private_source_archive_sha256'))
            and all(sha(ROOT / v['file']) == v['sha256'] for v in a['records']) for a in preliminary))
    pixels = True
    for client in ('web','mobile'):
        captured = read(E.parent / ('refund-fx-' + client) / 'media.json')['recording']
        raw_path = APP / captured['manifest']
        frame_record = read(raw_path)
        pixels = pixels and sha(raw_path) == captured['manifest_sha256'] and all(sha(raw_path.parent / v['file']) == v['sha256'] for v in frame_record['frames'])
    check('Every recorded raw viewport frame remains exact and private', pixels)
    dev, update = [read(E / n) for n in ('development-project-resources.json','development-update.json')]
    check('Only owned native development services reload to93 preserving facts and disabled integrations', dev['passed'] and dev['migration_count'] == 93
        and dev['backend_source_sha256'] == backend_source and dev['integrations_disabled'] and dev['database']['disposable_test_databases'] == 0
        and update['passed'] and update['backend_source_sha256'] == backend_source and update['migration_count'] == 93)
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('All23 open packages five exclusions and eight draft Skills preserve fee-only AI exclusion', scope['counts']['active'] == 23 and scope['counts']['excluded'] == 5
        and scope['flags']['ai_secretary_required'] and scope['flags']['ai_execution_isolation_required'] and scope['flags']['ai_authorization_required']
        and not scope['flags']['ai_cost_control_required'] and len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    closed = read(E / 'native-cluster-closed.json')
    cleanup = read(E / 'test-database-cleanup.json')
    check('Interrupted synthetic fixture data remains privately archived before ordinary cleanup', cleanup['passed']
        and cleanup['owned_temporary_cluster_attested'] and cleanup['backend_source_sha256'] == backend_source
        and cleanup['remaining_disposable_databases'] == 0 and cleanup['archives_private']
        and not cleanup['development_or_production_database_modified'] and not cleanup['restoration_executed']
        and not cleanup['historical_migration_or_dr_acceptance']
        and cleanup['verification_script_sha256'] == sha(APP / 'scripts/cleanup_refund_fx_test_databases.py')
        and cleanup['retained_fixture_count'] == len(cleanup['archives'])
        and all(v['database_released'] and v['complete_archive_readable']
            and sha(APP / v['file']) == v['sha256'] and sha(APP / v['catalog_file']) == v['catalog_sha256'] for v in cleanup['archives']))
    check('Owned test cluster browser fixture and fixed review ports are released', closed['passed'] and closed['owned_cluster_stopped']
        and not (APP / '.local/business-browser-fixture.json').exists()
        and all(not port_open(p) for p in (5189,5190)))
    report = {'passed':all(c['passed'] for c in checks), 'checked_at':datetime.now(timezone.utc).isoformat(), 'checks':checks,
        'backend_source_sha256':backend_source, 'mobile_source_sha256':mobile_source, 'interaction_source_sha256':interaction,
        'interaction_kind':'actual_foreign_refunds', 'verification_script_sha256':sha(Path(__file__)),
        'full_goal_complete':False, 'production_ready':False, 'real_device_executed':False}
    (E / 'final-source-check.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed':report['passed'],'checks':len(checks),'failed':[c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


def port_open(port):
    with socket.socket() as sock:
        sock.settimeout(.5)
        return sock.connect_ex(('127.0.0.1',port)) == 0


if __name__ == '__main__': raise SystemExit(main())
