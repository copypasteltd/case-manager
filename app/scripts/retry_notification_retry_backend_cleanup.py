"""Retry completed batches only when their sole failure is native fixture DROP timeout.

No source or assertion is relaxed. The first complete attempt and every original
log are archived before serial retries; successful unchanged batches are retained.
"""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import zipfile

import verify_backend_batches as base

APP = base.APP
E = APP.parent / 'docs/quality/evidence/notification-retry-backend'
ATTEMPT = E / 'attempts/01-native-cleanup-timeout'


def read(p):
    return json.loads(p.read_text(encoding='utf-8'))


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def events(p):
    out = []
    for line in p.read_text(encoding='utf-8').splitlines():
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            pass
    return out


def checks(rows):
    return [{k:v[k] for k in ('Action', 'Package', 'Test', 'Elapsed') if k in v}
            for v in rows if v.get('Action') in ('pass', 'fail', 'skip') and v.get('Test')]


def main():
    assert not (APP / '.local/business-browser-fixture.json').exists(), 'Do not overlap native database check phases.'
    original, state = read(E / 'result.json'), read(E / 'batch-progress.json')
    source = base.digest()
    assert not original['passed'] and original['source_unchanged_during_tests'] and original['all_discovered_tests_executed']
    assert original['source_sha256'] == state['source_sha256'] == source
    assert all(b['status'] in ('passed', 'failed') for b in state['batches'])
    failed = [i for i,b in enumerate(state['batches']) if b['status'] == 'failed']
    assert failed and not ATTEMPT.exists()
    for index in failed:
        raw = APP / '.local' / ('notification-retry-backend-batch-' + str(index + 1) + '.log')
        rows = events(raw)
        assert sha(raw) == state['batches'][index]['raw_output_sha256']
        assert checks(rows) == state['batches'][index]['checks']
        outputs = [r.get('Output', '') for r in rows if r.get('Action') == 'output']
        errors = [v for v in outputs if re.search(r'\.go:\d+:', v)]
        assert errors and all('server_integration_test.go:86: timeout: context deadline exceeded' in v for v in errors), 'This helper cannot dismiss a business assertion or another error.'
    private = APP / '.local/notification-retry-attempts/backend01-complete'
    private.mkdir(parents=True, exist_ok=False)
    ATTEMPT.mkdir(parents=True)
    for name in ('result.json', 'batch-progress.json'):
        shutil.copyfile(E / name, ATTEMPT / name)
    raw_records = []
    for index,batch in enumerate(state['batches']):
        raw = APP / '.local' / ('notification-retry-backend-batch-' + str(index + 1) + '.log')
        assert sha(raw) == batch['raw_output_sha256'] and checks(events(raw)) == batch['checks']
        saved = private / raw.name; shutil.copyfile(raw, saved)
        raw_records.append({'batch':index + 1, 'file':saved.relative_to(APP).as_posix(), 'sha256':sha(saved)})
    with zipfile.ZipFile(private / 'source-inputs.zip', 'w', zipfile.ZIP_DEFLATED, compresslevel=1) as archive:
        for p in sorted((APP / 'backend').rglob('*')):
            if p.is_file() and (p.suffix in ('.go', '.sql') or p.name in ('go.mod', 'go.sum') or p.relative_to(APP / 'backend').as_posix() == 'internal/contract/openapi.json'):
                archive.write(p, p.relative_to(APP).as_posix())
    base.save(ATTEMPT / 'attempt.json', {'passed':False, 'terminal_exit_code':1, 'complete':True,
        'source_sha256':source, 'raw_logs':raw_records, 'source_archive_sha256':sha(private / 'source-inputs.zip'),
        'failed_batches':[i+1 for i in failed], 'reason':'Native fixture DROP exceeded its independent two-minute cleanup context; no business assertion or source change is ignored.', 'not_current_pass_evidence':True})
    retried_families = []
    for index in failed:
        prior = state['batches'][index]
        failed_names = sorted({v['Test'].split('/')[0] for v in prior['checks'] if v['Action'] == 'fail'})
        required = {tuple(v) for v in state['expected_tests'] if v[1] in failed_names}
        families = [{'Package':package,'Test':test} for package,test in sorted(required)]
        retained = [v for v in prior['checks'] if (v['Package'],v['Test'].split('/')[0]) not in required]
        assert all(v['Action'] == 'pass' for v in retained)
        print('Serial cleanup retry for ' + ', '.join(failed_names), flush=True)
        batch = {'names':prior['names'], 'status':'running', 'checks':[], 'started_at':datetime.now(timezone.utc).isoformat(), 'cleanup_retry':True}
        raw = APP / '.local' / ('notification-retry-backend-retry-' + str(index + 1) + '.log')
        assert not raw.exists(), 'Retain each retry attempt under its original name.'
        assert base.digest() == source
        pattern = '^(' + '|'.join(re.escape(name) for name in failed_names) + ')$'
        with raw.open('w', encoding='utf-8') as output:
            code = subprocess.run(base.command('-run', pattern, '-timeout=20m'), cwd=APP, stdout=output, stderr=subprocess.STDOUT).returncode
        fresh = checks(events(raw))
        observed = {(r['Package'], r['Test']) for r in fresh if '/' not in r['Test']}
        unchanged = base.digest() == source
        passed = code == 0 and unchanged and observed == required and all(v['Action'] == 'pass' for v in fresh)
        batch['checks'] = retained + fresh
        batch.update(status='passed' if passed else 'failed', exit_code=code, source_unchanged_during_tests=unchanged,
            completed_at=datetime.now(timezone.utc).isoformat(), raw_output='ignored app/.local/' + raw.name, raw_output_sha256=sha(raw))
        batch['evidence_segments'] = [
            {'raw_log':(private / ('notification-retry-backend-batch-' + str(index + 1) + '.log')).relative_to(APP).as_posix(),
             'sha256':sha(private / ('notification-retry-backend-batch-' + str(index + 1) + '.log')),
             'excluded_test_families':families, 'selected_records':len(retained), 'kind':'retained_successful_families'},
            {'raw_log':raw.relative_to(APP).as_posix(),'sha256':sha(raw),'excluded_test_families':[],
             'selected_records':len(fresh),'kind':'rerun_failed_cleanup_families'}]
        retried_families.extend(families)
        state['batches'][index] = batch
        base.save(E / 'batch-progress.json', state)
        if not passed:
            raise SystemExit('Serial retry did not pass; all attempts remain preserved.')
    result_checks = [v for b in state['batches'] for v in b['checks']]
    observed = {(v['Package'], v['Test']) for v in result_checks if '/' not in v['Test']}
    passed = source == base.digest() and observed == {tuple(v) for v in state['expected_tests']} and all(b['status'] == 'passed' for b in state['batches']) and all(v['Action'] == 'pass' for v in result_checks)
    original.update(passed=passed, checks=result_checks, executed_at=datetime.now(timezone.utc).isoformat(),
        cleanup_retry={'initial_record':(ATTEMPT / 'result.json').relative_to(APP.parent).as_posix(),
            'initial_record_sha256':sha(ATTEMPT / 'result.json'), 'attempt_record_sha256':sha(ATTEMPT / 'attempt.json'),
            'serial_retried_batches':[i+1 for i in failed], 'retained_successful_batches':len(state['batches'])-len(failed),
            'retried_test_families':retried_families, 'successful_families_in_failed_batches_retained':True,
            'same_source_sha256':source, 'verification_script_sha256':sha(Path(__file__))},
        execution='All current tests executed at identical source. Only failed top-level cleanup test families reran serially; every successful family retains its original exact log segment. Failed original attempts remain archived; no assertion/source change.')
    base.save(E / 'result.json', original)
    print(json.dumps({'passed':passed, 'retried_batches':[i+1 for i in failed], 'records':len(result_checks)}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
