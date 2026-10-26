"""Read-only inspection of the exact owned development schema and API."""
from datetime import datetime, timezone
import importlib.util
import json
import os
from pathlib import Path
import subprocess
from urllib.error import HTTPError
from urllib.parse import unquote, urlsplit
from urllib.request import Request, urlopen

APP=Path(__file__).resolve().parents[1]
EVIDENCE=APP.parent/'docs/quality/evidence/project-templates'


def module(name,file):
    spec=importlib.util.spec_from_file_location(name,APP/'scripts'/file)
    result=importlib.util.module_from_spec(spec);spec.loader.exec_module(result);return result


def main():
    values=json.loads((APP/'.local/environment.json').read_text(encoding='utf-8'))
    db=urlsplit(values['CP_DATABASE_URL'])
    if db.hostname!='127.0.0.1' or db.port!=14660 or values.get('CP_HTTP_ADDR')!='127.0.0.1:18080':
        raise SystemExit('Only the owned native loopback development environment may be inspected.')
    base=module('template_development_base','verify_agent_execution_development.py');base.EVIDENCE=EVIDENCE
    if base.main():return 1
    baseline=json.loads((EVIDENCE/'development-runtime.json').read_text(encoding='utf-8'))
    environment={**os.environ,'PGHOST':db.hostname,'PGPORT':str(db.port),'PGUSER':unquote(db.username),'PGPASSWORD':unquote(db.password),'PGDATABASE':unquote(db.path[1:]),'PGCONNECT_TIMEOUT':'5'}
    query="""SELECT json_build_object(
      'history_append_only',has_table_privilege(current_user,'project_template_events','SELECT') AND has_table_privilege(current_user,'project_template_events','INSERT') AND NOT has_table_privilege(current_user,'project_template_events','UPDATE') AND NOT has_table_privilege(current_user,'project_template_events','DELETE'),
      'templates_no_delete',NOT has_table_privilege(current_user,'project_templates','DELETE'),
      'revision_guard',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname='project_template_revision_guard'),
      'history_guard',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname='project_template_event_immutable'),
      'deferred_history_consistency',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname='project_template_event_consistency' AND tgdeferrable AND tginitdeferred),
      'one_draft_per_family',(SELECT count(*) FROM pg_indexes WHERE indexname='project_templates_one_draft' AND indexdef LIKE '%UNIQUE%' AND indexdef LIKE '%WHERE%'),
      'managed_template_count',(SELECT count(*) FROM project_templates WHERE creator_id IS NOT NULL),
      'enabled_telegram_channels',(SELECT count(*) FROM telegram_channels WHERE delivery_enabled),
      'active_runtime_profiles',(SELECT count(*) FROM agent_runtime_profiles WHERE state='active'),
      'disposable_test_databases',(SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[0-9a-f]{32}$'))"""
    result=subprocess.run([str(APP/'.tools/pgsql/bin/psql.exe'),'-X','-A','-t','-q','-v','ON_ERROR_STOP=1','-c',query],env=environment,capture_output=True,text=True,encoding='utf-8',timeout=15,creationflags=subprocess.CREATE_NO_WINDOW)
    if result.returncode:raise SystemExit('Development template query failed; private diagnostics withheld.')
    database=json.loads(result.stdout)
    identifier='00000000-0000-4000-8000-000000000000'
    paths=[('GET','/v1/project-templates'),('POST','/v1/project-templates'),('GET','/v1/project-templates/'+identifier),('PATCH','/v1/project-templates/'+identifier),('GET','/v1/project-templates/'+identifier+'/history')]+[('POST','/v1/project-templates/'+identifier+'/'+action) for action in ('publish','retire','revise')]
    probes=[]
    for method,path in paths:
        try:
            with urlopen(Request('http://127.0.0.1:18080'+path,method=method,data=b'' if method!='GET' else None,headers={'Content-Type':'application/json'}),timeout=5) as response:status=response.status
        except HTTPError as error:status=error.code;error.close()
        probes.append({'method':method,'path':path,'status':status,'expected':401,'passed':status==401})
    owner=module('template_development_owner','verify_notifications_sources.py').owned_process
    worker_owner='project-templates-worker-owner.json' if (APP/'.local/project-templates-worker-owner.json').exists() else 'telegram-delivery-worker-owner.json'
    ownership={'api':owner('project-templates-api-owner.json','api'),'notification_worker':owner(worker_owner,'worker-notification')}
    passed=baseline['passed'] and baseline['database']['migration_count']==64 and baseline['database']['migration_manifest_matches_source']
    passed=passed and all(database[key] for key in ('history_append_only','templates_no_delete')) and all(database[key]==1 for key in ('revision_guard','history_guard','deferred_history_consistency','one_draft_per_family'))
    passed=passed and database['active_runtime_profiles']==database['enabled_telegram_channels']==database['disposable_test_databases']==0 and values.get('CP_STORAGE_DRIVER')=='disabled' and all(probe['passed'] for probe in probes) and all(ownership.values())
    report={'passed':bool(passed),'checked_at':datetime.now(timezone.utc).isoformat(),'database':database,'migration_count':64,'probes':probes,'ownership':ownership,'integrations_disabled':True,'production_deployment':False,'real_cloud_or_model':False}
    (EVIDENCE/'development-project-templates.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':bool(passed),'migration_count':64,'probes':len(probes),'integrations_disabled':True}))
    return 0 if passed else 1


if __name__=='__main__':raise SystemExit(main())
