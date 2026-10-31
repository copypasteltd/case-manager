"""Own a native disposable fixture and either Web or Taro H5, then close all owned resources."""
import argparse
import hashlib
import contract_body_native_cluster as cluster
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
from project_catalog_sources import APP, digest


class QuietHandler(SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def copyfile(self, source, outputfile):
        try:
            super().copyfile(source, outputfile)
        except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
            pass


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--mobile', action='store_true')
    args = parser.parse_args()
    if args.mobile:
        raise SystemExit('This focused capture covers Web only; mobile browser journeys remain deferred.')
    group = 'project-catalog-' + ('mobile' if args.mobile else 'web')
    cluster.OWNER = APP / '.local/project-catalog-browser-cluster.json'
    cluster.E = APP.parent / 'docs/quality/evidence/project-catalog' / ('native-' + group)
    local = APP / '.local'
    fixture_file = local / 'business-browser-fixture.json'
    report = APP.parent / 'docs/quality/evidence' / group / 'result.json'
    if fixture_file.exists() or report.exists():
        raise SystemExit('An owned fixture or evidence record exists; inspect and archive that attempt first.')
    node = str(APP / '.tools/node-v22.23.4-win-x64/node.exe')
    if not node:
        raise SystemExit('Native workspace dependencies are required.')
    flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
    process = vite = build = browser = server = thread = configuration = None
    fixture_status = browser_status = None
    build_source = digest()
    success = False

    def release():
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
            process = subprocess.Popen([str(APP / '.tools/go/bin/go.exe'), 'test', './internal/httpapi', '-count=1',
                '-run', '^TestAgentProjectCatalog/browser_workbench$', '-timeout=25m', '-v'], cwd=APP / 'backend',
                env={**cluster.test_environment(), 'CP_BROWSER_BUSINESS_FIXTURE':'1','CP_BROWSER_PROJECT_CATALOG_FIXTURE':'1'},
                stdout=native_log, stderr=subprocess.STDOUT, creationflags=flags)
            deadline = time.monotonic() + 150
            while not fixture_file.exists():
                if process.poll() is not None or time.monotonic() > deadline:
                    raise RuntimeError('Native project planning catalog fixture did not become ready; private logs remain ignored.')
                time.sleep(0.2)
            for _ in range(30):
                try:
                    configuration = json.loads(fixture_file.read_text(encoding='utf-8'))
                    break
                except json.JSONDecodeError:
                    time.sleep(0.05)
            if not configuration or configuration.get('fixture_kind') != 'project-catalog':
                raise RuntimeError('Fixture identity did not match this check.')
            if args.mobile:
                server = ThreadingHTTPServer(('127.0.0.1', 5189), functools.partial(QuietHandler, directory=str(APP / 'mobile/dist/h5')))
                build = subprocess.Popen([node, str(APP / 'node_modules/@tarojs/cli/bin/taro'), 'build', '--type', 'h5'], cwd=APP / 'mobile',
                    env={**os.environ, 'CP_MOBILE_BUILD_PROFILE': 'local', 'CP_MOBILE_API_ORIGIN': configuration['api_origin'], 'CI': 'true', 'TARO_CHECK_UPDATE': 'false'},
                    stdout=build_log, stderr=subprocess.STDOUT, creationflags=flags)
                if build.wait(timeout=720):
                    raise RuntimeError('Native Taro H5 review build failed.')
                thread = threading.Thread(target=server.serve_forever, daemon=True)
                thread.start()
            else:
                vite = subprocess.Popen([node, str(APP / 'node_modules/vite/bin/vite.js'), '--port', '5189', '--strictPort'],
                    cwd=APP / 'web', env={**os.environ, 'CP_DEV_API_ORIGIN': configuration['api_origin']},
                    stdout=build_log, stderr=subprocess.STDOUT, creationflags=flags)
                deadline = time.monotonic() + 45
                while True:
                    try:
                        with urlopen(configuration['origin'], timeout=2):
                            break
                    except Exception:
                        if vite.poll() is not None or time.monotonic() > deadline:
                            raise RuntimeError('Owned Web review server did not become ready.')
                        time.sleep(0.2)
            if digest() != build_source:
                raise RuntimeError('Implementation changed during review startup/build.')
            print('Owned native fixture ready; checking people/template selection, human approval and actual project initialization.', flush=True)
            with (local / (group + '-browser.log')).open('w', encoding='utf-8') as browser_log:
                browser = subprocess.Popen([node, str(APP / 'scripts/check_project_catalog.mjs'), *(['--mobile'] if args.mobile else [])],
                    cwd=APP, env={**os.environ, 'CP_BROWSER_EVIDENCE_GROUP': group}, stdout=browser_log, stderr=subprocess.STDOUT,
                    creationflags=flags)
                browser_status = browser.wait(timeout=650)
            release()
            fixture_status = process.wait(timeout=150)
            success = browser_status == fixture_status == 0
    finally:
        stop_owned(browser)
        stop_owned(build)
        release()
        if process and process.poll() is None:
            try:
                fixture_status = process.wait(timeout=150)
            except subprocess.TimeoutExpired:
                stop_owned(process)
        stop_owned(vite)
        if server:
            if thread and thread.is_alive():
                server.shutdown()
                thread.join(timeout=10)
            server.server_close()
        if cluster.OWNER.exists():
            cluster.stop()
            identity = hashlib.sha256(cluster.OWNER.read_bytes()).hexdigest()[:16]
            cluster.OWNER.rename(cluster.OWNER.with_name('project-catalog-browser-cluster-closed-' + identity + '.json'))
        cleaned = not fixture_file.exists() and process is not None and process.poll() is not None
        build_stopped = not build or build.poll() is not None
        browser_stopped = not browser or browser.poll() is not None
        stopped = (not vite or vite.poll() is not None) and (not thread or not thread.is_alive()) and build_stopped and browser_stopped
        if report.exists():
            value = json.loads(report.read_text(encoding='utf-8'))
            value.update({'native_fixture_exit_code': fixture_status, 'isolated_database_cleanup_passed': fixture_status == 0 and cleaned,
                'owned_review_server_stopped': stopped, 'build_source_sha256': build_source,
                'owned_build_process_stopped': build_stopped, 'owned_browser_runner_stopped': browser_stopped,
                'build_source_after_sha256': digest(), 'build_source_unchanged': digest() == build_source})
            value['passed'] = value['passed'] and success and cleaned and stopped and value['build_source_unchanged']
            report.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        print(json.dumps({'passed': success and cleaned and stopped, 'native_fixture_exit_code': fixture_status,
            'owned_review_server_stopped': stopped, 'private_fixture_removed': cleaned}), flush=True)
    return 0 if success and cleaned and stopped else 1


if __name__ == '__main__':
    raise SystemExit(main())
