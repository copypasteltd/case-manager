"""Independent156 proof: generated Office bytes, explicit creation scope and exact recovery."""
import hashlib, json, subprocess
from pathlib import Path
from agent_office_create_sources import ROOT, APP, digest

BASE = '60c4e98f0636b0e94cbe4f85990f4b1fe143e813'
E = ROOT / 'docs/quality/evidence/agent-office-create'
STEPS = ('web-agent-office-create-tests', 'mobile-agent-office-create-compile', 'mobile-agent-office-create-tests', 'runtime-tests', 'skills-catalog', 'typecheck', 'web-build', 'weapp-build', 'douyin-build')
read = lambda p: json.loads(p.read_text(encoding='utf-8'))
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
def git(*args): return subprocess.check_output(['git', *args], cwd=ROOT)

def verify(include_scope=True):
    checks, paths = [], []
    def check(name, value): checks.append({'name': name, 'passed': bool(value)})
    def record(path): paths.append(path); return read(path)
    native = record(E / 'native-focused3.json')
    results = native['test_results']; names = {r['Test'] for r in results}
    check('Actual selected native file/worker/HTTP/PG38 records pass for unchanged backend inputs', native['passed'] and len(results) == 38 and all(r['Action'] == 'pass' for r in results) and native['source_sha256'] == native['source_after_sha256'] == digest(native['source_files']))
    check('Three real Office formats, ordinary exact read, fresh recovery and I/O revocation execute', all('TestAgentOfficeCreate/' + name in names for name in (
        'docx_real_binary_scan_commit_and_exact_receipt', 'xlsx_real_binary_scan_commit_and_exact_receipt', 'pptx_real_binary_scan_commit_and_exact_receipt',
        'cancelled_office_draft_recovers_exact_recipe_bytes_with_fresh_scan_and_first_version', 'authorization_change_during_binary_io_aborts_without_formal_file',
        'existing_office_file_update_stays_outside_new_creation_authority')))
    context = record(E / 'native-context-current.json')
    check('Selected existing context/audience/indexed-search/file-text/update regressions pass after receipt fix', context['passed'] and context['source_sha256'] == context['source_after_sha256'] == digest(context['source_files']) and all(r['Action'] == 'pass' for r in context['test_results']) and {'TestAgentContextAudience', 'TestAgentIndexedSearch', 'TestAgentFileText', 'TestAgentFileUpdates'} <= {r['Test'] for r in context['test_results']} )
    for step in STEPS:
        file = sorted(E.glob(step + '-*.json'), key=lambda p: int(p.stem.rsplit('-', 1)[1]))[-1]
        value = record(file)
        check(step + ' passes for current exact client/runtime/Skill inputs', value['passed'] and value['source_unchanged'] and value['source_sha256'] == value['source_after_sha256'] == digest(value['source_files']) and not value['new_android_apk'] and not value['full_regression'])
    transcript = record(E / 'runtime-and-client-summary.json')
    check('Actual runtime60/Web7/shared mobile4 and eight draft package hashes are explicit', transcript['passed'] and transcript['runtime_tests'] == 60 and transcript['runtime_passed'] == 60 and transcript['runtime_skipped'] == 0 and len(transcript['runtime_test_names']) == 60 and transcript['web_tests'] == 7 and transcript['mobile_tests'] == 4 and transcript['catalog']['count'] == 8 and not transcript['catalog']['actual_model_invoked'] and all(r['state'] == 'draft' for r in transcript['catalog']['skills']) and next(r for r in transcript['catalog']['skills'] if r['id'] == 'file-update')['version'] == '1.5.0' and all(sha(ROOT / n) == value for n, value in transcript['records'].items()))
    browser = record(E.parent / 'agent-office-create-web/result.json')
    manifest = record(E.parent / 'agent-office-create-web/source-manifest.json')
    capture = record(E.parent / 'agent-office-create-web/media.json')
    check('Five actual Web actions bind current inputs and close all owned resources', browser['passed'] and len(browser['checks']) == 5 and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'] and browser['build_source_unchanged'] and browser['source_sha256'] == manifest['sha256'] == digest(manifest['files']) and not capture['real_model_invoked'] and not capture['real_cloud_invoked'])
    media = record(E / 'media.json'); visual = record(E / 'visual-review.json')
    check('Actual three PNGs, silent pixel MP4 and disclosed screenshot GIF retain reviewed hashes', media['passed'] and visual['passed'] and visual['artifacts'] == media['artifacts'] and visual['reviewed_screenshots'] == capture['screenshots'] and len(capture['screenshots']) == 3 and all(sha(ROOT / row['file']) == row['sha256'] for row in [*media['artifacts'], *capture['screenshots']]))
    release = record(ROOT / 'docs/releases/v0.1.0-rc.8/release-manifest.json')
    refs = release['prior_freezes_unchanged'] + [{'tag': 'v0.1.0-rc.8', 'source_commit': '160d78f8032be21c9eab72fc28415fda598fb7df', 'artifact_commit': 'bbf8328e0340d605b5c2552775a4302ffe41cb2b'}]
    check('All eight source tags and artifact branches remain immutable', len(refs) == 8 and all(git('rev-parse', r['tag'] + '^{}').decode().strip() == r['source_commit'] and git('rev-parse', 'codex/release-' + r['tag']).decode().strip() == r['artifact_commit'] for r in refs))
    old = git('ls-tree', '-r', '--name-only', BASE, '--', 'app/backend/migrations').decode().splitlines()
    check('No new schema; all113 migration bytes retain the published baseline', len([n for n in old if n.endswith('.sql')]) == 113 and all((ROOT / n).read_bytes() == git('show', BASE + ':' + n) for n in old))
    changed = git('diff', BASE, '--name-only').decode().splitlines()
    old_scripts = set(git('ls-tree', '-r', '--name-only', BASE, '--', 'app/scripts').decode().splitlines())
    check('Earlier evidence/media/freezes and existing verification helpers retain their bytes', not any(n.startswith(('docs/quality/evidence/', 'docs/media/', 'docs/releases/')) and n != 'docs/media/README.md' and not n.startswith(('docs/quality/evidence/agent-office-create', 'docs/media/screenshots/agent-office-create', 'docs/media/videos/agent-office-create', 'docs/media/gifs/agent-office-create')) for n in changed) and not any(n in old_scripts and n != 'app/scripts/build_openapi.py' for n in changed))
    retained = record(E / 'retained-functional-proof.json')
    check('Earlier broad regressions and frozen APK154/business153 are retained, not current execution', retained['passed'] and all(sha(ROOT / n) == value for n, value in retained['records'].items()) and retained['android_business_source_increment'] == 153 and retained['android_schema'] == 113 and retained['android_artifact_increment'] == 154 and not retained['new_android_apk'] and retained['remaining_work_packages'] == 23)
    api = record(E / 'api-contract.json')
    check('Existing paths stay equivalent; only disclosed create/target/diff schemas expand', api['passed'] and api['old_paths_changed'] == [] and api['new_paths'] == 0 and api['new_schemas'] == ['AgentOfficePreview'] and set(api['old_schemas_changed']) == {'AgentFileCreateInput', 'AgentFileUpdateTarget', 'AgentFileDiff'} and sha(ROOT / 'docs/development/openapi.json') == api['sha256'] and (ROOT / 'docs/development/openapi.json').read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes())
    creation = read(APP / 'skills/file-update/input.schema.json')
    check('File-update Skill stays draft1.5.0, existing updates remain text-only and other seven draft Skills stay unchanged', read(APP / 'skills/file-update/manifest.json')['version'] == '1.5.0' and creation['oneOf'][0]['properties']['format']['enum'] == ['text/plain', 'text/markdown'] and len(creation['oneOf'][1]['properties']['media_type']['enum']) == 5 and len(list((APP / 'skills').glob('*/manifest.json'))) == 8 and all(read(p)['state'] == 'draft' for p in (APP / 'skills').glob('*/manifest.json')) and all((ROOT / n).read_bytes() == git('show', BASE + ':' + n) for n in git('ls-tree', '-r', '--name-only', BASE, '--', 'app/skills', 'docs/skills').decode().splitlines() if not n.startswith(('app/skills/file-update/', 'docs/skills/file-update/'))))
    failures = record(E / 'attempts.json')
    check('Failed native fixture and mobile typecheck remain traceable', failures['passed'] and failures['failed_attempts_count'] >= 2 and all(sha(ROOT / n) == value for n, value in failures['records'].items()))
    cleanup = record(E / 'resource-cleanup.json')
    check('Owned native/HTTP/browser resources closed and original PG/API owners preserved', cleanup['passed'] and cleanup['original_listeners'] == {'14660': 3888, '18080': 2600} and cleanup['owned_listener_ports_open'] == [] and not cleanup['private_fixture_present'])
    for name in (('scope-check.json', 'acceptance-applicability-check.json') if include_scope else ()):
        value = record(E / name); check(name + ' verifies classification without passing business acceptance', value['passed'] and (name != 'acceptance-applicability-check.json' or value['business_cases_passed'] == 0))
    baseline = record(ROOT / 'docs/quality/current-functional-baseline.json')
    check('Source156/runtime60 is separate from frozen153/APK154 and still-open production acceptance', baseline['increment'] == 156 and baseline['schema_migration'] == 113 and baseline['current_development_database_schema'] == 100 and baseline['current_runtime_tests_executed'] and baseline['runtime_tests'] == 60 and not baseline['current_android_apk_built'] and baseline['retained_android_artifact_increment'] == 154 and not baseline['full_goal_complete'] and not baseline['production_ready'])
    return {'passed': all(row['passed'] for row in checks), 'checks': checks, 'records': {p.relative_to(ROOT).as_posix(): sha(p) for p in paths}, 'interaction_source_sha256': digest(), 'verification_script_sha256': sha(Path(__file__)),
        'retained': {'backend_full_increment': 121, 'client_full_increment': 136, 'development_android_increment': 154, 'development_android_source_increment': 153, 'development_android_schema': 113},
        'current_runtime_tests': 60, 'full_backend_suite_executed': False, 'new_android_apk': False, 'formal_android_acceptance': False, 'full_goal_complete': False, 'production_ready': False, 'virtualization_used': False}

if __name__ == '__main__':
    import sys
    result = verify('--prepare' not in sys.argv)
    (E / 'final-source-check.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'checks': len(result['checks']), 'failed': [row['name'] for row in result['checks'] if not row['passed']]}))
    raise SystemExit(0 if result['passed'] else 1)
