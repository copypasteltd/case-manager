"""Source manifest shared by the owned mobile project resources build/browser checks."""
import hashlib
import os
from pathlib import Path

APP = Path(__file__).resolve().parents[1]


def sources():
    names = {'package.json', 'package-lock.json', 'scripts/test_backend.py',
             'scripts/mobile_project_resources_sources.py', 'scripts/run_mobile_project_resources_browser.py',
             'scripts/check_mobile_project_resources.mjs', 'scripts/mobile_picker_browser.mjs', 'scripts/notification_browser_common.mjs'}
    for directory in ('mobile', 'packages/contracts', 'packages/design'):
        for current, directories, files in os.walk(APP / directory):
            directories[:] = [name for name in directories if name not in {'node_modules', 'dist', '.taro', '.swc', 'android', 'native', '.gradle', 'build'}]
            names.update((Path(current) / name).relative_to(APP).as_posix() for name in files)
    for path in (APP / 'backend').rglob('*'):
        if path.is_file() and (path.suffix in ('.go', '.sql') or path.name in ('go.mod', 'go.sum') or path.relative_to(APP).as_posix() == 'backend/internal/contract/openapi.json'):
            names.add(path.relative_to(APP).as_posix())
    return sorted(names)


def digest(names=None):
    value = hashlib.sha256()
    for name in names or sources():
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()
