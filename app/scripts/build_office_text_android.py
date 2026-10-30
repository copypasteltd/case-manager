"""Run the unchanged native build with disclosed single-worker Metro indexing.

Only the npm bundle subprocess is replaced by the identical underlying Metro
CLI with max-workers 1. Preserve the native host's no-Expo source-map guard.
Application inputs, Gradle commands, signature/profile and APK checks are intact.
"""
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import sys

import build_android as base

APP = base.APP
original_run = subprocess.run
invocations = []


def run(command, *args, **kwargs):
    if command == ['npm.cmd', 'run', 'build:rn'] and Path(kwargs['cwd']).resolve() == APP / 'mobile':
        executable = shutil.which('node.exe')
        if not executable:
            raise RuntimeError('The checkout native Node22 must be on PATH.')
        node = Path(executable).resolve()
        if node.parent != (APP / '.tools/node-v22.23.4-win-x64').resolve():
            raise RuntimeError('Use the checkout native Node22 for this scoped build.')
        (APP / 'mobile/android/app/src/main/assets').mkdir(parents=True, exist_ok=True)
        actual = [str(node), str(APP / 'node_modules/react-native/cli.js'), 'bundle',
            '--platform', 'android', '--dev', 'false', '--entry-file', 'index.js', '--max-workers', '1', '--reset-cache',
            '--bundle-output', 'android/app/src/main/assets/index.android.bundle',
            '--assets-dest', 'android/app/src/main/res', '--sourcemap-output', '../.local/android/index.android.map']
        result = original_run(actual, *args, **kwargs)
        invocations.append({'command': actual[2:], 'max_workers': 1, 'exit_code': result.returncode})
        if result.returncode == 0:
            mapping = json.loads((APP / '.local/android/index.android.map').read_text(encoding='utf-8'))
            if any(re.search(r'[/\\]node_modules[/\\](?:expo(?:-[^/\\]+)?|@unimodules)[/\\]', name) for name in mapping['sources']):
                raise RuntimeError('The bundle requires an Expo native extension; update and validate the native host.')
        return result
    return original_run(command, *args, **kwargs)


def main():
    if '--evidence-group' in sys.argv or '--bundle-only' in sys.argv:
        raise SystemExit('This wrapper is limited to the complete office-text development APK.')
    subprocess.run = run
    try:
        sys.argv += ['--evidence-group', 'office-text']
        result = base.main()
    finally:
        subprocess.run = original_run
    if result == 0:
        assert len(invocations) == 1 and invocations[0]['exit_code'] == 0
        evidence = APP.parent / 'docs/quality/evidence/office-text/build.json'
        record = json.loads(evidence.read_text(encoding='utf-8'))
        record['scoped_bundle_invocation'] = invocations[0]
        record['scoped_build_script'] = 'app/scripts/build_office_text_android.py'
        record['scoped_build_script_sha256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
        record['bundle_source_map_guard_applied'] = True
        record['source_map_guard'] = 'Same no-Expo/no-unimodules rule as mobile/native/build-bundle.cjs.'
        evidence.write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
    return result


if __name__ == '__main__':
    raise SystemExit(main())
