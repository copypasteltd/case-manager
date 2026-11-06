"""Verify full-code-first order and explicitly bounded selected round161 evidence."""
import hashlib
import json
from pathlib import Path
import subprocess
import zipfile
from run_batch_round161 import ROOT, APP, EVIDENCE as E, source_names, source_digest, client_source_names, load_plan

BASE='f83f6ad27da5f07b839256e8c71aa53c22bdb418'
def digest(names=None):return source_digest(names or source_names())
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def read(path):return json.loads(path.read_text(encoding='utf-8'))
def verify():
    records={};checks=[];plan=load_plan();current=digest()
    def record(path):records[path.relative_to(ROOT).as_posix()]=sha(path);return read(path)
    def check(name,value):checks.append({'name':name,'passed':bool(value)})
    def latest(phase,client=False):
        wanted=source_digest(client_source_names()) if client else current
        found=[p for p in E.glob(phase+'-*.json') if read(p).get('passed') and read(p).get('source_sha256')==wanted]
        return record(max(found,key=lambda p:read(p)['started_at'])) if found else {}
    initial=record(E/'freeze-code-initial.json')
    first_native=record(E/'native-tests-initial.json');first_clients=record(E/'client-checks-initial.json')
    native=latest('native-tests');clients=latest('client-checks',True);freeze=latest('freeze-code')
    ui_repair=record(E/'native-retained-disposition.json')
    backend_inputs=[n for n in source_names() if n.startswith('app/backend/')]
    unchanged_backend=ui_repair.get('passed') and ui_repair.get('source_sha256')==current and ui_repair.get('unchanged_backend_source_sha256')==source_digest(backend_inputs)
    if not native and unchanged_backend:
        native=record(E/ui_repair['native_record'])
        check('UI-only repair retains byte-identical selected native inputs without replay',sha(E/ui_repair['native_record'])==ui_repair['native_record_sha256'] and ui_repair['source_changes_after_native_checks']==['app/mobile/src/pages/files/history-restores.tsx'])
        with zipfile.ZipFile(E/'initial-code-wave.zip') as archive:
            old_native={n:archive.read(n) for n in archive.namelist()}
        with zipfile.ZipFile(E/'batch-11-source-delta.zip') as archive:
            old_native.update({n:archive.read(n) for n in archive.namelist()})
        h=hashlib.sha256()
        for name in native['source_files']:
            h.update(name.encode('utf-8')+b'\0'+old_native[name]+b'\0')
        check('Native source archive reconstructs the exact execution and unchanged current backend',h.hexdigest()==native['source_sha256'] and all(old_native[name]==(ROOT/name).read_bytes() for name in backend_inputs))
    check('All eight declared implementations and23 package dispositions precede both initial check batches',
          plan['implementation_completed_before_tests'] and initial['tests_executed'] is False and len(plan['round_tasks'])==8
          and initial['started_at']<min(first_native['started_at'],first_clients['started_at']))
    check('Selected native execution binds unchanged backend inputs after the disclosed UI-only repair',native.get('passed') and native.get('source_unchanged') and not native.get('full_backend_suite_executed'))
    check('Every selected current client step succeeds with unchanged actual inputs',clients.get('passed') and clients.get('source_unchanged') and all(i.get('passed') for i in clients.get('test_results',[])))
    check('Current source is recorded after all batch repairs',freeze.get('source_sha256')==current)
    repair=record(E/'batch-repairs.json');check('Initial failures and batch repair scope remain disclosed',repair.get('initial_failures_preserved') and repair.get('affected_only_replay'))
    for client in ['web','mobile']:
        group=record(E/(client+'-capture-reference.json'));directory=ROOT/group['directory']
        result=record(directory/'result.json');media=record(directory/'media.json');encoded=record(directory/'encoded-media.json');interaction=record(directory/'interaction-media.json')
        manifest=record(directory/'source-manifest.json')
        inputs=manifest.get('files',manifest.get('source_files',[]))
        check(client+' real local UI capture closes all owned browser/server/PG resources',result.get('passed') and result.get('source_unchanged') and result.get('owned_browser_closed') and result.get('owned_review_server_stopped') and result.get('isolated_database_cleanup_passed'))
        check(client+' affected screenshots retain exact byte hashes',all(sha(ROOT/i['file'])==i['sha256'] for i in media['screenshots']))
        check(client+' recorded video and GIF conversion retain exact hashes',encoded.get('passed') and interaction.get('passed') and all(sha(ROOT/i['file'])==i['sha256'] for i in encoded['artifacts']) and sha(ROOT/interaction['file'])==interaction['sha256'])
        h=hashlib.sha256();archived_tools={}
        if client=='web':
            tools=record(E/'capture-tools-repair-2.json')
            check('Retained Web uses its exact archived capture tools',sha(E/'capture-tools-repair-2.zip')==tools['archive_sha256'])
            with zipfile.ZipFile(E/'capture-tools-repair-2.zip') as archive:
                archived_tools={n.removeprefix('app/'):archive.read(n) for n in archive.namelist()}
        for name in inputs:
            data=archived_tools[name] if name in archived_tools else (APP/name).read_bytes()
            h.update(name.encode('utf-8')+b'\0'+data+b'\0')
        check(client+' capture binds current actual application bytes and exact capture-tool source',result.get('source_sha256')==h.hexdigest() and result.get('build_source_unchanged'))
    visual=record(E/'visual-review.json')
    check('Inspected actual screenshots and GIF frames bind immutable bytes',visual.get('reviewed') and len(visual.get('screenshots',[]))==6 and len(visual.get('gif_frames',[]))==2 and all(sha(ROOT/i['file'])==i['sha256'] for i in visual['screenshots']+visual['gif_frames']))
    retained_clients=record(E/'retained-client-dispositions.json')
    check('Retained unit/catalog/signing results are identified without replay claims',retained_clients.get('passed') and not retained_clients.get('reexecuted') and all(sha(ROOT/i['file'])==i['sha256'] for i in retained_clients['records']))
    check('Initial complete source and every failed repair source remain reconstructible',all(sha(E/i['archive'])==i['archive_sha256'] for i in repair['source_archives']))
    build=record(E/'native-command-build.json');build_disposition=record(E/'native-build-disposition.json')
    check('Native command build retained after test-fixture and mobile-only repairs',build.get('passed') and build.get('source_unchanged') and build_disposition.get('passed') and (build_disposition.get('source_sha256')==current or unchanged_backend))
    old_migrations=subprocess.check_output(['git','diff','--name-only',BASE,'--','app/backend/migrations'],cwd=ROOT,text=True).splitlines()
    check('Normal upgrades124–127 preserve migrations1–123',all(Path(n).name[:4] in ('0124','0125','0126','0127') for n in old_migrations) and plan['schema']==127)
    old_proofs=subprocess.check_output(['git','diff','--name-only',BASE,'--','docs/quality/evidence','docs/media'],cwd=ROOT,text=True).splitlines()
    check('Earlier proof and media bytes remain immutable',all(n=='docs/media/README.md' or n.startswith(('docs/quality/evidence/batch-round-161','docs/media/batch-round-161/')) for n in old_proofs))
    tags=subprocess.check_output(['git','tag','--list','v0.1.0-rc.*'],cwd=ROOT,text=True).splitlines()
    check('Eight freezes and all draft packages remain explicit',len(tags)==8 and len(plan['packages'])==23 and len(plan['excluded_ids'])==5 and all(read(p)['state']=='draft' for p in (APP/'skills').glob('*/manifest.json')))
    retained=record(ROOT/'docs/quality/evidence/batch-round-160/final-source-check.json')
    return {'passed':all(i['passed'] for i in checks),'checks':checks,'records':records,'retained':{'round160':retained.get('interaction_source_sha256'),'apk':'154/business153/schema113'},'interaction_source_sha256':current,'verification_script_sha256':sha(Path(__file__)),'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False}
if __name__=='__main__':
    value=verify();(E/'final-source-check.json').write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':value['passed'],'checks':value['checks']},ensure_ascii=False))
    raise SystemExit(0 if value['passed'] else 1)
