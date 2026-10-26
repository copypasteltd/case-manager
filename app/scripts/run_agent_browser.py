"""Own the isolated agent-source fixture, Vite, browser and their bounded cleanup."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time
from urllib.request import Request, urlopen

APP = Path(__file__).resolve().parents[1]
LOCAL = APP / '.local'
FIXTURE = LOCAL / 'business-browser-fixture.json'
RUNS = '--runs' in sys.argv[1:]
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
DELIVERABLE_CREATION = '--deliverable-creation' in sys.argv[1:]
DELIVERABLE_UPDATES = '--deliverable-updates' in sys.argv[1:]
MILESTONE_CREATION = '--milestone-creation' in sys.argv[1:]
PROJECT_STAGES = '--project-stages' in sys.argv[1:]
TELEGRAM_DELIVERY = '--telegram-delivery' in sys.argv[1:]
SUBSCRIPTION_REVIEW = '--subscription-review' in sys.argv[1:]
FINANCE_SUMMARY = '--finance-summary' in sys.argv[1:]
SERVICE_SUMMARY = '--service-summary' in sys.argv[1:]
TELEGRAM_DISPATCH = '--telegram-dispatch' in sys.argv[1:]
TELEGRAM = '--telegram' in sys.argv[1:]
OA_REQUESTS = '--oa-requests' in sys.argv[1:]
if sys.argv[1:] not in ([], ['--runs'], ['--plans'], ['--plan-discovery'], ['--mutations'], ['--file-text'], ['--file-updates'], ['--notifications'], ['--account-security'], ['--organization'], ['--file-creates'], ['--oa-requests'], ['--post-writes'], ['--deliverable-creation'], ['--deliverable-updates'], ['--milestone-creation'], ['--project-stages'], ['--telegram'], ['--telegram-dispatch'], ['--service-summary'], ['--subscription-review'], ['--finance-summary'], ['--telegram-delivery']):
    raise SystemExit('Usage: python app/scripts/run_agent_browser.py [--runs|--plans|--plan-discovery|--mutations|--file-text|--file-updates|--notifications|--account-security|--organization|--file-creates|--oa-requests|--post-writes|--deliverable-creation|--deliverable-updates|--milestone-creation|--project-stages|--telegram|--telegram-dispatch|--service-summary|--subscription-review|--finance-summary|--telegram-delivery]')
group = os.environ.get('CP_BROWSER_EVIDENCE_GROUP', 'deliverable-creation-web' if DELIVERABLE_CREATION else 'deliverable-updates-web' if DELIVERABLE_UPDATES else 'milestone-creation-web' if MILESTONE_CREATION else 'telegram-delivery-web' if TELEGRAM_DELIVERY else 'subscription-review-web' if SUBSCRIPTION_REVIEW else 'finance-summary-web' if FINANCE_SUMMARY else 'service-summary-web' if SERVICE_SUMMARY else 'telegram-dispatch-web' if TELEGRAM_DISPATCH else 'telegram-ingress-web' if TELEGRAM else 'secretary-project-stages-web' if PROJECT_STAGES else 'secretary-post-writes-web' if POST_WRITES else 'secretary-oa-requests-web' if OA_REQUESTS else 'secretary-file-creates-web' if FILE_CREATES else 'secretary-organization-web' if ORGANIZATION else 'account-security-web' if ACCOUNT else 'notifications-web' if NOTIFICATIONS else 'secretary-file-updates-web' if FILE_UPDATES else 'secretary-files-web' if FILE_TEXT else 'secretary-mutations-web' if MUTATIONS else 'secretary-discovery-web' if DISCOVERY else 'secretary-plans-web' if PLANS else 'secretary-runs' if RUNS else 'agent-sources')
if not group.replace('-', '').isalnum():
    raise SystemExit('Invalid browser evidence group.')
REPORT = APP.parent / 'docs/quality/evidence' / group / 'result.json'
if FIXTURE.exists():
    raise SystemExit('A browser fixture file already exists; inspect its owning run first.')
node = shutil.which('node')
vite_entry = APP / 'node_modules/vite/bin/vite.js'
if not node or not vite_entry.is_file():
    raise SystemExit('The native Node/Vite workspace dependencies are required.')
LOCAL.mkdir(exist_ok=True)
flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
fixture_process = vite_process = None
configuration = None
success = False
fixture_status = None

def stop_owned(process):
    if process and process.poll() is None:
        if os.name == 'nt':
            subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'], capture_output=True, creationflags=flags)
        else:
            process.terminate()
        process.wait(timeout=15)

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
    if configuration and fixture_process and fixture_process.poll() is None:
        try:
            request = Request(configuration['api_origin'] + '/__fixture/stop', method='POST',
                              headers={'Authorization': 'Bearer ' + configuration['stop_token']})
            with urlopen(request, timeout=5):
                pass
        except Exception:
            pass

try:
    with (LOCAL / 'browser-agent-native.log').open('w', encoding='utf-8') as native_log, (LOCAL / 'browser-agent-vite.log').open('w', encoding='utf-8') as vite_log:
        fixture_process = subprocess.Popen([sys.executable, str(APP / 'scripts/test_backend.py'), '--package', './internal/httpapi', '-run', '^TestAgentDeliverableCreation$' if DELIVERABLE_CREATION else '^TestAgentDeliverableUpdates$' if DELIVERABLE_UPDATES else '^TestAgentMilestoneCreation$' if MILESTONE_CREATION else '^TestTelegramDelivery$' if TELEGRAM_DELIVERY else '^TestAgentSubscriptionReview$' if SUBSCRIPTION_REVIEW else '^TestAgentFinanceSummary$' if FINANCE_SUMMARY else '^TestAgentServiceSummary$' if SERVICE_SUMMARY else '^TestTelegramDispatch$' if TELEGRAM_DISPATCH else '^TestTelegramIngress$' if TELEGRAM else '^TestAgentProjectStages$' if PROJECT_STAGES else '^TestAgentPostWrites$' if POST_WRITES else '^TestAgentOARequests$' if OA_REQUESTS else '^TestAgentFileCreates$' if FILE_CREATES else '^TestAgentOrganizationBrowserFixture$' if ORGANIZATION else '^TestAccountSecurityBrowserFixture$' if ACCOUNT else '^TestInAppNotifications$' if NOTIFICATIONS else '^TestAgentFileUpdates$' if FILE_UPDATES else '^TestAgentFileText$' if FILE_TEXT else '^TestAgentRunMutations$' if MUTATIONS else '^TestAgentProjectTools$' if DISCOVERY else '^TestAgentPlans$' if PLANS else '^TestAgentContextAudience$', '-timeout=15m', '-v'],
            cwd=APP, env={**os.environ, **({'CP_BROWSER_DELIVERABLE_CREATION_FIXTURE':'1'} if DELIVERABLE_CREATION else {}), **({'CP_BROWSER_DELIVERABLE_UPDATE_FIXTURE':'1'} if DELIVERABLE_UPDATES else {}), **({'CP_BROWSER_MILESTONE_CREATION_FIXTURE':'1','CP_BROWSER_AGENT_FIXTURE':'1'} if MILESTONE_CREATION else {}), 'CP_BROWSER_AGENT_FIXTURE': '1', **({'CP_BROWSER_TELEGRAM_DELIVERY_FIXTURE':'1'} if TELEGRAM_DELIVERY else {}), **({'CP_BROWSER_SUBSCRIPTION_REVIEW_FIXTURE':'1','CP_BROWSER_AGENT_FIXTURE':'1'} if SUBSCRIPTION_REVIEW else {}), **({'CP_BROWSER_FINANCE_SUMMARY_FIXTURE':'1','CP_BROWSER_AGENT_FIXTURE':'1'} if FINANCE_SUMMARY else {}), **({'CP_BROWSER_SERVICE_SUMMARY_FIXTURE':'1','CP_BROWSER_AGENT_FIXTURE':'1'} if SERVICE_SUMMARY else {}), **({'CP_BROWSER_TELEGRAM_DISPATCH_FIXTURE':'1'} if TELEGRAM_DISPATCH else {}), **({'CP_BROWSER_TELEGRAM_FIXTURE':'1'} if TELEGRAM else {}), **({'CP_BROWSER_PROJECT_STAGE_FIXTURE':'1'} if PROJECT_STAGES else {}), **({'CP_BROWSER_POST_WRITE_FIXTURE':'1'} if POST_WRITES else {}), **({'CP_BROWSER_OA_REQUEST_FIXTURE':'1'} if OA_REQUESTS else {}), **({'CP_BROWSER_FILE_CREATE_FIXTURE':'1'} if FILE_CREATES else {}), **({'CP_BROWSER_ORGANIZATION_FIXTURE':'1'} if ORGANIZATION else {}), **({'CP_BROWSER_ACCOUNT_FIXTURE':'1'} if ACCOUNT else {}), **({'CP_BROWSER_NOTIFICATION_FIXTURE':'1'} if NOTIFICATIONS else {}), **({'CP_BROWSER_AGENT_FILE_UPDATE_FIXTURE':'1'} if FILE_UPDATES else {}), **({'CP_BROWSER_AGENT_FILE_FIXTURE':'1'} if FILE_TEXT else {}), **({'CP_BROWSER_AGENT_MUTATION_FIXTURE':'1'} if MUTATIONS else {}), **({'CP_BROWSER_AGENT_PLAN_DISCOVERY_FIXTURE':'1'} if DISCOVERY else {}), **({'CP_BROWSER_AGENT_PLAN_FIXTURE':'1'} if PLANS else {}), **({'CP_BROWSER_AGENT_RUN_FIXTURE': '1'} if RUNS else {})}, stdout=native_log, stderr=subprocess.STDOUT, creationflags=flags)
        deadline = time.monotonic() + 150
        while not FIXTURE.exists():
            if fixture_process.poll() is not None:
                raise RuntimeError('Native fixture failed before becoming ready; inspect app/.local/browser-agent-native.log.')
            if time.monotonic() > deadline:
                raise RuntimeError('Native browser fixture readiness timed out.')
            time.sleep(0.2)
        # The test writes and closes this owned private file before serving work.
        for _ in range(20):
            try:
                configuration = json.loads(FIXTURE.read_text(encoding='utf-8'))
                break
            except json.JSONDecodeError:
                time.sleep(0.05)
        if not configuration:
            raise RuntimeError('Native browser fixture metadata is incomplete.')
        vite_process = subprocess.Popen([node, str(vite_entry), '--port', '5189', '--strictPort'], cwd=APP / 'web',
            env={**os.environ, 'CP_DEV_API_ORIGIN': configuration['api_origin']}, stdout=vite_log, stderr=subprocess.STDOUT, creationflags=flags)
        deadline = time.monotonic() + 30
        while True:
            if vite_process.poll() is not None:
                raise RuntimeError('Owned Vite failed; inspect app/.local/browser-agent-vite.log.')
            try:
                with urlopen(configuration['origin'], timeout=1) as response:
                    if response.status == 200:
                        break
            except Exception:
                pass
            if time.monotonic() > deadline:
                raise RuntimeError('Owned Vite readiness timed out.')
            time.sleep(0.2)
        print('Isolated agent fixture and owned Vite are ready; running real source and policy checks.', flush=True)
        with (LOCAL / 'browser-agent-check.log').open('w', encoding='utf-8') as browser_log:
            browser_status = subprocess.call([node, str(APP / ('scripts/check_deliverable_creation.mjs' if DELIVERABLE_CREATION else 'scripts/check_deliverable_updates.mjs' if DELIVERABLE_UPDATES else 'scripts/check_milestone_creation.mjs' if MILESTONE_CREATION else 'scripts/check_telegram_delivery.mjs' if TELEGRAM_DELIVERY else 'scripts/check_subscription_review.mjs' if SUBSCRIPTION_REVIEW else 'scripts/check_finance_summary.mjs' if FINANCE_SUMMARY else 'scripts/check_service_summary.mjs' if SERVICE_SUMMARY else 'scripts/check_telegram_dispatch.mjs' if TELEGRAM_DISPATCH else 'scripts/check_telegram_ingress.mjs' if TELEGRAM else 'scripts/check_project_stages.mjs' if PROJECT_STAGES else 'scripts/check_post_writes.mjs' if POST_WRITES else 'scripts/check_oa_requests.mjs' if OA_REQUESTS else 'scripts/check_file_creates.mjs' if FILE_CREATES else 'scripts/check_organization.mjs' if ORGANIZATION else 'scripts/check_account_security.mjs' if ACCOUNT else 'scripts/check_notifications.mjs' if NOTIFICATIONS else 'scripts/check_agent_file_updates.mjs' if FILE_UPDATES else 'scripts/check_agent_file_text.mjs' if FILE_TEXT else 'scripts/check_agent_mutations.mjs' if MUTATIONS else 'scripts/check_agent_plan_discovery.mjs' if DISCOVERY else 'scripts/check_agent_plans.mjs' if PLANS else 'scripts/check_agent_runs.mjs' if RUNS else 'scripts/check_agent_sources.mjs'))], cwd=APP, creationflags=flags, stdout=browser_log, stderr=subprocess.STDOUT)
        if browser_status:
            print('Browser check failed; inspect ignored app/.local/browser-agent-check.log.', flush=True)
        release_fixture()
        fixture_status = wait_fixture(fixture_process)
        success = browser_status == 0 and fixture_status == 0
finally:
    release_fixture()
    if fixture_process and fixture_process.poll() is None:
        try:
            fixture_status = wait_fixture(fixture_process, timeout=30)
        except subprocess.TimeoutExpired:
            stop_owned(fixture_process)
    stop_owned(vite_process)
    if REPORT.exists():
        report = json.loads(REPORT.read_text(encoding='utf-8'))
        report['native_fixture_exit_code'] = fixture_status
        report['isolated_database_cleanup_passed'] = fixture_status == 0
        report['owned_vite_stopped'] = vite_process is not None and vite_process.poll() is not None
        report['passed'] = report['passed'] and success
        REPORT.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': success, 'native_fixture_exit_code': fixture_status, 'owned_vite_stopped': vite_process is not None and vite_process.poll() is not None}), flush=True)
raise SystemExit(0 if success else 1)
