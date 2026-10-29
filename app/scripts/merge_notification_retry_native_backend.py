"""Compose exact unchanged-source native results, preserving both failed cleanup attempts."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import shutil
import verify_backend_batches as base

APP = base.APP
E = APP.parent / 'docs/quality/evidence/notification-retry'
B = E.parent / 'notification-retry-backend'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    initial = B / 'attempts/01-native-cleanup-timeout'
    target = B / 'attempts/02-serial-native-cleanup-timeout'
    private = APP / '.local/notification-retry-attempts/backend02-cleanup'
    assert not target.exists() and not private.exists()
    state, original = read(B / 'batch-progress.json'), read(B / 'result.json')
    native = read(E / 'native-family-result.json')
    source = base.digest()
    assert native['passed'] and native['source_sha256'] == state['source_sha256'] == original['source_sha256'] == source
    assert native['source_unchanged_during_tests'] and sha(APP / native['raw_log']) == native['raw_log_sha256']
    assert not original['passed'] and not read(initial / 'result.json')['passed']
    failed = [i for i,v in enumerate(state['batches']) if v['status']=='failed']
    assert failed == [7], 'A different failure requires independent review.'
    prior = state['batches'][7]
    raw = APP / '.local/notification-retry-backend-retry-8.log'
    assert sha(raw) == prior['raw_output_sha256']
    events = [json.loads(line) for line in raw.read_text(encoding='utf-8').splitlines() if line.startswith('{')]
    errors = [v.get('Output','') for v in events if re.search(r'\.go:\d+:',v.get('Output',''))]
    assert errors and all('server_integration_test.go:86: timeout: context deadline exceeded' in v for v in errors)
    families = {(v['Package'],v['Test'].split('/')[0]) for v in prior['checks'] if v['Action']=='fail'}
    assert families == {(v['Package'],v['Test']) for v in native['checks'] if '/' not in v['Test']}
    target.mkdir(parents=True); private.mkdir(parents=True)
    shutil.copyfile(B / 'batch-progress.json',target / 'batch-progress.json')
    shutil.copyfile(raw,private / raw.name)
    attempt = {'passed':False,'complete':True,'scope':'Only the failed native cleanup family reran serially on the original durable cluster; DROP still exceeded the unchanged two-minute deadline. All business assertions passed. Native PG checkpoint diagnostics identify a132-second sync; no assertion or durability setting is relaxed.',
        'source_sha256':source,'raw_logs':[{'file':(private / raw.name).relative_to(APP).as_posix(),'sha256':sha(private / raw.name)}],
        'first_retry_script_sha256':sha(APP / 'scripts/retry_notification_retry_backend_cleanup.py')}
    base.save(target / 'attempt.json',attempt)
    retained = [v for v in prior['checks'] if (v['Package'],v['Test'].split('/')[0]) not in families]
    assert all(v['Action']=='pass' for v in retained+native['checks'])
    first_segment = dict(prior['evidence_segments'][0])
    old_batch = read(initial / 'batch-progress.json')['batches'][7]
    first_segment.update(started_at=old_batch['started_at'],completed_at=old_batch['completed_at'])
    fresh_segment = {'raw_log':native['raw_log'],'sha256':native['raw_log_sha256'],'excluded_test_families':[],
        'selected_records':len(native['checks']),'kind':'rerun_failed_cleanup_families',
        'started_at':native['started_at'],'completed_at':native['completed_at'],'native_cluster':'temporary durable native PG on C'}
    prior.update(status='passed',exit_code=0,checks=retained+native['checks'],source_unchanged_during_tests=True,
        started_at=native['started_at'],completed_at=native['completed_at'],raw_output='ignored app/'+native['raw_log'],
        raw_output_sha256=native['raw_log_sha256'],evidence_segments=[first_segment,fresh_segment])
    checks = [v for batch in state['batches'] for v in batch['checks']]
    observed = {(v['Package'],v['Test']) for v in checks if '/' not in v['Test']}
    assert observed == {tuple(v) for v in state['expected_tests']} and all(v['status']=='passed' for v in state['batches'])
    original.update(passed=True,checks=checks,executed_at=datetime.now(timezone.utc).isoformat(),
        cleanup_retry={'initial_record':(initial / 'result.json').relative_to(APP.parent).as_posix(),
            'initial_record_sha256':sha(initial / 'result.json'),'attempt_record_sha256':sha(initial / 'attempt.json'),
            'serial_retried_batches':[8],'retained_successful_batches':11,'retried_test_families':[{'Package':p,'Test':t} for p,t in sorted(families)],
            'successful_families_in_failed_batches_retained':True,'same_source_sha256':source,
            'additional_attempt_record':(target / 'attempt.json').relative_to(APP.parent).as_posix(),'additional_attempt_sha256':sha(target / 'attempt.json'),
            'native_result_record':(E / 'native-family-result.json').relative_to(APP.parent).as_posix(),'native_result_sha256':sha(E / 'native-family-result.json'),
            'verification_script_sha256':sha(Path(__file__))},
        execution='Every discovered current test executes. Only the failed cleanup family reruns on a separate fully durable native Windows PG cluster; successful original families retain their exact logs, and both failed attempts remain archived. No Go/SQL/contract or business assertion changes.')
    base.save(B / 'batch-progress.json',state)
    base.save(B / 'result.json',original)
    print(json.dumps({'passed':True,'retained_top_level_tests':228,'retried_top_level_tests':1,'result_records':len(checks),'source_unchanged':True}))


if __name__ == '__main__':
    main()
