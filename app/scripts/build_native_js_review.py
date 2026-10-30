"""Compile current Android JS with the existing disclosed native Windows adapter.

This does not create, sign or validate an APK. Keep the unchanged native host's
no-Expo/no-unimodules map guard, explicit build profile and actual API origin.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

import build_refund_fx_android as native_adapter

APP = Path(__file__).resolve().parents[1]


def main():
    node = APP / '.tools/node-v22.23.4-win-x64/node.exe'
    if not node.is_file() or not os.environ.get('CP_MOBILE_API_ORIGIN') or not os.environ.get('CP_MOBILE_BUILD_PROFILE'):
        raise SystemExit('Explicit native Node22, mobile build profile and API origin are required.')
    bundle = APP / 'mobile/android/app/src/main/assets/index.android.bundle'
    mapping = APP / '.local/android/index.android.map'
    bundle.parent.mkdir(parents=True, exist_ok=True)
    mapping.parent.mkdir(parents=True, exist_ok=True)
    # Match the repository native builder's explicit environment allowlist.
    # Do not forward provider credentials or unrelated parent tool settings.
    allowed = {'PATH', 'PATHEXT', 'SYSTEMROOT', 'WINDIR', 'COMSPEC', 'TEMP', 'TMP', 'APPDATA', 'LOCALAPPDATA',
        'PROGRAMFILES', 'PROGRAMFILES(X86)', 'PROGRAMDATA', 'USERPROFILE', 'HOMEDRIVE', 'HOMEPATH',
        'NUMBER_OF_PROCESSORS', 'PROCESSOR_ARCHITECTURE', 'OS'}
    environment = {**{key: value for key, value in os.environ.items() if key.upper() in allowed},
        'CP_MOBILE_API_ORIGIN': os.environ['CP_MOBILE_API_ORIGIN'], 'CP_MOBILE_BUILD_PROFILE': os.environ['CP_MOBILE_BUILD_PROFILE'],
        'TARO_ENV': 'rn', 'NODE_ENV': 'production', 'PYTHONUTF8': '1'}
    # Reuse only the published bundle adapter, never its main()/APK runner or
    # historical evidence writer. It preserves current resolver/entry sources,
    # excludes generated directories and applies the disclosed native Windows
    # filesystem bridge. The underlying four-worker Metro CLI is unchanged.
    result = native_adapter.run(['npm.cmd', 'run', 'build:rn'], cwd=APP / 'mobile', env=environment,
        stdout=sys.stdout, stderr=subprocess.STDOUT,
        creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0, timeout=900)
    if result.returncode:
        return result.returncode
    source_map = json.loads(mapping.read_text(encoding='utf-8'))
    if not bundle.is_file() or not source_map.get('sources') or any(re.search(r'[/\\]node_modules[/\\](?:expo(?:-[^/\\]+)?|@unimodules)[/\\]', name) for name in source_map['sources']):
        raise SystemExit('The current bundle/map is missing or requires an unsupported Expo native extension.')
    assert len(native_adapter.invocations) == 1 and native_adapter.invocations[0]['exit_code'] == 0
    print(json.dumps({'passed': True, 'max_workers': 4, 'no_expo_map_guard': True, 'native_invocation': native_adapter.invocations[0],
        'bundle_sha256': hashlib.sha256(bundle.read_bytes()).hexdigest(), 'source_map_sha256': hashlib.sha256(mapping.read_bytes()).hexdigest(),
        'source_count': len(source_map['sources']), 'apk_created': False, 'formal_android_acceptance': False}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
