"""Bind Android and shared-client verification to the current checkout."""
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
EVIDENCE = APP.parent / 'docs/quality/evidence/android-native'


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / filename)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def read(path): return json.loads(path.read_text(encoding='utf-8-sig'))


def digest(names):
    value = hashlib.sha256()
    for name in names: value.update(name.encode('utf-8') + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def main():
    checks = []
    def check(name, passed): checks.append({'name': name, 'passed': bool(passed)})
    native = module('android_build_sources', 'build_android.py')
    build = read(EVIDENCE / 'build.json')
    check('Native build and Android tests pass with current frozen source', build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == native.sources()[0])
    inspect = read(EVIDENCE / 'apk-inspection.json')
    apk = APP / build['artifacts']['apk']['path']
    check('The inspected APK is the exact artifact from the recorded build', inspect['passed'] and inspect['apk_sha256'] == build['artifacts']['apk']['sha256'] == hashlib.sha256(apk.read_bytes()).hexdigest())
    web = read(EVIDENCE / 'web-build.json')
    check('All fifty-four Web tests and production build match current source', web['passed'] and web['tests']['numPassedTests'] == 54 and web['tests']['numFailedTests'] == 0 and web['source_sha256'] == digest(web['sources']))
    mobile = read(EVIDENCE / 'mobile-build.json')
    check('Sixty-six mobile tests and both mini builds match current source', mobile['passed'] and mobile['tests']['pass'] == 66 and mobile['source_sha256'] == module('android_mobile_regression', 'verify_mobile.py').digest()[0])
    runtime = read(EVIDENCE / 'runtime-build.json')
    check('Thirty-four runtime tests and all draft skill checks match source', runtime['status'] == 'passed' and runtime['tests']['pass'] == 34 and runtime['source_sha256'] == module('android_runtime_regression', 'verify_agent_runtime.py').digest()[0])
    previous = read(EVIDENCE.parent / 'telegram-delivery/result.json')
    backend_sha = module('android_backend_reference', 'verify_backend_batches.py').digest()
    check('The existing full backend proof still matches unchanged backend code', previous['passed'] and previous['source_sha256'] == backend_sha)
    browser_folder = EVIDENCE.parent / 'android-native-mobile'
    browser, manifest = read(browser_folder / 'result.json'), read(browser_folder / 'source-manifest.json')
    check('The actual mobile browser regression is current and all owned resources closed', browser['passed'] and len(browser['checks']) == 2 and all(c['passed'] for c in browser['checks']) and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'] and manifest['sha256'] == digest(manifest['files']))
    check('The private browser fixture was removed', not (APP / '.local/business-browser-fixture.json').exists())
    with socket.socket() as probe: review_closed = probe.connect_ex(('127.0.0.1', 5189)) != 0
    check('The owned H5 review listener is closed', review_closed)
    paths = ['mobile/babel.config.js', 'mobile/native/babel-primitives.cjs', 'mobile/native/babel-primitives.test.cjs', 'package-lock.json']
    before = digest(paths)
    with (APP / '.local/android/native-js-tests.log').open('w', encoding='utf-8') as output:
        result = subprocess.run(['npm.cmd', 'run', 'test:native-js', '--workspace', '@copypaste/mobile'], cwd=APP, stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW)
    text = (APP / '.local/android/native-js-tests.log').read_text(encoding='utf-8', errors='replace')
    numbers = {name: int(value) for name, value in re.findall(r'^# (tests|pass|fail|skipped|cancelled) (\d+)\s*$', text, re.MULTILINE)}
    js = {'passed': result.returncode == 0 and before == digest(paths) and numbers.get('pass') == 2 and all(numbers.get(name) == 0 for name in ('fail', 'skipped', 'cancelled')), 'tests': numbers, 'source_sha256': before, 'sources': paths}
    (EVIDENCE / 'native-js.json').write_text(json.dumps(js, indent=2) + '\n', encoding='utf-8')
    check('Both actual native Babel transform tests pass with unchanged inputs', js['passed'])
    suite_files = list((APP / 'mobile/native/jvm-tests/build/test-results/test').glob('TEST-*.xml'))
    suites = [ET.parse(file).getroot() for file in suite_files]
    jvm_sources = [APP / ('mobile/android/app/src/main/java/hk/copypaste/workspace/' + name) for name in ('ApiHTTP.java', 'ReadCache.java')]
    jvm_sources += list((APP / 'mobile/android/app/src/test/java').rglob('*.java'))
    check('The separate JVM boundary suite executed all eleven current cases', sum(int(s.get('tests')) for s in suites) == 11 and all(all(int(s.get(name)) == 0 for name in ('errors', 'failures', 'skipped')) for s in suites) and all(file.stat().st_mtime >= max(p.stat().st_mtime for p in jvm_sources) for file in suite_files))
    audit = read(APP / '.local/android/audit.json')
    audit_report = {'checked_at': datetime.now(timezone.utc).isoformat(), 'package_lock_sha256': hashlib.sha256((APP / 'package-lock.json').read_bytes()).hexdigest(), 'vulnerabilities': audit['metadata']['vulnerabilities'], 'production_security_gate_passed': False}
    (EVIDENCE / 'dependency-audit.json').write_text(json.dumps(audit_report, indent=2) + '\n', encoding='utf-8')
    check('Dependency warnings are recorded as an open production gate', audit_report['vulnerabilities']['total'] > 0 and not audit_report['production_security_gate_passed'])
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks, 'backend_sha256': backend_sha,
              'native_source_sha256': native.sources()[0], 'package_lock_sha256': audit_report['package_lock_sha256'],
              'virtualization_used': False, 'device_executed': False, 'production_release': False, 'full_goal_complete': False}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__': raise SystemExit(main())
