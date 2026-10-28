"""Freeze actual Android result scope without reclassifying cached JVM results as new executions."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import shutil
import xml.etree.ElementTree as ET
from build_android import sources

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/finance-fx'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def copy_exact(source, target):
    if target.exists() and target.read_bytes() != source.read_bytes():
        raise SystemExit('Existing frozen evidence differs; archive that attempt before replacing it.')
    target.parent.mkdir(parents=True, exist_ok=True)
    if not target.exists():
        shutil.copyfile(source, target)


def main():
    build = json.loads((EVIDENCE / 'build.json').read_text(encoding='utf-8'))
    assert build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == sources()[0]
    assert sha(APP / build['artifacts']['apk']['path']) == build['artifacts']['apk']['sha256']
    log = APP / '.local/android/native.log'
    assert log.stat().st_mtime <= datetime.fromisoformat(build['checked_at']).timestamp()
    tasks = re.findall(r'^> Task :app:testDebugUnitTest(?: .*)?$', log.read_text(encoding='utf-8'), re.MULTILINE)
    assert len(tasks) == 1 and tasks[0] in ['> Task :app:testDebugUnitTest', '> Task :app:testDebugUnitTest UP-TO-DATE', '> Task :app:testDebugUnitTest FROM-CACHE']
    xml = sorted((APP / 'mobile/android/app/build/test-results/testDebugUnitTest').glob('TEST-*.xml'))
    suites = [ET.parse(file).getroot() for file in xml]
    assert len(xml) == 4 and sum(int(suite.attrib['tests']) for suite in suites) == 26
    assert all(int(suite.attrib[key]) == 0 for suite in suites for key in ('failures', 'errors', 'skipped'))
    copy_exact(log, EVIDENCE / 'native-build.log')
    for file in xml:
        copy_exact(file, EVIDENCE / 'android-unit-results' / file.name)
    cached = 'UP-TO-DATE' in tasks[0] or 'FROM-CACHE' in tasks[0]
    report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'task': tasks[0],
        'gradle_cached': cached, 'new_test_execution': not cached, 'tests': 26, 'native_log_sha256': sha(log),
        'xml_sha256': {file.name: sha(file) for file in xml}, 'xml_artifact_directory': 'android-unit-results',
        'freezer_sha256': sha(Path(__file__)), 'device_executed': False}
    (EVIDENCE / 'android-test-scope.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    copy_exact(APP / '.local/finance-fx-client-mobile-tests.log', EVIDENCE / 'mobile-tests.log')
    print(json.dumps({'passed': True, 'native_tests': 26, 'gradle_cached': cached}))


if __name__ == '__main__':
    main()
