"""Bind the final amortization delivery to current source bytes and actual checks."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/cost-amortization'
PUBLISHED = '815e89317785a8f97bc474b925e67b5f2bfa5895'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def module(name):
    spec = importlib.util.spec_from_file_location('amortization_proof_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def digest(names, previous=None):
    h = hashlib.sha256()
    for name in names:
        raw = previous[1] if previous and name == previous[0] else (APP / name).read_bytes()
        h.update(name.encode() + b'\0' + raw + b'\0')
    return h.hexdigest()


def main():
    checks = []
    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})
    source = module('verify_backend_batches').digest()
    backend, state, raw = [read(E.parent / 'cost-amortization-backend' / name) for name in ('result.json', 'batch-progress.json', 'raw-log-archive.json')]
    expected = {tuple(v) for v in state['expected_tests']}
    observed = {(v['Package'], v['Test']) for v in backend['checks'] if '/' not in v['Test']}
    check('Every discovered backend test passes against the complete current Go, SQL and embedded contract',
          backend['passed'] and backend['source_sha256'] == source and backend['source_unchanged_during_tests']
          and backend['all_discovered_tests_executed'] and expected == observed and all(v['Action'] == 'pass' for v in backend['checks']))
    check('All completed batch logs retain exact run-specific private bytes',
          raw['passed'] and raw['source_sha256'] == source and raw['records'] == len(backend['checks'])
          and len(raw['logs']) == len(state['batches']) and raw['freezer_sha256'] == sha(APP / 'scripts/freeze_amortization_backend.py')
          and all(sha(APP / v['raw_log']) == v['sha256'] and (APP / v['raw_log']).stat().st_size == v['bytes'] for v in raw['logs']))
    tests = {v['Test'] for v in backend['checks']}
    required = {'TestVersionedCostAmortization/' + v for v in (
        'no_implicit_plan_or_implicit_management_authority', 'schema_queries_current_permissions_and_audit_are_enforced',
        'preview_is_audited_but_does_not_publish_or_change_cost', 'independent_scope_report_retains_source_plan_and_exact_company_share',
        'export_pins_plan_and_cost_references_without_changing_legacy_csv', 'immutable_rows_cannot_be_edited_or_appended_in_later_transaction',
        'next_version_concurrency_history_and_stopping_are_explicit', 'reversal_retains_prior_month_and_negates_on_actual_date_only')}
    required |= {'TestAmortizationConservesEveryProjectAndCompanyMinorUnit', 'TestAmortizationCalendarUsesExactHalfOpenServiceDays',
                 'TestAmortizationCumulativeRoundingDoesNotCreateNegativeTails', 'TestAmortizationRefusesUnknownRulesInvalidPeriodsAndSourcePrecision'}
    check('Native arithmetic, authority, atomicity, history, export, reversal and FX scenarios actually execute', required <= tests)
    clients = read(E / 'client-checks.json')
    check('All target check/build results bind current complete inputs and exact command logs',
          clients['passed'] and clients['source_unchanged'] and clients['source_sha256'] == digest(clients['source_files'])
          and len(clients['checks']) == 8 and all(v['passed'] and v['exit_code'] == 0
          and sha(APP / '.local' / Path(v['raw_log']).name) == v['raw_log_sha256'] for v in clients['checks']))
    attempt = read(E / 'attempts/06-bigint-build-compatibility/status.json')
    check('The compiled-target failure remains archived and every final client command is a fresh post-fix execution',
          not attempt['passed'] and attempt['web_before_fix_passed'] and not attempt['mobile_before_fix_passed']
          and attempt['owned_android_build_tree_stopped'] and not attempt['android_artifact_produced']
          and 'web_refresh' not in clients
          and all(not v.get('same_increment_execution_retained', False) for v in clients['checks']))
    web, mobile, runtime = [read(E / name) for name in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    mobile_source = module('verify_mobile').digest()[0]
    check('Web, shared mobile, mini-program and runtime summaries bind actual fresh commands and current sources',
          web['passed'] and web['source_sha256'] == digest(web['sources']) and web['tests']['numPassedTests'] == 172
          and mobile['passed'] and mobile['source_sha256'] == mobile_source and mobile['tests']['pass'] == 273
          and mobile['tests']['fail'] == mobile['tests']['skipped'] == mobile['tests']['cancelled'] == 0
          and all(v['page_count'] == 20 for v in mobile['targets'].values())
          and runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime').digest()[0] and runtime['tests']['pass'] == 55)
    build, inspection, feature, jvm = [read(E / name) for name in ('build.json', 'apk-inspection.json', 'android-feature-content.json', 'android-test-scope.json')]
    check('Fresh native APK and JS artifact bytes match their complete build inputs and static inspections',
          build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == digest(build['sources'])
          and inspection['passed'] and feature['passed'] and not feature['artifact_executed'] and not feature['real_device']
          and feature['verification_script_sha256'] == sha(APP / 'scripts/verify_amortization_android_content.py')
          and all(sha(APP / v['path']) == v['sha256'] for v in build['artifacts'].values()))
    check('JVM records preserve actual task cache status and original XML bytes',
          jvm['passed'] and jvm['tests'] == 26 and jvm['new_test_execution'] == (not jvm['gradle_cached']) and not jvm['device_executed']
          and jvm['freezer_sha256'] == sha(APP / 'scripts/freeze_amortization_android.py')
          and sha(E / 'native-build.log') == jvm['native_log_sha256']
          and all(sha(E / jvm['xml_artifact_directory'] / name) == value for name, value in jvm['xml_sha256'].items()))
    linux = read(E / 'linux-build.json')
    check('Both Linux targets cross-compile from current sources without execution or deployment',
          linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == source
          and len(linux['artifacts']) == 2 and not linux['artifact_executed'] and not linux['kernel_boundary_executed'] and not linux['deployed']
          and all(sha(APP / v['file']) == v['sha256'] for v in linux['artifacts']))
    interaction = module('amortization_sources').digest()
    screenshots, browser_checks = [], []
    for client in ('web', 'mobile'):
        folder = E.parent / ('cost-amortization-' + client)
        result, manifest, capture = [read(folder / name) for name in ('result.json', 'source-manifest.json', 'capture.json')]
        browser_checks.append(result['passed'] and result['source_unchanged'] and result['source_sha256'] == manifest['sha256'] == digest(manifest['files'])
              and result['build_source_sha256'] == result['build_source_after_sha256'] == interaction and result['build_source_unchanged']
              and result['owned_browser_closed'] and result['browser_contexts_closed'] == 1 and result['owned_review_server_stopped']
              and result['owned_build_process_stopped'] and result['owned_browser_runner_stopped'] and result['isolated_database_cleanup_passed']
              and result['native_fixture_exit_code'] == 0 and len(result['checks']) == 6 and not result['console_errors'] and not result['server_errors']
              and capture['synthetic_data'] and capture['real_browser_pixels'])
        screenshots.extend(capture['screenshots'])
    check('Real Web and Taro H5 preview, MFA publication, history and report flows close all owned resources', all(browser_checks))
    media = read(E / 'media.json')
    check('Two real authenticated silent recordings bind exact MP4/GIF bytes and pixel manifests',
          media['passed'] and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_amortization_media.py')
          and len(media['recordings']) == 2 and all(sha(ROOT / v['capture_manifest']) == v['capture_manifest_sha256']
          and v['build_source_sha256'] == interaction and v['capture_frame_count'] >= 5
          and all(sha(ROOT / a['file']) == a['sha256'] and (ROOT / a['file']).stat().st_size == a['bytes'] for a in v['artifacts']) for v in media['recordings']))
    visual = read(E / 'visual-review.json')
    check('Eight visually inspected screenshots bind exact synthetic business pixels',
          len(screenshots) == 8 and visual['passed'] and visual['synthetic_data'] and visual['screenshots'] == screenshots
          and all(sha(ROOT / v['file']) == v['sha256'] for v in screenshots))
    reused = read(E / 'reused-evidence.json')
    blobs = subprocess.check_output(['git', 'hash-object', '--', *[v['file'] for v in reused['media']]], cwd=ROOT).decode().splitlines()
    check('All199 prior media and retained dependency records preserve exact published bytes',
          reused['passed'] and reused['published_commit'] == PUBLISHED and len(reused['media']) == 199
          and blobs == [v['git_blob'] for v in reused['media']] and all(sha(ROOT / v['file']) == v['sha256'] for v in reused['media'] + reused['records']))
    audit, findings = [read(E.parent / 'dependency-remediation' / name) for name in ('dependency-audit.json', 'remaining-findings.json')]
    check('Unchanged lock retains the full open70 dependency register without claiming a new audit',
          audit['package_lock_sha256'] == sha(APP / 'package-lock.json') and audit['vulnerabilities'] == findings['counts']
          and findings['counts']['total'] == len(findings['items']) == 70 and all(v['status'] == 'open' for v in findings['items']))
    development, upgrade = [read(E / name) for name in ('development-project-resources.json', 'development-update.json')]
    check('Only attested native schema83 services are refreshed with zero disposable databases and no external activation',
          development['passed'] and development['migration_count'] == 83 and development['backend_source_sha256'] == source
          and upgrade['passed'] and upgrade['backend_source_sha256'] == source and upgrade['legacy_and_usage_rows_preserved']
          and development['database']['disposable_test_databases'] == 0 and development['integrations_disabled'])
    cleanup = read(E / 'attempts/07-resource-cleanup/cleanup-result.json')
    inspection = read(E / 'attempts/07-resource-cleanup/ownership-inspection.json')
    failed = read(E / 'attempts/05-fixture-cleanup-timeout/status.json')
    check('The archived failed cleanup is retained and its exact idle fixture is removed without force or business-database changes',
          not failed['passed'] and cleanup['passed'] and cleanup['failed_test_result_unchanged']
          and inspection['original_raw_sha256'] == failed['raw_log_sha256']
          and cleanup['database'] == inspection['databases'][0]['name'] and cleanup['oid'] == inspection['databases'][0]['oid']
          and cleanup['ownership_creation_delta_seconds'] < 5 and cleanup['connections_before'] == 0
          and cleanup['disposable_databases_after'] == 0 and not cleanup['force_termination_used']
          and not cleanup['development_business_database_modified'])
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    skills = [read(p) for p in sorted((APP / 'skills').glob('*/manifest.json'))]
    check('All23 work packages and eight draft Skills remain open; five exclusions are not passes',
          sum(v['status'] == 'active' for v in scope['items']) == 23 and sum(v['status'] == 'excluded' for v in scope['items']) == 5
          and next(v for v in scope['items'] if v['id'] == 'GAP-03')['status'] == 'active' and len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    with socket.socket() as probe:
        probe.settimeout(1)
        check('Temporary port5189 is released and private browser fixture removed', probe.connect_ex(('127.0.0.1', 5189)) != 0 and not (APP / '.local/business-browser-fixture.json').exists())
    report = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
              'verification_script_sha256': sha(Path(__file__)), 'backend_source_sha256': source, 'mobile_source_sha256': mobile_source,
              'interaction_kind': 'versioned_cost_amortization', 'interaction_source_sha256': interaction,
              'full_goal_complete': False, 'production_ready': False}
    (E / 'final-source-check.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [v['name'] for v in checks if not v['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
