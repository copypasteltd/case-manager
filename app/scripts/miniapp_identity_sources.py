"""Exact complete fixture, actual mobile client and capture/build helper bytes."""
import hashlib
import subprocess
from pathlib import Path

APP = Path(__file__).resolve().parents[1]


def sources():
    scopes = ['backend', 'web', 'mobile', 'packages', 'agent-runtime', 'skills', 'deploy/native',
        'package.json', 'package-lock.json', 'scripts/miniapp_identity_sources.py',
        'scripts/run_miniapp_identity_browser.py', 'scripts/check_miniapp_identity.mjs', 'scripts/miniapp_identity_recording.mjs',
        'scripts/encode_miniapp_identity_media.py', 'scripts/verify_miniapp_identity_clients.py', 'scripts/verify_miniapp_identity_backend.py', 'scripts/test_backend.py', 'scripts/mobile_picker_browser.mjs', 'scripts/notification_browser_common.mjs']
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', *scopes], cwd=APP).decode('utf-8').split('\0')
    return sorted(set(name for name in names if name and (APP / name).is_file()))



def digest(names=None):
    h = hashlib.sha256()
    for name in names or sources():
        h.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return h.hexdigest()
