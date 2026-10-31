"""Independent155 focused source proof; retained freezes are not new acceptance."""
import hashlib, json, subprocess
from pathlib import Path
from business_followups_sources import APP, ROOT, digest

E = ROOT / 'docs/quality/evidence/business-followups'
BASE = 'b0f7173b6c6b4ba3ff38691f3be925b0911260b0'
STEPS = ('web-business-followups-tests', 'mobile-business-followups-compile', 'mobile-business-followups-tests', 'typecheck', 'web-build', 'weapp-build', 'douyin-build')
read = lambda p: json.loads(p.read_text(encoding='utf-8'))
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
def git(*args): return subprocess.check_output(['git', *args], cwd=ROOT)

def verify(include_scope=True):
    checks, paths = [], []
    def check(name, value): checks.append({'name': name, 'passed': bool(value)})
    def record(path): paths.append(path); return read(path)
    native = record(E / 'native-focused4.json')
    events = native['test_results']
    names = {row['Test'] for row in events}
    check('Actual native HTTP/PG eleven subcases and one suite pass for unchanged current backend inputs', native['passed'] and len(events) == 12 and all(row['Action'] == 'pass' for row in events) and native['source_sha256'] == native['source_after_sha256'] == digest(native['source_files']))
    check('Ordinary OA return/resubmit and delivery changes/resubmit/independent acceptance execute', all('TestBusinessFollowups/' + name in names for name in ('ordinary_submission_return_and_resubmission_recompute_current_attention', 'formal_submission_moves_attention_to_an_independent_current_reviewer', 'request_changes_returns_ownership_and_exact_next_revision_then_independent_accept_clears_work', 'removing_current_review_authority_hides_count_while_ordinary_metadata_remains_readable', 'current_human_session_rejects_stale_versions_and_executor_identities')))
    for step in STEPS:
        path = sorted(E.glob(step + '-*.json'), key=lambda p: int(p.stem.rsplit('-', 1)[1]))[-1]
        value = record(path)
        check(step + ' passes for exact current client inputs', value['passed'] and value['source_unchanged'] and value['source_sha256'] == value['source_after_sha256'] == digest(value['source_files']) and not value['new_android_apk'] and not value['full_regression'])
    browser = record(E.parent / 'business-followups-web/result.json')
    manifest = record(E.parent / 'business-followups-web/source-manifest.json')
    capture = record(E.parent / 'business-followups-web/media.json')
    check('Four actual Web checks bind current app/helper inputs and close owned contexts/servers', browser['passed'] and len(browser['checks']) == 4 and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'] and browser['build_source_unchanged'] and browser['source_sha256'] == manifest['sha256'] == digest(manifest['files']))
    media = record(E / 'media.json')
    visual = record(E / 'visual-review.json')
    check('Three reviewed actual screenshots, pixel MP4 and disclosed screenshot GIF bind current capture hashes', media['passed'] and visual['passed'] and visual['artifacts'] == media['artifacts'] and visual['reviewed_screenshots'] == capture['screenshots'] and len(capture['screenshots']) == 3 and all(sha(ROOT / row['file']) == row['sha256'] for row in [*media['artifacts'], *capture['screenshots']]))
    release = record(ROOT / 'docs/releases/v0.1.0-rc.8/release-manifest.json')
    refs = release['prior_freezes_unchanged'] + [{'tag': 'v0.1.0-rc.8', 'source_commit': '160d78f8032be21c9eab72fc28415fda598fb7df', 'artifact_commit': 'bbf8328e0340d605b5c2552775a4302ffe41cb2b'}]
    check('All eight source tags and artifact branches stay immutable', len(refs) == 8 and all(git('rev-parse', r['tag'] + '^{}').decode().strip() == r['source_commit'] and git('rev-parse', 'codex/release-' + r['tag']).decode().strip() == r['artifact_commit'] for r in refs))
    migrations = git('ls-tree', '-r', '--name-only', BASE, '--', 'app/backend/migrations').decode().splitlines()
    check('All113 ordinary schema upgrades retain exact bytes; no new migration', len([n for n in migrations if n.endswith('.sql')]) == 113 and all((ROOT / n).read_bytes() == git('show', BASE + ':' + n) for n in migrations))
    changed = git('diff', BASE, '--name-only').decode().splitlines()
    old_scripts = set(git('ls-tree', '-r', '--name-only', BASE, '--', 'app/scripts').decode().splitlines())
    check('Earlier proof, public media, releases and existing verification helpers are preserved', not any(n.startswith(('docs/quality/evidence/', 'docs/media/', 'docs/releases/')) and n != 'docs/media/README.md' and not n.startswith(('docs/quality/evidence/business-followups', 'docs/media/screenshots/business-followups', 'docs/media/videos/business-followups', 'docs/media/gifs/business-followups')) for n in changed) and not any(n in old_scripts and n != 'app/scripts/build_openapi.py' for n in changed))
    retained = record(E / 'retained-functional-proof.json')
    check('Full backend121/clients136/runtime151 and frozen154 APK remain explicitly retained', retained['passed'] and all(sha(ROOT / n) == value for n, value in retained['records'].items()) and retained['android_business_source_increment'] == 153 and retained['android_schema'] == 113 and retained['android_artifact_increment'] == 154 and not retained['new_android_apk'] and not retained['runtime_reexecuted'])
    api = record(E / 'api-additive.json')
    check('One new read operation/four schemas are additive; prior API definitions stay equivalent', api['passed'] and api['old_paths_changed'] == [] and api['old_schemas_changed'] == [] and api['new_paths'] == 1 and api['new_schemas'] == 4 and sha(ROOT / 'docs/development/openapi.json') == api['sha256'] and (ROOT / 'docs/development/openapi.json').read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes())
    attempts = record(E / 'attempts.json')
    check('Failed development/capture records remain preserved with explicit scope', attempts['passed'] and attempts['failed_attempts_count'] >= 4 and all(sha(ROOT / n) == value for n, value in attempts['records'].items()))
    check('All eight Skills remain draft', len(list((APP / 'skills').glob('*/manifest.json'))) == 8 and all(read(path)['state'] == 'draft' for path in (APP / 'skills').glob('*/manifest.json')))
    cleanup = record(E / 'resource-cleanup.json')
    check('Owned temporary browser resources closed; original PG/API owners preserved', cleanup['passed'] and cleanup['original_listeners'] == {'14660': 3888, '18080': 2600} and cleanup['owned_listener_ports_open'] == [] and not cleanup['private_fixture_present'])
    for name in (('scope-check.json', 'acceptance-applicability-check.json') if include_scope else ()):
        value = record(E / name)
        check(name + ' verifies classification without executing business acceptance', value['passed'] and (name != 'acceptance-applicability-check.json' or value['business_cases_passed'] == 0))
    baseline = record(ROOT / 'docs/quality/current-functional-baseline.json')
    check('Current155 source and frozen153 development APK remain distinct; production scope stays open', baseline['increment'] == 155 and baseline['schema_migration'] == 113 and baseline['current_development_database_schema'] == 100 and not baseline['current_android_apk_built'] and baseline['retained_android_artifact_increment'] == 154 and not baseline['full_goal_complete'] and not baseline['production_ready'] and retained['remaining_work_packages'] == 23)
    return {'passed': all(row['passed'] for row in checks), 'checks': checks, 'records': {p.relative_to(ROOT).as_posix(): sha(p) for p in paths}, 'retained': {'backend_full_increment': 121, 'client_full_increment': 136, 'runtime_increment': 151, 'runtime_tests': 58, 'development_android_increment': 154, 'development_android_source_increment': 153, 'development_android_schema': 113, 'new_android_apk': False}, 'interaction_source_sha256': digest(), 'verification_script_sha256': sha(Path(__file__)), 'full_backend_suite_executed': False, 'formal_android_acceptance': False, 'full_goal_complete': False, 'production_ready': False, 'new_android_apk': False, 'virtualization_used': False}

if __name__ == '__main__':
    import sys
    report = verify('--prepare' not in sys.argv)
    (E / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(report['checks']), 'failed': [row['name'] for row in report['checks'] if not row['passed']]}))
    raise SystemExit(0 if report['passed'] else 1)
