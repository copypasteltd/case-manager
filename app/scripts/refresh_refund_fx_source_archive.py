"""Preserve an earlier source archive, then freeze final capture inputs exactly."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
import zipfile
from refund_fx_sources import APP, sources, digest

ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/refund-fx'
PUBLIC = E.parent / 'refund-fx-attempts/12-before-navigation-source'
PRIVATE = APP / '.local/refund-fx-attempts/12-before-navigation-source'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
assert not PUBLIC.exists() and not PRIVATE.exists()
old = E / 'source-archive.json'
prior = json.loads(old.read_text())
archive = APP / '.local/refund-fx-final-sources.zip'
assert sha(archive) == prior['private_archive_sha256']
PUBLIC.mkdir(parents=True)
PRIVATE.mkdir(parents=True)
saved = PRIVATE / 'source.zip'
shutil.move(archive,saved)
shutil.move(old,PUBLIC / old.name)
preserved = {'passed':False,'counts_as_current_pass':False,'reason':'Capture helper now waits for the committed Taro redirect instead of racing document navigation. Earlier archive remains exact.',
    'source_archive':saved.relative_to(ROOT).as_posix(),'source_archive_sha256':sha(saved),
    'public_records':[{'file':(PUBLIC/old.name).relative_to(ROOT).as_posix(),'sha256':sha(PUBLIC/old.name)}],
    'private_records':[],'archived_at':datetime.now(timezone.utc).isoformat()}
(PUBLIC / 'preservation.json').write_text(json.dumps(preserved,indent=2)+'\n',encoding='utf-8')
names=sources();before=digest(names)
with zipfile.ZipFile(archive,'x',zipfile.ZIP_DEFLATED) as z:
    for n in names:z.write(APP/n,n)
assert before==digest(names)
old.write_text(json.dumps({'passed':True,'source_sha256':before,'private_archive_sha256':sha(archive),
    'files':[{'file':n,'sha256':sha(APP/n)} for n in names],'credentials_included':False,'source_only':True},indent=2)+'\n',encoding='utf-8')
print(json.dumps({'passed':True,'final_source_files':len(names),'earlier_source_retained':True}))
