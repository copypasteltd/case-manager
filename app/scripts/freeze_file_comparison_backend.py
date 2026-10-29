"""Preserve this completed run's raw batch bytes before generic logs are reused."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil

from verify_backend_batches import digest

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/file-comparison-backend'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    state, result = [read(E / name) for name in ('batch-progress.json', 'result.json')]
    assert result['passed'] and result['source_sha256'] == state['source_sha256'] == digest()
    assert result['checks'] == [v for b in state['batches'] for v in b['checks']]
    archive = APP / '.local/file-comparison-backend'
    archive.mkdir(parents=True, exist_ok=True)
    logs = []
    for number, batch in enumerate(state['batches'], 1):
        assert batch['status'] == 'passed' and batch['source_unchanged_during_tests']
        segments = batch.get('evidence_segments') or [{'raw_log':'.local/file-comparison-backend-batch-' + str(number) + '.log',
            'sha256':batch['raw_output_sha256'],'excluded_test_families':[], 'selected_records':len(batch['checks']), 'kind':'complete_successful_batch'}]
        selected = []
        for part, segment in enumerate(segments, 1):
            raw = APP / segment['raw_log']
            assert sha(raw) == segment['sha256']
            observed = []
            for line in raw.read_text(encoding='utf-8').splitlines():
                try:
                    event = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if event.get('Action') in ('pass', 'fail', 'skip') and event.get('Test'):
                    observed.append({k:event[k] for k in ('Action','Package','Test','Elapsed') if k in event})
            excluded = {(v['Package'],v['Test']) for v in segment['excluded_test_families']}
            assert not excluded or excluded <= {(v['Package'],v['Test']) for v in result['cleanup_retry']['retried_test_families']}
            effective = [v for v in observed if (v['Package'],v['Test'].split('/')[0]) not in excluded]
            assert len(effective) == segment['selected_records'] and all(v['Action'] == 'pass' for v in effective)
            selected.extend(effective)
            saved = archive / ('batch-' + str(number) + ('-part-' + str(part) if len(segments)>1 else '') + '.log')
            assert not saved.exists() or sha(saved) == sha(raw), 'An existing raw archive must never be overwritten.'
            if not saved.exists():
                shutil.copyfile(raw, saved)
            logs.append({'batch':number,'part':part,'raw_log':saved.relative_to(APP).as_posix(), 'runner_sha256':batch['runner_sha256'],
                'sha256':sha(saved),'bytes':saved.stat().st_size,'records':len(effective),'raw_records':len(observed),
                'excluded_test_families':segment['excluded_test_families'],'kind':segment['kind'],
                'started_at':segment.get('started_at',batch['started_at']),'completed_at':segment.get('completed_at',batch['completed_at'])})
        assert selected == batch['checks']
    report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(),
              'source_sha256': result['source_sha256'], 'freezer_sha256': sha(Path(__file__)),
              'logs': logs, 'records': sum(v['records'] for v in logs),
              'raw_logs_private': True, 'prior_increments_raw_logs_reverified': False,
              'scope': 'All 12 complete successful current-source batches retain exact private raw logs, with no excluded test families or cleanup substitutions. This adapter preserves existing executions and does not rerun tests. Earlier failed fixture attempts remain separate; prior-increment private logs are not reverified.'}
    (E / 'raw-log-archive.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'archived_batches': len(state['batches']), 'raw_segments':len(logs), 'records': report['records']}))


if __name__ == '__main__':
    main()
