"""Exact complete fixture, both actual clients and capture/build helper bytes."""
import hashlib
import subprocess
from pathlib import Path

APP = Path(__file__).resolve().parents[1]


def sources():
    scopes = ['backend', 'web', 'mobile', 'packages/contracts', 'packages/design', 'deploy/native',
        'package.json', 'package-lock.json', 'scripts/build_status_bridge.py', 'scripts/status_bridge_sources.py',
        'scripts/run_status_bridge_browser.py', 'scripts/check_status_bridge.mjs', 'scripts/status_bridge_recording.mjs',
        'scripts/test_backend.py', 'scripts/mobile_picker_browser.mjs', 'scripts/notification_browser_common.mjs']
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', *scopes], cwd=APP).decode('utf-8').split('\0')
    return sorted(set(name for name in names if name and (APP / name).is_file()))



def digest():
    h = hashlib.sha256()
    for name in sources():
        h.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return h.hexdigest()
