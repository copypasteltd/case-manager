"""Preserve pre-remediation client/capture bytes without final-source pass credit."""
import hashlib
import json
from pathlib import Path
import shutil
import zipfile
from refund_fx_sources import APP, sources, digest

ROOT = APP.parent
PUBLIC = ROOT / 'docs/quality/evidence/refund-fx-preliminary'
PRIVATE = APP / '.local/refund-fx-preliminary'
assert not PUBLIC.exists() and not PRIVATE.exists()
assert not (APP / '.local/business-browser-fixture.json').exists()
PUBLIC.mkdir(parents=True)
PRIVATE.mkdir()
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
names = sources()
archive = PRIVATE / 'source.zip'
with zipfile.ZipFile(archive, 'x', zipfile.ZIP_DEFLATED) as z:
    for name in names: z.write(APP / name, name)
records = []
for group in ('refund-fx-web','refund-fx-mobile'):
    original = ROOT / 'docs/quality/evidence' / group
    for p in original.glob('*'):
        target = (PUBLIC if p.suffix == '.json' else PRIVATE) / group / p.name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.move(p, target)
        records.append({'file':target.relative_to(ROOT).as_posix(),'sha256':sha(target)})
p = ROOT / 'docs/quality/evidence/refund-fx/client-checks.json'
assert not json.loads(p.read_text())['passed']
target = PUBLIC / p.name
shutil.move(p, target)
records.append({'file':target.relative_to(ROOT).as_posix(),'sha256':sha(target)})
for p in (APP / '.local').glob('refund_fx-client-*.log'):
    target = PRIVATE / p.name
    shutil.move(p, target)
    records.append({'file':target.relative_to(ROOT).as_posix(),'sha256':sha(target)})
for p in list((ROOT / 'docs/media/screenshots').glob('refund-fx-web-*.png')) + [APP / '.local/android/bundle.log', APP / '.local/refund_fx-android-run.log']:
    target = PRIVATE / p.name
    shutil.copyfile(p, target)
    records.append({'file':target.relative_to(ROOT).as_posix(),'sha256':sha(target)})
report = {'passed':False,'counts_as_final_regression':False,'reason':'Legacy CommonJS test compiler could not consume the new shared money validator. Owned Metro and H5 prebuilds stopped before remediation. Prior Web passed its exact earlier source only.',
    'source_archive':archive.relative_to(ROOT).as_posix(),'source_archive_sha256':sha(archive),
    'interaction_source_sha256':digest(),'records':records,'archive_script_sha256':sha(Path(__file__))}
(PUBLIC / 'preservation.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'preserved':len(records),'final_pass_credit':False}))
