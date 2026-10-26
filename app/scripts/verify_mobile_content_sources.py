"""Recompute fixed-content mobile evidence; read-only native development probes.

Does not rerun backend tests, launch browsers, deploy or contact cloud providers.
"""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlparse
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
EVIDENCE = ROOT / 'docs/quality/evidence/mobile-content'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def files(directory):
    result = []
    for child in directory.iterdir():
        if child.is_dir():
            if child.name not in ('dist', 'node_modules', '.taro', '.swc'):
                result.extend(files(child))
        elif child.is_file():
            result.append(child)
    return result


def digest(paths, base):
    result = hashlib.sha256()
    for path in paths:
        result.update(path.relative_to(base).as_posix().encode() + b'\0' + path.read_bytes() + b'\0')
    return result.hexdigest()


def main():
    backend_root = APP / 'backend'
    backend = sorted(path for path in files(backend_root) if path.suffix in ('.go', '.sql') or path.name in ('go.mod', 'go.sum') or path.relative_to(backend_root).as_posix() == 'internal/contract/openapi.json')
    suite = read(EVIDENCE / 'backend-result.json')
    actual_backend = digest(backend, backend_root)
    backend_ok = suite['passed'] and suite['all_discovered_tests_executed'] and suite['source_unchanged_during_tests'] and suite['source_sha256'] == actual_backend
    web = read(EVIDENCE / 'web-build.json')
    web_paths = [(APP / name).resolve() for name in web['sources']]
    for path in web_paths:
        path.relative_to(APP.resolve())
    actual_web = digest(web_paths, APP.resolve())
    web_ok = web['passed'] and web['source_unchanged_during_run'] and web['source_sha256'] == actual_web
    browser = read(EVIDENCE / 'result.json')
    browser_paths = sorted(set(backend + files(APP / 'mobile') + [APP / name for name in (
        'package.json', 'package-lock.json', 'scripts/check_mobile_content.mjs',
        'scripts/run_mobile_browser.py', 'scripts/test_backend.py', 'scripts/sync_design.py', 'scripts/build_openapi.py',
        'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json')]), key=lambda path: path.relative_to(APP).as_posix())
    actual_browser = digest(browser_paths, APP)
    browser_ok = browser['passed'] and browser['source_unchanged_during_fixture_and_build'] and browser['build_and_browser_sources_match'] and browser['source_unchanged_during_run'] and browser['source_sha256'] == actual_browser and browser['source_files'] == len(browser_paths)
    spec = importlib.util.spec_from_file_location('cp_mobile_build_check', APP / 'scripts/verify_mobile.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    actual_mobile, mobile_count = module.digest()
    mobile = read(EVIDENCE / 'mobile-build.json')
    mobile_ok = mobile['passed'] and mobile['source_unchanged_during_run'] and mobile['source_sha256'] == actual_mobile and mobile['source_files'] == mobile_count
    visual = read(EVIDENCE / 'visual-review.json')
    visual_ok = visual.get('passed') is True and visual.get('browser_source_sha256') == actual_browser and visual.get('browser_executed_at') == browser.get('completed_at') and len(visual.get('images', [])) >= 4
    for item in visual.get('images', []):
        path = (EVIDENCE / item['file']).resolve()
        path.relative_to(EVIDENCE.resolve())
        visual_ok = visual_ok and path.suffix == '.png' and path.is_file() and hashlib.sha256(path.read_bytes()).hexdigest() == item['sha256']
    contract_ok = (ROOT / 'docs/development/openapi.json').read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes()
    cleanup = {key: browser.get(key) for key in ('all_browser_contexts_closed', 'all_browser_tabs_closed', 'isolated_database_cleanup_passed', 'owned_review_server_stopped')}
    cleanup_ok = all(cleanup.values()) and browser.get('native_fixture_exit_code') == 0 and browser.get('browser_exit_code') == 0
    environment = read(APP / '.local/environment.json')
    connection = urlparse(environment['CP_DATABASE_URL'])
    if environment.get('CP_ENV') not in ('development', 'local') or connection.hostname != '127.0.0.1':
        raise SystemExit('Only owned loopback development probes are supported.')
    sql_env = {**os.environ, 'PGHOST': connection.hostname, 'PGPORT': str(connection.port or 5432),
               'PGUSER': unquote(connection.username or ''), 'PGPASSWORD': unquote(connection.password or ''),
               'PGDATABASE': unquote(connection.path.lstrip('/')), 'PGCONNECT_TIMEOUT': '5'}
    query = "SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[a-z0-9_]+$'"
    outcome = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query], env=sql_env, capture_output=True, text=True, encoding='utf-8', creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if outcome.returncode:
        raise SystemExit('Owned database inventory failed; raw diagnostics withheld.')
    database_count = int(outcome.stdout.strip())
    probes = []
    for route in ('/', '/health/ready'):
        with urlopen('http://localhost:5178' + route, timeout=5) as response:
            probes.append({'path': route, 'status': response.status, 'bytes': len(response.read())})
    absent = not (APP / '.local/business-browser-fixture.json').exists()
    passed = backend_ok and web_ok and browser_ok and mobile_ok and visual_ok and contract_ok and cleanup_ok and absent and database_count == 0 and all(probe['status'] == 200 for probe in probes)
    report = {'passed': passed, 'executed_at': datetime.now(timezone.utc).isoformat(),
              'backend_source_sha256': actual_backend, 'backend_source_files': len(backend), 'matches_full_backend_regression': backend_ok,
              'web_source_sha256': actual_web, 'matches_web_build': web_ok,
              'browser_source_sha256': actual_browser, 'browser_source_files': len(browser_paths), 'matches_browser_record': browser_ok,
              'mobile_source_sha256': actual_mobile, 'mobile_source_files': mobile_count, 'matches_mobile_build': mobile_ok,
              'matches_visual_review_and_images': visual_ok,
              'document_and_embedded_openapi_match': contract_ok, 'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'browser_cleanup': cleanup,
              'temporary_fixture_absent': absent, 'remaining_disposable_databases': database_count,
              'development_web_probes': probes, 'production_deployment': False, 'actual_android_or_mini_program_runtime': False}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({key: report[key] for key in ('passed', 'matches_full_backend_regression', 'matches_web_build', 'matches_browser_record', 'matches_mobile_build', 'matches_visual_review_and_images', 'remaining_disposable_databases')}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
