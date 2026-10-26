"""Bind repository screenshots and recordings to the actual application sources."""
import hashlib
import os
from pathlib import Path

APP = Path(__file__).resolve().parents[1]


def sources():
    names = {'package.json', 'package-lock.json', 'scripts/test_backend.py',
             'scripts/repository_showcase_sources.py', 'scripts/run_repository_showcase.py',
             'scripts/capture_repository_showcase.mjs', 'scripts/repository_recording.mjs',
             'scripts/notification_browser_common.mjs', 'scripts/mobile_picker_browser.mjs'}
    for directory in ('web', 'mobile', 'packages/contracts', 'packages/design'):
        for current, directories, files in os.walk(APP / directory):
            directories[:] = [d for d in directories if d not in
                              {'node_modules', 'dist', 'build', '.taro', '.swc', '.gradle', 'android', 'native'}]
            names.update((Path(current) / name).relative_to(APP).as_posix() for name in files)
    for p in (APP / 'backend').rglob('*'):
        if p.is_file() and (p.suffix in ('.go', '.sql') or p.name in ('go.mod', 'go.sum')
                            or p.relative_to(APP).as_posix() == 'backend/internal/contract/openapi.json'):
            names.add(p.relative_to(APP).as_posix())
    return sorted(names)


def digest(names=None):
    value = hashlib.sha256()
    for name in names or sources():
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()
