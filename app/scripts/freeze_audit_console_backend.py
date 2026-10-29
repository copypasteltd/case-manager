"""Preserve this completed run's raw batch bytes before generic logs are reused."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil

from verify_backend_batches import digest

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/audit-console-backend'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    state, result = [read(E / name) for name in ('batch-progress.json', 'result.json')]
    assert result['passed'] and result['source_sha256'] == state['source_sha256'] == digest()
    assert result['checks'] == [v for b in state['batches'] for v in b['checks']]
    archive = APP / '.local/audit-console-backend'
    archive.mkdir(parents=True, exist_ok=True)
    logs = []
    for number, batch in enumerate(state['batches'], 1):
        assert batch['status'] == 'passed' and batch['source_unchanged_during_tests']
        raw = APP / '.local' / ('audit-console-backend-batch-' + str(number) + '.log')
        observed = []
        for line in raw.read_text(encoding='utf-8').splitlines():
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if event.get('Action') in ('pass', 'fail', 'skip') and event.get('Test'):
                observed.append({k: event[k] for k in ('Action', 'Package', 'Test', 'Elapsed') if k in event})
        assert observed == batch['checks'] and all(v['Action'] == 'pass' for v in observed)
        saved = archive / ('batch-' + str(number) + '.log')
        assert not saved.exists() or sha(saved) == sha(raw), 'An existing raw archive must never be overwritten.'
        if not saved.exists():
            shutil.copyfile(raw, saved)
        logs.append({'batch': number, 'raw_log': saved.relative_to(APP).as_posix(),
                     'sha256': sha(saved), 'bytes': saved.stat().st_size, 'records': len(observed),
                     'started_at': batch['started_at'], 'completed_at': batch['completed_at']})
    report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(),
              'source_sha256': result['source_sha256'], 'freezer_sha256': sha(Path(__file__)),
              'logs': logs, 'records': sum(v['records'] for v in logs),
              'raw_logs_private': True, 'prior_increments_raw_logs_reverified': False,
              'scope': 'Only this completed current-source run is archived; no prior private log retention claim.'}
    (E / 'raw-log-archive.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'archived_batches': len(logs), 'records': report['records']}))


if __name__ == '__main__':
    main()
