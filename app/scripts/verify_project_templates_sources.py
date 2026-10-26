"""Bind the template increment to executed checks, artifacts and owned cleanup."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
from urllib.request import urlopen

APP=Path(__file__).resolve().parents[1]
EVIDENCE=APP.parent/'docs/quality/evidence/project-templates'


def read(path):return json.loads(path.read_text(encoding='utf-8'))
def digest(names):
    result=hashlib.sha256()
    for name in names:result.update(name.encode()+b'\0'+(APP/name).read_bytes()+b'\0')
    return result.hexdigest()
def module(name,file):
    spec=importlib.util.spec_from_file_location(name,APP/'scripts'/file)
    result=importlib.util.module_from_spec(spec);spec.loader.exec_module(result);return result


def main():
    checks=[]
    def check(name,value):checks.append({'name':name,'passed':bool(value)})
    backend=module('template_backend_source','verify_backend_batches.py').digest()
    result=read(EVIDENCE/'result.json')
    check('All discovered backend tests executed without failures or skips against current sources',result['passed'] and result['source_sha256']==backend and result['source_unchanged_during_tests'] and result['all_discovered_tests_executed'] and all(item['Action']=='pass' for item in result['checks']))
    observed={item['Test'] for item in result['checks']}
    template_cases=[item for item in result['checks'] if item['Test'].startswith('TestProjectTemplateManagement/')]
    check('New PostgreSQL/HTTP template authorization, lifecycle, concurrency and rollback cases all passed',len(template_cases)==14 and all(item['Action']=='pass' for item in template_cases))
    check('Secretary exact-plan and existing project/closeout/financial regressions ran',{'TestAgentPlans','TestProjectTemplateManagement','TestProjectChangesAndBudgets','TestProjectCloseoutAndArchive','TestProjectCollaborationIntegration','TestContractFinancialReconciliation'}.issubset(observed))
    linux=read(EVIDENCE/'linux-build.json');artifact=APP/linux['artifact']
    check('Current backend compiles to preserved Linux artifact without executing it',linux['passed'] and linux['source_sha256']==backend and artifact.exists() and hashlib.sha256(artifact.read_bytes()).hexdigest()==linux['artifact_sha256'] and not linux['artifact_executed'] and not linux['deployed'])
    contract=APP/'backend/internal/contract/openapi.json';document=APP.parent/'docs/development/openapi.json'
    check('Generated backend contract and documented OpenAPI are identical',contract.read_bytes()==document.read_bytes() and 'ProjectTemplateAction' in read(contract)['components']['schemas'])
    web=read(EVIDENCE/'web-build.json')
    check('Web tests and production build match final source files',web['passed'] and web['source_unchanged_during_run'] and web['source_sha256']==digest(web['sources']) and web['tests']['numPassedTests']==54 and web['tests']['numFailedTests']==web['tests']['numPendingTests']==0)
    browser_folder=EVIDENCE.parent/'project-templates-web';browser=read(browser_folder/'result.json');manifest=read(browser_folder/'source-manifest.json')
    check('Four actual API browser chains and six settled responsive checks match current sources',browser['passed'] and browser['source_unchanged_during_tests'] and browser['source_sha256']==digest(manifest['files']) and len(browser['checks'])==4 and all(item['passed'] for item in browser['checks']) and len(browser['layouts'])==6 and all(item['passed'] for item in browser['layouts']) and not browser['errors'] and not browser['server_errors'] and not browser.get('failure'))
    check('Owned browser, tabs, temporary Vite and isolated test database closed',browser['owned_browser_closed'] and browser['all_owned_browser_tabs_closed'] and browser['owned_vite_stopped'] and browser['isolated_database_cleanup_passed'] and browser['native_fixture_exit_code']==0 and not browser['cleanup_errors'])
    mobile=read(EVIDENCE/'mobile-build.json')
    check('Current shared types pass mobile tests, typecheck and both mini-program builds',mobile['passed'] and mobile['source_sha256']==module('template_mobile_source','verify_mobile.py').digest()[0] and mobile['source_unchanged_during_run'] and mobile['tests']['pass']==79 and mobile['tests']['fail']==mobile['tests']['skipped']==mobile['tests']['cancelled']==0)
    for target in ('weapp','tt'):
        pages=read(APP/'mobile/dist'/target/'app.json')['pages']
        check(target+' retains all fifteen public pages',len(pages)==15 and 'pages/uploads/index' in pages)
    runtime=read(EVIDENCE.parent/'android-native/runtime-build.json')
    check('Unchanged runtime reuses its passing 34-test source-bound evidence',runtime['status']=='passed' and runtime['source_sha256']==runtime['source_after_sha256']==module('template_runtime_source','verify_agent_runtime.py').digest()[0] and runtime['tests']['pass']==34)
    build=read(EVIDENCE.parent/'android-upload/build.json');apk=APP/build['artifacts']['apk']['path']
    check('Previous frozen Android upload APK remains intact; not claimed as a new native build',build['passed'] and apk.exists() and hashlib.sha256(apk.read_bytes()).hexdigest()==build['artifacts']['apk']['sha256'] and not build['production_release'] and not build['device_executed'])
    dev=read(EVIDENCE/'development-project-templates.json');base=read(EVIDENCE/'development-runtime.json');update=read(EVIDENCE/'development-update.json')
    check('Owned API runs current source and exact 64-migration manifest with append-only history',dev['passed'] and base['passed'] and base['database']['migration_count']==64 and base['database']['migration_manifest_matches_source'] and update['passed'] and update['source_sha256']==backend and not update['postgres_restarted'] and not update['notification_worker_restarted'] and not update['vite_restarted'])
    check('Storage/model/TG remain disabled in development',dev['integrations_disabled'] and dev['database']['active_runtime_profiles']==dev['database']['enabled_telegram_channels']==0 and not update['external_integration_started'])
    owner=module('template_owned_source','verify_notifications_sources.py').owned_process
    worker_owner='project-templates-worker-owner.json' if (APP/'.local/project-templates-worker-owner.json').exists() else 'telegram-delivery-worker-owner.json'
    check('API and notification-worker identities match recorded creation times',owner('project-templates-api-owner.json','api') and owner(worker_owner,'worker-notification'))
    interruption=read(EVIDENCE/'interruption.json');restoration=read(EVIDENCE/'development-restoration.json')
    check('Interrupted services restored after confirmed exit without terminating live processes or reinitializing PG',restoration['passed'] and restoration['source_sha256']==backend==interruption['source_sha256'] and restoration['restored_after_confirmed_process_exit'] and not restoration['live_process_terminated'] and not restoration['pg_cluster_reinitialized'] and not restoration['external_integration_started'] and not interruption['incomplete_batch_counted_as_pass'])
    cleanup=read(EVIDENCE/'interrupted-fixture-cleanup.json')
    check('Empty interrupted fixture verified and removed without forced disconnect; no disposable databases remain',cleanup['passed'] and cleanup['empty_fixture_verified'] and len(cleanup['migration_hashes'])==2 and not cleanup['forced_disconnect'] and not cleanup['application_database_selected'] and dev['database']['disposable_test_databases']==0)
    with socket.socket() as probe:
        probe.settimeout(1)
        check('Temporary review port and private fixture metadata released',probe.connect_ex(('127.0.0.1',5189))!=0 and not (APP/'.local/business-browser-fixture.json').exists())
    for address in ('http://127.0.0.1:18080/health/ready','http://localhost:5178/health/ready','http://localhost:5178/'):
        try:
            with urlopen(address,timeout=5) as response:ok=response.status==200
        except OSError:ok=False
        check('Existing owned service is ready: '+address,ok)
    audit=read(EVIDENCE.parent/'android-native/dependency-audit.json')
    lock_hash=hashlib.sha256((APP/'package-lock.json').read_bytes()).hexdigest()
    check('Dependency lock remains the previously audited one with unresolved findings retained',lock_hash==audit['package_lock_sha256'] and not audit['production_security_gate_passed'])
    report={'passed':all(item['passed'] for item in checks),'checked_at':datetime.now(timezone.utc).isoformat(),'backend_source_sha256':backend,'checks':checks,
        'full_goal_complete':False,'real_cloud_verified':False,'real_model_verified':False,'real_device_verified':False,'production_deployed':False,
        'native_apk_reference':'previous increment66 frozen upload APK; unchanged artifact, no new native feature claimed',
        'runtime_reference':'unchanged source reuses increment65 controlled-runtime evidence',
        'scope':'Project-template management and exact transactional project initialization; excludes project boards/timelines, real resources, all-client template administration and complete first release.'}
    (EVIDENCE/'final-source-check.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':report['passed'],'checks':len(checks),'failed':[item['name'] for item in checks if not item['passed']]}))
    return 0 if report['passed'] else 1


if __name__=='__main__':raise SystemExit(main())
