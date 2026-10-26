"""Bind mobile overview/business reading to actual builds, reads and cleanup."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/mobile-project-overview'


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

    previous = EVIDENCE.parent / 'project-overview'
    backend = module('mobile_overview_backend', 'verify_backend_batches.py').digest()
    regression = read(previous / 'result.json')
    check('Unchanged backend retains actual increment69 evidence: 142 tests and 872 results without skips',
          regression['passed'] and regression['source_sha256'] == backend and regression['source_unchanged_during_tests']
          and regression['discovered_top_level_tests'] == 142 and regression['all_discovered_tests_executed']
          and len(regression['checks']) == 872 and all(c['Action'] == 'pass' for c in regression['checks']))
    web = read(previous / 'web-build.json')
    check('Unchanged Web retains its source-bound 75-test and production-build evidence',
          web['passed'] and web['source_sha256'] == digest(web['sources']) and web['source_unchanged_during_run']
          and web['tests']['numPassedTests'] == 75 and web['tests']['numFailedTests'] == web['tests']['numPendingTests'] == 0)
    runtime = read(EVIDENCE.parent / 'android-native/runtime-build.json')
    check('Unchanged controlled secretary runtime retains its source-bound 34-test evidence',
          runtime['status'] == 'passed' and runtime['source_sha256'] == runtime['source_after_sha256']
          == module('mobile_overview_runtime', 'verify_agent_runtime.py').digest()[0] and runtime['tests']['pass'] == 34)
    linux = read(previous / 'linux-build.json')
    artifact = APP / '.local/project-overview-platform-linux-amd64'
    check('Unchanged Linux backend artifact remains intact and has never been run or deployed',
          linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend
          and artifact.is_file() and hashlib.sha256(artifact.read_bytes()).hexdigest() == linux['artifact_sha256']
          and not linux['artifact_executed'] and not linux['deployed'])
    mobile = read(EVIDENCE / 'mobile-build.json')
    check('Current mobile passes 89 tests, typecheck and both mini-program builds on one fixed source',
          mobile['passed'] and mobile['source_sha256'] == module('mobile_overview_mobile', 'verify_mobile.py').digest()[0]
          and mobile['source_unchanged_during_run'] and mobile['tests']['pass'] == mobile['tests']['tests'] == 89
          and mobile['tests']['fail'] == mobile['tests']['skipped'] == mobile['tests']['cancelled'] == 0
          and len(mobile['commands']) == 4 and all(c['exit_code'] == 0 for c in mobile['commands']))
    for target in ('weapp', 'tt'):
        directory = APP / 'mobile/dist' / target
        config = read(directory / 'app.json')
        check('The ' + target + ' build actually contains all sixteen registered pages including finance',
              len(config['pages']) == 16 and 'pages/finance/index' in config['pages']
              and all((directory / (page + '.js')).is_file() for page in config['pages']))
    folder = EVIDENCE.parent / 'mobile-project-overview-browser'
    browser, manifest = read(folder / 'result.json'), read(folder / 'source-manifest.json')
    check('Six actual mobile/API business, pagination, transport-failure and authorization chains match current source',
          browser['passed'] and browser['source_sha256'] == digest(manifest['files']) == module('mobile_overview_browser', 'mobile_project_overview_sources.py').digest()
          and browser['source_unchanged_during_tests'] and browser['build_and_browser_sources_match'] and browser['source_unchanged_during_fixture_and_build']
          and browser['real_taro_h5_api'] and browser['isolated_native_database'] and len(browser['checks']) == 6
          and all(c['passed'] for c in browser['checks']) and not browser['failure'] and not browser['errors'] and not browser['server_errors'])
    check('Overview, actual cost and narrow viewer pass eighteen light/dark 320/390/768 layout and hit-area checks',
          len(browser['layouts']) == 18 and all(c['passed'] for c in browser['layouts'])
          and {c['width'] for c in browser['layouts']} == {320, 390, 768} and {c['theme'] for c in browser['layouts']} == {'light', 'dark'})
    check('All owned browser tabs, contexts, review server and disposable native database were released',
          browser['owned_browser_closed'] and browser['all_owned_browser_tabs_closed'] and browser['owned_review_server_stopped']
          and browser['native_fixture_exit_code'] == 0 and browser['isolated_database_cleanup_passed'] and not browser['cleanup_errors'])
    visual = read(folder / 'visual-review.json')
    check('Current overview and financial screenshots were visually reviewed with exact source and image hashes',
          visual['passed'] and visual['browser_source_sha256'] == browser['source_sha256']
          and all((folder / item['path']).is_file() and hashlib.sha256((folder / item['path']).read_bytes()).hexdigest() == item['sha256'] for item in visual['images']))
    android = read(EVIDENCE / 'build.json')
    apk = APP / android['artifacts']['apk']['path']
    check('Actual new Android APK and offline bundle bind current source without virtualization or device claims',
          android['passed'] and android['source_sha256'] == digest(android['sources']) and android['source_unchanged_during_build']
          and all(p['passed'] for p in android['phases']) and apk.is_file() and hashlib.sha256(apk.read_bytes()).hexdigest() == android['artifacts']['apk']['sha256']
          and not android['virtualization_used'] and not android['emulator_started'] and not android['device_executed'] and not android['production_release'])
    inspection = read(EVIDENCE / 'apk-inspection.json')
    check('APK signature, sixteen pages, intended native adapters, upload bridge and all seventeen package checks pass',
          inspection['passed'] and inspection['apk_sha256'] == android['artifacts']['apk']['sha256']
          and len(inspection['checks']) == 17 and all(c['passed'] for c in inspection['checks'])
          and not inspection['device_executed'] and not inspection['emulator_started'])
    java = read(EVIDENCE.parent / 'android-upload/jvm-source-proof.json')
    check('Unchanged pure Java boundaries retain exact 25-case JVM evidence; current Android build passes the same cases',
          java['passed'] and java['tests'] == 25 and java['source_sha256'] == digest(java['sources'])
          and len(java['xml_sha256']) == 4 and all(
              hashlib.sha256((APP / 'mobile/native/jvm-tests/build/test-results/test' / name).read_bytes()).hexdigest() == value
              for name, value in java['xml_sha256'].items())
          and sum(int(s['tests']) for s in inspection['tests']) == 25
          and all(int(s[k]) == 0 for s in inspection['tests'] for k in ('failures', 'errors', 'skipped')))
    old = read(EVIDENCE.parent / 'android-upload/build.json')
    old_apk = APP / old['artifacts']['apk']['path']
    check('Increment66 frozen Android upload APK is preserved independently of the new APK',
          old_apk.is_file() and hashlib.sha256(old_apk.read_bytes()).hexdigest() == old['artifacts']['apk']['sha256'] and old_apk != apk)
    dev, base = read(EVIDENCE / 'development-project-templates.json'), read(EVIDENCE / 'development-runtime.json')
    check('Owned development still has all sixty-four matching migrations and no disposable test database',
          dev['passed'] and base['passed'] and base['database']['migration_count'] == 64
          and base['database']['migration_manifest_matches_source'] and dev['database']['disposable_test_databases'] == 0)
    check('Cloud, model and Telegram integrations remain disabled',
          dev['integrations_disabled'] and dev['database']['active_runtime_profiles'] == dev['database']['enabled_telegram_channels'] == 0)
    route = read(EVIDENCE / 'development-overview.json')
    check('Current development overview route is loaded and requires authentication',
          route['passed'] and route['anonymous_status'] == 401 and route['read_only_probe'] and not route['real_business_data_seeded'])
    owner = module('mobile_overview_owner', 'verify_notifications_sources.py').owned_process
    check('Original daily API and notification worker retain their exact owned process identities',
          owner('project-templates-api-owner.json', 'api') and owner('project-templates-worker-owner.json', 'worker-notification'))
    with socket.socket() as probe:
        probe.settimeout(1)
        check('Temporary browser port5189 and its private fixture file were released',
              probe.connect_ex(('127.0.0.1', 5189)) != 0 and not (APP / '.local/business-browser-fixture.json').exists())
    for address in ('http://127.0.0.1:18080/health/ready', 'http://localhost:5178/health/ready', 'http://localhost:5178/'):
        try:
            with urlopen(address, timeout=5) as response:
                ready = response.status == 200
        except OSError:
            ready = False
        check('Daily development is still ready: ' + address, ready)
    audit = read(EVIDENCE.parent / 'android-native/dependency-audit.json')
    check('No dependency lock changed; unresolved production security issues are still explicitly open',
          hashlib.sha256((APP / 'package-lock.json').read_bytes()).hexdigest() == audit['package_lock_sha256'] and not audit['production_security_gate_passed'])
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
        'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'backend_source_sha256': backend,
        'reused_evidence': {'backend': 'increment69 unchanged, not rerun', 'web': 'increment69 unchanged, not rerun', 'runtime': 'increment65 unchanged, not rerun', 'java': 'increment66 final pure-Java source proof and exact JVM XML; current Android build also passes the same 25 cases'},
        'full_goal_complete': False, 'production_deployed': False, 'real_cloud_verified': False, 'real_model_verified': False, 'real_device_verified': False,
        'scope': 'Mobile project aggregation, original financial/business reads, actual two mini-program builds and new Android packaging; complete company journeys, native/device/platform and production acceptance remain open.'}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
