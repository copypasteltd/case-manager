"""Exercise the actual native Taro watch server and close only its own resources."""
from datetime import datetime, timezone
import hashlib
import http.client
import json
import os
import re
from pathlib import Path
import shutil
import socket
import subprocess
import time
import urllib.request
from dependency_toolchain_sources import digest

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/dependency-toolchain'
PORT = 5189


def free_port():
    with socket.socket() as sock:
        sock.settimeout(1)
        return sock.connect_ex(('127.0.0.1', PORT)) != 0


def stop_owned(process):
    if process is not None and process.poll() is None:
        if os.name == 'nt':
            subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'], capture_output=True, timeout=30)
        else:
            process.terminate()
        process.wait(timeout=30)


def main():
    target = E / 'h5-server-check.json'
    assert not target.exists() and not (E / 'h5-client-check.json').exists(), 'Preserve earlier attempts.'
    assert free_port(), 'Owned test port must be unused before starting.'
    before = digest()
    node = shutil.which('node')
    assert node
    environment = {**os.environ, 'CP_MOBILE_BUILD_PROFILE': 'local', 'CP_MOBILE_API_ORIGIN': 'http://127.0.0.1:18080',
                   'CP_MOBILE_DEV_PORT': str(PORT), 'TARO_CHECK_UPDATE': 'false', 'NO_COLOR': '1', 'CI': 'true'}
    log = APP / '.local/dependency-toolchain-h5-server.log'
    browser_log = APP / '.local/dependency-toolchain-h5-browser.log'
    command = [node, str(APP / 'node_modules/@tarojs/cli/bin/taro'), 'build', '--type', 'h5', '--watch']
    flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
    checks = []
    process = None
    browser_process = None
    failure = None
    compilation_summary = None
    try:
        with log.open('xb') as out:
            process = subprocess.Popen(command, cwd=APP / 'mobile', env=environment, stdout=out,
                                       stderr=subprocess.STDOUT, creationflags=flags)
            print('Started owned native Taro H5 watch process.', flush=True)
            deadline = time.monotonic() + 240
            ready = False
            compilation_summary = None
            while time.monotonic() < deadline:
                if process.poll() is not None:
                    raise RuntimeError('Taro watch exited before readiness.')
                try:
                    with urllib.request.urlopen(f'http://127.0.0.1:{PORT}/', timeout=3) as response:
                        ready_html = response.status == 200 and b'<html' in response.read().lower()
                except (OSError, urllib.error.URLError):
                    ready_html = False
                compiled = re.findall(r'^webpack [^\r\n]* compiled[^\r\n]*', log.read_text(encoding='utf-8', errors='replace'), re.MULTILINE)
                if compiled:
                    compilation_summary = compiled[-1]
                    if re.search(r'\b[1-9]\d* errors?\b', compilation_summary):
                        raise RuntimeError('Current Taro compilation failed: ' + compilation_summary)
                ready = ready_html and compilation_summary is not None and ('successfully' in compilation_summary or re.search(r'compiled with \d+ warnings?\b', compilation_summary))
                if ready:
                    break
                time.sleep(0.5)
            assert ready, 'Taro watch did not become ready within the technical deadline.'
            checks.append({'name': 'Actual Taro H5 watch starts on the explicit loopback port', 'passed': True})
            checks.append({'name': 'The current watch compilation completes without errors', 'passed': True})
            connection = http.client.HTTPConnection('127.0.0.1', PORT, timeout=10)
            try:
                connection.request('GET', '/', headers={'Host': 'untrusted.invalid'})
                response = connection.getresponse(); response.read()
                assert response.status in (400, 403)
            finally:
                connection.close()
            checks.append({'name': 'Actual Taro server rejects an untrusted Host', 'passed': True})
            connection = http.client.HTTPConnection('127.0.0.1', PORT, timeout=10)
            try:
                connection.request('GET', '/', headers={'Origin': 'http://untrusted.invalid'})
                response = connection.getresponse(); response.read()
                allow_origin = response.getheader('Access-Control-Allow-Origin')
                assert response.status in (400, 403) or allow_origin not in ('*', 'http://untrusted.invalid')
            finally:
                connection.close()
            checks.append({'name': 'Untrusted browser origins are denied or receive no permissive CORS header', 'passed': True})
            with browser_log.open('xb') as browser_output:
                browser_process = subprocess.Popen([node, str(APP / 'scripts/toolchain_h5_smoke.mjs'), f'http://127.0.0.1:{PORT}',
                                                    str(E / 'h5-client-check.json')], cwd=APP, env=environment,
                                                   stdout=browser_output, stderr=subprocess.STDOUT, creationflags=flags)
                browser_status = browser_process.wait(timeout=240)
            client = json.loads((E / 'h5-client-check.json').read_bytes())
            assert browser_status == 0 and client['passed'] and client['context_closed'] and client['browser_closed'], client.get('error') or 'The isolated browser check failed.'
            checks.append({'name': 'Current login UI renders/reloads and browser/context close', 'passed': True})
    except Exception as error:
        failure = type(error).__name__ + ': ' + str(error)
    finally:
        stop_owned(browser_process)
        stop_owned(process)
        deadline = time.monotonic() + 10
        while not free_port() and time.monotonic() < deadline:
            time.sleep(0.2)
    same = before == digest()
    record = {'passed': failure is None and same and free_port() and len(checks) == 5,
              'checked_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': before, 'source_unchanged': same,
              'checks': checks, 'owned_process_terminal': process is not None and process.poll() is not None,
              'compilation_summary': compilation_summary,
              'owned_port_released': free_port(), 'failure': failure,
              'raw_server_log_sha256': hashlib.sha256(log.read_bytes()).hexdigest(),
              'raw_browser_log_sha256': hashlib.sha256(browser_log.read_bytes()).hexdigest() if browser_log.exists() else None,
              'client_record_sha256': hashlib.sha256((E / 'h5-client-check.json').read_bytes()).hexdigest() if (E / 'h5-client-check.json').exists() else None,
              'scope': 'Native loopback Taro development-server/login smoke check; no credentials, media capture, business acceptance or performance benchmark.'}
    target.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': record['passed'], 'checks': len(checks), 'port_released': record['owned_port_released'], 'failure': failure}), flush=True)
    if failure:
        print(log.read_text(encoding='utf-8', errors='replace')[-2500:])
        if browser_log.exists(): print(browser_log.read_text(encoding='utf-8', errors='replace')[-1500:])
    return 0 if record['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
