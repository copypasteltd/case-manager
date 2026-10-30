"""Bind exact complete-contract-body sources and owned capture helpers."""
import hashlib
from pathlib import Path
import subprocess

APP = Path(__file__).resolve().parents[1]


def sources():
    scopes = ['backend', 'web', 'mobile', 'agent-runtime', 'skills', 'packages/contracts', 'packages/design',
        'package.json', 'package-lock.json', 'scripts/contract_body_sources.py',
        'scripts/run_contract_body_browser.py', 'scripts/check_contract_body.mjs',
        'scripts/contract_body_recording.mjs', 'scripts/mobile_picker_browser.mjs', 'scripts/project_concern_picker.mjs', 'scripts/build_openapi.py',
        'scripts/contract_body_browser_common.mjs', 'scripts/test_backend.py',
        'scripts/test_contract_body_native.py', 'scripts/contract_body_native_cluster.py', 'scripts/windows_native_path.py']
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', *scopes], cwd=APP).decode('utf-8').split('\0')
    return sorted(set(name for name in names if name and (APP / name).is_file()))


def digest(names=None):
    h = hashlib.sha256()
    for name in names or sources():
        h.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return h.hexdigest()
