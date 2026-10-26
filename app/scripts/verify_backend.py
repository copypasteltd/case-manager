"""Run isolated native tests and save an outcome report without raw test data."""
from datetime import datetime, timezone
import json
import hashlib
from pathlib import Path
import subprocess
import sys

APP = Path(__file__).resolve().parents[1]
def source_digest():
    digest=hashlib.sha256()
    backend=APP/'backend'
    for path in sorted(backend.rglob('*')):
        if path.is_file() and (path.suffix in ('.go','.sql') or path.name in ('go.mod','go.sum') or path.relative_to(backend).as_posix()=='internal/contract/openapi.json'):
            digest.update(path.relative_to(backend).as_posix().encode('utf-8')+b'\0')
            digest.update(path.read_bytes()+b'\0')
    return digest.hexdigest()

tested_source=source_digest()
raw = APP / '.local' / 'backend-verification.log'
raw.parent.mkdir(exist_ok=True)
checks = []
with raw.open('w', encoding='utf-8') as output:
    process = subprocess.Popen(
        [sys.executable, str(APP / 'scripts/test_backend.py'), '-json', '-timeout=20m'],
        cwd=APP, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
        encoding='utf-8', errors='replace')
    for line in process.stdout:
        output.write(line)
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get('Action') in ('pass', 'fail', 'skip') and event.get('Test'):
            checks.append({key: event[key] for key in ('Action', 'Package', 'Test', 'Elapsed') if key in event})
            if '/' not in event['Test']:
                print(event['Action'] + ': ' + event['Test'], flush=True)
    status = process.wait()
report = {
    'passed': status == 0 and bool(checks) and all(item['Action'] == 'pass' for item in checks) and source_digest()==tested_source,
    'source_sha256': tested_source,
    'source_unchanged_during_tests': source_digest()==tested_source,
    'executed_at': datetime.now(timezone.utc).isoformat(),
    'command': 'python app/scripts/verify_backend.py',
    'native_database': True,
    'raw_output': 'Ignored app/.local/backend-verification.log; not a document artifact',
    'checks': checks,
}
target = APP.parent / 'docs/quality/evidence/backend/result.json'
target.parent.mkdir(exist_ok=True, parents=True)
target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': report['passed'], 'recorded_test_results': len(checks)}), flush=True)
raise SystemExit(status if status else (0 if report['passed'] else 1))
