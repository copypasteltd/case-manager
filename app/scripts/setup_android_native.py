"""Install pinned Windows native build tools inside this checkout, without a device."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import stat
import subprocess
import urllib.request
import zipfile

APP = Path(__file__).resolve().parents[1]
CACHE = APP / '.tools/downloads'
TOOLS = APP / '.tools'
PACKAGES = [
    ('zulu17.70.111-ca-jdk17.0.21-win_x64.zip', 196336246, 'sha256',
     'ed209ff4293ff581c81d3b4268faf108fbe610e6ed43883de9e5f04308d68a72',
     'https://cdn.azul.com/zulu/bin/', 'jdk17', 'zulu17.70.111-ca-jdk17.0.21-win_x64'),
]


def digest(file, algorithm):
    value = hashlib.new(algorithm)
    with file.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''): value.update(block)
    return value.hexdigest()


def download(name, size, algorithm, expected, base, offline):
    target = CACHE / name
    if target.is_file() and target.stat().st_size == size and digest(target, algorithm) == expected:
        return target
    if offline:
        raise RuntimeError('A verified cached tool archive is missing: ' + name)
    temporary = target.with_suffix('.part')
    block = 262144
    with temporary.open('wb') as stream: stream.truncate(size)

    def part(start):
        end = min(size, start + block) - 1
        for attempt in range(3):
            try:
                request = urllib.request.Request(base + name, headers={'Range': f'bytes={start}-{end}'})
                with urllib.request.urlopen(request, timeout=20) as response:
                    if response.status != 206 or response.headers.get('Content-Range') != f'bytes {start}-{end}/{size}':
                        raise RuntimeError('The tool source did not return the requested byte range.')
                    data = response.read(end - start + 1)
                if len(data) != end - start + 1: raise RuntimeError('Incomplete native tool download.')
                with temporary.open('r+b') as stream: stream.seek(start); stream.write(data)
                return
            except Exception:
                if attempt == 2: raise

    # Only one batch is queued; a failed host cannot leave thousands of retries.
    with ThreadPoolExecutor(max_workers=8) as pool:
        for batch in range(0, size, block * 8):
            list(pool.map(part, range(batch, min(size, batch + block * 8), block)))
    if digest(temporary, algorithm) != expected: raise RuntimeError('Native tool publisher checksum mismatch.')
    temporary.replace(target)
    return target


def install(archive, parent, expected_root):
    parent.mkdir(parents=True, exist_ok=True)
    root = parent / expected_root
    with zipfile.ZipFile(archive) as package:
        for item in package.infolist():
            name = PurePosixPath(item.filename)
            mode = item.external_attr >> 16
            if not name.parts or name.parts[0] != expected_root or name.is_absolute() or '..' in name.parts or ':' in item.filename or '\\' in item.filename or stat.S_ISLNK(mode):
                raise RuntimeError('Invalid native tool archive path.')
        if not root.exists(): package.extractall(parent)
        # Do not overwrite an existing installation. Verify executable inputs.
        for item in package.infolist():
            if item.is_dir() or not item.filename.endswith(('.exe', '.dll', 'source.properties')): continue
            target = parent / item.filename
            if not target.is_file(): raise RuntimeError('Incomplete native tool installation.')
            with package.open(item) as source:
                expected = hashlib.file_digest(source, 'sha256').hexdigest()
            if digest(target, 'sha256') != expected: raise RuntimeError('A native tool executable differs from its verified archive.')
    return root


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--offline', action='store_true', help='Require the already verified archive cache.')
    args = parser.parse_args()
    if os.name != 'nt': raise SystemExit('This bootstrap installs Windows native tools only.')
    CACHE.mkdir(parents=True, exist_ok=True)
    installed = []
    for name, size, algorithm, expected, base, destination, root in PACKAGES:
        archive = download(name, size, algorithm, expected, base, args.offline)
        installed.append(install(archive, TOOLS / destination, root))
        print(json.dumps({'package': name, 'publisher_checksum_verified': True}), flush=True)
    result = subprocess.run([str(installed[0] / 'bin/java.exe'), '-version'], capture_output=True, text=True, creationflags=subprocess.CREATE_NO_WINDOW)
    if result.returncode: raise RuntimeError('The JDK version probe failed.')
    print(json.dumps({'passed': True, 'virtualization_used': False, 'device_started': False, 'global_sdk_modified': False}))


if __name__ == '__main__': main()
