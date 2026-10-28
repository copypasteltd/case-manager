"""Verify cost-independent execution, source-bound checks and honest acceptance scope."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/cost-free-execution'
PUBLISHED = '27a0d30aaa1ac80ce17c6703fce2eb7fb5e5848e'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def module(name):
    spec = importlib.util.spec_from_file_location('cost_scope_' + name, APP / 'scripts' / name)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def digest(names):
    h = hashlib.sha256()
    for name in names:
        h.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return h.hexdigest()


def main():
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    agent = APP / 'backend/internal/agent'
    admission = (agent / 'admission.go').read_text()
    execution = (agent / 'execution.go').read_text()
    budget = (agent / 'budget.go').read_text(encoding='utf-8')
    usage = (agent / 'execution_usage.go').read_text()
    check('Admission and execution have no monthly fee policy or reservation prerequisite',
        'reserveRunBudget' not in admission and 'RUNTIME_BUDGET_UNRESERVED' not in execution
        and 'AGENT_BUDGET_UNCONFIGURED' not in budget and 'agent_budget_policies' not in admission + execution)
    check('Technical capacity, authority, cancellation, deadline and usage validation remain',
        all(v in admission for v in ('RUN_CAPACITY_EXCEEDED', 'CONVERSATION_RUN_ACTIVE', 'pg_advisory_xact_lock'))
        and all(v in execution for v in ('RUNTIME_AUTHORITY_CHANGED', 'RUNTIME_CANCEL_REQUESTED', 'RUNTIME_DEADLINE_EXCEEDED', 'RUNTIME_CONFINEMENT_UNVERIFIED'))
        and 'validExecutionUsage' in usage and 'RUNTIME_USAGE_CONFLICT' in usage)
    legacy = sorted((APP / 'backend/migrations').glob('*.sql'))
    check('All78 original migrations retain exact published bytes;79 adds the direct tenant/run usage FK',
        len(legacy) == 79 and all(p.read_bytes() == subprocess.check_output(['git', 'show', PUBLISHED + ':app/backend/migrations/' + p.name], cwd=ROOT) for p in legacy[:-1])
        and 'FOREIGN KEY(org_id,run_id) REFERENCES agent_runs(org_id,id)' in legacy[-1].read_text())
    contract = read(ROOT / 'docs/development/openapi.json')
    retired = [contract['paths'][p]['post'] for p in ('/agent-budget/policies', '/agent-budget/entries/{id}/reconcile')]
    check('Legacy fee write contracts are deprecated with410 and no success response',
        all(o['deprecated'] and '410' in o['responses'] and not {'200', '201'} & set(o['responses']) for o in retired)
        and (APP / 'backend/internal/contract/openapi.json').read_bytes() == (ROOT / 'docs/development/openapi.json').read_bytes()
        and 'AI_COST_CONTROL_RETIRED' in budget)
    backend = read(E.parent / 'cost-free-execution-backend/result.json')
    backend_source = module('verify_backend_batches.py').digest()
    check('Every discovered current-source backend test passes with complete native provenance',
        backend['passed'] and backend['source_sha256'] == backend_source and backend['source_unchanged_during_tests']
        and backend['all_discovered_tests_executed'] and backend['discovered_top_level_tests'] == 189
        and len(backend['checks']) >= 1100 and all(r['Action'] == 'pass' for r in backend['checks']))
    cases = [r for r in backend['checks'] if r['Test'].startswith('TestAgentCostFreeExecution/')]
    check('All12 real execution/legacy scope cases pass, including no/zero/exhausted policy and immutable telemetry',
        len(cases) == 12 and all(r['Action'] == 'pass' for r in cases))
    clients = read(E / 'client-checks.json')
    check('Eight client type/test/build commands pass on exact current inputs',
        clients['passed'] and clients['source_unchanged'] and len(clients['checks']) == 8
        and clients['source_sha256'] == digest(clients['source_files'])
        and all(r['passed'] and sha(APP / '.local' / Path(r['raw_log']).name) == r['raw_log_sha256'] for r in clients['checks']))
    web, mobile, runtime = [read(E / name) for name in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    check('Web135/mobile248/runtime51 proofs bind their actual current sources',
        web['passed'] and web['source_sha256'] == digest(web['sources']) and web['tests']['numPassedTests'] == 135
        and mobile['passed'] and mobile['source_sha256'] == module('verify_mobile.py').digest()[0] and mobile['tests']['pass'] == 248
        and runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime.py').digest()[0] and runtime['tests']['pass'] == 51)
    check('Both real mini-program outputs retain20 pages without a device/emulator claim',
        all(row['page_count'] == 20 and sha(APP / 'mobile/dist' / target / 'app.json') == row['app_json_sha256'] for target, row in mobile['targets'].items()))
    build, apk, jvm = [read(E / name) for name in ('build.json', 'apk-inspection.json', 'android-test-scope.json')]
    check('Android source/build/APK bytes and18 fresh package inspections match',
        build['passed'] and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android.py').sources()[0]
        and sha(APP / build['artifacts']['apk']['path']) == build['artifacts']['apk']['sha256']
        and apk['passed'] and len(apk['checks']) == 18 and all(r['passed'] for r in apk['checks'])
        and not any(build[k] for k in ('device_executed', 'emulator_started', 'virtualization_used', 'production_release')))
    check('JVM26 scope declares actual execution/cache and original XML bytes',
        jvm['passed'] and jvm['tests'] == 26 and jvm['new_test_execution'] == (not jvm['gradle_cached'])
        and not jvm['device_executed'] and all(sha(E / jvm['xml_artifact_directory'] / name) == value for name, value in jvm['xml_sha256'].items()))
    feature = read(E / 'android-feature-inspection.json')
    check('Packaged current JS contains the archive and omits retired fee forms',
        feature['passed'] and all(r['passed'] for r in feature['checks'])
        and feature['apk_sha256'] == build['artifacts']['apk']['sha256'] and not feature['runtime_rendering_claimed'])
    linux = read(E / 'linux-build.json')
    check('Linux cross-build binds current backend without execution/deployment',
        linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_source
        and not linux['artifact_executed'] and not linux['deployed'])
    browser_checks = []
    interaction = module('cost_free_execution_sources.py').digest()
    for client in ('web', 'mobile'):
        result = read(E.parent / ('cost-free-execution-' + client) / 'result.json')
        browser_checks.append(result['passed'] and result['source_unchanged'] and result['build_source_unchanged']
            and result['build_source_sha256'] == interaction and result['build_source_after_sha256'] == interaction
            and result['owned_browser_closed'] and result['owned_review_server_stopped'] and result['isolated_database_cleanup_passed']
            and result['native_fixture_exit_code'] == 0 and len(result['checks']) == 4)
    check('Actual authenticated Web/H5 archive flows pass on current inputs and close every owned review resource', all(browser_checks))
    media = read(E / 'media.json')
    check('Two actual capture manifests produce byte-bound original-speed MP4 and bounded GIF artifacts',
        media['passed'] and len(media['recordings']) == 2 and all(
            sha(ROOT / r['capture_manifest']) == r['capture_manifest_sha256'] and r['capture_frame_count'] >= 5
            and all(sha(ROOT / a['file']) == a['sha256'] and (ROOT / a['file']).stat().st_size == a['bytes'] for a in r['artifacts']) for r in media['recordings']))
    previous = read(E / 'publication-before.json')
    check('All17 previous publication snapshots retain their exact Git bytes',
        previous['published_commit'] == PUBLISHED and len(previous['files']) == 17 and all(
            sha(ROOT / r['snapshot']) == r['sha256'] and (ROOT / r['snapshot']).read_bytes() == subprocess.check_output(['git', 'show', PUBLISHED + ':' + r['file']], cwd=ROOT) for r in previous['files']))
    retained = []
    for line in subprocess.check_output(['git', 'ls-tree', '-r', PUBLISHED, '--', 'docs/media'], cwd=ROOT).decode().splitlines():
        meta, name = line.split('\t', 1)
        if Path(name).suffix in ('.png', '.mp4', '.gif'):
            retained.append((name, meta.split()[2]))
    actual = subprocess.check_output(['git', 'hash-object', '--', *[name for name, _ in retained]], cwd=ROOT).decode().splitlines()
    check('All167 previous published media objects remain exact; new captures do not rewrite history',
        len(retained) == 167 and actual == [blob for _, blob in retained])
    audit = read(E.parent / 'dependency-maintenance/dependency-audit.json')
    check('Retained audit binds unchanged lock and keeps all80 findings/security gate open',
        sha(APP / 'package-lock.json') == audit['package_lock_sha256'] and audit['vulnerabilities']['total'] == 80
        and not audit['production_security_gate_passed'])
    dev = read(E / 'development-project-resources.json')
    upgrade = read(E / 'development-update.json')
    check('Native schema79 upgrade/inspection preserves existing rows, attests services and leaves no test DBs/external activation',
        dev['passed'] and dev['migration_count'] == 79 and dev['backend_source_sha256'] == backend_source
        and dev['database']['disposable_test_databases'] == 0 and dev['integrations_disabled'] and all(dev['ownership'].values())
        and upgrade['passed'] and upgrade['legacy_and_usage_rows_preserved'] and not upgrade['external_integration_started']
        and not dev['production_deployment'] and not upgrade['production_deployment'])
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    gap9 = next(r for r in scope['items'] if r['id'] == 'GAP-09')
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('Real Codex and23 whole work packages stay open; fee adaptation is a completed subtask and8 Skills remain draft',
        scope['counts']['active'] == 23 and scope['counts']['excluded'] == 5 and gap9['status'] == 'active'
        and 'COST-FREE-ADMISSION-100' in gap9['completed_adaptations'] and not scope['flags']['ai_cost_control_required']
        and scope['flags']['ai_secretary_required'] and scope['flags']['ai_execution_isolation_required']
        and len(skills) == 8 and all(r['state'] == 'draft' for r in skills))
    with socket.socket() as sock:
        sock.settimeout(.3)
        review_closed = sock.connect_ex(('127.0.0.1', 5189)) != 0
    check('No temporary review server or credential-bearing fixture remains',
        review_closed and not (APP / '.local/business-browser-fixture.json').exists())
    result = {'passed': all(r['passed'] for r in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
        'interaction_kind': 'cost_independent_execution_and_archive', 'interaction_source_sha256': interaction,
        'mobile_source_sha256': module('verify_mobile.py').digest()[0], 'backend_source_sha256': backend_source,
        'verification_script_sha256': sha(Path(__file__)), 'new_backend_test_execution': True, 'new_browser_capture': True,
        'full_goal_complete': False, 'production_ready': False, 'real_device': False, 'real_cloud': False, 'real_model': False,
        'real_telegram_network': False, 'real_wechat_or_douyin_network': False, 'virtualization_used': False}
    (E / 'final-source-check.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'checks': len(checks), 'failed': [r['name'] for r in checks if not r['passed']]}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
