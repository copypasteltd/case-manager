"""Run the actual Android/JVM boundary suites using native Windows tools only."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import xml.etree.ElementTree as ET
from windows_native_path import native_app

APP = Path(__file__).resolve().parents[1]


def sources():
    names = sorted(path.relative_to(APP).as_posix() for directory in (
        'mobile/android/app/src/main/java', 'mobile/android/app/src/test/java', 'mobile/native/jvm-tests',
    ) for path in (APP / directory).rglob('*') if path.is_file() and not any(part in ('build', '.gradle') for part in path.relative_to(APP).parts))
    value = hashlib.sha256()
    for name in names: value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest(), names


def main():
    native = native_app()
    jdk = list((native / '.tools/jdk17').glob('*/bin/java.exe'))
    if len(jdk) != 1: raise SystemExit('The pinned native JDK is required.')
    allowed = {'PATH', 'PATHEXT', 'SYSTEMROOT', 'WINDIR', 'COMSPEC', 'TEMP', 'TMP', 'APPDATA', 'LOCALAPPDATA', 'USERPROFILE', 'HOMEDRIVE', 'HOMEPATH', 'NUMBER_OF_PROCESSORS', 'PROCESSOR_ARCHITECTURE', 'OS'}
    sdk = Path(os.environ['LOCALAPPDATA']) / 'Android/Sdk'
    env = {**{k: v for k, v in os.environ.items() if k.upper() in allowed}, 'JAVA_HOME': str(jdk[0].parents[1]),
           'ANDROID_HOME': str(sdk), 'ANDROID_SDK_ROOT': str(sdk), 'GRADLE_USER_HOME': str(native / '.local/android/gradle-home'),
           'CP_MOBILE_API_ORIGIN': 'http://127.0.0.1:18080', 'CP_MOBILE_BUILD_PROFILE': 'local'}
    before, names = sources()
    commands = [
        ('jvm', ['cmd.exe', '/d', '/c', 'gradlew.bat', '--no-daemon', '--console=plain', '-p', '../native/jvm-tests', 'test']),
        ('android', ['cmd.exe', '/d', '/c', 'gradlew.bat', '--no-daemon', '--console=plain', '--init-script', str(native / 'mobile/native/google-mirror.init.gradle'), ':app:testDebugUnitTest']),
    ]
    phases = []
    for name, command in commands:
        with (native / ('.local/android/upload-' + name + '-tests.log')).open('w', encoding='utf-8') as output:
            result = subprocess.run(command, cwd=native / 'mobile/android', env=env, stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW)
        phase = {'name': name, 'exit_code': result.returncode, 'passed': result.returncode == 0}
        phases.append(phase); print(json.dumps(phase), flush=True)
        if result.returncode: return 1
    suites = {}
    for name, folder in [('jvm', 'mobile/native/jvm-tests/build/test-results/test'), ('android', 'mobile/android/app/build/test-results/testDebugUnitTest')]:
        values = [ET.parse(path).getroot() for path in (APP / folder).glob('TEST-*.xml')]
        suites[name] = {key: sum(int(value.get(key, '0')) for value in values) for key in ('tests', 'failures', 'errors', 'skipped')}
    passed = before == sources()[0] and all(v['tests'] == 25 and all(v[k] == 0 for k in ('failures', 'errors', 'skipped')) for v in suites.values())
    report = {'passed': passed, 'checked_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': before, 'sources': names, 'phases': phases, 'suites': suites, 'device_executed': False, 'virtualization_used': False}
    target = APP.parent / 'docs/quality/evidence/android-upload/java-tests.json'; target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': passed, 'suites': suites}))
    return 0 if passed else 1


if __name__ == '__main__': raise SystemExit(main())
