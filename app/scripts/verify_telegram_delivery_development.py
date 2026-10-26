"""Inspect only owned loopback services; never start Telegram delivery."""
from datetime import datetime, timezone
import importlib.util
import hashlib
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlparse
from urllib.error import HTTPError
from urllib.request import Request, urlopen

APP=Path(__file__).resolve().parents[1]
EVIDENCE=APP.parent/'docs/quality/evidence/telegram-delivery'


def main():
    values=json.loads((APP/'.local/environment.json').read_text(encoding='utf-8'))
    db=urlparse(values['CP_DATABASE_URL'])
    if db.hostname!='127.0.0.1' or db.port!=14660 or values.get('CP_HTTP_ADDR')!='127.0.0.1:18080':
        raise SystemExit('Only owned native loopback services may be inspected.')
    EVIDENCE.mkdir(parents=True,exist_ok=True)
    spec=importlib.util.spec_from_file_location('baseline',APP/'scripts/verify_agent_execution_development.py')
    baseline=importlib.util.module_from_spec(spec);spec.loader.exec_module(baseline);baseline.EVIDENCE=EVIDENCE
    if baseline.main():return 1
    common=json.loads((EVIDENCE/'development-runtime.json').read_text(encoding='utf-8'))
    env={**os.environ,'PGHOST':db.hostname,'PGPORT':str(db.port),'PGUSER':unquote(db.username),'PGPASSWORD':unquote(db.password),'PGDATABASE':unquote(db.path[1:]),'PGCONNECT_TIMEOUT':'5'}
    query="""SELECT json_build_object(
      'journal_append_only',has_table_privilege(current_user,'telegram_delivery_events','SELECT') AND has_table_privilege(current_user,'telegram_delivery_events','INSERT') AND NOT has_table_privilege(current_user,'telegram_delivery_events','UPDATE') AND NOT has_table_privilege(current_user,'telegram_delivery_events','DELETE'),
      'head_no_delete',NOT has_table_privilege(current_user,'telegram_deliveries','DELETE'),
      'delivery_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O' AND tgname IN ('telegram_delivery_event_immutable','telegram_delivery_event_binding','telegram_delivery_guard')),
      'deferred_event_binding',(SELECT count(*) FROM pg_constraint WHERE conname='telegram_delivery_current_event' AND condeferrable AND condeferred AND convalidated),
      'enabled_channels',(SELECT count(*) FROM telegram_channels WHERE delivery_enabled),
      'delivery_count',(SELECT count(*) FROM telegram_deliveries),
      'event_count',(SELECT count(*) FROM telegram_delivery_events),
      'worker_budget_count',(SELECT count(*) FROM telegram_delivery_budgets),
      'active_profiles',(SELECT count(*) FROM agent_runtime_profiles WHERE state='active'),
      'disposable_test_databases',(SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[0-9a-f]{32}$')
    )"""
    proc=subprocess.run([str(APP/'.tools/pgsql/bin/psql.exe'),'-X','-A','-t','-q','-v','ON_ERROR_STOP=1','-c',query],env=env,capture_output=True,text=True,encoding='utf-8',timeout=15,creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0)
    if proc.returncode:raise SystemExit('Owned schema inspection failed; private diagnostics withheld.')
    actual=json.loads(proc.stdout)
    identifier='00000000-0000-4000-8000-000000000000'
    routes=[('GET','/v1/me/telegram/requests/'+identifier),('GET','/v1/me/telegram/requests/'+identifier+'/deliveries'),('POST','/v1/me/telegram/requests/'+identifier+'/deliveries/'+identifier+'/reconcile')]
    probes=[]
    for method,route in routes:
        req=Request('http://'+values['CP_HTTP_ADDR']+route,method=method,data=b'' if method=='POST' else None,headers={'Content-Type':'application/json'})
        try:
            with urlopen(req,timeout=5) as response:status=response.status
        except HTTPError as failure:status=failure.code
        probes.append({'method':method,'path':route,'status':status,'passed':status==401})
    manifests=[json.loads(p.read_text(encoding='utf-8')) for p in sorted((APP/'skills').glob('*/manifest.json'))]
    effective={**os.environ,**values}
    disabled=all(effective.get(k,'') in ('','disabled') for k in ('CP_TELEGRAM_BOTS','CP_TELEGRAM_INGRESS','CP_TELEGRAM_DISPATCH_BOTS','CP_TELEGRAM_DELIVERY_BOTS'))
    passed=bool(common['passed'] and common['database']['migration_count']==63 and actual['journal_append_only'] and actual['head_no_delete'] and actual['delivery_guards']==3 and actual['deferred_event_binding']==1 and all(actual[k]==0 for k in ('enabled_channels','delivery_count','event_count','worker_budget_count','active_profiles','disposable_test_databases')) and len(manifests)==8 and all(m['state']=='draft' for m in manifests) and disabled and all(p['passed'] for p in probes))
    report={'passed':passed,'verification_script_sha256':hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),'checked_at':datetime.now(timezone.utc).isoformat(),'migration_count':common['database']['migration_count'],'database':actual,'probes':probes,'all_eight_skills_draft':all(m['state']=='draft' for m in manifests),'production_integrations_disabled':disabled,'actual_model_invoked':False,'actual_telegram_contacted':False,'schema_applied_to_owned_development_only':True}
    (EVIDENCE/'development-telegram-delivery.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':passed,'migration_count':report['migration_count'],'delivery_guards':actual['delivery_guards']}))
    return 0 if passed else 1


if __name__=='__main__':raise SystemExit(main())
