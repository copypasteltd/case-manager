"""Keep failed real browser attempts and exact source bytes before bounded repairs."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
import zipfile
from payment_fee_sources import APP, digest, sources

ROOT = APP.parent


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('label')
    parser.add_argument('--mobile', action='store_true')
    args = parser.parse_args()
    assert args.label.replace('-', '').isalnum()
    group = 'payment-fee-' + ('mobile' if args.mobile else 'web')
    original = ROOT / 'docs/quality/evidence' / group
    result = json.loads((original / 'result.json').read_text(encoding='utf-8'))
    assert not result['passed'] and result['owned_review_server_stopped'] and result['owned_browser_closed']
    assert result['build_source_sha256'] == result['build_source_after_sha256'] == digest()
    names = set(sources())
    if (original / 'source-manifest.json').exists():
        names.update(json.loads((original / 'source-manifest.json').read_text(encoding='utf-8'))['files'])
    private = APP / '.local/payment-fee-attempts' / args.label
    public = ROOT / 'docs/quality/evidence/payment-fee-attempts' / args.label
    assert not private.exists() and not public.exists()
    private.mkdir(parents=True)
    public.mkdir(parents=True)
    archive = private / 'source.zip'
    with zipfile.ZipFile(archive, 'x', zipfile.ZIP_DEFLATED) as output:
        for name in sorted(names):
            path = (APP / name).resolve()
            assert path.is_relative_to(ROOT.resolve()) and path.is_file()
            output.write(path, path.relative_to(ROOT.resolve()).as_posix())
    records, logs = [], []
    for path in sorted(original.iterdir()):
        assert path.is_file()
        target = (public if path.suffix == '.json' else private) / path.name
        shutil.move(path, target)
        (records if path.suffix == '.json' else logs).append({'file': target.relative_to(ROOT).as_posix(), 'sha256': sha(target)})
    for path in sorted((APP / '.local').glob(group + '-*.log')):
        target = private / path.name
        shutil.move(path, target)
        logs.append({'file': target.relative_to(ROOT).as_posix(), 'sha256': sha(target)})
    # Freeze screenshots and raw pixel folders before another attempt can reuse output names.
    for path in sorted((ROOT / 'docs/media/screenshots').glob(group + '-*.png')):
        target = private / 'screenshots' / path.name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(path, target)
        logs.append({'file': target.relative_to(ROOT).as_posix(), 'sha256': sha(target)})
    for folder in sorted((APP / '.local/repository-showcase').glob(group + '-*')):
        if not folder.is_dir():
            continue
        target = private / 'recordings' / folder.name
        shutil.copytree(folder, target)
        for path in sorted(target.rglob('*')):
            if path.is_file():
                logs.append({'file': path.relative_to(ROOT).as_posix(), 'sha256': sha(path)})
    report = {'archived_at': datetime.now(timezone.utc).isoformat(), 'passed': False, 'counts_as_current_pass': False,
        'source_archive': archive.relative_to(ROOT).as_posix(), 'source_archive_sha256': sha(archive),
        'interaction_source_sha256': digest(), 'source_files': sorted(names),
        'public_records': records, 'private_records': logs, 'archive_script_sha256': sha(Path(__file__))}
    (public / 'preservation.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'archived': args.label, 'source_files': len(names), 'pass_claim': False}))


if __name__ == '__main__':
    main()
