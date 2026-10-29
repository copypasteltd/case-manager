"""Preserve failed raw logs and exact backend source before a repair."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import sys
import zipfile
import verify_backend_batches as backend

APP=backend.APP
label,log_name,reason=sys.argv[1:]
assert all(c.isalnum() or c=='-' for c in label) and Path(log_name).name==log_name
target=APP.parent/'docs/quality/evidence/contract-reminders/attempts'/label
target.mkdir(parents=True,exist_ok=False)
raw=APP/'.local'/log_name
archive=APP/'.local'/('contract-reminders-'+label+'-source.zip')
files=sorted(p for p in (APP/'backend').rglob('*') if p.is_file() and (p.suffix in ('.go','.sql') or p.name in ('go.mod','go.sum','openapi.json')))
with zipfile.ZipFile(archive,'x',compression=zipfile.ZIP_DEFLATED) as output:
    for path in files:output.write(path,path.relative_to(APP).as_posix())
checks=[]
for line in raw.read_text(encoding='utf-8').splitlines():
    try:event=json.loads(line)
    except ValueError:continue
    if event.get('Test') and event.get('Action') in ('pass','fail','skip'):
        checks.append({k:event[k] for k in ('Action','Package','Test','Elapsed') if k in event})
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
record={'passed':False,'checked_at':datetime.now(timezone.utc).isoformat(),'reason':reason,
    'source_sha256':backend.digest(),'source_archive':archive.relative_to(APP).as_posix(),
    'source_archive_sha256':sha(archive),'raw_log':raw.relative_to(APP).as_posix(),
    'raw_log_sha256':sha(raw),'checks':checks,'archiver_sha256':sha(Path(__file__))}
(target/'attempt.json').write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'archived_failed_attempt':True,'checks':len(checks),'failed':[c['Test'] for c in checks if c['Action']=='fail']}))
