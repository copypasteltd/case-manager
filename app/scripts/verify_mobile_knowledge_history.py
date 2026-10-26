"""Bind the retained increment82 capture to its published Git bytes, without rewriting it."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
COMMIT = 'ec8c0fcb142a124e8606dd2e3ff4fa07fe8dba38'
MANIFEST = ROOT / 'docs/quality/evidence/mobile-telegram-browser/source-manifest.json'
OUTPUT = ROOT / 'docs/quality/evidence/mobile-knowledge/historical-telegram-capture.json'


def main():
    manifest = json.loads(MANIFEST.read_text(encoding='utf-8'))
    names = manifest['files']
    assert len(names) == len(set(names)) and all('\n' not in n and '\r' not in n for n in names)
    data = subprocess.check_output(['git', 'cat-file', '--batch'], cwd=ROOT,
        input=''.join(COMMIT + ':app/' + n + '\n' for n in names).encode())
    offset, digest = 0, hashlib.sha256()
    for name in names:
        end = data.index(b'\n', offset)
        header = data[offset:end].split()
        assert len(header) == 3 and header[1] == b'blob', 'Historical source blob missing: ' + name
        size = int(header[2]); value = data[end+1:end+1+size]
        assert len(value) == size and data[end+1+size:end+2+size] == b'\n'
        digest.update(name.encode() + b'\0' + value + b'\0'); offset = end+2+size
    assert offset == len(data) and digest.hexdigest() == manifest['sha256']
    report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'commit': COMMIT,
        'sources': len(names), 'source_sha256': digest.hexdigest(), 'all_sources_from_git': True,
        'manifest_sha256': hashlib.sha256(MANIFEST.read_bytes()).hexdigest(),
        'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'current_source_claim': False, 'historical_evidence_modified': False}
    OUTPUT.write_text(json.dumps(report, indent=2)+'\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'historical_sources': len(names), 'commit': COMMIT}))


if __name__ == '__main__':
    main()
