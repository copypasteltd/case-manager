"""Own the project-template native fixture, Vite, browser and bounded cleanup."""
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
REPORT = APP.parent / 'docs/quality/evidence/project-templates-web/result.json'
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
    deadline=time.monotonic()+timeout
    while time.monotonic()<deadline:
        try:
            return process.wait(timeout=min(10,max(0.1,deadline-time.monotonic())))
        except subprocess.TimeoutExpired:
            print('Waiting for owned native fixture cleanup.',flush=True)
    raise subprocess.TimeoutExpired('native project-template cleanup',timeout)


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
    with (LOCAL / 'browser-project-templates-native.log').open('w', encoding='utf-8') as native_log, (LOCAL / 'browser-project-templates-vite.log').open('w', encoding='utf-8') as vite_log:
        fixture_process = subprocess.Popen([sys.executable, str(APP / 'scripts/test_backend.py'), '--package','./internal/httpapi','-run', '^TestProjectTemplateManagement$', '-timeout=15m', '-v'],
            cwd=APP, env={**os.environ, 'CP_BROWSER_BUSINESS_FIXTURE': '1', 'CP_BROWSER_AGENT_FIXTURE': '1'}, stdout=native_log, stderr=subprocess.STDOUT, creationflags=flags)
        deadline = time.monotonic() + 150
        while not FIXTURE.exists():
            if fixture_process.poll() is not None:
                raise RuntimeError('Native fixture failed before becoming ready; inspect app/.local/browser-project-templates-native.log.')
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
                raise RuntimeError('Owned Vite failed; inspect app/.local/browser-project-templates-vite.log.')
            try:
                with urlopen(configuration['origin'], timeout=1) as response:
                    if response.status == 200:
                        break
            except Exception:
                pass
            if time.monotonic() > deadline:
                raise RuntimeError('Owned Vite readiness timed out.')
            time.sleep(0.2)
        print('Native isolated fixture and owned Vite are ready; running the real business browser chain.', flush=True)
        with (LOCAL/'browser-project-templates-node.log').open('w',encoding='utf-8') as browser_log:
            browser_status = subprocess.call([node, str(APP / 'scripts/check_project_templates.mjs')], cwd=APP, stdout=browser_log,stderr=subprocess.STDOUT,creationflags=flags)
        release_fixture()
        fixture_status = wait_fixture(fixture_process)
        success = browser_status == 0 and fixture_status == 0
finally:
    release_fixture()
    if fixture_process and fixture_process.poll() is None:
        try:
            fixture_status = wait_fixture(fixture_process)
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
