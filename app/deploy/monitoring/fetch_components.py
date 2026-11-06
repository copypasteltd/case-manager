"""Fetch only pinned official native archives and selected regular members."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import tarfile
import urllib.request
import uuid
import zipfile

LOCK = Path(__file__).with_name('components.lock.json')


def sha(path):
    with path.open('rb') as handle:
        return hashlib.file_digest(handle, 'sha256').hexdigest()


def fetch(platform: str, destination: Path, cache: Path):
    if platform not in ('windows-amd64', 'linux-amd64') or destination.exists() or destination.is_symlink():
        raise ValueError('NEW_DESTINATION_AND_SUPPORTED_PLATFORM_REQUIRED')
    if not destination.parent.is_dir() or any(p.is_symlink() for p in [destination.parent, *destination.parent.parents]):
        raise ValueError('DESTINATION_PARENT')
    lock = json.loads(LOCK.read_text('utf-8'))
    cache.mkdir(parents=True, exist_ok=True)
    destination.mkdir(mode=0o700)
    receipts = []
    # No command executes and no service starts here.
    for name, item in lock['components'].items():
        if platform not in item['platforms']:
            continue
        definition = item['platforms'][platform]
        prefix = name + '-' + item['version'] + '.' + platform
        archive_name = prefix + '.' + definition['extension']
        archive = cache / archive_name
        url = f'https://github.com/prometheus/{name}/releases/download/v{item["version"]}/{archive_name}'
        if not archive.exists():
            pending = archive.with_suffix(archive.suffix + '.' + uuid.uuid4().hex + '.partial')
            with urllib.request.urlopen(urllib.request.Request(url, headers={'User-Agent': 'CopyPaste-native-monitoring'}), timeout=60) as source, pending.open('xb') as output:
                shutil.copyfileobj(source, output, 1024 * 1024)
            pending.rename(archive)
        if not archive.is_file() or archive.is_symlink() or sha(archive) != definition['sha256']:
            raise ValueError('PINNED_ARCHIVE_CHECKSUM')
        expected = {binary + ('.exe' if platform.startswith('windows') else '') for binary in item['executables']}
        expected |= {'LICENSE', 'NOTICE'}
        members = {}
        if platform.startswith('windows'):
            with zipfile.ZipFile(archive) as source:
                for member in source.infolist():
                    filename = member.filename.removeprefix(prefix + '/')
                    if member.filename == prefix + '/' + filename and filename in expected:
                        if filename in members or member.file_size > 256 * 1024 * 1024 or (member.external_attr >> 16) & 0o170000 == 0o120000:
                            raise ValueError('ARCHIVE_MEMBER')
                        members[filename] = source.read(member)
        else:
            with tarfile.open(archive, 'r:gz') as source:
                for member in source.getmembers():
                    filename = member.name.removeprefix(prefix + '/')
                    if member.name == prefix + '/' + filename and filename in expected:
                        if filename in members or not member.isfile() or member.size > 256 * 1024 * 1024:
                            raise ValueError('ARCHIVE_MEMBER')
                        with source.extractfile(member) as content:
                            members[filename] = content.read()
        if not expected <= members.keys():
            raise ValueError('ARCHIVE_CONTENTS')
        for filename, content in members.items():
            target = destination / (filename if filename not in ('LICENSE', 'NOTICE') else name + '-' + filename)
            with target.open('xb') as handle:
                handle.write(content)
            os.chmod(target, 0o755 if filename not in ('LICENSE', 'NOTICE') else 0o644)
        receipts.append({'component': name, 'version': item['version'], 'archive_sha256': definition['sha256'],
                         'files': {filename: hashlib.sha256(data).hexdigest() for filename, data in sorted(members.items())}})
    result = {'platform': platform, 'lock_sha256': sha(LOCK), 'components': receipts, 'services_started': False}
    (destination / 'components.json').write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--platform', choices=('windows-amd64', 'linux-amd64'), required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--cache', type=Path, required=True)
    args = parser.parse_args()
    try:
        result = fetch(args.platform, args.output, args.cache)
        print(json.dumps({'passed': True, 'platform': args.platform, 'components': len(result['components']), 'services_started': False}))
    except Exception:
        # The operator inspects private dependency logs; never echo token-bearing URLs.
        print(json.dumps({'passed': False, 'code': 'PINNED_NATIVE_COMPONENT_PREPARATION'}))
        raise SystemExit(2)
