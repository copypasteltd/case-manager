"""Bind audit delivery to executed checks, exact artifacts and declared retained evidence."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/audit-console'
PUBLISHED = '66b7707fc363f7021bf4de62ab515a2dbba236b0'


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
    group = E.parent / 'audit-console-backend'
    backend, progress, raw = [read(group / name) for name in ('result.json', 'batch-progress.json', 'raw-log-archive.json')]
    expected = {tuple(v) for v in progress['expected_tests']}
    observed = {(v['Package'],v['Test']) for v in backend['checks'] if '/' not in v['Test']}
    check('Every discovered current Go/SQL/contract test passes without skips', backend['passed'] and backend['source_unchanged_during_tests'] and backend['source_sha256'] == backend_source and expected == observed and backend['all_discovered_tests_executed'] and all(v['Action'] == 'pass' for v in backend['checks']))
    check('Final batch records retain exact run-specific private logs', raw['passed'] and raw['source_sha256'] == backend_source and raw['records'] == len(backend['checks']) and len(raw['logs']) == len(progress['batches']) and raw['freezer_sha256'] == sha(APP / 'scripts/freeze_audit_console_backend.py') and all(sha(APP / v['raw_log']) == v['sha256'] and (APP / v['raw_log']).stat().st_size == v['bytes'] for v in raw['logs']))
    names = {v['Test'] for v in backend['checks']}
    required = {'TestAuditConsoleIntegration','TestAuditFilterValidation','TestAuditCSVIsDeterministicMetadataText','TestAuditCSVRejectsOversizeWithoutTruncation'}
    check('Real authority, four-channel, pagination, CSV, immutability, audit rollback and revocation scenarios execute', required <= names and len([v for v in names if v.startswith('TestAuditConsoleIntegration/')]) == 9)
    failed_contract = read(group / 'attempts/01-incomplete-contract/result.json')
    cleanup = backend.get('cleanup_retry')
    cleanup_valid = True
    if cleanup:
        initial = ROOT / cleanup['initial_record']; attempt = initial.parent / 'attempt.json'
        first = read(initial); retained = read(attempt)
        cleanup_valid = not first['passed'] and first['source_unchanged_during_tests'] and first['source_sha256'] == backend_source and sha(initial) == cleanup['initial_record_sha256'] and sha(attempt) == cleanup['attempt_record_sha256'] and cleanup['verification_script_sha256'] == sha(APP / 'scripts/retry_audit_backend_cleanup.py') and all(sha(APP / v['file']) == v['sha256'] for v in retained['raw_logs'])
    check('Incomplete-contract and native-cleanup failures remain explicit rather than being reclassified as passes', not failed_contract['passed'] and cleanup_valid)
    current = read(ROOT / 'docs/development/openapi.json')
    old = json.loads(subprocess.check_output(['git','show',PUBLISHED + ':docs/development/openapi.json'], cwd=ROOT))
    check('All prior314 API paths and687 models remain exact while the audit contract is added', (ROOT / 'docs/development/openapi.json').read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes() and len(current['paths']) == 320 and len(current['components']['schemas']) == 698 and all(current['paths'][k] == v for k,v in old['paths'].items()) and all(current['components']['schemas'][k] == v for k,v in old['components']['schemas'].items()))
    clients = read(E / 'client-checks.json')
    check('Final client command summaries bind current inputs and exact original logs', clients['passed'] and clients['source_unchanged'] and clients['source_sha256'] == digest(clients['source_files']) and len(clients['checks']) == 8 and all(v['passed'] and v['exit_code'] == 0 and sha(APP / '.local' / Path(v['raw_log']).name) == v['raw_log_sha256'] for v in clients['checks']))
    refresh = clients['web_refresh']; original = read(ROOT / refresh['original_record'])
    previous = {v['source']:ROOT / v['file'] for v in refresh['previous_files']}
    check('Only two Web files changed after the complete original run; three Web checks reran and five unaffected commands retain provenance', sha(ROOT / refresh['original_record']) == refresh['original_record_sha256'] and original['passed'] and original['source_sha256'] == digest(original['source_files'], previous) and set(previous) == {'web/src/App.tsx','web/src/AuditConsole.tsx'} and all(sha(ROOT / v['file']) == v['sha256'] for v in refresh['previous_files']) and set(refresh['fresh_commands']) == {'typecheck','web-tests','web-build'} and len(refresh['retained_commands']) == 5 and refresh['verification_script_sha256'] == sha(APP / 'scripts/refresh_audit_console_web.py'))
    web, mobile, runtime = [read(E / n) for n in ('web-build.json','mobile-build.json','runtime-build.json')]
    mobile_source = module('verify_mobile').digest()[0]
    check('Web185, mobile281 and runtime55 results match their current implementation scopes', web['passed'] and web['source_sha256'] == digest(web['sources']) and web['tests']['numPassedTests'] == 185 and mobile['passed'] and mobile['source_sha256'] == mobile_source and mobile['tests'] == {'pass':281,'fail':0,'cancelled':0,'skipped':0} and runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime').digest()[0] and runtime['tests']['pass'] == 55)
    check('Both actual mini-program builds retain20 configured pages', all(v['page_count'] == 20 and sha(APP / 'mobile/dist' / target / 'app.json') == v['app_json_sha256'] for target,v in mobile['targets'].items()) and set(mobile['targets']) == {'weapp','tt'})
    build, inspection, feature, jvm = [read(E / n) for n in ('build.json','apk-inspection.json','android-feature-content.json','android-test-scope.json')]
    check('Fresh native APK binds exact current source and bundle bytes with18 plus4 static checks', build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android').sources()[0] and inspection['passed'] and feature['passed'] and len(inspection['checks']) == 18 and len(feature['checks']) == 4 and not feature['artifact_executed'] and not feature['real_device'] and feature['verification_script_sha256'] == sha(APP / 'scripts/verify_audit_console_android_content.py') and all(sha(APP / a['path']) == a['sha256'] for a in build['artifacts'].values()))
    check('JVM26 results disclose actual Gradle cache status and retain original XML bytes', jvm['passed'] and jvm['tests'] == 26 and jvm['new_test_execution'] == (not jvm['gradle_cached']) and not jvm['device_executed'] and sha(E / 'native-build.log') == jvm['native_log_sha256'] and len(jvm['xml_sha256']) == 4 and all(sha(E / 'android-unit-results' / k) == v for k,v in jvm['xml_sha256'].items()))
    linux = read(E / 'linux-build.json')
    check('Both current Linux artifacts cross-compile without claiming Linux execution', linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_source and len(linux['artifacts']) == 2 and all(sha(APP / a['file']) == a['sha256'] for a in linux['artifacts']) and not linux['artifact_executed'] and not linux['kernel_boundary_executed'] and not linux['deployed'])
    browser_sources = module('audit_console_sources').digest()
    for client in ('web','mobile'):
        directory = E.parent / ('audit-console-' + client)
        browser, manifest, capture = [read(directory / n) for n in ('result.json','source-manifest.json','capture.json')]
        check('Actual ' + client + ' browser/API journey passes and all owned resources close', browser['passed'] and len(browser['checks']) == 5 and browser['source_unchanged'] and browser['source_sha256'] == manifest['sha256'] == digest(manifest['files']) and browser['build_source_sha256'] == browser['build_source_after_sha256'] == browser_sources and browser['build_source_unchanged'] and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['native_fixture_exit_code'] == 0 and browser['isolated_database_cleanup_passed'] and not browser['console_errors'] and not browser['server_errors'] and all(sha(ROOT / s['file']) == s['sha256'] for s in capture['screenshots']))
    media = read(E / 'media.json')
    check('Both silent MP4/GIF pairs encode exact authenticated synthetic frame manifests', media['passed'] and len(media['recordings']) == 2 and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_audit_media.py') and all(sha(ROOT / r['capture_manifest']) == r['capture_manifest_sha256'] and all(sha(ROOT / a['file']) == a['sha256'] for a in r['artifacts']) for r in media['recordings']))
    retained = read(E / 'reused-evidence.json')
    check('211 previous media and unchanged dependency audit retain exact published bytes and original date', retained['passed'] and retained['published_commit'] == PUBLISHED and len(retained['media']) == 211 and not retained['new_dependency_audit'] and retained['package_lock_sha256'] == sha(APP / 'package-lock.json') and all(sha(ROOT / v['file']) == v['sha256'] for v in [*retained['media'],*retained['records']]) and not subprocess.check_output(['git','diff','--name-only',PUBLISHED,'--','docs/quality/evidence/dependency-integrity'], cwd=ROOT).strip())
    cleanup_record = read(E / 'fixture-cleanup.json')
    check('The three exact inactive invalid fixture remnants are attributed, removed without FORCE and independently checked as zero', cleanup_record['passed'] and len(cleanup_record['removed']) == 3 and cleanup_record['remaining_disposable_databases'] == 0 and not cleanup_record['force_used'] and cleanup_record['development_database_preserved'] and cleanup_record['verification_script_sha256'] == sha(APP / 'scripts/cleanup_audit_fixture_databases.py') and cleanup_record['plan_sha256'] == sha(E / 'fixture-cleanup-plan.json') and all(v['invalid_drop_marker'] and not v['template'] and v['active_connections'] == 0 and v['matched_failed_tests'] and not v['contents_inspected'] for v in cleanup_record['removed']))
    dev = read(E / 'development-project-resources.json'); update = read(E / 'development-update.json')
    check('Native schema84 upgrade preserves historical rows and current owned services, with no fixture databases or integrations enabled', dev['passed'] and dev['migration_count'] == 84 and dev['backend_source_sha256'] == backend_source and dev['database']['disposable_test_databases'] == 0 and dev['integrations_disabled'] and all(dev['ownership'].values()) and dev['worker_sources_match'] and update['passed'] and update['legacy_and_usage_rows_preserved'] and update['backend_source_sha256'] == backend_source and not update['production_deployment'])
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('23 active packages,5 exclusions and8 draft Skills retain AI authorization and isolation without fee controls', scope['counts']['active'] == 23 and scope['counts']['excluded'] == 5 and not scope['flags']['ai_cost_control_required'] and scope['flags']['ai_execution_isolation_required'] and scope['flags']['ai_authorization_required'] and len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    with socket.socket() as probe:
        probe.settimeout(1)
        closed = probe.connect_ex(('127.0.0.1',5189)) != 0
    check('Owned browser fixture file and review port are released', closed and not (APP / '.local/business-browser-fixture.json').exists())
    proof = {'passed':all(v['passed'] for v in checks), 'checked_at':datetime.now(timezone.utc).isoformat(), 'checks':checks,
        'interaction_kind':'organization_audit_console', 'interaction_source_sha256':browser_sources,
        'mobile_source_sha256':mobile_source, 'backend_source_sha256':backend_source,
        'verification_script_sha256':sha(Path(__file__)), 'historical_evidence_commit':PUBLISHED,
        'new_browser_capture':True, 'new_backend_test_execution':True, 'full_goal_complete':False, 'production_ready':False}
    (E / 'final-source-check.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed':proof['passed'], 'checks':len(checks), 'failed':[v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
