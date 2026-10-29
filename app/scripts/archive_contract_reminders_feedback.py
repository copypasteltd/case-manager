"""Keep the failed actual mobile journey and its exact inputs before fixing feedback."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/contract-reminders'
browser = E.parent / 'contract-reminders-mobile'
attempt = E / 'attempts/05-mobile-validation-feedback'
private = APP / '.local/contract-reminders-before-feedback'
read = lambda p: json.loads(p.read_text(encoding='utf-8'))
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
result, manifest = read(browser / 'result.json'), read(browser / 'source-manifest.json')
clients = read(E / 'client-checks.json')
assert not result['passed'] and result['owned_browser_closed'] and result['isolated_database_cleanup_passed']
assert clients['passed'] and clients['source_unchanged']
attempt.mkdir(parents=True, exist_ok=False)
private.mkdir(exist_ok=False)
archive = private / 'exact-inputs.zip'
names = sorted(set(clients['source_files'] + manifest['files']))
with zipfile.ZipFile(archive, 'x', compression=zipfile.ZIP_DEFLATED) as output:
    for name in names:
        output.write(APP / name, name)
logs = []
for check in clients['checks']:
    path = APP / '.local' / Path(check['raw_log']).name
    assert sha(path) == check['raw_log_sha256']
    target = private / path.name
    path.rename(target)
    logs.append({'file': target.relative_to(APP).as_posix(), 'sha256': sha(target)})
for suffix in ('native', 'browser', 'build'):
    path = APP / '.local' / ('contract-reminders-mobile-' + suffix + '.log')
    target = private / path.name
    path.rename(target)
    logs.append({'file': target.relative_to(APP).as_posix(), 'sha256': sha(target)})
for path in list(browser.iterdir()):
    path.rename(attempt / path.name)
browser.rmdir()
for name in ('client-checks.json', 'web-build.json', 'mobile-build.json', 'runtime-build.json'):
    (E / name).rename(attempt / name)
record = {
    'passed': False, 'recorded_at': datetime.now(timezone.utc).isoformat(),
    'reason': 'Actual mobile validation rejected duplicate lead days, but the generic unknown-error renderer hid the actionable local validation message. Preserve the failed UI journey and the successful pre-repair client run; fix the form boundary and rerun current-source client and browser checks.',
    'original_client_commands_passed': True, 'original_mobile_tests': 299,
    'browser_passed': False, 'owned_browser_closed': result['owned_browser_closed'],
    'isolated_database_cleanup_passed': result['isolated_database_cleanup_passed'],
    'browser_record_sha256': sha(attempt / 'result.json'),
    'client_record_sha256': sha(attempt / 'client-checks.json'),
    'source_archive': archive.relative_to(APP).as_posix(),
    'source_archive_sha256': sha(archive), 'raw_logs': logs,
    'archiver_sha256': sha(Path(__file__)),
}
(attempt / 'attempt.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'preserved_failed_browser': True, 'preserved_client_commands': 8, 'source_files': len(names)}))
