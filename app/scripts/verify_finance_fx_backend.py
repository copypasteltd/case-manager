"""Run every discovered backend test in two isolated native batch processes.

Each integration fixture creates its own random PostgreSQL database and removes
it through t.Cleanup. No completed result from another source digest is reused.
"""
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import subprocess
import threading

import verify_backend_batches as base

APP = base.APP
EVIDENCE = APP.parent / 'docs/quality/evidence/finance-fx-backend'
PROGRESS = EVIDENCE / 'batch-progress.json'


def main():
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    if PROGRESS.exists():
        raise SystemExit('Archive the previous attempt before starting fresh checks.')
    source = base.digest()
    expected = base.discover()
    assert base.digest() == source
    names = sorted({name for _, name in expected})
    state = {'source_sha256': source, 'started_at': datetime.now(timezone.utc).isoformat(),
             'expected_tests': [list(item) for item in expected], 'parallel_batches': 2,
             'batches': [{'names': names[i:i + 20], 'status': 'pending', 'checks': []}
                         for i in range(0, len(names), 20)]}
    lock = threading.Lock()

    def save(index=None, batch=None):
        with lock:
            if index is not None:
                state['batches'][index] = batch
            base.save(PROGRESS, state)

    save()

    def run_batch(index, selected):
        if base.digest() != source:
            raise RuntimeError('Backend source changed before batch start.')
        batch = {'names': selected, 'status': 'running', 'checks': [],
                 'started_at': datetime.now(timezone.utc).isoformat()}
        save(index, dict(batch, checks=[]))
        print('Running native backend batch ' + str(index + 1), flush=True)
        raw = APP / '.local' / ('finance-fx-backend-batch-' + str(index + 1) + '.log')
        pattern = '^(' + '|'.join(re.escape(name) for name in selected) + ')$'
        with raw.open('w', encoding='utf-8') as output:
            process = subprocess.Popen(base.command('-run', pattern, '-timeout=20m'),
                cwd=APP, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                text=True, encoding='utf-8', errors='replace')
            for line in process.stdout:
                output.write(line)
                output.flush()
                try:
                    event = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if event.get('Action') in ('pass', 'fail', 'skip') and event.get('Test'):
                    batch['checks'].append({key: event[key] for key in
                        ('Action', 'Package', 'Test', 'Elapsed') if key in event})
            status = process.wait()
        observed = {(row['Package'], row['Test']) for row in batch['checks'] if '/' not in row['Test']}
        required = {item for item in expected if item[1] in selected}
        unchanged = base.digest() == source
        passed = (status == 0 and unchanged and observed == required
                  and all(row['Action'] == 'pass' for row in batch['checks']))
        batch.update(status='passed' if passed else 'failed', exit_code=status,
            source_unchanged_during_tests=unchanged, completed_at=datetime.now(timezone.utc).isoformat(),
            raw_output='ignored app/.local/' + raw.name,
            raw_output_sha256=hashlib.sha256(raw.read_bytes()).hexdigest())
        save(index, batch)
        print(json.dumps({'batch': index + 1, 'passed': passed,
                          'result_records': len(batch['checks'])}), flush=True)

    selections = [list(batch['names']) for batch in state['batches']]
    with ThreadPoolExecutor(max_workers=2) as pool:
        pending = [pool.submit(run_batch, index, selected) for index, selected in enumerate(selections)]
        for future in as_completed(pending):
            future.result()
    checks = [row for batch in state['batches'] for row in batch['checks']]
    observed = {(row['Package'], row['Test']) for row in checks if '/' not in row['Test']}
    unchanged = base.digest() == source and all(batch['source_unchanged_during_tests'] for batch in state['batches'])
    passed = (unchanged and observed == set(expected) and all(batch['status'] == 'passed' for batch in state['batches'])
              and all(row['Action'] == 'pass' for row in checks))
    report = {'passed': passed, 'source_sha256': source, 'source_unchanged_during_tests': unchanged,
        'executed_at': datetime.now(timezone.utc).isoformat(),
        'command': 'python app/scripts/verify_finance_fx_backend.py',
        'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'execution': 'Every discovered test; at most two isolated native batches, each at most 20 top-level names',
        'batch_count': len(state['batches']), 'parallel_batches': 2,
        'discovered_top_level_tests': len(expected), 'all_discovered_tests_executed': observed == set(expected),
        'native_database': True, 'raw_output': 'Ignored app/.local/finance-fx-backend-batch-*.log',
        'checks': checks}
    base.save(EVIDENCE / 'result.json', report)
    print(json.dumps({'passed': passed, 'top_level_tests': len(expected),
        'recorded_test_results': len(checks), 'batches': len(state['batches'])}), flush=True)
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
