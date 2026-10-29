"""Preserve the successful mobile and failed Web captures before repairing a DOM-only check."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile
from contract_reminders_sources import APP, digest, sources

ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/contract-reminders'
attempt = E / 'attempts/06-web-layout-adapter'
private = APP / '.local/contract-reminders-before-layout-adapter'
read = lambda p: json.loads(p.read_text(encoding='utf-8'))
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
results = {client: read(E.parent / ('contract-reminders-' + client) / 'result.json') for client in ('web', 'mobile')}
assert not results['web']['passed'] and len(results['web']['checks']) == 4
assert results['mobile']['passed'] and len(results['mobile']['checks']) == 5
assert all(v['owned_browser_closed'] and v['owned_review_server_stopped'] and v['isolated_database_cleanup_passed'] and v['build_source_sha256'] == digest() for v in results.values())
assert not (APP / '.local/business-browser-fixture.json').exists()
attempt.mkdir(parents=True, exist_ok=False)
private.mkdir(exist_ok=False)
archive = private / 'exact-inputs.zip'
names = set(sources())
for client in results:
    names.update(read(E.parent / ('contract-reminders-' + client) / 'source-manifest.json')['files'])
with zipfile.ZipFile(archive, 'x', compression=zipfile.ZIP_DEFLATED) as output:
    for name in sorted(names): output.write(APP / name, name)
logs, media = [], []
for client in results:
    folder = E.parent / ('contract-reminders-' + client)
    assert folder.resolve().is_relative_to((ROOT / 'docs/quality/evidence').resolve())
    target = attempt / client
    assert target.resolve().is_relative_to(attempt.resolve()) and not target.exists()
    folder.rename(target)
    for suffix in ('native', 'browser', 'build'):
        path = APP / '.local' / ('contract-reminders-' + client + '-' + suffix + '.log')
        saved = private / path.name
        path.rename(saved)
        logs.append({'file': saved.relative_to(APP).as_posix(), 'sha256': sha(saved)})
    frames = APP / '.local/repository-showcase' / ('contract-reminders-' + client)
    saved = private / ('frames-' + client)
    assert frames.resolve().is_relative_to((APP / '.local/repository-showcase').resolve())
    assert saved.resolve().is_relative_to(private.resolve()) and not saved.exists()
    frames.rename(saved)
    logs.append({'file': (saved / 'frames.json').relative_to(APP).as_posix(), 'sha256': sha(saved / 'frames.json')})
    for screenshot in (ROOT / 'docs/media/screenshots').glob('contract-reminders-' + client + '-*.png'):
        saved = attempt / 'screenshots' / screenshot.name
        saved.parent.mkdir(exist_ok=True)
        screenshot.rename(saved)
        media.append({'file': saved.relative_to(ROOT).as_posix(), 'sha256': sha(saved)})
record = {'passed': False, 'recorded_at': datetime.now(timezone.utc).isoformat(),
    'reason': 'Web policy publication, persisted history and stale-version rejection passed, but the width adapter called Page.evaluate with a DOM-node callback. Repair only the layout locator; preserve the passing mobile capture too because the shared checker digest changes. Both actual journeys will be recaptured at the final helper source.',
    'application_source_changed': False, 'web_business_groups_passed': 4, 'mobile_groups_passed': 5,
    'web_record_sha256': sha(attempt / 'web/result.json'), 'mobile_record_sha256': sha(attempt / 'mobile/result.json'),
    'source_archive': archive.relative_to(APP).as_posix(), 'source_archive_sha256': sha(archive),
    'private_logs_and_manifests': logs, 'screenshots': media, 'archiver_sha256': sha(Path(__file__))}
(attempt / 'attempt.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'preserved_failed_web': True, 'preserved_passed_mobile': True, 'screenshots': len(media), 'application_source_changed': False}))
