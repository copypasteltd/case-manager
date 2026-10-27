"""Inspect the actual APK and test outputs; never starts a device."""
from datetime import datetime, timezone
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import xml.etree.ElementTree as ET
import zipfile

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/android-native'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--evidence-group', default='android-native')
    args = parser.parse_args()
    if not re.fullmatch(r'[a-z][a-z0-9-]{0,63}', args.evidence_group): raise SystemExit('Invalid evidence group.')
    evidence = APP.parent / 'docs/quality/evidence' / args.evidence_group
    build = json.loads((evidence / 'build.json').read_text(encoding='utf-8'))
    if not build['passed']: raise SystemExit('A successful recorded native build is required.')
    apk = APP / build['artifacts']['apk']['path']
    sdk = Path(os.environ.get('CP_ANDROID_SDK_ROOT', str(Path(os.environ['LOCALAPPDATA']) / 'Android/Sdk')))
    tools = sdk / 'build-tools/34.0.0'
    jdk = sorted((APP / '.tools/jdk17').glob('*/bin/java.exe'))
    if len(jdk) != 1 or not apk.is_file(): raise SystemExit('The current native APK and JDK are required.')
    env = {**os.environ, 'JAVA_HOME': str(jdk[0].parents[1])}
    checks = []
    def check(name, passed): checks.append({'name': name, 'passed': bool(passed)})
    def command(name, args):
        result = subprocess.run(args, env=env, capture_output=True, text=True, encoding='utf-8', errors='replace', creationflags=subprocess.CREATE_NO_WINDOW)
        (APP / ('.local/android/' + name + '.log')).write_text(result.stdout + result.stderr, encoding='utf-8')
        if result.returncode: raise RuntimeError('Native artifact inspection failed: ' + name)
        return result.stdout
    badging = command('apk-badging', [str(tools / 'aapt2.exe'), 'dump', 'badging', str(apk)])
    permissions = re.findall(r"uses-permission(?:-sdk-\d+)?: name='([^']+)'", badging)
    package = re.search(r"package: name='([^']+)'", badging)
    architecture = re.findall(r"'([^']+)'", next((line for line in badging.splitlines() if line.startswith('native-code:')), ''))
    check('Development application ID is separate from production', package and package[1] == 'hk.copypaste.workspace.debug')
    private_permission = 'hk.copypaste.workspace.debug.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION'
    upload_permissions = {'android.permission.FOREGROUND_SERVICE', 'android.permission.FOREGROUND_SERVICE_DATA_SYNC', 'android.permission.POST_NOTIFICATIONS'}
    required_permissions = {'android.permission.INTERNET'} | upload_permissions
    check('Only network, upload service, notification and private receiver permissions are packaged', required_permissions.issubset(permissions) and set(permissions).issubset(required_permissions | {private_permission}))
    check('Only the intended physical-device ABI is packaged', architecture == ['arm64-v8a'])
    check('Supported SDK bounds are present', "sdkVersion:'23'" in badging and "targetSdkVersion:'34'" in badging)
    manifest = command('apk-manifest', [str(tools / 'aapt2.exe'), 'dump', 'xmltree', str(apk), '--file', 'AndroidManifest.xml'])
    check('System backup is disabled', bool(re.search(r'(?:android:|/android:)allowBackup[^\n]*=(?:false|\(type 0x12\)0x0+)\b', manifest)))
    check('The native launcher is present', 'hk.copypaste.workspace.MainActivity' in manifest)
    permission_blocks = re.split(r'\n\s*E:', manifest)
    check('Any private receiver permission is signature protected', private_permission not in permissions or any(block.lstrip().startswith('permission ') and private_permission in block and re.search(r'(?:android:|/android:)protectionLevel[^\n]*0x0*2\b', block) for block in permission_blocks))
    signature = command('apk-signature', ['cmd.exe', '/d', '/c', str(tools / 'apksigner.bat'), 'verify', '--verbose', '--print-certs', str(apk)])
    check('APK signature verification succeeds', bool(re.search(r'Verified using v[23] scheme[^\n]*: true', signature)))
    suites = []
    for file in sorted((APP / 'mobile/android/app/build/test-results/testDebugUnitTest').glob('TEST-*.xml')):
        root = ET.parse(file).getroot()
        suites.append({key: root.attrib.get(key) for key in ('name', 'tests', 'failures', 'errors', 'skipped')})
    check('All twenty-five Android transport, cache and upload tests pass', sum(int(s['tests']) for s in suites) == 25 and all(all(int(s[k]) == 0 for k in ('failures', 'errors', 'skipped')) for s in suites) and any(s['name'].endswith('.UploadTransferTest') for s in suites))
    with zipfile.ZipFile(apk) as archive:
        bundle = archive.read('assets/index.android.bundle')
        expected = APP / 'mobile/android/app/src/main/assets/index.android.bundle'
        check('Packaged business code matches the current offline bundle', hashlib.sha256(bundle).digest() == hashlib.sha256(expected.read_bytes()).digest())
        names = archive.namelist()
        check('Native engine and application code are included', 'lib/arm64-v8a/libhermes.so' in names and 'classes.dex' in names)
        dex = b''.join(archive.read(name) for name in names if re.fullmatch(r'classes\d*\.dex', name))
        check('The actual native upload bridge, streaming transport and service are packaged', all(('Lhk/copypaste/workspace/' + name + ';').encode() in dex for name in ('CPUploadModule', 'UploadTransfer', 'UploadServiceHost', 'SAFUploadSource', 'UploadStore')))
    source_map = json.loads((APP / '.local/android/index.android.map').read_text(encoding='utf-8'))
    sources = [name.replace('\\', '/') for name in source_map['sources']]
    check('Project budget comparison and shared exact-source guards are in the actual business graph', any(name.endswith('/mobile/src/pages/project/budget-comparison.tsx') for name in sources) and any(name.endswith('/packages/contracts/src/budget-comparison.ts') for name in sources))
    required = ['platform/app-root.rn.tsx', 'platform/taro.rn.ts', 'platform/theme.rn.ts', 'platform/content-sink.rn.ts', 'platform/primitives.rn.tsx', 'platform/components.rn.tsx', 'pages/content/view.rn.tsx', 'platform/uploads.rn.ts', 'platform/upload-entry.rn.tsx', 'pages/uploads/view.rn.tsx']
    check('The business graph uses the intended native platform adapters', all(any(name.endswith('/mobile/src/' + suffix) for name in sources) for suffix in required))
    check('No Expo extension or WebView entered the offline business graph', not any(re.search(r'/node_modules/(?:expo(?:-[^/]+)?|@unimodules|react-native-webview)/', name) for name in sources))
    pages = re.findall(r"'([^']+)'", (APP / 'mobile/src/app.config.ts').read_text(encoding='utf-8').split('window:')[0])
    check('All seventeen registered business pages including finance and token budgets are included', len(pages) == 17 and 'pages/budget/index' in pages and 'pages/finance/index' in pages and all(any(name.endswith('/mobile/src/' + page + '.tsx') for name in sources) for page in pages))
    check('The user-started dataSync upload service is private', any(block.lstrip().startswith('service ') and 'hk.copypaste.workspace.UploadServiceHost' in block and re.search(r'(?:android:|/android:)exported[^\n]*=(?:false|\(type 0x12\)0x0+)\b', block) and re.search(r'(?:android:|/android:)foregroundServiceType[^\n]*0x0*1\b', block) for block in permission_blocks))
    check('Upload does not register a boot-time restart receiver', 'android.intent.action.BOOT_COMPLETED' not in manifest and 'android.permission.RECEIVE_BOOT_COMPLETED' not in permissions)
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
              'apk_sha256': hashlib.sha256(apk.read_bytes()).hexdigest(), 'apk_bytes': apk.stat().st_size,
              'application_id': package[1] if package else None, 'permissions': permissions, 'architectures': architecture,
              'tests': suites, 'device_executed': False, 'emulator_started': False, 'production_release': False}
    evidence.mkdir(parents=True, exist_ok=True)
    (evidence / 'apk-inspection.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__': raise SystemExit(main())
