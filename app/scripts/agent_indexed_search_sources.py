"""Bind the complete file search implementation and its owned capture helpers."""
import hashlib
from pathlib import Path
import subprocess

APP = Path(__file__).resolve().parents[1]


def sources():
    scopes = ['backend', 'web', 'mobile', 'agent-runtime', 'skills', 'packages',
        'package.json', 'package-lock.json', 'scripts/agent_indexed_search_sources.py',
        'scripts/run_agent_indexed_search_browser.py', 'scripts/check_agent_indexed_search.mjs',
        'scripts/agent_indexed_search_recording.mjs', 'scripts/mobile_picker_browser.mjs',
        'scripts/notification_browser_common.mjs', 'scripts/test_backend.py']
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', *scopes], cwd=APP).decode('utf-8').split('\0')
    return sorted(set(name for name in names if name and (APP / name).is_file()))


def digest(names=None):
    h = hashlib.sha256()
    for name in names or sources():
        h.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return h.hexdigest()
