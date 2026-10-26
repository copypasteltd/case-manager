"""Own a native database fixture and a local Taro H5 review server."""
import functools
import hashlib
from datetime import datetime, timezone
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import threading
import time
from urllib.request import Request, urlopen
from urllib.parse import urlparse

APP = Path(__file__).resolve().parents[1]
LOCAL = APP / '.local'
FIXTURE = LOCAL / 'business-browser-fixture.json'
PROJECTS = '--projects' in sys.argv[1:]
CONTENT = '--content' in sys.argv[1:]
OA = '--oa' in sys.argv[1:]
STATUS = '--status' in sys.argv[1:]
SECRETARY = '--secretary' in sys.argv[1:]
PLANS = '--plans' in sys.argv[1:]
DISCOVERY = '--plan-discovery' in sys.argv[1:]
MUTATIONS = '--mutations' in sys.argv[1:]
FILE_TEXT = '--file-text' in sys.argv[1:]
FILE_UPDATES = '--file-updates' in sys.argv[1:]
NOTIFICATIONS = '--notifications' in sys.argv[1:]
ACCOUNT = '--account-security' in sys.argv[1:]
ORGANIZATION = '--organization' in sys.argv[1:]
FILE_CREATES = '--file-creates' in sys.argv[1:]
POST_WRITES = '--post-writes' in sys.argv[1:]
TELEGRAM_DELIVERY = '--telegram-delivery' in sys.argv[1:]
SUBSCRIPTION_REVIEW = '--subscription-review' in sys.argv[1:]
FINANCE_SUMMARY = '--finance-summary' in sys.argv[1:]
SERVICE_SUMMARY = '--service-summary' in sys.argv[1:]
TELEGRAM_DISPATCH = '--telegram-dispatch' in sys.argv[1:]
DELIVERABLE_CREATION = '--deliverable-creation' in sys.argv[1:]
DELIVERABLE_UPDATES = '--deliverable-updates' in sys.argv[1:]
MILESTONE_CREATION = '--milestone-creation' in sys.argv[1:]
PROJECT_STAGES = '--project-stages' in sys.argv[1:]
OA_REQUESTS = '--oa-requests' in sys.argv[1:]
FROZEN = DELIVERABLE_CREATION or DELIVERABLE_UPDATES or MILESTONE_CREATION or SUBSCRIPTION_REVIEW or TELEGRAM_DELIVERY or FINANCE_SUMMARY or SERVICE_SUMMARY or TELEGRAM_DISPATCH or CONTENT or OA or STATUS or SECRETARY or PLANS or DISCOVERY or MUTATIONS or FILE_TEXT or FILE_UPDATES or NOTIFICATIONS or ACCOUNT or ORGANIZATION or FILE_CREATES or OA_REQUESTS or POST_WRITES or PROJECT_STAGES
if len(sys.argv[1:]) > 1 or any(value not in ('--projects', '--content', '--oa', '--status', '--secretary', '--plans', '--plan-discovery', '--mutations', '--file-text', '--file-updates', '--notifications', '--account-security', '--organization', '--file-creates', '--oa-requests', '--post-writes', '--deliverable-creation', '--deliverable-updates', '--milestone-creation', '--project-stages', '--telegram-dispatch', '--service-summary', '--subscription-review', '--finance-summary', '--telegram-delivery') for value in sys.argv[1:]):
    raise SystemExit('Usage: python app/scripts/run_mobile_browser.py [--projects | --content | --oa | --status | --secretary | --plans | --plan-discovery | --mutations | --file-text | --file-updates | --notifications | --account-security | --organization | --file-creates | --oa-requests | --post-writes | --deliverable-creation | --deliverable-updates | --milestone-creation | --project-stages | --telegram-dispatch | --service-summary | --subscription-review | --finance-summary | --telegram-delivery]')
