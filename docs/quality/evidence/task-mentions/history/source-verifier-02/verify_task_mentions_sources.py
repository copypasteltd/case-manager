"""Verify current task-discussion sources, actual local checks and retained history."""
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
E = ROOT / 'docs/quality/evidence/task-mentions'
PUBLISHED = 'e0b715929418644c61dd1367608383f6d64d1137'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(names, content=None):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (content(name) if content else (APP / name).read_bytes()) + b'\0')
    return value.hexdigest()


def module(filename):
    spec = importlib.util.spec_from_file_location('mentions_' + filename.replace('.', '_'), APP / 'scripts' / filename)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def git_bytes(name):
    return subprocess.check_output(['git', 'show', PUBLISHED + ':' + name], cwd=ROOT)


def main():
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    backend = read(E.parent / 'task-mentions-backend/result.json')
    backend_sha = module('verify_backend_batches.py').digest()
    check('Fresh complete native backend171 top-level/1133 records in9 batches matches current sources',
          backend['passed'] and backend['source_sha256'] == backend_sha
          and backend['source_unchanged_during_tests'] and backend['all_discovered_tests_executed']
          and backend['native_database'] and backend['discovered_top_level_tests'] == 171
          and len(backend['checks']) == 1133 and backend['batch_count'] == 9
          and all(c['Action'] == 'pass' for c in backend['checks']))
    linux = read(E / 'linux-build.json')
    check('Fresh Linux cross-compile matches current backend; no execution or deployment',
          linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_sha
          and not linux['artifact_executed'] and not linux['deployed'])

    migrations = sorted((APP / 'backend/migrations').glob('*.sql'))
    check('Migration76 is additive; earlier75 retain published94 bytes', len(migrations) == 76
          and migrations[-1].name == '0076_task_comment_mentions.sql'
          and all(p.read_bytes() == git_bytes(p.relative_to(ROOT).as_posix()) for p in migrations[:-1]))
    spec = read(ROOT / 'docs/development/openapi.json')
    verbs = {'get', 'post', 'patch', 'put', 'delete', 'head', 'options', 'trace'}
    check('Both exact contracts and generated types expose286 paths/355 operations/625 schemas',
          (ROOT / 'docs/development/openapi.json').read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes()
          and len(spec['paths']) == 286 and sum(k in verbs for v in spec['paths'].values() for k in v) == 355
          and len(spec['components']['schemas']) == 625
          and '/v1/comments/{id}' in spec['paths'] and '/v1/tasks/{id}/mention-candidates' in spec['paths']
          and all(t in (APP / 'packages/contracts/src/api.d.ts').read_text(encoding='utf-8')
                  for t in ('mentioned_user_ids', 'mentions_filtered', 'mention-candidates')))

    clients = read(E / 'client-checks.json')
    fresh = clients['passed'] and clients['source_unchanged'] and len(clients['checks']) == 8
    fresh = fresh and clients['source_sha256'] == clients['source_after_sha256'] == digest(clients['source_files'])
    for item in clients['checks']:
        raw = APP / '.local' / ('task-mentions-' + item['name'] + '.log')
        fresh = fresh and item['passed'] and item['exit_code'] == 0 and sha(raw) == item['raw_log_sha256']
    check('Eight fresh type/test/build commands bind complete current inputs and original private logs', fresh)
    tests = {c['name']: c['test_count'] for c in clients['checks'] if c['name'].endswith('-tests')}
    check('Actual fresh test counts are135 Web/238 mobile/51 runtime',
          tests == {'web-tests': 135, 'mobile-tests': 238, 'runtime-tests': 51})
    mobile, web, runtime = [read(E / n) for n in ('mobile-build.json', 'web-build.json', 'runtime-build.json')]
    mobile_sha = module('verify_mobile.py').digest()[0]
    check('Three publication summaries derive from this exact run without rerunning tests',
          all(r['derived_from']['sha256'] == sha(E / 'client-checks.json')
              and r['derived_from']['summary_adapter_sha256'] == sha(APP / 'scripts/freeze_task_mentions_clients.py')
              and not r['derived_from']['adapter_reran_tests'] for r in (mobile, web, runtime))
          and mobile['passed'] and mobile['source_sha256'] == mobile_sha and mobile['tests']['pass'] == 238
          and web['passed'] and web['source_sha256'] == digest(web['sources']) and web['tests']['numPassedTests'] == 135
          and runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime.py').digest()[0]
          and runtime['tests']['pass'] == 51 and runtime['skills_check']['passed']
          and all(r['tests'][k] == 0 for r in (mobile, runtime) for k in ('fail', 'skipped', 'cancelled')))
    pages = [read(APP / ('mobile/dist/' + target + '/app.json'))['pages'] for target in ('weapp', 'tt')]
    check('WeChat/Douyin compile the same20 pages; task discussion stays inside existing detail page',
          pages[0] == pages[1] and len(pages[0]) == 20 and 'pages/detail/index' in pages[0]
          and 'pages/files/index' in pages[0] and all(mobile['targets'][t]['pages'] == pages[0] for t in ('weapp', 'tt')))

    native, inspection, unit = [read(E / n) for n in ('build.json', 'apk-inspection.json', 'android-test-scope.json')]
    check('Current native development APK and18 package checks match complete current sources',
          native['passed'] and native['source_unchanged_during_build']
          and native['source_sha256'] == module('build_android.py').sources()[0] == digest(native['sources'])
          and sha(APP / native['artifacts']['apk']['path']) == native['artifacts']['apk']['sha256'] == inspection['apk_sha256']
          and inspection['passed'] and len(inspection['checks']) == 18 and all(c['passed'] for c in inspection['checks'])
          and not native['device_executed'] and not native['production_release'] and not native['virtualization_used'])
    xml = sorted((E / 'android-unit-results').glob('TEST-*.xml'))
    suites = [ET.parse(p).getroot() for p in xml]
    check('26 JVM results explicitly retain Gradle UP-TO-DATE cache scope; no fresh JVM/device execution',
          unit['passed'] and unit['gradle_cached'] and not unit['new_test_execution'] and not unit['device_executed']
          and 'UP-TO-DATE' in unit['task'] and len(xml) == 4 and sum(int(s.attrib['tests']) for s in suites) == unit['tests'] == 26
          and all(int(s.attrib[k]) == 0 for s in suites for k in ('failures', 'errors', 'skipped'))
          and all(sha(p) == unit['xml_sha256'][p.name] for p in xml)
          and sha(E / 'native-build.log') == unit['native_log_sha256']
          and sha(APP / 'scripts/freeze_task_mentions_android.py') == unit['freezer_sha256'])

    browser_module = module('task_mentions_sources.py')
    browser_sha = browser_module.digest()
    browsers = []
    for client in ('web', 'mobile'):
        directory = E.parent / ('task-mentions-' + client)
        result, manifest = read(directory / 'result.json'), read(directory / 'source-manifest.json')
        browsers.append(result)
        check(client + ': eight actual authenticated API/UI flows bind955 complete startup/build/capture inputs',
              result['passed'] and len(result['checks']) == 8 and result['source_count'] == 955
              and manifest['files'] == browser_module.sources()
              and result['source_sha256'] == result['source_after_sha256'] == manifest['sha256'] == digest(manifest['files']) == browser_sha
              and result['source_unchanged'] and result['build_source_unchanged']
              and result['build_source_sha256'] == result['build_source_after_sha256'] == browser_sha
              and result['real_native_api'] and not any(result[k] for k in ('real_cloud', 'real_model', 'real_device', 'real_telegram_network'))
              and not result['errors'] and not result['server_errors'] and len(result['controlled_response_faults']) == 2)
        check(client + ': two identities, actual screenshots/layouts and owned resource cleanup',
              result['owned_browser_closed'] and result['owned_review_server_stopped']
              and result['native_fixture_exit_code'] == 0 and result['isolated_database_cleanup_passed']
              and result['browser_contexts_created'] == result['browser_contexts_closed'] == 2
              and len(result['layouts']) == len(result['screenshots']) == 4
              and all(v['document'] <= v['width'] + 1 for v in result['layouts'])
              and all((ROOT / 'docs/media/screenshots' / name).is_file() for name in result['screenshots']))

    checker = (APP / 'scripts/check_task_mentions.mjs').read_text(encoding='utf-8')
    check('Passing browser flows include target-body viewport bounds and explicitly labelled committed-response faults',
          'focusedBounds.y + focusedBounds.height <= 900' in checker
          and 'checker does not scroll it' in checker and 'incomplete committed201' in checker
          and 'exact-discussion' in checker and all(any('original idempotency key' in c for c in r['checks']) for r in browsers))
    media = read(E / 'media.json')
    valid = media['passed'] and len(media['recordings']) == 2 and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_task_mentions_media.py')
    for recording in media['recordings']:
        capture = read(ROOT / recording['capture_manifest'])
        browser = browsers[0 if recording['client'] == 'web' else 1]
        original = browser['recordings'][0]
        valid = valid and recording['source_sha256'] == recording['build_source_sha256'] == browser_sha
        valid = valid and recording['capture_manifest_sha256'] == sha(ROOT / recording['capture_manifest']) == original['manifest_sha256']
        valid = valid and capture['frame_count'] == recording['capture_frame_count'] > 5
        valid = valid and capture['real_browser_pixels'] and capture['credentials_excluded'] and capture['synthetic_data']
        valid = valid and recording['gif_speed_multiplier'] == 1.0 and len(recording['artifacts']) == 2
        valid = valid and abs(float(recording['probe']['format']['duration']) * 1000 - recording['capture_duration_ms']) < 800
        for artifact in recording['artifacts']:
            path = ROOT / artifact['file']
            valid = valid and sha(path) == artifact['sha256'] and path.stat().st_size == artifact['bytes'] < 15_000_000
            if path.suffix == '.gif':
                with Image.open(path) as pic:
                    duration = 0
                    for frame in range(pic.n_frames):
                        pic.seek(frame)
                        duration += pic.info.get('duration', 0)
                    valid = valid and pic.n_frames > 1 and abs(duration - recording['capture_duration_ms']) < 800
    check('Two real-pixel captures bind exact frame manifests/timing and four original-speed MP4/GIF artifacts', valid)
    visual = read(E / 'visual-review.json')
    check('Direct image review binds eight PNGs and first/middle/last samples of both GIFs',
          visual['passed'] and len(visual['files']) == 10
          and all(sha(ROOT / item['file']) == item['sha256'] for item in visual['files']))
    counts = [len(list((ROOT / 'docs/media' / folder).glob('*.' + ext)))
              for folder, ext in [('screenshots', 'png'), ('videos', 'mp4'), ('gifs', 'gif')]]
    check('Actual media catalog contains90 PNG/33 MP4/33 GIF and retains all published94 media bytes',
          counts == [90, 33, 33] and not subprocess.check_output(['git', 'diff', PUBLISHED, '--', 'docs/media/screenshots', 'docs/media/videos', 'docs/media/gifs'], cwd=ROOT))

    history = E / 'history/published-94'
    provenance = read(history / 'provenance.json')
    with zipfile.ZipFile(history / 'application-source.zip') as archive:
        old = {name: archive.read(name) for name in archive.namelist() if not name.endswith('/')}
        valid = provenance['ref'] == PUBLISHED and sha(history / 'application-source.zip') == provenance['application_archive_sha256']
        valid = valid and len(archive.namelist()) == provenance['application_entries']
        valid = valid and all(old[p.relative_to(ROOT).as_posix()] == p.read_bytes() for p in migrations[:-1])
    snapshots = [p for p in history.rglob('*') if p.is_file() and p.name not in ('application-source.zip', 'provenance.json')]
    valid = valid and all(p.read_bytes() == git_bytes(p.relative_to(history).as_posix()) for p in snapshots)
    check('Published94 complete application archive, entry snapshots and backend evidence retain exact Git bytes', valid)
    attempts = sorted((E / 'history/browser-attempts').glob('*/attempt.json'))
    valid = len(attempts) == 9
    outcomes = []
    for p in attempts:
        attempt, manifest = read(p), read(p.parent / 'source-manifest.json')
        result_path = p.parent / 'result.json'
        outcomes.append(read(result_path)['passed'] if result_path.exists() else None)
        with zipfile.ZipFile(p.parent / 'source.zip') as archive:
            valid = valid and sha(p.parent / 'source.zip') == attempt['archive_sha256']
            valid = valid and digest(manifest['files'], archive.read) == manifest['sha256'] == attempt['source_sha256']
        for item in attempt.get('media', []):
            valid = valid and sha(p.parent / 'media' / Path(item['path']).relative_to('docs/media')) == item['sha256']
    check('Nine original browser diagnostics/supersessions retain exact source ZIPs, statuses and generated media',
          valid and outcomes == [None, False, False, False, False, True, False, True, True])

    audit = read(E.parent / 'dependency-hardening/dependency-audit.json')
    check('Unchanged lock reuses original90 audit86 findings/20high; security gate remains open',
          sha(APP / 'package-lock.json') == audit['package_lock_sha256']
          and audit['vulnerabilities']['total'] == 86 and audit['vulnerabilities']['high'] == 20
          and not audit['production_security_gate_passed'])
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('All eight Skills retain published94 source bytes and draft state',
          len(skills) == 8 and all(s['state'] == 'draft' for s in skills)
          and not subprocess.check_output(['git', 'diff', PUBLISHED, '--', 'app/skills'], cwd=ROOT))
    dev, runtime_dev, update = [read(E / n) for n in ('development-project-resources.json', 'development-runtime.json', 'development-update.json')]
    check('Owned native development update and readonly76-migration/zero-test-database inspection; external integrations off',
          dev['passed'] and runtime_dev['passed'] and update['passed'] and dev['migration_count'] == 76
          and dev['database']['disposable_test_databases'] == 0 and dev['integrations_disabled'] and dev['worker_sources_match']
          and dev['backend_source_sha256'] == backend_sha and all(dev['ownership'].values())
          and dev['read_only_inspection'] and not dev['production_deployment']
          and dev['verification_script_sha256'] == sha(APP / 'scripts/verify_task_mentions_development.py'))
    with socket.socket() as probe:
        probe.settimeout(1)
        released = probe.connect_ex(('127.0.0.1', 5189)) != 0
    check('Owned review port is released and private fixture is absent',
          released and not (APP / '.local/business-browser-fixture.json').exists())
    gaps = (ROOT / 'docs/delivery/03-goal-gap-assessment.md').read_text(encoding='utf-8-sig')
    check('All28 stable gap IDs and full four-client/capacity scope remain explicit',
          set(re.findall(r'\| (GAP-\d{2}) \|', gaps)) == {f'GAP-{i:02}' for i in range(1, 29)}
          and all(t in gaps for t in ('120GB', '2TB', 'Android', '微信', '抖音', '当前20个注册页面')))
    guides = [(ROOT / p).read_text(encoding='utf-8-sig') for p in ('README.md', 'docs/README.zh-CN.md', 'docs/README.en.md', 'docs/README.md', 'docs/CHANGELOG.md', 'docs/media/README.md')]
    check('Current bilingual entry points, changelog and media navigation point to95 workflow and verification',
          all('95-task-discussion-and-mentions.md' in text and '95-task-discussion-verification.md' in text for text in guides))
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
              'checks': checks, 'backend_source_sha256': backend_sha, 'browser_source_sha256': browser_sha,
              'mobile_source_sha256': mobile_sha, 'verification_script_sha256': sha(Path(__file__)),
              'full_goal_complete': False, 'audit_reexecuted': False, 'jvm_tests_reexecuted': False,
              'scope': 'Fresh native backend and client commands, actual synthetic Web/H5 flows, current development APK and readonly local inspection. JVM cache reuse and original dependency audit remain explicit. No real cloud/model/TG/device/capacity/production acceptance.'}
    (E / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
