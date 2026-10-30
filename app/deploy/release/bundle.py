"""Bounded, deterministic production bundles. No extraction of archive metadata."""
import gzip
import argparse
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import sys
import tarfile

MAX_FILES = 20000
MAX_BYTES = 1024 << 20
MAX_FILE_BYTES = 128 << 20
MANIFEST = 'release.json'
PUBLIC_DOTFILES = {'.runkit_example.js', '.eslintrc.yml'}


class ReleaseError(Exception):
    """Only fixed, non-secret diagnoses cross the operator/log boundary."""


def require(ok, message):
    if not ok:
        raise ReleaseError(message)


def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False, allow_nan=False) + '\n').encode('utf-8')


def digest(data):
    return hashlib.sha256(data).hexdigest()


def file_sha(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''):
            h.update(block)
    return h.hexdigest()


def unique_pairs(pairs):
    value = {}
    for key, item in pairs:
        require(key not in value, 'Duplicate JSON member refused.')
        value[key] = item
    return value


def read_json(raw):
    try:
        return json.loads(raw, object_pairs_hook=unique_pairs,
                          parse_constant=lambda _: require(False, 'Non-finite JSON refused.'))
    except (ValueError, TypeError, UnicodeError) as error:
        raise ReleaseError('Invalid JSON document.') from error


def safe_name(name):
    require(isinstance(name, str) and len(name) <= 512 and name.isascii()
            and not any(c in name for c in '\\:\x00\r\n')
            and all(re.fullmatch(r'[A-Za-z0-9_@.+-]+', p) and p not in ('.', '..')
                    and (not p.startswith('.') or p in PUBLIC_DOTFILES) for p in name.split('/'))
            and PurePosixPath(name).as_posix() == name, 'Unsafe package member.')
    return name


def inventory(root):
    root = Path(root)
    require(root.is_dir() and not root.is_symlink(), 'Package root must be a real directory.')
    out = []
    for path in sorted(root.rglob('*'), key=lambda p: p.relative_to(root).as_posix()):
        mode = path.lstat().st_mode
        require(not stat.S_ISLNK(mode), 'Package symlink refused.')
        if stat.S_ISDIR(mode):
            continue
        require(stat.S_ISREG(mode), 'Special package member refused.')
        name = safe_name(path.relative_to(root).as_posix())
        if name == MANIFEST:
            continue
        size = path.stat().st_size
        require(size <= MAX_FILE_BYTES, 'Package member exceeds bound.')
        out.append({'path': name, 'bytes': size, 'sha256': file_sha(path),
                    'mode': 365 if name.startswith('bin/') else 292})  # 0555/0444
    require(0 < len(out) <= MAX_FILES and sum(v['bytes'] for v in out) <= MAX_BYTES, 'Package inventory exceeds bound.')
    return out


def validate_manifest(value):
    require(isinstance(value, dict) and set(value) == {'format', 'target', 'source', 'schema', 'files'}, 'Unsupported release manifest.')
    require(type(value['format']) is int and value['format'] == 1 and value['target'] == 'linux-amd64', 'Unsupported release target.')
    source = value['source']
    require(isinstance(source, dict) and set(source) == {'base_commit', 'inputs_sha256', 'working_tree_dirty'}
            and re.fullmatch(r'[0-9a-f]{40}', source['base_commit'])
            and re.fullmatch(r'[0-9a-f]{64}', source['inputs_sha256'])
            and type(source['working_tree_dirty']) is bool, 'Invalid source identity.')
    files, schema = value['files'], value['schema']
    require(isinstance(files, list) and 0 < len(files) <= MAX_FILES and isinstance(schema, list) and 0 < len(schema) <= 1000, 'Invalid release inventory.')
    names = []
    for f in files:
        require(isinstance(f, dict) and set(f) == {'path', 'bytes', 'sha256', 'mode'}, 'Invalid member description.')
        name = safe_name(f['path'])
        require(name != MANIFEST and type(f['bytes']) is int and 0 <= f['bytes'] <= MAX_FILE_BYTES
                and re.fullmatch(r'[0-9a-f]{64}', f['sha256']) and f['mode'] == (365 if name.startswith('bin/') else 292), 'Invalid member integrity.')
        require(name.startswith(('bin/', 'web/', 'runtime/', 'systemd/', 'metadata/')), 'Unapproved package section.')
        require(not any(p in ('.local', '.tools', '.git') or p.startswith('.env') for p in name.split('/')), 'Private package member refused.')
        names.append(name)
    require(names == sorted(set(names)) and sum(f['bytes'] for f in files) <= MAX_BYTES, 'Duplicate or oversized inventory.')
    require({'bin/platform', 'bin/agent-native', 'bin/status-bridge', 'web/index.html',
             'runtime/app/agent-runtime/dist/worker.js', 'metadata/runtime.json'} <= set(names), 'Required release component missing.')
    previous = ''
    for row in schema:
        require(isinstance(row, dict) and set(row) == {'name', 'sha256'}
                and re.fullmatch(r'[0-9]{4}_[a-z0-9_]+\.sql', row['name'])
                and row['name'] > previous and re.fullmatch(r'[0-9a-f]{64}', row['sha256']), 'Invalid schema inventory.')
        previous = row['name']
    return value


