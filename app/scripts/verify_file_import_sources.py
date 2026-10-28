"""Verify increment96 from exact code, native interactions, retained media and live checks."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT/'docs/quality/evidence/file-imports'
PUBLISHED='684d822d4d7e74308b6dc217aa0f972ed1542436'


def read(p): return json.loads(p.read_text(encoding='utf-8-sig'))
def sha(p): return hashlib.sha256(p.read_bytes()).hexdigest()
def module(n):
    s=importlib.util.spec_from_file_location('import_'+n,APP/'scripts'/n)
    m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m

def digest(names):
    h=hashlib.sha256()
    for n in names:h.update(n.encode()+b'\0'+(APP/n).read_bytes()+b'\0')
    return h.hexdigest()

def git_bytes(n): return subprocess.check_output(['git','show',PUBLISHED+':'+n],cwd=ROOT)


def main():
    checks=[]
    def check(name,passed):checks.append({'name':name,'passed':bool(passed)})
    backend=read(E.parent/'file-imports-backend/result.json');bs=module('verify_backend_batches.py').digest()
    names={c.get('Test') for c in backend['checks']}
    required={'TestFileImportManifest','TestFileImportCommit','TestFileImportAuthority','TestFileImportNativeRoundTrip','TestFileImportRetryAndDelta','TestImportProtocolBounds','TestNativeImportPlan','TestNativeImportOutput','TestNativeImportTransport','TestImportCommandWorkflow'}
    check('Complete fresh native backend covers all discovered tests and import protocol/transport/integration tests',backend['passed'] and backend['source_sha256']==bs and backend['source_unchanged_during_tests'] and backend['all_discovered_tests_executed'] and backend['native_database'] and required<=names and all(c['Action']=='pass' for c in backend['checks']))
    migrations=sorted((APP/'backend/migrations').glob('*.sql'))
    check('Migration77 is additive and all published76 migrations retain exact bytes',len(migrations)==77 and migrations[-1].name=='0077_file_imports.sql' and all(p.read_bytes()==git_bytes(p.relative_to(ROOT).as_posix()) for p in migrations[:-1]))
    contract=read(ROOT/'docs/development/openapi.json');old=json.loads(git_bytes('docs/development/openapi.json'));verbs={'get','post','put','patch','delete','head','options','trace'}
    check('OpenAPI293 paths/363 operations/635 schemas; all prior operations and models unchanged',len(contract['paths'])==293 and sum(k in verbs for p in contract['paths'].values() for k in p)==363 and len(contract['components']['schemas'])==635 and all(contract['paths'][k]==v for k,v in old['paths'].items()) and all(contract['components']['schemas'][k]==v for k,v in old['components']['schemas'].items()) and (ROOT/'docs/development/openapi.json').read_bytes()==(APP/'backend/internal/contract/openapi.json').read_bytes())
    check('Generated shared types include exact import approval, fixed update base and report data',all(n in (APP/'packages/contracts/src/api.d.ts').read_text(encoding='utf-8') for n in ('FileImportBatchInput','FileImportApprovalInput','FileImportRetryInput','source_namespace','mapped_owner_can_read','base_version_id')))
    cli=read(E/'native-cli.json')
    check('Actual Windows CLI builds/execution and Linux cross-compile match exact current backend',cli['passed'] and cli['source_sha256']==cli['source_after_sha256']==bs and cli['source_unchanged'] and len(cli['checks'])==7 and all(c['passed'] for c in cli['checks']) and cli['private_temp_source_and_outputs_removed'] and not cli['real_cloud'] and not cli['real_2tb_inventory'] and not cli['real_120gb_transport'] and cli['verification_script_sha256']==sha(APP/'scripts/verify_file_import_cli.py'))
    roundtrip=read(E/'native-round-trip.json')
    check('Real synthetic HTTP PUT lost-receipt resume keeps one upload and one logical file',roundtrip['synthetic'] and not roundtrip['real_cloud'] and roundtrip['put_requests']==1 and roundtrip['accepted_put_without_receipt_reconciled'] and roundtrip['logical_files']==roundtrip['upload_sessions']==1 and roundtrip['source_mutation_rejected'])
    check('Primary completion remains pending until actual controlled replica read/write verification',not roundtrip['before_replica']['observed_content_verified'] and roundtrip['before_replica']['pending']==1 and roundtrip['after_replica']['observed_content_verified'] and roundtrip['after_replica']['fully_verified']==1 and not roundtrip['cutover_approved'] and not roundtrip['after_replica']['cutover_approved'])
    linux=read(E/'linux-build.json')
    check('Platform Linux cross-compile matches current backend without execution or deployment',linux['passed'] and linux['source_sha256']==linux['source_after_sha256']==bs and not linux['artifact_executed'] and not linux['deployed'])
    clients=read(E/'client-checks.json')
    good=clients['passed'] and clients['source_unchanged'] and clients['source_sha256']==clients['source_after_sha256']==digest(clients['source_files']) and len(clients['checks'])==8
    good=good and all(c['passed'] and c['exit_code']==0 and sha(APP/'.local'/Path(c['raw_log']).name)==c['raw_log_sha256'] for c in clients['checks'])
    check('Eight fresh client type/test/build commands retain exact source and private raw log bindings',good)
    web,mobile,runtime=[read(E/n) for n in ('web-build.json','mobile-build.json','runtime-build.json')]
    check('Current Web135 and Taro238 tests plus both20-page mini-program builds',web['passed'] and web['source_sha256']==digest(web['sources']) and web['tests']['numPassedTests']==135 and mobile['passed'] and mobile['source_sha256']==module('verify_mobile.py').digest()[0] and mobile['tests']['pass']==238 and all(p['page_count']==20 for p in mobile['targets'].values()))
    check('Current runtime51 tests and draft Skill checks; real model remains unexecuted',runtime['status']=='passed' and runtime['source_sha256']==module('verify_agent_runtime.py').digest()[0] and runtime['tests']['pass']==51 and runtime['skills_check']['passed'] and runtime['skills_check']['executed_now'] and not runtime['real_model'])
    build=read(E/'build.json');inspection=read(E/'apk-inspection.json');scope=read(E/'android-test-scope.json')
    check('Fresh Android development build and actual18 APK inspections match current inputs',build['passed'] and build['source_sha256']==module('build_android.py').sources()[0] and build['source_unchanged_during_build'] and inspection['passed'] and len(inspection['checks'])==18 and all(c['passed'] for c in inspection['checks']) and sha(APP/build['artifacts']['apk']['path'])==build['artifacts']['apk']['sha256'])
    check('Android JVM26 task is honestly scoped as executed or cached; no device execution',scope['passed'] and scope['tests']==26 and scope['new_test_execution']==(not scope['gradle_cached']) and not scope['device_executed'] and sha(E/'native-build.log')==scope['native_log_sha256'])
    media=[]
    for line in subprocess.check_output(['git','ls-tree','-r',PUBLISHED,'--','docs/media'],cwd=ROOT).decode().splitlines():
        meta,name=line.split('\t',1)
        if Path(name).suffix.lower() in ('.png','.gif','.mp4'):
            raw=(ROOT/name).read_bytes();obj=hashlib.sha1(b'blob '+str(len(raw)).encode()+b'\0'+raw).hexdigest();media.append(obj==meta.split()[2])
    check('All156 existing page media files retain exact published95 bytes and capture scope',len(media)==156 and all(media))
    historical=[]
    for group in ('task-mentions','task-mentions-web','task-mentions-mobile','task-mentions-backend'):
        for n in subprocess.check_output(['git','ls-tree','-r','--name-only',PUBLISHED,'--','docs/quality/evidence/'+group],cwd=ROOT).decode().splitlines():historical.append((ROOT/n).read_bytes()==git_bytes(n))
    check('Original95 evidence is retained byte for byte and is not recast as fresh browser evidence',bool(historical) and all(historical))
    previous=read(E/'publication-before.json')
    check('Published bilingual guides, ledger, baseline and migration plan have exact original snapshots',previous['published_commit']==PUBLISHED and all(sha(ROOT/c['snapshot'])==c['sha256'] and (ROOT/c['snapshot']).read_bytes()==git_bytes(c['source']) for c in previous['files']))
    audit=read(E.parent/'dependency-hardening/dependency-audit.json')
    check('Unchanged lock retains original90 dependency audit and open production gate',(APP/'package-lock.json').read_bytes()==git_bytes('app/package-lock.json') and audit['production_security_gate_passed'] is False)
    dev=read(E/'development-project-resources.json');update=read(E/'development-update.json')
    check('Native development schema77 and exact owned API/workers match sources; integrations remain disabled',dev['passed'] and dev['migration_count']==77 and dev['backend_source_sha256']==bs and dev['worker_sources_match'] and all(dev['ownership'].values()) and dev['database']['disposable_test_databases']==0 and dev['integrations_disabled'] and update['passed'] and update['source_unchanged'] and not update['external_integration_started'] and not update['production_deployment'])
    unused=True
    for port in (5189,):
        with socket.socket() as sock:
            sock.settimeout(.2);unused=unused and sock.connect_ex(('127.0.0.1',port))!=0
    check('No completed-check review server remains; no browser was used in96',unused)
    result={'passed':all(c['passed'] for c in checks),'checked_at':datetime.now(timezone.utc).isoformat(),'checks':checks,
        'interaction_kind':'native_cli_and_http','interaction_source_sha256':module('file_import_sources.py').digest(),
        'backend_source_sha256':bs,'mobile_source_sha256':module('verify_mobile.py').digest()[0],
        'verification_script_sha256':sha(Path(__file__)),'full_goal_complete':False,'production_ready':False,'real_cloud':False,'real_120gb_transport':False,'real_2tb_migration':False,
        'browser_evidence_reused_from_increment':95,'new_page_capture':False}
    (E/'final-source-check.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':result['passed'],'checks':len(checks),'failed':[c['name'] for c in checks if not c['passed']]}))
    return 0 if result['passed'] else 1


if __name__=='__main__':raise SystemExit(main())
