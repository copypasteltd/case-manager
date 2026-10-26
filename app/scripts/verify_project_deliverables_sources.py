"""Recompute final local evidence sources and check owned Web/database cleanup.

Read-only probes; never starts a browser or sends an external request.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlparse
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
EVIDENCE = ROOT / 'docs/quality/evidence/project-deliverables'


def save(name, report):
    (EVIDENCE / name).write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def backend_files():
    return sorted(p for p in (APP / 'backend').rglob('*') if p.is_file() and
                  (p.suffix in ('.go', '.sql') or p.name in ('go.mod', 'go.sum') or p.relative_to(APP / 'backend').as_posix() == 'internal/contract/openapi.json'))


def digest(paths, base):
    value = hashlib.sha256()
    for path in paths:
        value.update(path.relative_to(base).as_posix().encode('utf-8') + b'\0')
        value.update(path.read_bytes() + b'\0')
    return value.hexdigest()


def web_files(directory):
    result = []
    for child in directory.iterdir():
        if child.is_dir():
            if child.name not in ('dist', 'node_modules', '.taro', '.swc'):
                result.extend(web_files(child))
        elif child.is_file():
            result.append(child)
    return result


def main():
    suite = json.loads((ROOT / 'docs/quality/evidence/backend/result.json').read_text(encoding='utf-8'))
    browser = json.loads((EVIDENCE / 'result.json').read_text(encoding='utf-8'))
    backend_source = backend_files()
    browser_source = sorted(set(backend_source + web_files(APP / 'web/src') + web_files(APP / 'mobile/src') + [APP / name for name in (
        'web/vite.config.ts', 'web/package.json', 'package.json', 'package-lock.json',
        'packages/contracts/src/api.d.ts', 'packages/design/src/tokens.json',
        'scripts/check_project_deliverables.mjs', 'scripts/run_spend_browser.py')]), key=lambda p: p.relative_to(APP).as_posix())
    actual_backend = digest(backend_source, APP / 'backend')
    actual_browser = digest(browser_source, APP)
    same_contract = (ROOT / 'docs/development/openapi.json').read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes()
    source_ok = suite['passed'] and suite['source_unchanged_during_tests'] and suite['all_discovered_tests_executed'] and suite['source_sha256'] == actual_backend
    source_ok = source_ok and browser['passed'] and browser['source_unchanged_during_tests'] and browser['source_sha256'] == actual_browser and browser['source_file_count'] == len(browser_source) and same_contract
    report = {'passed': source_ok, 'checked_at': datetime.now(timezone.utc).isoformat(),
              'backend_source_sha256': actual_backend, 'backend_matches_success_record': actual_backend == suite['source_sha256'],
              'backend_source_file_count': len(backend_source), 'browser_source_sha256': actual_browser,
              'browser_matches_success_record': actual_browser == browser['source_sha256'], 'browser_source_file_count': len(browser_source),
              'document_and_embedded_openapi_match': same_contract}
    save('final-source-check.json', report)

    values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    connection = urlparse(values['CP_DATABASE_URL'])
    if values.get('CP_ENV') not in ('development', 'local') or connection.hostname != '127.0.0.1':
        raise SystemExit('Only owned loopback development probes are supported.')
    env = {**os.environ, 'PGHOST': connection.hostname, 'PGPORT': str(connection.port or 5432),
           'PGUSER': unquote(connection.username or ''), 'PGPASSWORD': unquote(connection.password or ''),
           'PGDATABASE': unquote(connection.path.lstrip('/')), 'PGCONNECT_TIMEOUT': '5'}
    query = "SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[a-z0-9_]+$'"
    sql = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
                         env=env, capture_output=True, text=True, encoding='utf-8', creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if sql.returncode:
        raise SystemExit('Owned database inventory failed; raw diagnostics withheld.')
    count = int(sql.stdout.strip())
    probes = []
    for route, content_type in (('/', 'text/html'), ('/health/ready', 'application/json')):
        with urlopen('http://localhost:5178' + route, timeout=5) as response:
            actual_type = response.headers.get('Content-Type', '')
            probes.append({'path': route, 'status': response.status, 'expected_status': 200,
                           'content_type': actual_type, 'content_type_matches': actual_type.startswith(content_type), 'bytes': len(response.read())})
    cleanup_keys = ('all_owned_browser_contexts_closed', 'all_owned_browser_tabs_closed', 'isolated_database_cleanup_passed', 'owned_vite_stopped')
    cleanup = {key: browser.get(key, False) for key in cleanup_keys}
    absent = not (APP / '.local/business-browser-fixture.json').exists()
    passed = source_ok and all(cleanup.values()) and browser['native_fixture_exit_code'] == 0 and absent and count == 0 and all(p['status'] == 200 and p['content_type_matches'] for p in probes)
    save('development-web-and-cleanup.json', {'passed': passed, 'executed_at': datetime.now(timezone.utc).isoformat(),
         'scope': 'Owned local development Web/API proxy and disposable native test database cleanup; not production',
         'web_origin': 'http://localhost:5178', 'web_probes': probes, 'remaining_disposable_databases': count,
         'temporary_browser_fixture_absent': absent, 'browser_cleanup': cleanup, 'production_deployment': False, 'worker_started': False})
    print(json.dumps({'passed': passed, 'backend_matches': actual_backend == suite['source_sha256'], 'browser_matches': actual_browser == browser['source_sha256'], 'remaining_disposable_databases': count}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