CHECKER = 'scripts/check_mobile_deliverable_creation.mjs' if DELIVERABLE_CREATION else 'scripts/check_mobile_deliverable_updates.mjs' if DELIVERABLE_UPDATES else 'scripts/check_mobile_milestone_creation.mjs' if MILESTONE_CREATION else 'scripts/check_mobile_telegram_delivery.mjs' if TELEGRAM_DELIVERY else 'scripts/check_mobile_subscription_review.mjs' if SUBSCRIPTION_REVIEW else 'scripts/check_mobile_finance_summary.mjs' if FINANCE_SUMMARY else 'scripts/check_mobile_service_summary.mjs' if SERVICE_SUMMARY else 'scripts/check_mobile_telegram_dispatch.mjs' if TELEGRAM_DISPATCH else 'scripts/check_mobile_project_stages.mjs' if PROJECT_STAGES else 'scripts/check_mobile_post_writes.mjs' if POST_WRITES else 'scripts/check_mobile_oa_requests.mjs' if OA_REQUESTS else 'scripts/check_mobile_file_creates.mjs' if FILE_CREATES else 'scripts/check_mobile_organization.mjs' if ORGANIZATION else 'scripts/check_mobile_account_security.mjs' if ACCOUNT else 'scripts/check_mobile_notifications.mjs' if NOTIFICATIONS else 'scripts/check_mobile_file_updates.mjs' if FILE_UPDATES else 'scripts/check_mobile_file_text.mjs' if FILE_TEXT else 'scripts/check_mobile_mutations.mjs' if MUTATIONS else 'scripts/check_mobile_plan_discovery.mjs' if DISCOVERY else 'scripts/check_mobile_plans.mjs' if PLANS else 'scripts/check_mobile_secretary.mjs' if SECRETARY else 'scripts/check_mobile_status.mjs' if STATUS else 'scripts/check_mobile_oa.mjs' if OA else 'scripts/check_mobile_content.mjs' if CONTENT else 'scripts/check_mobile.mjs'
REPORT = APP.parent / ('docs/quality/evidence/deliverable-creation-mobile/result.json' if DELIVERABLE_CREATION else 'docs/quality/evidence/deliverable-updates-mobile/result.json' if DELIVERABLE_UPDATES else 'docs/quality/evidence/milestone-creation-mobile/result.json' if MILESTONE_CREATION else 'docs/quality/evidence/telegram-delivery-mobile/result.json' if TELEGRAM_DELIVERY else 'docs/quality/evidence/subscription-review-mobile/result.json' if SUBSCRIPTION_REVIEW else 'docs/quality/evidence/finance-summary-mobile/result.json' if FINANCE_SUMMARY else 'docs/quality/evidence/service-summary-mobile/result.json' if SERVICE_SUMMARY else 'docs/quality/evidence/telegram-dispatch-mobile/result.json' if TELEGRAM_DISPATCH else 'docs/quality/evidence/secretary-project-stages-mobile/result.json' if PROJECT_STAGES else 'docs/quality/evidence/secretary-post-writes-mobile/result.json' if POST_WRITES else 'docs/quality/evidence/secretary-oa-requests-mobile/result.json' if OA_REQUESTS else 'docs/quality/evidence/secretary-file-creates-mobile/result.json' if FILE_CREATES else 'docs/quality/evidence/secretary-organization-mobile/result.json' if ORGANIZATION else 'docs/quality/evidence/account-security-mobile/result.json' if ACCOUNT else 'docs/quality/evidence/notifications-mobile/result.json' if NOTIFICATIONS else 'docs/quality/evidence/secretary-file-updates-mobile/result.json' if FILE_UPDATES else 'docs/quality/evidence/secretary-files-mobile/result.json' if FILE_TEXT else 'docs/quality/evidence/secretary-mutations-mobile/result.json' if MUTATIONS else 'docs/quality/evidence/secretary-discovery-mobile/result.json' if DISCOVERY else 'docs/quality/evidence/secretary-plans-mobile/result.json' if PLANS else 'docs/quality/evidence/mobile-secretary/result.json' if SECRETARY else 'docs/quality/evidence/mobile-status/result.json' if STATUS else 'docs/quality/evidence/mobile-oa/result.json' if OA else 'docs/quality/evidence/mobile-content/result.json' if CONTENT else 'docs/quality/evidence/mobile-projects/result.json' if PROJECTS else 'docs/quality/evidence/mobile-browser/result.json')
if os.environ.get('CP_BROWSER_EVIDENCE_GROUP'):
    group = os.environ['CP_BROWSER_EVIDENCE_GROUP']
    if not group.replace('-', '').isalnum() or not (SECRETARY or PLANS or DISCOVERY or MUTATIONS or FILE_TEXT or FILE_UPDATES or ORGANIZATION or FILE_CREATES or OA_REQUESTS or POST_WRITES or DELIVERABLE_CREATION or DELIVERABLE_UPDATES or MILESTONE_CREATION or PROJECT_STAGES or TELEGRAM_DISPATCH or SERVICE_SUMMARY or FINANCE_SUMMARY or SUBSCRIPTION_REVIEW or TELEGRAM_DELIVERY):
        raise SystemExit('Custom evidence group requires the secretary regression.')
    REPORT = APP.parent / 'docs/quality/evidence' / group / 'result.json'
