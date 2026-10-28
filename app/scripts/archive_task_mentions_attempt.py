"""Retain failed/superseded owned browser attempts and their exact input bytes."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
import zipfile
from task_mentions_sources import APP, digest

parser = argparse.ArgumentParser()
parser.add_argument('--client', choices=['web', 'mobile'], required=True)
parser.add_argument('--reason', required=True)
args = parser.parse_args()
root = APP.parent
group = 'task-mentions-' + args.client
source = (root / 'docs/quality/evidence' / group).resolve()
history = (root / 'docs/quality/evidence/task-mentions/history/browser-attempts').resolve()
assert source.is_relative_to((root / 'docs/quality/evidence').resolve())
assert source.name == group and source.is_dir()
manifest = json.loads((source / 'source-manifest.json').read_text())
assert digest(manifest['files']) == manifest['sha256'], 'Archive before changing the exact attempt inputs.'
history.mkdir(parents=True, exist_ok=True)
target = history / (str(len(list(history.iterdir())) + 1).zfill(2) + '-' + group)
assert not target.exists() and target.is_relative_to(history)
shutil.move(str(source), str(target))
with zipfile.ZipFile(target / 'source.zip', 'w', zipfile.ZIP_DEFLATED) as archive:
    for name in manifest['files']:
        archive.writestr(name, (APP / name).read_bytes())
media = []
for directory in ['screenshots', 'videos', 'gifs']:
    for path in (root / 'docs/media' / directory).glob(group + '-*'):
        saved = target / 'media' / directory / path.name
        saved.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(path, saved)
        media.append({'path': str(path.relative_to(root)).replace('\\', '/'), 'sha256': hashlib.sha256(path.read_bytes()).hexdigest()})
(target / 'attempt.json').write_text(json.dumps({'archived_at': datetime.now(timezone.utc).isoformat(), 'reason': args.reason,
    'source_sha256': manifest['sha256'], 'source_entries': len(manifest['files']), 'media': media,
    'archive_sha256': hashlib.sha256((target / 'source.zip').read_bytes()).hexdigest()}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'retained': str(target.relative_to(root)), 'source_entries': len(manifest['files']), 'media': len(media)}))
