"""Preserve actual earlier passing clients and interrupted Metro before a mobile UX repair."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
import zipfile
from refund_fx_sources import APP, sources, digest

ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/refund-fx'
PUBLIC = E.parent / 'refund-fx-preliminary/02-before-mobile-validation'
PRIVATE = APP / '.local/refund-fx-preliminary/02-before-mobile-validation'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
assert not PUBLIC.exists() and not PRIVATE.exists()
assert not (E / 'build.json').exists(), 'Do not discard a completed native build.'
clients = json.loads((E / 'client-checks.json').read_text())
assert clients['passed'] and clients['source_unchanged']
PUBLIC.mkdir(parents=True)
PRIVATE.mkdir(parents=True)
names = sources()
before = digest(names)
archive = PRIVATE / 'source.zip'
with zipfile.ZipFile(archive, 'x', zipfile.ZIP_DEFLATED) as z:
    for n in names:
        z.write(APP / n, n)
assert digest(names) == before
records = []
for n in ('client-checks.json', 'web-build.json', 'mobile-build.json', 'runtime-build.json',
          'development-project-resources.json', 'development-runtime.json'):
    path = E / n
    if path.exists():
        target = PUBLIC / n
        shutil.move(path, target)
        records.append({'file':target.relative_to(ROOT).as_posix(), 'sha256':sha(target)})
logs = list((APP / '.local').glob('refund_fx-client-*.log'))
logs += [APP / '.local/refund_fx-skill-check.log', APP / '.local/refund_fx-android-final.log', APP / '.local/android/bundle.log']
for path in logs:
    if path.exists():
        target = PRIVATE / path.name
        shutil.move(path, target)
        records.append({'file':target.relative_to(ROOT).as_posix(), 'sha256':sha(target)})
report = {'checked_at':datetime.now(timezone.utc).isoformat(), 'counts_as_current_pass':False,
    'earlier_client_commands_passed':8, 'earlier_mobile_tests':413,
    'android_attempt':'Owned Metro deliberately stopped before application inputs changed; no completed native build claim.',
    'development_inspection':'Earlier inspection correctly failed two stale verifier assertions; no database change is needed.',
    'source_sha256':before, 'private_source_archive':archive.relative_to(ROOT).as_posix(), 'private_source_archive_sha256':sha(archive),
    'records':records, 'archive_script_sha256':sha(Path(__file__))}
(PUBLIC / 'preservation.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'preserved':len(records),'counts_as_current_pass':False}))