if FIXTURE.exists():
    raise SystemExit('An owned fixture file already exists; inspect its owner before starting another run.')
node = shutil.which('node')
npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
if not node or not npm:
    raise SystemExit('Native Node/npm are required.')
LOCAL.mkdir(exist_ok=True)
flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
REPORT.parent.mkdir(parents=True, exist_ok=True)
REPORT.write_text(json.dumps({'started_at': datetime.now(timezone.utc).isoformat(), 'passed': False, 'phase': 'starting owned native fixture'}) + '\n', encoding='utf-8')
configuration = None
process = None
server = None
fixture_status = None
browser_status = None
success = False
build_before = None
build_after = None


def content_source_digest():
    if TELEGRAM_DELIVERY or SUBSCRIPTION_REVIEW or FINANCE_SUMMARY or SERVICE_SUMMARY or NOTIFICATIONS or ACCOUNT or ORGANIZATION or FILE_CREATES or OA_REQUESTS or POST_WRITES or DELIVERABLE_CREATION or DELIVERABLE_UPDATES or MILESTONE_CREATION or PROJECT_STAGES or TELEGRAM_DISPATCH:
        backend = [p for p in (APP / 'backend').rglob('*') if p.is_file() and (p.suffix in ('.go', '.sql') or p.name in ('go.mod', 'go.sum') or p.relative_to(APP).as_posix() == 'backend/internal/contract/openapi.json')]
        files = list(backend)
        for folder in ('mobile', 'packages/contracts', 'packages/design'):
            for directory, directories, names in os.walk(APP / folder):
                directories[:] = [name for name in directories if name not in {'node_modules', 'dist', '.taro', '.swc', 'android', 'native', '.gradle', 'build'}]
                files.extend(Path(directory) / name for name in names)
        files.extend(APP / name for name in ('package.json', 'package-lock.json', 'scripts/notification_browser_common.mjs', *(['scripts/subscription_review_browser_common.mjs'] if SUBSCRIPTION_REVIEW else []), CHECKER, 'scripts/run_mobile_browser.py', 'scripts/test_backend.py', 'scripts/build_openapi.py'))
        result = hashlib.sha256()
        for path in sorted(set(files), key=lambda p: p.relative_to(APP).as_posix()):
            result.update(path.relative_to(APP).as_posix().encode('utf-8') + b'\0' + path.read_bytes() + b'\0')
        return result.hexdigest()
    files = []
    for directory, directories, names in os.walk(APP / 'mobile'):
        directories[:] = [name for name in directories if name not in {'dist', 'node_modules', '.taro', '.swc', 'android', 'native', '.gradle', 'build'}]
        files.extend(Path(directory) / name for name in names)
    files.extend(p for p in (APP / 'backend').rglob('*') if p.is_file() and (p.suffix in ('.go', '.sql') or p.name in ('go.mod', 'go.sum') or p.relative_to(APP).as_posix() == 'backend/internal/contract/openapi.json'))
    files.extend(APP / name for name in ['package.json', 'package-lock.json', CHECKER, 'scripts/run_mobile_browser.py', *(['scripts/notification_browser_common.mjs', 'packages/contracts/package.json'] if NOTIFICATIONS else []), 'scripts/test_backend.py', 'scripts/sync_design.py', 'scripts/build_openapi.py', 'packages/contracts/src/api.d.ts', 'packages/contracts/src/notifications.ts', 'packages/design/src/tokens.json', *(['packages/contracts/src/mutation.ts'] if MUTATIONS or FILE_TEXT or FILE_UPDATES else []), *(['packages/contracts/src/file-update.ts', 'packages/contracts/package.json'] if FILE_UPDATES else [])])
    result = hashlib.sha256()
    for path in sorted(set(files), key=lambda p: p.relative_to(APP).as_posix()):
        result.update(path.relative_to(APP).as_posix().encode() + b'\0' + path.read_bytes() + b'\0')
    return result.hexdigest()


