"""Exact inputs for dependency maintenance; retained UI evidence keeps its own source scope."""
import hashlib
from pathlib import Path
import subprocess

APP = Path(__file__).resolve().parents[1]


def sources():
    scopes = ['backend', 'web', 'mobile', 'packages', 'agent-runtime', 'skills', 'deploy/native',
        'package.json', 'package-lock.json', 'scripts/dependency_security.test.mjs',
        'scripts/dependency_maintenance.test.mjs', 'scripts/dependency_maintenance_sources.py',
        'scripts/verify_dependency_maintenance_security.py', 'scripts/verify_dependency_maintenance_clients.py',
        'scripts/freeze_dependency_maintenance_clients.py', 'scripts/freeze_dependency_maintenance_android.py',
        'scripts/verify_dependency_maintenance_development.py', 'scripts/verify_dependency_maintenance_sources.py',
        'scripts/build_android.py', 'scripts/inspect_android_apk.py', 'scripts/verify_mobile.py']
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', *scopes], cwd=APP).decode().split('\0')
    return sorted(set(name for name in names if name and (APP / name).is_file()))


def digest(names=None):
    result = hashlib.sha256()
    for name in names or sources():
        result.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return result.hexdigest()
