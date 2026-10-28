"""Verify current OA prose evidence without upgrading fixture/cache scope to production."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/oa-prose'
PUBLISHED = 'ed7049fdf52f5dca177f61f9fa945ca4d08f2d73'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def module(name):
    spec = importlib.util.spec_from_file_location('oa_prose_scope_' + name, APP / 'scripts' / name)
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

    migrations = sorted((APP / 'backend/migrations').glob('*.sql'))
    check('Original migrations1-79 keep published bytes and80 is an append-only upgrade',
        len(migrations) == 80 and all(p.read_bytes() == subprocess.check_output(
            ['git', 'show', PUBLISHED + ':app/backend/migrations/' + p.name], cwd=ROOT) for p in migrations[:-1]))
    schema = migrations[-1].read_text(encoding='utf-8')
    check('Database retains immutable tenant/run drafts and normal-command adoption receipts',
        all(value in schema for value in ('agent_oa_text_drafts', 'agent_oa_text_adoptions',
            'agent_oa_text_draft_immutable', 'agent_oa_text_adoption_immutable',
            'FOREIGN KEY(org_id,run_id)', '/internal/agent-oa-text-adopt/', 'source_snapshots')))
    contract = read(ROOT / 'docs/development/openapi.json')
    check('Generated API includes human adoption and prose contracts with embedded parity',
        len(contract['paths']) == 299 and len(contract['components']['schemas']) == 651
        and '/runs/{id}/oa-text-adoption' in contract['paths']
        and 'AgentOATextAdopt' in contract['components']['schemas']
        and (APP / 'backend/internal/contract/openapi.json').read_bytes() == (ROOT / 'docs/development/openapi.json').read_bytes())

    backend = read(E.parent / 'oa-prose-backend/result.json')
    backend_source = module('verify_backend_batches.py').digest()
    check('Every discovered current-source backend test passes with native provenance',
        backend['passed'] and backend['source_sha256'] == backend_source
        and backend['source_unchanged_during_tests'] and backend['all_discovered_tests_executed']
        and backend['discovered_top_level_tests'] == 190 and len(backend['checks']) >= 1190
        and all(row['Action'] == 'pass' for row in backend['checks']))
    first = read(E.parent / 'oa-prose-backend/attempt01/result.json')
    progress = read(E.parent / 'oa-prose-backend/attempt01/batch-progress.json')
    retries = backend['retries']
    check('Failed and incomplete batches remain original;2 failed and6 missing parents have exact serial provenance',
        not first['passed'] and not first['all_discovered_tests_executed']
        and first['source_sha256'] == progress['source_sha256'] == backend_source
        and sha(E.parent / 'oa-prose-backend/attempt01/result.json') == backend['first_attempt']['sha256']
        and len(retries) == 8 and sum(row['previously_missing'] for row in retries) == 6
        and {(row['Package'], row['Test']) for row in backend['checks'] if '/' not in row['Test']}
            == {tuple(row) for row in progress['expected_tests']}
        and all(row['passed'] and row['source_unchanged'] and row['source_sha256'] == backend_source
            and sha(APP / '.local' / Path(row['raw_output']).name) == row['raw_output_sha256'] for row in retries)
        and all(sha(APP / '.local' / Path(row['raw_output']).name) == row['raw_output_sha256'] for row in progress['batches']))
    prose = [row for row in backend['checks'] if row['Test'].startswith('TestAgentOARequests/')
        and ('_prose_' in row['Test'] or '/prose_' in row['Test'])]
    boundary = [row for row in backend['checks'] if row['Test'] == 'TestOAProseMCPBoundary']
    check('Eight prose journeys and the actual MCP allowlist pass, including observed-source withdrawal and atomic rollback',
        len(prose) == 8 and len(boundary) == 1 and all(row['Action'] == 'pass' for row in prose + boundary))
    clients = read(E / 'client-checks.json')
    check('Eight fresh client commands bind current sources and their original logs',
        clients['passed'] and clients['source_unchanged'] and len(clients['checks']) == 8
        and clients['source_sha256'] == digest(clients['source_files'])
        and all(row['passed'] and sha(APP / '.local' / Path(row['raw_log']).name) == row['raw_log_sha256'] for row in clients['checks']))
    web, mobile, runtime = [read(E / name) for name in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    mobile_source = module('verify_mobile.py').digest()[0]
    check('Web142/mobile255/runtime52 test and build proofs match current inputs',
        web['passed'] and web['source_sha256'] == digest(web['sources']) and web['tests']['numPassedTests'] == 142
        and mobile['passed'] and mobile['source_sha256'] == mobile_source and mobile['tests']['pass'] == 255
        and runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime.py').digest()[0]
        and runtime['tests']['pass'] == 52)
    check('Both mini-program builds retain20 actual pages without device or platform claims',
        len(mobile['targets']) == 2 and all(row['page_count'] == 20
            and sha(APP / 'mobile/dist' / target / 'app.json') == row['app_json_sha256'] for target, row in mobile['targets'].items()))

    build, apk, jvm, feature = [read(E / name) for name in
        ('build.json', 'apk-inspection.json', 'android-test-scope.json', 'android-feature-content.json')]
    check('Current Android bundle/APK and18 package inspections are source-bound',
        build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android.py').sources()[0]
        and all(sha(APP / row['path']) == row['sha256'] for row in build['artifacts'].values())
        and apk['passed'] and apk['apk_sha256'] == build['artifacts']['apk']['sha256']
        and len(apk['checks']) == 18 and all(row['passed'] for row in apk['checks'])
        and not any(build[key] for key in ('device_executed', 'emulator_started', 'virtualization_used', 'production_release')))
    check('Four checks inspect the actual packaged prose controls and selected-field protocol',
        feature['passed'] and len(feature['checks']) == 4 and all(row['passed'] for row in feature['checks'])
        and feature['apk_sha256'] == build['artifacts']['apk']['sha256']
        and feature['bundle_sha256'] == build['artifacts']['bundle']['sha256'] and not feature['artifact_executed'])
    check('JVM scope honestly records execution/cache and exact XML/log bytes',
        jvm['passed'] and jvm['tests'] == 26 and jvm['new_test_execution'] == (not jvm['gradle_cached'])
        and not jvm['device_executed'] and sha(E / 'native-build.log') == jvm['native_log_sha256']
        and all(sha(E / jvm['xml_artifact_directory'] / name) == value for name, value in jvm['xml_sha256'].items()))
    linux = read(E / 'linux-build.json')
    check('Linux cross-build binds current backend without execution or deployment',
        linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_source
        and sha(APP / '.local/oa-prose-platform-linux-amd64') == linux['artifact']['sha256']
        and not linux['artifact_executed'] and not linux['deployed'] and not linux['virtualization_used'])

    interaction = module('oa_prose_sources.py').digest()
    browsers = []
    for client in ('web', 'mobile'):
        folder = E.parent / ('oa-prose-' + client)
        result, manifest, capture = [read(folder / name) for name in ('result.json', 'source-manifest.json', 'capture.json')]
        browsers.append(result['passed'] and result['source_unchanged'] and result['build_source_unchanged']
            and result['source_sha256'] == manifest['sha256'] == digest(manifest['files'])
            and result['build_source_sha256'] == result['build_source_after_sha256'] == interaction
            and result['owned_browser_closed'] and result['browser_contexts_closed'] == 1
            and result['owned_review_server_stopped'] and result['isolated_database_cleanup_passed']
            and result['native_fixture_exit_code'] == 0 and len(result['checks']) == 3
            and not result['console_errors'] and not result['server_errors']
            and capture['synthetic_data'] and {row['width'] for row in capture['layouts']} == {390, 768, 1440}
            and {row['kind'] for row in capture['adoptions']} == {'contract', 'purchase'}
            and all(row['operation_confirmed'] and row['new_business_version'] == 2
                and not row['approval_submitted'] for row in capture['adoptions']))
    check('Authenticated Web and Taro H5 perform real human adoption and claim intent flows and close owned resources', all(browsers))
    media = read(E / 'media.json')
    check('Two real-pixel capture manifests bind four new MP4/GIF artifacts',
        media['passed'] and len(media['recordings']) == 2 and all(
            sha(ROOT / row['capture_manifest']) == row['capture_manifest_sha256'] and row['capture_frame_count'] >= 20
            and row['build_source_sha256'] == interaction
            and all(sha(ROOT / item['file']) == item['sha256'] and (ROOT / item['file']).stat().st_size == item['bytes']
                for item in row['artifacts']) for row in media['recordings']))
    visual = read(E / 'visual-review.json')
    check('Four published before/after screenshots retain reviewed bytes and contain synthetic data',
        visual['passed'] and len(visual['screenshots']) == 4 and visual['synthetic_data']
        and all(sha(ROOT / row['file']) == row['sha256'] for row in visual['screenshots']))
    reused = read(E / 'reused-evidence.json')
    blobs = subprocess.check_output(['git', 'hash-object', '--', *[row['file'] for row in reused['media']]], cwd=ROOT).decode().splitlines()
    check('All173 previously published media and retained audit/JVM records remain byte-exact',
        reused['published_commit'] == PUBLISHED and len(reused['media']) == 173
        and blobs == [row['git_blob'] for row in reused['media']]
        and all(sha(ROOT / row['file']) == row['sha256'] for row in reused['media'] + reused['records']))
    audit = read(E.parent / 'dependency-maintenance/dependency-audit.json')
    check('Unchanged lock retains the original80-finding audit with the security gate open',
        sha(APP / 'package-lock.json') == audit['package_lock_sha256']
        and audit['vulnerabilities']['total'] == 80 and not audit['production_security_gate_passed'])

    cleanup, stopped = [read(E / name) for name in ('fixture-cleanup.json', 'interrupted-fixture-cleanup.json')]
    old_dev = read(E / 'attempts/development-before-interrupted-fixture-cleanup/development-project-resources.json')
    check('Four exact owned fixtures have explicit cleanup provenance; the initial failed environment inspection remains',
        cleanup['passed'] and len(cleanup['targets']) == 3
        and all(row['removed'] and row['invalid_before'] and row['connections_before'] == 0
            and row['drop_log_time_matches'] for row in cleanup['targets'])
        and stopped['passed'] and stopped['removed'] and stopped['creation_within_interrupted_case']
        and stopped['standard_synthetic_seed_verified'] and stopped['commercial_records_empty']
        and stopped['source_migration_ledger_matched'] and not stopped['forced_disconnect']
        and not cleanup['postgres_restarted'] and not stopped['postgres_restarted']
        and not cleanup['production_database_touched'] and not stopped['production_database_touched']
        and not old_dev['passed'] and old_dev['database']['disposable_test_databases'] == 1)

    dev, upgrade = [read(E / name) for name in ('development-project-resources.json', 'development-update.json')]
    check('Native schema80 and exact owned services pass with no disposable databases or external activation',
        dev['passed'] and dev['migration_count'] == 80 and dev['backend_source_sha256'] == backend_source
        and dev['database']['disposable_test_databases'] == 0 and dev['integrations_disabled']
        and all(dev['ownership'].values()) and all(dev['miniapp_metadata'].values())
        and upgrade['passed'] and upgrade['migration_count'] == 80 and upgrade['legacy_and_usage_rows_preserved']
        and not upgrade['external_integration_started'] and not dev['production_deployment'] and not upgrade['production_deployment'])
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    gap12 = next(row for row in scope['items'] if row['id'] == 'GAP-12')
    skills = [read(path) for path in (APP / 'skills').glob('*/manifest.json')]
    oa = read(APP / 'skills/oa-assist/manifest.json')
    check('Selected prose is a completed subtask;23 full packages and8 draft Skills remain open',
        scope['counts']['active'] == 23 and scope['counts']['excluded'] == 5 and gap12['status'] == 'active'
        and 'OA-PROSE-101' in gap12['completed_adaptations'] and not scope['flags']['ai_cost_control_required']
        and scope['flags']['ai_secretary_required'] and scope['flags']['ai_execution_isolation_required']
        and len(skills) == 8 and all(row['state'] == 'draft' for row in skills)
        and oa['version'] == '1.3.0')
    with socket.socket() as sock:
        sock.settimeout(.3)
        review_closed = sock.connect_ex(('127.0.0.1', 5189)) != 0
    check('No temporary review listener or credential-bearing fixture remains',
        review_closed and not (APP / '.local/business-browser-fixture.json').exists())

    result = {'passed': all(row['passed'] for row in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'checks': checks, 'interaction_kind': 'oa_prose_drafting_and_human_adoption',
        'interaction_source_sha256': interaction, 'mobile_source_sha256': mobile_source,
        'backend_source_sha256': backend_source, 'verification_script_sha256': sha(Path(__file__)),
        'new_backend_test_execution': True, 'new_browser_capture': True, 'full_goal_complete': False,
        'production_ready': False, 'real_device': False, 'real_cloud': False, 'real_model': False,
        'real_telegram_network': False, 'real_wechat_or_douyin_network': False, 'virtualization_used': False}
    (E / 'final-source-check.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'checks': len(checks), 'failed': [row['name'] for row in checks if not row['passed']]}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
