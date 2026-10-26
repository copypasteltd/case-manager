"""Reconstruct retained increment83 capture from its original Git blobs, never current modified sources."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
COMMIT = 'afbb92608440c70a295623ff293b323bf07fd55b'
MANIFEST = 'docs/quality/evidence/mobile-knowledge-browser/source-manifest.json'
RETAINED = [MANIFEST, 'docs/quality/evidence/mobile-knowledge-browser/result.json',
    'docs/quality/evidence/mobile-knowledge/final-source-check.json',
    'docs/quality/evidence/mobile-knowledge/source-normalization.json',
    'docs/quality/evidence/mobile-knowledge/verification.md']


def reconstruct():
    manifest = json.loads(subprocess.check_output(['git', 'show', COMMIT + ':' + MANIFEST], cwd=ROOT))
    names = ['app/' + name for name in manifest['files']] + RETAINED
    requests = ''.join(COMMIT + ':' + name + '\n' for name in names).encode()
    output = subprocess.run(['git', 'cat-file', '--batch'], cwd=ROOT, input=requests, stdout=subprocess.PIPE, check=True).stdout
    offset, blobs = 0, {}
    for name in names:
        end = output.index(b'\n', offset)
        header = output[offset:end].split()
        assert len(header) == 3 and header[1] == b'blob'
        size = int(header[2]); offset = end + 1
        blobs[name] = output[offset:offset + size]; offset += size + 1
    assert offset == len(output)
    h = hashlib.sha256()
    for name in manifest['files']:
        h.update(name.encode() + b'\0' + blobs['app/' + name] + b'\0')
    assert h.hexdigest() == manifest['sha256']
    retained = []
    for name in RETAINED:
        assert (ROOT / name).read_bytes() == blobs[name]
        retained.append({'file': name, 'sha256': hashlib.sha256(blobs[name]).hexdigest()})
    original = json.loads(blobs[RETAINED[1]])
    assert original['passed'] and original['source_sha256'] == h.hexdigest()
    return {'passed': True, 'commit': COMMIT, 'source_count': len(manifest['files']),
        'source_sha256': h.hexdigest(), 'retained_files': retained,
        'scope': 'Retained increment83 capture reconstructed from exact original Git blobs; no current-source or new-execution claim.'}


def main():
    value = reconstruct()
    value['recorded_at'] = datetime.now(timezone.utc).isoformat()
    target = ROOT / 'docs/quality/evidence/project-concerns/historical-83.json'
    target.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({k: value[k] for k in ('passed', 'source_count', 'source_sha256')}))


if __name__ == '__main__':
    main()
