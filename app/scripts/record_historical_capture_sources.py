"""Record exact historical Git sources and an explicitly ignored generated-cache supplement."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path

from verify_repository_publication import APP, ROOT, git, object_bytes, read, sha

ALLOWED_CACHE = {'web/tsconfig.tsbuildinfo'}


def main():
    baseline = read(ROOT / 'docs/quality/current-functional-baseline.json')
    commit = baseline['historical_capture_commit']
    manifest_path = ROOT / 'docs/quality/evidence/repository-showcase/source-manifest.json'
    manifest = read(manifest_path)
    tracked = {name.decode('utf-8') for name in git('ls-tree', '-rz', '--name-only', commit).split(b'\0') if name}
    committed = [name for name in manifest['files'] if 'app/' + name in tracked]
    missing = set(manifest['files']) - set(committed)
    assert missing == ALLOWED_CACHE, 'Only the known ignored generated cache can supplement this capture.'
    blobs = object_bytes(['app/' + name for name in committed], commit + ':')
    original, hashes, entries = hashlib.sha256(), hashlib.sha256(), []
    for name in manifest['files']:
        if name in missing:
            assert git('check-ignore', '--', 'app/' + name).strip() == ('app/' + name).encode()
            data, origin = (APP / name).read_bytes(), 'ignored-generated-cache'
        else:
            data, origin = blobs['app/' + name], 'historical-git-blob'
        value = hashlib.sha256(data).hexdigest()
        original.update(name.encode() + b'\0' + data + b'\0')
        hashes.update(name.encode() + b'\0' + value.encode() + b'\0')
        entries.append({'name': name, 'sha256': value, 'bytes': len(data), 'origin': origin})
    assert original.hexdigest() == manifest['sha256'], 'Original byte-stream source digest must reconstruct exactly.'
    report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'source_commit': commit,
              'original_capture_source_sha256': manifest['sha256'], 'reconstructed_source_sha256': original.hexdigest(),
              'original_manifest_sha256': sha(manifest_path), 'per_file_hash_manifest_sha256': hashes.hexdigest(),
              'source_count': len(entries), 'git_source_count': len(committed), 'generated_cache_count': len(missing),
              'generated_cache_bytes_published': False, 'files': entries,
              'preparation_script_sha256': sha(Path(__file__)),
              'scope': 'At preparation time, reconstructed the exact original source byte-stream from historical Git blobs plus the unchanged ignored cache. Future clones check Git blob hashes and retain the recorded cache hash; generated cache bytes are intentionally absent from Git.'}
    target = ROOT / baseline['historical_capture_source_record']
    assert not target.exists(), 'Never overwrite the source reconstruction receipt.'
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'sources': len(entries), 'git_sources': len(committed),
                      'ignored_generated_supplements': sorted(missing), 'original_digest_reconstructed': True}))


if __name__ == '__main__':
    main()
