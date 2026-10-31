"""Seal native artifacts plus complete application source/docs and public media.

Uses a private temporary Git index, never a checkout reset or the user's index.
Historical evidence stays in the immutable Git source tag, outside the compact
source convenience archive. Every application source blob must be present.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import sys
import tarfile
import zipfile

import build_production_release as server
import verify_repository_publication as publication
from build_candidate_144 import APP, ROOT, E, LOCAL, VERSION, SOURCE, sha

OUT = APP / '.local/frozen-releases' / (VERSION + '-assets-compact')
SERVER = APP / '.local/frozen-releases' / (VERSION + '-server-node22')
RELEASE = ROOT / 'docs/releases' / VERSION
read = lambda p: json.loads(p.read_text(encoding='utf-8'))


def git(*args, env=None, data=None):
    return subprocess.run(['git', *args], cwd=ROOT, env=env, input=data, capture_output=True, check=True).stdout


def write(path, value):
    assert not path.exists(), 'Never overwrite an earlier sealed result.'
    path.write_text(json.dumps(value, indent=2) + '\n', encoding='utf-8')


def main():
    assert not OUT.exists() and not (RELEASE / 'release-manifest.json').exists()
    b, clients, apk = [read(p) for p in (SERVER / 'build.json', E / 'miniapps-build.json', E / 'build.json')]
    inspection = read(E / 'apk-inspection.json')
    smoke = read(E / 'native-smoke.json')
    assert smoke['passed'] and smoke['source_unchanged'] and smoke['exit_code'] == 0
    assert all(r['Action'] == 'pass' for r in smoke['test_results'])
    assert b['passed'] and b['source']['base_commit'] == SOURCE and not b['source']['working_tree_dirty']
    assert b['tool_versions']['node'] == 'v22.23.4' and b['migrations'] == 109 and b['all_skills_remain_draft']
    assert b['source_files'] == server.inputs()
    assert clients['passed'] and clients['source_commit'] == SOURCE and apk['passed'] and inspection['passed']
    assert git('rev-parse', 'HEAD').decode().strip() == SOURCE
    OUT.mkdir()
    RELEASE.mkdir(parents=True, exist_ok=True)
    assets = []

    def copy(source, suffix):
        target = OUT / ('case-manager-' + VERSION + '-' + suffix)
        shutil.copyfile(source, target)
        return target

    native = copy(ROOT / b['archive'], 'linux-amd64.tar.gz')
    assert sha(native) == b['archive_sha256']
    mini = copy(ROOT / clients['archive'], 'miniapps.zip')
    assert sha(mini) == clients['archive_sha256']
    android = copy(APP / apk['artifacts']['apk']['path'], 'android-debug.apk')
    assert sha(android) == apk['artifacts']['apk']['sha256'] == inspection['apk_sha256']
    target = APP / '.local/frozen-releases' / (VERSION + '-inspection-compact')
    result = subprocess.run([sys.executable, '-B', str(APP / 'deploy/release/bundle.py'), 'inspect',
        '--archive', str(native), '--manifest-sha256', b['manifest_sha256'], '--output', str(target)],
        cwd=ROOT, capture_output=True, check=True)
    inspected = json.loads(result.stdout)
    assert inspected['passed']

    # Preserve every application source, lockfile, specification and guide.
    # Large historical evidence stays in Git; never archive ignored local data.
    entries = {}
    for row in git('ls-tree', '-rz', SOURCE).split(b'\0'):
        if not row:
            continue
        metadata, raw_name = row.split(b'\t', 1)
        mode, kind, blob = metadata.decode().split()
        assert kind == 'blob' and mode in ('100644', '100755')
        name = raw_name.decode('utf-8')
        assert not PurePosixPath(name).is_absolute() and '..' not in PurePosixPath(name).parts
        entries[name] = blob
    source_index = OUT.parent / (VERSION + '-source-compact.index')
    assert not source_index.exists()
    env = {**os.environ, 'GIT_INDEX_FILE': str(source_index)}
    git('read-tree', SOURCE, env=env)
    media_names = [p for p in entries if p.startswith('docs/media/')]
    evidence_names = [p for p in entries if p.startswith('docs/quality/evidence/')]
    git('update-index', '--force-remove', '-z', '--stdin', env=env,
        data=b''.join(p.encode() + b'\0' for p in media_names + evidence_names))
    tree = git('write-tree', env=env).decode().strip()
    source_zip = OUT / ('case-manager-' + VERSION + '-source.zip')
    media_zip = OUT / ('case-manager-' + VERSION + '-media.zip')
    git('archive', '--format=zip', '--output=' + str(source_zip), tree)
    git('archive', '--format=zip', '--output=' + str(media_zip), SOURCE, 'docs/media')

    known = publication.private_values()
    patterns = [re.compile(p) for p in (
        rb'-----BEGIN (?:RSA |EC |OPENSSH |DSA |ENCRYPTED )?PRIVATE KEY-----',
        rb'eyJ[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{15,}',
        rb'(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{50,})',
        rb'\bsk-(?:proj-|svcacct-)?[A-Za-z0-9_-]{32,}\b',
        rb'\b[0-9]{8,12}:AA[A-Za-z0-9_-]{30,40}\b', rb'\b(?:AKIA|ASIA)[A-Z0-9]{16}\b')]
    scanned, reconstructed = 0, set()

    def check(name, data):
        nonlocal scanned
        scanned += 1
        assert not any(value in data for value in known), 'Private generated value found; publication refused.'
        assert b'\0' in data[:8192] or not any(p.search(data) for p in patterns), 'Private key/token pattern found; publication refused.'

    with tarfile.open(native, 'r:gz') as archive:
        for member in archive:
            assert member.isfile()
            check(member.name, archive.extractfile(member).read())
    for path in (mini, android, source_zip, media_zip):
        with zipfile.ZipFile(path) as archive:
            assert archive.testzip() is None
            for member in archive.infolist():
                if member.is_dir():
                    continue
                name = member.filename
                assert not PurePosixPath(name).is_absolute() and '..' not in PurePosixPath(name).parts
                data = archive.read(member)
                check(name, data)
                if path in (source_zip, media_zip):
                    assert name in entries and name not in reconstructed
                    assert hashlib.sha1(b'blob ' + str(len(data)).encode() + b'\0' + data).hexdigest() == entries[name]
                    reconstructed.add(name)
    assert reconstructed == set(entries) - set(evidence_names)
    assert {p for p in reconstructed if p.startswith('app/')} == {p for p in entries if p.startswith('app/')}
    for path in (native, mini, android, source_zip, media_zip):
        assert 0 < path.stat().st_size < 100 * 1024 * 1024, 'Git asset exceeds size bound.'
        assets.append({'name': path.name, 'bytes': path.stat().st_size, 'sha256': sha(path),
            'git_blob': git('hash-object', '--no-filters', str(path)).decode().strip()})
    scope = read(E / 'android-build-scope.json')
    manifest = {'format': 1, 'version': VERSION, 'status': 'frozen_candidate',
        'created_at': datetime.now(timezone.utc).isoformat(), 'source': {'tag': VERSION,
            'commit': SOURCE, 'server_inputs_sha256': b['source']['inputs_sha256'], 'clean_at_server_build': True},
        'artifact_branch': 'codex/release-' + VERSION, 'schema_version': 109,
        'server_release_id': b['release_id'], 'server_manifest_sha256': b['manifest_sha256'], 'assets': assets,
        'source_archive': {'total_frozen_git_blobs': len(entries), 'packaged_git_blob_count': len(reconstructed),
            'complete_application_source': True, 'application_source_blobs': sum(p.startswith('app/') for p in entries),
            'source_zip_excludes': ['docs/media/', 'docs/quality/evidence/'],
            'media_zip_contains_all_frozen_media': True, 'historical_evidence_remains_in_frozen_git': True,
            'all_packaged_blob_bytes_match_git_source': True, 'compact_source_tree': tree},
        'android': {'development_only': True, 'formal_signed_release': False, 'content_checks': len(inspection['checks']),
            'current_map_modules': len(inspection['current_source_map_files']), 'native_test_task': scope['task'],
            'native_test_count': scope['test_count'], 'native_test_cached': scope['cached'],
            'new_native_test_execution': not scope['cached'], 'standalone_jvm_rerun': False},
        'client_profile': 'local; set actual HTTPS API/platform identities and rebuild before server use',
        'executed_this_increment': ['Linux amd64 cross-build, Web and secretary runtime packaging',
            'WeChat/Douyin local builds', 'Android development bundle/APK and native artifact inspection',
            'Server archive inspect, exact source archive reconstruction and expanded private-material scan',
            'Eight selected primary native HTTP/PG business suites; no full regression'],
        'primary_business_smoke': {'record': 'docs/quality/evidence/release-candidate-144/native-smoke.json',
            'record_sha256': sha(E / 'native-smoke.json'),
            'top_level_suites': sum('/' not in r['Test'] for r in smoke['test_results']),
            'passing_result_records': len(smoke['test_results']),
            'full_backend_regression': False, 'real_cloud_or_model': False},
        'retained_not_reexecuted': {'business_increment': 143, 'full_client_increment': 136, 'web_tests': 452, 'shared_mobile_tests': 537,
            'selected_native_http_pg_suites': 3, 'native_passing_records': 28, 'runtime_increment': 143,
            'runtime_tests': 56, 'full_backend_and_runtime_increment': 121,
            'screenshots_video_gif_latest_increment': 143, 'business_tests_reexecuted': False},
        'deferred': ['Full backend/runtime regression and additional browser journeys',
            'Persistent development PG100-to109 upgrade', 'Real Linux, production PG, OSS/R2, Codex, TG and platform acceptance',
            'Formal Android signature/publication and complete current-scope acceptance'],
        'excluded_by_user': ['Real-device verification', 'Network performance/live120GB special',
            'Historical migration/disaster recovery', 'AI fee controls'],
        'remaining_work_packages': 23, 'eight_skills_status': 'draft', 'linux_executed': False,
        'deployed': False, 'production_ready': False, 'github_release_page_created': False,
        'github_release_api_status': 'Not retried; previous stored account returned HTTP403/suspended. SSH delivery only.',
        'first_freeze_unchanged': {'tag': 'v0.1.0-rc.1', 'source_commit': 'a95c6afbd0e76684a64dc641c6abe598ad625277',
            'artifact_commit': '8ddb3f7a98a24910af49ede2a6c8b8c744d50973'},
        'previous_freeze_unchanged': {'tag': 'v0.1.0-rc.2', 'source_commit': '40cd118634117204e2e310395fac32b59af5cecc',
            'artifact_commit': '467b2276c0d890691d1c06238e79bb60d5f0c665'}}
    write(OUT / 'release-manifest.json', manifest)
    shutil.copyfile(OUT / 'release-manifest.json', RELEASE / 'release-manifest.json')
    sums = assets + [{'name': 'release-manifest.json', 'sha256': sha(OUT / 'release-manifest.json')}]
    (OUT / 'SHA256SUMS.txt').write_text(''.join(r['sha256'] + '  ' + r['name'] + '\n' for r in sums), encoding='ascii')
    shutil.copyfile(OUT / 'SHA256SUMS.txt', RELEASE / 'SHA256SUMS.txt')
    report = {k: b[k] for k in ('passed', 'release_id', 'manifest_sha256', 'archive_sha256', 'source',
        'files', 'bytes', 'migrations', 'all_skills_remain_draft', 'tool_versions', 'linux_executed', 'systemd_executed', 'deployed', 'production_ready')}
    report.update({'commands': [{k: r[k] for k in ('exit_code', 'elapsed_seconds', 'raw_log_sha256')} for r in b['commands']],
        'archive_inspection': inspected, 'seal_script_sha256': sha(Path(__file__)),
        'private_material_check': {'passed': True, 'archive_members': scanned,
            'known_private_value_count': len(known), 'matches': [], 'scope': 'Expanded bytes, known generated private values and high-confidence text patterns; no pixel OCR.'},
        'complete_source_blob_count': len(reconstructed), 'private_logs_or_databases_published': False})
    write(RELEASE / 'server-build.json', report)
    write(E / 'seal.json', {'passed': True, 'source_commit': SOURCE, 'assets': assets,
        'manifest_sha256': sha(RELEASE / 'release-manifest.json'), 'server_receipt_sha256': sha(RELEASE / 'server-build.json'),
        'seal_script_sha256': sha(Path(__file__)), 'expanded_members': scanned, 'complete_source_blobs': len(reconstructed),
        'known_private_matches': 0, 'high_confidence_token_matches': 0, 'production_ready': False})
    print(json.dumps({'passed': True, 'source_commit': SOURCE, 'source_blobs': len(reconstructed),
        'expanded_members': scanned, 'assets': assets}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
