"""Read current focused proof; never relabel earlier full suites or frozen APKs."""
import hashlib, json, subprocess
from pathlib import Path
from storage_operations_sources import APP, ROOT, digest

E = ROOT / 'docs/quality/evidence/storage-operations'
BASE = '5b80dab1483f63e54c95069d1c5fb68bef9dd897'
STEPS = ('web-storage-tests', 'mobile-storage-compile', 'mobile-storage-tests', 'typecheck', 'web-build', 'weapp-build', 'douyin-build')

def read(p): return json.loads(p.read_text(encoding='utf-8'))
def sha(p): return hashlib.sha256(p.read_bytes()).hexdigest()
def latest(step): return sorted(E.glob(step + '-*.json'), key=lambda p: int(p.stem.rsplit('-', 1)[1]))[-1]

def verify():
    checks, paths = [], []
    def check(name, value): checks.append({'name': name, 'passed': bool(value)})
    p = E / 'native-final.json'; native = read(p); paths.append(p)
    names = {v['Test'] for v in native['test_results']}
    check('focused native HTTP/PG group and all recorded subcases passed at exact current backend sources', native['passed'] and native['exit_code'] == 0 and native['source_sha256'] == native['source_after_sha256'] == digest(native['source_files']) and len(names) == 15 and all(v['Action'] == 'pass' for v in native['test_results']))
    check('reviewed retries and idempotent historical receipts cover all four authenticated channel types', all('TestStorageOperations/' + channel + '_reviewed_retry_receipt_and_replay_preserve_ordinary_authority' in names for channel in ('web', 'wechat', 'douyin', 'android')))
    check('metadata privacy, ordinary file authority, job progress, worker success, immutable pages and revocation are actually exercised', all('TestStorageOperations/' + name in names for name in ('metadata_reader_sees_failure_and_fixed_ids_without_content_or_provider_details', 'organization_operator_does_not_gain_ordinary_file_content_or_retry_authority', 'unknown_provider_error_is_replaced_with_fixed_public_code', 'review_binds_job_progress_even_without_replica_version_change', 'worker_verification_and_exact_original_retry_receipt_are_separate_facts', 'immutable_history_pages_are_complete_and_cursor_bound_to_current_reader', 'cached_receipts_recheck_recent_mfa_and_revoked_organization_authority')))
    for step in STEPS:
        p = latest(step); r = read(p); paths.append(p)
        check(step + ' binds current portable client sources', r['passed'] and r['source_unchanged'] and r['source_sha256'] == r['source_after_sha256'] == digest(r['source_files']) and not r['new_android_apk'])
    browser_dir = E.parent / 'storage-operations-web'
    browser = read(browser_dir / 'result.json'); manifest = read(browser_dir / 'source-manifest.json')
    check('actual Web reviewed retry and immutable receipt close all owned browser/server/PG resources', browser['passed'] and len(browser['checks']) == 3 and browser['source_unchanged'] and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'] and browser['native_fixture_exit_code'] == 0 and browser['source_sha256'] == digest(manifest['files']) and browser['build_source_sha256'] == browser['build_source_after_sha256'] == digest())
    media = read(E / 'media.json')
    check('three actual PNGs, silent viewport MP4 and disclosed screenshot GIF match exact published bytes', media['passed'] and len(media['artifacts']) == 5 and all(sha(ROOT / v['file']) == v['sha256'] for v in media['artifacts']))
    review = read(E / 'visual-review.json')
    check('sampled visual inspection refers to exactly these artifacts', review['passed'] and review['artifacts'] == media['artifacts'])
    check('acceptance applicability is checked and does not execute business cases', read(E / 'acceptance-applicability-check.json')['passed'] and read(E / 'acceptance-applicability-check.json')['business_cases_passed'] == 0)
    changed = subprocess.check_output(['git', 'diff', BASE, '--name-only'], cwd=ROOT).decode().splitlines()
    check('earlier evidence/media, migrations1-110 and frozen releases retain original bytes', not any(n.startswith(('docs/quality/evidence/', 'docs/media/screenshots/', 'docs/media/videos/', 'docs/media/gifs/')) and 'storage-operations' not in n for n in changed) and not any(n.startswith('app/backend/migrations/') and not n.endswith('0111_storage_operations.sql') for n in changed) and not any(n.startswith('docs/releases/') for n in changed))
    check('owned fixture credentials and active cluster records are removed', not any((APP / ('.local/' + n)).exists() for n in ('business-browser-fixture.json', 'storage-operations-native-cluster.json', 'storage-operations-browser-cluster.json')))
    check('all four frozen source tags remain at exact published business commits', subprocess.check_output(['git', 'rev-parse', 'v0.1.0-rc.1^{}', 'v0.1.0-rc.2^{}', 'v0.1.0-rc.3^{}', 'v0.1.0-rc.4^{}'], cwd=ROOT).decode().splitlines() == ['a95c6afbd0e76684a64dc641c6abe598ad625277', '40cd118634117204e2e310395fac32b59af5cecc', 'e1c7a671cc4c7f52888a1e6ed72d7ce1a9c042d3', '294bf0509a7c87f63d9c829d5317fe33fb76a6f3'])
    check('all eight Skill packages remain draft without new model acceptance', len(list((APP / 'skills').glob('*/manifest.json'))) == 8 and all(read(p)['state'] == 'draft' for p in (APP / 'skills').glob('*/manifest.json')))
    retained_paths = [ROOT / ('docs/quality/evidence/release-candidate-146/' + n) for n in ('final-source-check.json', 'build.json', 'apk-inspection.json', 'remote-publication.json')] + [ROOT / 'docs/quality/evidence/release-candidate-144/github-release-api.json']
    check('rc4 development artifacts and unchanged GitHub API rejection are retained at their exact published bytes', all(sha(p) == hashlib.sha256(subprocess.check_output(['git', 'show', BASE + ':' + p.relative_to(ROOT).as_posix()], cwd=ROOT)).hexdigest() for p in retained_paths))
    paths += [E / n for n in ('native-cluster-closed.json', 'selected-test-counts.json', 'media.json', 'visual-review.json', 'acceptance-applicability-check.json', 'attempts.json')]
    paths += [browser_dir / n for n in ('result.json', 'source-manifest.json', 'media.json')] + retained_paths
    return {'passed': all(v['passed'] for v in checks), 'checks': checks, 'records': {p.relative_to(ROOT).as_posix(): sha(p) for p in paths}, 'retained': {'base_commit': BASE, 'full_backend_increment': 121, 'full_client_increment': 136, 'runtime_increment': 145, 'android_artifact_increment': 146, 'android_business_increment': 145, 'android_schema': 110, 'current_apk_built': False, 'frozen_release': 'v0.1.0-rc.4', 'previous_proof_and_media_reexecuted': False, 'inactive_browser_hook_counted_as_business_case': False}, 'interaction_source_sha256': digest(), 'verification_script_sha256': sha(Path(__file__)), 'full_backend_suite_executed': False, 'formal_android_acceptance': False, 'full_goal_complete': False, 'production_ready': False, 'new_android_apk': False, 'full_regression': False, 'virtualization_used': False}

if __name__ == '__main__':
    r = verify(); (E / 'final-source-check.json').write_text(json.dumps(r, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': r['passed'], 'checks': r['checks']}, ensure_ascii=True)); raise SystemExit(0 if r['passed'] else 1)
