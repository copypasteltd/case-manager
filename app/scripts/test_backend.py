"""Run meaningful native backend checks against an isolated disposable database."""
import json
import os
from pathlib import Path
import subprocess
import sys
from urllib.parse import quote, urlsplit, urlunsplit

APP=Path(__file__).resolve().parents[1]
values=json.loads((APP/'.local/environment.json').read_text(encoding='utf-8'))
source=urlsplit(values['CP_DATABASE_URL'])
admin=urlunsplit((source.scheme,'postgres:'+quote(values['_superuser_password'])+'@127.0.0.1:'+str(values['_postgres_port']),'/postgres',source.query,''))
arguments = sys.argv[1:]
package = './...'
if '--package' in arguments:
    index = arguments.index('--package')
    if index + 1 >= len(arguments) or arguments[index + 1] not in ('./...', './internal/httpapi'):
        raise SystemExit('Unsupported native test package.')
    package = arguments[index + 1]
    arguments = arguments[:index] + arguments[index + 2:]
env={**os.environ,**{k:v for k,v in values.items() if k.startswith('CP_')},
     'CP_TEST_ADMIN_URL':admin,'CP_ENV':'test','GOPROXY':'https://goproxy.cn,https://proxy.golang.org,direct','GODEBUG':'http2client=0'}
raise SystemExit(subprocess.call([str(APP/'.tools/go/bin/go.exe'),'test',package,'-count=1',*arguments],cwd=APP/'backend',env=env))
