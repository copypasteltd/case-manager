"""Build the current development APK with ordinary validated Metro caches.

Four native Windows workers and the checkout-only filesystem adapter remain.
Only the unconditional cache reset is removed; the actual command, current map,
embedded JS/APK, signature and suite outcomes are still inspected independently.
No emulator/device is started and cached JVM results are labelled explicitly.
"""
import json
import os
import subprocess
import sys
import xml.etree.ElementTree as ET
import build_android as base
import build_refund_fx_android as adapter

APP = base.APP
system_run = subprocess.run


def cached_run(command, *args, **kwargs):
    if isinstance(command, list) and 'bundle' in command and '--reset-cache' in command:
        command.remove('--reset-cache')
    return system_run(command, *args, **kwargs)


def run(command, *args, **kwargs):
    if command == ['npm.cmd', 'run', 'build:rn']:
        bridge = APP / '.local/android/refund-fx-fs-diagnostic.cjs'
        kwargs['env'] = {**kwargs['env'], 'NODE_OPTIONS': '--require="' + bridge.as_posix() + '"'}
    return adapter.run(command, *args, **kwargs)


def main():
    if len(sys.argv) != 1:
        raise SystemExit('Build the complete known-original-retirement development APK without extra arguments.')
    temp = APP / '.local/android/office-authoring-metro-temp'
    temp.mkdir(parents=True, exist_ok=True)
    os.environ.update({'TEMP': str(temp), 'TMP': str(temp)})
    adapter.original_run = cached_run
    subprocess.run = run
    sys.argv += ['--evidence-group', 'object-retirement', '--google-mirror']
    try:
        result = base.main()
    finally:
        subprocess.run = system_run
        adapter.original_run = system_run
    if result:
        return result
    native = base.native_app()
    jdk = next((native / '.tools/jdk17').glob('*/bin/java.exe')).parents[1]
    env = {**os.environ, 'JAVA_HOME': str(jdk), 'GRADLE_USER_HOME': str(native / '.local/android/gradle-home')}
    log = APP / '.local/android/object-retirement-jvm.log'
    with log.open('wb') as output:
        r = system_run(['cmd.exe', '/d', '/c', 'gradlew.bat', '--no-daemon', '--console=plain', '-p', '../native/jvm-tests', 'test'], cwd=native / 'mobile/android', env=env, stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW)
    if r.returncode:
        return r.returncode
    suites = {}
    for label, folder in [('jvm', 'mobile/native/jvm-tests/build/test-results/test'), ('android', 'mobile/android/app/build/test-results/testDebugUnitTest')]:
        rows = [ET.parse(p).getroot() for p in (APP / folder).glob('TEST-*.xml')]
        suites[label] = {key: sum(int(row.get(key, '0')) for row in rows) for key in ('tests', 'failures', 'errors', 'skipped')}
    assert all(v['tests'] == 28 and v['failures'] == v['errors'] == v['skipped'] == 0 for v in suites.values())
    report = {'passed': True, 'suites': suites, 'native_invocations': adapter.invocations, 'apk_created': True, 'development_only': True, 'formal_android_acceptance': False, 'real_device_used': False, 'virtualization_used': False, 'metro_cache_reset': False, 'suite_execution_scope': 'Normal Gradle incremental execution; actual EXECUTED/UP-TO-DATE outcomes and original XML timestamps are retained separately.'}
    (APP.parent / 'docs/quality/evidence/object-retirement/native-suites.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(report))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
