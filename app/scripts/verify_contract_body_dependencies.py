"""Audit the current lock after declaring an already-resolved hash dependency."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/contract-body'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    assert not (E / 'dependency-audit.json').exists(), 'Preserve an existing completed audit.'
    source = sha(APP / 'package-lock.json')
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    assert npm
    command = ['audit', '--package-lock-only', '--json', '--include=dev', '--include=optional', '--include=peer']
    raw = APP / '.local/contract-body-dependency-audit.json'
    flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
    with raw.open('x', encoding='utf-8') as output:
        result = subprocess.run([npm, *command], cwd=APP, stdout=output, stderr=subprocess.STDOUT, timeout=240, creationflags=flags)
    actual = json.loads(raw.read_text(encoding='utf-8'))
    counts = actual['metadata']['vulnerabilities']
    report = {'passed': result.returncode == 0 and counts['total'] == 0 and source == sha(APP / 'package-lock.json'),
        'checked_at': datetime.now(timezone.utc).isoformat(), 'command': ['npm', *command], 'exit_code': result.returncode,
        'package_lock_sha256': source, 'source_unchanged': source == sha(APP / 'package-lock.json'),
        'node_version': subprocess.check_output(['node', '--version'], text=True).strip(),
        'npm_version': subprocess.check_output([npm, '--version'], text=True, creationflags=flags).strip(),
        'vulnerabilities': counts, 'raw_report_sha256': sha(raw),
        'all_dependency_scopes_included': True, 'advisories_suppressed': False,
        'npm_audit_gate_passed': result.returncode == 0 and counts['total'] == 0,
        'production_security_gate_passed': False,
        'scope': 'Fresh current-lock advisory scan. The contracts workspace now explicitly declares the already-resolved @noble/hashes 2.4.0. No previous dependency evidence is relabelled as a new execution; formal artifact security remains GAP-26.'}
    (E / 'dependency-audit.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'advisories': counts['total'], 'production_security_gate_passed': False}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
