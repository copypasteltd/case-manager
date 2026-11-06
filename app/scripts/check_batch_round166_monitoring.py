"""Check exact generated files using pinned upstream native binaries, no listeners."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys

from run_batch_round166 import APP, ROOT, EVIDENCE, PRIVATE, write_once


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--attempt',required=True);args=parser.parse_args()
    import re
    if not re.fullmatch(r'[a-z0-9-]{1,48}',args.attempt):raise ValueError('Safe immutable attempt required.')
    sys.path.insert(0,str(APP/'deploy/monitoring'))
    from fetch_components import fetch
    from config import load
    from render import build, CREDS
    from rule_cases import cases
    working=PRIVATE/('monitor-validation-'+args.attempt)
    working.mkdir(parents=True,exist_ok=False)
    component_dir=working/'bin'
    fetch('windows-amd64',component_dir,APP/'.local/monitoring-archives')
    plan=load(APP/'deploy/monitoring/example.json');files=build(plan)
    originals={name:hashlib.sha256(text.encode()).hexdigest() for name,text in files.items()}
    config=working/'config';config.mkdir()
    credential=working/'synthetic-credentials';credential.mkdir()
    (credential/'telegram-token').write_text('123456:'+('a'*35),encoding='utf-8')
    (credential/'telegram-chat').write_text('-1001122334455',encoding='utf-8')
    (credential/'watchdog-url').write_text('https://receiver.example.invalid/synthetic-watchdog',encoding='utf-8')
    for name,text in files.items():
        if name.startswith('config/'):
            # Only credential filenames and rule include paths are projected into
            # this disposable Windows workspace. No probe/send/listener runs.
            content=text.replace(CREDS,credential.as_posix())
            if name.endswith('prometheus.yml'):
                obj=json.loads(content);obj['rule_files']=[(config/'rules.yml').as_posix()];content=json.dumps(obj)
            (config/Path(name).name).write_text(content,encoding='utf-8')
    rule_test=working/'rule-tests.yml'
    rule_test.write_text(json.dumps(cases((config/'rules.yml').resolve(),['api_ready',*[t['id'] for t in plan['targets']]],plan['required_units'])),encoding='utf-8')
    commands=[('prometheus-config',[component_dir/'promtool.exe','check','config',config/'prometheus.yml']),
              ('prometheus-rules',[component_dir/'promtool.exe','check','rules',config/'rules.yml']),
              ('prometheus-rule-cases',[component_dir/'promtool.exe','test','rules',rule_test]),
              ('alertmanager-config',[component_dir/'amtool.exe','check-config',config/'alertmanager.yml']),
              ('blackbox-config',[component_dir/'blackbox_exporter.exe','--config.file='+str(config/'blackbox.yml'),'--config.check'])]
    results=[]
    for name,command in commands:
        log=working/(name+'.log')
        with log.open('xb') as stream:
            code=subprocess.run(list(map(str,command)),cwd=working,stdout=stream,stderr=subprocess.STDOUT,timeout=60).returncode
        results.append({'name':name,'passed':code==0,'exit_code':code,'private_log_sha256':hashlib.sha256(log.read_bytes()).hexdigest()})
        print(json.dumps(results[-1]),flush=True)
    receipt=json.loads((component_dir/'components.json').read_text('utf-8'))
    record={'passed':all(v['passed'] for v in results),'checks':results,'exact_generated_files':originals,'components':receipt,
            'validation_projection':'Only private synthetic credential paths and absolute rule include paths replace production locations.',
            'services_started':False,'real_target_probed':False,'real_receiver_contacted':False,'systemd_executed':False,'linux_binaries_executed':False,
            'component_lock_sha256':hashlib.sha256((APP/'deploy/monitoring/components.lock.json').read_bytes()).hexdigest()}
    write_once(EVIDENCE/('monitor-tools-'+args.attempt+'.json'),record)
    return 0 if record['passed'] else 1


if __name__=='__main__':raise SystemExit(main())
