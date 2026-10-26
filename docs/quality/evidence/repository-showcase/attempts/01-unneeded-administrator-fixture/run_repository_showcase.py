"""Own a synthetic native fixture, Web/Taro servers and browser recordings."""
from datetime import datetime, timezone
import functools
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
from repository_showcase_sources import APP, sources, digest

LOCAL = APP / '.local'
FIXTURE = LOCAL / 'business-browser-fixture.json'
EVIDENCE = APP.parent / 'docs/quality/evidence/repository-showcase'
FLAGS = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0


def stop_owned(process):
    if process and process.poll() is None:
        if os.name == 'nt':
            subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                           capture_output=True, creationflags=FLAGS, timeout=20)
        else:
            process.terminate()
        process.wait(timeout=20)


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
        raise SystemExit('An existing private fixture must be inspected by its owner first.')
    node = shutil.which('node')
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    if not node or not npm:
        raise SystemExit('Install the native Node workspace dependencies first.')
    LOCAL.mkdir(exist_ok=True)
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    files = sources()
    before = digest(files)
    (EVIDENCE / 'source-manifest.json').write_text(json.dumps({'files': files, 'sha256': before}, indent=2) + '\n', encoding='utf-8')
    report = {'passed': False, 'started_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': before,
              'synthetic_data': True, 'real_model': False, 'real_cloud': False, 'real_device': False}
    fixture = vite = server = thread = configuration = None
    fixture_status = None
    phases = []
    failure = None

    def release():
        if fixture and fixture.poll() is None and configuration:
            try:
                request = Request(configuration['api_origin'] + '/__fixture/stop', method='POST',
                                  headers={'Authorization': 'Bearer ' + configuration['stop_token']})
                with urlopen(request, timeout=5):
                    pass
            except Exception:
                pass

    try:
        # Refuse an occupied review port; never replace an unrelated listener.
        probe = ThreadingHTTPServer(('127.0.0.1', 5189), QuietHandler)
        probe.server_close()
        with (LOCAL / 'repository-showcase-native.log').open('w', encoding='utf-8') as log:
            fixture = subprocess.Popen([sys.executable, str(APP / 'scripts/test_backend.py'), '--package', './internal/httpapi',
                                        '-run', '^TestProjectOverview$', '-timeout=18m', '-v'], cwd=APP,
                                       env={**os.environ, 'CP_BROWSER_BUSINESS_FIXTURE': '1', 'CP_BROWSER_MOBILE_FIXTURE': '1'},
                                       stdout=log, stderr=subprocess.STDOUT, creationflags=FLAGS)
            deadline = time.monotonic() + 180
            while not FIXTURE.exists():
                if fixture.poll() is not None:
                    raise RuntimeError('Owned native showcase fixture stopped before readiness; inspect its private log.')
                if time.monotonic() > deadline:
                    raise RuntimeError('Owned fixture readiness observation timed out.')
                time.sleep(0.2)
            for _ in range(20):
                try:
                    configuration = json.loads(FIXTURE.read_text(encoding='utf-8'))
                    break
                except json.JSONDecodeError:
                    time.sleep(0.05)
            if not configuration or not configuration.get('isolated_database') or not configuration.get('contract_id'):
                raise RuntimeError('The showcase requires the exact isolated overview fixture.')
            env = {**os.environ, 'CP_MOBILE_BUILD_PROFILE': 'local', 'CP_MOBILE_API_ORIGIN': configuration['api_origin'],
                   'CI': 'true', 'TARO_CHECK_UPDATE': 'false'}
            with (LOCAL / 'repository-showcase-h5.log').open('w', encoding='utf-8') as output:
                built = subprocess.run([npm, 'run', 'build:preview', '--workspace', '@copypaste/mobile'], cwd=APP,
                                       env=env, stdout=output, stderr=subprocess.STDOUT, creationflags=FLAGS, timeout=480)
            if built.returncode:
                raise RuntimeError('Current Taro H5 showcase build failed; inspect its private log.')
            if digest(files) != before:
                raise RuntimeError('Application sources changed while preparing the showcase.')
            with (LOCAL / 'repository-showcase-vite.log').open('w', encoding='utf-8') as output:
                vite = subprocess.Popen([node, str(APP / 'node_modules/vite/bin/vite.js'), '--port', '5189', '--strictPort'],
                                        cwd=APP / 'web', env={**os.environ, 'CP_DEV_API_ORIGIN': configuration['api_origin']},
                                        stdout=output, stderr=subprocess.STDOUT, creationflags=FLAGS)
            deadline = time.monotonic() + 40
            while True:
                if vite.poll() is not None:
                    raise RuntimeError('Owned Web showcase server stopped before readiness.')
                try:
                    with urlopen(configuration['origin'], timeout=1) as response:
                        if response.status == 200:
                            break
                except Exception:
                    pass
                if time.monotonic() > deadline:
                    raise RuntimeError('Owned Web showcase server readiness timed out.')
                time.sleep(0.2)
            for target in ('web', 'mobile'):
                if target == 'mobile':
                    stop_owned(vite)
                    server = ThreadingHTTPServer(('127.0.0.1', 5189),
                                                 functools.partial(QuietHandler, directory=str(APP / 'mobile/dist/h5')))
                    thread = threading.Thread(target=server.serve_forever, daemon=True)
                    thread.start()
                print('Recording current ' + target + ' UI with real isolated API; credentials excluded.', flush=True)
                with (LOCAL / ('repository-showcase-' + target + '.log')).open('w', encoding='utf-8') as output:
                    completed = subprocess.run([node, str(APP / 'scripts/capture_repository_showcase.mjs'), target], cwd=APP,
                                               stdout=output, stderr=subprocess.STDOUT, creationflags=FLAGS, timeout=300)
                phases.append({'target': target, 'exit_code': completed.returncode})
                if completed.returncode:
                    raise RuntimeError('Owned ' + target + ' showcase capture failed; inspect its sanitized report.')
            release()
            deadline = time.monotonic() + 150
            while fixture.poll() is None and time.monotonic() < deadline:
                try:
                    fixture_status = fixture.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    print('Waiting for the owned showcase fixture to clean its database.', flush=True)
            if fixture.poll() is None:
                raise RuntimeError('Owned native fixture cleanup observation timed out.')
            fixture_status = fixture.returncode
    except Exception as error:
        failure = str(error)
    finally:
        if not configuration and fixture and fixture.poll() is None and FIXTURE.exists():
            candidate = json.loads(FIXTURE.read_text(encoding='utf-8'))
            if candidate.get('isolated_database') and candidate.get('fixture_kind') == 'mobile-business':
                configuration = candidate
        release()
        if fixture and fixture.poll() is None:
            try:
                fixture_status = fixture.wait(timeout=60)
            except subprocess.TimeoutExpired:
                stop_owned(fixture)
        if fixture_status is None and fixture:
            fixture_status = fixture.poll()
        stop_owned(vite)
        if server:
            if thread and thread.is_alive():
                server.shutdown()
                thread.join(timeout=5)
            server.server_close()
        report.update({'completed_at': datetime.now(timezone.utc).isoformat(), 'source_after_sha256': digest(files),
                       'source_unchanged': digest(files) == before, 'phases': phases, 'failure': failure,
                       'native_fixture_exit_code': fixture_status, 'isolated_database_cleanup_passed': fixture_status == 0,
                       'owned_servers_closed': (not vite or vite.poll() is not None) and (not thread or not thread.is_alive()),
                       'private_fixture_removed': not FIXTURE.exists()})
        report['passed'] = (not failure and len(phases) == 2 and all(p['exit_code'] == 0 for p in phases)
                            and report['source_unchanged'] and report['isolated_database_cleanup_passed']
                            and report['owned_servers_closed'] and report['private_fixture_removed'])
        (EVIDENCE / 'result.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'phases': phases, 'cleanup': report['isolated_database_cleanup_passed'], 'failure': failure}), flush=True)
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
