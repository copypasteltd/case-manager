"""Run the complete post-wave release/JVM/artifact batch, collecting all failures."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
from run_batch_round167 import APP,ROOT,EVIDENCE,PRIVATE,load_plan,source_names,source_digest,write_once


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--attempt',required=True);parser.add_argument('--steps',default='release,jvm,production');args=parser.parse_args()
    import re
    if not re.fullmatch('[a-z0-9-]{1,48}',args.attempt):raise ValueError('Safe immutable attempt required.')
    load_plan();assert (EVIDENCE/'freeze-code-initial.json').is_file()
    names=source_names();before=source_digest(names);PRIVATE.mkdir(parents=True,exist_ok=True)
    commands={
      'release':([sys.executable,'-B','-m','unittest','discover','-s','deploy/release','-p','test*.py','-v'],APP),
      'jvm':([sys.executable,'-B',str(APP/'scripts/check_batch_round167_jvm.py'),'--attempt',args.attempt],APP),
      'production':([sys.executable,'-B',str(APP/'scripts/check_batch_round167_package.py'),'--attempt',args.attempt],APP),
    }
    steps=args.steps.split(',');assert len(steps)==len(set(steps)) and set(steps)<=commands.keys()
    results=[]
    for name in steps:
        if name=='release': inputs=[p for p in names if p.startswith('app/deploy/release/') or p=='app/scripts/build_production_release.py']
        elif name=='jvm': inputs=[p for p in names if p.startswith('app/mobile/android/') or p=='app/scripts/check_batch_round167_jvm.py']
        else:
            from build_production_release import inputs as package_inputs
            inputs=[row['path'] for row in package_inputs()]+['app/scripts/check_batch_round167_package.py']
        inputs=sorted(set(inputs));bound={p:hashlib.sha256((ROOT/p).read_bytes()).hexdigest() for p in inputs}
        command,cwd=commands[name];log=PRIVATE/(name+'-'+args.attempt+'.log');began=datetime.now(timezone.utc).isoformat()
        with log.open('xb') as handle:
            result=subprocess.run(command,cwd=cwd,stdout=handle,stderr=subprocess.STDOUT,creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0)
        results.append({'step':name,'passed':result.returncode==0,'exit_code':result.returncode,'started_at':began,'source_file_sha256':bound,'private_log_sha256':hashlib.sha256(log.read_bytes()).hexdigest()})
        print(json.dumps({k:v for k,v in results[-1].items() if k!='source_file_sha256'}),flush=True)
    same=names==source_names() and before==source_digest(names)
    record={'passed':all(row['passed'] for row in results) and same,'source_sha256':before,'source_unchanged':same,'steps':results,'tests_executed':True,'formal_acceptance':False,'real_integrations_executed':False,'virtualization_used':False}
    write_once(EVIDENCE/('engineering-'+args.attempt+'.json'),record)
    return 0 if record['passed'] else 1

if __name__=='__main__':raise SystemExit(main())
