"""Bind fresh mobile setup evidence and explicitly reused unchanged backend/Web proofs."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/mobile-project-setup'
PREVIOUS = EVIDENCE.parent / 'deliverable-creation'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / filename)
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

    backend = module('setup_backend', 'verify_backend_batches.py').digest()
    regression = read(PREVIOUS / 'result.json')
    check('Unchanged complete backend source retains the increment74 full eight-batch proof; not newly executed',
          regression['passed'] and regression['source_sha256'] == backend
          and regression['source_unchanged_during_tests'] and regression['all_discovered_tests_executed']
          and regression['discovered_top_level_tests'] == 146 and regression['batch_count'] == 8
          and len(regression['checks']) == 919 and all(c['Action'] == 'pass' for c in regression['checks']))
    runtime = read(PREVIOUS / 'runtime-build.json')
    check('Unchanged runtime source retains all thirty-nine controlled tests; not newly executed',
          runtime['status'] == 'passed' and runtime['source_sha256'] == runtime['source_after_sha256']
          == module('setup_runtime', 'verify_agent_runtime.py').digest()[0]
          and runtime['tests']['pass'] == runtime['tests']['tests'] == 39
          and runtime['tests']['fail'] == runtime['tests']['skipped'] == runtime['tests']['cancelled'] == 0)
    manifests = [read(p) for p in sorted((APP / 'skills').glob('*/manifest.json'))]
    update_skill = next(m for m in manifests if m['skill_id'] == 'project-update')
    check('All eight Skills remain draft and project-update remains version1.4.0 with nine exact write tools',
          len(manifests) == 8 and all(m['state'] == 'draft' for m in manifests)
          and update_skill['version'] == '1.4.0' and len(update_skill['completion_tools']) == 9)
    web = read(PREVIOUS / 'web-build.json')
    web_sources = sorted(set(['scripts/verify_web.py', 'package.json', 'package-lock.json'] + [
        p.relative_to(APP).as_posix() for directory in ('web', 'packages/design', 'packages/contracts')
        for p in (APP / directory).rglob('*')
        if p.is_file() and not any(part in ('node_modules', 'dist', '.taro', '.swc') for part in p.relative_to(APP).parts)
        and p.suffix in ('.ts', '.tsx', '.css', '.json', '.html')]))
    check('Unchanged complete Web source retains eighty-one tests and production build; not newly executed',
          web['passed'] and web['source_unchanged_during_run'] and web_sources == web['sources']
          and web['source_sha256'] == digest(web_sources)
          and web['tests']['numTotalTests'] == web['tests']['numPassedTests'] == 81
          and web['tests']['numFailedTests'] == web['tests']['numPendingTests'] == 0
          and all(c['exit_code'] == 0 for c in web['commands']))
    mobile = read(EVIDENCE / 'mobile-build.json')
    check('All ninety-nine mobile tests, type checks and both mini-program builds pass at the final source',
          mobile['passed'] and mobile['source_unchanged_during_run']
          and mobile['source_sha256'] == module('setup_mobile', 'verify_mobile.py').digest()[0]
          and mobile['tests']['tests'] == mobile['tests']['pass'] == 99
          and mobile['tests']['fail'] == mobile['tests']['cancelled'] == mobile['tests']['skipped'] == 0
          and all(c['exit_code'] == 0 and not c['timed_out'] for c in mobile['commands']))
    check('Both actual mini-program app manifests retain sixteen registered pages',
          all(len(read(APP / 'mobile/dist' / target / 'app.json')['pages']) == 16 for target in ('weapp', 'tt')))
    folder = EVIDENCE.parent / 'mobile-project-setup-browser'
    browser, manifest = read(folder / 'result.json'), read(folder / 'source-manifest.json')
    current_browser = module('setup_browser_sources', 'mobile_project_setup_sources.py')
    check('All seven real mobile API flows bind the complete final backend, UI and wheel checker source',
          browser['passed'] and browser['source_unchanged'] and manifest['files'] == current_browser.sources()
          and browser['source_sha256'] == browser['source_after_sha256'] == manifest['sha256'] == current_browser.digest()
          and len(browser['checks']) == 7 and not browser['errors'] and not browser['server_errors'])
    check('Creation and membership layouts pass at 320, 390 and 768 CSS pixels',
          len(browser['layouts']) == 6 and all(v['passed'] for v in browser['layouts'])
          and {(v['page'], v['width']) for v in browser['layouts']}
          == {(page, width) for page in ('project-creation', 'project-members') for width in (320, 390, 768)})
    check('Native fixture, fresh H5 build and actual browser used identical source before and after build',
          browser['build_and_browser_sources_match'] and browser['source_unchanged_during_fixture_and_build']
          and browser['source_sha256_before_fixture_and_build'] == browser['source_sha256_after_build'] == browser['source_sha256'])
    check('All four owned contexts, browser, native fixture and temporary server are closed',
          browser['owned_browser_closed'] and browser['browser_contexts_closed'] == 4 and not browser['cleanup_errors']
          and browser['native_fixture_exit_code'] == browser['browser_exit_code'] == 0
          and browser['isolated_database_cleanup_passed'] and browser['owned_review_server_stopped'])
    android, inspection = read(EVIDENCE / 'build.json'), read(EVIDENCE / 'apk-inspection.json')
    apk = APP / android['artifacts']['apk']['path']
    check('The new frozen Android development APK binds the complete final native and portable source',
          android['passed'] and android['source_unchanged_during_build']
          and android['source_sha256'] == digest(android['sources'])
          == module('setup_android', 'build_android.py').sources()[0]
          and apk.is_file() and apk.stat().st_size == android['artifacts']['apk']['bytes']
          and hashlib.sha256(apk.read_bytes()).hexdigest() == android['artifacts']['apk']['sha256']
          and all(v['passed'] and v['exit_code'] == 0 for v in android['phases']))
    check('All seventeen APK checks pass without emulator, virtualization, device execution or production release',
          inspection['passed'] and inspection['apk_sha256'] == android['artifacts']['apk']['sha256']
          and len(inspection['checks']) == 17 and all(c['passed'] for c in inspection['checks'])
          and not android['virtualization_used'] and not android['emulator_started']
          and not android['device_executed'] and not android['production_release'])
    retained = read(EVIDENCE / 'android-test-reuse.json')
    check('Actual frozen Gradle log and four Android XML results identify twenty-five cached tests, not new execution',
          retained['passed'] and retained['gradle_cached'] and not retained['new_test_execution']
          and retained['retained_tests'] == 25 and retained['task'] == '> Task :app:testDebugUnitTest UP-TO-DATE'
          and hashlib.sha256((EVIDENCE / 'native-build.log').read_bytes()).hexdigest() == retained['native_log_sha256']
          and retained['task'] in (EVIDENCE / 'native-build.log').read_text(encoding='utf-8').splitlines()
          and len(retained['xml_sha256']) == 4 and all(hashlib.sha256(
              (APP / 'mobile/android/app/build/test-results/testDebugUnitTest' / name).read_bytes()).hexdigest() == value
              for name, value in retained['xml_sha256'].items()))
    java = read(EVIDENCE.parent / 'android-upload/jvm-source-proof.json')
    check('Unchanged pure Java boundaries retain the exact increment66 source and four XML proofs; not newly executed',
          java['passed'] and java['tests'] == 25 and java['source_sha256'] == digest(java['sources'])
          and len(java['xml_sha256']) == 4 and all(hashlib.sha256(
              (APP / 'mobile/native/jvm-tests/build/test-results/test' / name).read_bytes()).hexdigest() == value
              for name, value in java['xml_sha256'].items()))
    linux = read(PREVIOUS / 'linux-build.json')
    artifact = APP / '.local/deliverable-creation-platform-linux-amd64'
    check('Unchanged backend retains its exact increment74 Linux artifact; not rebuilt, executed or deployed',
          linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend
          and artifact.is_file() and hashlib.sha256(artifact.read_bytes()).hexdigest() == linux['artifact_sha256']
          and not linux['artifact_executed'] and not linux['deployed'])
    dev, base = read(EVIDENCE / 'development-project-setup.json'), read(EVIDENCE / 'development-runtime.json')
    update = read(PREVIOUS / 'development-update.json')
    owner = read(APP / '.local/project-templates-api-owner.json')
    check('Fresh daily checks retain the same owned API identity and all sixty-nine migration hashes',
          dev['passed'] and base['passed'] and base['database']['migration_count'] == 69
          and base['database']['migration_manifest_matches_source'] and update['passed']
          and update['source_sha256'] == backend and update['api_pid'] == owner['pid']
          and all(dev['ownership'].values()) and all(p['passed'] for p in base['probes']))
    check('Fresh schema inspection confirms original append-only and deferred evidence constraints',
          dev['database']['mutation_append_only'] and dev['database']['deliverable_tool_constraints'] == 3
          and dev['database']['deliverable_scope_constraint'] == 1 and dev['database']['deliverable_receipt_function']
          and dev['database']['milestone_unicode_whitespace'] and dev['database']['deferred_evidence'] == 2
          and dev['verification_script_sha256'] == hashlib.sha256((APP / 'scripts/verify_mobile_project_setup_development.py').read_bytes()).hexdigest()
          and base['verification_script_sha256'] == hashlib.sha256((APP / 'scripts/verify_agent_execution_development.py').read_bytes()).hexdigest())
    check('External integrations remain disabled, all Skills draft and disposable databases absent',
          dev['integrations_disabled'] and dev['all_eight_skills_draft']
          and dev['database']['active_profiles'] == dev['database']['enabled_telegram_channels'] == dev['database']['disposable_test_databases'] == 0)
    check('Embedded and documented OpenAPI copies still match',
          (APP / 'backend/internal/contract/openapi.json').read_bytes() == (APP.parent / 'docs/development/openapi.json').read_bytes())
    check('Private browser fixture credentials have been removed', not (APP / '.local/business-browser-fixture.json').exists())
    with socket.socket() as probe:
        probe.settimeout(1)
        check('Temporary browser review port5189 is released', probe.connect_ex(('127.0.0.1', 5189)) != 0)
    audit = read(EVIDENCE.parent / 'android-native/dependency-audit.json')
    check('The unchanged dependency lock keeps the unresolved production security gate open',
          hashlib.sha256((APP / 'package-lock.json').read_bytes()).hexdigest() == audit['package_lock_sha256']
          and not audit['production_security_gate_passed'] and audit['vulnerabilities']['total'] == 114)
    reused = [PREVIOUS / name for name in ('result.json', 'runtime-build.json', 'web-build.json', 'linux-build.json', 'development-update.json')]
    reused.append(EVIDENCE.parent / 'android-upload/jvm-source-proof.json')
    result = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
              'checks': checks, 'backend_source_sha256': backend,
              'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'reused_evidence': [{'path': p.relative_to(APP.parent).as_posix(), 'sha256': hashlib.sha256(p.read_bytes()).hexdigest(),
                                   'new_execution': False} for p in reused],
              'android_unit_test_scope': 'Twenty-five Gradle UP-TO-DATE results retained; not newly executed or device-tested.',
              'full_goal_complete': False, 'production_deployed': False, 'real_model_verified': False,
              'real_cloud_verified': False, 'real_device_verified': False}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
