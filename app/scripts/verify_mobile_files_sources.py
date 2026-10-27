"""Bind mobile file workflows, actual local browser evidence and declared source-matched reuse."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import socket
import subprocess
import xml.etree.ElementTree as ET
import zipfile
from PIL import Image

APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent
E=ROOT/'docs/quality/evidence/mobile-files'
OLD=E.parent/'agent-indexed-search'
REF='54f340c1c17c59bc70ac640330a26590c5adad2f'
def read(p): return json.loads(p.read_text(encoding='utf-8-sig'))
def sha(p): return hashlib.sha256(p.read_bytes()).hexdigest()
def digest(names, content=None):
    h=hashlib.sha256()
    for n in names:h.update(n.encode()+b'\0'+(content(n) if content else (APP/n).read_bytes())+b'\0')
    return h.hexdigest()
def module(filename):
    spec=importlib.util.spec_from_file_location('files_'+filename.replace('.','_'),APP/'scripts'/filename)
    value=importlib.util.module_from_spec(spec);spec.loader.exec_module(value);return value

def main():
    checks=[]
    def check(name,passed):checks.append({'name':name,'passed':bool(passed)})
    backend=read(OLD/'result.json');backend_sha=module('verify_backend_batches.py').digest()
    check('Reuse published93 full backend167 top-level/1129 records/9 batches by unchanged complete source; no fresh full run',backend['passed'] and backend['source_sha256']==backend_sha and backend['all_discovered_tests_executed'] and backend['discovered_top_level_tests']==167 and len(backend['checks'])==1129 and backend['batch_count']==9)
    for group, filename, verifier, count in [('runtime','runtime-build.json','verify_agent_runtime.py',51),('Web','web-build.json',None,130)]:
        r=read(OLD/filename);current=module(verifier).digest()[0] if verifier else digest(r['sources'])
        check('Reuse unchanged-source published93 '+group+' checks; no fresh run',r['source_sha256']==current and (r['status']=='passed' and r['tests']['pass']==count if verifier else r['passed'] and r['tests']['numPassedTests']==count))
    linux=read(OLD/'linux-build.json')
    check('Reuse unchanged-source93 Linux compile; no artifact execution or deployment',linux['passed'] and linux['source_sha256']==backend_sha and not linux['artifact_executed'] and not linux['deployed'])
    migrations=sorted((APP/'backend/migrations').glob('*.sql'))
    check('All75 migrations and both contracts retain published93 bytes',len(migrations)==75 and all(p.read_bytes()==subprocess.check_output(['git','show',REF+':'+p.relative_to(ROOT).as_posix()],cwd=ROOT) for p in migrations) and (ROOT/'docs/development/openapi.json').read_bytes()==(APP/'backend/internal/contract/openapi.json').read_bytes()==subprocess.check_output(['git','show',REF+':app/backend/internal/contract/openapi.json'],cwd=ROOT))
    mobile=read(E/'mobile-build.json')
    check('Fresh233 mobile tests, types and both mini-program builds bind current complete sources',mobile['passed'] and mobile['source_unchanged_during_run'] and mobile['source_sha256']==module('verify_mobile.py').digest()[0] and mobile['tests']['tests']==mobile['tests']['pass']==233 and all(mobile['tests'][k]==0 for k in ('fail','skipped','cancelled')) and len(mobile['commands'])==4 and all(c['exit_code']==0 for c in mobile['commands']))
    pages=[read(APP/('mobile/dist/'+t+'/app.json'))['pages'] for t in ('weapp','tt')]
    check('Both mini-programs register the same20 pages including the actual file workspace',pages[0]==pages[1] and len(pages[0])==20 and 'pages/files/index' in pages[0])
    android,inspection,unit=[read(E/n) for n in ('build.json','apk-inspection.json','android-test-scope.json')]
    check('Fresh Android development artifact binds current source and18 APK checks; no device claim',android['passed'] and android['source_unchanged_during_build'] and android['source_sha256']==module('build_android.py').sources()[0] and sha(APP/android['artifacts']['apk']['path'])==android['artifacts']['apk']['sha256']==inspection['apk_sha256'] and inspection['passed'] and len(inspection['checks'])==18 and all(c['passed'] for c in inspection['checks']) and not android['device_executed'] and not android['production_release'] and not android['virtualization_used'])
    xml=sorted((E/'android-unit-results').glob('TEST-*.xml'));suites=[ET.parse(p).getroot() for p in xml]
    check('26 JVM results and original Gradle task/cache scope are frozen explicitly',unit['passed'] and len(xml)==4 and sum(int(s.attrib['tests']) for s in suites)==unit['tests']==26 and all(int(s.attrib[k])==0 for s in suites for k in ('failures','errors','skipped')) and all(sha(p)==unit['xml_sha256'][p.name] for p in xml) and sha(E/'native-build.log')==unit['native_log_sha256'] and sha(APP/'scripts/freeze_mobile_files_evidence.py')==unit['freezer_sha256'] and unit['new_test_execution']==(not unit['gradle_cached']) and not unit['device_executed'])
    browser=read(E.parent/'mobile-files-browser/result.json');manifest=read(E.parent/'mobile-files-browser/source-manifest.json');full_sha=module('mobile_files_sources.py').digest()
    check('Nine actual authenticated browser/API flows and two identities bind stable startup/build/capture bytes',browser['passed'] and len(browser['checks'])==9 and browser['source_sha256']==browser['source_after_sha256']==manifest['sha256']==digest(manifest['files'])==full_sha and browser['source_sha256_before_fixture_and_build']==browser['source_sha256_after_build']==full_sha and browser['source_unchanged'] and browser['source_unchanged_during_fixture_and_build'] and browser['build_and_browser_sources_match'] and browser['real_native_api'] and not browser['real_cloud'] and not browser['real_model'] and not browser['real_device'] and not browser['real_telegram_network'] and not browser['errors'] and not browser['server_errors'])
    check('Both browser contexts, owned review server and isolated native database cleaned up',browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['native_fixture_exit_code']==0 and browser['isolated_database_cleanup_passed'] and browser['browser_contexts_created']==browser['browser_contexts_closed']==2 and not browser['cleanup_errors'])
    check('Eight explicit H5 portrait/landscape/tablet/dark and200percent root-font layouts; browser scope only',len(browser['layouts'])==8 and all(v['passed'] for v in browser['layouts']) and len([v for v in browser['layouts'] if v.get('browser_text_percent')==200 and not v['real_device']])==2)
    check('Four actual screenshots have exact published bytes and explicit synthetic scope',len(browser['screenshots'])==4 and all(sha(ROOT/p['file'])==p['sha256'] and p['synthetic_data'] and not p['real_device'] for p in browser['screenshots']))
    media=read(E/'media.json');valid=media['passed'] and media['encoder_script_sha256']==sha(APP/'scripts/encode_mobile_files_media.py') and len(media['recordings'])==2
    for r in media['recordings']:
        frames=read(ROOT/r['capture_manifest']);clip=next(c for c in browser['recordings'] if c['name']=='mobile-files-'+r['client'])
        valid=valid and r['source_sha256']==r['build_source_sha256']==full_sha and r['capture_manifest_sha256']==sha(ROOT/r['capture_manifest'])==clip['manifest_sha256'] and frames['frame_count']==r['capture_frame_count']>5 and frames['real_browser_pixels'] and frames['credentials_excluded'] and frames['synthetic_data'] and all(sha(ROOT/a['file'])==a['sha256'] and (ROOT/a['file']).stat().st_size==a['bytes']<15_000_000 for a in r['artifacts'])
        with Image.open(next(ROOT/a['file'] for a in r['artifacts'] if a['file'].endswith('.gif'))) as pic:
            duration=0
            for i in range(pic.n_frames):pic.seek(i);duration+=pic.info.get('duration',0)
            valid=valid and pic.n_frames>1 and abs(duration*r['gif_speed_multiplier']-r['capture_duration_ms'])<800
    check('Two real-pixel captures bind original frame hashes, timing and four MP4/GIF artifacts',valid)
    visual=read(E/'visual-review.json')
    check('Direct inspection binds four PNGs and first/middle/last samples of both GIFs',visual['passed'] and len(visual['files'])==6 and all(sha(ROOT/p['file'])==p['sha256'] for p in visual['files']))
    history=read(E/'historical-93.json')
    check('Published93 source reconstruction and89 original evidence/media files including8 media retain original bytes',history==module('record_mobile_files_history.py').reconstruct() and len(history['preserved'])==89 and history['preserved_media_count']==8)
    snapshots=read(E/'publication-before.json')
    check('Ten original published93 entry snapshots remain exact Git objects',len(snapshots['files'])==10 and all(sha(ROOT/v['snapshot'])==v['sha256'] and (ROOT/v['snapshot']).read_bytes()==subprocess.check_output(['git','show',REF+':'+v['path']],cwd=ROOT) for v in snapshots['files']))
    valid=True;attempts=sorted((E/'attempts').glob('*/attempt.json'))
    for p in attempts:
        a=read(p);r=read(p.parent/'result.json');m=read(p.parent/'source-manifest.json')
        valid=valid and all(sha(ROOT/v['path'])==v['sha256'] for v in a.get('original_media',[]))
        with zipfile.ZipFile(p.parent/'sources.zip') as z:
            valid=valid and a['passed']==r['passed'] and (not a['passed'] or a['superseded']) and a['diagnostic_only'] and sha(p.parent/'sources.zip')==a['source_zip_sha256'] and digest(a['startup_manifest'],z.read)==a['complete_startup_source_sha256'] and digest(m['files'],z.read)==m['sha256']==a['capture_source_sha256'] and a['owned_browser_closed'] and a['owned_review_server_stopped'] and a['isolated_database_cleanup_passed']
    check('Three failed browser observations and one passing fast-capture supersession retain exact sources and original status',bool(attempts) and valid)
    diagnostic=read(E/'diagnostics/05-apk-page-count/diagnostic.json')
    check('Original19-page inspection mismatch retains failed report and exact inspector; limited diagnostic scope',not diagnostic['passed'] and diagnostic['diagnostic_only'] and all(sha(ROOT/v['path'])==v['sha256'] for v in diagnostic['files']))
    diagnostic=read(E/'diagnostics/06-publication-scope/diagnostic.json')
    check('Initial candidate whitespace-scope mismatch retains original report and baseline; binary/history bytes unchanged',not diagnostic['passed'] and diagnostic['diagnostic_only'] and all(sha(ROOT/v['path'])==v['sha256'] for v in diagnostic['files']))
    audit=read(E.parent/'dependency-hardening/dependency-audit.json')
    check('Unchanged lock explicitly reuses90 audit86 findings/20high; security gate remains open',audit['package_lock_sha256']==sha(APP/'package-lock.json') and audit['vulnerabilities']['total']==86 and audit['vulnerabilities']['high']==20 and not audit['production_security_gate_passed'])
    skills=[read(p) for p in (APP/'skills').glob('*/manifest.json')]
    check('All eight Skills remain draft with no source changes',len(skills)==8 and all(s['state']=='draft' for s in skills) and not subprocess.check_output(['git','diff',REF,'--','app/skills'],cwd=ROOT))
    dev,runtime=[read(E/n) for n in ('development-project-resources.json','development-runtime.json')]
    check('Fresh read-only native development inspection;75 migrations, no disposable database or external activation',dev['passed'] and runtime['passed'] and dev['migration_count']==75 and dev['integrations_disabled'] and dev['database']['disposable_test_databases']==0 and all(dev['ownership'].values()) and dev['verification_script_sha256']==sha(APP/'scripts/verify_mobile_files_development.py'))
    with socket.socket() as probe:probe.settimeout(1);released=probe.connect_ex(('127.0.0.1',5189))!=0
    check('Owned review port released and private fixture removed',released and not (APP/'.local/business-browser-fixture.json').exists())
    gaps=(ROOT/'docs/delivery/03-goal-gap-assessment.md').read_text(encoding='utf-8-sig')
    check('All28 stable gap IDs and full four-client capacity scope remain explicit',set(re.findall(r'\| (GAP-\d{2}) \|',gaps))=={f'GAP-{i:02}' for i in range(1,29)} and all(t in gaps for t in ('120GB','2TB','Android','微信','抖音')))
    report={'passed':all(c['passed'] for c in checks),'checked_at':datetime.now(timezone.utc).isoformat(),'checks':checks,'backend_source_sha256':backend_sha,'browser_source_sha256':full_sha,'mobile_source_sha256':mobile['source_sha256'],'verification_script_sha256':sha(Path(__file__)),'full_goal_complete':False,'reused_backend_web_runtime_linux_group':'agent-indexed-search','reused_audit_group':'dependency-hardening','audit_reexecuted':False,'scope':'Fresh mobile/native artifact, actual synthetic browser/API flows and readonly development inspection. Prior full backend/Web/runtime/Linux checks reused only by unchanged complete sources. No real cloud/model/TG/device/capacity/production acceptance.'}
    (E/'final-source-check.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');print(json.dumps({'passed':report['passed'],'checks':len(checks),'failed':[c['name'] for c in checks if not c['passed']]}));return 0 if report['passed'] else 1
if __name__=='__main__':raise SystemExit(main())
