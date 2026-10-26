"""Own native PostgreSQL fixture, Taro H5 review and browser; no virtual runtime."""
from datetime import datetime, timezone
import functools
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import shutil
import subprocess
import sys
import threading
import time
from urllib.request import Request, urlopen
from mobile_tasks_sources import APP, digest, sources

LOCAL = APP / '.local'
FIXTURE = LOCAL / 'business-browser-fixture.json'
EVIDENCE = APP.parent / 'docs/quality/evidence/mobile-tasks-browser'
REPORT = EVIDENCE / 'result.json'
if FIXTURE.exists():
    raise SystemExit('An existing private browser fixture must have its owner inspected first.')
node, npm = shutil.which('node'), shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
if not node or not npm:
    raise SystemExit('Native Node/npm are required.')
flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
LOCAL.mkdir(exist_ok=True)
EVIDENCE.mkdir(parents=True, exist_ok=True)
names = sources()
before = digest(names)
(EVIDENCE / 'source-manifest.json').write_text(json.dumps({'files': names, 'sha256': before}, indent=2) + '\n', encoding='utf-8')
REPORT.write_text(json.dumps({'passed': False, 'started_at': datetime.now(timezone.utc).isoformat(), 'phase': 'starting owned native fixture'}) + '\n', encoding='utf-8')
process = server = thread = configuration = None
browser_status = fixture_status = None
after_build = None
success = False


class QuietHandler(SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def copyfile(self, source, outputfile):
        try:
            super().copyfile(source, outputfile)
        except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
            pass


def release():
    if process and process.poll() is None and configuration:
        try:
            request = Request(configuration['api_origin'] + '/__fixture/stop', method='POST', headers={'Authorization': 'Bearer ' + configuration['stop_token']})
            with urlopen(request, timeout=5):
                pass
        except Exception:
            pass


def wait_owned(timeout=150):
    deadline = time.monotonic() + timeout
    while process and process.poll() is None:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise subprocess.TimeoutExpired('owned task workflow fixture cleanup', timeout)
        try:
            return process.wait(timeout=min(10, remaining))
        except subprocess.TimeoutExpired:
            print('Waiting for the owned native fixture to release its database.', flush=True)
    return process.poll() if process else None


try:
    # Binding is a read-only ownership probe: do not replace another listener.
    server = ThreadingHTTPServer(('127.0.0.1', 5189), functools.partial(QuietHandler, directory=str(APP / 'mobile/dist/h5')))
    with (LOCAL / 'mobile-tasks-native.log').open('w', encoding='utf-8') as native_log, (LOCAL / 'mobile-tasks-h5.log').open('w', encoding='utf-8') as build_log:
        process = subprocess.Popen([sys.executable, str(APP / 'scripts/test_backend.py'), '--package', './internal/httpapi', '-run', '^TestProjectTemplateManagement$', '-timeout=20m', '-v'], cwd=APP,
            env={**os.environ, 'CP_BROWSER_BUSINESS_FIXTURE': '1', 'CP_BROWSER_MOBILE_FIXTURE': '1'}, stdout=native_log, stderr=subprocess.STDOUT, creationflags=flags)
        deadline = time.monotonic() + 180
        while not FIXTURE.exists():
            if process.poll() is not None:
                raise RuntimeError('Owned task workflow fixture stopped before readiness; inspect its private log.')
            if time.monotonic() >= deadline:
                raise RuntimeError('Owned task workflow fixture readiness timed out.')
            time.sleep(0.2)
        for _ in range(20):
            try:
                configuration = json.loads(FIXTURE.read_text(encoding='utf-8'))
                break
            except json.JSONDecodeError:
                time.sleep(0.05)
        if not configuration or configuration.get('fixture_kind') != 'mobile-business' or not configuration.get('isolated_database'):
            raise RuntimeError('Unexpected browser fixture ownership or database scope.')
        environment = {**os.environ, 'CP_MOBILE_BUILD_PROFILE': 'local', 'CP_MOBILE_API_ORIGIN': configuration['api_origin'], 'CI': 'true', 'TARO_CHECK_UPDATE': 'false'}
        status = subprocess.run([npm, 'run', 'build:preview', '--workspace', '@copypaste/mobile'], cwd=APP, env=environment, stdout=build_log, stderr=subprocess.STDOUT, creationflags=flags, timeout=720).returncode
        if status:
            raise RuntimeError('The Taro H5 review build failed; inspect its local log.')
        after_build = digest(names)
        if before != after_build:
            raise RuntimeError('Sources changed during fixture/build preparation; preserve this attempt and rerun from fixed source.')
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        print('Owned native fixture and Taro review are ready; checking real task creation, editing and dependencies.', flush=True)
        with (LOCAL / 'mobile-tasks-browser.log').open('w', encoding='utf-8') as browser_log:
            browser_status = subprocess.run([node, str(APP / 'scripts/check_mobile_tasks.mjs')], cwd=APP, stdout=browser_log, stderr=subprocess.STDOUT, creationflags=flags, timeout=720).returncode
        release()
        fixture_status = wait_owned()
        success = browser_status == fixture_status == 0
finally:
    if not configuration and process and process.poll() is None and FIXTURE.exists():
        try:
            candidate = json.loads(FIXTURE.read_text(encoding='utf-8'))
            if candidate.get('fixture_kind') == 'mobile-business' and candidate.get('isolated_database'):
                configuration = candidate
        except (OSError, ValueError):
            pass
    release()
    if process and process.poll() is None:
        try:
            fixture_status = wait_owned()
        except subprocess.TimeoutExpired:
            if os.name == 'nt':
                subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'], capture_output=True, creationflags=flags)
            else:
                process.terminate()
            process.wait(timeout=15)
    if process and fixture_status is None:
        fixture_status = process.poll()
    if server:
        if thread and thread.is_alive():
            server.shutdown()
            thread.join(timeout=5)
        server.server_close()
    result = json.loads(REPORT.read_text(encoding='utf-8'))
    result.update({'native_fixture_exit_code': fixture_status, 'browser_exit_code': browser_status, 'isolated_database_cleanup_passed': fixture_status == 0,
        'owned_review_server_stopped': bool(server and (not thread or not thread.is_alive())), 'source_sha256_before_fixture_and_build': before,
        'source_sha256_after_build': after_build, 'source_unchanged_during_fixture_and_build': before == after_build,
        'build_and_browser_sources_match': before == result.get('source_sha256') == after_build})
    result['passed'] = bool(result.get('passed') and success and result['build_and_browser_sources_match'] and digest(names) == before)
    REPORT.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': result['passed'], 'native_fixture_exit_code': fixture_status, 'owned_review_server_stopped': result['owned_review_server_stopped']}), flush=True)
raise SystemExit(0 if result['passed'] else 1)
