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
E = ROOT / 'docs/quality/evidence/dependency-hardening'
PREVIOUS = E.parent / 'finance-reports'
B = E.parent / 'dependency-hardening-mobile'
REF = 'e87656f822b44795305dd3d8a4ca4f2042cdb8a5'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


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
    runtime, web = read(E / 'runtime-build.json'), read(E / 'web-build.json')
    check('Fresh47-test runtime build matches current runtime and Skill digest',
        runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime.py').digest()[0]
        and runtime['tests']['tests'] == runtime['tests']['pass'] == 47 and runtime['tests']['fail'] == runtime['tests']['skipped'] == runtime['tests']['cancelled'] == 0)
    check('Fresh112-test Web build binds every declared current source byte',
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
        and unit['freezer_sha256'] == sha(APP / 'scripts/freeze_dependency_hardening_evidence.py')
        and len(xml) == 4 and sum(int(s.attrib['tests']) for s in suites) == 26
        and all(int(s.attrib[k]) == 0 for s in suites for k in ('failures','errors','skipped'))
        and all(unit['xml_sha256'][p.name] == sha(p) for p in xml)
        and unit['new_test_execution'] != unit['gradle_cached'] and not unit['device_executed'])
    full_sha = module('dependency_hardening_sources.py').digest()
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
        media['passed'] and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_dependency_hardening_media.py')
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
    history = read(E / 'historical-89.json')
    check('Published89 startup/capture reconstruct from Git; all103 evidence/media files stay at original bytes',
        history == module('record_dependency_hardening_history.py').reconstruct()
        and history['complete_startup_source']['reconstructed'] and len(history['preserved_evidence']) == 103
        and history['preserved_media_count'] == 8 and all(v['complete_capture_manifest_reconstructed'] for v in history['clients'].values()))
    publication = read(E / 'publication-before.json')
    check('All12 original publication snapshots retain their exact published89 bytes',
        publication['all_snapshots_match_published_bytes'] and len(publication['files']) == 12
        and all(sha(ROOT / f['snapshot']) == f['sha256'] and (ROOT / f['snapshot']).read_bytes() == subprocess.check_output(['git','show',publication['commit'] + ':' + f['file']],cwd=ROOT) for f in publication['files']))
    security = read(E / 'security-checks.json')
    check('Twelve bounded archive/prototype/serialization regressions and two native JS compatibility cases pass against current declared bytes',
        security['passed'] and security['source_unchanged'] and security['source_sha256'] == module('verify_dependency_security.py').digest()
        and [c['tests']['pass'] for c in security['checks']] == [12,2]
        and all(c['passed'] and c['log_sha256'] == sha(E / (c['name'] + '.log')) for c in security['checks'])
        and security['selected_dependency_tree_valid'] and security['selected_dependency_tree_sha256'] == sha(E / 'selected-dependency-tree.json'))
    audit, raw = read(E / 'dependency-audit.json'), read(E / 'audit-after.json')
    check('Complete current lock audit has zero critical findings; every remaining finding and open production gate stays explicit',
        audit['passed'] and audit['source_unchanged'] and audit['package_lock_sha256'] == sha(APP / 'package-lock.json')
        and audit['raw_report_sha256'] == sha(E / audit['raw_report'])
        and audit['vulnerabilities'] == raw['metadata']['vulnerabilities'] and audit['vulnerabilities']['critical'] == 0
        and audit['vulnerabilities']['high'] > 0 and not audit['production_security_gate_passed']
        and audit['all_dependency_scopes_included'] and not audit['advisories_suppressed'])
    before_source = read(E / 'audit-before-source.json')
    before_raw = read(E / 'audit-before.json')
    remaining = read(E / 'remaining-findings.json')
    check('Before audit binds unchanged published89 lock; all86 remaining packages retain original current advisory content',
        before_source['raw_audit_sha256'] == sha(E / 'audit-before.json')
        and before_source['package_lock_sha256'] == hashlib.sha256(subprocess.check_output(['git','show',before_source['commit'] + ':app/package-lock.json'],cwd=ROOT)).hexdigest()
        and audit['before_vulnerabilities'] == before_raw['metadata']['vulnerabilities']
        and remaining['count'] == len(remaining['findings']) == audit['vulnerabilities']['total'] == 86
        and {v['name'] for v in remaining['findings']} == set(raw['vulnerabilities'])
        and all(v['via'] == raw['vulnerabilities'][v['name']]['via'] and v['nodes'] == raw['vulnerabilities'][v['name']]['nodes'] for v in remaining['findings'])
        and not remaining['production_security_gate_passed'])
    attempt = E / 'attempts/resolution-01'
    diagnosis = read(attempt / 'diagnosis.json'); archive_count = 1
    check('Initial incompatible webpack failure remains retained; missing original lock bytes are disclosed, not invented',
        not diagnosis['passed'] and not diagnosis['complete_original_lock_bytes_retained']
        and diagnosis['available_source_zip_sha256'] == sha(attempt / 'available-sources.zip')
        and all(r['sha256'] == sha(attempt / r['file']) for r in diagnosis['records'])
        and not read(attempt / 'mobile-build.json')['passed'] and bool(diagnosis['limitation']))
    complete_attempts = True
    for attempt_file in sorted((E / 'attempts').glob('*/attempt.json')):
        a = read(attempt_file); archive = attempt_file.parent / 'sources.zip'; archive_count += 1
        with zipfile.ZipFile(archive) as z:
            names = sorted(z.namelist())
            complete_attempts = complete_attempts and len(names) == a['source_count'] and sha(archive) == a['source_zip_sha256']
            complete_attempts = complete_attempts and digest(names, z.read) == a['complete_startup_source_sha256']
            if a['browser_was_started']:
                m, r = read(attempt_file.parent / 'source-manifest.json'), read(attempt_file.parent / 'result.json')
                complete_attempts = complete_attempts and digest(m['files'], z.read) == m['sha256'] == r['source_sha256']
                complete_attempts = complete_attempts and r['build_source_sha256'] == a['complete_startup_source_sha256']
                complete_attempts = complete_attempts and a['raw_recording_frames_ignored'] and len(a['capture_manifests']) == 2 and all(c['sha256'] == sha(attempt_file.parent / c['file']) for c in a['capture_manifests'])
            complete_attempts = complete_attempts and not a['passed'] and a['diagnostic_only'] and a['browser_closed'] and a['database_cleanup_passed']
    check('Both later startup/rate-limit failures retain their actual complete sources and cleanup; neither is labeled a current pass',
        complete_attempts and archive_count == 3)
    with socket.socket() as probe:
        probe.settimeout(1); released = probe.connect_ex(('127.0.0.1',5189)) != 0
    check('The owned review port is released and the private fixture has been removed', released and not (APP / '.local/business-browser-fixture.json').exists())
    gaps = (ROOT / 'docs/delivery/03-goal-gap-assessment.md').read_text(encoding='utf-8-sig')
    design = (ROOT / 'docs/development/90-dependency-hardening.md').read_text(encoding='utf-8-sig')
    check('All28 gap IDs and the complete four-client real-device/cloud/model/production acceptance limits remain explicit',
        all('GAP-' + str(i).zfill(2) in gaps for i in range(1,29)) and all(s in design for s in ('完整四端','Android','MFA','11.1.4')))
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'checks': checks, 'backend_source_sha256': backend_sha, 'browser_source_sha256': full_sha,
        'mobile_source_sha256': mobile['source_sha256'], 'full_goal_complete': False,
        'reused_evidence_group': 'finance-reports', 'reused_evidence_was_reexecuted': False, 'fresh_web_runtime_builds': True,
        'failed_attempts': archive_count, 'verification_script_sha256': sha(Path(__file__)),
        'scope': 'Fresh Web/runtime/mobile/native builds, bounded security regressions, complete audit and actual synthetic UI flows; exact-byte backend/Linux evidence reuse; explicit JVM cache scope; no real device/cloud/model/TG/production acceptance.'}
    (E / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
