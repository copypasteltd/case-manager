"""Retain exact native suite XML and label normal Gradle cache outcomes."""
import hashlib
import json
from pathlib import Path
import re
import xml.etree.ElementTree as ET

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/host-resources'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()


def main():
    logs = {'android': APP / '.local/android/native.log', 'jvm': APP / '.local/android/host-resources-jvm.log'}
    outcomes = {}
    results = []
    for suite, folder, task in [('android', 'mobile/android/app/build/test-results/testDebugUnitTest', ':app:testDebugUnitTest'), ('jvm', 'mobile/native/jvm-tests/build/test-results/test', ':test')]:
        text = logs[suite].read_text(encoding='utf-8', errors='replace')
        matches = re.findall(r'^> Task ' + re.escape(task) + r'(?: (UP-TO-DATE|FROM-CACHE|SKIPPED|NO-SOURCE|FAILED))?\s*$', text, re.M)
        assert len(matches) == 1 and matches[0] not in ('SKIPPED', 'NO-SOURCE', 'FAILED'), 'Require one successful test task outcome.'
        outcomes[suite] = matches[0] or 'EXECUTED'
        target = E / (suite + '-unit-results')
        target.mkdir(exist_ok=True)
        for file in sorted((APP / folder).glob('TEST-*.xml')):
            dst = target / file.name
            if dst.exists():
                assert dst.read_bytes() == file.read_bytes()
            else:
                dst.write_bytes(file.read_bytes())
            prior = ROOT / 'docs/quality/evidence/office-authoring' / (suite + '-unit-results') / file.name
            if outcomes[suite] == 'UP-TO-DATE':
                assert prior.is_file() and sha(prior) == sha(dst), 'Cached test XML must match its published source-bound result.'
            row = ET.parse(dst).getroot()
            results.append({'suite': suite, 'file': dst.relative_to(ROOT).as_posix(), 'sha256': sha(dst), 'tests': int(row.get('tests')), 'failures': int(row.get('failures')), 'errors': int(row.get('errors')), 'skipped': int(row.get('skipped')), 'original_suite_timestamp': row.get('timestamp'), 'gradle_outcome': outcomes[suite], 'cached_from_increment': 129 if outcomes[suite] == 'UP-TO-DATE' else None})
    assert all(sum(row['tests'] for row in results if row['suite'] == suite) == 28 for suite in ('android', 'jvm'))
    assert all(row['failures'] == row['errors'] == row['skipped'] == 0 for row in results)
    record = {'passed': True, 'source_sha256': json.loads((E / 'build.json').read_bytes())['source_sha256'], 'complete_build_record_sha256': sha(E / 'build.json'), 'suites_record_sha256': sha(E / 'native-suites.json'), 'test_task_outcomes': outcomes, 'private_logs': [{'name': p.name, 'sha256': sha(p)} for p in logs.values()], 'xml_results': results, 'formal_android_acceptance': False, 'device_used': False, 'virtualization_used': False}
    (E / 'native-test-scope.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'android': 28, 'jvm': 28, 'task_outcomes': outcomes}))


if __name__ == '__main__':
    main()
