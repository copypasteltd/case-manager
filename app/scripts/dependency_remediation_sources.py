"""Complete source inputs for dependency remediation and its retained evidence."""
import hashlib
from pathlib import Path
import subprocess

APP = Path(__file__).resolve().parents[1]


def sources():
    scopes = ['backend', 'web', 'mobile', 'packages', 'agent-runtime', 'skills', 'deploy/native',
              'package.json', 'package-lock.json', 'scripts/dependency_security.test.mjs',
              'scripts/dependency_maintenance.test.mjs', 'scripts/dependency_remediation.test.mjs',
              'scripts/dependency_remediation_sources.py', 'scripts/verify_dependency_remediation_security.py',
              'scripts/audit_dependency_remediation.py', 'scripts/retain_dependency_remediation_evidence.py',
              'scripts/verify_dependency_remediation_clients.py', 'scripts/freeze_dependency_remediation_clients.py',
              'scripts/freeze_dependency_remediation_android.py', 'scripts/verify_dependency_remediation_sources.py',
              'scripts/verify_dependency_remediation_development.py', 'scripts/build_android.py',
              'scripts/inspect_android_apk.py', 'scripts/verify_mobile.py']
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', *scopes], cwd=APP).decode().split('\0')
    return sorted(set(name for name in names if name and (APP / name).is_file()))


def digest(names=None):
    value = hashlib.sha256()
    for name in names or sources():
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()
