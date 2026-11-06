"""One post-wave native engineering batch, with immutable per-attempt evidence."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

from run_batch_round166 import APP, ROOT, EVIDENCE, PRIVATE, load_plan, source_names, source_digest, write_once


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--attempt',required=True)
    parser.add_argument('--steps',default='release,monitor-plan,go-private,monitor-tools')
    args=parser.parse_args()
    import re
    if not re.fullmatch(r'[a-z0-9-]{1,48}',args.attempt):raise ValueError('Safe immutable attempt required.')
    load_plan()
    initial=json.loads((EVIDENCE/'freeze-code-initial.json').read_text('utf-8'))
    if initial['tests_executed']:raise ValueError('A complete pretest source freeze is required.')
    out=EVIDENCE/('engineering-'+args.attempt+'.json')
    if out.exists():raise ValueError('Preserve prior attempt.')
    PRIVATE.mkdir(parents=True,exist_ok=True)
    names=source_names();before=source_digest(names);started=datetime.now(timezone.utc).isoformat()
    commands={
        'release':([sys.executable,'-B','-m','unittest','discover','-s','deploy/release','-p','test*.py','-v'],APP),
        'monitor-plan':([sys.executable,'-B','-m','unittest','discover','-s','deploy/monitoring','-p','test*.py','-v'],APP),
        'go-private':([str(APP/'.tools/go/bin/go.exe'),'test','./internal/telegram','./internal/project','-count=1','-json','-run','^Test(PrivateBotConfiguration|PrivateIngressAndDispatchConfiguration|CloseoutCalendarProjection)$'],APP/'backend'),
        'monitor-tools':([sys.executable,'-B',str(APP/'scripts/check_batch_round166_monitoring.py'),'--attempt',args.attempt],APP),
    }
    steps=args.steps.split(',')
    if not set(steps)<=commands.keys() or len(set(steps))!=len(steps):raise ValueError('Unknown/duplicate step.')
    results=[]
    for name in steps:
        command,cwd=commands[name];log=PRIVATE/(name+'-'+args.attempt+'.log')
        with log.open('xb') as stream:
            code=subprocess.run(command,cwd=cwd,stdout=stream,stderr=subprocess.STDOUT,
                                creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0).returncode
        results.append({'step':name,'passed':code==0,'exit_code':code,'private_log_sha256':hashlib.sha256(log.read_bytes()).hexdigest()})
        print(json.dumps(results[-1]),flush=True)
    same=source_names()==names and source_digest(names)==before
    record={'passed':all(v['passed'] for v in results) and same,'started_at':started,'finished_at':datetime.now(timezone.utc).isoformat(),
            'source_sha256':before,'source_file_sha256':{n:hashlib.sha256((ROOT/n).read_bytes()).hexdigest() for n in names},
            'source_unchanged':same,'steps':results,'tests_executed':True,'systemd_executed':False,'real_receiver_contacted':False,
            'production_deployed':False,'native_symlinks_executed':os.name!='nt','process_effects_injected':True,'virtualization_used':False}
    write_once(out,record)
    return 0 if record['passed'] else 1


if __name__=='__main__':raise SystemExit(main())
