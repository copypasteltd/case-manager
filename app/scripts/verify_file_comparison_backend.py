"""Execute every discovered backend test against the owned durable native PG."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
import verify_backend_batches as base

APP = base.APP
E = APP.parent / 'docs/quality/evidence/file-comparison-backend'
PROGRESS = E / 'batch-progress.json'
RUNNER = APP / 'scripts/test_file_comparison_native.py'

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--resume', action='store_true')
    args = parser.parse_args()
    E.mkdir(parents=True, exist_ok=True)
    assert args.resume == PROGRESS.exists(), 'Use explicit resume only for an existing run.'
    source = base.digest()
    base.command = lambda *items: [sys.executable, str(RUNNER), '-json', *items]
    expected = base.discover()
    assert base.digest() == source
    names = sorted({name for _, name in expected})
    if args.resume:
        state = json.loads(PROGRESS.read_text(encoding='utf-8'))
        assert state['source_sha256'] == source and state['expected_tests'] == [list(v) for v in expected]
        retained = []
        for n, batch in enumerate(state['batches'], 1):
            raw = APP / '.local' / ('file-comparison-backend-batch-' + str(n) + '.log')
            if batch['status'] == 'passed':
                assert batch['exit_code'] == 0 and batch['source_unchanged_during_tests']
                assert sha(raw) == batch['raw_output_sha256']
                assert all(row['Action'] == 'pass' for row in batch['checks'])
                assert {(v['Package'], v['Test']) for v in batch['checks'] if '/' not in v['Test']} == {v for v in expected if v[1] in batch['names']}
                retained.append(n)
            elif raw.exists():
                # An incomplete or failed raw output is never overwritten or counted as a pass.
                frozen = raw.with_name(raw.stem + '-prior-' + sha(raw)[:12] + raw.suffix)
                if not frozen.exists(): frozen.write_bytes(raw.read_bytes())
                state.setdefault('prior_attempts', []).append({'batch': n, 'status': batch['status'], 'raw_sha256': sha(frozen), 'private_log': frozen.name, 'counts_as_passed': False})
        state.setdefault('resumes', []).append({'at': datetime.now(timezone.utc).isoformat(), 'retained_complete_batches': retained})
    else:
        state = {'source_sha256': source, 'started_at': datetime.now(timezone.utc).isoformat(), 'expected_tests': [list(v) for v in expected], 'parallel_batches': 1,
            'batches': [{'names': names[i:i+20], 'status': 'pending', 'checks': []} for i in range(0, len(names), 20)]}
    base.save(PROGRESS, state)
    for index, batch in enumerate(state['batches']):
        if batch['status'] == 'passed': continue
        assert base.digest() == source, 'Source changed before a batch; do not reuse previous tests.'
        selected = batch['names']
        batch = {'names': selected, 'status': 'running', 'checks': [], 'started_at': datetime.now(timezone.utc).isoformat(), 'runner_sha256': sha(Path(__file__))}
        state['batches'][index] = batch
        base.save(PROGRESS, state)
        print('Running native backend batch ' + str(index+1), flush=True)
        raw = APP / '.local' / ('file-comparison-backend-batch-' + str(index+1) + '.log')
        pattern = '^(' + '|'.join(re.escape(name) for name in selected) + ')$'
        with raw.open('w', encoding='utf-8') as output:
            process = subprocess.Popen([sys.executable, str(RUNNER), '-json', '-run', pattern, '-timeout=20m'], cwd=APP, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, encoding='utf-8', errors='replace')
            for line in process.stdout:
                output.write(line); output.flush()
                try: event = json.loads(line)
                except json.JSONDecodeError: continue
                if event.get('Action') in ('pass', 'fail', 'skip') and event.get('Test'):
                    batch['checks'].append({k: event[k] for k in ('Action','Package','Test','Elapsed') if k in event})
            status = process.wait()
        observed = {(v['Package'], v['Test']) for v in batch['checks'] if '/' not in v['Test']}
        unchanged = base.digest() == source
        passed = status == 0 and unchanged and observed == {v for v in expected if v[1] in selected} and all(v['Action'] == 'pass' for v in batch['checks'])
        batch.update(status='passed' if passed else 'failed', exit_code=status, source_unchanged_during_tests=unchanged, completed_at=datetime.now(timezone.utc).isoformat(), raw_output='ignored app/.local/' + raw.name, raw_output_sha256=sha(raw))
        base.save(PROGRESS, state)
        print(json.dumps({'batch':index+1,'passed':passed,'result_records':len(batch['checks'])}),flush=True)
        if not passed: break
    checks = [v for b in state['batches'] for v in b['checks']]
    observed = {(v['Package'],v['Test']) for v in checks if '/' not in v['Test']}
    unchanged = base.digest() == source and all(b.get('source_unchanged_during_tests',False) for b in state['batches'])
    passed = unchanged and observed == set(expected) and all(b['status']=='passed' for b in state['batches']) and all(v['Action']=='pass' for v in checks)
    report = {'passed':passed,'source_sha256':source,'source_unchanged_during_tests':unchanged,'executed_at':datetime.now(timezone.utc).isoformat(),'command':'python app/scripts/verify_file_comparison_backend.py','verification_script_sha256':sha(Path(__file__)),
        'execution':'Every discovered test; serial isolated native batches, at most20 top-level names per batch','batch_count':len(state['batches']),'parallel_batches':1,'discovered_top_level_tests':len(expected),'all_discovered_tests_executed':observed==set(expected),'native_database':True,
        'raw_output':'Ignored app/.local/file-comparison-backend-batch-*.log','checks':checks,'resumes':state.get('resumes',[]),'prior_attempts':state.get('prior_attempts',[])}
    base.save(E/'result.json',report)
    print(json.dumps({'passed':passed,'top_level_tests':len(expected),'recorded_test_results':len(checks),'batches':len(state['batches'])}),flush=True)
    return 0 if passed else 1

if __name__ == '__main__':
    raise SystemExit(main())
