"""Run source-bound focused checks while retaining the last published full regression.

This does not turn unexecuted cases, a retained APK or platform acceptance into
current passes. The verification plan is versioned with the implementation.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import socket
import subprocess
import sys
import time

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def plan(path=None):
    path = path or read(ROOT / 'docs/quality/current-functional-baseline.json')['incremental_configuration']
    file = (ROOT / path).resolve()
    assert file.is_relative_to(ROOT / 'docs/quality')
    return read(file)


def sources(config):
    scopes = ['backend', 'web', 'mobile', 'agent-runtime', 'skills', 'packages', 'package.json', 'package-lock.json', *config['helper_sources']]
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', *scopes], cwd=APP).decode('utf-8').split('\0')
    return sorted(set(name for name in names if name and (APP / name).is_file()) | {'../' + config['configuration_file']})


def digest(names=None, config=None, substitutions=None):
    config = config or plan()
    value = hashlib.sha256()
    for name in names or sources(config):
        data = substitutions[name] if substitutions and name in substitutions else (APP / name).read_bytes()
        value.update(name.encode() + b'\0' + data + b'\0')
    return value.hexdigest()


def fixture_adjustment(config):
    """Allow only the disclosed MFA setup call in an opt-in browser fixture.

    Reconstruct the actual earlier execution inputs, rather than relabelling an
    old combined digest as current. Every production byte and selected test
    body must remain identical; command/acceptance changes are not permitted.
    """
    adjustment = config.get('fixture_only_adjustment')
    if not adjustment:
        return None
    name = 'backend/internal/httpapi/operations_workbench_integration_test.go'
    before_file = ROOT / adjustment['before_file']
    before_plan = ROOT / adjustment['before_plan']
    assert before_file.resolve().is_relative_to(directory(config)) and before_plan.resolve().is_relative_to(directory(config))
    before = before_file.read_bytes()
    marker = b'func TestOperationsWorkbenchBrowserFixture(t *testing.T) {'
    needle = b'\toperationsRole(t, f, f.alice, "status_operator")\n\tservice := operationsService(t, f, committed.File.ProjectID)'
    replacement = b'\toperationsRole(t, f, f.alice, "status_operator")\n\tf.aliceToken = memberMFAToken(f, "alice", "android")\n\tservice := operationsService(t, f, committed.File.ProjectID)'
    assert before.count(marker) == before.count(needle) == 1 and before.index(needle) > before.index(marker)
    assert (APP / name).read_bytes() == before.replace(needle, replacement, 1)
    assert read(before_plan) == {key:value for key,value in config.items() if key != 'fixture_only_adjustment'}
    substitutions = {name:before, '../'+config['configuration_file']:before_plan.read_bytes()}
    return {'execution_source_sha256':digest(config=config, substitutions=substitutions),
        'adjustment':'One MFA enrollment/verification call in the opt-in browser fixture only; production inputs, executed test bodies and commands unchanged.',
        'snapshots':[{'file':file.relative_to(ROOT).as_posix(),'sha256':sha(file)} for file in (before_file,before_plan)]}


def directory(config):
    target = ROOT / config['evidence_directory']
    assert target.resolve().is_relative_to(ROOT / 'docs/quality/evidence')
    target.mkdir(parents=True, exist_ok=True)
    return target


def native_command(command):
    node = APP / '.tools/node-v22.23.4-win-x64/node.exe'
    aliases = {'python': [sys.executable], 'npm': [str(node), str(node.parent / 'node_modules/npm/bin/npm-cli.js')], 'node': [str(node)]}
    return aliases[command[0]] + command[1:]


def run(config, name):
    check = next(item for item in config['commands'] if item['id'] == name)
    evidence = directory(config)
    report = evidence / (name + '.json')
    if report.exists():
        raise SystemExit('Preserve the existing result before a new attempt: ' + str(report))
    private = APP / '.local/scoped-increments' / config['name']
    private.mkdir(parents=True, exist_ok=True)
    log = private / (name + '.log')
    before = digest(config=config)
    started, clock = datetime.now(timezone.utc).isoformat(), time.monotonic()
    env = {**os.environ, 'PYTHONUTF8': '1', 'CI': 'true', 'TARO_CHECK_UPDATE': 'false'}
    env.update(check.get('env', {}))
    env['PATH'] = str(APP / '.tools/node-v22.23.4-win-x64') + os.pathsep + env.get('PATH', '')
    command = native_command(check['command'])
    with log.open('wb') as output:
        result = subprocess.run(command, cwd=APP / check.get('cwd', ''), env=env, stdout=output, stderr=subprocess.STDOUT,
            creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    parsed, cases, parser_error = {}, [], None
    try:
        text = log.read_text(encoding='utf-8', errors='replace')
        if check['parser'] == 'go_json':
            rows = [json.loads(line) for line in text.splitlines() if line.startswith('{')]
            cases = [{k: row[k] for k in ('Action', 'Package', 'Test') if k in row} for row in rows if row.get('Test') and row['Action'] in ('pass', 'fail', 'skip')]
            top = [row['Test'] for row in cases if '/' not in row['Test'] and row['Action'] == 'pass']
            parsed = {'top_level_tests': top, 'pass': sum(row['Action'] == 'pass' for row in cases), 'fail': sum(row['Action'] == 'fail' for row in cases), 'skip': sum(row['Action'] == 'skip' for row in cases)}
            assert set(top) == set(check['selected_tests']) and parsed['fail'] == parsed['skip'] == 0
        elif check['parser'] == 'vitest_json':
            full = read(APP / check['json_output'])
            parsed = {key: full[key] for key in ('numTotalTests', 'numPassedTests', 'numFailedTests', 'numPendingTests', 'success')}
            assert parsed['success'] and parsed['numFailedTests'] == parsed['numPendingTests'] == 0 and parsed['numPassedTests'] >= check['minimum_tests']
        elif check['parser'] == 'tap':
            for key in ('tests', 'pass', 'fail', 'cancelled', 'skipped'):
                values = [int(value) for value in re.findall(r'^# ' + key + r' (\d+)\s*$', text, re.M)]
                assert values, key
                parsed[key] = sum(values)
            assert parsed['tests'] == parsed['pass'] >= check['minimum_tests'] and parsed['fail'] == parsed['cancelled'] == parsed['skipped'] == 0
        else:
            assert check['parser'] == 'exit'
    except (ValueError, KeyError, AssertionError, OSError) as error:
        parser_error = type(error).__name__
    after = digest(config=config)
    record = {'passed': result.returncode == 0 and not parser_error and before == after, 'id': name,
        'started_at': started, 'finished_at': datetime.now(timezone.utc).isoformat(), 'elapsed_seconds': round(time.monotonic()-clock, 3),
        'command': check['command'], 'cwd': check.get('cwd', ''), 'exit_code': result.returncode, 'parser': check['parser'],
        'source_sha256': before, 'source_after_sha256': after, 'source_unchanged': before == after,
        'tests': parsed, 'cases': cases, 'private_log_sha256': sha(log), 'parser_error': parser_error,
        'runner_script_sha256': sha(Path(__file__)), 'scope': check['scope'], 'real_device': False, 'virtualization_used': False}
    report.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'id': name, 'passed': record['passed'], 'tests': parsed, 'elapsed_seconds': record['elapsed_seconds']}), flush=True)
    return 0 if record['passed'] else 1


def retained(config):
    commit = config['retained_base_commit']
    assert re.fullmatch(r'[a-f0-9]{40}', commit)
    rows = []
    for line in subprocess.check_output(['git', 'ls-tree', '-r', commit, '--', 'docs/quality/evidence', 'docs/media'], cwd=ROOT).decode('utf-8').splitlines():
        meta, name = line.split('\t', 1)
        if name.startswith('docs/media/') and Path(name).suffix.lower() not in ('.png', '.gif', '.mp4'):
            continue
        rows.append((name, meta.split()[2]))
    # All listed repository files retain raw bytes (-text, no content filter).
    # Compute the exact SHA-1 Git blob identity locally. A large Windows pipe
    # must not couple the input path list and output hashes into a blocked
    # subprocess; no shell argument limit or line-ending conversion is used.
    assert all(len(blob)==40 for _,blob in rows), 'Expected this repository SHA-1 object format.'
    def blob_identity(name):
        file = ROOT / name
        value = hashlib.sha1(b'blob '+str(file.stat().st_size).encode()+b'\0', usedforsecurity=False)
        with file.open('rb') as stream:
            for block in iter(lambda:stream.read(1024*1024),b''):
                value.update(block)
        return value.hexdigest()
    # Independent bounded reads preserve input order and every exact Git check.
    # At most eight handles and eight 1-MiB buffers are live at a time.
    with ThreadPoolExecutor(max_workers=8) as executor:
        objects = list(executor.map(blob_identity, (name for name,_ in rows)))
    unchanged = len(rows) == len(objects) and all(blob == actual for (_, blob), actual in zip(rows, objects))
    baseline = subprocess.check_output(['git', 'show', commit + ':docs/quality/current-functional-baseline.json'], cwd=ROOT)
    return {'passed': unchanged, 'source_commit': commit, 'original_files': len(rows),
        'original_media': sum(Path(name).suffix.lower() in ('.png', '.gif', '.mp4') for name, _ in rows),
        'original_git_object_manifest_sha256': hashlib.sha256(json.dumps(rows, ensure_ascii=False, separators=(',', ':')).encode()).hexdigest(),
        'baseline_sha256': hashlib.sha256(baseline).hexdigest(), 'baseline_increment': json.loads(baseline)['increment'],
        'full_regression_reexecuted': False, 'retained_android_apk_is_current': False,
        'scope': 'Exact published evidence/media bytes retained. Prior full regression and APK remain bound to their original sources; no current full-suite/native-artifact claim.'}


def verify(config=None, save=False):
    config = config or plan()
    evidence, checks = directory(config), []
    current = digest(config=config)
    adjusted = fixture_adjustment(config)
    accepted = {current} | ({adjusted['execution_source_sha256']} if adjusted else set())
    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})
    records = []
    for item in config['commands']:
        record = read(evidence / (item['id'] + '.json'))
        records.append({'file': (evidence / (item['id']+'.json')).relative_to(ROOT).as_posix(), 'sha256': sha(evidence / (item['id']+'.json'))})
        check('Source-bound executed check ' + item['id'], record['passed'] and record['command'] == item['command'] and record['exit_code'] == 0
            and record['source_sha256'] == record['source_after_sha256'] and record['source_sha256'] in accepted
            and record['source_unchanged'] and not record['virtualization_used'])
    for group in config['browser_groups']:
        browser = ROOT / 'docs/quality/evidence' / group
        report, manifest = read(browser / 'result.json'), read(browser / 'source-manifest.json')
        check('Actual UI/API flow and owned cleanup ' + group, report['passed'] and report['source_unchanged']
            and report['source_sha256'] == manifest['sha256'] == digest(manifest['files'], config)
            and report['owned_browser_closed'] and report['browser_contexts_closed'] >= 1
            and report['native_fixture_exit_code'] == 0 and report['isolated_database_cleanup_passed']
            and report['owned_review_server_stopped'] and report['build_source_unchanged'])
        records += [{'file': (browser / name).relative_to(ROOT).as_posix(), 'sha256': sha(browser / name)} for name in ('result.json', 'source-manifest.json', 'media.json')]
    media = read(evidence / 'media.json')
    check('Actual screenshot and recording outputs retain declared hashes', media['passed'] and all(sha(ROOT / item['file']) == item['sha256'] for item in media['artifacts']))
    visual = read(evidence / 'visual-review.json')
    check('All affected visual outputs actually reviewed', visual['passed'] and {item['file'] for item in visual['artifacts']} == {item['file'] for item in media['artifacts']}
        and all(item['reviewed'] and item['sha256'] == sha(ROOT / item['file']) for item in visual['artifacts']))
    update = read(evidence / 'development-update.json')
    development = read(evidence / 'development-project-resources.json')
    spec = importlib.util.spec_from_file_location('scoped_backend_sources', APP / 'scripts/verify_backend_batches.py')
    backend = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(backend)
    upgrade = config.get('schema_upgrade')
    schema_valid = (not update['schema_upgrade_executed']) if not upgrade else (
        update['previous_migration_count'] in (upgrade['from_count'], upgrade['to_count'])
        and update['migration_count'] == upgrade['to_count']
        and update['schema_upgrade_executed'] == (update['previous_migration_count'] == upgrade['from_count']))
    check('Owned development services match current backend and preserve existing business records',
        update['passed'] and development['passed'] and update['backend_source_sha256'] == development['backend_source_sha256'] == backend.digest()
        and update['history_before'] == update['history_after'] and update['legacy_and_usage_rows_preserved']
        and update['migration_count'] == development['migration_count'] and schema_valid
        and not update['postgres_restarted'] and not update['external_integration_started'] and not update['production_deployment']
        and development['worker_sources_match'] and all(development['ownership'].values())
        and development['database']['disposable_test_databases'] == 0 and development['integrations_disabled']
        and development['read_only_inspection'] and not development['production_deployment'])
    released = []
    for port in (5189,5190):
        with socket.socket() as probe:
            probe.settimeout(.3)
            released.append(probe.connect_ex(('127.0.0.1',port)) != 0)
    check('Both owned review ports are released', all(released))
    records += [{'file':(evidence/name).relative_to(ROOT).as_posix(),'sha256':sha(evidence/name)}
        for name in ('media.json','visual-review.json','development-update.json','development-project-resources.json','development-runtime.json')]
    previous = retained(config)
    check('All prior published proof/media bytes remain unchanged with honest retained scope', previous['passed'] and previous['baseline_increment'] == config['retained_base_increment'])
    old_migrations = subprocess.check_output(['git', 'ls-tree', '-r', config['retained_base_commit'], '--', 'app/backend/migrations'], cwd=ROOT).decode().splitlines()
    historic = {}
    for line in old_migrations:
        meta, name = line.split('\t', 1)
        raw = (ROOT / name).read_bytes()
        historic[name] = hashlib.sha1(b'blob '+str(len(raw)).encode()+b'\0'+raw, usedforsecurity=False).hexdigest() == meta.split()[2]
    actual_migrations = {file.relative_to(ROOT).as_posix() for file in (APP / 'backend/migrations').rglob('*') if file.is_file()}
    expected_new = {item['file'] for item in upgrade['new_migrations']} if upgrade else set()
    check('Historical schema bytes unchanged and only declared new upgrades added', all(historic.values())
        and actual_migrations == set(historic) | expected_new
        and (not upgrade or len(expected_new) == upgrade['to_count'] - upgrade['from_count']
             and all(sha(ROOT/item['file']) == item['sha256'] for item in upgrade['new_migrations'])))
    check('Owned browser fixture released', not (APP / '.local/business-browser-fixture.json').exists())
    check('Scope and platform acceptance are not replaced by focused checks', config['production_ready'] is False and config['full_goal_complete'] is False
        and config['formal_android_acceptance'] is False and config['full_backend_suite_executed'] is False)
    report = {'passed': all(row['passed'] for row in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'verification_mode': 'scoped_increment', 'increment': config['increment'], 'checks': checks,
        'interaction_kind': config['name'], 'interaction_source_sha256': current,
        'verification_script_sha256': sha(Path(__file__)), 'configuration_sha256': sha(ROOT / config['configuration_file']),
        'records': records, 'retained': previous, 'fixture_only_adjustment':adjusted,
        'full_backend_suite_executed': False, 'formal_android_acceptance': False,
        'full_goal_complete': False, 'production_ready': False}
    if save:
        names = sources(config)
        (evidence / 'source-manifest.json').write_text(json.dumps({'sha256': current, 'files': [{'file': name, 'sha256': sha(APP/name)} for name in names]}, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
        (evidence / 'reused-evidence.json').write_text(json.dumps(previous, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
        (evidence / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    return report


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--plan')
    parser.add_argument('--run')
    args = parser.parse_args()
    config = plan(args.plan)
    if args.run:
        if args.run == 'clients':
            for item in config['commands']:
                if item['id'] != 'backend' and run(config, item['id']):
                    return 1
            return 0
        return run(config, args.run)
    report = verify(config, save=True)
    print(json.dumps({'passed': report['passed'], 'checks': len(report['checks']), 'failed': [row['name'] for row in report['checks'] if not row['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