def verify_tree(root, expected):
    raw = (Path(root) / MANIFEST).read_bytes()
    require(len(raw) <= 8 << 20 and digest(raw) == expected, 'Release manifest digest differs.')
    value = validate_manifest(read_json(raw))
    require(inventory(root) == value['files'], 'Release inventory differs from actual files.')
    return value


def write_archive(root, output):
    """Unique output, fixed timestamps/ownership/modes, no host path metadata."""
    root, output = Path(root), Path(output)
    raw = (root / MANIFEST).read_bytes()
    value = verify_tree(root, digest(raw))
    with output.open('xb') as target, gzip.GzipFile(filename='', mode='wb', fileobj=target, mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode='w|', format=tarfile.USTAR_FORMAT) as archive:
            for row in [*value['files'], {'path': MANIFEST, 'mode': 292}]:
                data = (root / row['path']).read_bytes()
                member = tarfile.TarInfo(row['path'])
                member.size, member.mode, member.mtime = len(data), row['mode'], 0
                archive.addfile(member, io.BytesIO(data))
    return {'archive_sha256': file_sha(output), 'manifest_sha256': digest(raw), 'release_id': 'r-' + digest(raw)[:24]}


def extract_archive(archive_path, destination, expected):
    """Copy regular bytes only into a new private directory; never tar.extract."""
    require(re.fullmatch(r'[a-f0-9]{64}', expected) is not None, 'Expected manifest digest required.')
    destination = Path(destination)
    require(not destination.exists() and not destination.is_symlink(), 'Extraction destination already exists.')
    destination.mkdir(mode=0o700)
    seen, total = set(), 0
    with tarfile.open(archive_path, mode='r|gz') as archive:
        for member in archive:
            name = safe_name(member.name)
            require(name not in seen and member.isreg() and not member.pax_headers
                    and 0 <= member.size <= (8 << 20 if name == MANIFEST else MAX_FILE_BYTES), 'Unsafe or duplicate archive member.')
            seen.add(name)
            total += member.size
            require(len(seen) <= MAX_FILES + 1 and total <= MAX_BYTES + (8 << 20), 'Archive exceeds bound.')
            target = destination / name
            target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
            with archive.extractfile(member) as source, target.open('xb') as output:
                while block := source.read(1 << 20):
                    output.write(block)
                output.flush()
                os.fsync(output.fileno())
    return verify_tree(destination, expected)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=('inspect',))
    parser.add_argument('--archive', type=Path, required=True)
    parser.add_argument('--manifest-sha256', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    value = extract_archive(args.archive, args.output, args.manifest_sha256)
    print(json.dumps({'passed': True, 'release_id': 'r-' + args.manifest_sha256[:24], 'files': len(value['files']),
                      'schema_versions': len(value['schema']), 'deployment_executed': False}))


if __name__ == '__main__':
    try:
        main()
    except (ReleaseError, OSError, ValueError, tarfile.TarError):
        print('Package inspection failed; retain the private attempt directory.', file=sys.stderr)
        raise SystemExit(1)
