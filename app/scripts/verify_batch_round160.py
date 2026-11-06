"""Verify complete-wave order, selected current proofs and honest retained limits."""
import hashlib
import json
from pathlib import Path
import subprocess
from run_batch_round160 import ROOT, APP, EVIDENCE as E, source_names, source_digest, client_source_names, load_plan

BASE = 'bd9c74ea777e6bd6321fb68b2f4bce0ef5cd66e9'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(names=None):
    return source_digest(names or source_names())


def reconstructed(names, overrides):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + overrides.get(name, ROOT / name).read_bytes() + b'\0')
    return value.hexdigest()


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT, text=True).splitlines()


def verify():
    checks, records = [], {}
    current, plan = digest(), load_plan()

    def check(name, value):
        checks.append({'name': name, 'passed': bool(value)})

    def record(path):
        records[path.relative_to(ROOT).as_posix()] = sha(path)
        return read(path)

    def latest(phase, client=False):
        target = source_digest(client_source_names()) if client else current
        matches = [p for p in E.glob(phase + '-*.json') if read(p).get('passed') and read(p).get('source_sha256') == target]
        if not matches:
            raise ValueError('Current-source passing proof missing: ' + phase)
        return record(max(matches, key=lambda p: read(p)['started_at']))

    initial = record(E / 'freeze-code-initial.json')
    first_native, first_clients = record(E / 'native-tests-initial.json'), record(E / 'client-checks-initial.json')
    freeze, native, clients = latest('freeze-code'), latest('native-tests'), latest('client-checks', True)
    repairs = record(E / 'batch-repairs.json')
    check('Complete six-task code/fixture wave and23-package review precede first unified checks',
          plan['implementation_completed_before_tests'] and len(plan['round_tasks']) == 6
          and all(i['state'] == 'implemented' for i in plan['round_tasks'])
          and plan['code_completed_at'] < initial['started_at'] < first_native['started_at']
          and initial['started_at'] < first_clients['started_at'] and not initial['tests_executed']
          and len(plan['packages']) == 23 and all(i['acceptance_state'] == 'open' for i in plan['packages'])
          and freeze['reviewed_package_ids'] == native['reviewed_package_ids'] == clients['reviewed_package_ids'])
    check('Initial native/client failures and both batch repairs remain explicit',
          not first_native['passed'] and not first_clients['passed'] and [i['batch'] for i in repairs['repairs']] == [1, 2])
    prior = record(E / 'native-tests-batch-1.json')
    adjustment = repairs['repairs'][1]
    before = ROOT / adjustment['before_file']
    records[adjustment['before_file']] = sha(before)
    required = {'TestAgentOrganizationPlans', 'TestAgentOrganizationGuards', 'TestAgentOrganizationTools',
                'TestCompletedUploadRetirementFourClients', 'TestCompletedUploadRetirementFencesAndRenewal',
                'TestCompletedUploadRetirementProtection', 'TestOperationsWorkbenchIncidents', 'TestPurchaseCompletion',
                'TestRound160ForeignCloseoutPrincipal', 'TestRound160CompleteWorkInventory'}
    passed = {i.get('Test') for i in prior['test_results'] if i['Action'] == 'pass'}
    unaffected = [i for i in prior['test_results'] if i['Action'] == 'pass' and i.get('Test')
                  and not i['Test'].startswith('TestRound160TerminalObservation') and i['Test'] != 'TestAgentOrganizationBrowserFixture']
    check('Ten unaffected selected native suites retain85 passing records with exact pre-SQL-fix reconstruction',
          prior['source_unchanged'] and not prior['passed'] and required <= passed and len(unaffected) == 85
          and sha(before) == adjustment['before_sha256']
          and reconstructed(prior['source_files'], {'app/backend/internal/files/completed_upload_retirement.go': before}) == prior['source_sha256'])
    results = native['test_results']
    check('Affected terminal HEAD suite passes under exact current complete code source',
          native['source_unchanged'] and native['source_after_sha256'] == current
          and native['native_test_selector'] == '^TestRound160TerminalObservation$'
          and len(results) == 5 and all(i['Action'] == 'pass' for i in results)
          and any(i.get('Test') == 'TestRound160TerminalObservation' for i in results) and not native['full_backend_suite_executed'])
    steps = {'typecheck', 'web-tests', 'mobile-tests', 'mobile-new-contracts', 'web-build', 'weapp-build', 'douyin-build'}
    check('Seven current client steps pass with unchanged actual inputs',
          clients['source_unchanged'] and clients['source_files'] == client_source_names()
          and clients['source_after_sha256'] == source_digest(client_source_names())
          and {i['step'] for i in clients['test_results']} == steps and all(i['passed'] for i in clients['test_results']))
    retained_steps = {i['step']: i for i in first_clients['test_results']}
    check('Initial runtime64 and draft Skill catalog results retained with unchanged runtime/Skill sources',
          not git('diff', '--name-only', BASE, '--', 'app/agent-runtime', 'app/skills', 'docs/skills')
          and all(retained_steps[n]['passed'] for n in ['runtime-tests', 'skill-catalog']))
    release = record(E / 'native-release-initial.json')
    check('Native packaging/controller result binds current catalog/service inputs without Linux acceptance',
          release['passed'] and not release['production_acceptance'] and not release['virtualization_used']
          and all(sha(ROOT / i['file']) == i['sha256'] for i in release['sources']))
    command_build = record(E / 'native-command-build.json')
    check('Native server command including the new dedicated worker compiles at exact current source',
          command_build['passed'] and command_build['exit_code'] == 0 and command_build['source_unchanged']
          and command_build['source_sha256'] == current and not command_build['production_acceptance'])
    for client, count, shots in [('web', 2, 2), ('mobile', 4, 4)]:
        directory = E.parent / ('batch-round-160-functional-wave-' + client + '-capture-2')
        result, manifest = record(directory / 'result.json'), record(directory / 'source-manifest.json')
        capture, video = record(directory / 'encoded-media.json'), record(directory / 'capture-manifest.json')
        h = hashlib.sha256()
        for name in manifest['files']:
            h.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
        check(client + ' actual native HTTP/PG UI flow binds current source and closes owned resources',
              result['passed'] and h.hexdigest() == manifest['sha256'] == result['source_sha256']
              and result['build_source_sha256'] == current and result['build_source_unchanged']
              and result['source_unchanged'] and len(result['checks']) == count
              and not result['console_errors'] and not result['server_errors'] and result['native_fixture_exit_code'] == 0
              and all(result[k] for k in ['owned_browser_closed', 'owned_review_server_stopped', 'owned_build_process_stopped',
                                          'owned_browser_runner_stopped', 'isolated_database_cleanup_passed']))
        check(client + ' screenshots and silent actual recording match immutable output hashes',
              capture['passed'] and capture['synthetic_data'] and not capture['real_cloud'] and not capture['production_acceptance']
              and len(capture['screenshots']) == shots and all(sha(ROOT / i['file']) == i['sha256'] for i in capture['screenshots'] + capture['artifacts'])
              and capture['encoder_sha256'] == sha(APP / 'scripts/render_batch_round160_media.py')
              and video['credentials_excluded'] and video['synthetic_data'] and video['real_browser_pixels'])
        interaction = record(directory / 'interaction-media.json')
        check(client + ' interaction GIF derives from the exact actual recording',
              interaction['passed'] and interaction['actual_viewport_recording_conversion']
              and sha(ROOT / interaction['file']) == interaction['sha256'] and sha(ROOT / interaction['source_video']) == interaction['source_video_sha256']
              and sha(APP / 'scripts/derive_batch_round160_interaction.py') == interaction['source_script_sha256'])
    capture_adjustment = record(E / 'capture-review-adjustment.json')
    original_checker = ROOT / capture_adjustment['before_checker']
    records[capture_adjustment['before_checker']] = sha(original_checker)
    original_web = record(ROOT / capture_adjustment['first_record'])
    original_mobile = record(E.parent / 'batch-round-160-functional-wave-mobile-initial/result.json')
    check('Wrong-group initial Web capture and failed mobile locator retained without application changes',
          original_web['passed'] and not original_mobile['passed'] and not capture_adjustment['application_changed']
          and sha(original_checker) == capture_adjustment['before_sha256'])
    visual = record(E / 'visual-review.json')
    check('Six screenshots and two interaction GIFs have hash-bound manual visual review',
          visual['passed'] and len(visual['files']) == 8 and not visual['production_acceptance']
          and all(sha(ROOT / i['file']) == i['sha256'] for i in visual['files']))
    cleanup = [record(p) for p in sorted(E.glob('*/native-cluster-closed.json'))]
    check('All seven owned native clusters stopped while developmentPG100 is preserved',
          len(cleanup) == 7 and all(i['passed'] and i['owned_cluster_stopped'] and i['development_cluster_preserved']
                                  and i['remaining_disposable_databases'] == 0 for i in cleanup)
          and not (APP / '.local/business-browser-fixture.json').exists())
    retained = record(ROOT / 'docs/quality/evidence/batch-round-159/final-source-check.json')
    old_android = record(ROOT / 'docs/quality/evidence/batch-round-158/client-scope-disposition.json')
    check('Prior full regression, failed Android JS and APK154 keep original provenance',
          retained['passed'] and not retained['full_backend_suite_executed'] and not old_android['current_android_js_build_passed']
          and not old_android['new_android_apk'] and plan['current_android_js_rebuild_deferred'] and not plan['new_android_apk'])
    forbidden = []
    for name in git('diff', '--name-only', BASE, '--', 'app/backend/migrations', 'docs/quality/evidence', 'docs/media'):
        if name.startswith('app/backend/migrations/'):
            if int(Path(name).name.split('_')[0]) <= 121:
                forbidden.append(name)
        elif name != 'docs/media/README.md' and not name.startswith(('docs/quality/evidence/batch-round-160', 'docs/media/batch-round-160/')):
            forbidden.append(name)
    check('Earlier121 migrations and all earlier proof/media bytes remain unchanged', not forbidden)
    check('Only normal append-only122-123 schema upgrades are added', plan['schema'] == 123
          and all((APP / 'backend/migrations' / n).is_file() for n in ['0122_closeout_refund_units.sql', '0123_terminal_upload_observations.sql']))
    check('Eight prior freezes preserve rc8 business153/schema113', len(git('tag', '--list', 'v0.1.0-rc.*')) == 8
          and git('rev-parse', 'v0.1.0-rc.8^{}') == ['160d78f8032be21c9eab72fc28415fda598fb7df'])
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    check('Eight draft Skills,23 active packages and five exclusions remain explicit',
          len(skills) == 8 and all(i['state'] == 'draft' for i in skills)
          and len([i for i in scope['items'] if i['status'] == 'active']) == 23
          and len(plan['excluded_ids']) == len([i for i in scope['items'] if i['status'] != 'active']) == 5 and not plan['production_acceptance'])
    return {'increment': 160, 'passed': all(i['passed'] for i in checks), 'checks': checks, 'records': records,
            'interaction_source_sha256': current, 'verification_script_sha256': sha(Path(__file__)),
            'full_backend_suite_executed': False, 'formal_android_acceptance': False, 'full_goal_complete': False,
            'production_ready': False, 'virtualization_used': False, 'new_android_apk': False, 'native_result_records': len(results),
            'retained_unaffected_native_pass_records': len(unaffected), 'web_tests': 520, 'mobile_tests': 566, 'retained_runtime_tests': 64,
            'current_four_client_builds': False, 'current_client_builds': ['Web', 'WeChat', 'Douyin'],
            'current_android_js_build_passed': False, 'deferred_checks': ['android-bundle', 'full-backend-regression'],
            'retained': {'full_backend_increment': 121, 'prior_selected_increment': 159, 'development_android_increment': 154,
                         'development_android_source_increment': 153, 'development_android_schema': 113, 'runtime_and_skills_attempt': '160-initial'}}


if __name__ == '__main__':
    value = verify()
    (E / 'final-source-check.json').write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': value['passed'], 'checks': len(value['checks']), 'failed': [i['name'] for i in value['checks'] if not i['passed']]}))
    raise SystemExit(0 if value['passed'] else 1)
