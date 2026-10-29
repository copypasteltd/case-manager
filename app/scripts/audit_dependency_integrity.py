"""Record the complete current audit and every unresolved package finding."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/dependency-integrity'
PUBLISHED = 'a7f323f631eb07d6d6852d3d666df8e085af02a3'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def save(name, value):
    target = E / name
    assert not target.exists(), 'Archive the earlier record before rerunning: ' + name
    target.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def main():
    assert not (E / 'audit-after.json').exists(), 'Preserve earlier audit attempts.'
    before = json.loads((E / 'audit-before.json').read_bytes())
    source = json.loads((E / 'audit-before-source.json').read_bytes())
    old_lock = subprocess.check_output(['git', 'show', PUBLISHED + ':app/package-lock.json'], cwd=ROOT)
    assert source['commit'] == PUBLISHED and sha(old_lock) == source['package_lock_sha256']
    assert sha((E / 'audit-before.json').read_bytes()) == source['raw_log_sha256'] and source['all_dependency_scopes']
    npm = Path(shutil.which('npm.cmd' if os.name == 'nt' else 'npm'))
    node = shutil.which('node')
    cli = npm.parent / 'node_modules/npm/bin/npm-cli.js'
    args = ['audit', '--package-lock-only', '--json', '--include=dev', '--include=optional', '--include=peer']
    command = [node, str(cli), *args] if os.name == 'nt' else [str(npm), *args]
    lock = (APP / 'package-lock.json').read_bytes()
    result = subprocess.run(command, cwd=APP, capture_output=True, timeout=600)
    after = json.loads(result.stdout.decode('utf-8-sig'))
    assert result.returncode in (0, 1) and not after.get('error') and 'vulnerabilities' in after.get('metadata', {})
    assert lock == (APP / 'package-lock.json').read_bytes()
    (E / 'audit-after.json').write_bytes(result.stdout)
    record = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'command': ['npm', *args],
              'exit_code': result.returncode, 'package_lock_sha256': sha(lock), 'source_unchanged': True,
              'node_version': subprocess.check_output([node, '--version']).decode().strip(),
              'npm_version': subprocess.check_output([node, str(cli), '--version']).decode().strip(),
              'vulnerabilities': after['metadata']['vulnerabilities'], 'before_vulnerabilities': before['metadata']['vulnerabilities'],
              'raw_report_sha256': sha(result.stdout), 'before_source_record_sha256': sha((E / 'audit-before-source.json').read_bytes()),
              'all_dependency_scopes_included': True, 'advisories_suppressed': False,
              'npm_audit_gate_passed': after['metadata']['vulnerabilities']['total'] == 0,
              'production_security_gate_passed': False,
              'scope': 'Full current-lock audit includes development, build, optional and peer packages. A zero advisory scan is checked with the disclosed local patches and real consumer/build evidence; it does not establish production readiness.'}
    save('dependency-audit.json', record)
    pending = []
    for index, (name, value) in enumerate(sorted(after['vulnerabilities'].items()), 1):
        pending.append({'id': f'DEP-108-{index:03}', 'name': name, 'severity': value['severity'], 'status': 'open',
                        'direct': value['isDirect'], 'nodes': value['nodes'], 'via': value['via'], 'effects': value['effects'],
                        'range': value['range'], 'fix_available': value['fixAvailable'],
                        'remaining': 'Inspect the actual consumers and fix, replace or record an approved evidence-based disposition. Development-only placement is not an exemption.'})
    save('remaining-findings.json', {'audit_record_sha256': sha((E / 'dependency-audit.json').read_bytes()),
                                  'counts': record['vulnerabilities'], 'items': pending})
    direct = [{'name': item['name'], 'severity': item['severity'], 'status': 'open',
               'nodes': item['nodes'], 'direct_consumers': item['effects'],
               'advisories': [value for value in item['via'] if isinstance(value, dict)]}
              for item in pending if any(isinstance(value, dict) for value in item['via'])]
    save('direct-advisory-packages.json', {'audit_record_sha256': sha((E / 'dependency-audit.json').read_bytes()),
         'package_findings': len(pending), 'direct_advisory_packages': len(direct),
         'propagation_only_packages': len(pending) - len(direct), 'not_unique_cve_count': True, 'items': direct})
    old = json.loads(old_lock)['packages']; new = json.loads(lock)['packages']
    save('lock-graph-delta.json', {'before_lock_sha256': sha(old_lock), 'after_lock_sha256': sha(lock),
                                'removed_locations': sorted(set(old) - set(new)), 'added_locations': sorted(set(new) - set(old)),
                                'changed_locations': sorted(k for k in set(old) & set(new) if old[k] != new[k]),
                                'removed_finding_packages': sorted(set(before['vulnerabilities']) - set(after['vulnerabilities'])),
                                'new_finding_packages': sorted(set(after['vulnerabilities']) - set(before['vulnerabilities'])),
                                'scope': 'Package-level advisory chains and installation locations; no CVE-count equivalence is implied.'})
    print(json.dumps({'audit_completed': True, 'before': record['before_vulnerabilities'],
                      'after': record['vulnerabilities'], 'security_gate_passed': False}), flush=True)


if __name__ == '__main__':
    main()
