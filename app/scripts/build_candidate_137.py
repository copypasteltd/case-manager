"""Build the four-client candidate inputs natively; never start a device/browser.

Server packaging uses build_production_release.py separately. This wrapper reuses
the disclosed Windows Metro adapter, preserves caches and records actual Gradle
outcomes. It never writes an earlier increment's evidence.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time
import xml.etree.ElementTree as ET
import zipfile

import build_android as android
import build_refund_fx_android as adapter
from retirement_reauthorization_sources import sources, digest

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/release-candidate-137'
LOCAL = APP / '.local/frozen-releases/v0.1.0-rc.2-clients-cache'
VERSION = 'v0.1.0-rc.2'
SOURCE = '40cd118634117204e2e310395fac32b59af5cecc'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()


def write(name, value):
    target = E / name
    if target.exists():
        raise RuntimeError('Archive the previous attempt before repeating this check.')
    target.write_text(json.dumps(value, indent=2) + '\n', encoding='utf-8')


def main():
    assert not LOCAL.exists(), 'Use a new, explicitly archived attempt.'
    assert subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip() == SOURCE
    LOCAL.mkdir(parents=True)
    E.mkdir(parents=True, exist_ok=True)
    node = APP / '.tools/node-v22.23.4-win-x64/node.exe'
    env = {**os.environ, 'PATH': str(node.parent) + os.pathsep + os.environ.get('PATH', ''),
        'CI': 'true', 'TARO_CHECK_UPDATE': 'false', 'CP_MOBILE_BUILD_PROFILE': 'local',
        'CP_MOBILE_API_ORIGIN': 'http://127.0.0.1:18080'}
    assert subprocess.check_output([str(node), '--version']).decode().strip() == 'v22.23.4'
    names = sources(['mobile', 'packages', 'package.json', 'package-lock.json'])
    before = digest(names)
    phases = []
    for target in ('weapp', 'tt'):
        log = LOCAL / (target + '.log')
        began = time.monotonic()
        with log.open('xb') as stream:
            result = subprocess.run([str(node.with_name('npm.cmd')), 'run', 'build:' + target],
                cwd=APP, env=env, stdout=stream, stderr=subprocess.STDOUT,
                creationflags=subprocess.CREATE_NO_WINDOW)
        row = {'target': target, 'exit_code': result.returncode,
            'elapsed_seconds': round(time.monotonic() - began, 3), 'private_log_sha256': sha(log)}
        phases.append(row)
        print(json.dumps(row), flush=True)
        if result.returncode:
            write('miniapps-build.json', {'passed': False, 'phases': phases})
            return 1
    mini = LOCAL / ('case-manager-' + VERSION + '-miniapps.zip')
    files = []
    with zipfile.ZipFile(mini, 'x', zipfile.ZIP_DEFLATED) as archive:
        for target in ('weapp', 'tt'):
            for path in sorted((APP / 'mobile/dist' / target).rglob('*')):
                assert not path.is_symlink()
                if not path.is_file():
                    continue
                name = target + '/' + path.relative_to(APP / 'mobile/dist' / target).as_posix()
                info = zipfile.ZipInfo(name, (1980, 1, 1, 0, 0, 0))
                info.compress_type = zipfile.ZIP_DEFLATED
                archive.writestr(info, path.read_bytes())
                files.append({'file': name, 'bytes': path.stat().st_size, 'sha256': sha(path)})
    assert before == digest(names)
    write('miniapps-build.json', {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(),
        'source_commit': SOURCE, 'source_files': names, 'source_sha256': before,
        'source_unchanged': True, 'phases': phases, 'files': files,
        'archive': mini.relative_to(ROOT).as_posix(), 'archive_sha256': sha(mini),
        'api_origin': env['CP_MOBILE_API_ORIGIN'], 'profile': 'local', 'node': 'v22.23.4',
        'wrapper_sha256': sha(Path(__file__)), 'platform_published': False})

    # Reuse actual source resolution and native filesystem results. Only retain
    # ordinary Metro caches; no source, test result or provider result is mocked.
    actual_run = subprocess.run
    previous_adapter_run = adapter.original_run
    previous_argv = sys.argv[:]
    old_environment = {key: os.environ.get(key) for key in ('PATH', 'CP_MOBILE_BUILD_PROFILE', 'CP_MOBILE_API_ORIGIN', 'TEMP', 'TMP')}
    adapter.invocations.clear()

    def cached_run(command, *args, **kwargs):
        if isinstance(command, list) and 'bundle' in command and '--reset-cache' in command:
            command = [part for part in command if part != '--reset-cache']
        return actual_run(command, *args, **kwargs)

    def native_run(command, *args, **kwargs):
        if command == ['npm.cmd', 'run', 'build:rn']:
            bridge = APP / '.local/android/refund-fx-fs-diagnostic.cjs'
            kwargs['env'] = {**kwargs['env'], 'NODE_OPTIONS': '--require="' + bridge.as_posix() + '"'}
        return adapter.run(command, *args, **kwargs)

    try:
        os.environ.update({key: env[key] for key in ('PATH', 'CP_MOBILE_BUILD_PROFILE', 'CP_MOBILE_API_ORIGIN')})
        # Keep the validated129–133 Metro cache location; a fresh TEMP would
        # force a full dependency index despite not requesting cache reset.
        temp = APP / '.local/android/office-authoring-metro-temp'
        temp.mkdir(parents=True, exist_ok=True)
        os.environ.update({'TEMP': str(temp), 'TMP': str(temp)})
        adapter.original_run = cached_run
        subprocess.run = native_run
        sys.argv = [str(Path(__file__)), '--evidence-group', 'release-candidate-137', '--google-mirror']
        result = android.main()
    finally:
        subprocess.run = actual_run
        adapter.original_run = previous_adapter_run
        sys.argv = previous_argv
        for key, value in old_environment.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
    if result:
        return result
    assert before == digest(names) and len(adapter.invocations) == 1
    raw = adapter.invocations[0]
    log = APP / '.local/android/native.log'
    text = log.read_text(encoding='utf-8', errors='replace')
    tasks = re.findall(r'^> Task :app:testDebugUnitTest(?: .*)?$', text, re.M)
    assert len(tasks) == 1
    xml = sorted((APP / 'mobile/android/app/build/test-results/testDebugUnitTest').glob('TEST-*.xml'))
    suites = [ET.parse(p).getroot() for p in xml]
    assert sum(int(s.get('tests')) for s in suites) == 28
    assert all(int(s.get(k)) == 0 for s in suites for k in ('failures', 'errors', 'skipped'))
    shutil.copyfile(log, E / 'native-build.log')
    for path in xml:
        target = E / 'android-unit-results' / path.name
        target.parent.mkdir(exist_ok=True)
        shutil.copyfile(path, target)
    write('android-build-scope.json', {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(),
        'source_commit': SOURCE, 'source_sha256': before, 'source_unchanged': True,
        'source_files': names, 'wrapper_sha256': sha(Path(__file__)),
        'adapter_sha256': sha(APP / 'scripts/build_refund_fx_android.py'),
        'base_builder_sha256': sha(APP / 'scripts/build_android.py'),
        'actual_cache_reset': False, 'adapter_declared_command_includes_cache_reset': True,
        'max_workers': raw['max_workers'], 'derived_config_sha256': raw['derived_config_sha256'],
        'filesystem_bridge_sha256': raw['windows_checkout_link_bridge_sha256'],
        'map_guard': 'No Expo/unimodules; current resolver and actual native filesystem results.',
        'task': tasks[0], 'cached': 'UP-TO-DATE' in tasks[0] or 'FROM-CACHE' in tasks[0],
        'xml': [{'file': p.relative_to(ROOT).as_posix(), 'sha256': sha(p),
            'original_suite_timestamp': ET.parse(p).getroot().get('timestamp')} for p in (E / 'android-unit-results').glob('TEST-*.xml')],
        'test_count': 28, 'standalone_jvm_rerun': False, 'real_device': False, 'virtualization': False})
    print(json.dumps({'passed': True, 'miniapp_files': len(files), 'android_task': tasks[0]}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
