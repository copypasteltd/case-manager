"""Own the isolated spend-documents fixture, Vite, browser and their bounded cleanup."""
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
if sys.argv[1:] not in ([], ['--fulfillment'], ['--costs'], ['--payments'], ['--subscriptions'], ['--contract-finance'], ['--contract-closure'], ['--purchase-variance'], ['--purchase-completion'], ['--deliverables'], ['--project-changes'], ['--project-closeout']):
    raise SystemExit('Usage: python scripts/run_spend_browser.py [--fulfillment|--costs|--payments|--subscriptions|--contract-finance|--contract-closure|--purchase-variance|--purchase-completion|--deliverables|--project-changes|--project-closeout]')
mode = sys.argv[1] if sys.argv[1:] else ''
label, checker, directory = {
    '': ('spend', 'check_spend.mjs', 'spend-documents'),
    '--fulfillment': ('purchase-fulfillment', 'check_purchase_fulfillment.mjs', 'purchase-fulfillment'),
    '--costs': ('actual-costs', 'check_costs.mjs', 'actual-costs'),
    '--payments': ('payments-settlement', 'check_payments.mjs', 'payments-settlement'),
    '--subscriptions': ('subscriptions', 'check_subscriptions.mjs', 'subscriptions'),
    '--contract-finance': ('contract-finance', 'check_contract_finance.mjs', 'contract-finance'),
    '--contract-closure': ('contract-closure', 'check_contract_closure.mjs', 'contract-closure'),
    '--purchase-variance': ('purchase-variance', 'check_purchase_variance.mjs', 'purchase-variance'),
    '--purchase-completion': ('purchase-completion', 'check_purchase_completion.mjs', 'purchase-completion'),
    '--deliverables': ('project-deliverables', 'check_project_deliverables.mjs', 'project-deliverables'),
    '--project-changes': ('project-changes', 'check_project_changes.mjs', 'project-changes'),
    '--project-closeout': ('project-closeout', 'check_project_closeout.mjs', 'project-closeout'),
}[mode]
REPORT = APP.parent / 'docs/quality/evidence' / directory / 'result.json'
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
    with (LOCAL / ('browser-' + label + '-native.log')).open('w', encoding='utf-8') as native_log, (LOCAL / ('browser-' + label + '-vite.log')).open('w', encoding='utf-8') as vite_log:
        fixture_process = subprocess.Popen([sys.executable, str(APP / 'scripts/test_backend.py'), '-run', '^TestContractSignatureAndActivation$', '-timeout=15m', '-v'],
            cwd=APP, env={**os.environ, 'CP_BROWSER_MOBILE_FIXTURE': '1', **({'CP_BROWSER_DELIVERABLE_FIXTURE': '1'} if mode == '--deliverables' else {}), **({'CP_BROWSER_PROJECT_CHANGE_FIXTURE': '1'} if mode == '--project-changes' else {}), **({'CP_BROWSER_PROJECT_CLOSEOUT_FIXTURE': '1'} if mode == '--project-closeout' else {})}, stdout=native_log, stderr=subprocess.STDOUT, creationflags=flags)
        deadline = time.monotonic() + 150
        while not FIXTURE.exists():
            if fixture_process.poll() is not None:
                raise RuntimeError('Native fixture failed before becoming ready; inspect app/.local/browser-' + label + '-native.log.')
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
                raise RuntimeError('Owned Vite failed; inspect app/.local/browser-' + label + '-vite.log.')
            try:
                with urlopen(configuration['origin'], timeout=1) as response:
                    if response.status == 200:
                        break
            except Exception:
                pass
            if time.monotonic() > deadline:
                raise RuntimeError('Owned Vite readiness timed out.')
            time.sleep(0.2)
        print('Isolated fixture and owned Vite are ready; running ' + label + ' Web/API checks.', flush=True)
        with (LOCAL / ('browser-' + label + '-check.log')).open('w', encoding='utf-8') as browser_log:
            browser_status = subprocess.call([node, str(APP / 'scripts' / checker)], cwd=APP, creationflags=flags, stdout=browser_log, stderr=subprocess.STDOUT)
        if browser_status:
            print('Browser check failed; inspect ignored app/.local/browser-' + label + '-check.log.', flush=True)
        release_fixture()
        fixture_status = fixture_process.wait(timeout=30)
        success = browser_status == 0 and fixture_status == 0
finally:
    release_fixture()
    if fixture_process and fixture_process.poll() is None:
        try:
            fixture_status = fixture_process.wait(timeout=20)
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
