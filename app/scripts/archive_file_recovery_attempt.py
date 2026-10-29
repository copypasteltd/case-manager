"""Preserve a failed or incomplete owned attempt without rewriting its result."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
import zipfile
from file_recovery_sources import APP, sources, digest

parser = argparse.ArgumentParser()
parser.add_argument('name')
parser.add_argument('--group', required=True)
parser.add_argument('--reason', required=True)
args = parser.parse_args()
assert all(c.isalnum() or c == '-' for c in args.name + args.group)
root = APP.parent
folder = root / 'docs/quality/evidence/file-recovery/attempts' / args.name
assert not folder.exists()
folder.mkdir(parents=True)
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
archive = APP / '.local' / (args.name + '-source.zip')
names = sources()
with zipfile.ZipFile(archive, 'x', zipfile.ZIP_DEFLATED) as target:
    for name in names:
        target.write(APP / name, name)
previous = root / 'docs/quality/evidence' / args.group
assert previous.resolve().is_relative_to((root / 'docs/quality/evidence').resolve())
assert previous.resolve() != folder.resolve()
records = []
if previous.exists():
    for path in previous.rglob('*'):
        if path.is_file():
            name = path.relative_to(previous)
            (folder / name).parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(path, folder / name)
            records.append({'file': name.as_posix(), 'sha256': sha(path)})
logs = []
for path in sorted((APP / '.local').glob(args.group + '*.log')):
    target = APP / '.local' / (args.name + '-' + path.name)
    shutil.copyfile(path, target)
    logs.append({'file': target.relative_to(APP).as_posix(), 'sha256': sha(target)})
record = {'passed': False, 'archived_at': datetime.now(timezone.utc).isoformat(), 'reason': args.reason,
    'source_sha256': digest(names), 'source_archive': archive.relative_to(APP).as_posix(),
    'source_archive_sha256': sha(archive), 'records': records, 'private_logs': logs,
    'archiver_sha256': sha(Path(__file__)), 'reclassified_as_final_pass': False}
(folder / 'attempt.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
if previous.exists():
    # The exact original public bytes are now in the attempt directory.
    # Remove only those files, then their empty directories.
    for path in sorted(previous.rglob('*'), reverse=True):
        if path.is_file():
            path.unlink()
        elif path.is_dir():
            path.rmdir()
    previous.rmdir()
print(json.dumps({'passed': False, 'attempt': args.name, 'preserved_records': len(records), 'private_logs': len(logs)}))
