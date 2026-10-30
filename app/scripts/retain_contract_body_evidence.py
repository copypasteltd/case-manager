"""Keep published media and prior proofs exact; disclose the one existing hash dependency declaration."""
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/contract-body'
PUBLISHED = '30c364a7794c3fe7049c6ab04c4ba5b41d306ec3'


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def main():
    media = []
    for row in git('ls-tree', '-rz', PUBLISHED, '--', 'docs/media/screenshots', 'docs/media/videos', 'docs/media/gifs').split(b'\0'):
        if not row:
            continue
        metadata, name = row.split(b'\t', 1)
        name = name.decode('utf-8')
        if Path(name).suffix not in ('.png', '.mp4', '.gif'):
            continue
        blob, data = metadata.split()[2].decode(), (ROOT / name).read_bytes()
        assert hashlib.sha1(b'blob ' + str(len(data)).encode() + b'\0' + data).hexdigest() == blob, name
        media.append({'file': name, 'git_blob': blob, 'sha256': sha(data), 'bytes': len(data)})
    assert len(media) == 296
    names = ['docs/quality/evidence/dependency-integrity/' + n for n in ('dependency-audit.json', 'audit-after.json', 'security-checks.json')]
    names += ['docs/quality/evidence/payment-fx/' + n for n in ('client-checks.json', 'final-source-check.json', 'media.json')]
    names += ['docs/quality/evidence/payment_fx-backend/result.json']
    records = []
    for name in names:
        data = git('show', PUBLISHED + ':' + name)
        assert data == (ROOT / name).read_bytes()
        records.append({'file': name, 'sha256': sha(data)})
    name = 'app/package.json'
    assert git('show', PUBLISHED + ':' + name) == (ROOT / name).read_bytes()
    name = 'app/mobile/package.json'
    previous, current = json.loads(git('show', PUBLISHED + ':' + name)), json.loads((ROOT / name).read_bytes())
    assert current['scripts']['test'] == previous['scripts']['test'] + ' ../.local/mobile-concern-tests/mobile/src/platform/contract-body.test.js'
    current['scripts']['test'] = previous['scripts']['test']
    assert current == previous
    name = 'app/packages/contracts/package.json'
    previous, current = json.loads(git('show', PUBLISHED + ':' + name)), json.loads((ROOT / name).read_bytes())
    assert current['exports'].pop('./contract-body') == './src/contract-body.ts'
    assert current.pop('dependencies') == {'@noble/hashes': '2.4.0'}
    assert current == previous
    name = 'app/package-lock.json'
    previous, current = json.loads(git('show', PUBLISHED + ':' + name)), json.loads((ROOT / name).read_bytes())
    assert previous['packages']['node_modules/@noble/hashes']['version'] == '2.4.0'
    assert current['packages']['packages/contracts'].pop('dependencies') == {'@noble/hashes': '2.4.0'}
    assert current == previous
    assert not git('diff', '--name-only', PUBLISHED, '--', 'docs/quality/evidence/dependency-integrity').strip()
    report = {'passed': True, 'published_commit': PUBLISHED, 'media': sorted(media, key=lambda v: v['file']), 'records': records,
        'package_lock_sha256': sha((ROOT / name).read_bytes()), 'resolved_dependency_packages_unchanged': True,
        'declared_existing_dependency': '@noble/hashes 2.4.0', 'new_dependency_audit_record': 'dependency-audit.json',
        'scope': '296 historical media and original evidence retain their exact bytes and dates. Existing resolved packages are unchanged; contracts explicitly declares its hash dependency and mobile selects nine new tests. Fresh current-lock audit is recorded separately.'}
    (E / 'reused-evidence.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'retained_media': len(media), 'resolved_dependency_packages_changed': False}))


if __name__ == '__main__':
    main()
