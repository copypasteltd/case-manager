"""Preserve a genuinely passed browser source/capture before changing its checker."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import zipfile
from contract_body_sources import APP, digest, sources

ROOT = APP.parent


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('label')
    parser.add_argument('--mobile', action='store_true')
    args = parser.parse_args()
    assert args.label.replace('-', '').isalnum()
    group = 'contract-body-' + ('mobile' if args.mobile else 'web')
    old = ROOT / 'docs/quality/evidence' / group
    public = ROOT / 'docs/quality/evidence/contract-body-attempts' / args.label
    private = APP / '.local/contract-body-attempts' / args.label
    assert not public.exists() and not private.exists()
    result, manifest, media = [json.loads((old / n).read_text(encoding='utf-8')) for n in ('result.json', 'source-manifest.json', 'media.json')]
    assert result['passed'] and result['build_source_sha256'] == digest() and result['owned_browser_closed'] and result['native_fixture_exit_code'] == 0
    public.mkdir(parents=True)
    private.mkdir(parents=True)
    archive = private / 'source.zip'
    with zipfile.ZipFile(archive, 'x', zipfile.ZIP_DEFLATED) as output:
        for name in sorted(set(sources()) | set(manifest['files'])):
            output.write(APP / name, 'app/' + name)
    records = []
    for path in list(old.iterdir()):
        target = (public if path.suffix == '.json' else private) / path.name
        shutil.move(path, target)
        records.append({'file': target.relative_to(ROOT).as_posix(), 'sha256': sha(target)})
    for item in media['screenshots']:
        path = ROOT / item['file']
        assert sha(path) == item['sha256']
        target = private / path.name
        shutil.copyfile(path, target)
        records.append({'file': target.relative_to(ROOT).as_posix(), 'sha256': sha(target)})
    for path in list((APP / '.local').glob(group + '-*.log')):
        target = private / path.name
        shutil.move(path, target)
        records.append({'file': target.relative_to(ROOT).as_posix(), 'sha256': sha(target)})
    capture = APP / media['recording']['manifest']
    assert sha(capture) == media['recording']['manifest_sha256']
    # A later capture reuses the live directory. Preserve bytes, not its mutable path.
    recording = private / 'recording'
    recording.mkdir()
    manifest_copy = recording / 'frames.json'
    shutil.copyfile(capture, manifest_copy)
    assert sha(manifest_copy) == media['recording']['manifest_sha256']
    records.append({'file': manifest_copy.relative_to(ROOT).as_posix(), 'sha256': sha(manifest_copy)})
    for frame in json.loads(manifest_copy.read_text(encoding='utf-8'))['frames']:
        assert Path(frame['file']).name == frame['file'] and frame['file'].endswith('.jpg')
        source = capture.parent / frame['file']
        assert sha(source) == frame['sha256']
        target = recording / frame['file']
        shutil.copyfile(source, target)
        assert sha(target) == frame['sha256']
        records.append({'file': target.relative_to(ROOT).as_posix(), 'sha256': sha(target)})
    report = {'passed_original_browser': True, 'counts_as_current_pass': False,
        'reason': 'The shared checker changes; original successful source, captures and raw logs remain separate from a final fresh execution.',
        'source_archive': archive.relative_to(ROOT).as_posix(), 'source_archive_sha256': sha(archive),
        'records': records, 'archive_script_sha256': sha(Path(__file__))}
    (public / 'preservation.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'archived_original_pass': True, 'final_current_pass_credit': False}))


if __name__ == '__main__':
    main()
