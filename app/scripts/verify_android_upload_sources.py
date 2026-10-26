"""Verify this upload increment against real artifacts and unchanged regression sources."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import socket
import subprocess
import xml.etree.ElementTree as ET

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/android-upload'
PREVIOUS = EVIDENCE.parent / 'android-native'


def read(path): return json.loads(path.read_text(encoding='utf-8-sig'))


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / filename)
    result = importlib.util.module_from_spec(spec); spec.loader.exec_module(result); return result


def digest(names):
    value = hashlib.sha256()
    for name in names: value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def node_check(name, command, sources, expected):
    before = digest(sources)
    with (APP / ('.local/android/' + name + '.log')).open('w', encoding='utf-8') as output:
        result = subprocess.run(command, cwd=APP, stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW)
    text = (APP / ('.local/android/' + name + '.log')).read_text(encoding='utf-8', errors='replace')
    numbers = {key: int(value) for key, value in re.findall(r'^# (tests|pass|fail|skipped|cancelled) (\d+)\s*$', text, re.MULTILINE)}
    passed = result.returncode == 0 and before == digest(sources) and numbers.get('pass') == expected and all(numbers.get(key) == 0 for key in ('fail', 'skipped', 'cancelled'))
    report = {'passed': passed, 'tests': numbers, 'source_sha256': before, 'sources': sources, 'native_device_executed': False}
    (EVIDENCE / (name + '.json')).write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    return passed


def main():
    checks = []
    def check(name, passed): checks.append({'name': name, 'passed': bool(passed)})
    build = read(EVIDENCE / 'build.json')
    native_source = module('android_upload_build', 'build_android.py').sources()[0]
    check('The successful native build matches the current frozen inputs', build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == native_source)
    inspection = read(EVIDENCE / 'apk-inspection.json'); apk = APP / build['artifacts']['apk']['path']
    check('The inspected APK is the preserved artifact from this build', inspection['passed'] and inspection['apk_sha256'] == build['artifacts']['apk']['sha256'] == hashlib.sha256(apk.read_bytes()).hexdigest() and '/artifacts/android-upload/' in apk.as_posix())
    mobile = read(EVIDENCE / 'mobile-build.json')
    check('All seventy-nine mobile tests and both mini builds match current source', mobile['passed'] and mobile['tests']['pass'] == 79 and all(mobile['tests'][key] == 0 for key in ('fail', 'cancelled', 'skipped')) and mobile['source_sha256'] == module('android_upload_mobile', 'verify_mobile.py').digest()[0])
    pages = re.findall(r"'([^']+)'", (APP / 'mobile/src/app.config.ts').read_text(encoding='utf-8').split('window:')[0])
    check('The current application registers fifteen pages including the upload route', len(pages) == 15 and 'pages/uploads/index' in pages)
    web = read(PREVIOUS / 'web-build.json')
    check('The reused fifty-four Web tests and production build still match unchanged source', web['passed'] and web['tests']['numPassedTests'] == 54 and web['source_sha256'] == digest(web['sources']))
    runtime = read(PREVIOUS / 'runtime-build.json')
    check('The reused thirty-four runtime tests and draft skill checks still match source', runtime['status'] == 'passed' and runtime['tests']['pass'] == 34 and runtime['source_sha256'] == module('android_upload_runtime', 'verify_agent_runtime.py').digest()[0])
    backend = read(EVIDENCE.parent / 'telegram-delivery/result.json')
    backend_sha = module('android_upload_backend', 'verify_backend_batches.py').digest()
    check('The prior complete backend regression matches unchanged backend source', backend['passed'] and backend['source_sha256'] == backend_sha)
    folder = EVIDENCE.parent / 'android-upload-mobile'; browser = read(folder / 'result.json'); manifest = read(folder / 'source-manifest.json')
    check('The fresh actual-API H5 regression matches current source and closed its resources', browser['passed'] and len(browser['checks']) == 2 and all(c['passed'] for c in browser['checks']) and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'] and manifest['sha256'] == digest(manifest['files']))
    check('The private browser fixture has been removed', not (APP / '.local/business-browser-fixture.json').exists())
    with socket.socket() as probe: review_closed = probe.connect_ex(('127.0.0.1', 5189)) != 0
    check('The owned H5 review listener is closed', review_closed)
    check('Both actual Taro native Babel transforms pass', node_check('native-js', ['npm.cmd', 'run', 'test:native-js', '--workspace', '@copypaste/mobile'], ['mobile/babel.config.js', 'mobile/native/babel-primitives.cjs', 'mobile/native/babel-primitives.test.cjs', 'package-lock.json'], 2))
    check('Actual native page functions handle transformed classes and scrolling', node_check('native-layout', ['node', '--test', 'scripts/test_native_upload_layout.cjs'], ['scripts/test_native_upload_layout.cjs', 'mobile/src/platform/primitives.rn.tsx', 'mobile/src/platform/native-styles.ts', 'package-lock.json'], 2))
    java_sources = [('mobile/android/app/src/main/java/hk/copypaste/workspace/' + name + '.java') for name in ('ApiHTTP', 'ReadCache', 'UploadLayout', 'UploadSource', 'UploadPermit', 'UploadTransfer')]
    java_sources += sorted(p.relative_to(APP).as_posix() for p in (APP / 'mobile/android/app/src/test/java').rglob('*.java'))
    result_files = sorted((APP / 'mobile/native/jvm-tests/build/test-results/test').glob('TEST-*.xml'))
    suites = [ET.parse(path).getroot() for path in result_files]
    java_ok = sum(int(value.get('tests')) for value in suites) == 25 and all(all(int(value.get(key)) == 0 for key in ('failures', 'errors', 'skipped')) for value in suites) and all(path.stat().st_mtime >= max((APP / name).stat().st_mtime for name in java_sources) for path in result_files)
    java_report = {'passed': java_ok, 'tests': 25, 'source_sha256': digest(java_sources), 'sources': java_sources, 'xml_sha256': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in result_files}, 'scope': 'Actual unchanged pure-Java sources and HTTP tests; excludes SAF, SQLite, activity and service device execution.'}
    (EVIDENCE / 'jvm-source-proof.json').write_text(json.dumps(java_report, indent=2) + '\n', encoding='utf-8')
    check('All twenty-five standalone JVM cases match current actual Java inputs', java_ok)
    audit = read(PREVIOUS / 'dependency-audit.json'); lock = hashlib.sha256((APP / 'package-lock.json').read_bytes()).hexdigest()
    check('The dependency audit matches the unchanged lock and keeps the production gate open', audit['package_lock_sha256'] == lock and not audit['production_security_gate_passed'])
    preserved = read(PREVIOUS / 'artifact-preservation.json')
    check('The prior Android artifact remains preserved independently', preserved['passed'] and hashlib.sha256((APP.parent / preserved['preserved_path']).read_bytes()).hexdigest() == preserved['apk_sha256'])
    report = {'passed': all(value['passed'] for value in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
              'native_source_sha256': native_source, 'backend_sha256': backend_sha, 'package_lock_sha256': lock,
              'reused_evidence': {'web': '../android-native/web-build.json', 'runtime': '../android-native/runtime-build.json', 'backend': '../telegram-delivery/result.json', 'audit': '../android-native/dependency-audit.json'},
              'verifier_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'device_executed': False, 'real_120gb_executed': False, 'virtualization_used': False, 'production_release': False, 'full_goal_complete': False}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [value['name'] for value in checks if not value['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__': raise SystemExit(main())
