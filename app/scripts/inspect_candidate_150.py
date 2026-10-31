"""Inspect the exact new development APK and its actual Metro graph; no device."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import zipfile
from build_android import sources
from build_candidate_150 import LOCAL

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/release-candidate-150'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
read = lambda p: json.loads(p.read_text(encoding='utf-8'))


def main():
    assert not (E / 'apk-inspection.json').exists(), 'Archive an earlier inspection first.'
    mini = read(E / 'miniapps-build.json')
    archive_path = ROOT / mini['archive']
    assert mini['passed'] and sha(archive_path) == mini['archive_sha256']
    mini_checks = []
    with zipfile.ZipFile(archive_path) as archive:
        assert archive.testzip() is None
        for target in ('weapp', 'tt'):
            files = [n for n in archive.namelist() if n.startswith(target + '/') and n.endswith('.js')]
            content = b'\n'.join(archive.read(n) for n in files).decode('utf-8')
            # Taro emits non-ASCII UI literals as JavaScript Unicode escapes.
            # Normalize only those literals for this presence inspection.
            content = re.sub(r'\\u([0-9a-fA-F]{4})', lambda match: chr(int(match[1], 16)), content)
            tokens = ('replica_id', 'version_id', 'replica=', '/versions/', '通知对应副本')
            mini_checks.append({'target': target, 'passed': all(token in content for token in tokens),
                'javascript_files': len(files), 'required_markers': list(tokens)})
    mini_report = {'passed': all(r['passed'] for r in mini_checks), 'archive_sha256': sha(archive_path),
        'checks': mini_checks, 'inspection_script_sha256': sha(Path(__file__)), 'platform_executed': False}
    assert mini_report['passed'], 'Current notification routing is missing from a miniapp artifact.'
    (E / 'miniapps-inspection.json').write_text(json.dumps(mini_report, indent=2) + '\n', encoding='utf-8')
    build = read(E / 'build.json')
    assert build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == sources()[0]
    apk = APP / build['artifacts']['apk']['path']
    assert sha(apk) == build['artifacts']['apk']['sha256']
    sdk = Path(os.environ.get('CP_ANDROID_SDK_ROOT', str(Path(os.environ['LOCALAPPDATA']) / 'Android/Sdk')))
    tools = sdk / 'build-tools/34.0.0'
    jdk = list((APP / '.tools/jdk17').glob('*/bin/java.exe'))
    assert len(jdk) == 1
    env = {**os.environ, 'JAVA_HOME': str(jdk[0].parents[1])}
    logs = LOCAL

    def command(name, args):
        result = subprocess.run(args, capture_output=True, env=env, creationflags=subprocess.CREATE_NO_WINDOW)
        assert result.returncode == 0, 'Native APK inspection failed.'
        (logs / (name + '.log')).write_bytes(result.stdout + result.stderr)
        return result.stdout.decode('utf-8', errors='replace')

    badging = command('badging', [str(tools / 'aapt2.exe'), 'dump', 'badging', str(apk)])
    manifest = command('manifest', [str(tools / 'aapt2.exe'), 'dump', 'xmltree', str(apk), '--file', 'AndroidManifest.xml'])
    signature = command('signature', ['cmd.exe', '/d', '/c', str(tools / 'apksigner.bat'), 'verify', '--verbose', '--print-certs', str(apk)])
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    permissions = re.findall(r"uses-permission(?:-sdk-\d+)?: name='([^']+)'", badging)
    required = {'android.permission.INTERNET', 'android.permission.FOREGROUND_SERVICE',
        'android.permission.FOREGROUND_SERVICE_DATA_SYNC', 'android.permission.POST_NOTIFICATIONS'}
    private = 'hk.copypaste.workspace.debug.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION'
    blocks = re.split(r'\n\s*E:', manifest)
    check('Separate development application ID', "package: name='hk.copypaste.workspace.debug'" in badging)
    check('Only required transport/upload/notification and signature-private permissions',
        required.issubset(permissions) and set(permissions).issubset(required | {private})
        and (private not in permissions or any(b.lstrip().startswith('permission ') and private in b
            and re.search(r'protectionLevel[^\n]*0x0*2\b', b) for b in blocks)))
    check('Expected arm64 architecture and declared SDK23–34', "native-code: 'arm64-v8a'" in badging
        and "sdkVersion:'23'" in badging and "targetSdkVersion:'34'" in badging)
    check('Backup disabled and upload foreground service private without boot restart',
        bool(re.search(r'allowBackup[^\n]*=(?:false|\(type 0x12\)0x0+)\b', manifest))
        and any(b.lstrip().startswith('service ') and 'hk.copypaste.workspace.UploadServiceHost' in b
            and re.search(r'exported[^\n]*=(?:false|\(type 0x12\)0x0+)\b', b)
            and re.search(r'foregroundServiceType[^\n]*0x0*1\b', b) for b in blocks)
        and 'android.intent.action.BOOT_COMPLETED' not in manifest)
    check('Actual development APK signature verifies', bool(re.search(r'Verified using v[23] scheme[^\n]*: true', signature)))
    with zipfile.ZipFile(apk) as package:
        bundle = package.read('assets/index.android.bundle')
        check('Current portable JS bytes are embedded in APK', hashlib.sha256(bundle).hexdigest() == build['artifacts']['bundle']['sha256'])
        check('Native engine and actual upload/Office bridge packaged', 'lib/arm64-v8a/libhermes.so' in package.namelist()
            and all(('Lhk/copypaste/workspace/' + name + ';').encode() in b''.join(package.read(p)
                for p in package.namelist() if re.fullmatch(r'classes\d*\.dex', p))
                for name in ('CPUploadModule', 'UploadTransfer', 'UploadServiceHost', 'SAFUploadSource', 'UploadStore', 'GeneratedUploadSource')))
    mapping = APP / '.local/android/index.android.map'
    data = read(mapping)
    assert len(data['sources']) == len(data['sourcesContent'])
    paths = [p.replace('\\', '/') for p in data['sources']]
    check('No Expo extension or WebView entered business graph', not any(re.search(
        r'/node_modules/(?:expo(?:-[^/]+)?|@unimodules|react-native-webview)/', p) for p in paths))
    rows = []
    for path in sorted([*(APP / 'mobile/src').rglob('*'), *(APP / 'packages/contracts/src').rglob('*')]):
        if not path.is_file() or path.suffix not in ('.ts', '.tsx'):
            continue
        name = path.relative_to(APP).as_posix()
        suffix = name if name.startswith('packages/') else name.removeprefix('mobile/')
        indices = [i for i, p in enumerate(paths) if p == suffix or p.endswith('/' + suffix)]
        if not indices:
            continue
        assert len(indices) == 1
        value = data['sourcesContent'][indices[0]]
        assert isinstance(value, str) and value.replace('\r\n', '\n') == path.read_text(encoding='utf-8').replace('\r\n', '\n')
        rows.append({'file': path.relative_to(ROOT).as_posix(), 'sha256': sha(path), 'matches_actual_map': True})
    selected = {r['file'].removeprefix('app/') for r in rows}
    check('Latest replica notification routing and earlier business modules match actual current source map',
        {'mobile/src/pages/detail/index.tsx', 'mobile/src/pages/files/replica-retry.tsx', 'packages/contracts/src/notifications.ts', 'mobile/src/pages/settings/storage.tsx', 'packages/contracts/src/storage-operations.ts', 'mobile/src/pages/files/retirement.tsx', 'packages/contracts/src/object-retirement.ts',
         'mobile/src/pages/telegram/operations.tsx', 'mobile/src/pages/telegram/inbox.tsx',
         'mobile/src/pages/secretary/runtime.tsx', 'mobile/src/pages/secretary/project-catalog.tsx',
         'mobile/src/pages/secretary/subscription-portfolio.tsx', 'packages/contracts/src/subscription-portfolio.ts',
         'packages/contracts/src/project-catalog.ts', 'packages/contracts/src/agent-runtime.ts',
         'mobile/src/pages/secretary/oa-requests.tsx', 'mobile/src/pages/secretary/oa-text.tsx',
         'packages/contracts/src/oa-text.ts'}.issubset(selected)
        and len(rows) >= 80)
    pages = re.findall(r"'([^']+)'", (APP / 'mobile/src/app.config.ts').read_text(encoding='utf-8').split('window:')[0])
    check('All twenty registered business pages are in actual native graph', len(pages) == 20
        and all('mobile/src/' + p + '.tsx' in selected for p in pages))
    report = {'passed': all(r['passed'] for r in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'checks': checks, 'apk_sha256': sha(apk), 'apk_bytes': apk.stat().st_size,
        'build_record_sha256': sha(E / 'build.json'), 'source_sha256': build['source_sha256'],
        'source_map_sha256': sha(mapping), 'current_source_map_files': rows,
        'inspection_script_sha256': sha(Path(__file__)), 'device_executed': False,
        'emulator_started': False, 'production_release': False}
    (E / 'apk-inspection.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'actual_current_modules': len(rows),
        'failed': [r['name'] for r in checks if not r['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
