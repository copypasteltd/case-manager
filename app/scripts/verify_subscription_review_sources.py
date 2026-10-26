"""Bind the detailed finance tools to fixed sources, actual receipts, builds and owned cleanup."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/subscription-review'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / filename)
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

    backend = module('review_backend_source', 'verify_backend_batches.py').digest()
    regression = read(EVIDENCE / 'result.json')
    check('Every discovered backend test executes on the current fixed source without failure or skip',
          regression['passed'] and regression['source_sha256'] == backend and regression['source_unchanged_during_tests']
          and regression['discovered_top_level_tests'] == 143 and regression['all_discovered_tests_executed']
          and regression['batch_count'] == 8 and all(c['Action'] == 'pass' for c in regression['checks']))
    detail = [c for c in regression['checks'] if c['Test'].startswith('TestAgentSubscriptionReview/')]
    check('All fourteen actual HTTP, association, paging, coverage, immutable receipt, rollback and withdrawal cases pass',
          len(detail) == 14 and all(c['Action'] == 'pass' for c in detail))
    runtime = read(EVIDENCE / 'runtime-build.json')
    check('Current SDK runtime, schemas and linked Skill instructions pass thirty-six controlled tests and build',
          runtime['status'] == 'passed' and runtime['source_sha256'] == runtime['source_after_sha256']
          == module('review_runtime_source', 'verify_agent_runtime.py').digest()[0]
          and runtime['tests']['pass'] == runtime['tests']['tests'] == 36
          and runtime['tests']['fail'] == runtime['tests']['skipped'] == runtime['tests']['cancelled'] == 0
          and len(runtime['commands']) == 2 and all(c['exit_code'] == 0 for c in runtime['commands']))
    manifests = [read(p) for p in sorted((APP / 'skills').glob('*/manifest.json'))]
    review = next(m for m in manifests if m['skill_id'] == 'subscription-review')
    check('All eight Skills stay draft and the subscription review package permits only its six read tools',
          len(manifests) == 8 and all(m['state'] == 'draft' for m in manifests) and review['version'] == '1.2.0'
          and set(review['allowed_tools']) == {'read_resource', 'inspect_operation', 'read_finance_summary',
                                              'list_subscriptions', 'read_subscription_review', 'read_finance_record'})
    for target, count in (('web', 4), ('mobile', 3)):
        folder = EVIDENCE.parent / ('subscription-review-' + target)
        result, source = read(folder / 'result.json'), read(folder / 'source-manifest.json')
        check('Current ' + target + ' browser verifies all detailed sources against actual API facts and exact source digests',
              result['passed'] and result['source_sha256'] == result['source_after_sha256'] == digest(source['files'])
              and result['source_unchanged'] and len(result['checks']) == count and all(c['passed'] for c in result['checks'])
              and not result.get('failure') and not result['console_errors'] and not result['server_errors'])
        check(target + ' reads every detailed source chunk and closes its owned browser, server and native fixture',
              result['checks'][-1]['pages'] >= 3 and result['owned_browser_closed'] and result['browser_contexts_closed'] == 1
              and result['native_fixture_exit_code'] == 0 and result['isolated_database_cleanup_passed']
              and (result['owned_vite_stopped'] if target == 'web' else result['owned_review_server_stopped']))
        if target == 'mobile':
            check('Taro browser execution uses exactly the source frozen before the actual H5 build',
                  result['source_unchanged_during_fixture_and_build'] and result['build_and_browser_sources_match'])
        visual = read(folder / 'visual-review.json')
        check(target + ' screenshot is visually reviewed and bound to the exact successful source',
              visual['passed'] and visual['browser_source_sha256'] == result['source_sha256']
              and all(hashlib.sha256((folder / v['path']).read_bytes()).hexdigest() == v['sha256'] for v in visual['images']))
    web = read(EVIDENCE.parent / 'project-overview/web-build.json')
    check('Unchanged Web retains its source-bound seventy-five tests and production build',
          web['passed'] and web['source_sha256'] == digest(web['sources']) and web['source_unchanged_during_run']
          and web['tests']['numPassedTests'] == 75 and web['tests']['numFailedTests'] == web['tests']['numPendingTests'] == 0)
    mobile = read(EVIDENCE.parent / 'mobile-project-overview/mobile-build.json')
    check('Unchanged mobile retains eighty-nine tests, typecheck and both actual sixteen-page mini-program builds',
          mobile['passed'] and mobile['source_sha256'] == module('review_mobile_source', 'verify_mobile.py').digest()[0]
          and mobile['source_unchanged_during_run'] and mobile['tests']['pass'] == mobile['tests']['tests'] == 89
          and all(c['exit_code'] == 0 for c in mobile['commands']) and all(
              len(read(APP / 'mobile/dist' / target / 'app.json')['pages']) == 16 for target in ('weapp', 'tt')))
    android = read(EVIDENCE.parent / 'mobile-project-overview/build.json')
    apk = APP / android['artifacts']['apk']['path']
    inspection = read(EVIDENCE.parent / 'mobile-project-overview/apk-inspection.json')
    check('Unchanged increment70 Android APK and all seventeen package checks remain intact without a rebuild or device claim',
          android['passed'] and android['source_sha256'] == digest(android['sources']) and apk.is_file()
          and hashlib.sha256(apk.read_bytes()).hexdigest() == android['artifacts']['apk']['sha256']
          and inspection['passed'] and inspection['apk_sha256'] == android['artifacts']['apk']['sha256']
          and len(inspection['checks']) == 17 and all(c['passed'] for c in inspection['checks'])
          and not android['virtualization_used'] and not android['emulator_started'] and not android['device_executed'])
    java = read(EVIDENCE.parent / 'android-upload/jvm-source-proof.json')
    check('Pure Java boundaries retain the final increment66 source proof and four exact twenty-five-case JVM XML files',
          java['passed'] and java['tests'] == 25 and java['source_sha256'] == digest(java['sources'])
          and len(java['xml_sha256']) == 4 and all(hashlib.sha256(
              (APP / 'mobile/native/jvm-tests/build/test-results/test' / name).read_bytes()).hexdigest() == value
              for name, value in java['xml_sha256'].items()))
    linux = read(EVIDENCE / 'linux-build.json'); artifact = APP / '.local/subscription-review-platform-linux-amd64'
    check('Current backend cross-compiles for Linux without executing or deploying its artifact',
          linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend
          and artifact.is_file() and hashlib.sha256(artifact.read_bytes()).hexdigest() == linux['artifact_sha256']
          and not linux['artifact_executed'] and not linux['deployed'])
    dev, base = read(EVIDENCE / 'development-subscription-review.json'), read(EVIDENCE / 'development-runtime.json')
    update = read(EVIDENCE / 'development-update.json')
    check('Owned development runs all sixty-five exact migrations and the current reviewed backend',
          dev['passed'] and base['passed'] and base['database']['migration_count'] == 65
          and base['database']['migration_manifest_matches_source'] and update['passed']
          and update['source_sha256'] == backend and update['api_ready'] and not update['postgres_restarted']
          and not update['notification_worker_restarted'] and not update['vite_restarted'])
    check('Only the exact owned development API was replaced; worker identity and disabled integrations are preserved',
          all(dev['ownership'].values()) and dev['integrations_disabled'] and dev['all_eight_skills_draft']
          and dev['database']['active_profiles'] == dev['database']['enabled_telegram_channels'] == dev['database']['report_count'] == 0)
    with socket.socket() as probe:
        probe.settimeout(1)
        check('No disposable database, temporary listener or private browser fixture remains',
              dev['database']['disposable_test_databases'] == 0 and probe.connect_ex(('127.0.0.1', 5189)) != 0
              and not (APP / '.local/business-browser-fixture.json').exists())
    cleanup = read(EVIDENCE / 'fixture-cleanup.json')
    check('The exact fixture left by the cancelled DROP was verified and disposed after all tests settled',
          cleanup['passed'] and cleanup['state_before_drop']['connections'] == 0
          and cleanup['state_before_drop']['connection_limit'] == -2 and not cleanup['force_used']
          and not cleanup['daily_database_modified'] and not cleanup['postgres_restarted'])
    for address in ('http://127.0.0.1:18080/health/ready', 'http://localhost:5178/health/ready', 'http://localhost:5178/'):
        try:
            with urlopen(address, timeout=5) as response:
                ready = response.status == 200
        except OSError:
            ready = False
        check('Daily service remains ready: ' + address, ready)
    audit = read(EVIDENCE.parent / 'android-native/dependency-audit.json')
    check('Dependency lock is unchanged and unresolved production security findings remain open',
          hashlib.sha256((APP / 'package-lock.json').read_bytes()).hexdigest() == audit['package_lock_sha256']
          and not audit['production_security_gate_passed'])
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
              'backend_source_sha256': backend, 'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'reused_evidence': {'web': 'increment69 unchanged, not rerun', 'mobile': 'increment70 unchanged, not rerun',
                                  'android': 'increment70 unchanged APK, not rebuilt', 'java': 'increment66 final source and XML proof'},
              'full_goal_complete': False, 'production_deployed': False, 'real_cloud_verified': False,
              'real_model_verified': False, 'real_device_verified': False}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
