"""Preserve a completed failed browser attempt and verify its exact source bytes."""
import argparse
import hashlib
import json
from pathlib import Path
import zipfile

ROOT = Path(__file__).resolve().parents[2]
APP = ROOT / 'app'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('number', type=int)
    parser.add_argument('label')
    parser.add_argument('--mobile', action='store_true')
    args = parser.parse_args()
    assert 1 <= args.number <= 99 and args.label.replace('-', '').isalnum()
    client = 'mobile' if args.mobile else 'web'
    group = 'payment-fx-' + client
    source = ROOT / 'docs/quality/evidence' / group
    result = json.loads((source / 'result.json').read_text(encoding='utf-8'))
    assert not result['passed'] and result['owned_browser_closed'] and result['owned_review_server_stopped'] and result['isolated_database_cleanup_passed']
    assert not (APP / '.local/business-browser-fixture.json').exists()
    manifest = json.loads((source / 'source-manifest.json').read_text(encoding='utf-8'))
    name = f'{args.number:02d}-' + args.label
    archive = APP / '.local/payment_fx-attempts' / (name + '.zip')
    archive.parent.mkdir(parents=True, exist_ok=True)
    assert not archive.exists()
    h = hashlib.sha256()
    with zipfile.ZipFile(archive, 'x', zipfile.ZIP_DEFLATED) as z:
        for file in manifest['files']:
            path = (APP / file).resolve()
            assert path.is_relative_to(APP.resolve())
            data = path.read_bytes()
            h.update(file.encode() + b'\0' + data + b'\0')
            z.writestr(file, data)
    assert h.hexdigest() == manifest['sha256'], 'Source changed during archiving; preserve this archive and reconstruct exact prior inputs separately.'
    target = ROOT / 'docs/quality/evidence/payment-fx/attempts' / name
    assert source.resolve().is_relative_to((ROOT / 'docs/quality/evidence').resolve())
    assert target.resolve().is_relative_to((ROOT / 'docs/quality/evidence/payment-fx/attempts').resolve())
    target.mkdir(parents=True, exist_ok=False)
    source.rename(target / client)
    logs = []
    for suffix in ('native', 'browser', 'build'):
        old = APP / '.local' / (group + '-' + suffix + '.log')
        saved = archive.parent / (name + '-' + suffix + '.log')
        assert not saved.exists()
        saved.write_bytes(old.read_bytes())
        logs.append({'path': saved.relative_to(ROOT).as_posix(), 'sha256': hashlib.sha256(saved.read_bytes()).hexdigest()})
    (target / 'attempt.json').write_text(json.dumps({'passed': False, 'counts_as_passed': False,
        'source_sha256': h.hexdigest(), 'source_binding_verified': True,
        'private_source_archive': archive.relative_to(ROOT).as_posix(), 'archive_sha256': hashlib.sha256(archive.read_bytes()).hexdigest(),
        'private_logs': logs, 'failure': result.get('failure')}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'preserved': True, 'attempt': name, 'source_verified': True}))


if __name__ == '__main__':
    main()
