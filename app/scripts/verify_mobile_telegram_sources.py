"""Verify current mobile-telegram implementation, new artifacts and exact retained unchanged proofs."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import xml.etree.ElementTree as ET

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
EVIDENCE = ROOT / 'docs/quality/evidence/mobile-telegram'
BROWSER = EVIDENCE.parent / 'mobile-telegram-browser'
PREVIOUS = EVIDENCE.parent / 'deliverable-creation'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def module(filename):
    spec = importlib.util.spec_from_file_location('task_proof_' + filename.replace('.', '_'), APP / 'scripts' / filename)
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

    backend = module('verify_backend_batches.py').digest()
    regression = read(EVIDENCE.parent / 'status-bridge/result.json')
    check('Complete unchanged Go/SQL/contract source retains the original eight-batch 154-test/984-record proof; not rerun',
          regression['passed'] and regression['source_sha256'] == backend and regression['source_unchanged_during_tests']
          and regression['all_discovered_tests_executed'] and regression['discovered_top_level_tests'] == 154
          and len(regression['checks']) == 984 and all(item['Action'] == 'pass' for item in regression['checks']))
    runtime = read(PREVIOUS / 'runtime-build.json')
    check('Complete unchanged runtime retains thirty-nine controlled tests; no real model execution',
          runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime.py').digest()[0]
          and runtime['tests']['pass'] == 39 and runtime['tests']['fail'] == runtime['tests']['skipped'] == 0)
    web = read(EVIDENCE.parent / 'status-bridge/web-build.json')
    web_sources = sorted(set(['scripts/verify_web.py', 'package.json', 'package-lock.json'] + [
        p.relative_to(APP).as_posix() for directory in ('web', 'packages/design', 'packages/contracts')
        for p in (APP / directory).rglob('*') if p.is_file()
        and not any(part in ('node_modules', 'dist', '.taro', '.swc') for part in p.relative_to(APP).parts)
        and p.suffix in ('.ts', '.tsx', '.css', '.json', '.html')]))
    check('Complete unchanged Web sources retain eighty-one tests and build; not rerun',
          web['passed'] and web_sources == web['sources'] and web['source_sha256'] == digest(web_sources)
          and web['tests']['numPassedTests'] == 81 and web['tests']['numFailedTests'] == web['tests']['numPendingTests'] == 0)
    mobile = read(EVIDENCE / 'mobile-build.json')
    mobile_digest = module('verify_mobile.py').digest()[0]
    check('All167 mobile tests, types and both fresh mini-program builds pass at the exact current source',
          mobile['passed'] and mobile['source_unchanged_during_run'] and mobile['source_sha256'] == mobile_digest
          and mobile['tests']['tests'] == mobile['tests']['pass'] == 167
          and mobile['tests']['fail'] == mobile['tests']['skipped'] == mobile['tests']['cancelled'] == 0
          and all(c['exit_code'] == 0 and not c['timed_out'] for c in mobile['commands']))
    check('Both current mini-program builds keep the same sixteen registered pages',
          all(len(read(APP / 'mobile/dist' / name / 'app.json')['pages']) == 16 for name in ('weapp', 'tt')))
    browser, manifest = read(BROWSER / 'result.json'), read(BROWSER / 'source-manifest.json')
    browser_source = module('mobile_telegram_sources.py')
    check('Nine actual UI/API flows bind the complete current backend, client and checker sources',
          browser['passed'] and browser['source_unchanged'] and len(browser['checks']) == 9
          and manifest['files'] == browser_source.sources() and browser['source_sha256'] == manifest['sha256'] == browser_source.digest()
          and not browser['errors'] and not browser['server_errors'])
    check('Actual identity, channel configuration and complete history layouts pass at320/390/768 CSS pixels',
          len(browser['layouts']) == 9 and all(item['passed'] for item in browser['layouts'])
          and {(item['page'], item['width']) for item in browser['layouts']}
          == {(page, width) for page in ('telegram-own-binding', 'telegram-channel-config', 'telegram-channel-history') for width in (320, 390, 768)})
    check('Native fixture, H5 build and browser used unchanged sources and cleaned all owned resources',
          browser['build_and_browser_sources_match'] and browser['source_unchanged_during_fixture_and_build']
          and browser['native_fixture_exit_code'] == 0 and browser['isolated_database_cleanup_passed']
          and browser['owned_browser_closed'] and browser['browser_contexts_created'] == browser['browser_contexts_closed'] == 2
          and browser['owned_review_server_stopped'] and not browser['cleanup_errors']
          and not (APP / '.local/business-browser-fixture.json').exists())
    with socket.socket() as probe:
        probe.settimeout(1)
        check('Owned temporary review port5189 is released', probe.connect_ex(('127.0.0.1', 5189)) != 0)
    android, inspection = read(EVIDENCE / 'build.json'), read(EVIDENCE / 'apk-inspection.json')
    apk = APP / android['artifacts']['apk']['path']
    bundle_sources = [name.replace('\\', '/') for name in read(APP / '.local/android/index.android.map')['sources']]
    check('Fresh frozen Android APK binds complete current portable/native source and actual artifact bytes',
          android['passed'] and android['source_unchanged_during_build']
          and android['source_sha256'] == digest(android['sources']) == module('build_android.py').sources()[0]
          and apk.exists() and apk.stat().st_size == android['artifacts']['apk']['bytes']
          and sha(apk) == android['artifacts']['apk']['sha256']
          and all(item['passed'] and item['exit_code'] == 0 for item in android['phases'])
          and all(any(name.endswith('/mobile/src/' + suffix) for name in bundle_sources)
                  for suffix in ('pages/telegram/access.tsx', 'pages/telegram/channels.tsx', 'platform/telegram-input.ts', 'platform/telegram-clipboard.rn.ts', 'platform/command.tsx')))
    check('All17 APK inspections pass with no device, emulator, virtualization or production-release claim',
          inspection['passed'] and inspection['apk_sha256'] == android['artifacts']['apk']['sha256']
          and len(inspection['checks']) == 17 and all(item['passed'] for item in inspection['checks'])
          and not android['emulator_started'] and not android['virtualization_used'] and not android['device_executed'] and not android['production_release'])
    retained = read(EVIDENCE / 'android-test-scope.json')
    suites = [ET.parse(EVIDENCE / 'android-unit-results' / name).getroot() for name in retained['xml_sha256']]
    cached = 'UP-TO-DATE' in retained['task'] or 'FROM-CACHE' in retained['task']
    check('Native test scope matches the frozen real Gradle log and all four XML results',
          retained['passed'] and retained['tests'] == 25 and len(retained['xml_sha256']) == 4
          and retained['native_log_sha256'] == sha(EVIDENCE / 'native-build.log')
          and retained['task'] in (EVIDENCE / 'native-build.log').read_text(encoding='utf-8').splitlines()
          and retained['gradle_cached'] == cached and retained['new_test_execution'] == (not cached)
          and sum(int(s.attrib['tests']) for s in suites) == 25
          and all(int(s.attrib[k]) == 0 for s in suites for k in ('failures', 'errors', 'skipped'))
          and all(sha(EVIDENCE / 'android-unit-results' / name) == value for name, value in retained['xml_sha256'].items()))
    java = read(EVIDENCE.parent / 'android-upload/jvm-source-proof.json')
    check('Unchanged pure Java transfer boundaries retain the exact original25-test source proof; not rerun',
          java['passed'] and java['tests'] == 25 and java['source_sha256'] == digest(java['sources']))
    linux = read(EVIDENCE.parent / 'status-bridge/linux-build.json')
    check('Unchanged backend retains its exact original Linux artifact; not rebuilt or deployed',
          linux['passed'] and linux['source_sha256'] == backend
          and sha(APP / '.local/status-bridge-platform-linux-amd64') == linux['artifact_sha256']
          and not linux['artifact_executed'] and not linux['deployed'])
    dev, base = read(EVIDENCE / 'development-project-resources.json'), read(EVIDENCE / 'development-runtime.json')
    check('Fresh owned daily API/worker and all69 migration hashes pass read-only inspection',
          dev['passed'] and base['passed'] and base['database']['migration_count'] == 69
          and base['database']['migration_manifest_matches_source'] and all(p['passed'] for p in base['probes'])
          and all(dev['ownership'].values())
          and dev['verification_script_sha256'] == sha(APP / 'scripts/verify_mobile_telegram_development.py'))
    check('Real integrations remain disabled, all eight Skills remain draft and no isolated test database survives',
          dev['integrations_disabled'] and dev['all_eight_skills_draft']
          and dev['database']['disposable_test_databases'] == dev['database']['active_profiles'] == dev['database']['enabled_telegram_channels'] == 0)
    check('Embedded/documented OpenAPI remain byte-identical',
          (APP / 'backend/internal/contract/openapi.json').read_bytes() == (ROOT / 'docs/development/openapi.json').read_bytes())
    media = read(EVIDENCE / 'media.json')
    frames, provenance = read(EVIDENCE / 'recording-frames.json'), read(EVIDENCE / 'recording-provenance.json')
    check('The published original frame manifest binds actual capture timestamps, hashes and credential-exclusion scope',
          provenance['passed'] and provenance['manifest_sha256'] == sha(EVIDENCE / 'recording-frames.json')
          == media['frame_manifest_sha256'] == browser['recordings'][0]['manifest_sha256']
          and frames['frame_count'] == len(frames['frames']) == provenance['frames'] == media['source_frames']
          and frames['credentials_excluded'] and frames['synthetic_data'] and frames['real_browser_pixels'] and not frames['audio'])
    check('Frozen actual native-test scope and capture provenance bind the reproducible evidence freezer',
          retained['freezer_sha256'] == provenance['freezer_sha256'] == sha(APP / 'scripts/freeze_mobile_telegram_evidence.py'))
    check('New actual Telegram access recording, encoder and both published outputs match recorded digests',
          media['passed'] and media['browser_result_sha256'] == sha(BROWSER / 'result.json')
          and media['encoder_sha256'] == sha(APP / 'scripts/encode_mobile_telegram_media.py')
          and media['helper_sha256'] == sha(APP / 'scripts/encode_repository_showcase.py')
          and len(media['artifacts']) == 2 and all(sha(ROOT / item['file']) == item['sha256'] for item in media['artifacts']))
    visual = read(EVIDENCE / 'visual-review.json')
    check('New screenshots and sampled GIF review bind the inspected actual outputs',
          visual['reviewed'] and len(browser['screenshots']) == 3
          and all(sha(ROOT / item['file']) == item['sha256'] for item in browser['screenshots'] + visual['artifacts']))
    historical = read(EVIDENCE / 'historical-status-capture.json')
    check('Historical status capture retains all787 exact Git blobs at increment81 without rewriting its evidence',
          historical['passed'] and historical['sources'] == 787 and historical['all_sources_from_git']
          and historical['commit'] == '9c710a0f4c785a02539441ec1611bc6678f0050d'
          and historical['source_sha256'] == read(EVIDENCE.parent / 'status-bridge-browser/source-manifest.json')['sha256']
          and historical['manifest_sha256'] == sha(EVIDENCE.parent / 'status-bridge-browser/source-manifest.json')
          and historical['verification_script_sha256'] == sha(APP / 'scripts/verify_mobile_telegram_history.py')
          and not historical['current_source_claim'] and not historical['historical_evidence_modified'])
    audit = read(EVIDENCE.parent / 'android-native/dependency-audit.json')
    check('Dependency lock remains unchanged with its unresolved production security gate',
          sha(APP / 'package-lock.json') == audit['package_lock_sha256'] and not audit['production_security_gate_passed'])
    result = {'passed': all(item['passed'] for item in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
              'checks': checks, 'backend_source_sha256': backend, 'mobile_source_sha256': mobile_digest,
              'browser_source_sha256': browser_source.digest(), 'verification_script_sha256': sha(Path(__file__)),
              'functional_scope': 'New mobile Telegram identity and channel configuration workflows/builds/browser/APK; unchanged backend/runtime/Web proofs reused at exact source digests.',
              'full_goal_complete': False, 'production_deployed': False, 'real_cloud_verified': False,
              'real_model_verified': False, 'real_device_verified': False}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'checks': len(checks), 'failed': [item['name'] for item in checks if not item['passed']]}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
