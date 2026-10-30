"""Run every discovered backend test in resumable native batches.

Only completed batches with an unchanged backend digest are reusable. A stopped
process never counts as a pass; interrupted batches run again on explicit resume.
Raw test output stays in the ignored local directory.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/backend'
PROGRESS = EVIDENCE / 'batch-progress.json'


def digest():
    value = hashlib.sha256()
    backend = APP / 'backend'
    for path in sorted(backend.rglob('*')):
        if path.is_file() and (path.suffix in ('.go', '.sql') or path.name in ('go.mod', 'go.sum') or path.relative_to(backend).as_posix() == 'internal/contract/openapi.json' or path.relative_to(backend).as_posix().startswith('internal/files/office_templates/')):
            value.update(path.relative_to(backend).as_posix().encode('utf-8') + b'\0')
            value.update(path.read_bytes() + b'\0')
    return value.hexdigest()


def save(path, data):
    temporary = path.with_suffix('.tmp')
    temporary.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    temporary.replace(path)


def command(*args):
    return [sys.executable, str(APP / 'scripts/test_backend.py'), '-json', *args]


def discover():
    result = subprocess.run(command('-list', '^Test'), cwd=APP, capture_output=True, text=True, encoding='utf-8', errors='replace')
    if result.returncode:
        raise SystemExit('Test discovery failed; raw diagnostics withheld.')
    expected = set()
    for line in result.stdout.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        name = event.get('Output', '').strip()
        if event.get('Action') == 'output' and re.fullmatch(r'Test[A-Za-z0-9_]+', name):
            expected.add((event['Package'], name))
    if not expected:
        raise SystemExit('No backend tests discovered; refusing an empty verification.')
    return sorted(expected)


def main():
    global EVIDENCE, PROGRESS
    parser = argparse.ArgumentParser()
    parser.add_argument('--batch-size', type=int, default=8)
    parser.add_argument('--resume', action='store_true')
    parser.add_argument('--evidence-group', default='backend')
    args = parser.parse_args()
    if not args.evidence_group.replace('-', '').isalnum():
        raise SystemExit('Invalid evidence group.')
    EVIDENCE = APP.parent / 'docs/quality/evidence' / args.evidence_group
    PROGRESS = EVIDENCE / 'batch-progress.json'
    if not 1 <= args.batch_size <= 20:
        raise SystemExit('Batch size must be between 1 and 20 top-level test names.')
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    (APP / '.local').mkdir(exist_ok=True)
    source = digest()
    expected = discover()
    if digest() != source:
        raise SystemExit('Backend changed during discovery; start again with a fixed source.')
    expected_json = [list(item) for item in expected]
    if args.resume:
        state = json.loads(PROGRESS.read_text(encoding='utf-8'))
        if state['source_sha256'] != source or state['expected_tests'] != expected_json:
            raise SystemExit('Source or discovered tests changed; completed batches cannot be reused.')
    else:
        if PROGRESS.exists():
            raise SystemExit('A batch record exists; inspect it and use --resume, or archive it before a fresh run.')
        names = sorted({name for _, name in expected})
        state = {'source_sha256': source, 'started_at': datetime.now(timezone.utc).isoformat(),
                 'expected_tests': expected_json,
                 'batches': [{'names': names[start:start + args.batch_size], 'status': 'pending', 'checks': []}
                             for start in range(0, len(names), args.batch_size)]}
        save(PROGRESS, state)
    for index, batch in enumerate(state['batches']):
        if batch['status'] == 'passed':
            continue
        if digest() != source:
            raise SystemExit('Backend changed; refusing to mix results from different sources.')
        batch.update(status='running', checks=[], started_at=datetime.now(timezone.utc).isoformat())
        save(PROGRESS, state)
        raw = APP / '.local' / ('backend-batch-' + str(index + 1) + '.log')
        pattern = '^(' + '|'.join(re.escape(name) for name in batch['names']) + ')$'
        print('Running native backend batch ' + str(index + 1) + '/' + str(len(state['batches'])), flush=True)
        with raw.open('w', encoding='utf-8') as output:
            process = subprocess.Popen(command('-run', pattern, '-timeout=20m'), cwd=APP, stdout=subprocess.PIPE,
                                       stderr=subprocess.STDOUT, text=True, encoding='utf-8', errors='replace')
            for line in process.stdout:
                output.write(line)
                output.flush()
                try:
                    event = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if event.get('Action') in ('pass', 'fail', 'skip') and event.get('Test'):
                    batch['checks'].append({key: event[key] for key in ('Action', 'Package', 'Test', 'Elapsed') if key in event})
                    if '/' not in event['Test']:
                        print(event['Action'] + ': ' + event['Test'], flush=True)
            status = process.wait()
        observed = {(check['Package'], check['Test']) for check in batch['checks'] if '/' not in check['Test']}
        required = {item for item in expected if item[1] in batch['names']}
        unchanged = digest() == source
        passed = status == 0 and unchanged and observed == required and all(check['Action'] == 'pass' for check in batch['checks'])
        batch.update(status='passed' if passed else 'failed', source_unchanged_during_tests=unchanged,
                     completed_at=datetime.now(timezone.utc).isoformat(), exit_code=status)
        save(PROGRESS, state)
        if not passed:
            print(json.dumps({'passed': False, 'failed_batch': index + 1, 'raw_output': str(raw.relative_to(APP))}), flush=True)
            return 1
    checks = [check for batch in state['batches'] for check in batch['checks']]
    observed = {(check['Package'], check['Test']) for check in checks if '/' not in check['Test']}
    passed = observed == set(expected) and digest() == source and all(check['Action'] == 'pass' for check in checks)
    report = {'passed': passed, 'source_sha256': source,
              'source_unchanged_during_tests': all(batch['source_unchanged_during_tests'] for batch in state['batches']) and digest() == source,
              'executed_at': datetime.now(timezone.utc).isoformat(),
              'command': 'python app/scripts/verify_backend_batches.py --batch-size ' + str(args.batch_size) + ' --evidence-group ' + args.evidence_group,
              'execution': 'Every discovered test; serial resumable batches, not a single Go test process',
              'batch_count': len(state['batches']), 'discovered_top_level_tests': len(expected),
              'all_discovered_tests_executed': observed == set(expected),
              'native_database': True, 'raw_output': 'Ignored app/.local/backend-batch-*.log; not document artifacts',
              'checks': checks}
    save(EVIDENCE / 'result.json', report)
    print(json.dumps({'passed': passed, 'recorded_test_results': len(checks), 'top_level_tests': len(expected), 'batches': len(state['batches'])}), flush=True)
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
