"""Record the complete npm audit against exact lock bytes, including unresolved findings."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/dependency-hardening'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def main():
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    target = EVIDENCE / 'audit-after.json'
    if target.exists():
        raise SystemExit('Preserve the existing audit attempt before rerunning.')
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    if not npm:
        raise SystemExit('Native npm is required.')
    lock = (APP / 'package-lock.json').read_bytes()
    process = subprocess.run([npm, 'audit', '--json'], cwd=APP, capture_output=True, timeout=180)
    raw = process.stdout
    value = json.loads(raw.decode('utf-8-sig'))
    if process.returncode not in (0, 1) or value.get('error') or 'vulnerabilities' not in value.get('metadata', {}):
        raise SystemExit('Audit did not return a complete advisory result; no success evidence written.')
    assert lock == (APP / 'package-lock.json').read_bytes(), 'Lock changed during audit.'
    target.write_bytes(raw)
    before = json.loads((EVIDENCE / 'audit-before.json').read_text(encoding='utf-8-sig'))
    before_source = json.loads((EVIDENCE / 'audit-before-source.json').read_text(encoding='utf-8-sig'))
    result = {
        'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(),
        'command': ['npm', 'audit', '--json'], 'exit_code': process.returncode,
        'npm_version': subprocess.check_output([npm, '--version'], cwd=APP).decode().strip(),
        'package_lock_sha256': sha(lock), 'source_unchanged': True,
        'vulnerabilities': value['metadata']['vulnerabilities'],
        'before_vulnerabilities': before['metadata']['vulnerabilities'],
        'before_source_record': 'audit-before-source.json', 'before_source_record_sha256': sha((EVIDENCE / 'audit-before-source.json').read_bytes()),
        'raw_report': 'audit-after.json', 'raw_report_sha256': sha(raw),
        'all_dependency_scopes_included': True, 'advisories_suppressed': False,
        'production_security_gate_passed': False,
        'scope': 'Complete audit of the current lock, including build/development dependencies. A completed audit is not an accepted security gate; remaining findings require reachability review and remediation. Changes from the older increment65 feed are not remediation.',
        'before_source': before_source,
    }
    (EVIDENCE / 'dependency-audit.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'completed': True, 'before': result['before_vulnerabilities'], 'after': result['vulnerabilities'], 'production_security_gate_passed': False}))


if __name__ == '__main__':
    main()
