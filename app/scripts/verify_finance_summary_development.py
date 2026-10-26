"""Inspect only the owned native development services; never enable an executor."""
from datetime import datetime, timezone
import importlib.util
import hashlib
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlparse

APP=Path(__file__).resolve().parents[1]
EVIDENCE=APP.parent/'docs/quality/evidence/finance-summary'

def main():
    values=json.loads((APP/'.local/environment.json').read_text(encoding='utf-8'))
    db=urlparse(values['CP_DATABASE_URL'])
    if db.hostname!='127.0.0.1' or db.port!=14660 or values.get('CP_HTTP_ADDR')!='127.0.0.1:18080':
        raise SystemExit('Only the owned native loopback services may be inspected.')
    EVIDENCE.mkdir(parents=True,exist_ok=True)
    spec=importlib.util.spec_from_file_location('baseline',APP/'scripts/verify_agent_execution_development.py')
    baseline=importlib.util.module_from_spec(spec);spec.loader.exec_module(baseline);baseline.EVIDENCE=EVIDENCE
    if baseline.main():return 1
    common=json.loads((EVIDENCE/'development-runtime.json').read_text(encoding='utf-8'))
    env={**os.environ,'PGHOST':db.hostname,'PGPORT':str(db.port),'PGUSER':unquote(db.username),'PGPASSWORD':unquote(db.password),'PGDATABASE':unquote(db.path[1:]),'PGCONNECT_TIMEOUT':'5'}
    query="""SELECT json_build_object(
      'report_append_only',has_table_privilege(current_user,'agent_finance_reports','SELECT') AND has_table_privilege(current_user,'agent_finance_reports','INSERT') AND NOT has_table_privilege(current_user,'agent_finance_reports','UPDATE') AND NOT has_table_privilege(current_user,'agent_finance_reports','DELETE'),
      'report_guards',(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgenabled='O' AND tgname IN ('agent_finance_report_immutable','agent_finance_report_binding','agent_finance_report_receipt')),
      'incident_telegram_constraint',(SELECT count(*) FROM pg_constraint WHERE conrelid='agent_runtime_profiles'::regclass AND conname='agent_runtime_profiles_telegram_configuration_sha256_check' AND pg_get_constraintdef(oid) LIKE '%incident-summary%'),
      'finance_tool_constraint',(SELECT count(*) FROM pg_constraint WHERE conname IN ('agent_tool_receipts_tool_check','agent_execution_observations_tool_check') AND pg_get_constraintdef(oid) LIKE '%read_finance_summary%'),
      'active_profiles',(SELECT count(*) FROM agent_runtime_profiles WHERE state='active'),
      'development_report_count',(SELECT count(*) FROM agent_finance_reports),
      'disposable_test_databases',(SELECT count(*) FROM pg_database WHERE datname ~ '^cp_test_[0-9a-f]{32}$')
    )"""
    proc=subprocess.run([str(APP/'.tools/pgsql/bin/psql.exe'),'-X','-A','-t','-q','-v','ON_ERROR_STOP=1','-c',query],env=env,capture_output=True,text=True,encoding='utf-8',timeout=15,creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0)
    if proc.returncode:raise SystemExit('Owned schema inspection failed; private diagnostics withheld.')
    actual=json.loads(proc.stdout)
    manifests=[json.loads(p.read_text(encoding='utf-8')) for p in sorted((APP/'skills').glob('*/manifest.json'))]
    effective={**os.environ,**values}
    disabled=not effective.get('CP_TELEGRAM_BOTS') and not effective.get('CP_TELEGRAM_DISPATCH_BOTS')
    passed=bool(common['passed'] and common['database']['migration_count']==62 and actual['report_append_only'] and actual['report_guards']==3 and actual['incident_telegram_constraint']==1 and actual['finance_tool_constraint']==2 and actual['active_profiles']==0 and actual['development_report_count']==0 and len(manifests)==8 and all(m['state']=='draft' for m in manifests) and disabled)
    report={'passed':passed,'verification_script_sha256':hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),'checked_at':datetime.now(timezone.utc).isoformat(),'migration_count':common['database']['migration_count'],'database':actual,'all_eight_skills_draft':all(m['state']=='draft' for m in manifests),'production_integrations_disabled':disabled,'actual_model_invoked':False,'actual_telegram_contacted':False,'schema_applied_to_owned_development_only':True}
    (EVIDENCE/'development-finance-summary.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':passed,'migration_count':report['migration_count'],'report_guards':actual['report_guards']}))
    return 0 if passed else 1

if __name__=='__main__':raise SystemExit(main())
