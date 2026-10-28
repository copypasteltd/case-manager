"""Record complete before/after feeds and every unresolved package finding."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/dependency-remediation'
PUBLISHED = '52899b6531d118b02770d2a40842d4e56622872c'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def save(name, value):
    (E / name).write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def main():
    E.mkdir(parents=True, exist_ok=True)
    assert not (E / 'audit-after.json').exists(), 'Archive previous audit before rerunning.'
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    assert npm
    old_lock = subprocess.check_output(['git', 'show', PUBLISHED + ':app/package-lock.json'], cwd=ROOT)
    assert sha(old_lock) == 'a14002ac9152073560ee7a3256c5752cd8cb2ff5e5da3c15a7582ee8ff3b6f44'
    before_raw = (APP / '.local/dependency-remediation-audit-before.json').read_bytes()
    before = json.loads(before_raw)
    assert before['metadata']['vulnerabilities']['total'] == 80
    (E / 'audit-before.json').write_bytes(before_raw)
    save('audit-before-source.json', {'published_commit': PUBLISHED, 'package_lock_sha256': sha(old_lock),
         'raw_report_sha256': sha(before_raw), 'command': ['npm', 'audit', '--json'],
         'scope': 'Fresh full-scope audit before the first source/lock edit in increment103. Raw report and published lock are retained; no omitted dependency scope.'})
    lock = (APP / 'package-lock.json').read_bytes()
    args = ['audit', '--json', '--include=dev', '--include=optional', '--include=peer']
    # Own the real process rather than only npm.cmd's short-lived wrapper.
    node = shutil.which('node')
    cli = Path(npm).parent / 'node_modules/npm/bin/npm-cli.js'
    command = [node, str(cli), *args] if os.name == 'nt' else [npm, *args]
    assert node and (os.name != 'nt' or cli.is_file())
    try:
        result = subprocess.run(command, cwd=APP, capture_output=True, timeout=600)
    except subprocess.TimeoutExpired:
        name = 'audit-timeout-' + datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%S%f') + '.json'
        save(name, {'passed': False, 'audit_completed': False, 'command': ['npm', *args],
                    'package_lock_sha256': sha(lock), 'reason': 'Technical command deadline exceeded; no partial result accepted.'})
        raise SystemExit('Full audit did not complete; failed attempt retained.')
    after = json.loads(result.stdout.decode('utf-8-sig'))
    assert result.returncode in (0, 1) and not after.get('error') and 'vulnerabilities' in after.get('metadata', {})
    assert lock == (APP / 'package-lock.json').read_bytes()
    (E / 'audit-after.json').write_bytes(result.stdout)
    record = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(),
              'command': ['npm', *args], 'exit_code': result.returncode,
              'npm_version': subprocess.check_output([npm, '--version'], cwd=APP).decode().strip(),
              'package_lock_sha256': sha(lock), 'source_unchanged': True,
              'vulnerabilities': after['metadata']['vulnerabilities'], 'before_vulnerabilities': before['metadata']['vulnerabilities'],
              'raw_report': 'audit-after.json', 'raw_report_sha256': sha(result.stdout),
              'before_source_record': 'audit-before-source.json', 'before_source_record_sha256': sha((E / 'audit-before-source.json').read_bytes()),
              'all_dependency_scopes_included': True, 'advisories_suppressed': False,
              'production_security_gate_passed': False,
              'scope': 'Complete current-lock scan including build/development/optional/peer dependencies. A completed scan does not close GAP-26.'}
    save('dependency-audit.json', record)
    pending = []
    for index, (name, finding) in enumerate(sorted(after['vulnerabilities'].items()), 1):
        pending.append({'id': f'DEP-103-{index:03}', 'name': name, 'severity': finding['severity'],
                        'status': 'open', 'direct': finding['isDirect'], 'nodes': finding['nodes'],
                        'via': finding['via'], 'effects': finding['effects'], 'range': finding['range'],
                        'fix_available': finding['fixAvailable'],
                        'remaining': 'Review concrete callers and advisory reachability; apply a compatible fix or record an approved evidence-based disposition. Development-only placement is not an exemption.'})
    save('remaining-findings.json', {'audit_record_sha256': sha((E / 'dependency-audit.json').read_bytes()), 'counts': record['vulnerabilities'], 'items': pending})
    old = json.loads(old_lock)['packages']; new = json.loads(lock)['packages']
    removed = sorted(set(old) - set(new)); added = sorted(set(new) - set(old))
    changed = sorted(k for k in set(old) & set(new) if old[k] != new[k])
    save('lock-graph-delta.json', {'before_lock_sha256': sha(old_lock), 'after_lock_sha256': sha(lock),
         'removed_locations': removed, 'added_locations': added, 'changed_locations': changed,
         'scope': 'Installation locations, not unique vulnerabilities. Local adapters retain their own package names; upstream aliases and versions remain visible to audit.'})
    print(json.dumps({'audit_completed': True, 'before': record['before_vulnerabilities'], 'after': record['vulnerabilities'], 'security_gate_passed': False}))


if __name__ == '__main__':
    main()
