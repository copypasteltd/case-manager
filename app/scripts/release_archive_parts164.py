"""Lossless, ordered media archive parts for native Git delivery."""
import argparse
import hashlib
import json
from pathlib import Path


def _hash(path):
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            digest.update(block)
    return digest.hexdigest()


def _paths(directory, manifest):
    parts = manifest['parts']
    if not parts or [p['sequence'] for p in parts] != list(range(1, len(parts) + 1)):
        raise ValueError('Missing or unordered archive parts')
    names = [p['name'] for p in parts]
    if len(set(names)) != len(names) or any(Path(n).name != n or '/' in n or '\\' in n or ':' in n or n in ('.', '..') for n in names):
        raise ValueError('Archive parts must have unique local filenames')
    return [(directory / row['name'], row) for row in parts]


def verify_parts(directory, manifest):
    digest = hashlib.sha256()
    size = 0
    limit = manifest['part_bytes_limit']
    if not isinstance(limit, int) or not 0 < limit < 100 * 1024 * 1024:
        raise ValueError('Invalid part size limit')
    for path, row in _paths(directory, manifest):
        if not path.is_file() or not 0 < row['bytes'] <= limit or path.stat().st_size != row['bytes'] or _hash(path) != row['sha256']:
            raise ValueError('Missing, truncated or changed archive part: ' + path.name)
        with path.open('rb') as stream:
            for block in iter(lambda: stream.read(1024 * 1024), b''):
                digest.update(block)
                size += len(block)
    if size != manifest['archive_bytes'] or digest.hexdigest() != manifest['archive_sha256']:
        raise ValueError('Reassembled archive differs from the exact original')
    return True


def package_archive(source, directory, archive_name, limit=90 * 1024 * 1024):
    if Path(archive_name).name != archive_name or '/' in archive_name or '\\' in archive_name or ':' in archive_name:
        raise ValueError('Use a local archive filename')
    if not isinstance(limit, int) or not 0 < limit < 100 * 1024 * 1024:
        raise ValueError('Invalid part size limit')
    rows = []
    with source.open('rb') as stream:
        while True:
            block = stream.read(limit)
            if not block:
                break
            name = archive_name + '.' + str(len(rows) + 1).zfill(3)
            with (directory / name).open('xb') as target:
                target.write(block)
            rows.append({'sequence': len(rows) + 1, 'name': name, 'bytes': len(block), 'sha256': hashlib.sha256(block).hexdigest()})
    manifest = {'archive_name': archive_name, 'archive_bytes': source.stat().st_size, 'archive_sha256': _hash(source), 'part_bytes_limit': limit, 'parts': rows, 'reassembly': 'Concatenate exact parts in sequence; verify full SHA256 before opening.'}
    verify_parts(directory, manifest)
    manifest['reassembly_verified'] = True
    return manifest


def assemble(directory, manifest, output):
    verify_parts(directory, manifest)
    with output.open('xb') as target:
        for path, _ in _paths(directory, manifest):
            with path.open('rb') as source:
                for block in iter(lambda: source.read(1024 * 1024), b''):
                    target.write(block)
    if output.stat().st_size != manifest['archive_bytes'] or _hash(output) != manifest['archive_sha256']:
        raise ValueError('Output checksum mismatch; do not open it')
    return output


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', required=True, type=Path)
    parser.add_argument('--directory', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    manifest = json.loads(args.manifest.read_text(encoding='utf8'))['media_archive']
    result = assemble(args.directory, manifest, args.output)
    print(json.dumps({'passed': True, 'file': str(result), 'sha256': _hash(result)}))
