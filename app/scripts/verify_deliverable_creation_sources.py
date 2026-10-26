"""Bind deliverable draft creation evidence to the actual fixed implementation and artifacts."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/deliverable-creation'


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

    backend = module('delivery_backend', 'verify_backend_batches.py').digest()
    regression = read(EVIDENCE / 'result.json')
    check('Every discovered backend test passes against the current complete source',
          regression['passed'] and regression['source_sha256'] == backend
          and regression['source_unchanged_during_tests'] and regression['all_discovered_tests_executed']
          and regression['discovered_top_level_tests'] == 146 and regression['batch_count'] == 8
          and all(c['Action'] == 'pass' for c in regression['checks']))
    creation = [c for c in regression['checks'] if c['Test'].startswith('TestAgentDeliverableCreation/')]
    check('All ten real deliverable draft creation integration scenarios pass',
          len(creation) == 10 and all(c['Action'] == 'pass' for c in creation))
    unicode_cases = {'TestAgentMilestoneCreation/creates_once_with_original_command_and_exact_result',
                     'TestAgentProjectStages/exact_milestone_fields_retry_read_and_real_completion'}
    actual_unicode = {c['Test'] for c in regression['checks'] if c['Action'] == 'pass' and c['Test'] in unicode_cases}
    reproduction = read(EVIDENCE / 'whitespace-reproduction.json')
    check('Both original milestone whitespace failures are reproduced before migration69 and pass against the current source',
          reproduction['expected_failure_reproduced'] and reproduction['exit_code'] == 1
          and reproduction['migration_count'] == 68 and len(reproduction['observed_failures']) == 2
          and actual_unicode == unicode_cases)
    runtime = read(EVIDENCE / 'runtime-build.json')
    check('Current SDK, schemas and Skill instructions pass all thirty-nine controlled tests',
          runtime['status'] == 'passed' and runtime['source_sha256'] == runtime['source_after_sha256']
          == module('delivery_runtime', 'verify_agent_runtime.py').digest()[0]
          and runtime['tests']['pass'] == runtime['tests']['tests'] == 39
          and runtime['tests']['fail'] == runtime['tests']['skipped'] == runtime['tests']['cancelled'] == 0)
    manifests = [read(p) for p in sorted((APP / 'skills').glob('*/manifest.json'))]
    update_skill = next(m for m in manifests if m['skill_id'] == 'project-update')
    check('All eight Skills remain draft; project-update 1.4.0 includes exact deliverable draft creation',
          len(manifests) == 8 and all(m['state'] == 'draft' for m in manifests)
          and update_skill['version'] == '1.4.0' and 'create_deliverable' in update_skill['allowed_tools']
          and len(update_skill['allowed_tools']) == 11 and len(update_skill['completion_tools']) == 9)
    web = read(EVIDENCE / 'web-build.json')
    check('All eighty-one Web tests and production build bind the current source',
          web['passed'] and web['source_unchanged_during_run'] and web['source_sha256'] == digest(web['sources'])
          and web['tests']['numTotalTests'] == web['tests']['numPassedTests'] == 81
          and web['tests']['numFailedTests'] == web['tests']['numPendingTests'] == 0
          and all(c['exit_code'] == 0 for c in web['commands']))
    mobile = read(EVIDENCE / 'mobile-build.json')
    check('All eighty-nine mobile tests, type checks and both sixteen-page mini-program builds pass',
          mobile['passed'] and mobile['source_unchanged_during_run']
          and mobile['source_sha256'] == module('delivery_mobile', 'verify_mobile.py').digest()[0]
          and mobile['tests']['tests'] == mobile['tests']['pass'] == 89
          and all(c['exit_code'] == 0 for c in mobile['commands'])
          and all(len(read(APP / 'mobile/dist' / target / 'app.json')['pages']) == 16 for target in ('weapp', 'tt')))
    for target in ('web', 'mobile'):
        folder = EVIDENCE.parent / ('deliverable-creation-' + target)
        result, sources = read(folder / 'result.json'), read(folder / 'source-manifest.json')
        check(target + ' actual browser history, complete metadata, association, retry and cancellation match current sources',
              result['passed'] and result['source_sha256'] == result['source_after_sha256'] == digest(sources['files'])
              and result['source_unchanged'] and len(result['checks']) == 5
              and not result['console_errors'] and not result['server_errors'])
        check(target + ' owned browser, context, temporary server and native fixture are closed',
              result['owned_browser_closed'] and result['browser_contexts_closed'] == 1
              and result['native_fixture_exit_code'] == 0 and result['isolated_database_cleanup_passed']
              and (result['owned_vite_stopped'] if target == 'web' else result['owned_review_server_stopped']))
        if target == 'mobile':
            check('Mobile native fixture, fresh H5 build and actual browser use identical fixed sources',
                  result['build_and_browser_sources_match']
                  and result['source_sha256_before_fixture_and_build'] == result['source_sha256_after_build'] == result['source_sha256'])
    android, inspection = read(EVIDENCE / 'build.json'), read(EVIDENCE / 'apk-inspection.json')
    apk = APP / android['artifacts']['apk']['path']
    check('New frozen Android development APK matches current sources and all seventeen package checks',
          android['passed'] and android['source_sha256'] == digest(android['sources']) and apk.is_file()
          and hashlib.sha256(apk.read_bytes()).hexdigest() == android['artifacts']['apk']['sha256']
          and inspection['passed'] and inspection['apk_sha256'] == android['artifacts']['apk']['sha256']
          and len(inspection['checks']) == 17 and all(c['passed'] for c in inspection['checks'])
          and not android['virtualization_used'] and not android['emulator_started'] and not android['device_executed'])
    java = read(EVIDENCE.parent / 'android-upload/jvm-source-proof.json')
    check('Unchanged pure Java boundaries retain their exact increment66 source and four JVM XML proofs',
          java['passed'] and java['tests'] == 25 and java['source_sha256'] == digest(java['sources'])
          and len(java['xml_sha256']) == 4 and all(hashlib.sha256(
              (APP / 'mobile/native/jvm-tests/build/test-results/test' / name).read_bytes()).hexdigest() == value
              for name, value in java['xml_sha256'].items()))
    linux = read(EVIDENCE / 'linux-build.json')
    artifact = APP / '.local/deliverable-creation-platform-linux-amd64'
    check('Current backend cross-compiles for Linux without execution or deployment',
          linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend
          and artifact.is_file() and hashlib.sha256(artifact.read_bytes()).hexdigest() == linux['artifact_sha256']
          and not linux['artifact_executed'] and not linux['deployed'])
    dev, base = read(EVIDENCE / 'development-deliverable-creation.json'), read(EVIDENCE / 'development-runtime.json')
    update = read(EVIDENCE / 'development-update.json')
    check('Daily API uses the current backend and sixty-nine exact migrations with unchanged PG, worker and Vite',
          dev['passed'] and base['passed'] and base['database']['migration_count'] == 69
          and base['database']['migration_manifest_matches_source'] and update['passed']
          and update['source_sha256'] == backend and update['api_ready'] and not update['postgres_restarted']
          and not update['notification_worker_restarted'] and not update['vite_restarted'])
    check('Development deliverable guards, original login and append-only evidence remain enforced',
          dev['database']['deliverable_tool_constraints'] == 3 and dev['database']['deliverable_scope_constraint'] == 1
          and dev['database']['deliverable_receipt_function'] and dev['database']['milestone_unicode_whitespace']
          and dev['database']['mutation_append_only']
          and dev['database']['deferred_evidence'] == 2
          and dev['verification_script_sha256'] == hashlib.sha256((APP / 'scripts/verify_deliverable_creation_development.py').read_bytes()).hexdigest())
    check('External integrations stay disabled and no disposable databases remain',
          dev['integrations_disabled'] and dev['all_eight_skills_draft']
          and dev['database']['active_profiles'] == dev['database']['enabled_telegram_channels'] == dev['database']['disposable_test_databases'] == 0)
    check('OpenAPI copies match exactly', (APP / 'backend/internal/contract/openapi.json').read_bytes()
          == (APP.parent / 'docs/development/openapi.json').read_bytes())
    check('Private fixture credentials have been removed', not (APP / '.local/business-browser-fixture.json').exists())
    with socket.socket() as probe:
        probe.settimeout(1)
        check('Temporary browser review port is released', probe.connect_ex(('127.0.0.1', 5189)) != 0)
    audit = read(EVIDENCE.parent / 'android-native/dependency-audit.json')
    check('Unchanged dependency lock keeps unresolved production security findings open',
          hashlib.sha256((APP / 'package-lock.json').read_bytes()).hexdigest() == audit['package_lock_sha256']
          and not audit['production_security_gate_passed'])
    result = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
              'checks': checks, 'backend_source_sha256': backend,
              'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'reused_evidence': {'android_unit_tests': 'Gradle UP-TO-DATE; 25 retained passing results, not newly executed',
                                  'java': 'increment66 exact final pure Java source and JVM XML proof'},
              'full_goal_complete': False, 'production_deployed': False, 'real_model_verified': False,
              'real_cloud_verified': False, 'real_device_verified': False}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
