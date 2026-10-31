"""Check personal and retained secretary Bot API adapters without external sends."""
from datetime import datetime, timezone
import hashlib,json,os,subprocess,time
from notification_telegram_sources import APP,ROOT,digest,sources

def main():
 e=ROOT/'docs/quality/evidence/notification-telegram';e.mkdir(parents=True,exist_ok=True)
 report=e/'telegram-transport.json';assert not report.exists(),'Preserve earlier proof.'
 local=APP/'.local/notification-telegram';local.mkdir(parents=True,exist_ok=True);log=local/'telegram-transport.jsonl'
 names=sources(['backend','scripts/run_notification_telegram_transport.py']);before=digest(names);start=datetime.now(timezone.utc).isoformat();clock=time.monotonic()
 command=['go','test','./internal/telegram','-run','^(TestPersonalNoticeTransport|TestTelegramBotAPIOutcomes|TestTelegramDeliveryConfiguration)$','-count=1','-json']
 with log.open('wb') as stream:result=subprocess.run([str(APP/'.tools/go/bin/go.exe'),*command[1:]],cwd=APP/'backend',stdout=stream,stderr=subprocess.STDOUT,creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0)
 events=[]
 for line in log.read_text(encoding='utf-8',errors='replace').splitlines():
  try:
   r=json.loads(line)
   if r.get('Action') in ('pass','fail','skip') and r.get('Test'):events.append({k:r[k] for k in ('Action','Test','Elapsed') if k in r})
  except ValueError:pass
 after=digest(names);record={'passed':result.returncode==0 and before==after,'started_at':start,'elapsed_seconds':round(time.monotonic()-clock,3),'command':command,'exit_code':result.returncode,'source_files':names,'source_sha256':before,'source_after_sha256':after,'source_unchanged':before==after,'test_results':events,'private_log_sha256':hashlib.sha256(log.read_bytes()).hexdigest(),'provider':'local controlled RoundTripper only','real_telegram_delivery':False,'production_ready':False,'virtualization_used':False}
 report.write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8');print(json.dumps({'passed':record['passed'],'tests':len(events),'exit_code':result.returncode}));return 0 if record['passed'] else 1
if __name__=='__main__':raise SystemExit(main())
