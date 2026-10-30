"""Verify unchanged published media and disclose reused dependency-only evidence."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/payment-fee'
PUBLISHED = 'e4a66e32cc4a2bf1a663506381a2be6f3ebeb15c'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT)


def main():
    media = []
    for row in git('ls-tree', '-rz', PUBLISHED, '--', 'docs/media/screenshots', 'docs/media/videos', 'docs/media/gifs').split(b'\0'):
        if not row:
            continue
        metadata, name = row.split(b'\t', 1)
        name = name.decode('utf-8')
        if Path(name).suffix not in ('.png', '.mp4', '.gif'):
            continue
        path = ROOT / name
        blob = metadata.split()[2].decode()
        assert hashlib.sha1(b'blob ' + str(path.stat().st_size).encode() + b'\0' + path.read_bytes()).hexdigest() == blob
        media.append({'file': name, 'git_blob': blob, 'sha256': sha(path), 'bytes': path.stat().st_size})
    assert len(media) == 306
    names = ['docs/quality/evidence/contract-body/' + n for n in ('client-checks.json', 'final-source-check.json', 'media.json', 'dependency-audit.json')]
    names += ['docs/quality/evidence/contract_body-backend/result.json']
    names += ['docs/quality/evidence/dependency-integrity/' + n for n in ('dependency-audit.json', 'audit-after.json', 'security-checks.json')]
    records = []
    for name in names:
        assert git('show', PUBLISHED + ':' + name) == (ROOT / name).read_bytes()
        records.append({'file': name, 'sha256': sha(ROOT / name)})
    for name in ('app/package.json', 'app/package-lock.json'):
        assert git('show', PUBLISHED + ':' + name) == (ROOT / name).read_bytes()
    old = json.loads(git('show', PUBLISHED + ':app/packages/contracts/package.json'))
    now = json.loads((ROOT / 'app/packages/contracts/package.json').read_text())
    assert now['exports'].pop('./payment-fees') == './src/payment-fees.ts' and old == now
    old = json.loads(git('show', PUBLISHED + ':app/mobile/package.json'))
    now = json.loads((ROOT / 'app/mobile/package.json').read_text())
    assert now['scripts']['test'] == old['scripts']['test'] + ' ../.local/mobile-concern-tests/mobile/src/platform/payment-fees.test.js'
    now['scripts']['test'] = old['scripts']['test']
    assert now == old
    previous = ROOT / 'docs/quality/evidence/contract-body/dependency-audit.json'
    audit = json.loads(previous.read_text())
    assert audit['passed'] and audit['vulnerabilities']['total'] == 0 and audit['package_lock_sha256'] == sha(ROOT / 'app/package-lock.json')
    audit.update({'reused_from': previous.relative_to(ROOT).as_posix(), 'reused_record_sha256': sha(previous),
        'reuse_checked_at': datetime.now(timezone.utc).isoformat(), 'executed_now': False,
        'scope': 'Published117 zero-advisory result reused for the byte-identical lock; no dependency changes or fresh advisory query. Formal artifact security remains GAP-26.'})
    (E / 'dependency-audit.json').write_text(json.dumps(audit, indent=2) + '\n', encoding='utf-8')
    report = {'passed': True, 'published_commit': PUBLISHED, 'media': media, 'records': records,
        'package_lock_sha256': sha(ROOT / 'app/package-lock.json'), 'resolved_dependency_packages_unchanged': True,
        'scope': '306 published media and original117 proof records remain byte-exact. The current feature adds one package export and eight shared test selections; installed/resolved dependencies are unchanged.'}
    (E / 'reused-evidence.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    preliminary = []
    for path in sorted((ROOT / 'app/.local').glob('payment-fee-*')):
        if path.is_file() and (path.name.startswith('payment-fee-targeted-v') or path.name.startswith('payment-fee-preliminary-')):
            preliminary.append({'file': path.relative_to(ROOT).as_posix(), 'sha256': sha(path)})
    for path in sorted((ROOT / 'app/.local/payment-fee-attempts').rglob('*')):
        if path.is_file():
            preliminary.append({'file': path.relative_to(ROOT).as_posix(), 'sha256': sha(path)})
    (E / 'preliminary-checks.json').write_text(json.dumps({'counts_as_final_regression': False, 'records': preliminary,
        'scope': 'One initial compile failure, successful targeted backend/Web/type checks, and the first read-only development check with its stale schema expectation. Only the separate complete unchanged-source final commands count as regression evidence.'}, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'retained_media': len(media), 'dependency_query_executed_now': False}))


if __name__ == '__main__':
    main()
