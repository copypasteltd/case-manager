"""Freeze the discovered history-contract regression before fixing its source."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import zipfile
from refund_fx_sources import APP, sources, digest

ROOT = APP.parent
PUBLIC = ROOT / 'docs/quality/evidence/refund-fx-attempts/06-history-contract'
PRIVATE = APP / '.local/refund-fx-attempts/06-history-contract'
assert not PUBLIC.exists() and not PRIVATE.exists()
assert not (APP / '.local/business-browser-fixture.json').exists()
state = json.loads((PUBLIC.parent.parent / 'refund-fx-backend/batch-progress.json').read_text())
assert state['batches'][8]['status'] == 'failed'
assert not (PUBLIC.parent.parent / 'refund-fx/build.json').exists()
PUBLIC.mkdir(parents=True)
PRIVATE.mkdir(parents=True)
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
names = sorted(set(sources()) | {p.relative_to(APP).as_posix() for p in (APP / 'scripts').glob('*refund_fx*') if p.is_file()})
archive = PRIVATE / 'source.zip'
with zipfile.ZipFile(archive, 'x', zipfile.ZIP_DEFLATED) as z:
    for n in names:
        z.write(APP / n, (APP / n).relative_to(ROOT).as_posix())
records = []
def move(source, target):
    assert source.resolve().is_relative_to(ROOT.resolve()) and target.resolve().is_relative_to(ROOT.resolve())
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.move(source, target)
    items = [target] if target.is_file() else target.rglob('*')
    for p in items:
        if p.is_file(): records.append({'file':p.relative_to(ROOT).as_posix(),'sha256':sha(p)})
for group in ('refund-fx', 'refund-fx-backend'):
    path = PUBLIC.parent.parent / group
    assert not subprocess.check_output(['git','ls-files','--',str(path.relative_to(ROOT))],cwd=ROOT).strip()
    move(path, PUBLIC / group)
for pattern in ('backend-batch-*.log','refund_fx-client-*.log','refund_fx-skill-check.log','refund_fx-android-final.log'):
    for path in (APP / '.local').glob(pattern): move(path, PRIVATE / path.name)
move(APP / '.local/android/bundle.log', PRIVATE / 'bundle.log')
move(APP / '.local/refund-fx-final-sources.zip', PRIVATE / 'earlier-final-sources.zip')
for path in (ROOT / 'docs/media').glob('**/refund-fx-*'):
    if path.is_file():
        target = PRIVATE / 'media' / path.parent.name / path.name
        target.parent.mkdir(parents=True,exist_ok=True)
        shutil.copyfile(path,target)
        records.append({'file':target.relative_to(ROOT).as_posix(),'sha256':sha(target),'original_file':path.relative_to(ROOT).as_posix()})
report = {'passed':False,'counts_as_current_pass':False,'archived_at':datetime.now(timezone.utc).isoformat(),
    'reason':'PaymentRevision accidentally required the new optional refund_fx field; legacy history returned RESPONSE_CONTRACT_ERROR. Eight complete backend batches passed, batch9 failed one existing settlement case. No remaining batches or native APK claimed.',
    'source_archive':archive.relative_to(ROOT).as_posix(),'source_archive_sha256':sha(archive),
    'interaction_source_sha256':digest(),'public_records':[v for v in records if not v['file'].startswith('app/.local/')],
    'private_records':[v for v in records if v['file'].startswith('app/.local/')],
    'archive_script_sha256':sha(Path(__file__))}
(PUBLIC / 'preservation.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'preserved_records':len(records),'counts_as_current_pass':False}))
