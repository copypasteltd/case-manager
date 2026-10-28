"""Verify current multipart implementation, build results and honest acceptance scope."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/multipart-cleanup'
PUBLISHED = '8ea95266ddb35ad5629ead455564ce6ae94090cc'


def module(name):
    spec = importlib.util.spec_from_file_location('cleanup_proof_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def main():
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    backend = read(E.parent / 'multipart-cleanup-backend/result.json')
    state = read(E.parent / 'multipart-cleanup-backend/batch-progress.json')
    source = module('verify_backend_batches').digest()
    expected = {tuple(value) for value in state['expected_tests']}
    observed = {(v['Package'], v['Test']) for v in backend['checks'] if '/' not in v['Test']}
    check('Every discovered backend test passes against complete current Go/SQL/contract inputs',
          backend['passed'] and backend['source_sha256'] == source and backend['source_unchanged_during_tests']
          and backend['all_discovered_tests_executed'] and expected == observed and all(v['Action'] == 'pass' for v in backend['checks']))
    raw = read(E.parent / 'multipart-cleanup-backend/raw-log-archive.json')
    check('Current backend batch logs retain exact private bytes in a run-specific archive',
          raw['passed'] and raw['source_sha256'] == source and raw['records'] == len(backend['checks'])
          and len(raw['logs']) == len(state['batches']) and raw['freezer_sha256'] == sha(APP / 'scripts/freeze_multipart_cleanup_backend.py')
          and all(sha(APP / v['raw_log']) == v['sha256'] and (APP / v['raw_log']).stat().st_size == v['bytes'] for v in raw['logs']))
    cleanup = {v['Test'] for v in backend['checks'] if v['Test'].startswith('TestMultipartCleanup')}
    required_cleanup = {
        'TestMultipartCleanupFourHumanClients/' + client for client in ('web', 'android', 'wechat', 'douyin')
    } | {
        'TestMultipartCleanupAuthorizationAndSourceChanges/' + name for name in (
            'hidden_scope_stale_revision_and_changed_policy_cannot_authorize_cleanup',
            'expired_MFA_cannot_approve_and_cancellation_prevents_queued_execution',
            'revoked_approval_session_blocks_worker_before_cloud_access')
    } | {
        'TestMultipartCleanupRetriesAndReceipts/' + name for name in (
            'denial_stays_failed_and_explicit_retry_preserves_old_results',
            'uncertain_abort_retries_instead_of_inventing_released_bytes',
            'receipt_audit_rollback_reconciles_absence_without_a_second_abort',
            'expired_lease_cannot_perform_cloud_operations')
    } | {
        'TestMultipartCleanupAtomicityAndExhaustion/' + name for name in (
            'approval_audit_failure_rolls_back_reservations_and_replay_queues_exactly_once',
            'changed_provider_binding_blocks_approved_work_before_I_O',
            'crashed_final_attempt_has_an_immutable_failure_receipt_and_explicit_retry')
    }
    check('Four-client cleanup, authorization, retries, atomic approval and exhausted-worker receipts execute',
          required_cleanup.issubset(cleanup))
    check('Official SDK fixtures reject generic missing-object and permission errors as absence proof',
          any(v['Test'] == 'TestMultipartAbsenceRequiresExactProviderEvidence' for v in backend['checks'])
          and any(v['Test'] == 'TestMultipartCleanupSeparatesAPIMetadataAndWorkerIdentity' for v in backend['checks']))
    clients = read(E / 'client-checks.json')
    check('Eight fresh client commands retain matching current inputs and original raw logs',
          clients['passed'] and clients['source_unchanged'] and clients['source_sha256'] == digest(clients['source_files'])
          and len(clients['checks']) == 8 and all(v['passed'] and sha(APP / '.local' / Path(v['raw_log']).name) == v['raw_log_sha256'] for v in clients['checks']))
    web, mobile, runtime = [read(E / name) for name in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    mobile_source = module('verify_mobile').digest()[0]
    check('Web159/mobile261/runtime52 results and both twenty-page mini-program builds bind current sources',
          web['passed'] and web['source_sha256'] == digest(web['sources']) and web['tests']['numPassedTests'] == 159
          and mobile['passed'] and mobile['source_sha256'] == mobile_source and mobile['tests']['pass'] == 261
          and runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime').digest()[0]
          and runtime['tests']['pass'] == 52 and all(v['page_count'] == 20 and sha(APP / 'mobile/dist' / target / 'app.json') == v['app_json_sha256'] for target, v in mobile['targets'].items()))
    build, apk, jvm = [read(E / name) for name in ('build.json', 'apk-inspection.json', 'android-test-scope.json')]
    check('Native Android bundle and APK inspections bind current sources without a device claim',
          build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android').sources()[0]
          and all(sha(APP / v['path']) == v['sha256'] for v in build['artifacts'].values()) and apk['passed'] and len(apk['checks']) == 18)
    feature = read(E / 'android-feature-content.json')
    check('Actual Android package contains all cleanup controls and protocols',
          feature['passed'] and len(feature['checks']) == 4 and all(v['passed'] for v in feature['checks'])
          and feature['source_sha256'] == build['source_sha256'] and not feature['artifact_executed'] and not feature['real_device']
          and feature['bundle_sha256'] == build['artifacts']['bundle']['sha256']
          and feature['apk_sha256'] == build['artifacts']['apk']['sha256']
          and feature['verification_script_sha256'] == sha(APP / 'scripts/verify_multipart_cleanup_android_content.py'))
    check('JVM evidence preserves actual task/cache status and exact XML bytes',
          jvm['passed'] and jvm['tests'] == 26 and jvm['freezer_sha256'] == sha(APP / 'scripts/freeze_multipart_cleanup_android.py')
          and not jvm['device_executed'] and jvm['new_test_execution'] == (not jvm['gradle_cached'])
          and sha(E / 'native-build.log') == jvm['native_log_sha256']
          and all(sha(E / jvm['xml_artifact_directory'] / name) == value for name, value in jvm['xml_sha256'].items()))
    linux = read(E / 'linux-build.json')
    check('Current backend cross-compiles for Linux without execution or deployment',
          linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == source
          and sha(APP / '.local/multipart-cleanup-platform-linux-amd64') == linux['artifact']['sha256']
          and not linux['artifact_executed'] and not linux['deployed'])
    interaction = module('multipart_cleanup_sources').digest()
    screenshots = []
    browsers = []
    for client in ('web', 'mobile'):
        folder = E.parent / ('multipart-cleanup-' + client)
        result, manifest, shots = [read(folder / name) for name in ('result.json', 'source-manifest.json', 'screenshots.json')]
        browsers.append(result['passed'] and result['source_unchanged'] and result['source_sha256'] == manifest['sha256'] == digest(manifest['files'])
                        and result['build_source_sha256'] == result['build_source_after_sha256'] == interaction and result['build_source_unchanged']
                        and result['owned_browser_closed'] and result['browser_contexts_closed'] == 1 and result['owned_review_server_stopped']
                        and result['owned_build_process_stopped'] and result['owned_browser_runner_stopped'] and result['isolated_database_cleanup_passed']
                        and result['native_fixture_exit_code'] == 0 and len(result['checks']) == 4 and not result['console_errors'] and not result['server_errors']
                        and shots['credentials_excluded'] and shots['synthetic_data'])
        screenshots.extend(shots['screenshots'])
    check('Web and Taro H5 execute real API selection, observation and receipt flows and close owned resources', all(browsers))
    media = read(E / 'media.json')
    check('Two actual browser recordings bind exact published MP4/GIF bytes and capture manifests',
          media['passed'] and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_multipart_cleanup_media.py')
          and len(media['recordings']) == 2 and all(sha(ROOT / v['capture_manifest']) == v['capture_manifest_sha256']
          and v['build_source_sha256'] == interaction and v['capture_frame_count'] >= 5
          and all(sha(ROOT / a['file']) == a['sha256'] and (ROOT / a['file']).stat().st_size == a['bytes'] for a in v['artifacts']) for v in media['recordings']))
    visual = read(E / 'visual-review.json')
    check('Six inspected screenshots bind exact recorded synthetic business pixels',
          len(screenshots) == 6 and visual['passed'] and visual['synthetic_data'] and visual['screenshots'] == screenshots
          and all(sha(ROOT / v['file']) == v['sha256'] for v in screenshots))
    reused = read(E / 'reused-evidence.json')
    blobs = subprocess.check_output(['git', 'hash-object', '--', *[v['file'] for v in reused['media']]], cwd=ROOT).decode().splitlines()
    check('All189 earlier media and retained dependency/JVM records preserve exact published bytes',
          reused['passed'] and reused['published_commit'] == PUBLISHED and len(reused['media']) == 189
          and blobs == [v['git_blob'] for v in reused['media']]
          and all(sha(ROOT / v['file']) == v['sha256'] for v in reused['media'] + reused['records']))
    audit = read(E.parent / 'dependency-remediation/dependency-audit.json')
    findings = read(E.parent / 'dependency-remediation/remaining-findings.json')
    check('Unchanged dependency lock retains the open70 finding register without a new audit claim',
          audit['package_lock_sha256'] == sha(APP / 'package-lock.json')
          and audit['vulnerabilities'] == findings['counts'] and findings['counts']['total'] == 70
          and len(findings['items']) == 70 and all(v['status'] == 'open' for v in findings['items']))
    development, upgrade = [read(E / name) for name in ('development-project-resources.json', 'development-update.json')]
    check('Owned native schema82 and services are current with no test databases or external activation',
          development['passed'] and development['migration_count'] == 82 and development['backend_source_sha256'] == source
          and upgrade['passed'] and upgrade['backend_source_sha256'] == source and upgrade['legacy_and_usage_rows_preserved']
          and development['database']['disposable_test_databases'] == 0 and development['integrations_disabled'])
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    skills = [read(p) for p in sorted((APP / 'skills').glob('*/manifest.json'))]
    check('All23 work packages and eight draft Skills stay open; five exclusions are not passes',
          sum(v['status'] == 'active' for v in scope['items']) == 23 and sum(v['status'] == 'excluded' for v in scope['items']) == 5
          and next(v for v in scope['items'] if v['id'] == 'GAP-05')['status'] == 'active'
          and len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    with socket.socket() as probe:
        probe.settimeout(1)
        check('Owned review port5189 is released and private browser fixture removed',
              probe.connect_ex(('127.0.0.1', 5189)) != 0 and not (APP / '.local/business-browser-fixture.json').exists())
    report = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
              'verification_script_sha256': sha(Path(__file__)), 'backend_source_sha256': source, 'mobile_source_sha256': mobile_source,
              'interaction_kind': 'controlled_abandoned_multipart_cleanup', 'interaction_source_sha256': interaction,
              'full_goal_complete': False, 'production_ready': False}
    (E / 'final-source-check.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [v['name'] for v in checks if not v['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
