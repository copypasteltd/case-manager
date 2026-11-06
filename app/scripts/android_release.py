"""Private, explicit Android release inputs and independently inspected artifacts."""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import zipfile
import atexit
from urllib.parse import urlparse

APP = Path(__file__).resolve().parents[1]


def lock_release(cache):
    """Hold an OS lock across bundle, signing, verification and version publication."""
    handle = (cache / 'android-build.lock').open('a+b')
    if handle.seek(0, 2) == 0:
        handle.write(b'0')
        handle.flush()
    handle.seek(0)
    try:
        if os.name == 'nt':
            import msvcrt
            msvcrt.locking(handle.fileno(), msvcrt.LK_NBLCK, 1)
        else:
            import fcntl
            fcntl.flock(handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
    except OSError:
        handle.close()
        raise ValueError('Another controlled Android build owns the artifact/version ledger.') from None
    atexit.register(handle.close)
    return handle


def load_release(path, origin, local, environ=None):
    environ = os.environ if environ is None else environ
    config_path = Path(path).resolve(strict=True)
    url=urlparse(origin)
    if config_path.is_relative_to(APP.parent) or local or url.scheme!='https' or not url.hostname or url.hostname in ('localhost','127.0.0.1','::1') or url.username or url.password or url.query or url.fragment or url.path not in ('','/'):
        raise ValueError('Formal signing requires a private configuration outside the repository and an HTTPS profile.')
    value = json.loads(config_path.read_text(encoding='utf-8'))
    required = {'keystore', 'alias', 'store_password_env', 'key_password_env', 'certificate_sha256', 'version_code', 'version_name', 'previous_version_code'}
    if not isinstance(value, dict) or set(value) != required:
        raise ValueError('The private signing configuration has an unsupported shape.')
    if type(value['version_code']) is not int or type(value['previous_version_code']) is not int or not 0 <= value['previous_version_code'] < value['version_code'] <= 2100000000:
        raise ValueError('An explicitly increasing Android version code is required.')
    if not isinstance(value['version_name'], str) or not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+(?:[-+][a-zA-Z0-9.-]+)?', value['version_name']):
        raise ValueError('An explicit semantic release version is required.')
    if not isinstance(value['certificate_sha256'], str) or not re.fullmatch('[0-9a-f]{64}', value['certificate_sha256']):
        raise ValueError('Pin the expected signing certificate SHA-256.')
    if not isinstance(value['alias'], str) or not re.fullmatch(r'[A-Za-z0-9_.-]{1,100}', value['alias']):
        raise ValueError('A bounded signing alias is required.')
    for key in ('store_password_env', 'key_password_env'):
        if not isinstance(value[key], str) or not re.fullmatch(r'CP_ANDROID_SECRET_[A-Z0-9_]{1,80}', value[key]) or not environ.get(value[key]):
            raise ValueError('Both private password environment references must be configured.')
    store = Path(value['keystore'])
    if not store.is_absolute() or not store.resolve(strict=True).is_file() or store.resolve().is_relative_to(APP.parent):
        raise ValueError('The signing keystore must be a real private file outside the repository.')
    return config_path, value


def certificate(output):
    matches = re.findall(r'(?:SHA256:|SHA-256 digest:)\s*([0-9A-Fa-f:]{64,95})', output)
    hashes = {value.replace(':', '').lower() for value in matches}
    if len(hashes) != 1 or not re.fullmatch('[0-9a-f]{64}', next(iter(hashes), '')):
        raise ValueError('A single independently inspected signing certificate is required.')
    return next(iter(hashes))


def inspect_release(apk, aab, bundle_sha256, config, sdk, jdk, env, runner=subprocess.run):
    for file in (apk, aab):
        entry = 'assets/index.android.bundle' if file.suffix == '.apk' else 'base/assets/index.android.bundle'
        with zipfile.ZipFile(file) as archive:
            if hashlib.sha256(archive.read(entry)).hexdigest() != bundle_sha256:
                raise ValueError('A release artifact does not contain the pinned current business bundle.')
    checks = [([str(sdk / 'build-tools/34.0.0/apksigner.bat'), 'verify', '--verbose', '--print-certs', str(apk)], True),
              ([str(jdk / 'jarsigner.exe'), '-J-Duser.language=en', '-verify', str(aab)], False),
              ([str(jdk / 'keytool.exe'), '-J-Duser.language=en', '-printcert', '-jarfile', str(aab)], True)]
    for command, needs_certificate in checks:
        result = runner(command, env=env, capture_output=True, text=True, creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
        if result.returncode or needs_certificate and certificate(result.stdout) != config['certificate_sha256'] or not needs_certificate and 'jar verified.' not in result.stdout:
            raise ValueError('Release signature inspection failed; no artifact was accepted.')
    result = runner([str(sdk / 'build-tools/34.0.0/aapt2.exe'), 'dump', 'badging', str(apk)], env=env, capture_output=True, text=True, creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
    expected = f"versionCode='{config['version_code']}' versionName='{config['version_name']}'"
    if result.returncode or "name='hk.copypaste.workspace'" not in result.stdout or expected not in result.stdout or 'application-debuggable' in result.stdout:
        raise ValueError('The signed APK package, increasing version or release flags differ from the declared inputs.')
    return {'certificate_sha256': config['certificate_sha256'], 'version_code': config['version_code'], 'version_name': config['version_name'], 'apk_signature_verified': True, 'aab_signature_verified': True, 'release_flags_verified': True, 'external_store_acceptance': False}
