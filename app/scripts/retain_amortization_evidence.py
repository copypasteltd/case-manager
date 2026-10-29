"""Retain published media/audit provenance without presenting them as new checks."""
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/cost-amortization'
PUBLISHED = '815e89317785a8f97bc474b925e67b5f2bfa5895'
names = subprocess.check_output(['git','ls-tree','-rz','--name-only',PUBLISHED,'--','docs/media/screenshots','docs/media/videos','docs/media/gifs'],cwd=ROOT).decode().split('\0')
media = []
for name in sorted(v for v in names if Path(v).suffix in ('.png','.mp4','.gif')):
    raw = (ROOT / name).read_bytes()
    blob = subprocess.check_output(['git','rev-parse',PUBLISHED+':'+name],cwd=ROOT).decode().strip()
    actual = subprocess.check_output(['git','hash-object','--',name],cwd=ROOT).decode().strip()
    assert actual == blob, name
    media.append({'file':name,'git_blob':blob,'sha256':hashlib.sha256(raw).hexdigest(),'bytes':len(raw)})
records=[]
for name in ['docs/quality/evidence/dependency-remediation/dependency-audit.json','docs/quality/evidence/dependency-remediation/remaining-findings.json','docs/quality/evidence/dependency-remediation/android-test-scope.json']:
    previous=subprocess.check_output(['git','show',PUBLISHED+':'+name],cwd=ROOT)
    assert previous == (ROOT/name).read_bytes()
    records.append({'file':name,'sha256':hashlib.sha256(previous).hexdigest()})
assert len(media)==199
out={'published_commit':PUBLISHED,'media':media,'records':records,'passed':True,'scope':'Previously published bytes retained exactly; no new capture, dependency audit or JVM execution is asserted.'}
E.mkdir(parents=True,exist_ok=True)
(E/'reused-evidence.json').write_text(json.dumps(out,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'passed':True,'retained_media':len(media),'retained_records':len(records)}))
