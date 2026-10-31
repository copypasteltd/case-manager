"""Source-bound focused proof; old freezes and full regressions remain retained."""
import hashlib, json, subprocess
from pathlib import Path
from replica_notifications_sources import APP, ROOT, digest, sources

E = ROOT / 'docs/quality/evidence/replica-notifications'
BASE = '53f5974183c24dfe258fa650f9a9c379a1f1bbac'
STEPS = ('web-replica-notice-tests', 'mobile-replica-notice-compile', 'mobile-replica-notice-tests', 'typecheck', 'web-build', 'weapp-build', 'douyin-build')
read = lambda p: json.loads(p.read_text(encoding='utf-8'))
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()

def git(*args): return subprocess.check_output(['git', *args], cwd=ROOT)

def verify():
    checks, paths = [], []
    def check(name, value): checks.append({'name': name, 'passed': bool(value)})
    def record(p): paths.append(p); return read(p)
    native = record(E / 'native-mfa-policy.json')
    names = {v['Test'] for v in native['test_results']}
    check('selected native notification suites pass at exact current backend inputs', native['passed'] and native['exit_code'] == 0 and native['source_sha256'] == native['source_after_sha256'] == digest(native['source_files']) and len(names) == 15 and all(v['Action'] == 'pass' for v in native['test_results']) and 'TestInAppNotifications' in names)
    check('all four real current human sessions read the same exact file version and replica', all('TestReplicaFailureNotifications/' + c + '_current_human_reads_fixed_notification_target' in names for c in ('web', 'wechat', 'douyin', 'android')))
    check('actual worker failure, acknowledgement, queueing, recovery, replay, stale event and revocation are covered', all('TestReplicaFailureNotifications/' + n in names for n in ('real_worker_failure_delivers_exact_replica_version_without_provider_coordinates', 'read_acknowledgement_never_resolves_failed_replication', 'ordinary_retry_queue_acceptance_keeps_attention_until_byte_verification', 'actual_verified_recovery_coalesces_into_new_unread_delivery', 'replayed_dispatch_keeps_immutable_failure_and_recovery_history', 'healthy_initial_replication_does_not_create_recovery_noise', 'delayed_failure_after_success_is_suppressed_without_invented_recovery', 'revoked_recipient_loses_reads_and_recovery_delivery')))
    attempt = record(E / 'attempts.json')
    failed = record(E / 'native-focused.json')
    old_test = ROOT / attempt['failed_native_source_snapshot']
    h = hashlib.sha256()
    for name in failed['source_files']:
        data = old_test.read_bytes() if name == 'backend/internal/httpapi/replica_notifications_integration_test.go' else (APP / name).read_bytes()
        h.update(name.encode() + b'\0' + data + b'\0')
    check('initial MFA-policy fixture failure and exact failed source remain disclosed', not failed['passed'] and failed['exit_code'] == 1 and h.hexdigest() == failed['source_sha256'] == failed['source_after_sha256'] and sha(old_test) == attempt['failed_native_source_sha256'] and attempt['production_authorization_weakened'] is False)
    for step in STEPS:
        p = sorted(E.glob(step + '-*.json'), key=lambda p: int(p.stem.rsplit('-', 1)[1]))[-1]
        r = record(p)
        check(step + ' binds exact current portable client inputs', r['passed'] and r['source_unchanged'] and r['source_sha256'] == r['source_after_sha256'] == digest(r['source_files']) and not r['new_android_apk'])
    counts = record(E / 'selected-test-counts.json')
    check('selected client counts and inactive hook are kept distinct from business cases', counts['passed'] and counts['web_selected_tests'] == counts['mobile_selected_tests'] == 5 and counts['native_result_records'] == 15 and counts['new_native_business_subcases'] == 12 and counts['inactive_browser_hook_records'] == 1 and counts['existing_notification_top_level_tests'] == 1 and not counts['inactive_hook_counted_as_business_case'])
    browser_dir = E.parent / 'replica-notifications-web'
    browser = record(browser_dir / 'result.json'); manifest = record(browser_dir / 'source-manifest.json')
    capture = record(E / 'capture-source-scope.json')
    check('actual Web recovery opening and read close every owned browser, API, server and PG', browser['passed'] and len(browser['checks']) == 3 and browser['source_unchanged'] and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'] and browser['native_fixture_exit_code'] == 0 and browser['source_sha256'] == browser['source_after_sha256'] == digest(manifest['files']))
    check('captured build inputs still match exactly with only this later read-only verifier addition', capture['passed'] and capture['sha256'] == digest(capture['files']) == browser['build_source_sha256'] == browser['build_source_after_sha256'] and set(sources()) - set(capture['files']) == {'scripts/verify_replica_notifications.py'} and not set(capture['files']) - set(sources()) and capture['later_inspection_only_additions'] == ['scripts/verify_replica_notifications.py'])
    media = record(E / 'media.json'); record(browser_dir / 'media.json')
    check('three actual PNGs, silent viewport MP4 and disclosed screenshot-tour GIF match exact bytes', media['passed'] and len(media['artifacts']) == 5 and all(sha(ROOT / v['file']) == v['sha256'] for v in media['artifacts']) and media['recordings'][0]['gif_is_screen_recording'] is False)
    review = record(E / 'visual-review.json')
    check('sampled visual review refers to exactly the current five media artifacts', review['passed'] and review['artifacts'] == media['artifacts'])
    applicability = record(E / 'acceptance-applicability-check.json')
    check('current AC/TC applicability remains classification only', applicability['passed'] and applicability['business_cases_passed'] == 0)
    changed = git('diff', BASE, '--name-only').decode().splitlines()
    check('all prior proof/media,111 migrations and five frozen releases preserve exact bytes', not any(n.startswith(('docs/quality/evidence/', 'docs/media/screenshots/', 'docs/media/videos/', 'docs/media/gifs/')) and 'replica-notifications' not in n for n in changed) and not any(n.startswith(('app/backend/migrations/', 'docs/releases/')) for n in changed))
    releases = record(ROOT / 'docs/releases/v0.1.0-rc.5/release-manifest.json')['prior_freezes_unchanged'] + [{'tag':'v0.1.0-rc.5', 'source_commit':'4155ce9c19304b8c65f5636e09707a5eba1e21cd', 'artifact_commit':'0f7a0c0158c7b86a8301a7f6d6798d6608f1951f'}]
    check('all five exact annotated source and artifact refs remain fixed', all(git('rev-parse', r['tag'] + '^{}').decode().strip() == r['source_commit'] and git('rev-parse', 'codex/release-' + r['tag']).decode().strip() == r['artifact_commit'] for r in releases))
    retained_paths = [ROOT / ('docs/quality/evidence/release-candidate-148/' + n) for n in ('final-source-check.json', 'build.json', 'apk-inspection.json', 'remote-publication.json')] + [ROOT / 'docs/quality/evidence/release-candidate-144/github-release-api.json']
    check('rc5 APK/current artifact provenance and API rejection retain exact published bytes', all(p.read_bytes() == git('show', BASE + ':' + p.relative_to(ROOT).as_posix()) for p in retained_paths))
    check('owned fixture capabilities and active cluster ownership records are removed', not any((APP / ('.local/' + n)).exists() for n in ('business-browser-fixture.json','replica-notifications-native-cluster.json','replica-notifications-browser-cluster.json')))
    check('all eight Skill packages stay draft with no new model acceptance', len(list((APP / 'skills').glob('*/manifest.json'))) == 8 and all(read(p)['state'] == 'draft' for p in (APP / 'skills').glob('*/manifest.json')))
    paths += [E / n for n in ('native-cluster-closed.json','reused-evidence.json')] + retained_paths + [old_test]
    return {'passed':all(r['passed'] for r in checks),'checks':checks,'records':{p.relative_to(ROOT).as_posix():sha(p) for p in paths},'retained':{'base_commit':BASE,'full_backend_increment':121,'full_client_increment':136,'runtime_increment':145,'android_artifact_increment':148,'android_business_increment':147,'android_schema':111,'current_apk_built':False,'frozen_release':'v0.1.0-rc.5','previous_proof_and_media_reexecuted':False,'inactive_browser_hook_counted_as_business_case':False},'interaction_source_sha256':digest(),'verification_script_sha256':sha(Path(__file__)),'fixture_only_adjustment':'Only isolated project-manager test accounts explicitly require MFA before actual enrollment; first failed attempt and exact old test source retained. Production authorization is unchanged.','inspection_only_adjustment':'The read-only verification module was added after capture. Exact captured build source list/hash is independently retained and matches every current captured input; only this module is added. Its current bytes are separately bound.','full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False,'new_android_apk':False,'full_regression':False,'virtualization_used':False}

if __name__ == '__main__':
    report=verify(); (E/'final-source-check.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':report['passed'],'checks':len(report['checks']),'failed':[r['name'] for r in report['checks'] if not r['passed']]})); raise SystemExit(0 if report['passed'] else 1)
