"""Freeze exact capture metadata and actual native-test scope; never publish raw frames or credentials."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import shutil
import xml.etree.ElementTree as ET
from build_android import sources as android_sources
from status_bridge_sources import APP, digest

ROOT = APP.parent
EVIDENCE = ROOT / 'docs/quality/evidence/status-bridge'
BROWSER = EVIDENCE.parent / 'status-bridge-browser'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def copy_exact(source, destination):
    if destination.exists() and destination.read_bytes() != source.read_bytes():
        raise SystemExit('An existing frozen evidence file differs; archive that completed attempt first.')
    if not destination.exists():
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, destination)


def write(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--capture-only', action='store_true')
    args = parser.parse_args()
    browser = read(BROWSER / 'result.json')
    assert browser['passed'] and browser['source_unchanged'] and browser['source_sha256'] == digest()
    assert browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['native_fixture_exit_code'] == 0
    assert browser['contexts_created'] == browser['contexts_closed'] == 2
    assert not (APP / '.local/business-browser-fixture.json').exists()
    assert len(browser['recordings']) == 1
    recording = browser['recordings'][0]
    source = (APP / recording['manifest']).resolve()
    assert source.is_relative_to((APP / '.local/repository-showcase').resolve()) and sha(source) == recording['manifest_sha256']
    frames = read(source)
    assert frames['credentials_excluded'] and frames['synthetic_data'] and frames['real_browser_pixels'] and not frames['audio']
    assert frames['frame_count'] == len(frames['frames']) == recording['frame_count']
    for frame in frames['frames']:
        assert re.fullmatch(r'frame-[0-9]{6}\.jpg', frame['file']) and sha(source.parent / frame['file']) == frame['sha256']
    copy_exact(source, EVIDENCE / 'recording-frames.json')
    own_sha = sha(Path(__file__))
    write(EVIDENCE / 'recording-provenance.json', {
        'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'source': 'Ignored actual JPEG frames under app/.local; no raw frame publication.',
        'published_manifest': 'recording-frames.json', 'manifest_sha256': sha(source), 'frames': frames['frame_count'],
        'freezer_sha256': own_sha, 'public_business_identifiers_included': True, 'credentials_excluded': True,
    })
    if not args.capture_only:
        from verify_mobile import digest as portable_digest
        build = read(EVIDENCE / 'build.json')
        assert build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == android_sources()[0]
        assert len(build['phases']) == 2 and all(p['passed'] and p['exit_code'] == 0 for p in build['phases'])
        assert sha(APP / build['artifacts']['apk']['path']) == build['artifacts']['apk']['sha256']
        log = APP / '.local/android/native.log'
        assert log.stat().st_mtime <= datetime.fromisoformat(build['checked_at']).timestamp()
        tasks = re.findall(r'^> Task :app:testDebugUnitTest(?: .*)?$', log.read_text(encoding='utf-8'), re.MULTILINE)
        assert len(tasks) == 1 and tasks[0] in ['> Task :app:testDebugUnitTest', '> Task :app:testDebugUnitTest UP-TO-DATE', '> Task :app:testDebugUnitTest FROM-CACHE']
        xml = sorted((APP / 'mobile/android/app/build/test-results/testDebugUnitTest').glob('TEST-*.xml'))
        suites = [ET.parse(path).getroot() for path in xml]
        assert len(xml) == 4 and sum(int(s.attrib['tests']) for s in suites) == 25
        assert all(int(s.attrib[k]) == 0 for s in suites for k in ['failures', 'errors', 'skipped'])
        copy_exact(log, EVIDENCE / 'native-build.log')
        for path in xml:
            copy_exact(path, EVIDENCE / 'android-unit-results' / path.name)
        cached = 'UP-TO-DATE' in tasks[0] or 'FROM-CACHE' in tasks[0]
        write(EVIDENCE / 'android-test-scope.json', {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(),
            'task': tasks[0], 'gradle_cached': cached, 'new_test_execution': not cached, 'tests': 25,
            'native_log_sha256': sha(log), 'xml_sha256': {p.name: sha(p) for p in xml},
            'xml_artifact_directory': 'android-unit-results', 'freezer_sha256': own_sha, 'device_executed': False})
        mobile = read(EVIDENCE / 'mobile-build.json')
        assert mobile['passed'] and mobile['source_unchanged_during_run'] and mobile['tests']['pass'] == 154
        assert mobile['source_sha256'] == portable_digest()[0]
        copy_exact(APP / '.local/mobile-verification.log', EVIDENCE / 'mobile-verification.log')
    print(json.dumps({'passed': True, 'capture_frames': frames['frame_count'], 'native_scope_frozen': not args.capture_only}))


if __name__ == '__main__':
    main()
