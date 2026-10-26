"""Bind the project overview to current source, real checks and owned cleanup."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/project-overview'


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

    backend = module('overview_backend_manifest', 'verify_backend_batches.py').digest()
    result = read(EVIDENCE / 'result.json')
    check('All 142 discovered backend tests and 872 results pass on current source without skips',
          result['passed'] and result['source_sha256'] == backend and result['source_unchanged_during_tests']
          and result['all_discovered_tests_executed'] and result['discovered_top_level_tests'] == 142
          and len(result['checks']) == 872 and all(item['Action'] == 'pass' for item in result['checks']))
    progress = read(EVIDENCE / 'batch-progress.json')
    check('All eight native backend batches bind the same unchanged source',
          progress['source_sha256'] == backend and len(progress['batches']) == 8
          and all(b['status'] == 'passed' and b['source_unchanged_during_tests'] and b['exit_code'] == 0 for b in progress['batches']))
    check('New overview and real controlled-budget lifecycle tests ran in the full regression',
          any(c['Test'] == 'TestProjectOverview' for c in result['checks'])
          and any(c['Test'] == 'TestProjectChangesAndBudgets' for c in result['checks']))
    linux = read(EVIDENCE / 'linux-build.json')
    artifact = APP / '.local/project-overview-platform-linux-amd64'
    check('Current Linux artifact is source-bound and has never been executed or deployed',
          linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend
          and artifact.is_file() and hashlib.sha256(artifact.read_bytes()).hexdigest() == linux['artifact_sha256']
          and not linux['artifact_executed'] and not linux['deployed'])
    web = read(EVIDENCE / 'web-build.json')
    check('Current Web passes 75 tests, type checking and the production build',
          web['passed'] and web['source_sha256'] == digest(web['sources']) and web['source_unchanged_during_run']
          and web['tests']['numPassedTests'] == 75 and web['tests']['numFailedTests'] == web['tests']['numPendingTests'] == 0)
    folder = EVIDENCE.parent / 'project-overview-web'
    browser, manifest = read(folder / 'result.json'), read(folder / 'source-manifest.json')
    check('Four actual Web/API overview, navigation, permission and read-only chains match current source',
          browser['passed'] and browser['source_sha256'] == digest(manifest['files']) and browser['source_unchanged_during_tests']
          and browser['real_web_api'] and browser['isolated_native_database'] and len(browser['checks']) == 4
          and all(c['passed'] for c in browser['checks']) and not browser['failure'] and not browser['errors'] and not browser['server_errors'])
    check('Both full and narrow reader overviews pass all six 320/390/1440 layouts',
          len(browser['layouts']) == 6 and all(c['passed'] for c in browser['layouts'])
          and {c['width'] for c in browser['layouts']} == {320, 390, 1440})
    check('Owned browser, contexts, tabs, Vite and isolated database were released',
          browser['owned_browser_closed'] and browser['all_owned_browser_tabs_closed'] and browser['owned_vite_stopped']
          and browser['native_fixture_exit_code'] == 0 and browser['isolated_database_cleanup_passed'] and not browser['cleanup_errors'])
    review = read(folder / 'visual-review.json')
    check('Current light desktop and dark narrow screenshots were visually reviewed',
          review['passed'] and review['browser_source_sha256'] == browser['source_sha256']
          and all((folder / item['path']).is_file() and hashlib.sha256((folder / item['path']).read_bytes()).hexdigest() == item['sha256'] for item in review['images']))
    mobile = read(EVIDENCE / 'mobile-build.json')
    check('Updated shared types pass 79 mobile tests, typecheck and both mini-program builds',
          mobile['passed'] and mobile['source_sha256'] == module('overview_mobile_manifest', 'verify_mobile.py').digest()[0]
          and mobile['source_unchanged_during_run'] and mobile['tests']['pass'] == 79
          and mobile['tests']['fail'] == mobile['tests']['skipped'] == mobile['tests']['cancelled'] == 0
          and {c['script'] for c in mobile['commands']} == {'test', 'typecheck', 'build:weapp', 'build:tt'}
          and all(c['exit_code'] == 0 and not c['timed_out'] for c in mobile['commands']))
    runtime = read(EVIDENCE.parent / 'android-native/runtime-build.json')
    check('Unchanged controlled runtime retains its source-bound 34-test evidence',
          runtime['status'] == 'passed' and runtime['source_sha256'] == runtime['source_after_sha256']
          == module('overview_runtime_manifest', 'verify_agent_runtime.py').digest()[0] and runtime['tests']['pass'] == 34)
    apk_build = read(EVIDENCE.parent / 'android-upload/build.json')
    apk = APP / apk_build['artifacts']['apk']['path']
    check('Increment66 frozen Android upload APK remains intact; no new device/build claim',
          apk_build['passed'] and apk.is_file() and hashlib.sha256(apk.read_bytes()).hexdigest() == apk_build['artifacts']['apk']['sha256']
          and not apk_build['device_executed'] and not apk_build['production_release'])
    dev, base = read(EVIDENCE / 'development-project-templates.json'), read(EVIDENCE / 'development-runtime.json')
    check('All 64 owned migration hashes match current source; no test database remains',
          dev['passed'] and base['passed'] and base['database']['migration_count'] == 64
          and base['database']['migration_manifest_matches_source'] and dev['database']['disposable_test_databases'] == 0)
    route = read(EVIDENCE / 'development-overview.json')
    check('Owned daily API loads the overview route and requires authentication',
          route['passed'] and route['anonymous_status'] == 401 and route['read_only_probe'] and not route['real_business_data_seeded'])
    update = read(EVIDENCE / 'development-update.json')
    check('Only the verified owned development API was updated to the current source',
          update['passed'] and update['source_sha256'] == backend and update['api_ready']
          and not update['postgres_restarted'] and not update['notification_worker_restarted'] and not update['vite_restarted']
          and not update['external_integration_started'] and not update['production_deployment'])
    check('Cloud, model and Telegram integrations remain disabled',
          dev['integrations_disabled'] and dev['database']['active_runtime_profiles'] == dev['database']['enabled_telegram_channels'] == 0)
    owner = module('overview_process_owner', 'verify_notifications_sources.py').owned_process
    check('Current API and original notification worker creation times match their owner records',
          owner('project-templates-api-owner.json', 'api') and owner('project-templates-worker-owner.json', 'worker-notification'))
    documented = APP.parent / 'docs/development/openapi.json'
    contract = read(documented)
    check('Documented/embedded OpenAPI agree on the new route and exact contract inventory',
          documented.read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes()
          and '/projects/{id}/overview' in contract['paths'] and len(contract['paths']) == 270
          and len(contract['components']['schemas']) == 589
          and 'ProjectOverview:' in (APP / 'packages/contracts/src/api.d.ts').read_text(encoding='utf-8'))
    with socket.socket() as probe:
        probe.settimeout(1)
        check('Private browser fixture removed and temporary port5189 released',
              probe.connect_ex(('127.0.0.1', 5189)) != 0 and not (APP / '.local/business-browser-fixture.json').exists())
    for address in ('http://127.0.0.1:18080/health/ready', 'http://localhost:5178/health/ready', 'http://localhost:5178/'):
        try:
            with urlopen(address, timeout=5) as response:
                ready = response.status == 200
        except OSError:
            ready = False
        check('Owned daily development is ready: ' + address, ready)
    audit = read(EVIDENCE.parent / 'android-native/dependency-audit.json')
    check('Dependency lock is unchanged; the unresolved production security gate remains open',
          hashlib.sha256((APP / 'package-lock.json').read_bytes()).hexdigest() == audit['package_lock_sha256']
          and not audit['production_security_gate_passed'])
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
              'backend_source_sha256': backend, 'checks': checks, 'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'full_goal_complete': False, 'production_deployed': False, 'real_cloud_verified': False, 'real_model_verified': False, 'real_device_verified': False,
              'reused_evidence': {'runtime': 'increment65 unchanged', 'android': 'increment66 frozen APK'},
              'scope': 'Read-authorized Web project aggregation and exact financial relationships; cross-platform parity, complete company journeys and the production first release remain open.'}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
