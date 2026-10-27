"""Freeze a failed owned browser attempt and its exact complete source bytes before edits."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import zipfile
from mobile_files_sources import APP, sources, digest

ROOT = APP.parent


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('name')
    parser.add_argument('--reason', required=True)
    parser.add_argument('--superseded', action='store_true')
    args = parser.parse_args()
    assert args.name.replace('-', '').isalnum()
    browser = ROOT / 'docs/quality/evidence/mobile-files-browser'
    report = json.loads((browser / 'result.json').read_text(encoding='utf-8'))
    manifest = json.loads((browser / 'source-manifest.json').read_text(encoding='utf-8'))
    assert (not report['passed'] or args.superseded) and report['owned_browser_closed'] and report['isolated_database_cleanup_passed'] and report['owned_review_server_stopped']
    assert digest() == report['source_sha256_before_fixture_and_build']
    assert digest(manifest['files']) == report['source_sha256'] == manifest['sha256']
    target = ROOT / 'docs/quality/evidence/mobile-files/attempts' / args.name
    target.mkdir(parents=True, exist_ok=False)
    complete = sorted(set(sources() + manifest['files']))
    archive = target / 'sources.zip'
    with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as output:
        for name in complete:
            path = (APP / name).resolve()
            assert path.is_relative_to(APP.resolve()) and path.is_file()
            output.write(path, name)
    for file in browser.iterdir():
        if file.is_file():
            shutil.copyfile(file, target / file.name)
            file.unlink()
    for file in (ROOT / 'docs/media/screenshots').glob('mobile-files-*.png'):
        (target / 'media').mkdir(exist_ok=True)
        shutil.copyfile(file, target / 'media' / file.name)
        file.unlink()
    for clip in ('directory', 'lifecycle'):
        frames = APP / ('.local/repository-showcase/mobile-files-' + clip)
        if frames.exists():
            destination = APP / '.local/mobile-files/failed-frames' / args.name / clip
            assert frames.resolve().is_relative_to((APP / '.local').resolve()) and not destination.exists()
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.move(str(frames), str(destination))
    receipt = {'passed': report['passed'], 'diagnostic_only': True, 'superseded': args.superseded, 'reason': args.reason, 'source_count': len(complete),
        'complete_startup_source_sha256': report['source_sha256_before_fixture_and_build'], 'capture_source_sha256': report['source_sha256'],
        'source_zip_sha256': hashlib.sha256(archive.read_bytes()).hexdigest(),
        'source_manifest': manifest['files'], 'startup_manifest': sources(),
        'owned_browser_closed': True, 'owned_review_server_stopped': True, 'isolated_database_cleanup_passed': True,
        'raw_frames_published': False, 'scope': 'Exact original source and diagnostics, retained before correction or explicit supersession. Status is preserved; no current-source or production claim.'}
    (target / 'attempt.json').write_text(json.dumps(receipt, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'archived': args.name, 'exact_source_count': len(complete), 'owned_cleanup_passed': True}))


if __name__ == '__main__':
    main()
