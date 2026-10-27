"""Bind search implementation, actual native checks, synthetic pixels and retained history."""
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

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/agent-indexed-search'
REF = '5de7553dd165e69c8566394a76095ad41bee300b'


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
    spec = importlib.util.spec_from_file_location('search_' + filename.replace('.', '_'), APP / 'scripts' / filename)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def main():
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    backend = read(E / 'result.json')
    backend_sha = module('verify_backend_batches.py').digest()
    check('Full native backend:167 top-level tests,1129 result records,9 batches and stable complete sources',
        backend['passed'] and backend['source_sha256'] == backend_sha and backend['source_unchanged_during_tests']
        and backend['all_discovered_tests_executed'] and backend['discovered_top_level_tests'] == 167
        and backend['batch_count'] == 9 and len(backend['checks']) == 1129
        and all(c['Action'] == 'pass' for c in backend['checks']))
    cases = [c for c in backend['checks'] if c.get('Test', '').startswith(('TestAgentIndexedSearch', 'TestSearchBodyUnicode'))]
    check('Actual indexed-MCP HTTP/PostgreSQL and Unicode body matching cases passed',
        len(cases) == 17 and all(c['Action'] == 'pass' for c in cases))
    migrations = sorted((APP / 'backend/migrations').glob('*.sql'))
    check('All75 migrations remain byte-identical to published92',
        len(migrations) == 75 and migrations[-1].name == '0075_file_text_search.sql'
        and all(subprocess.check_output(['git', 'show', REF + ':' + p.relative_to(ROOT).as_posix()], cwd=ROOT) == p.read_bytes() for p in migrations))
    api_path = ROOT / 'docs/development/openapi.json'
    api = read(api_path)
    check('Matching OpenAPI copies declare285 paths,353 operations,622 schemas and optional derived-context evidence',
        api_path.read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes()
        and len(api['paths']) == 285 and len(api['components']['schemas']) == 622
        and sum(k in ('get', 'post', 'put', 'patch', 'delete', 'head', 'options', 'trace') for p in api['paths'].values() for k in p) == 353
        and '/search' in api['paths'] and '/files/{id}/versions/{version}/text' in api['paths']
        and 'text_index' in api['components']['schemas']['ConversationContextView']['properties']
        and 'text_index' not in api['components']['schemas']['ConversationContextView'].get('required', [])
        and 'FileTextIndexEvidence' in api['components']['schemas'])
    runtime, web = read(E / 'runtime-build.json'), read(E / 'web-build.json')
    check('Fresh51 runtime tests and build bind current runtime and Skill bytes',
        runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime.py').digest()[0]
        and runtime['tests']['tests'] == runtime['tests']['pass'] == 51
        and all(runtime['tests'][k] == 0 for k in ('fail', 'skipped', 'cancelled')))
    check('Fresh130 Web tests and production bundle bind current unchanged declared source bytes',
        web['passed'] and web['source_unchanged_during_run'] and web['source_sha256'] == digest(web['sources'])
        and web['tests']['numPassedTests'] == 130 and web['tests']['numFailedTests'] == 0)
    linux = read(E / 'linux-build.json')
    check('Fresh Linux compile binds current backend; no artifact execution or deployment',
        linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_sha
        and not linux['artifact_executed'] and not linux['deployed'])
    mobile = read(E / 'mobile-build.json')
    check('Fresh222 mobile tests, types and both mini-program builds bind current unchanged sources',
        mobile['passed'] and mobile['source_unchanged_during_run'] and mobile['source_sha256'] == module('verify_mobile.py').digest()[0]
        and mobile['tests']['tests'] == mobile['tests']['pass'] == 222
        and all(mobile['tests'][k] == 0 for k in ('fail', 'skipped', 'cancelled'))
        and len(mobile['commands']) == 4 and all(c['exit_code'] == 0 for c in mobile['commands']))
    pages = [read(APP / ('mobile/dist/' + target + '/app.json'))['pages'] for target in ('weapp', 'tt')]
    check('Both mini-programs register the same19 pages including search',
        pages[0] == pages[1] and len(pages[0]) == 19 and 'pages/search/index' in pages[0])
    android, inspection, unit = [read(E / n) for n in ('build.json', 'apk-inspection.json', 'android-test-scope.json')]
    check('Current Android development APK binds current bundle/source and18 artifact checks, without device/release claims',
        android['passed'] and android['source_unchanged_during_build'] and android['source_sha256'] == module('build_android.py').sources()[0]
        and sha(APP / android['artifacts']['apk']['path']) == android['artifacts']['apk']['sha256'] == inspection['apk_sha256']
        and inspection['passed'] and len(inspection['checks']) == 18 and all(c['passed'] for c in inspection['checks'])
        and not android['device_executed'] and not android['production_release'] and not android['virtualization_used'])
    xml = sorted((E / 'android-unit-results').glob('TEST-*.xml'))
    suites = [ET.parse(p).getroot() for p in xml]
    check('26 retained JVM results, native log and freezer hashes preserve the actual Gradle cache scope',
        unit['passed'] and len(xml) == 4 and sum(int(s.attrib['tests']) for s in suites) == 26
        and all(int(s.attrib[k]) == 0 for s in suites for k in ('failures', 'errors', 'skipped'))
        and all(sha(p) == unit['xml_sha256'][p.name] for p in xml)
        and sha(E / 'native-build.log') == unit['native_log_sha256']
        and sha(APP / 'scripts/freeze_agent_indexed_search_evidence.py') == unit['freezer_sha256']
        and unit['new_test_execution'] == (not unit['gradle_cached']) and not unit['device_executed'])
    full_sha = module('agent_indexed_search_sources.py').digest()
    for client in ('web', 'mobile'):
        base = E.parent / ('agent-indexed-search-' + client)
        browser, manifest, capture = [read(base / n) for n in ('result.json', 'source-manifest.json', 'capture.json')]
        check(client + ':8 actual authenticated UI/API checks bind exact sources and completed owned-resource cleanup',
            browser['passed'] and len(browser['checks']) == 8 and capture['synthetic_data']
            and browser['source_sha256'] == browser['source_after_sha256'] == manifest['sha256'] == digest(manifest['files'])
            and browser['build_source_sha256'] == browser['build_source_after_sha256'] == full_sha
            and browser['source_unchanged'] and browser['build_source_unchanged']
            and browser['native_fixture_exit_code'] == 0 and browser['isolated_database_cleanup_passed']
            and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['browser_contexts_closed'] == 1
            and not browser['console_errors'] and not browser['server_errors'])
        acceptance = read(base / 'acceptance.json')
        check(client + ': current exact-version UI preparation and historical result withdrawal; no implicit send or live model',
            acceptance['passed'] and acceptance['actual_native_api'] and acceptance['controlled_MCP_result']
            and acceptance['historical_result_withdrawal'] and acceptance['implicit_sends'] == 0
            and not acceptance['real_cloud'] and not acceptance['real_model'] and not acceptance['actual_device'])
    media = read(E / 'media.json')
    valid = media['passed'] and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_agent_indexed_search_media.py') and len(media['recordings']) == 2
    for r in media['recordings']:
        result = read(E.parent / ('agent-indexed-search-' + r['client']) / 'result.json')
        frame_manifest = read(ROOT / r['capture_manifest'])
        valid = valid and r['source_sha256'] == result['source_sha256'] and r['build_source_sha256'] == full_sha
        valid = valid and r['capture_manifest_sha256'] == sha(ROOT / r['capture_manifest']) and r['capture_frame_count'] == frame_manifest['frame_count'] > 5
        valid = valid and frame_manifest['real_browser_pixels'] and frame_manifest['credentials_excluded'] and frame_manifest['synthetic_data']
        valid = valid and len(r['artifacts']) == 2 and all(sha(ROOT / a['file']) == a['sha256'] and (ROOT / a['file']).stat().st_size == a['bytes'] < 15_000_000 for a in r['artifacts'])
        with Image.open(next(ROOT / a['file'] for a in r['artifacts'] if a['file'].endswith('.gif'))) as picture:
            duration = 0
            for index in range(picture.n_frames):
                picture.seek(index)
                duration += picture.info.get('duration', 0)
            valid = valid and picture.n_frames > 1 and abs(duration * r['gif_speed_multiplier'] - r['capture_duration_ms']) < 800
    check('Actual pixels bind two capture manifests and four bounded MP4/GIF artifacts with original timing', valid)
    visual = read(E / 'visual-review.json')
    check('Direct review binds all4 screenshots and both GIF first/middle/last samples',
        visual['passed'] and len(visual['files']) == 6 and all(sha(ROOT / p['file']) == p['sha256'] for p in visual['files']))
    skills = [read(p) for p in sorted((APP / 'skills').glob('*/manifest.json'))]
    update = next(s for s in skills if s['skill_id'] == 'project-update')
    check('All8 Skills stay draft; project-update1.8.0 retains17 allowed and15 completion tools',
        len(skills) == 8 and all(s['state'] == 'draft' for s in skills) and update['version'] == '1.8.0'
        and len(update['allowed_tools']) == 17 and len(update['completion_tools']) == 15
        and {s['skill_id']:s['version'] for s in skills} == {
            'project-create':'1.1.0','project-update':'1.8.0','file-update':'1.2.0',
            'information-organize':'1.2.0','oa-assist':'1.2.0','project-briefing':'1.2.0',
            'subscription-review':'1.3.0','incident-summary':'1.2.0'})
    dev, runtime_dev = read(E / 'development-project-resources.json'), read(E / 'development-runtime.json')
    db = dev['database']
    check('Native development75 schema, text guards, least-privilege role and owned processes checked; no test DB or external activation',
        dev['passed'] and runtime_dev['passed'] and dev['migration_count'] == 75 and dev['integrations_disabled'] and dev['all_eight_skills_draft']
        and db['disposable_test_databases'] == db['active_profiles'] == db['enabled_telegram_channels'] == 0
        and all(dev['ownership'].values()) and dev['report_worker_source_matches']
        and db['text_table'] and db['text_guard'] == 1 and db['text_indexes'] == 3 and db['text_lock_definer']
        and db['formal_version_update_denied'] and db['text_delete_denied']
        and dev['verification_script_sha256'] == sha(APP / 'scripts/verify_agent_indexed_search_development.py'))
    history = read(E / 'historical-92.json')
    check('Published92 sources reconstruct from Git; all91 original evidence/media files and8 media remain byte-identical',
        history == module('record_agent_indexed_search_history.py').reconstruct() and history['complete_startup_source']['reconstructed']
        and len(history['preserved']) == 91 and history['preserved_media_count'] == 8
        and all(c['reconstructed'] for c in history['clients'].values()))
    publication = read(E / 'publication-before.json')
    check('All10 entry-point snapshots retain original published92 bytes',
        len(publication['files']) == 10 and all(sha(ROOT / f['snapshot']) == f['sha256']
        and (ROOT / f['snapshot']).read_bytes() == subprocess.check_output(['git', 'show', REF + ':' + f['file']], cwd=ROOT) for f in publication['files']))
    audit = read(E.parent / 'dependency-hardening/dependency-audit.json')
    check('Unchanged exact lock explicitly reuses original90 audit:86 findings,20 high and open security gate',
        audit['package_lock_sha256'] == sha(APP / 'package-lock.json')
        and (APP / 'package-lock.json').read_bytes() == subprocess.check_output(['git', 'show', REF + ':app/package-lock.json'], cwd=ROOT)
        and audit['vulnerabilities']['total'] == 86 and audit['vulnerabilities']['high'] == 20
        and audit['vulnerabilities']['critical'] == 0 and not audit['production_security_gate_passed'])
    attempts_valid = True
    for label in ('02-web-cookie-observation', '03-run-state-observation', '04-historical-result-withdrawal'):
        base = E / 'attempts' / label
        attempt = read(base / 'attempt.json')
        original = read(base / 'result.json')
        manifest = read(base / 'source-manifest.json')
        with zipfile.ZipFile(base / 'sources.zip') as archive:
            attempts_valid = attempts_valid and not attempt['passed'] and not original['passed'] and attempt['diagnostic_only']
            attempts_valid = attempts_valid and sha(base / 'sources.zip') == attempt['source_zip_sha256']
            attempts_valid = attempts_valid and digest(attempt['startup_manifest'], archive.read) == attempt['complete_startup_source_sha256']
            attempts_valid = attempts_valid and digest(manifest['files'], archive.read) == manifest['sha256'] == attempt['capture_source_sha256']
            attempts_valid = attempts_valid and attempt['owned_browser_closed'] and attempt['owned_review_server_stopped'] and attempt['isolated_database_cleanup_passed']
    native = E / 'attempts/01-shared-role-cast'
    attempt = read(native / 'attempt.json')
    with zipfile.ZipFile(native / 'backend-sources.zip') as archive:
        h = hashlib.sha256()
        for name in attempt['source_files']:
            h.update(name.encode() + b'\0' + archive.read(name) + b'\0')
        attempts_valid = attempts_valid and h.hexdigest() == attempt['backend_source_sha256']
        attempts_valid = attempts_valid and sha(native / 'backend-sources.zip') == attempt['source_zip_sha256'] and not attempt['passed'] and attempt['exit_code'] == 1
    check('One original native fixture failure and three original browser observation failures retain exact archived sources and failed status', attempts_valid)
    diagnostic_valid = True
    for label in ('05-development-observation', '06-source-manifest-observation', '07-publication-whitespace'):
        attempt = read(E / 'attempts' / label / 'attempt.json')
        diagnostic_valid = diagnostic_valid and not attempt['passed'] and attempt['diagnostic_only']
        diagnostic_valid = diagnostic_valid and all(sha(ROOT / f['snapshot']) == f['sha256'] for f in attempt['files'])
    check('Limited development/assembly/publication observation diagnostics retain exact failed reports and scripts without claiming full historical sources', diagnostic_valid)
    location = read(E / 'mobile-record-location.json')
    check('Mobile verification is a fresh current-source run; original misplaced report relocation is disclosed',
        location['passed'] and location['original_argument'] == '--help' and location['source_unchanged'] and location['record'] == 'mobile-build.json')
    with socket.socket() as probe:
        probe.settimeout(1)
        released = probe.connect_ex(('127.0.0.1', 5189)) != 0
    check('Owned browser port released and private synthetic fixture removed', released and not (APP / '.local/business-browser-fixture.json').exists())
    gaps = (ROOT / 'docs/delivery/03-goal-gap-assessment.md').read_text(encoding='utf-8-sig')
    check('All28 stable gaps and complete four-client/120GB/2TB scope remain explicit',
        set(re.findall(r'\| (GAP-\d{2}) \|', gaps)) == {f'GAP-{i:02}' for i in range(1, 29)}
        and all(t in gaps for t in ('120GB', '2TB', 'Android', '微信', '抖音')))
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'checks': checks, 'backend_source_sha256': backend_sha, 'browser_source_sha256': full_sha,
        'mobile_source_sha256': mobile['source_sha256'], 'verification_script_sha256': sha(Path(__file__)),
        'full_goal_complete': False, 'reused_audit_group': 'dependency-hardening', 'audit_reexecuted': False,
        'scope': 'Fresh native checks and authenticated synthetic browser pixels; JVM cache and old-lock audit scope explicit. No real cloud, model, TG, device, capacity or production acceptance.'}
    (E / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
