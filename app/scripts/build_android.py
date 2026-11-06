"""Build the native Android artifact without starting a device or emulator."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
from urllib.parse import urlparse
import zipfile

from windows_native_path import native_app
from android_release import load_release, inspect_release, lock_release

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/android-native'


def sources():
    names = {'package.json', 'package-lock.json', 'mobile/package.json',
             'mobile/babel.config.js', 'mobile/metro.config.js', 'mobile/index.js',
             'mobile/react-native.config.js', 'scripts/build_android.py', 'scripts/android_release.py', 'scripts/setup_android_native.py'}
    for directory in ('mobile/src', 'mobile/config', 'mobile/native', 'mobile/android', 'packages/contracts/src', 'packages/design/src',
                      'packages/archive-extraction', 'packages/image-size-compat', 'packages/latest-version-compat', 'packages/template-download',
                      'packages/template-minifier', 'packages/dev-server-compat', 'packages/deepmerge-safe', 'packages/uri-decoder-compat'):
        # Prune the existing exclusions before walking them. Generated Gradle
        # trees and dependency folders can contain many files; none of those
        # bytes belonged to this manifest under the previous implementation.
        for current, directories, files in os.walk(APP / directory, followlinks=False):
            directories[:] = [name for name in directories if name not in ('build', '.gradle', 'node_modules', 'assets') and not name.startswith('drawable-')]
            for name in files:
                path = Path(current) / name
                if not path.is_file() or name == 'local.properties' or path.suffix in ('.jks', '.keystore', '.log'):
                    continue
                names.add(path.relative_to(APP).as_posix())
    value = hashlib.sha256()
    for name in sorted(names):
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest(), sorted(names)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--release-config', help='Private signing JSON outside the repository; builds signed release APK and AAB.')
    parser.add_argument('--bundle-only', action='store_true')
    parser.add_argument('--google-mirror', action='store_true')
    parser.add_argument('--evidence-group', default='android-native')
    args = parser.parse_args()
    if not re.fullmatch(r'[a-z][a-z0-9-]{0,63}', args.evidence_group): raise SystemExit('Invalid evidence group.')
    evidence = APP.parent / 'docs/quality/evidence' / args.evidence_group
    origin = os.environ.get('CP_MOBILE_API_ORIGIN', '')
    url = urlparse(origin)
    local = os.environ.get('CP_MOBILE_BUILD_PROFILE') == 'local'
    if not url.hostname or url.username or url.password or url.query or url.fragment or url.path not in ('', '/') or (url.scheme != 'https' and not (local and url.scheme == 'http' and url.hostname in ('127.0.0.1', 'localhost'))):
        raise SystemExit('Set an explicit HTTPS API origin; a local profile permits loopback HTTP only.')
    release = None
    if args.release_config:
        if args.bundle_only: raise SystemExit('Formal packaging requires both signed APK and AAB.')
        try: release_path, release = load_release(args.release_config, origin, local)
        except (ValueError, OSError): raise SystemExit('Private formal signing inputs are incomplete or invalid; no signing key is generated.')
    native = native_app()
    cache = native / '.local/android'
    cache.mkdir(parents=True, exist_ok=True)
    build_lock = lock_release(cache)
    jdk = sorted((native / '.tools/jdk17').glob('*/bin/java.exe'))
    if len(jdk) != 1:
        raise SystemExit('One native JDK 17 is required under app/.tools/jdk17.')
    sdk = Path(os.environ.get('CP_ANDROID_SDK_ROOT', str(Path(os.environ.get('LOCALAPPDATA', '')) / 'Android/Sdk')))
    if not (sdk / 'platforms/android-34/android.jar').is_file():
        raise SystemExit('Android SDK platform 34 is required; no emulator will be started.')
    if not (sdk / 'build-tools/34.0.0/aapt2.exe').is_file():
        raise SystemExit('The existing native Android SDK must include build-tools 34.0.0.')
    allowed = {'PATH', 'PATHEXT', 'SYSTEMROOT', 'WINDIR', 'COMSPEC', 'TEMP', 'TMP', 'APPDATA', 'LOCALAPPDATA',
               'PROGRAMFILES', 'PROGRAMFILES(X86)', 'PROGRAMDATA', 'USERPROFILE', 'HOMEDRIVE', 'HOMEPATH',
               'NUMBER_OF_PROCESSORS', 'PROCESSOR_ARCHITECTURE', 'OS'}
    env = {**{key: value for key, value in os.environ.items() if key.upper() in allowed},
           'CP_MOBILE_API_ORIGIN': origin, 'CP_MOBILE_BUILD_PROFILE': 'local' if local else 'https',
           'JAVA_HOME': str(jdk[0].parents[1]), 'ANDROID_HOME': str(sdk),
           'ANDROID_SDK_ROOT': str(sdk), 'GRADLE_USER_HOME': str(cache / 'gradle-home'),
           'TARO_ENV': 'rn', 'NODE_ENV': 'production', 'PYTHONUTF8': '1'}
    if release:
        env['CP_ANDROID_RELEASE_CONFIG'] = str(release_path)
        for key in ('store_password_env', 'key_password_env'): env[release[key]] = os.environ[release[key]]
        history = cache / 'formal-version-history.json'
        ledger = json.loads(history.read_text()) if history.exists() else {}
        if release['version_code'] <= ledger.get(release['certificate_sha256'], 0): raise SystemExit('This signing identity has already produced this or a newer formal version.')
    # Scope native tool paths to this checkout; the user's SDK is read only.
    if not args.bundle_only:
        (native / 'mobile/android/local.properties').write_text('sdk.dir=' + sdk.as_posix() + '\n', encoding='utf-8')
        keystore = cache / 'debug.keystore'
        if not release and not keystore.exists():
            command = [str(jdk[0].with_name('keytool.exe')), '-genkeypair', '-keystore', str(keystore),
                       '-storepass', 'android', '-keypass', 'android', '-alias', 'androiddebugkey',
                       '-keyalg', 'RSA', '-keysize', '2048', '-validity', '3650',
                       '-dname', 'CN=Android Debug,O=CopyPaste,C=HK', '-noprompt']
            with (cache / 'debug-signing.log').open('w', encoding='utf-8') as output:
                result = subprocess.run(command, env=env, stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW)
            if result.returncode: raise SystemExit('The checkout-local debug signing key could not be created.')
    before, names = sources()
    started = datetime.now(timezone.utc)
    phases = []
    # Metro must use the same real path as npm's workspace links. Mixing its
    # file graph with a DOS alias can make the same workspace appear twice.
    commands = [('bundle', ['npm.cmd', 'run', 'build:rn'], APP / 'mobile')]
    if not args.bundle_only:
        command = ['cmd.exe', '/d', '/c', 'gradlew.bat', '--no-daemon', '--console=plain']
        if args.google_mirror: command += ['--init-script', str(native / 'mobile/native/google-mirror.init.gradle')]
        command += [':app:assembleRelease', ':app:bundleRelease'] if release else [':app:testDebugUnitTest', ':app:assembleDebug']
        commands.append(('native', command, native / 'mobile/android'))
    for name, command, cwd in commands:
        log = cache / (name + '.log')
        with log.open('w', encoding='utf-8') as output:
            result = subprocess.run(command, cwd=cwd, env=env, stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
        phases.append({'name': name, 'passed': result.returncode == 0, 'exit_code': result.returncode})
        print(json.dumps(phases[-1]), flush=True)
        if result.returncode:
            print('Native build stopped; inspect the owned local phase log.')
            return 1
    bundle = APP / 'mobile/android/app/src/main/assets/index.android.bundle'
    apk = APP / ('mobile/android/app/build/outputs/apk/release/app-release.apk' if release else 'mobile/android/app/build/outputs/apk/debug/app-debug.apk')
    aab = APP / 'mobile/android/app/build/outputs/bundle/release/app-release.aab'
    unchanged = before == sources()[0]
    artifacts = {}
    for label, file in [('bundle', bundle)] + ([] if args.bundle_only else ([('apk', apk), ('aab', aab)] if release else [('apk', apk)])):
        if not file.is_file() or (label == 'bundle' and file.stat().st_mtime < started.timestamp()):
            raise SystemExit('The expected native artifact was not produced.')
        artifacts[label] = {'path': file.relative_to(APP).as_posix(), 'bytes': file.stat().st_size, 'sha256': hashlib.sha256(file.read_bytes()).hexdigest()}
    if not args.bundle_only:
        with zipfile.ZipFile(apk) as package:
            if hashlib.sha256(package.read('assets/index.android.bundle')).hexdigest() != artifacts['bundle']['sha256']:
                raise SystemExit('The APK does not contain the current frozen business bundle.')
    if not args.bundle_only:
        # Preserve each inspected APK independently of Gradle's mutable output.
        saved = APP / '.local/android/artifacts' / args.evidence_group / artifacts['apk']['sha256'] / ('app-release.apk' if release else 'app-debug.apk')
        saved.parent.mkdir(parents=True, exist_ok=True)
        if saved.exists() and hashlib.sha256(saved.read_bytes()).hexdigest() != artifacts['apk']['sha256']:
            raise SystemExit('An existing immutable artifact differs; use a new source or evidence group.')
        if not saved.exists(): shutil.copyfile(apk, saved)
        artifacts['apk']['gradle_path'] = artifacts['apk']['path']
        artifacts['apk']['path'] = saved.relative_to(APP).as_posix()
    signing = None
    if release:
        signing = inspect_release(apk, aab, artifacts['bundle']['sha256'], release, sdk, jdk[0].parent, env)
        saved_aab = cache / 'artifacts' / args.evidence_group / artifacts['aab']['sha256'] / 'app-release.aab'
        saved_aab.parent.mkdir(parents=True, exist_ok=True)
        if not saved_aab.exists(): shutil.copyfile(aab, saved_aab)
        artifacts['aab']['gradle_path'] = artifacts['aab']['path']
        artifacts['aab']['path'] = saved_aab.relative_to(native).as_posix()
        if unchanged and all(p['passed'] for p in phases):
            ledger[release['certificate_sha256']] = release['version_code']
            temporary = history.with_suffix('.tmp')
            temporary.write_text(json.dumps(ledger, sort_keys=True)+'\n')
            os.replace(temporary, history)
    evidence.mkdir(parents=True, exist_ok=True)
    report = {'passed': unchanged and all(p['passed'] for p in phases), 'checked_at': datetime.now(timezone.utc).isoformat(),
              'source_sha256': before, 'sources': names, 'source_unchanged_during_build': unchanged,
              'phases': phases, 'artifacts': artifacts, 'profile': 'local' if local else 'https',
              'google_maven_source': 'aliyun-google-mirror' if args.google_mirror else 'google-official',
              'emulator_started': False, 'virtualization_used': False, 'device_executed': False, 'production_release': False, 'formal_signing': signing, 'formal_build': bool(release)}
    (evidence / ('bundle.json' if args.bundle_only else 'build.json')).write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'artifacts': list(artifacts)}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
