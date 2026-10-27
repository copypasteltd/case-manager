"""Verify exact mobile finance sources, artifacts, retained attempts and owned cleanup."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess
import xml.etree.ElementTree as ET
import zipfile
from PIL import Image

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/mobile-finance-operations'
PREVIOUS = E.parent / 'finance-reports'
B = E.parent / 'mobile-finance-operations-mobile'
REF = 'e87656f822b44795305dd3d8a4ca4f2042cdb8a5'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(names, content=None):
    h = hashlib.sha256()
    for name in names:
        h.update(name.encode() + b'\0' + (content(name) if content else (APP / name).read_bytes()) + b'\0')
    return h.hexdigest()


def module(filename):
    spec = importlib.util.spec_from_file_location('mobile_finance_' + filename.replace('.', '_'), APP / 'scripts' / filename)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def main():
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    backend = read(PREVIOUS / 'result.json')
    backend_sha = module('verify_backend_batches.py').digest()
    check('Original increment88 backend evidence is reused only with the same complete backend bytes:161 top-level,1057 results,9 batches',
        backend['passed'] and backend['source_sha256'] == backend_sha and backend['source_unchanged_during_tests']
        and backend['all_discovered_tests_executed'] and backend['discovered_top_level_tests'] == 161
        and backend['batch_count'] == 9 and len(backend['checks']) == 1057 and all(v['Action'] == 'pass' for v in backend['checks']))
    migrations = sorted((APP / 'backend/migrations').glob('*.sql'))
    check('All73 migrations retain their exact published increment88 bytes; no migration was added',
        len(migrations) == 73 and all(subprocess.check_output(['git', 'show', REF + ':' + p.relative_to(ROOT).as_posix()], cwd=ROOT) == p.read_bytes() for p in migrations))
    contract = ROOT / 'docs/development/openapi.json'
    api = read(contract)
    check('Both unchanged OpenAPI copies agree:284 paths,352 operations,619 schemas',
        contract.read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes()
        and len(api['paths']) == 284 and sum(k in ('get','post','put','patch','delete','head','options','trace') for p in api['paths'].values() for k in p) == 352
        and len(api['components']['schemas']) == 619)
    runtime, web = read(PREVIOUS / 'runtime-build.json'), read(PREVIOUS / 'web-build.json')
    check('Original47-test runtime build is reused only after comparing its complete current runtime and Skill digest',
        runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime.py').digest()[0]
        and runtime['tests']['tests'] == runtime['tests']['pass'] == 47 and runtime['tests']['fail'] == runtime['tests']['skipped'] == runtime['tests']['cancelled'] == 0)
    check('Original112-test Web build is reused at its original path with every declared current source byte equal',
        web['passed'] and web['source_unchanged_during_run'] and web['source_sha256'] == digest(web['sources'])
        and web['tests']['numPassedTests'] == 112 and web['tests']['numFailedTests'] == 0)
    linux = read(PREVIOUS / 'linux-build.json')
    check('Original Linux compilation still matches the complete backend; no execution or deployment is implied',
        linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_sha and not linux['artifact_executed'] and not linux['deployed'])
    mobile = read(E / 'mobile-build.json')
    check('Current204 mobile tests, types and two actual mini-program builds pass with unchanged complete declared sources',
        mobile['passed'] and mobile['source_unchanged_during_run'] and mobile['source_sha256'] == module('verify_mobile.py').digest()[0]
        and mobile['tests']['tests'] == mobile['tests']['pass'] == 204 and mobile['tests']['suites'] == 2
        and mobile['tests']['fail'] == mobile['tests']['skipped'] == mobile['tests']['cancelled'] == 0
        and len(mobile['commands']) == 4 and all(c['exit_code'] == 0 for c in mobile['commands']))
    pages = [read(APP / ('mobile/dist/' + target + '/app.json'))['pages'] for target in ('weapp','tt')]
    check('WeChat and Douyin each register the same18 pages including the actual finance page',
        pages[0] == pages[1] and len(pages[0]) == 18 and 'pages/finance/index' in pages[0])
    android, inspection, unit = [read(E / name) for name in ('build.json','apk-inspection.json','android-test-scope.json')]
    check('The actual Android development APK contains the current source-bound bundle and matches its preserved bytes',
        android['passed'] and android['source_unchanged_during_build'] and android['source_sha256'] == module('build_android.py').sources()[0]
        and sha(APP / android['artifacts']['apk']['path']) == android['artifacts']['apk']['sha256']
        and not android['device_executed'] and not android['production_release'] and not android['virtualization_used'])
    check('All18 package inspections bind the actual current APK', inspection['passed'] and len(inspection['checks']) == 18 and all(v['passed'] for v in inspection['checks']))
    native_graph = [name.replace('\\', '/') for name in read(APP / '.local/android/index.android.map')['sources']]
    check('The actual current Android business graph includes all new financial editors, operations, history and exact-input guards',
        all(any(name.endswith('/mobile/src/' + suffix) for name in native_graph) for suffix in
            ('pages/finance/editor.tsx','pages/finance/operations.tsx','pages/finance/history.tsx','platform/financial-input.ts')))
    xml = sorted((E / 'android-unit-results').glob('TEST-*.xml'))
    suites = [ET.parse(file).getroot() for file in xml]
    check('All26 JVM results retain exact XML and native task bytes; cached versus newly executed scope stays explicit',
        unit['passed'] and unit['tests'] == 26 and unit['native_log_sha256'] == sha(E / 'native-build.log')
        and unit['freezer_sha256'] == sha(APP / 'scripts/freeze_mobile_finance_evidence.py')
        and len(xml) == 4 and sum(int(s.attrib['tests']) for s in suites) == 26
        and all(int(s.attrib[k]) == 0 for s in suites for k in ('failures','errors','skipped'))
        and all(unit['xml_sha256'][p.name] == sha(p) for p in xml)
        and unit['new_test_execution'] != unit['gradle_cached'] and not unit['device_executed'])
    full_sha = module('mobile_finance_sources.py').digest()
    browser, manifest, capture = [read(B / name) for name in ('result.json','source-manifest.json','capture.json')]
    check('Nine actual mobile financial journeys,3 widths and full owned browser/API/database cleanup bind stable complete source bytes',
        browser['passed'] and len(browser['checks']) == 9 and len(capture['layouts']) == 3
        and browser['source_sha256'] == browser['source_after_sha256'] == manifest['sha256'] == digest(manifest['files'])
        and browser['build_source_sha256'] == browser['build_source_after_sha256'] == full_sha
        and browser['source_unchanged'] and browser['build_source_unchanged']
        and browser['native_fixture_exit_code'] == 0 and browser['isolated_database_cleanup_passed']
        and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['browser_contexts_closed'] == 1
        and not browser['console_errors'] and not browser['server_errors']
        and all(v['document'] <= v['width'] + 1 for v in capture['layouts']))
    proof = read(B / 'business-proof.json')
    check('The independently approved employee100 claim actually settles60/40 then returns to0/100 after its separately dated cash reversal',
        len(proof) == 1 and proof[0]['synthetic_data'] and proof[0]['cost_state'] == 'reversed'
        and proof[0]['employee_settlement_before_correction']['paid_amount'] == '60.000000'
        and proof[0]['employee_settlement_before_correction']['outstanding_amount'] == '40.000000'
        and proof[0]['employee_settlement_after_correction']['paid_amount'] == '0.000000'
        and proof[0]['employee_settlement_after_correction']['outstanding_amount'] == '100.000000')
    media = read(E / 'media.json')
    check('Two actual draft frame manifests bind the four published MP4/GIF artifacts and exact encoder bytes',
        media['passed'] and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_mobile_finance_media.py')
        and len(media['recordings']) == len(capture['recordings']) == 2
        and all(r['source_sha256'] == browser['source_sha256'] and r['build_source_sha256'] == full_sha
            and r['capture_manifest_sha256'] == sha(ROOT / r['capture_manifest']) and r['capture_frame_count'] > 5
            and len(r['artifacts']) == 2 and all(sha(ROOT / a['file']) == a['sha256'] and (ROOT / a['file']).stat().st_size == a['bytes'] for a in r['artifacts']) for r in media['recordings']))
    animation_valid = True
    for r in media['recordings']:
        file = next(ROOT / a['file'] for a in r['artifacts'] if a['file'].endswith('.gif'))
        with Image.open(file) as image:
            duration = 0
            for i in range(image.n_frames):
                image.seek(i); duration += image.info.get('duration', 0)
            animation_valid = animation_valid and image.n_frames > 1 and abs(duration * r['gif_speed_multiplier'] - r['capture_duration_ms']) < 800
    check('Both GIFs contain real animated frames and capture-derived bounded playback timing', animation_valid)
    visual = read(E / 'visual-review.json')
    check('Actual pixel review binds all4 screenshots and both GIF first/middle/last samples',
        visual['passed'] and len(visual['files']) == 6 and all(sha(ROOT / p['file']) == p['sha256'] for p in visual['files']))
    skills = [read(p) for p in sorted((APP / 'skills').glob('*/manifest.json'))]
    update = next(s for s in skills if s['skill_id'] == 'project-update')
    check('All8 Skills stay draft; unchanged project-update1.6.0 retains16 allowed and14 completion tools',
        len(skills) == 8 and all(s['state'] == 'draft' for s in skills) and update['version'] == '1.6.0'
        and len(update['allowed_tools']) == 16 and len(update['completion_tools']) == 14)
    dev = read(E / 'development-project-resources.json')
    check('Fresh read-only probes establish all73 migrations, owned DB-only workers and disabled integrations with zero disposable databases',
        dev['passed'] and dev['migration_count'] == 73 and dev['integrations_disabled'] and dev['all_eight_skills_draft']
        and dev['database']['disposable_test_databases'] == dev['database']['active_profiles'] == dev['database']['enabled_telegram_channels'] == 0
        and all(dev['ownership'].values()) and dev['report_worker_source_matches'] and dev['database']['report_guards'] == 4)
    history = read(E / 'historical-88.json')
    check('Published88 startup and both captures reconstruct from exact Git objects and all80 old evidence artifacts retain original bytes',
        history == module('record_mobile_finance_history.py').reconstruct() and history['complete_startup_source']['reconstructed']
        and len(history['preserved_evidence']) == 80 and all(v['complete_capture_manifest_reconstructed'] for v in history['clients'].values()))
    archives_valid, archive_count = True, 0
    for file in sorted((E / 'attempts').glob('*/attempt.json')):
        attempt = read(file); archive = file.parent / 'sources.zip'; archive_count += 1
        with zipfile.ZipFile(archive) as z:
            names = sorted(z.namelist())
            archives_valid = archives_valid and len(names) == attempt['source_count'] and sha(archive) == attempt['source_zip_sha256']
            archives_valid = archives_valid and digest(names, z.read) == attempt['complete_startup_source_sha256']
            if attempt['browser_was_started']:
                m, r = read(file.parent / 'source-manifest.json'), read(file.parent / 'result.json')
                archives_valid = archives_valid and digest(m['files'], z.read) == m['sha256'] == r['source_sha256']
                archives_valid = archives_valid and r['build_source_sha256'] == attempt['complete_startup_source_sha256']
            archives_valid = archives_valid and attempt['diagnostic_only'] and not attempt['passed'] and attempt['browser_closed'] and attempt['database_cleanup_passed']
    check('Every failed attempt retains exact complete startup/capture bytes and is never relabeled a current pass', archives_valid and archive_count >= 5)
    with socket.socket() as probe:
        probe.settimeout(1); released = probe.connect_ex(('127.0.0.1',5189)) != 0
    check('The owned review port is released and the private fixture has been removed', released and not (APP / '.local/business-browser-fixture.json').exists())
    gaps = (ROOT / 'docs/delivery/03-goal-gap-assessment.md').read_text(encoding='utf-8')
    design = (ROOT / 'docs/development/89-mobile-financial-operations.md').read_text(encoding='utf-8')
    check('All28 gap IDs and the complete four-client real-device/cloud/model/production acceptance limits remain explicit',
        all('GAP-' + str(i).zfill(2) in gaps for i in range(1,29)) and all(s in design for s in ('完整四端','Android','MFA','100,000')))
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'checks': checks, 'backend_source_sha256': backend_sha, 'browser_source_sha256': full_sha,
        'mobile_source_sha256': mobile['source_sha256'], 'full_goal_complete': False,
        'reused_evidence_group': 'finance-reports', 'reused_evidence_was_reexecuted': False,
        'failed_attempts': archive_count, 'verification_script_sha256': sha(Path(__file__)),
        'scope': 'Actual current mobile native builds and synthetic browser flows; exact-byte reuse of original backend/Web/runtime/Linux evidence; explicit JVM cache scope; no real device/cloud/model/TG/production acceptance.'}
    (E / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
