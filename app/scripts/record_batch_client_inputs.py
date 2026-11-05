"""Bind unchanged client dependencies across separately tested backend repairs."""
import argparse
from datetime import datetime,timezone
import hashlib,json
from run_batch_round import ROOT,APP,EVIDENCE,source_names,source_digest,write_once
PREFIXES=('app/web/','app/mobile/','app/packages/','app/agent-runtime/','app/skills/','docs/skills/')
def client_names():
    return [p for p in source_names() if p.startswith(PREFIXES) or p in ('app/package.json','app/package-lock.json')]
def client_digest():return source_digest(client_names())
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('position',choices=('before','after'));p.add_argument('--attempt',required=True);a=p.parse_args()
    if not a.attempt.replace('-','').isalnum():raise ValueError('Use an immutable safe attempt.')
    names=client_names();value={'position':a.position,'attempt':a.attempt,'captured_at':datetime.now(timezone.utc).isoformat(),'source_files':names,'source_sha256':source_digest(names),'product_source_sha256':source_digest(source_names()),'scope':'All Web/mobile/shared/runtime/Skill source and configuration plus top dependency locks. Backend repairs are checked independently.','recorder_sha256':hashlib.sha256(__import__('pathlib').Path(__file__).read_bytes()).hexdigest()}
    target=EVIDENCE/('client-inputs-'+a.attempt+'-'+a.position+'.json');write_once(target,value);print(json.dumps({'record':str(target),'source_count':len(names)}))
