"""Retry failed and explicitly missing top-level cases after the first batch attempt.

All successful current-source cases retain their original proof. The failed first
attempt, every raw-log digest and retry provenance remain reviewable. Each explicit --case must match the observed failed/missing parent set exactly.
No test is skipped, timeout increased, durability disabled or failure relabelled as a pass.
"""
from datetime import datetime, timezone
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess

import verify_backend_batches as base

APP = base.APP
E = APP.parent / 'docs/quality/evidence/finance-fx-backend'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--case', action='append', required=True, help='Exactly the inspected failed/missing top-level case name; repeat for each case.')
    args = parser.parse_args()
    path = E / 'result.json'
    original = json.loads(path.read_text())
    progress = json.loads((E / 'batch-progress.json').read_text())
    source = base.digest()
    assert not original['passed'] and original['source_sha256'] == progress['source_sha256'] == source
    assert original['source_unchanged_during_tests']
    assert all(b['status'] in ('passed', 'failed') for b in progress['batches'])
    archive = E / 'attempt01'
    assert not archive.exists(), 'A retry attempt exists; inspect it before another execution.'
    archive.mkdir()
    (archive / 'result.json').write_bytes(path.read_bytes())
    (archive / 'batch-progress.json').write_bytes((E / 'batch-progress.json').read_bytes())
    for batch in progress['batches']:
        assert sha(APP / '.local' / Path(batch['raw_output']).name) == batch['raw_output_sha256']
    expected = set(base.discover())
    assert expected == {tuple(item) for item in progress['expected_tests']}
    observed = {(r['Package'], r['Test']) for r in original['checks'] if '/' not in r['Test']}
    missing = expected - observed
    failed = {(r['Package'], r['Test'].split('/')[0]) for r in original['checks'] if r['Action'] != 'pass'}
    selected_cases = sorted(failed | missing)
    assert selected_cases and failed <= expected and len(expected) == original['discovered_top_level_tests']
    assert set(args.case) == {name for _, name in selected_cases}, 'Explicit cases must exactly match observed failures and missing cases.'
    retained = [r for r in original['checks'] if (r['Package'], r['Test'].split('/')[0]) not in selected_cases]
    retries = []
    checks = []
    for index, (package, name) in enumerate(selected_cases, 1):
        assert base.digest() == source
        log = APP / '.local' / ('finance-fx-backend-retry-' + str(index) + '.log')
        selected = []
        started = datetime.now(timezone.utc).isoformat()
        print('Retrying native case ' + name, flush=True)
        with log.open('w', encoding='utf-8') as output:
            process = subprocess.Popen(base.command('-run', '^' + re.escape(name) + '$', '-timeout=20m'), cwd=APP,
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, encoding='utf-8', errors='replace')
            for line in process.stdout:
                output.write(line)
                output.flush()
                try:
                    event = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if event.get('Action') in ('pass', 'fail', 'skip') and event.get('Test'):
                    selected.append({key: event[key] for key in ('Action', 'Package', 'Test', 'Elapsed') if key in event})
            status = process.wait()
        observed = {(r['Package'], r['Test']) for r in selected if '/' not in r['Test']}
        passed = status == 0 and observed == {(package, name)} and all(r['Action'] == 'pass' for r in selected) and base.digest() == source
        retries.append({'test': name, 'package': package, 'passed': passed, 'previously_missing': (package, name) in missing,
            'exit_code': status, 'started_at': started,
            'finished_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': source, 'source_unchanged': base.digest() == source,
            'raw_output': 'ignored app/.local/' + log.name, 'raw_output_sha256': sha(log), 'checks': selected})
        base.save(E / 'retry-progress.json', {'source_sha256': source, 'serial_execution': True, 'retries': retries})
        print(json.dumps({'test': name, 'passed': passed, 'result_records': len(selected)}), flush=True)
        if not passed:
            return 1
        checks += selected
    combined = retained + checks
    observed = {(r['Package'], r['Test']) for r in combined if '/' not in r['Test']}
    result = {**original, 'passed': observed == expected and base.digest() == source and all(r['Action'] == 'pass' for r in combined),
        'checks': combined, 'source_unchanged_during_tests': base.digest() == source, 'all_discovered_tests_executed': observed == expected,
        'execution': 'All discovered cases on fixed sources; completed passing parents retained; explicitly selected failed/missing parents executed serially.',
        'command': 'python app/scripts/verify_finance_fx_backend.py; python app/scripts/retry_finance_fx_backend.py ' + ' '.join('--case ' + name for name in args.case),
        'verified_at': datetime.now(timezone.utc).isoformat(), 'verification_script_sha256': sha(Path(__file__)),
        'first_attempt': {'record': 'attempt01/result.json', 'sha256': sha(archive / 'result.json'), 'passed': False,
            'verification_script_sha256': original['verification_script_sha256']},
        'retried_failed_tests': [list(item) for item in sorted(failed)], 'retained_passed_result_records': len(retained), 'first_attempt_missing_tests': [list(item) for item in sorted(missing)],
        'retries': retries}
    base.save(path, result)
    print(json.dumps({'passed': result['passed'], 'top_level_tests': len(expected), 'result_records': len(combined),
        'retried_failed_top_level_cases': len(failed), 'newly_completed_missing_cases': len(missing), 'first_attempt_preserved': True}), flush=True)
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
