"""Own a native disposable fixture and either Web or Taro H5, then close all owned resources."""
import argparse
import functools
from http.client import HTTPConnection
from http.server import BaseHTTPRequestHandler, SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import threading
import time
from urllib.request import Request, urlopen
from urllib.parse import urlsplit
from payment_fx_sources import APP, digest


class QuietHandler(SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def copyfile(self, source, outputfile):
        try:
            super().copyfile(source, outputfile)
        except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
            pass


class FixtureProxy(BaseHTTPRequestHandler):
    """Forward unmodified HTTP bytes to the owned loopback fixture, never mock it."""
    def log_message(self, *args):
        pass

    def relay(self):
        target = urlsplit(self.server.target_origin)
        assert target.scheme == 'http' and target.hostname == '127.0.0.1' and target.port
        connection = HTTPConnection(target.hostname, target.port, timeout=20)
        hop = {'host', 'connection', 'proxy-connection', 'transfer-encoding', 'keep-alive', 'te', 'trailer', 'upgrade'}
        try:
            length = int(self.headers.get('Content-Length', '0'))
            if not 0 <= length <= 1024 * 1024 or self.headers.get('Transfer-Encoding'):
                self.send_error(400)
                return
            headers = {k: v for k, v in self.headers.items() if k.lower() not in hop}
            headers['Host'] = target.netloc
            connection.request(self.command, self.path, self.rfile.read(length), headers)
            response = connection.getresponse()
            body = response.read(16 * 1024 * 1024 + 1)
            if len(body) > 16 * 1024 * 1024:
                raise OSError('Fixture response exceeds this comparison check bound')
            self.send_response(response.status)
            for key, value in response.getheaders():
                if key.lower() not in hop | {'content-length', 'server', 'date'}:
                    self.send_header(key, value)
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        except (OSError, ValueError):
            self.send_error(503)
        finally:
            connection.close()

    do_GET = do_POST = do_PUT = do_PATCH = do_DELETE = do_OPTIONS = relay


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--mobile', action='store_true')
    args = parser.parse_args()
    group = 'payment-fx-' + ('mobile' if args.mobile else 'web')
    local = APP / '.local'
    fixture_file = local / 'business-browser-fixture.json'
    report = APP.parent / 'docs/quality/evidence' / group / 'result.json'
    if fixture_file.exists() or report.exists():
        raise SystemExit('An owned fixture or evidence record exists; inspect and archive that attempt first.')
    node = shutil.which('node')
    if not node:
        raise SystemExit('Native workspace dependencies are required.')
    flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
    process = vite = build = browser = server = thread = configuration = proxy = proxy_thread = None
    fixture_status = browser_status = None
    build_source = digest()
    success = False
    failure_code = None

    def release():
        nonlocal configuration
        # A failed preparation wait can race a fixture that becomes ready later.
        # Only this owned running child may provide the private release nonce.
        if not configuration and process and process.poll() is None and fixture_file.exists():
            try:
                candidate = json.loads(fixture_file.read_text(encoding='utf-8'))
                if not isinstance(candidate, dict):
                    return
                origin = urlsplit(candidate.get('api_origin', ''))
                if candidate.get('fixture_kind') == 'human-file-scope-transfer' and candidate.get('scope_transfer_fixture') is True and candidate.get('isolated_database') is True and origin.scheme == 'http' and origin.hostname == '127.0.0.1' and origin.port and not origin.username and not origin.password and not origin.query and not origin.fragment and origin.path in ('', '/') and isinstance(candidate.get('stop_token'), str) and candidate['stop_token']:
                    configuration = candidate
            except (OSError, ValueError, TypeError):
                pass
        if configuration and process and process.poll() is None:
            try:
                request = Request(configuration['api_origin'] + '/__fixture/stop', method='POST',
                    headers={'Authorization': 'Bearer ' + configuration['stop_token']})
                with urlopen(request, timeout=5):
                    pass
            except Exception:
                pass

    def stop_owned(child):
        if child and child.poll() is None:
            if os.name == 'nt':
                subprocess.run(['taskkill', '/PID', str(child.pid), '/T', '/F'], capture_output=True, creationflags=flags)
            else:
                child.terminate()
            child.wait(timeout=15)

    try:
        with (local / (group + '-native.log')).open('w', encoding='utf-8') as native_log, (local / (group + '-build.log')).open('w', encoding='utf-8') as build_log:
            if args.mobile:
                # Compile before starting the fixed-lifetime domain fixture.
                # Only its eventual loopback API target varies between runs.
                proxy = ThreadingHTTPServer(('127.0.0.1', 5190), FixtureProxy)
                build = subprocess.Popen([node, str(APP / 'node_modules/@tarojs/cli/bin/taro'), 'build', '--type', 'h5'], cwd=APP / 'mobile',
                    env={**os.environ, 'CP_MOBILE_BUILD_PROFILE': 'local', 'CP_MOBILE_API_ORIGIN': 'http://127.0.0.1:5190', 'CI': 'true', 'TARO_CHECK_UPDATE': 'false'},
                    stdout=build_log, stderr=subprocess.STDOUT, creationflags=flags)
                if build.wait(timeout=720):
                    raise RuntimeError('Native Taro H5 review build failed.')
                if digest() != build_source:
                    raise RuntimeError('Implementation changed during the H5 prebuild.')
            process = subprocess.Popen([sys.executable, str(APP / 'scripts/test_payment_fx_native.py'), '--package', './internal/httpapi',
                '-run', '^TestFileScopeTransfers$', '-timeout=25m', '-v'], cwd=APP,
                env={**os.environ, 'CP_BROWSER_AGENT_FIXTURE':'1','CP_BROWSER_BUSINESS_FIXTURE':'1','CP_BROWSER_SCOPE_TRANSFER_FIXTURE':'1'},
                stdout=native_log, stderr=subprocess.STDOUT, creationflags=flags)
            # Preparation is separate from the unchanged 12-minute fixture
            # lifetime, 150-second cleanup wait and actual business deadlines.
            deadline = time.monotonic() + 300
            while not fixture_file.exists():
                if process.poll() is not None or time.monotonic() > deadline:
                    raise RuntimeError('Native business fixture did not become ready; private logs remain ignored.')
                time.sleep(0.2)
            for _ in range(30):
                try:
                    configuration = json.loads(fixture_file.read_text(encoding='utf-8'))
                    break
                except json.JSONDecodeError:
                    time.sleep(0.05)
            if not configuration or configuration.get('fixture_kind') != 'human-file-scope-transfer':
                raise RuntimeError('Fixture identity did not match this check.')
            if args.mobile:
                server = ThreadingHTTPServer(('127.0.0.1', 5189), functools.partial(QuietHandler, directory=str(APP / 'mobile/dist/h5')))
                proxy.target_origin = configuration['api_origin']
                proxy_thread = threading.Thread(target=proxy.serve_forever, daemon=True)
                proxy_thread.start()
                thread = threading.Thread(target=server.serve_forever, daemon=True)
                thread.start()
            else:
                vite = subprocess.Popen([node, str(APP / 'node_modules/vite/bin/vite.js'), '--port', '5189', '--strictPort'],
                    cwd=APP / 'web', env={**os.environ, 'CP_DEV_API_ORIGIN': configuration['api_origin']},
                    stdout=build_log, stderr=subprocess.STDOUT, creationflags=flags)
                deadline = time.monotonic() + 180
                while True:
                    try:
                        with urlopen(configuration['origin'], timeout=8):
                            break
                    except Exception:
                        if vite.poll() is not None or time.monotonic() > deadline:
                            raise RuntimeError('Owned Web review server did not become ready.')
                        time.sleep(0.2)
            if digest() != build_source:
                raise RuntimeError('Implementation changed during review startup/build.')
            print('Owned native fixture and review server ready; checking scoped business counts, ordinary detail navigation and authority changes.', flush=True)
            with (local / (group + '-browser.log')).open('w', encoding='utf-8') as browser_log:
                browser = subprocess.Popen([node, str(APP / 'scripts/check_payment_fx.mjs'), *(['--mobile'] if args.mobile else [])],
                    cwd=APP, env={**os.environ, 'CP_BROWSER_EVIDENCE_GROUP': group}, stdout=browser_log, stderr=subprocess.STDOUT,
                    creationflags=flags)
                browser_status = browser.wait(timeout=650)
            release()
            fixture_status = process.wait(timeout=150)
            success = browser_status == fixture_status == 0
    except Exception as error:
        failure_code = type(error).__name__
        failure_detail = str(error)
    finally:
        stop_owned(browser)
        stop_owned(build)
        release()
        if process and process.poll() is None:
            try:
                cleanup_deadline = time.monotonic() + 150
                while process.poll() is None:
                    release()
                    try:
                        fixture_status = process.wait(timeout=min(1, max(0.01, cleanup_deadline - time.monotonic())))
                    except subprocess.TimeoutExpired:
                        if time.monotonic() >= cleanup_deadline:
                            raise
                fixture_status = process.returncode
            except subprocess.TimeoutExpired:
                stop_owned(process)
        stop_owned(vite)
        if server:
            if thread and thread.is_alive():
                server.shutdown()
                thread.join(timeout=10)
            server.server_close()
        if proxy:
            if proxy_thread and proxy_thread.is_alive():
                proxy.shutdown()
                proxy_thread.join(timeout=10)
            proxy.server_close()
        cleaned = not fixture_file.exists() and process is not None and process.poll() is not None
        build_stopped = not build or build.poll() is not None
        browser_stopped = not browser or browser.poll() is not None
        proxy_stopped = not proxy_thread or not proxy_thread.is_alive()
        stopped = (not vite or vite.poll() is not None) and (not thread or not thread.is_alive()) and proxy_stopped and build_stopped and browser_stopped
        value = json.loads(report.read_text(encoding='utf-8')) if report.exists() else {
            'passed': False, 'checks': [], 'owned_browser_closed': browser is None,
            'failure': 'No browser result was produced; inspect the retained private runner logs.'}
        after = digest()
        value.update({'native_fixture_exit_code': fixture_status, 'isolated_database_cleanup_passed': fixture_status == 0 and cleaned,
            'owned_review_server_stopped': stopped, 'build_source_sha256': build_source,
            'owned_build_process_stopped': build_stopped, 'owned_browser_runner_stopped': browser_stopped,
            'owned_api_proxy_stopped': proxy_stopped, 'h5_prebuilt_before_fixture': bool(args.mobile),
            'real_api_forwarded_without_response_substitution': bool(args.mobile),
            'build_source_after_sha256': after, 'build_source_unchanged': after == build_source})
        if failure_code:
            value['runner_failure_code'] = failure_code
            value['runner_failure_detail'] = failure_detail
        value['passed'] = value['passed'] and success and cleaned and stopped and value['build_source_unchanged']
        report.parent.mkdir(parents=True, exist_ok=True)
        report.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        print(json.dumps({'passed': success and cleaned and stopped, 'native_fixture_exit_code': fixture_status,
            'owned_review_server_stopped': stopped, 'private_fixture_removed': cleaned}), flush=True)
    return 0 if success and cleaned and stopped else 1


if __name__ == '__main__':
    raise SystemExit(main())
