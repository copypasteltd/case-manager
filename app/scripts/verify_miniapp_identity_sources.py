"""Bind increment97 to current code, actual local checks and exact retained media."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import socket
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/miniapp-identity'
PUBLISHED = 'de8739c0e13513e291df42206ac6e89f025c3f77'


def read(path): return json.loads(path.read_text(encoding='utf-8-sig'))
def sha(path): return hashlib.sha256(path.read_bytes()).hexdigest()
def module(name):
    spec = importlib.util.spec_from_file_location('identity_' + name, APP / 'scripts' / name)
    value = importlib.util.module_from_spec(spec); spec.loader.exec_module(value); return value
def digest(names):
    value = hashlib.sha256()
    for name in names: value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()
def old_bytes(name): return subprocess.check_output(['git', 'show', PUBLISHED + ':' + name], cwd=ROOT)


def main():
    checks = []
    def check(name, value): checks.append({'name': name, 'passed': bool(value)})
    backend = read(E.parent / 'miniapp-identity-backend/result.json')
    source = module('verify_backend_batches.py').digest()
    required = {'TestOfficialMiniappExchange', 'TestMiniappProviderFailureBoundaries', 'TestMiniappBindingLifecycle',
        'TestMiniappProofBoundaries', 'TestMiniappMFAAndRevocation', 'TestMiniappMFAEnrollmentAndVerify',
        'TestMiniappRefreshRevocationRace', 'TestMiniappBrowserFixture'}
    check('Every discovered native backend test passed on the exact final sources', backend['passed']
        and backend['source_sha256'] == source and backend['source_unchanged_during_tests']
        and backend['all_discovered_tests_executed'] and backend['native_database']
        and required <= {row['Test'] for row in backend['checks']} and all(row['Action'] == 'pass' for row in backend['checks']))
    batches = read(E.parent / 'miniapp-identity-backend/batch-progress.json')
    check('Final backend recorder and every native batch bind exact current scripts, sources and raw logs',
        backend['verification_script_sha256'] == sha(APP / 'scripts/verify_miniapp_identity_backend.py')
        and backend['batch_count'] == len(batches['batches']) == 10 and backend['parallel_batches'] == 2
        and batches['source_sha256'] == source and all(row['status'] == 'passed'
        and row['source_unchanged_during_tests'] and sha(APP / '.local' / Path(row['raw_output']).name) == row['raw_output_sha256']
        for row in batches['batches']))
    dns = read(E / 'dns-fixture-check.json')
    check('DNS denial fixture passed twenty fresh repetitions; runtime transport retains published bytes',
        dns['passed'] and dns['backend_source_sha256'] == source and dns['top_level_passes'] == 20 and dns['failures'] == 0
        and sha(APP / '.local' / Path(dns['private_raw_log']).name) == dns['private_raw_log_sha256']
        and (APP / 'backend/internal/statusbridge/transport.go').read_bytes() == old_bytes('app/backend/internal/statusbridge/transport.go'))
    migrations = sorted((APP / 'backend/migrations').glob('*.sql'))
    check('Additive migration78 preserves all77 published migration bytes', len(migrations) == 78
        and migrations[-1].name == '0078_miniapp_identity.sql'
        and all(path.read_bytes() == old_bytes(path.relative_to(ROOT).as_posix()) for path in migrations[:-1]))
    contract = read(ROOT / 'docs/development/openapi.json'); old = json.loads(old_bytes('docs/development/openapi.json'))
    verbs = {'get', 'post', 'put', 'patch', 'delete', 'head', 'options', 'trace'}
    check('OpenAPI298 paths/369 operations/647 schemas preserve every published operation and model', len(contract['paths']) == 298
        and sum(k in verbs for p in contract['paths'].values() for k in p) == 369 and len(contract['components']['schemas']) == 647
        and all(contract['paths'][key] == value for key, value in old['paths'].items())
        and all(contract['components']['schemas'][key] == value for key, value in old['components']['schemas'].items())
        and (ROOT / 'docs/development/openapi.json').read_bytes() == (APP / 'backend/internal/contract/openapi.json').read_bytes())
    generated = (APP / 'packages/contracts/src/api.d.ts').read_bytes()
    subprocess.run(['npm.cmd', 'run', 'contract'], cwd=APP, capture_output=True, check=True)
    check('Shared generated types are reproducible', (APP / 'packages/contracts/src/api.d.ts').read_bytes() == generated)
    linux = read(E / 'linux-build.json')
    check('Current Linux API is cross-compiled without execution or deployment', linux['passed']
        and linux['source_sha256'] == linux['source_after_sha256'] == source and not linux['artifact_executed'] and not linux['deployed'])
    clients = read(E / 'client-checks.json')
    check('All eight fresh client commands bind complete source and exact private raw logs', clients['passed']
        and clients['source_unchanged'] and clients['source_sha256'] == clients['source_after_sha256'] == digest(clients['source_files'])
        and len(clients['checks']) == 8 and all(row['passed'] and row['exit_code'] == 0
        and sha(APP / '.local' / Path(row['raw_log']).name) == row['raw_log_sha256'] for row in clients['checks']))
    web, mobile, runtime = [read(E / name) for name in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    check('Current135 Web/248 Taro tests and two20-page mini-program builds passed', web['passed']
        and web['tests']['numPassedTests'] == 135 and mobile['passed'] and mobile['tests']['pass'] == 248
        and mobile['source_sha256'] == module('verify_mobile.py').digest()[0] and mobile['targets']['weapp']['page_count'] == mobile['targets']['tt']['page_count'] == 20)
    check('Current51 runtime tests, build and eight draft Skills remain source-bound', runtime['status'] == 'passed'
        and runtime['tests']['pass'] == 51 and runtime['source_sha256'] == module('verify_agent_runtime.py').digest()[0]
        and all(read(path)['state'] == 'draft' for path in (APP / 'skills').glob('*/manifest.json'))
        and len(list((APP / 'skills').glob('*/manifest.json'))) == 8)
    interaction = module('miniapp_identity_sources.py').digest()
    captures = []
    for client in ('web', 'mobile'):
        folder = E.parent / ('miniapp-identity-' + client); record = read(folder / 'result.json'); manifest = read(folder / 'source-manifest.json')
        valid = record['passed'] and record['source_unchanged'] and record['build_source_unchanged']
        valid = valid and record['source_sha256'] == record['source_after_sha256'] == record['build_source_sha256'] == record['build_source_after_sha256'] == manifest['sha256'] == interaction
        valid = valid and digest(manifest['files']) == interaction and record['owned_browser_closed'] and record['browser_contexts_created'] == record['browser_contexts_closed']
        valid = valid and record['isolated_database_cleanup_passed'] and record['native_fixture_exit_code'] == 0 and record['owned_review_server_stopped']
        valid = valid and not record['real_wechat_or_douyin_exchange'] and not record['real_device'] and not record['errors'] and not record['server_errors']
        for item in record['screenshots']:
            target = ROOT / ('docs/media/screenshots/miniapp-identity-' + client + '-' + item['file'])
            valid = valid and sha(folder / item['file']) == item['sha256'] and target.read_bytes() == (folder / item['file']).read_bytes()
        check('Actual ' + client + ' binding UI, lost-response recovery, screenshots and cleanup match final complete sources', valid)
        captures.append(record)
    media = read(E / 'media.json')
    check('Two actual silent viewport recordings have matching source, manifests and MP4/GIF bytes', media['passed']
        and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_miniapp_identity_media.py') and len(media['recordings']) == 2
        and all(row['source_sha256'] == interaction and sha(ROOT / row['capture_manifest']) == row['capture_manifest_sha256']
        and all(sha(ROOT / item['file']) == item['sha256'] and (ROOT / item['file']).stat().st_size == item['bytes'] for item in row['artifacts']) for row in media['recordings']))
    retained = []
    for line in subprocess.check_output(['git', 'ls-tree', '-r', PUBLISHED, '--', 'docs/media'], cwd=ROOT).decode().splitlines():
        _, name = line.split('\t', 1)
        if Path(name).suffix.lower() in ('.png', '.mp4', '.gif'): retained.append((ROOT / name).read_bytes() == old_bytes(name))
    check('All90 published PNG and33 MP4/33 GIF retain exact original bytes', len(retained) == 156 and all(retained))
    build, inspection, scope = [read(E / name) for name in ('build.json', 'apk-inspection.json', 'android-test-scope.json')]
    check('Current native Android APK and18 package inspections are source-bound', build['passed']
        and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android.py').sources()[0]
        and inspection['passed'] and len(inspection['checks']) == 18 and all(row['passed'] for row in inspection['checks'])
        and sha(APP / build['artifacts']['apk']['path']) == build['artifacts']['apk']['sha256'])
    check('JVM26 results distinguish real execution from Gradle cache; no device claimed', scope['passed'] and scope['tests'] == 26
        and scope['new_test_execution'] == (not scope['gradle_cached']) and not scope['device_executed']
        and sha(E / 'native-build.log') == scope['native_log_sha256'])
    previous = read(E / 'publication-before.json')
    check('Prior bilingual guides, requirements, gap ledger and baseline have exact published snapshots', previous['published_commit'] == PUBLISHED
        and all(sha(ROOT / row['snapshot']) == row['sha256'] and (ROOT / row['snapshot']).read_bytes() == old_bytes(row['source']) for row in previous['files']))
    audit = read(E.parent / 'dependency-hardening/dependency-audit.json')
    check('The unchanged dependency lock retains the original open production security gate', (APP / 'package-lock.json').read_bytes() == old_bytes('app/package-lock.json')
        and audit['production_security_gate_passed'] is False)
    dev, update = [read(E / name) for name in ('development-project-resources.json', 'development-update.json')]
    check('Owned native API/workers use schema78 and final sources with all external providers disabled', dev['passed']
        and dev['migration_count'] == 78 and dev['backend_source_sha256'] == source and dev['worker_sources_match']
        and all(dev['ownership'].values()) and dev['database']['disposable_test_databases'] == 0
        and dev['integrations_disabled'] and update['passed'] and update['source_unchanged']
        and not update['external_integration_started'] and not update['production_deployment'])
    gaps = (ROOT / 'docs/delivery/03-goal-gap-assessment.md').read_text(encoding='utf-8')
    check('All28 stable gap IDs remain, with actual platform/device acceptance explicitly open', set(re.findall(r'\| (GAP-\d{2}) \|', gaps)) == {f'GAP-{n:02d}' for n in range(1, 29)}
        and '真实AppID' in gaps and '真机' in gaps)
    with socket.socket() as sock:
        sock.settimeout(.2); unused = sock.connect_ex(('127.0.0.1', 5189)) != 0
    check('No owned review server or credential-bearing fixture remains', unused and not (APP / '.local/business-browser-fixture.json').exists())
    result = {'passed': all(row['passed'] for row in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
        'interaction_kind': 'native_identity_and_browser', 'interaction_source_sha256': interaction,
        'backend_source_sha256': source, 'mobile_source_sha256': module('verify_mobile.py').digest()[0],
        'verification_script_sha256': sha(Path(__file__)), 'full_goal_complete': False, 'production_ready': False,
        'real_wechat_or_douyin_exchange': False, 'real_device': False, 'real_cloud': False, 'real_model': False, 'real_telegram_network': False}
    (E / 'final-source-check.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'checks': len(checks), 'failed': [row['name'] for row in checks if not row['passed']]}))
    return 0 if result['passed'] else 1


if __name__ == '__main__': raise SystemExit(main())
