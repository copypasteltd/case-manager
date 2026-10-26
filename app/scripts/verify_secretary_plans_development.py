"""Read only the owned native development API, schema and plan protections."""
from datetime import datetime, timezone
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
from urllib.error import HTTPError
from urllib.parse import unquote, urlparse
from urllib.request import Request, urlopen

APP=Path(__file__).resolve().parents[1]
GROUP=sys.argv[1] if __name__=='__main__' and len(sys.argv)==2 else 'secretary-plans'
if __name__=='__main__' and (len(sys.argv)>2 or not GROUP.replace('-','').isalnum()):
    raise SystemExit('Usage: python app/scripts/verify_secretary_plans_development.py [evidence-group]')
EVIDENCE=APP.parent/'docs/quality/evidence'/GROUP

def main():
    EVIDENCE.mkdir(parents=True,exist_ok=True)
    spec=importlib.util.spec_from_file_location('execution_development',APP/'scripts/verify_agent_execution_development.py')
    baseline=importlib.util.module_from_spec(spec)
    spec.loader.exec_module(baseline)
    baseline.EVIDENCE=EVIDENCE
    baseline.main()
    common=json.loads((EVIDENCE/'development-runtime.json').read_text(encoding='utf-8'))
    values=json.loads((APP/'.local/environment.json').read_text(encoding='utf-8'))
    db=urlparse(values['CP_DATABASE_URL'])
    if db.hostname!='127.0.0.1' or not values['CP_HTTP_ADDR'].startswith('127.0.0.1:'):
        raise SystemExit('Owned loopback development configuration is required.')
    env={**os.environ,'PGHOST':db.hostname,'PGPORT':str(db.port or 5432),'PGUSER':unquote(db.username or ''),
         'PGPASSWORD':unquote(db.password or ''),'PGDATABASE':unquote(db.path.lstrip('/')),'PGCONNECT_TIMEOUT':'5'}
    query="""SELECT json_build_object(
      'can_read_plans',has_table_privilege(current_user,'agent_plans','SELECT'),
      'can_insert_plans',has_table_privilege(current_user,'agent_plans','INSERT'),
      'can_update_plan_head',has_table_privilege(current_user,'agent_plans','UPDATE'),
      'can_delete_plans',has_table_privilege(current_user,'agent_plans','DELETE'),
      'can_insert_events',has_table_privilege(current_user,'agent_plan_events','INSERT'),
      'can_rewrite_events',has_table_privilege(current_user,'agent_plan_events','UPDATE') OR has_table_privilege(current_user,'agent_plan_events','DELETE'),
      'plan_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN
        ('agent_plan_insert_binding','agent_plan_event_immutable','agent_plan_no_delete','agent_plan_identity','agent_plan_event_binding','agent_plan_history')),
      'operator_role_installed',EXISTS(SELECT 1 FROM roles WHERE key='agent_plan_operator'),
      'active_runtime_profiles',(SELECT count(*) FROM agent_runtime_profiles WHERE state='active'),
      'disposable_test_databases',(SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[0-9a-f]{32}$'))"""
    checked=subprocess.run([str(APP/'.tools/pgsql/bin/psql.exe'),'-X','-A','-t','-q','-v','ON_ERROR_STOP=1','-c',query],env=env,capture_output=True,text=True,encoding='utf-8',timeout=15,
                           creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0)
    if checked.returncode:
        raise SystemExit('Owned database plan probe failed; private diagnostics withheld.')
    database=json.loads(checked.stdout)
    identifier='00000000-0000-4000-8000-000000000000'
    routes=[('GET','/v1/conversations/'+identifier+'/plans'),('POST','/v1/conversations/'+identifier+'/plans'),('GET','/v1/plans/'+identifier)]+[
        ('POST','/v1/plans/'+identifier+'/'+action) for action in ('approve','revoke','commit')]
    probes=[]
    for method,route in routes:
        request=Request('http://'+values['CP_HTTP_ADDR']+route,method=method,data=b'{}' if method=='POST' else None,headers={'Content-Type':'application/json'})
        try:
            with urlopen(request,timeout=5) as response:
                status=response.status
        except HTTPError as error:
            status=error.code
            error.close()
        except OSError:
            status=None
        probes.append({'method':method,'path':route,'status':status,'passed':status==401})
    drafts=[json.loads(path.read_text(encoding='utf-8'))['state'] for path in sorted((APP/'skills').glob('*/manifest.json'))]
    passed=bool(common['passed'] and common['database']['migration_count']==49 and database['plan_guards']==6 and database['operator_role_installed']
                and all(database[k] for k in ('can_read_plans','can_insert_plans','can_update_plan_head','can_insert_events'))
                and not database['can_delete_plans'] and not database['can_rewrite_events'] and all(p['passed'] for p in probes) and drafts==['draft']*8)
    report={'passed':passed,'checked_at':datetime.now(timezone.utc).isoformat(),'environment':'Owned native loopback development only',
            'database':database,'probes':probes,'actual_skill_states':drafts,'real_model_invoked':False,'production_deployed':False}
    (EVIDENCE/'development-plans.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'plan_protection_checks_passed':passed,'migration_count':common['database']['migration_count'],'route_probes':len(probes)}))
    return 0 if passed else 1

if __name__=='__main__':
    raise SystemExit(main())
