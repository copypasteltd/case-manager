"""Recompute current token budget evidence, actual artifacts, retained history and owned cleanup."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess
import xml.etree.ElementTree as ET
from PIL import Image

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
EVIDENCE = ROOT / 'docs/quality/evidence/token-budgets'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def module(filename):
    spec = importlib.util.spec_from_file_location('token_budget_proof_' + filename.replace('.', '_'), APP / 'scripts' / filename)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def main():
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    backend_digest = module('verify_backend_batches.py').digest()
    backend = read(EVIDENCE / 'result.json')
    check('All 158 discovered backend tests and 1029 recorded results pass in eight source-bound batches',
        backend['passed'] and backend['source_sha256'] == backend_digest and backend['source_unchanged_during_tests']
        and backend['all_discovered_tests_executed'] and backend['discovered_top_level_tests'] == 158
        and backend['batch_count'] == 8 and len(backend['checks']) == 1029 and all(item['Action'] == 'pass' for item in backend['checks']))
    concerns = [item for item in backend['checks'] if item['Test'].startswith('TestAgentTokenBudgets/')]
    check('Fourteen native token budget scenarios prove original commands, authority, atomic rollback, exact retry, version stopping and immutable history',
        len(concerns) == 14 and all(item['Action'] == 'pass' for item in backend['checks']))
    check('Original manual collaboration and nullable PATCH semantics pass with the new migration',
        any(item['Test'] == 'TestProjectCollaborationIntegration' and item['Action'] == 'pass' for item in backend['checks'])
        and any(item['Test'].endswith('/normal_public_patches_distinguish_omission_from_explicit_null') for item in backend['checks']))
    migrations = sorted((APP / 'backend/migrations').glob('*.sql'))
    unchanged = all(subprocess.check_output(['git', 'show', '83bf46819fca3e670f5d3101b0234fbe46a2ad23:' + file.relative_to(ROOT).as_posix()], cwd=ROOT) == file.read_bytes() for file in migrations[:-1])
    check('Migration72 is additive and all 71 previously applied migration files retain exact original bytes',
        len(migrations) == 72 and migrations[-1].name == '0072_agent_token_budgets.sql' and unchanged)
    contract = ROOT / 'docs/development/openapi.json'
    api = read(contract)
    check('Both OpenAPI copies match with 275 paths, 342 operations and 598 schemas',
        contract.read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes()
        and len(api['paths']) == 275 and sum(key in ('get','post','put','patch','delete','head','options','trace') for path in api['paths'].values() for key in path) == 342
        and len(api['components']['schemas']) == 598)
    runtime = read(EVIDENCE / 'runtime-build.json')
    check('Fresh runtime build and 47 tests bind complete current runtime and Skill bytes',
        runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime.py').digest()[0]
        and runtime['tests']['pass'] == runtime['tests']['tests'] == 47 and runtime['tests']['fail'] == runtime['tests']['skipped'] == runtime['tests']['cancelled'] == 0)
    web = read(EVIDENCE / 'web-build.json')
    check('Fresh Web build and 98 tests match every declared source byte',
        web['passed'] and web['source_unchanged_during_run'] and web['source_sha256'] == digest(web['sources'])
        and web['tests']['numPassedTests'] == 98 and web['tests']['numFailedTests'] == 0)
    mobile = read(EVIDENCE / 'mobile-build.json')
    check('Fresh mobile typecheck, both mini-program builds and 187 tests pass across both actual TAP suites',
        mobile['passed'] and mobile['source_unchanged_during_run'] and mobile['source_sha256'] == module('verify_mobile.py').digest()[0]
        and mobile['tests']['pass'] == mobile['tests']['tests'] == 187 and mobile['tests']['suites'] == 2
        and mobile['tests']['fail'] == mobile['tests']['skipped'] == mobile['tests']['cancelled'] == 0
        and len(mobile['commands']) == 4 and all(command['exit_code'] == 0 for command in mobile['commands']))
    pages = [read(APP / ('mobile/dist/' + target + '/app.json'))['pages'] for target in ('weapp', 'tt')]
    check('Both mini-program outputs register seventeen pages including the token budget page', all(len(items) == 17 and 'pages/budget/index' in items for items in pages) and pages[0] == pages[1])
    android = read(EVIDENCE / 'build.json')
    check('The current Android development APK matches the complete native and portable source manifest',
        android['passed'] and android['source_unchanged_during_build'] and android['source_sha256'] == module('build_android.py').sources()[0]
        and sha(APP / android['artifacts']['apk']['path']) == android['artifacts']['apk']['sha256'])
    inspection = read(EVIDENCE / 'apk-inspection.json')
    check('Seventeen package inspections bind the actual new APK',
        inspection['passed'] and len(inspection['checks']) == 17 and all(item['passed'] for item in inspection['checks']))
    native = read(EVIDENCE / 'android-test-scope.json')
    xml = sorted((EVIDENCE / 'android-unit-results').glob('TEST-*.xml'))
    suites = [ET.parse(file).getroot() for file in xml]
    check('Twenty-five JVM results retain exact XML/log bytes and the actual Gradle execution/cache marker',
        native['passed'] and native['tests'] == 25 and native['native_log_sha256'] == sha(EVIDENCE / 'native-build.log')
        and native['freezer_sha256'] == sha(APP / 'scripts/freeze_token_budget_evidence.py')
        and len(xml) == 4 and sum(int(suite.attrib['tests']) for suite in suites) == 25
        and all(int(suite.attrib[key]) == 0 for suite in suites for key in ('errors','failures','skipped'))
        and all(native['xml_sha256'][file.name] == sha(file) for file in xml)
        and native['new_test_execution'] != native['gradle_cached'] and not native['device_executed'])
    linux = read(EVIDENCE / 'linux-build.json')
    check('Linux compilation matches current backend without execution or deployment',
        linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_digest and not linux['artifact_executed'] and not linux['deployed'])
    full_digest = module('token_budget_sources.py').digest()
    for client in ('web', 'mobile'):
        folder = EVIDENCE.parent / ('token-budgets-' + client)
        browser, manifest, capture = read(folder / 'result.json'), read(folder / 'source-manifest.json'), read(folder / 'capture.json')
        check(client + ': six actual MFA/API/form/send/retry/cancel flows and three responsive sizes pass with owned cleanup',
            browser['passed'] and len(browser['checks']) == 6 and len(capture['layouts']) == 3
            and browser['source_sha256'] == browser['source_after_sha256'] == manifest['sha256'] == digest(manifest['files'])
            and browser['source_unchanged'] and browser['build_source_unchanged'] and browser['build_source_sha256'] == full_digest
            and browser['native_fixture_exit_code'] == 0 and browser['isolated_database_cleanup_passed']
            and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['browser_contexts_closed'] == 1
            and not browser['console_errors'] and not browser['server_errors'])
    media = read(EVIDENCE / 'media.json')
    check('Two actual frame manifests and four MP4/GIF artifacts match the encoder, capture and output digests',
        media['passed'] and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_token_budget_media.py')
        and len(media['recordings']) == 2 and all(recording['build_source_sha256'] == full_digest
            and recording['capture_manifest_sha256'] == sha(ROOT / recording['capture_manifest'])
            and recording['capture_frame_count'] >= 5 and len(recording['artifacts']) == 2
            and all(sha(ROOT / artifact['file']) == artifact['sha256'] and (ROOT / artifact['file']).stat().st_size == artifact['bytes'] for artifact in recording['artifacts']) for recording in media['recordings']))
    gif_valid = True
    for recording in media['recordings']:
        file = next(ROOT / item['file'] for item in recording['artifacts'] if item['file'].endswith('.gif'))
        with Image.open(file) as image:
            durations = []
            for index in range(image.n_frames):
                image.seek(index); durations.append(image.info.get('duration', 0))
            gif_valid = gif_valid and image.n_frames > 1 and abs(sum(durations) * recording['gif_speed_multiplier'] - recording['capture_duration_ms']) < 500
    check('Both GIFs retain actual animated frames with bounded capture-derived timing', gif_valid)
    visual = read(EVIDENCE / 'visual-review.json')
    check('Manual pixel review binds four viewport screenshots and both actual GIFs',
        visual['reviewed'] and len(visual['files']) == 6 and all(sha(ROOT / item['file']) == item['sha256'] for item in visual['files']))
    manifests = [read(file) for file in sorted((APP / 'skills').glob('*/manifest.json'))]
    skill = next(value for value in manifests if value['skill_id'] == 'project-update')
    check('All eight Skills remain draft; project-update1.6.0 has exactly sixteen tools and fourteen completion tools',
        len(manifests) == 8 and all(value['state'] == 'draft' for value in manifests) and skill['version'] == '1.6.0'
        and len(skill['allowed_tools']) == 16 and len(skill['completion_tools']) == 14)
    development = read(EVIDENCE / 'development-project-resources.json')
    update = read(EVIDENCE / 'development-update.json')
    check('Only the verified owned development API was updated to the reviewed complete backend source',
        update['passed'] and update['source_sha256'] == backend_digest and update['migration_exit_code'] == 0
        and update['api_ready'] and not update['postgres_restarted'] and not update['notification_worker_restarted']
        and not update['external_integration_started'] and not update['production_deployment'])
    check('Fresh read-only native probes confirm migration72, owned services, disabled integrations and zero test databases',
        development['passed'] and development['migration_count'] == 72 and development['integrations_disabled']
        and development['all_eight_skills_draft'] and development['database']['disposable_test_databases'] == 0
        and development['database']['active_profiles'] == development['database']['enabled_telegram_channels'] == 0
        and all(development['ownership'].values()))
    history = read(EVIDENCE / 'historical-85.json')
    actual_history = module('record_token_budget_history.py').reconstruct()
    check('Published increment85 complete startup and both client captures reconstruct from Git without rewriting historical evidence',
        history == actual_history and history['complete_startup_source']['reconstructed']
        and all(c['complete_capture_manifest_reconstructed'] for c in history['clients'].values()))
    with socket.socket() as probe:
        probe.settimeout(1)
        cleanup = probe.connect_ex(('127.0.0.1', 5189)) != 0
    check('Owned browser port is released and private fixture file is removed', cleanup and not (APP / '.local/business-browser-fixture.json').exists())
    check('Launch gates remain explicit and all twenty-eight stable gap IDs remain in the register',
        all(('GAP-' + str(index).zfill(2)) in (ROOT / 'docs/delivery/03-goal-gap-assessment.md').read_text(encoding='utf-8') for index in range(1,29))
        and all(word in (ROOT / 'docs/development/86-secretary-token-budgets.md').read_text(encoding='utf-8') for word in ('120GB','2TB','Telegram','draft','Android')))
    check('Actual capture synchronization and test registration failures remain documented with cleanup records',
        all((EVIDENCE / name).exists() for name in ('attempts/web-01/diagnosis.json',
            'attempts/web-01/result.json','attempts/web-test-01/web-build.json','attempts/web-test-01/diagnosis.json')))
    report = {'passed': all(item['passed'] for item in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'checks': checks, 'backend_source_sha256': backend_digest, 'browser_source_sha256': full_digest,
        'mobile_source_sha256': mobile['source_sha256'], 'full_goal_complete': False,
        'verification_script_sha256': sha(Path(__file__)), 'scope': 'Source-bound native local checks and synthetic browser recordings; cached JVM scope explicit; no real model/device/cloud/production acceptance.'}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [item['name'] for item in checks if not item['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
