"""Run every discovered backend test in serial isolated native batch processes.

Each integration fixture creates its own random PostgreSQL database and removes
it through t.Cleanup. No completed result from another source digest is reused.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import subprocess
import threading

import verify_backend_batches as base

APP = base.APP
EVIDENCE = APP.parent / 'docs/quality/evidence/file-recovery-backend'
PROGRESS = EVIDENCE / 'batch-progress.json'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--resume', action='store_true')
    args = parser.parse_args()
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    if PROGRESS.exists() and not args.resume:
        raise SystemExit('Archive the previous attempt before starting fresh checks.')
    source = base.digest()
    base.command = lambda *args: [__import__('sys').executable,str(APP/'scripts/test_file_recovery_native.py'),'-json',*args]
    expected = base.discover()
    assert base.digest() == source
    names = sorted({name for _, name in expected})
    if args.resume:
        state = json.loads(PROGRESS.read_text(encoding='utf-8'))
        assert state['source_sha256'] == source
        assert state['expected_tests'] == [list(item) for item in expected]
        interrupted = EVIDENCE.parent / 'file-recovery/attempts/03-process-interruption'
        proof = json.loads((interrupted / 'attempt.json').read_text())
        assert not proof['passed'] and proof['completed_backend_batches'] == [1, 2, 3, 4]
        original_runner = hashlib.sha256((interrupted / 'original-backend-runner.py').read_bytes()).hexdigest()
        assert original_runner == proof['original_runner_sha256']
        completed = []
        for index, batch in enumerate(state['batches'], 1):
            if batch['status'] != 'passed':
                continue
            raw = APP / '.local' / ('file-recovery-backend-batch-' + str(index) + '.log')
            assert hashlib.sha256(raw.read_bytes()).hexdigest() == batch['raw_output_sha256']
            assert batch['source_unchanged_during_tests'] and batch['exit_code'] == 0
            assert all(v['Action'] == 'pass' for v in batch['checks'])
            assert {(v['Package'], v['Test']) for v in batch['checks'] if '/' not in v['Test']} == {v for v in expected if v[1] in batch['names']}
            batch.setdefault('runner_sha256', original_runner)
            completed.append(index)
        assert completed == proof['completed_backend_batches']
        state['resume'] = {'resumed_at': datetime.now(timezone.utc).isoformat(), 'completed_current_source_batches_retained': completed,
            'interrupted_batch_rerun': proof['interrupted_backend_batch'], 'interruption_record': 'docs/quality/evidence/file-recovery/attempts/03-process-interruption/attempt.json',
            'original_runner_sha256': original_runner, 'current_runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
        print('Retaining four complete source-identical batches; rerunning interrupted batch5 and all pending batches.', flush=True)
    else:
        state = {'source_sha256': source, 'started_at': datetime.now(timezone.utc).isoformat(),
                 'expected_tests': [list(item) for item in expected], 'parallel_batches': 1,
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
                 'started_at': datetime.now(timezone.utc).isoformat(), 'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
        save(index, dict(batch, checks=[]))
        print('Running native backend batch ' + str(index + 1), flush=True)
        raw = APP / '.local' / ('file-recovery-backend-batch-' + str(index + 1) + '.log')
        pattern = '^(' + '|'.join(re.escape(name) for name in selected) + ')$'
        with raw.open('w', encoding='utf-8') as output:
            process = subprocess.Popen([__import__('sys').executable,str(APP/'scripts/test_file_recovery_native.py'),'-json','-run',pattern,'-timeout=20m'],
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

    for index, batch in enumerate(state['batches']):
        if batch['status'] == 'passed':
            continue
        run_batch(index, list(batch['names']))
        if state['batches'][index]['status'] != 'passed':
            break
    checks = [row for batch in state['batches'] for row in batch['checks']]
    observed = {(row['Package'], row['Test']) for row in checks if '/' not in row['Test']}
    unchanged = base.digest() == source and all(batch.get('source_unchanged_during_tests', False) for batch in state['batches'])
    passed = (unchanged and observed == set(expected) and all(batch['status'] == 'passed' for batch in state['batches'])
              and all(row['Action'] == 'pass' for row in checks))
    report = {'passed': passed, 'source_sha256': source, 'source_unchanged_during_tests': unchanged,
        'executed_at': datetime.now(timezone.utc).isoformat(),
        'command': 'python app/scripts/verify_file_recovery_backend.py',
        'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'execution': 'Every discovered test; one isolated native batch at a time, each at most 20 top-level names',
        'batch_count': len(state['batches']), 'parallel_batches': 1,
        'discovered_top_level_tests': len(expected), 'all_discovered_tests_executed': observed == set(expected),
        'native_database': True, 'raw_output': 'Ignored app/.local/file-recovery-backend-batch-*.log',
        'checks': checks, 'resume': state.get('resume'), 'completed_batches_rerun': False if args.resume else None}
    base.save(EVIDENCE / 'result.json', report)
    print(json.dumps({'passed': passed, 'top_level_tests': len(expected),
        'recorded_test_results': len(checks), 'batches': len(state['batches'])}), flush=True)
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
