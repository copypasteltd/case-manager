"""Preserve exact successful raw batches; failed/interrupted attempts stay separate."""
import hashlib
import json
from pathlib import Path
import shutil
from verify_backend_batches import APP, digest

E = APP.parent / 'docs/quality/evidence/office-text-backend'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()


def main():
    state, result = [json.loads((E / n).read_text(encoding='utf-8')) for n in ('batch-progress.json','result.json')]
    assert result['passed'] and state['source_sha256'] == result['source_sha256'] == digest()
    out = APP / '.local/office-text-backend'
    out.mkdir(exist_ok=True)
    logs = []
    for i,batch in enumerate(state['batches'],1):
        assert batch['status'] == 'passed' and batch['source_unchanged_during_tests']
        raw = APP / '.local' / ('backend-batch-' + str(i) + '.log')
        observed = []
        for line in raw.read_text(encoding='utf-8').splitlines():
            try: v = json.loads(line)
            except ValueError: continue
            if v.get('Test') and v.get('Action') in ('pass','fail','skip'):
                observed.append({k:v[k] for k in ('Action','Package','Test','Elapsed') if k in v})
        assert observed == batch['checks'] and all(v['Action'] == 'pass' for v in observed)
        target = out / raw.name
        assert not target.exists() or sha(target) == sha(raw)
        if not target.exists(): shutil.copyfile(raw,target)
        logs.append({'batch':i,'file':target.relative_to(APP).as_posix(),'sha256':sha(target),'records':len(observed),
            'execution':'Initial complete passing batch; application inputs unchanged' if i==1 else 'Resumed unchanged cases with one Go package at a time; deadlines/leases unchanged'})
    record = {'passed':True,'source_sha256':digest(),'records':sum(v['records'] for v in logs),'logs':logs,
        'failed_interrupted_attempt':'docs/quality/evidence/office-text-attempts/02-concurrent-backend/preservation.json',
        'raw_logs_private':True,'earlier_failures_counted_as_final_pass':False,'freezer_sha256':sha(Path(__file__))}
    (E / 'raw-log-archive.json').write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':True,'batches':len(logs),'records':record['records']}))


if __name__ == '__main__':
    main()
