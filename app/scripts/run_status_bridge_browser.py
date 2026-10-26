"""Own an isolated native fixture, two sequential clients and bounded cleanup."""
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
from status_bridge_sources import APP, digest, sources

LOCAL = APP / '.local'
FIXTURE = LOCAL / 'business-browser-fixture.json'
EVIDENCE = APP.parent / 'docs/quality/evidence/status-bridge-browser'
REPORT = EVIDENCE / 'result.json'
FLAGS = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0


class QuietHandler(SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def copyfile(self, source, outputfile):
        try:
            super().copyfile(source, outputfile)
        except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
            pass


def main():
    if FIXTURE.exists():
        raise SystemExit('An existing fixture must have its ownership inspected first.')
    node, npm = shutil.which('node'), shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    if not node or not npm or not (LOCAL / 'status-bridge/bin/status-bridge.exe').is_file():
        raise SystemExit('Native Node/npm and the freshly built Windows bridge are required.')
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    names, before = sources(), digest()
    (EVIDENCE / 'source-manifest.json').write_text(json.dumps({'files': names, 'sha256': before}, indent=2) + '\n', encoding='utf-8')
    result = {'passed': False, 'started_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': before,
              'source_file_count': len(names), 'clients': [], 'phase': 'starting owned native fixture'}
    REPORT.write_text(json.dumps(result) + '\n', encoding='utf-8')
    fixture = vite = server = thread = config = None
    fixture_status = None
    after_build = None
    success = False

    def stop_owned(process):
        if process and process.poll() is None:
            if os.name == 'nt':
                subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'], capture_output=True, creationflags=FLAGS)
            else:
                process.terminate()
            process.wait(timeout=15)

    def release():
        if fixture and fixture.poll() is None and config:
            try:
                request = Request(config['api_origin'] + '/__fixture/stop', method='POST', headers={'Authorization': 'Bearer ' + config['stop_token']})
                with urlopen(request, timeout=5):
                    pass
            except Exception:
                pass

    def wait_fixture():
        deadline = time.monotonic() + 150
        while fixture and fixture.poll() is None:
            if time.monotonic() >= deadline:
                raise RuntimeError('Owned native fixture cleanup timed out.')
            try:
                return fixture.wait(timeout=10)
            except subprocess.TimeoutExpired:
                print('Waiting for the owned database fixture to release.', flush=True)
        return fixture.poll() if fixture else None

    try:
        # Reserve the port before creating data, then let the owned Vite bind it.
        server = ThreadingHTTPServer(('127.0.0.1', 5189), functools.partial(QuietHandler, directory=str(APP / 'mobile/dist/h5')))
        server.server_close()
        server = None
        with (LOCAL / 'status-bridge-native.log').open('w', encoding='utf-8') as native_log, (LOCAL / 'status-bridge-h5.log').open('w', encoding='utf-8') as h5_log, (LOCAL / 'status-bridge-vite.log').open('w', encoding='utf-8') as vite_log:
            fixture = subprocess.Popen([sys.executable, str(APP / 'scripts/test_backend.py'), '--package', './internal/httpapi', '-run', '^TestStatusBridgeNativeFlow$', '-timeout=20m', '-v'],
                cwd=APP, env={**os.environ, 'CP_BROWSER_BUSINESS_FIXTURE': '1', 'CP_BROWSER_STATUS_BRIDGE_FIXTURE': '1'}, stdout=native_log, stderr=subprocess.STDOUT, creationflags=FLAGS)
            deadline = time.monotonic() + 240
            while not FIXTURE.exists():
                if fixture.poll() is not None or time.monotonic() >= deadline:
                    raise RuntimeError('Owned native fixture did not become ready; inspect its private log.')
                time.sleep(0.2)
            for _ in range(20):
                try:
                    config = json.loads(FIXTURE.read_text(encoding='utf-8'))
                    break
                except json.JSONDecodeError:
                    time.sleep(0.05)
            if not config or config.get('fixture_kind') != 'native-status-bridge' or not config.get('isolated_database'):
                raise RuntimeError('Unexpected fixture scope.')
            env = {**os.environ, 'CP_MOBILE_BUILD_PROFILE': 'local', 'CP_MOBILE_API_ORIGIN': config['api_origin'], 'CI': 'true', 'TARO_CHECK_UPDATE': 'false'}
            code = subprocess.run([npm, 'run', 'build:preview', '--workspace', '@copypaste/mobile'], cwd=APP, env=env, stdout=h5_log, stderr=subprocess.STDOUT, creationflags=FLAGS, timeout=720).returncode
            if code:
                raise RuntimeError('Taro H5 build failed; inspect its private log.')
            after_build = digest()
            if before != after_build:
                raise RuntimeError('Source bytes changed during fixture or build preparation.')
            vite = subprocess.Popen([node, str(APP / 'node_modules/vite/bin/vite.js'), '--port', '5189', '--strictPort'], cwd=APP / 'web',
                env={**os.environ, 'CP_DEV_API_ORIGIN': config['api_origin']}, stdout=vite_log, stderr=subprocess.STDOUT, creationflags=FLAGS)
            deadline = time.monotonic() + 40
            while True:
                if vite.poll() is not None or time.monotonic() >= deadline:
                    raise RuntimeError('Owned Vite did not become ready.')
                try:
                    with urlopen(config['origin'], timeout=1) as response:
                        if response.status == 200:
                            break
                except Exception:
                    pass
                time.sleep(0.2)
            print('Owned native bridge fixture ready; checking Web and then Taro H5.', flush=True)
            for client in ('web', 'mobile'):
                if client == 'mobile':
                    stop_owned(vite)
                    server = ThreadingHTTPServer(('127.0.0.1', 5189), functools.partial(QuietHandler, directory=str(APP / 'mobile/dist/h5')))
                    thread = threading.Thread(target=server.serve_forever, daemon=True)
                    thread.start()
                with (LOCAL / ('status-bridge-' + client + '-browser.log')).open('w', encoding='utf-8') as log:
                    status = subprocess.run([node, str(APP / 'scripts/check_status_bridge.mjs')], cwd=APP,
                        env={**os.environ, 'CP_STATUS_BRIDGE_CLIENT': client}, stdout=log, stderr=subprocess.STDOUT, creationflags=FLAGS, timeout=540).returncode
                result['clients'].append({'client': client, 'exit_code': status})
                if status or digest() != before:
                    raise RuntimeError('Owned browser phase failed or its bound source changed; inspect its local log.')
            release()
            fixture_status = wait_fixture()
            success = fixture_status == 0
    finally:
        if not config and fixture and fixture.poll() is None and FIXTURE.exists():
            candidate = json.loads(FIXTURE.read_text(encoding='utf-8'))
            if candidate.get('fixture_kind') == 'native-status-bridge' and candidate.get('isolated_database'):
                config = candidate
        release()
        try:
            if fixture:
                fixture_status = wait_fixture()
        finally:
            stop_owned(fixture)
            stop_owned(vite)
            if server:
                if thread and thread.is_alive():
                    server.shutdown()
                    thread.join(timeout=5)
                server.server_close()
        phases = []
        for client in ('web', 'mobile'):
            path = EVIDENCE / (client + '.json')
            if path.is_file():
                phases.append(json.loads(path.read_text(encoding='utf-8')))
        after = digest()
        result.update({'completed_at': datetime.now(timezone.utc).isoformat(), 'source_sha256_after_build': after_build,
            'source_sha256_after_browser': after, 'source_unchanged': before == after_build == after,
            'phases': phases, 'native_fixture_exit_code': fixture_status, 'isolated_database_cleanup_passed': fixture_status == 0,
            'owned_review_server_stopped': not thread or not thread.is_alive(), 'owned_vite_stopped': vite is None or vite.poll() is not None,
            'owned_browser_closed': len(phases) == 2 and all(p['owned_browser_closed'] for p in phases),
            'contexts_created': sum(p['contexts_created'] for p in phases), 'contexts_closed': sum(p['contexts_closed'] for p in phases),
            'screenshots': [s for p in phases for s in p['screenshots']], 'recordings': [r for p in phases for r in p['recordings']],
            'checks': [c for p in phases for c in p['checks']], 'layouts': [l for p in phases for l in p['layouts']],
            'fixture_metadata_removed': not FIXTURE.exists(), 'real_device': False, 'synthetic_data': True})
        result['passed'] = bool(success and result['source_unchanged'] and len(phases) == 2 and all(p['passed'] and p['source_sha256'] == before for p in phases)
            and result['owned_browser_closed'] and result['contexts_created'] == result['contexts_closed'] == 2 and result['fixture_metadata_removed'])
        REPORT.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        print(json.dumps({'passed': result['passed'], 'native_fixture_exit_code': fixture_status, 'contexts_closed': result['contexts_closed']}), flush=True)
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
