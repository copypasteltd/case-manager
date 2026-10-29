"""Bind historical published media and dependency evidence without rerunning it."""
import hashlib
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[2]
E=ROOT/'docs/quality/evidence/contract-reminders'
PUBLISHED='8699b7d3401f2215bf37d2004870996fbfc8d5cf'
def git(*args):return subprocess.check_output(['git',*args],cwd=ROOT)
def sha(data):return hashlib.sha256(data).hexdigest()

def main():
    media=[]
    raw=git('ls-tree','-rz',PUBLISHED,'--','docs/media/screenshots','docs/media/videos','docs/media/gifs')
    for row in raw.split(b'\0'):
        if not row:continue
        metadata,name=row.split(b'\t',1);name=name.decode('utf-8')
        if Path(name).suffix not in ('.png','.mp4','.gif'):continue
        blob=metadata.split()[2].decode();data=(ROOT/name).read_bytes()
        assert hashlib.sha1(b'blob '+str(len(data)).encode()+b'\0'+data).hexdigest()==blob,name
        media.append({'file':name,'git_blob':blob,'sha256':sha(data),'bytes':len(data)})
    assert len(media)==233
    names=['docs/quality/evidence/dependency-integrity/'+n for n in ('dependency-audit.json','audit-after.json','security-checks.json')]
    names += ['docs/quality/evidence/notification-retry/'+n for n in ('client-checks.json','final-source-check.json','media.json')]
    names += ['docs/quality/evidence/notification-retry-backend/result.json']
    records=[]
    for name in names:
        before=git('show',PUBLISHED+':'+name);assert before==(ROOT/name).read_bytes()
        records.append({'file':name,'sha256':sha(before)})
    for name in ('app/package.json','app/package-lock.json','app/packages/contracts/package.json'):
        assert git('show',PUBLISHED+':'+name)==(ROOT/name).read_bytes()
    name='app/mobile/package.json'
    previous=json.loads(git('show',PUBLISHED+':'+name));current=json.loads((ROOT/name).read_bytes())
    expected=previous['scripts']['test']+' ../.local/mobile-concern-tests/mobile/src/platform/contract-reminders.test.js'
    assert current['scripts']['test']==expected
    current['scripts']['test']=previous['scripts']['test'];assert current==previous
    assert not git('diff','--name-only',PUBLISHED,'--','docs/quality/evidence/dependency-integrity').strip()
    record={'passed':True,'published_commit':PUBLISHED,'media':sorted(media,key=lambda v:v['file']),'records':records,
        'package_lock_sha256':sha((ROOT/'app/package-lock.json').read_bytes()),'new_dependency_audit':False,'new_dependency_security_test_execution':False,
        'mobile_test_selection_updated':True,'workspace_dependency_fields_unchanged':True,
        'scope':'233 historical media retain exact published bytes and original capture scope. Dependency results retain increment108 execution and date. Root manifest, lock, contracts manifest and all dependency fields are unchanged; only the mobile test command adds the new policy cases. Increment110 records are historical; current feature checks are separate.'}
    E.mkdir(parents=True,exist_ok=True);(E/'reused-evidence.json').write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':True,'retained_media':len(media),'retained_records':len(records),'new_dependency_audit':False}))
if __name__=='__main__':main()
