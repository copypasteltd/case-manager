"""Preserve published media and unchanged dependency provenance at their actual scope."""
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/audit-console'
PUBLISHED = '66b7707fc363f7021bf4de62ab515a2dbba236b0'


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def main():
    names = git('ls-tree', '-rz', '--name-only', PUBLISHED, '--', 'docs/media/screenshots', 'docs/media/videos', 'docs/media/gifs').decode().split('\0')
    media = []
    for name in sorted(v for v in names if Path(v).suffix in ('.png', '.mp4', '.gif')):
        file = ROOT / name
        blob = git('rev-parse', PUBLISHED + ':' + name).decode().strip()
        assert git('hash-object', '--', name).decode().strip() == blob, name
        media.append({'file':name, 'git_blob':blob, 'sha256':sha(file.read_bytes()), 'bytes':file.stat().st_size})
    assert len(media) == 211
    records = []
    for name in ('docs/quality/evidence/dependency-integrity/dependency-audit.json',
                 'docs/quality/evidence/dependency-integrity/audit-after.json',
                 'docs/quality/evidence/dependency-integrity/security-checks.json'):
        previous = git('show', PUBLISHED + ':' + name)
        assert previous == (ROOT / name).read_bytes()
        records.append({'file':name, 'sha256':sha(previous)})
    for name in ('app/package.json', 'app/package-lock.json'):
        assert git('show', PUBLISHED + ':' + name) == (ROOT / name).read_bytes()
    # The new shared contract export changes an entry point, not dependency resolution.
    for name in ('app/mobile/package.json', 'app/packages/contracts/package.json'):
        before = json.loads(git('show', PUBLISHED + ':' + name)); after = json.loads((ROOT / name).read_bytes())
        for field in ('dependencies', 'devDependencies', 'peerDependencies', 'optionalDependencies', 'engines'):
            assert before.get(field) == after.get(field)
    assert not git('diff', '--name-only', PUBLISHED, '--', 'docs/quality/evidence/dependency-integrity').strip()
    record = {'passed':True, 'published_commit':PUBLISHED, 'media':media, 'records':records,
        'media_counts':{suffix:sum(Path(v['file']).suffix == suffix for v in media) for suffix in ('.png', '.mp4', '.gif')},
        'package_lock_sha256':sha((ROOT / 'app/package-lock.json').read_bytes()),
        'new_dependency_audit':False, 'new_dependency_security_test_execution':False,
        'scope':'211 historical media retain exact published bytes and original capture scope. Dependency audit/security results retain increment108 execution and date; root manifest, lock and workspace dependency fields are unchanged. Current feature tests, builds and new captures are recorded separately.'}
    E.mkdir(parents=True, exist_ok=True)
    (E / 'reused-evidence.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed':True, 'retained_media':len(media), 'retained_records':len(records), 'new_dependency_audit':False}))


if __name__ == '__main__':
    main()