class QuietHandler(SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def copyfile(self, source, outputfile):
        try:
            super().copyfile(source, outputfile)
        except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
            # Navigation or closing an owned context may cancel an asset read.
            # Browser request failures are still recorded by the UI checker.
            pass


def wait_fixture(process, timeout=150):
    deadline = time.monotonic() + timeout
    while True:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise subprocess.TimeoutExpired('owned native fixture cleanup', timeout)
        try:
            return process.wait(timeout=min(10, remaining))
        except subprocess.TimeoutExpired:
            print('Waiting for owned native fixture cleanup; database stays running.', flush=True)


def release_fixture():
    if configuration and process and process.poll() is None:
        try:
            request = Request(configuration['api_origin'] + '/__fixture/stop', method='POST', headers={'Authorization': 'Bearer ' + configuration['stop_token']})
            with urlopen(request, timeout=5):
                pass
        except Exception:
            pass


try:
    with (LOCAL / 'browser-mobile-native.log').open('w', encoding='utf-8') as native_log, (LOCAL / 'browser-mobile-build.log').open('w', encoding='utf-8') as build_log:
        if FROZEN:
            build_before = content_source_digest()
        process = subprocess.Popen([sys.executable, str(APP / 'scripts/test_backend.py'), '--package', './internal/httpapi', '-run', '^TestAgentDeliverableCreation$' if DELIVERABLE_CREATION else '^TestAgentDeliverableUpdates$' if DELIVERABLE_UPDATES else '^TestAgentMilestoneCreation$' if MILESTONE_CREATION else '^TestTelegramDelivery$' if TELEGRAM_DELIVERY else '^TestAgentSubscriptionReview$' if SUBSCRIPTION_REVIEW else '^TestAgentFinanceSummary$' if FINANCE_SUMMARY else '^TestAgentServiceSummary$' if SERVICE_SUMMARY else '^TestTelegramDispatch$' if TELEGRAM_DISPATCH else '^TestAgentProjectStages$' if PROJECT_STAGES else '^TestAgentPostWrites$' if POST_WRITES else '^TestAgentOARequests$' if OA_REQUESTS else '^TestAgentFileCreates$' if FILE_CREATES else '^TestAgentOrganizationBrowserFixture$' if ORGANIZATION else '^TestAccountSecurityBrowserFixture$' if ACCOUNT else '^TestInAppNotifications$' if NOTIFICATIONS else '^TestAgentFileUpdates$' if FILE_UPDATES else '^TestAgentFileText$' if FILE_TEXT else '^TestAgentRunMutations$' if MUTATIONS else '^TestAgentProjectTools$' if DISCOVERY else '^TestAgentPlans$' if PLANS else '^TestAgentContextAudience$' if SECRETARY else '^TestMobileContentBrowserFixture$' if CONTENT else '^TestContractSignatureAndActivation$', '-timeout=15m', '-v'], cwd=APP, env={**os.environ, **({'CP_BROWSER_DELIVERABLE_CREATION_FIXTURE':'1'} if DELIVERABLE_CREATION else {}), **({'CP_BROWSER_DELIVERABLE_UPDATE_FIXTURE':'1'} if DELIVERABLE_UPDATES else {}), **({'CP_BROWSER_MILESTONE_CREATION_FIXTURE':'1','CP_BROWSER_AGENT_FIXTURE':'1'} if MILESTONE_CREATION else {}), 'CP_BROWSER_MOBILE_FIXTURE': '1', **({'CP_BROWSER_TELEGRAM_DELIVERY_FIXTURE':'1'} if TELEGRAM_DELIVERY else {}), **({'CP_BROWSER_SUBSCRIPTION_REVIEW_FIXTURE':'1','CP_BROWSER_AGENT_FIXTURE':'1'} if SUBSCRIPTION_REVIEW else {}), **({'CP_BROWSER_FINANCE_SUMMARY_FIXTURE':'1','CP_BROWSER_AGENT_FIXTURE':'1'} if FINANCE_SUMMARY else {}), **({'CP_BROWSER_SERVICE_SUMMARY_FIXTURE':'1','CP_BROWSER_AGENT_FIXTURE':'1'} if SERVICE_SUMMARY else {}), **({'CP_BROWSER_TELEGRAM_DISPATCH_FIXTURE':'1','CP_BROWSER_AGENT_FIXTURE':'1'} if TELEGRAM_DISPATCH else {}), **({'CP_BROWSER_PROJECT_STAGE_FIXTURE':'1'} if PROJECT_STAGES else {}), **({'CP_BROWSER_POST_WRITE_FIXTURE':'1'} if POST_WRITES else {}), **({'CP_BROWSER_OA_REQUEST_FIXTURE':'1'} if OA_REQUESTS else {}), **({'CP_BROWSER_FILE_CREATE_FIXTURE':'1'} if FILE_CREATES else {}), **({'CP_BROWSER_ORGANIZATION_FIXTURE':'1'} if ORGANIZATION else {}), **({'CP_BROWSER_ACCOUNT_FIXTURE':'1'} if ACCOUNT else {}), **({'CP_BROWSER_NOTIFICATION_FIXTURE':'1'} if NOTIFICATIONS else {}), **({'CP_BROWSER_AGENT_FILE_UPDATE_FIXTURE':'1'} if FILE_UPDATES else {}), **({'CP_BROWSER_AGENT_FILE_FIXTURE':'1'} if FILE_TEXT else {}), **({'CP_BROWSER_AGENT_MUTATION_FIXTURE':'1'} if MUTATIONS else {}), **({'CP_BROWSER_AGENT_PLAN_DISCOVERY_FIXTURE':'1'} if DISCOVERY else {}), **({'CP_BROWSER_AGENT_PLAN_FIXTURE':'1'} if PLANS else {}), **({'CP_BROWSER_AGENT_FIXTURE': '1', 'CP_BROWSER_AGENT_RUN_FIXTURE': '1'} if SECRETARY else {}), **({'CP_BROWSER_DELIVERABLE_FIXTURE': '1'} if PROJECTS else {})}, stdout=native_log, stderr=subprocess.STDOUT, creationflags=flags)
        deadline = time.monotonic() + 300
        while not FIXTURE.exists():
            if process.poll() is not None:
                raise RuntimeError('The native mobile fixture failed before readiness; inspect its private log.')
            if time.monotonic() >= deadline:
                raise RuntimeError('Native fixture readiness timed out.')
            time.sleep(0.2)
        for _ in range(20):
            try:
                configuration = json.loads(FIXTURE.read_text(encoding='utf-8'))
                break
            except json.JSONDecodeError:
                time.sleep(0.05)
        if not configuration or configuration.get('fixture_kind') != 'mobile-business':
            raise RuntimeError('Wrong or incomplete isolated fixture.')
        # Probe the fixed review port before doing any build work; do not stop
        # or replace a service owned by another task.
        server = ThreadingHTTPServer(('127.0.0.1', 5189), functools.partial(QuietHandler, directory=str(APP / 'mobile/dist/h5')))
        environment = {**os.environ, 'CP_MOBILE_BUILD_PROFILE': 'local', 'CP_MOBILE_API_ORIGIN': configuration['api_origin'], 'CI': 'true', 'TARO_CHECK_UPDATE': 'false'}
        outcome = subprocess.run([npm, 'run', 'build:preview', '--workspace', '@copypaste/mobile'], cwd=APP, env=environment, stdout=build_log, stderr=subprocess.STDOUT, timeout=480, creationflags=flags)
        if outcome.returncode:
            raise RuntimeError('The local H5 review build failed; inspect app/.local/browser-mobile-build.log.')
        if FROZEN:
            build_after = content_source_digest()
            if build_before != build_after:
                raise RuntimeError('Mobile sources changed during fixture startup or H5 build; the result is not a frozen-source proof.')
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        print('Native isolated fixture and local Taro H5 review server are ready.', flush=True)
        with (LOCAL / 'browser-mobile-ui.log').open('w', encoding='utf-8') as ui_log:
            browser_status = subprocess.run([node, str(APP / CHECKER), *(['--projects'] if PROJECTS else [])], cwd=APP, stdout=ui_log, stderr=subprocess.STDOUT, creationflags=flags, timeout=650 if OA or STATUS or SECRETARY else 500 if PROJECTS or CONTENT else 420).returncode
        print(json.dumps({'browser_exit_code': browser_status}), flush=True)
        release_fixture()
        fixture_status = wait_fixture(process)
        success = browser_status == 0 and fixture_status == 0
finally:
    # A slow native fixture can publish just as readiness times out. Recover
    # only this run's originally absent, owned mobile fixture so its stop
    # endpoint can release the Go cleanup instead of killing it mid-lifetime.
    if configuration is None and process and process.poll() is None and FIXTURE.exists():
        try:
            candidate = json.loads(FIXTURE.read_text(encoding='utf-8'))
            if candidate.get('fixture_kind') == 'mobile-business' and urlparse(candidate['api_origin']).hostname == '127.0.0.1':
                configuration = candidate
        except (OSError, ValueError, KeyError):
            pass
    release_fixture()
    if process and process.poll() is None:
        try:
            fixture_status = wait_fixture(process, timeout=30)
        except subprocess.TimeoutExpired:
            if os.name == 'nt':
                subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'], capture_output=True, creationflags=flags)
            else:
                process.terminate()
            process.wait(timeout=15)
    if process and fixture_status is None:
        fixture_status = process.poll()
    if server:
        # shutdown requires a running serve_forever loop. A failed build has
        # only a bound socket, which server_close can release directly.
        if 'thread' in globals() and thread.is_alive():
            server.shutdown()
            thread.join(timeout=5)
        server.server_close()
    if REPORT.exists():
        result = json.loads(REPORT.read_text(encoding='utf-8'))
        result['native_fixture_exit_code'] = fixture_status
        result['browser_exit_code'] = browser_status
        result['isolated_database_cleanup_passed'] = fixture_status == 0
        result['owned_review_server_stopped'] = server is not None
        if FROZEN:
            result['source_sha256_before_fixture_and_build'] = build_before
            result['source_sha256_after_build'] = build_after
            result['source_unchanged_during_fixture_and_build'] = bool(build_before and build_before == build_after)
            if 'source_sha256' in result:
                result['build_and_browser_sources_match'] = build_after == result['source_sha256']
                success = bool(success and result['source_unchanged_during_fixture_and_build'] and result['build_and_browser_sources_match'])
        result['passed'] = bool(result['passed'] and success)
        REPORT.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': success, 'native_fixture_exit_code': fixture_status, 'owned_review_server_stopped': server is not None}), flush=True)
raise SystemExit(0 if success else 1)
