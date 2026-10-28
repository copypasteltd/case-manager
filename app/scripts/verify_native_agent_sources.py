"""Bind final native broker checks to exact sources and retain honest UI provenance."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/native-agent-runtime'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def module(name):
    spec = importlib.util.spec_from_file_location('native_proof_' + name, APP / 'scripts' / (name + '.py'))
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
    source = module('verify_backend_batches').digest()
    backend = read(E.parent / 'native-agent-backend/result.json')
    state = read(E.parent / 'native-agent-backend/batch-progress.json')
    baseline = read(ROOT / 'docs/quality/current-functional-baseline.json')
    check('Every discovered backend test ran on unchanged final Go SQL and contract inputs',
          backend['passed'] and backend['source_sha256'] == state['source_sha256'] == source and backend['source_unchanged_during_tests']
          and backend['all_discovered_tests_executed'] and backend['discovered_top_level_tests'] == baseline['backend_top_level_tests'] == 219
          and len(backend['checks']) == baseline['backend_result_records']
          and backend['checks'] == [v for b in state['batches'] for v in b['checks']]
          and all(b['status'] == 'passed' and b['source_unchanged_during_tests'] for b in state['batches']) and all(v['Action'] == 'pass' for v in backend['checks']))
    native = [v for v in backend['checks'] if v['Package'].endswith('/internal/nativeruntime')]
    required = {'TestNativeObservationRequiresActualIndependentBoundaryFacts','TestNativeBrokerSignsFreshEvidenceAndStopsOnDisconnect',
                'TestNativeBrokerDoesNotSignInvalidOrUnrecordedObservation','TestNativeBrokerUnknownTeardownBlocksNewWorkAndKeepsMarker',
                'TestNativeGatewayKeepsProviderCredentialOutsideChildAndPinsTechnicalBounds','TestNativeGatewayRejectsPeerPathAuthorityAndRemoteProviderTools',
                'TestNativeGatewayRunCancellationClosesOwnedProviderStream','TestNativeGatewayBoundsConcurrentHostWork',
                'TestNativeConnectionLimitReleasesExactlyOnceAndRefusesOverflow','TestNativeControllerNeverSubstitutesAWindowsBoundary'}
    check('Actual controlled signature lifecycle peer HTTP and refusal tests are present in the full result', required <= {v['Test'] for v in native} and all(v['Action'] == 'pass' for v in native))
    archive = read(E.parent / 'native-agent-backend/raw-log-archive.json')
    check('Final11 batch logs are privately archived with exact current-source bytes',
          archive['passed'] and archive['source_sha256'] == source and archive['records'] == len(backend['checks']) and len(archive['logs']) == 11
          and archive['freezer_sha256'] == sha(APP / 'scripts/freeze_native_agent_backend.py')
          and all(sha(APP / v['raw_log']) == v['sha256'] for v in archive['logs']))
    attempt = read(E / 'attempts/01-before-tool-output-hardening/result.json')
    old_state = read(E / 'attempts/01-before-tool-output-hardening/batches/batch-progress.json')
    check('Intermediate source-change attempt is retained as incomplete and is not reused',
          not attempt['passed'] and not attempt['full_goal_complete'] and attempt['prior_source_sha256'] == old_state['source_sha256'] != source
          and len(attempt['logs']) == 5 and old_state['batches'][4]['status'] == 'failed' and old_state['batches'][4]['exit_code'] == 0
          and not old_state['batches'][4]['source_unchanged_during_tests'])
    runtime = read(E / 'runtime-build.json')
    check('Current55 runtime tests compilation and Skill checks bind exact current inputs',
          runtime['status'] == 'passed' and runtime['source_sha256'] == runtime['source_after_sha256'] == module('verify_agent_runtime').digest()[0]
          and runtime['tests'] == {'tests':55,'pass':55,'fail':0,'skipped':0,'cancelled':0} and all(v['exit_code'] == 0 for v in runtime['commands']))
    package = read(E / 'package-checks.json')
    release = APP / '.local/native-agent/runtime-release-v2'
    check('Real dedicated release imports successfully with exactly matching approved input bytes',
          package['passed'] and package['file_count'] == 685 and package['packaged_skill_count'] == 8 and package['all_skills_remain_draft']
          and package['release_sha256'] == module('build_native_runtime_release').release_digest(release)
          and package['packager_sha256'] == sha(APP / 'scripts/build_native_runtime_release.py')
          and package['tests_sha256'] == sha(APP / 'scripts/test_native_runtime_release.py')
          and package['verification_script_sha256'] == sha(APP / 'scripts/verify_native_agent_package.py')
          and all(sha(release / v['file']) == v['sha256'] == sha(ROOT / v['file']) for v in package['inventory'])
          and package['all_packaged_bytes_match_current_inputs'] and package['private_or_native_paths_excluded']
          and not package['linux_ownership_validated'] and not package['kernel_boundary_executed'] and not package['real_model_invoked'])
    linux = read(E / 'linux-build.json')
    check('Both native targets compile as current Linux ELF64 without an execution claim',
          linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == source and len(linux['artifacts']) == 2
          and linux['verification_script_sha256'] == sha(APP / 'scripts/verify_native_agent_linux_build.py')
          and all(sha(APP / v['file']) == v['sha256'] and v['format'] == 'ELF64-x86-64' for v in linux['artifacts'])
          and not linux['artifact_executed'] and not linux['kernel_boundary_executed'] and not linux['deployed'])
    policy = read(APP / 'deploy/native/agent-native.example.json')
    check('Published native templates keep host secrets separate and preserve fixed operator-reviewed bindings',
          policy['model_origin'] == 'https://api.openai.com/v1' and policy['limits']['memory_bytes'] == 1 << 30 and policy['max_concurrency'] == 3
          and policy['entry'] == policy['release'] + '/app/agent-runtime/dist/worker.js'
          and ' '.join(policy['library_mounts']).find('/etc ') == -1 and policy['model_key_file'] != policy['signing_key_file']
          and 'CP_AGENT_OPENAI_API_KEY=\n' in (APP / 'deploy/native/agent-worker.env.example').read_text(encoding='utf-8')
          and all(' --socket ' + policy['socket'] in (APP / 'deploy/native' / name).read_text(encoding='utf-8') for name in ('agent-native-launch.sh','agent-native-probe.sh')))
    reused = read(E / 'reused-evidence.json')
    check('Unchanged Web mobile and Android proof inputs retain104 scope without new execution',
          reused['passed'] and reused['published_commit'] == '3517ecfefe359d898f0ec40ac5c618fd99b6c29f'
          and reused['verification_script_sha256'] == sha(APP / 'scripts/retain_native_agent_evidence.py')
          and not reused['new_web_mobile_android_build_execution'] and not reused['new_browser_capture'] and not reused['current_backend_browser_flow_claimed']
          and all(sha(ROOT / v['file']) == v['sha256'] and ('copied_to' not in v or sha(ROOT / v['copied_to']) == v['sha256']) for v in reused['records']))
    mobile, android = [read(E / n) for n in ('mobile-build.json','build.json')]
    web = read(E.parent / 'multipart-cleanup/web-build.json')
    check('Retained159 Web261 mobile and Android builds still match exact current client inputs',
          web['passed'] and web['source_sha256'] == digest(web['sources']) and web['tests']['numPassedTests'] == 159
          and mobile['passed'] and mobile['source_sha256'] == module('verify_mobile').digest()[0] == reused['mobile_source_sha256'] and mobile['tests']['pass'] == 261
          and android['passed'] and android['source_unchanged_during_build']
          and android['source_sha256'] == digest(android['sources']) == reused['android_source_sha256']
          and all(sha(APP / v['path']) == v['sha256'] for v in android['artifacts'].values())
          and all(v['page_count'] == 20 and sha(APP / 'mobile/dist' / target / 'app.json') == v['app_json_sha256'] for target, v in mobile['targets'].items()))
    jvm = read(E.parent / 'multipart-cleanup/android-test-scope.json')
    check('Retained JVM26 results remain explicitly cached original evidence',
          jvm['passed'] and jvm['tests'] == 26 and jvm['gradle_cached'] and not jvm['new_test_execution'] and not reused['new_jvm_execution']
          and sha(E.parent / 'multipart-cleanup/native-build.log') == jvm['native_log_sha256']
          and len(jvm['xml_sha256']) == 4
          and all(sha(E.parent / 'multipart-cleanup' / jvm['xml_artifact_directory'] / name) == value for name, value in jvm['xml_sha256'].items()))
    media_ok = len(reused['media']) == 199
    for v in reused['media']:
        data = (ROOT / v['file']).read_bytes()
        media_ok &= hashlib.sha256(data).hexdigest() == v['sha256'] and hashlib.sha1(b'blob ' + str(len(data)).encode() + b'\0' + data).hexdigest() == v['git_blob'] and len(data) == v['bytes']
    check('All199 original screenshots MP4 and GIF files preserve published bytes and Git objects', media_ok and reused['media_counts'] == {'png':113,'mp4':43,'gif':43})
    development, update = [read(E / n) for n in ('development-project-resources.json','development-update.json')]
    check('Owned development schema82 services and current-source inspection pass without external activation',
          development['passed'] and update['passed'] and development['backend_source_sha256'] == update['backend_source_sha256'] == source
          and development['migration_count'] == update['migration_count'] == 82 and development['database']['disposable_test_databases'] == 0
          and development['integrations_disabled'] and update['legacy_and_usage_rows_preserved']
          and development['wrapper_sha256'] == sha(APP / 'scripts/verify_native_agent_development.py')
          and update['wrapper_sha256'] == sha(APP / 'scripts/update_native_agent_development.py')
          and development['delegated_implementation_sha256'] == sha(APP / 'scripts/verify_multipart_cleanup_development.py')
          and update['delegated_implementation_sha256'] == sha(APP / 'scripts/update_multipart_cleanup_development.py')
          and not development['native_controller_started'] and not update['native_controller_started'])
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('All23 packages eight drafts and five exclusions remain explicit without AI fee gates',
          sum(v['status'] == 'active' for v in scope['items']) == 23 and sum(v['status'] == 'excluded' for v in scope['items']) == 5
          and all(next(v for v in scope['items'] if v['id'] == gap)['status'] == 'active' for gap in ('GAP-09','GAP-10','GAP-13'))
          and not scope['flags']['ai_cost_control_required'] and scope['flags']['ai_execution_isolation_required']
          and len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    audit = read(E.parent / 'dependency-remediation/dependency-audit.json')
    findings = read(E.parent / 'dependency-remediation/remaining-findings.json')
    check('Unchanged dependency lock retains all70 open findings and no new audit claim',
          audit['package_lock_sha256'] == sha(APP / 'package-lock.json') == package['package_lock_sha256']
          and audit['vulnerabilities'] == findings['counts'] and findings['counts']['total'] == 70 and len(findings['items']) == 70
          and all(v['status'] == 'open' for v in findings['items']) and not audit['production_security_gate_passed'])
    with socket.socket() as probe:
        probe.settimeout(1)
        check('No owned review browser fixture or temporary port5189 remains', probe.connect_ex(('127.0.0.1',5189)) != 0 and not (APP / '.local/business-browser-fixture.json').exists())
    proof = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
             'backend_source_sha256': source, 'mobile_source_sha256': module('verify_mobile').digest()[0],
             'interaction_source_sha256': module('native_agent_sources').digest(), 'interaction_kind':'native_execution_controller_and_retained_ui',
             'verification_script_sha256':sha(Path(__file__)), 'new_backend_test_execution':True, 'new_browser_capture':False,
             'kernel_boundary_executed':False, 'real_model_invoked':False, 'full_goal_complete':False, 'production_ready':False}
    (E / 'final-source-check.json').write_text(json.dumps(proof,indent=2) + '\n',encoding='utf-8')
    print(json.dumps({'passed':proof['passed'],'checks':len(checks),'failed':[v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
