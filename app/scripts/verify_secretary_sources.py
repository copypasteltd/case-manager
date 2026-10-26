"""Recompute secretary implementation evidence and owned local cleanup facts.

Read-only: does not start tests, browsers, runtimes or external services.
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
EVIDENCE = ROOT / 'docs/quality/evidence/secretary-runs'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def digest(paths, separator=b'\0', trailing=True):
    value = hashlib.sha256()
    for relative in paths:
        path = (APP / relative).resolve()
        path.relative_to(APP)
        value.update(relative.encode('utf-8') + separator + path.read_bytes())
        if trailing:
            value.update(b'\0')
    return value.hexdigest()


def main():
    backend = read(EVIDENCE / 'backend-result.json')
    backend_paths = sorted(p.relative_to(APP / 'backend').as_posix() for p in (APP / 'backend').rglob('*')
                           if p.is_file() and (p.suffix in ('.go', '.sql') or p.name in ('go.mod', 'go.sum') or p.relative_to(APP / 'backend').as_posix() == 'internal/contract/openapi.json'))
    value = hashlib.sha256()
    for relative in backend_paths:
        value.update(relative.encode('utf-8') + b'\0' + (APP / 'backend' / relative).read_bytes() + b'\0')
    backend_ok = bool(backend.get('passed') and backend.get('all_discovered_tests_executed') and backend.get('source_unchanged_during_tests') and backend['source_sha256'] == value.hexdigest())
    web = read(EVIDENCE / 'web-build.json')
    web_ok = bool(web['passed'] and web['source_unchanged_during_run'] and web['source_sha256'] == digest(web['sources']))
    mobile_root = ROOT / 'docs/quality/evidence/mobile-secretary'
    mobile = read(mobile_root / 'builds.json')
    spec = importlib.util.spec_from_file_location('mobile_source_digest', APP / 'scripts/verify_mobile.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    mobile_hash, mobile_count = module.digest()
    mobile_ok = bool(mobile['passed'] and mobile['source_unchanged_during_run'] and mobile['source_sha256'] == mobile_hash and mobile['source_files'] == mobile_count)
    browser_checks = []
    for label, path in [('web', EVIDENCE), ('mobile', mobile_root)]:
        record = read(path / 'result.json')
        current = digest(record['source_files'], b'\n', False) if label == 'web' else digest(record['source_files'])
        cleanup_ok = bool(record.get('all_browser_contexts_closed') and record.get('isolated_database_cleanup_passed') and record.get('native_fixture_exit_code') == 0 and record.get('owned_vite_stopped' if label == 'web' else 'owned_review_server_stopped'))
        matches = record['source_sha256'] == current
        if label == 'mobile':
            matches = matches and record.get('build_and_browser_sources_match') and record.get('source_unchanged_during_fixture_and_build')
        browser_checks.append({'platform': label, 'passed': bool(record['passed'] and record['source_unchanged'] and matches and cleanup_ok), 'current_source_matches': bool(matches), 'cleanup_passed': cleanup_ok, 'checks': len(record['checks'])})
    environment = read(APP / '.local/environment.json')
    connection = urlparse(environment['CP_DATABASE_URL'])
    if environment.get('CP_ENV') not in ('development', 'local') or connection.hostname != '127.0.0.1':
        raise SystemExit('Only owned loopback development inventory is supported.')
    sql_env = {**os.environ, 'PGHOST': connection.hostname, 'PGPORT': str(connection.port or 5432),
               'PGUSER': unquote(connection.username or ''), 'PGPASSWORD': unquote(connection.password or ''),
               'PGDATABASE': unquote(connection.path.lstrip('/')), 'PGCONNECT_TIMEOUT': '5'}
    query = "SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[a-z0-9_]+$'"
    result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
                            env=sql_env, capture_output=True, text=True, encoding='utf-8', timeout=15,
                            creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if result.returncode:
        raise SystemExit('Owned database inventory failed; raw diagnostics withheld.')
    remaining = int(result.stdout.strip())
    probes = []
    for route in ('/', '/health/ready'):
        with urlopen('http://localhost:5178' + route, timeout=5) as response:
            probes.append({'path': route, 'status': response.status})
    absent = not (APP / '.local/business-browser-fixture.json').exists()
    contract_ok = (ROOT / 'docs/development/openapi.json').read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes()
    passed = bool(backend_ok and web_ok and mobile_ok and all(item['passed'] for item in browser_checks) and contract_ok and absent and remaining == 0 and all(item['status'] == 200 for item in probes))
    report = {'passed': passed, 'executed_at': datetime.now(timezone.utc).isoformat(), 'matches_full_backend_regression': backend_ok,
              'backend_source_sha256': value.hexdigest(), 'matches_web_build': web_ok, 'matches_mobile_build': mobile_ok,
              'browser_records': browser_checks, 'document_and_embedded_openapi_match': contract_ok,
              'remaining_disposable_databases': remaining, 'temporary_fixture_absent': absent, 'development_web_probes': probes,
              'original_development_services_restarted': False, 'production_deployment': False, 'actual_model_or_cloud': False,
              'actual_mini_program_or_android_runtime': False, 'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(report, ensure_ascii=False))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
