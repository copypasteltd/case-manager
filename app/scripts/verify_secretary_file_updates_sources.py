"""Recompute exact file-update proofs and owned cleanup against current files."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/secretary-file-updates'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def module(name, file):
    spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / file)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def digest(paths, separator=b'\0', trailing=True):
    value = hashlib.sha256()
    for name in paths:
        value.update(name.encode('utf-8') + separator + (APP / name).read_bytes())
        if trailing:
            value.update(b'\0')
    return value.hexdigest()


def main():
    backend = module('file_backend_manifest', 'verify_backend_batches.py').digest()
    runtime = module('file_runtime_manifest', 'verify_agent_runtime.py').digest()[0]
    mobile = module('file_mobile_manifest', 'verify_mobile.py').digest()[0]
    checks = []
    for name, expected, success in [('backend-result.json', backend, 'passed'), ('linux-build.json', backend, 'passed'),
                                    ('runtime-build.json', runtime, 'status'), ('mobile-build.json', mobile, 'passed')]:
        report = read(EVIDENCE / name)
        passed = report[success] == 'passed' if success == 'status' else report[success] is True
        checks.append({'name': name, 'passed': passed and report['source_sha256'] == expected, 'source_sha256': expected})
    web = read(EVIDENCE / 'web-build.json')
    web_hash = digest(web['sources'])
    checks.append({'name': 'web-build.json', 'passed': web['passed'] is True and web['source_sha256'] == web_hash, 'source_sha256': web_hash})
    for suffix, separator, trailing in [('web', b'\n', False), ('mobile', b'\0', True)]:
        name = 'secretary-file-updates-' + suffix
        report = read(APP.parent / 'docs/quality/evidence' / name / 'result.json')
        current = digest(report['source_files'], separator, trailing)
        closed = all(report.get(key) is True for key in ('owned_contexts_closed', 'owned_browser_closed', 'isolated_database_cleanup_passed'))
        checks.append({'name': name, 'passed': report['passed'] is True and closed and current == report['source_sha256'], 'source_sha256': current, 'checks': len(report['checks'])})
    development = read(EVIDENCE / 'development-file-updates.json')
    checks.append({'name': 'Owned development file evidence and disabled cloud role', 'passed': development['passed'] is True
                   and development['migration_count'] == 51 and development['database']['disposable_test_databases'] == 0
                   and development['agent_file_reading_disabled_in_development_configuration'] is True
                   and development['agent_file_writing_disabled_in_development_configuration'] is True
                   and development['database']['append_only_file_tables'] is True
                   and development['database']['file_update_guards'] == 13})
    actual_script = hashlib.sha256((APP / 'scripts/verify_secretary_file_updates_development.py').read_bytes()).hexdigest()
    checks.append({'name': 'Development verification script source', 'passed': development['verification_script_sha256'] == actual_script})
    checks.append({'name': 'OpenAPI embedded and documented copies', 'passed': (APP / 'backend/internal/contract/openapi.json').read_bytes() == (APP.parent / 'docs/development/openapi.json').read_bytes()})
    checks.append({'name': 'Private browser fixture removed', 'passed': not (APP / '.local/business-browser-fixture.json').exists()})
    with socket.socket() as probe:
        probe.settimeout(1)
        checks.append({'name': 'Owned temporary review port released', 'passed': probe.connect_ex(('127.0.0.1', 5189)) != 0})
    for url in ('http://localhost:5178/', 'http://localhost:5178/health/ready', 'http://127.0.0.1:18080/health/ready'):
        try:
            with urlopen(url, timeout=5) as response:
                passed = response.status == 200
        except OSError:
            passed = False
        checks.append({'name': url, 'passed': passed})
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
              'scope': 'Fixed-source native implementation, controlled draft and formal object bytes and cleanup; excludes production, real model/cloud, devices and capacity acceptance.'}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
