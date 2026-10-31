"""Verify frozen147/schema111 artifacts and distinguish retained execution."""
from datetime import datetime, timezone
import hashlib, json, socket, subprocess
from pathlib import Path

import build_android
import build_production_release as server
import storage_operations_sources as original
from build_candidate_148 import APP, ROOT, E, VERSION, SOURCE, sha

RELEASE = ROOT / 'docs/releases' / VERSION
HELPERS = ('build_candidate_148.py', 'build_candidate_148_server.py', 'inspect_candidate_148.py', 'seal_candidate_148.py', 'publish_candidate_148.py', 'verify_candidate_148.py')
read = lambda p: json.loads(p.read_text(encoding='utf-8'))

def digest():
    h = hashlib.sha256(original.digest().encode())
    for name in HELPERS: h.update(name.encode() + b'\0' + (APP / 'scripts' / name).read_bytes())
    return h.hexdigest()

def git(*args): return subprocess.check_output(['git', *args], cwd=ROOT).decode().strip()

def verify(save=False):
    checks, records = [], []
    def check(name, value): checks.append({'name': name, 'passed': bool(value)})
    def record(path):
        records.append({'file': path.relative_to(ROOT).as_posix(), 'sha256': sha(path)})
        return read(path)

    retained = record(E / 'retained-functional-proof.json')
    previous_path = ROOT / 'docs/quality/evidence/storage-operations/final-source-check.json'
    previous = record(previous_path)
    check('retained147 selected business/logic/browser/media proof matches exact current business sources', retained == previous and previous['passed'] and len(previous['checks']) == 19 and all(r['passed'] for r in previous['checks']) and all(sha(ROOT / n) == value for n, value in previous['records'].items()) and previous['interaction_source_sha256'] == original.digest() and previous['verification_script_sha256'] == sha(APP / 'scripts/verify_storage_operations.py') and git('rev-parse', SOURCE + ':' + previous_path.relative_to(ROOT).as_posix()) == git('hash-object', '--no-filters', str(previous_path)))
    native = record(E / 'retained-native-proof.json')
    names = {r['Test'] for r in native['test_results']}
    check('retained native15 records include13 substantive cases plus inactive hook/summary and all four authenticated retries', native == read(ROOT / 'docs/quality/evidence/storage-operations/native-final.json') and native['passed'] and native['source_sha256'] == native['source_after_sha256'] == original.digest(native['source_files']) and len(native['test_results']) == 15 and all(r['Action'] == 'pass' for r in native['test_results']) and all('TestStorageOperations/' + c + '_reviewed_retry_receipt_and_replay_preserve_ordinary_authority' in names for c in ('web', 'wechat', 'douyin', 'android')))

    manifest, build, seal = [record(p) for p in (RELEASE / 'release-manifest.json', RELEASE / 'server-build.json', E / 'seal.json')]
    scope = record(E / 'server-build-scope.json')
    check('fresh Linux/Web/runtime package fixes exact147 build inputs and schema111 with Node22', manifest['source']['commit'] == SOURCE and manifest['schema_version'] == 111 and build['passed'] and build['migrations'] == 111 and build['source']['base_commit'] == SOURCE and scope['passed'] and scope['frozen_input_bytes_verified'] and scope['inputs_sha256'] == build['source']['inputs_sha256'] == server.digest(server.canonical(server.inputs())) and scope['whole_workspace_dirty_at_build'] == build['source']['working_tree_dirty'] and scope['wrapper_sha256'] == sha(APP / 'scripts/build_candidate_148_server.py') and build['tool_versions']['node'] == 'v22.23.4' and build['archive_inspection']['passed'] and all(r['exit_code'] == 0 for r in build['commands']))
    check('no new primary business execution is claimed by packaging', not manifest['retained_business_smoke']['executed_this_increment'] and manifest['retained_business_smoke']['executed_increment'] == 147 and manifest['retained_business_smoke']['record_sha256'] == sha(E / 'retained-native-proof.json') and manifest['retained_business_smoke']['substantive_subcases'] == 13 and manifest['retained_business_smoke']['inactive_browser_hook_records'] == 1 and manifest['retained_business_smoke']['passing_result_records'] == 15 and not manifest['retained_not_reexecuted']['business_tests_reexecuted'])
    mini = record(E / 'miniapps-build.json')
    check('both fresh miniapps bind exact unchanged client sources and output hashes', mini['passed'] and mini['source_unchanged'] and mini['source_commit'] == SOURCE and mini['source_sha256'] == original.digest(mini['source_files']) and mini['wrapper_sha256'] == sha(APP / 'scripts/build_candidate_148.py') and {r['target'] for r in mini['phases']} == {'weapp', 'tt'} and all(r['exit_code'] == 0 for r in mini['phases']))
    mini_inspection = record(E / 'miniapps-inspection.json')
    check('actual two miniapp archives contain current replication paths and reviewed input logic', mini_inspection['passed'] and mini_inspection['archive_sha256'] == mini['archive_sha256'] and len(mini_inspection['checks']) == 2 and all(r['passed'] for r in mini_inspection['checks']))
    android, android_scope, inspection = [record(E / p) for p in ('build.json', 'android-build-scope.json', 'apk-inspection.json')]
    native_log = record(E / 'native-build-log.json'); raw_log = native_log['text'].encode('utf-8')
    check('original native Gradle log bytes and task outcomes are preserved verbatim', native_log['format'] == 'exact-utf8-log-v1' and len(raw_log) == native_log['bytes'] and hashlib.sha256(raw_log).hexdigest() == native_log['sha256'] and android_scope['task'] in native_log['text'])
    selected = {r['file'] for r in inspection['current_source_map_files']}
    check('fresh development APK contains exact current JS/DEX and replication module source content', android['passed'] and android['source_unchanged_during_build'] and android['source_sha256'] == build_android.sources()[0] and android_scope['passed'] and android_scope['source_sha256'] == original.digest(android_scope['source_files']) and android_scope['wrapper_sha256'] == sha(APP / 'scripts/build_candidate_148.py') and inspection['passed'] and all(r['passed'] for r in inspection['checks']) and inspection['build_record_sha256'] == sha(E / 'build.json') and inspection['inspection_script_sha256'] == sha(APP / 'scripts/inspect_candidate_148.py') and inspection['apk_sha256'] == android['artifacts']['apk']['sha256'] and {'app/mobile/src/pages/settings/storage.tsx', 'app/packages/contracts/src/storage-operations.ts'}.issubset(selected) and all(r['matches_actual_map'] and sha(ROOT / r['file']) == r['sha256'] for r in inspection['current_source_map_files']) and not inspection['device_executed'] and not inspection['production_release'])
    check('Gradle cached or fresh outcomes and original XML timestamps are explicit', android_scope['test_count'] == 28 and len(android_scope['xml']) == 4 and all(sha(ROOT / r['file']) == r['sha256'] for r in android_scope['xml']) and android_scope['cached'] == ('UP-TO-DATE' in android_scope['task'] or 'FROM-CACHE' in android_scope['task']) and manifest['android']['new_native_test_execution'] == (not android_scope['cached']) and not android_scope['standalone_jvm_rerun'] and not android_scope['real_device'] and not android_scope['virtualization'])
    check('five bounded sealed assets reconstruct all application source/docs and frozen media without private material', seal['passed'] and len(seal['assets']) == 5 and seal['assets'] == manifest['assets'] and seal['manifest_sha256'] == sha(RELEASE / 'release-manifest.json') and seal['server_receipt_sha256'] == sha(RELEASE / 'server-build.json') and seal['seal_script_sha256'] == sha(APP / 'scripts/seal_candidate_148.py') and seal['complete_source_blobs'] == manifest['source_archive']['packaged_git_blob_count'] and all(manifest['source_archive'][k] for k in ('complete_application_source', 'all_packaged_blob_bytes_match_git_source', 'media_zip_contains_all_frozen_media', 'historical_evidence_remains_in_frozen_git')) and seal['known_private_matches'] == seal['high_confidence_token_matches'] == 0 and all(0 < a['bytes'] < 100 * 1024 * 1024 for a in manifest['assets']))
    publication = record(RELEASE / 'git-publication.json'); artifact_commit = publication['artifact_commit']
    check('new refs are prepared with a private index and no overwritten refs or force push', publication['publisher_script_sha256'] == sha(APP / 'scripts/publish_candidate_148.py') and not publication['force_push_used'] and not publication['user_index_modified'])
    blobs = {}
    for row in git('ls-tree', '-r', '-l', artifact_commit, '--', 'docs/releases/' + VERSION).splitlines():
        metadata, name = row.split('\t'); _, kind, blob, size = metadata.split()
        if kind == 'blob': blobs[Path(name).name] = (blob, int(size))
    check('new immutable tag and artifact objects exactly match advertised five downloads', git('rev-parse', VERSION + '^{}') == SOURCE and git('rev-parse', 'codex/release-' + VERSION) == artifact_commit and all(blobs.get(a['name']) == (a['git_blob'], a['bytes']) for a in manifest['assets']) and git('rev-parse', artifact_commit + ':docs/releases/' + VERSION + '/release-manifest.json') == git('hash-object', '--no-filters', str(RELEASE / 'release-manifest.json')))
    check('all four prior frozen source and artifact refs retain exact identity', len(manifest['prior_freezes_unchanged']) == 4 and all(git('rev-parse', row['tag'] + '^{}') == row['source_commit'] and git('rev-parse', 'codex/release-' + row['tag']) == row['artifact_commit'] for row in manifest['prior_freezes_unchanged']))
    check('business sources and all111 migrations retain frozen147 bytes', not git('diff', '--name-only', SOURCE, '--', 'app/backend', 'app/web', 'app/mobile', 'app/packages', 'app/agent-runtime', 'app/skills', 'docs/skills') and not git('ls-files', '--others', '--exclude-standard', '--', 'app/backend', 'app/web', 'app/mobile', 'app/packages', 'app/agent-runtime', 'app/skills', 'docs/skills'))
    altered = git('diff', '--name-only', SOURCE, '--', 'docs/quality/evidence', 'docs/media/screenshots', 'docs/media/videos', 'docs/media/gifs').splitlines()
    check('all previous proof and media retain original bytes', all(name.startswith('docs/quality/evidence/release-candidate-148/') for name in altered))
    api_path = ROOT / 'docs/quality/evidence/release-candidate-144/github-release-api.json'; record(api_path)
    check('existing API account rejection stays retained without another request or Release-page claim', not manifest['github_release_page_created'] and git('rev-parse', SOURCE + ':' + api_path.relative_to(ROOT).as_posix()) == git('hash-object', '--no-filters', str(api_path)))
    reused = record(E / 'reused-evidence.json'); applicability = record(E / 'acceptance-applicability-check.json')
    check('applicability is classification only and retained test/media scope is explicit', reused['business_increment'] == 147 and reused['native_passing_records'] == 15 and reused['native_substantive_subcases'] == 13 and reused['selected_web_tests'] == reused['selected_mobile_tests'] == 7 and reused['runtime_increment'] == 145 and not reused['previous_tests_and_media_reexecuted'] and applicability['passed'] and applicability['business_cases_passed'] == 0)
    remote = record(E / 'remote-publication.json')
    check('authorized SSH publication is read back at all fifteen exact frozen refs', remote['passed'] and remote['verified_frozen_ref_count'] == 15 and remote['source_commit'] == SOURCE and remote['artifact_commit'] == artifact_commit and all(git('rev-parse', ref) == value for ref, value in remote['remote_frozen_refs'].items()) and remote['all_four_prior_freezes_unchanged'] and not remote['force_push_used'] and not remote['github_release_webpage_created'] and not remote['github_api_retried'])
    closed = True
    for port in (5189, 5190):
        with socket.socket() as probe:
            probe.settimeout(.2); closed = closed and probe.connect_ex(('127.0.0.1', port)) != 0
    check('no new browser or database fixture remains and eight Skills/23 work packages stay open', closed and not (APP / '.local/business-browser-fixture.json').exists() and len(list((APP / 'skills').glob('*/manifest.json'))) == 8 and all(read(p)['state'] == 'draft' for p in (APP / 'skills').glob('*/manifest.json')) and manifest['remaining_work_packages'] == 23 and not manifest['production_ready'] and not manifest['deployed'])
    report = {'passed': all(r['passed'] for r in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'increment': 148, 'verification_mode': 'frozen_release', 'checks': checks, 'records': records, 'retained': {'passed': checks[0]['passed'], 'business_increment': 147, 'full_backend_increment': 121, 'runtime_increment': 145, 'full_client_increment': 136, 'media_latest_increment': 147, 'previous_tests_and_media_reexecuted': False}, 'interaction_kind': 'release-candidate-148', 'interaction_source_sha256': digest(), 'verification_script_sha256': sha(Path(__file__)), 'fixture_only_adjustment': None, 'inspection_only_adjustment': None, 'current_android_apk_built': True, 'full_backend_suite_executed': False, 'formal_android_acceptance': False, 'new_browser_capture': False, 'current_development_database_schema': 100, 'schema_migration': 111, 'skipped_checks': manifest['deferred'], 'skip_authority': 'User authorized selected validation deferral for accelerated frozen delivery.', 'full_goal_complete': False, 'production_ready': False}
    if save: (E / 'final-source-check.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    return report

if __name__ == '__main__':
    r = verify(save=True); print(json.dumps({'passed': r['passed'], 'checks': len(r['checks']), 'failed': [v['name'] for v in r['checks'] if not v['passed']]})); raise SystemExit(0 if r['passed'] else 1)
