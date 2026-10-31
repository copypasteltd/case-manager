"""Verify frozen source/artifact identity and honest reuse, without replaying tests."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import socket
import subprocess

import build_android
import build_production_release as server
import subscription_portfolio_sources as original
from build_candidate_144 import APP, ROOT, E, VERSION, SOURCE, sha

RELEASE = ROOT / 'docs/releases' / VERSION
read = lambda p: json.loads(p.read_text(encoding='utf-8'))
HELPERS = ('build_candidate_144.py', 'inspect_candidate_144.py', 'seal_candidate_144.py', 'publish_candidate_144.py', 'verify_candidate_144.py', 'test_candidate_144_native.py')


def digest():
    h = hashlib.sha256(original.digest().encode())
    for name in HELPERS:
        h.update(name.encode() + b'\0' + (APP / 'scripts' / name).read_bytes())
    return h.hexdigest()


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT).decode('utf-8').strip()


def verify(save=False):
    checks, records = [], []

    def check(name, ok):
        checks.append({'name': name, 'passed': bool(ok)})

    def record(path):
        records.append({'file': path.relative_to(ROOT).as_posix(), 'sha256': sha(path)})
        return read(path)

    retained = record(E / 'retained-functional-proof.json')
    previous_path = ROOT / 'docs/quality/evidence/subscription-portfolio/final-source-check.json'
    previous = record(previous_path)
    check('Retained143 selected executed proof binds exact current business sources and original immutable records',
        retained['passed'] and previous['passed'] and retained == previous
        and len(retained['checks']) == 14 and all(r['passed'] for r in retained['checks'])
        and all(sha(ROOT / name) == value for name, value in retained['records'].items())
        and previous['interaction_source_sha256'] == original.digest()
        and previous['verification_script_sha256'] == sha(APP / 'scripts/verify_subscription_portfolio.py')
        and git('rev-parse', SOURCE + ':' + previous_path.relative_to(ROOT).as_posix())
            == git('hash-object', '--no-filters', str(previous_path)))
    import test_candidate_144_native as smoke
    native = record(E / 'native-smoke.json')
    expected = {'TestProjectCollaborationIntegration', 'TestSequentialApprovals', 'TestBusinessCostRecording',
        'TestCostPaymentRecording', 'TestUploadPromotionAndCommit', 'TestServiceStatus',
        'TestSubscriptionBilling', 'TestContractSignatureAndActivation'}
    actual = {r['Test'] for r in native['test_results'] if '/' not in r['Test'] and r['Action'] == 'pass'}
    check('Eight selected primary HTTP/PG suites actually execute at frozen source without skipped/failing results',
        native['passed'] and native['exit_code'] == 0 and native['source_unchanged']
        and native['source_sha256'] == native['source_after_sha256'] == smoke.digest(native['source_files'])
        and actual == expected and all(r['Action'] == 'pass' for r in native['test_results'])
        and native['native_pg_durable'] and not native['virtualization_used']
        and not (APP / '.local/release-candidate-144-native-cluster.json').exists()
        and read(E / 'native-cluster-closed.json')['passed'])
    manifest, build, seal = [record(p) for p in (RELEASE / 'release-manifest.json', RELEASE / 'server-build.json', E / 'seal.json')]
    check('Fresh clean-source Linux/Web/secretary package is schema109 with declared Node22',
        manifest['source']['commit'] == SOURCE and manifest['schema_version'] == 109
        and build['passed'] and build['source']['base_commit'] == SOURCE and not build['source']['working_tree_dirty']
        and build['migrations'] == 109 and build['tool_versions']['node'] == 'v22.23.4'
        and build['source']['inputs_sha256'] == server.digest(server.canonical(server.inputs()))
        and build['archive_inspection']['passed'] and all(r['exit_code'] == 0 for r in build['commands'])
        and manifest['primary_business_smoke']['record_sha256'] == sha(E / 'native-smoke.json')
        and manifest['primary_business_smoke']['top_level_suites'] == 8
        and manifest['primary_business_smoke']['passing_result_records'] == len(native['test_results']) == 95)
    mini = record(E / 'miniapps-build.json')
    check('Both fresh local miniapps bind exact unchanged current client source and sealed output',
        mini['passed'] and mini['source_unchanged'] and mini['source_commit'] == SOURCE
        and mini['source_sha256'] == original.digest(mini['source_files'])
        and mini['wrapper_sha256'] == sha(APP / 'scripts/build_candidate_144.py')
        and {r['target'] for r in mini['phases']} == {'weapp', 'tt'} and all(r['exit_code'] == 0 for r in mini['phases']))
    android, scope, inspection = [record(E / p) for p in ('build.json', 'android-build-scope.json', 'apk-inspection.json')]
    records.append({'file': (E / 'native-build.log').relative_to(ROOT).as_posix(), 'sha256': sha(E / 'native-build.log')})
    check('New development APK contains current actual JS/DEX and verifies signature without a device',
        android['passed'] and android['source_unchanged_during_build'] and android['source_sha256'] == build_android.sources()[0]
        and scope['passed'] and scope['source_unchanged'] and scope['source_sha256'] == original.digest(scope['source_files'])
        and scope['wrapper_sha256'] == sha(APP / 'scripts/build_candidate_144.py')
        and inspection['passed'] and all(r['passed'] for r in inspection['checks'])
        and inspection['build_record_sha256'] == sha(E / 'build.json')
        and inspection['inspection_script_sha256'] == sha(APP / 'scripts/inspect_candidate_144.py')
        and inspection['apk_sha256'] == android['artifacts']['apk']['sha256']
        and all(r['matches_actual_map'] and sha(ROOT / r['file']) == r['sha256'] for r in inspection['current_source_map_files'])
        and not inspection['device_executed'] and not inspection['production_release'])
    check('Gradle outcomes and original XML bytes/timestamps stay explicit; no fresh JVM claim',
        scope['test_count'] == 28 and len(scope['xml']) == 4
        and all(sha(ROOT / r['file']) == r['sha256'] for r in scope['xml'])
        and scope['cached'] == ('UP-TO-DATE' in scope['task'] or 'FROM-CACHE' in scope['task'])
        and not scope['standalone_jvm_rerun'] and not scope['real_device'] and not scope['virtualization'])
    check('Five bounded artifacts include all application source/docs and exact media; historical evidence stays in Git',
        seal['passed'] and len(seal['assets']) == 5 and seal['assets'] == manifest['assets']
        and seal['manifest_sha256'] == sha(RELEASE / 'release-manifest.json')
        and seal['server_receipt_sha256'] == sha(RELEASE / 'server-build.json')
        and seal['seal_script_sha256'] == sha(APP / 'scripts/seal_candidate_144.py')
        and seal['complete_source_blobs'] == manifest['source_archive']['packaged_git_blob_count']
        and manifest['source_archive']['complete_application_source']
        and manifest['source_archive']['all_packaged_blob_bytes_match_git_source']
        and manifest['source_archive']['media_zip_contains_all_frozen_media']
        and manifest['source_archive']['historical_evidence_remains_in_frozen_git']
        and seal['known_private_matches'] == seal['high_confidence_token_matches'] == 0
        and all(0 < a['bytes'] < 100 * 1024 * 1024 for a in manifest['assets']))
    publication = record(RELEASE / 'git-publication.json')
    check('Ref preparation preserves the user index and never overwrites existing refs',
        publication['publisher_script_sha256'] == sha(APP / 'scripts/publish_candidate_144.py')
        and not publication['force_push_used'] and not publication['user_index_modified'])
    artifact_commit = publication['artifact_commit']
    blobs = {}
    for row in git('ls-tree', '-r', '-l', artifact_commit, '--', 'docs/releases/' + VERSION).splitlines():
        metadata, name = row.split('\t')
        _, kind, blob, size = metadata.split()
        if kind == 'blob':
            blobs[Path(name).name] = (blob, int(size))
    check('New immutable tag and artifact Git objects match every advertised download',
        git('rev-parse', VERSION + '^{}') == SOURCE
        and git('rev-parse', 'codex/release-' + VERSION) == artifact_commit
        and all(blobs.get(a['name']) == (a['git_blob'], a['bytes']) for a in manifest['assets'])
        and git('rev-parse', artifact_commit + ':docs/releases/' + VERSION + '/release-manifest.json')
            == git('hash-object', '--no-filters', str(RELEASE / 'release-manifest.json')))
    check('Both previous frozen source/artifact refs remain immutable',
        git('rev-parse', 'v0.1.0-rc.1^{}') == 'a95c6afbd0e76684a64dc641c6abe598ad625277'
        and git('rev-parse', 'codex/release-v0.1.0-rc.1') == '8ddb3f7a98a24910af49ede2a6c8b8c744d50973'
        and git('rev-parse', 'v0.1.0-rc.2^{}') == '40cd118634117204e2e310395fac32b59af5cecc'
        and git('rev-parse', 'codex/release-v0.1.0-rc.2') == '467b2276c0d890691d1c06238e79bb60d5f0c665')
    check('All original migrations remain identical to frozen business source',
        not git('diff', '--name-only', SOURCE, '--', 'app/backend/migrations')
        and not git('ls-files', '--others', '--exclude-standard', '--', 'app/backend/migrations'))
    altered = git('diff', '--name-only', SOURCE, '--', 'docs/quality/evidence', 'docs/media/screenshots', 'docs/media/videos', 'docs/media/gifs').splitlines()
    check('Every earlier proof and public media remains unchanged; packaging does not replay business tests',
        all(name.startswith('docs/quality/evidence/release-candidate-144/') for name in altered)
        and not git('diff', '--name-only', SOURCE, '--', 'app/backend', 'app/web', 'app/mobile', 'app/packages', 'app/agent-runtime', 'app/skills', 'docs/skills')
        and not manifest['retained_not_reexecuted']['business_tests_reexecuted'])
    closed = True
    for port in (5189, 5190):
        with socket.socket() as probe:
            probe.settimeout(.2)
            closed = closed and probe.connect_ex(('127.0.0.1', port)) != 0
    check('No new browser/temporary fixture remains; eight Skills stay draft and23 packages open',
        closed and not (APP / '.local/business-browser-fixture.json').exists()
        and len(list((APP / 'skills').glob('*/manifest.json'))) == 8
        and all(read(p)['state'] == 'draft' for p in (APP / 'skills').glob('*/manifest.json'))
        and manifest['remaining_work_packages'] == 23 and not manifest['production_ready'] and not manifest['deployed'])
    report = {'passed': all(r['passed'] for r in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'increment': 144, 'verification_mode': 'frozen_release', 'checks': checks, 'records': records,
        'retained': {'passed': checks[0]['passed'], 'business_increment': 143, 'full_backend_increment': 121,
            'runtime_increment': 143, 'full_client_increment': 136, 'media_latest_increment': 143, 'previous_tests_and_media_reexecuted': False},
        'interaction_kind': 'release-candidate-144', 'interaction_source_sha256': digest(),
        'verification_script_sha256': sha(Path(__file__)), 'fixture_only_adjustment': None,
        'inspection_only_adjustment': None, 'current_android_apk_built': True,
        'full_backend_suite_executed': False, 'formal_android_acceptance': False,
        'new_browser_capture': False, 'current_development_database_schema': 100, 'schema_migration': 109,
        'skipped_checks': manifest['deferred'], 'skip_authority': 'User authorized selected validation deferral for accelerated candidate release.',
        'full_goal_complete': False, 'production_ready': False}
    if save:
        (E / 'final-source-check.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    return report


if __name__ == '__main__':
    result = verify(save=True)
    print(json.dumps({'passed': result['passed'], 'checks': len(result['checks']),
        'failed': [r['name'] for r in result['checks'] if not r['passed']]}))
    raise SystemExit(0 if result['passed'] else 1)
