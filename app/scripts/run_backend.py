"""Run a backend command using ignored local development credentials."""
import json
import os
from pathlib import Path
import subprocess
import sys

APP=Path(__file__).resolve().parents[1]
def main():
    config=APP/'.local'/'environment.json'
    if not config.exists():
        raise SystemExit('Run: python app/scripts/dev_database.py start')
    values=json.loads(config.read_text(encoding='utf-8'))
    subcommand=sys.argv[1:] or ['api']
    selected={k:v for k,v in values.items() if k.startswith('CP_') and (k!='CP_MIGRATION_DATABASE_URL' or subcommand[0] in ('migrate','bootstrap','dev-seed'))}
    env={**os.environ,**selected,
         'GOPROXY':'https://goproxy.cn,https://proxy.golang.org,direct','GODEBUG':'http2client=0'}
    if subcommand[0]!='worker-telegram-delivery':
        env.pop('CP_TELEGRAM_DELIVERY_BOTS',None)
    if subcommand[0] in ('worker-upload','worker-verify-small','worker-verify-large','worker-promote-small','worker-promote-large','worker-replica-small','worker-replica-large','worker-restore-small','worker-restore-large'):
        common={'CP_ENV','CP_DATABASE_URL','CP_STORAGE_DRIVER'}
        verify=subcommand[0].startswith('worker-verify-')
        if subcommand[0].startswith('worker-restore-'):
            roles=('_RESTORE_SOURCE_','_RESTORE_TARGET_')
        elif subcommand[0].startswith('worker-replica-'):
            roles=('_REPLICA_SOURCE_','_REPLICA_TARGET_')
        else:
            roles=('_PROMOTE_SOURCE_','_PROMOTE_TARGET_') if subcommand[0].startswith('worker-promote-') else (('_UPLOAD_VERIFY_',) if verify else ('_UPLOAD_LIFECYCLE_',))
        def worker_setting(key):
            return key in common or (verify and key.startswith(('CP_SCAN_','CP_CLAMD_'))) or (key.startswith(('CP_OSS_','CP_R2_')) and
                (key.endswith(('_ENDPOINT','_REGION','_BUCKET')) or any(role in key for role in roles)))
        env={k:v for k,v in env.items() if not k.startswith('CP_') or worker_setting(k)}
    if subcommand[0]=='worker-notification':
        allowed={'CP_ENV','CP_DATABASE_URL','CP_NOTIFICATION_CONTRACT_ORG_ID','CP_NOTIFICATION_CONTRACT_TIMEZONE','CP_NOTIFICATION_CONTRACT_DAYS_BEFORE'}
        env={k:v for k,v in env.items() if not k.startswith('CP_') or k in allowed}
    if subcommand[0] in ('worker-subscription','worker-report-export'):
        env={k:v for k,v in env.items() if not k.startswith('CP_') or k in {'CP_ENV','CP_DATABASE_URL'}}
    if subcommand[0]=='worker-telegram':
        env={k:v for k,v in env.items() if not k.startswith('CP_') or k in {'CP_ENV','CP_DATABASE_URL','CP_TELEGRAM_DISPATCH_BOTS'}}
    if subcommand[0]=='worker-telegram-delivery':
        env={k:v for k,v in env.items() if not k.startswith('CP_') or k in {'CP_ENV','CP_DATABASE_URL','CP_TELEGRAM_DELIVERY_BOTS'}}
    go=APP/'.tools'/'go'/'bin'/'go.exe'
    if subcommand[0] not in ('migrate','bootstrap','dev-seed'):
        env.pop('CP_MIGRATION_DATABASE_URL',None)
    command=[str(go),'run','./cmd/platform',*subcommand]
    return subprocess.call(command,cwd=APP/'backend',env=env)
if __name__=='__main__':
    raise SystemExit(main())
