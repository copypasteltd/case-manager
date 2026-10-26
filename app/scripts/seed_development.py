"""Initialize three development identities; never print their credentials."""
import json
import os
from pathlib import Path
import secrets
import subprocess

APP=Path(__file__).resolve().parents[1]
values=json.loads((APP/'.local/environment.json').read_text(encoding='utf-8'))
if values['CP_ENV']!='development':
    raise SystemExit('Development fixture only')
target=APP/'.local/development-accounts.json'
if not target.exists():
    accounts={'accounts':[
        {'login':'system.admin','display_name':'系统管理员','password':secrets.token_urlsafe(32)},
        {'login':'project.manager','display_name':'项目负责人','password':secrets.token_urlsafe(32)},
        {'login':'project.member','display_name':'项目成员','password':secrets.token_urlsafe(32)},
    ]}
    target.write_text(json.dumps(accounts,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
else:
    accounts=json.loads(target.read_text(encoding='utf-8'))
env={**os.environ,**{k:v for k,v in values.items() if k.startswith('CP_')},
     'GOPROXY':'https://goproxy.cn,https://proxy.golang.org,direct','GODEBUG':'http2client=0'}
result=subprocess.run([str(APP/'.tools/go/bin/go.exe'),'run','./cmd/platform','dev-seed'],
    cwd=APP/'backend',env=env,input=json.dumps(accounts).encode(),check=True)
print('Development identities created. Credentials stay in ignored app/.local/development-accounts.json')
