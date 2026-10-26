"""Native Web tests and production build with a fixed, recorded source manifest."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time

APP = Path(__file__).resolve().parents[1]
if len(sys.argv) != 2 or not sys.argv[1].replace('-', '').isalnum():
    raise SystemExit('Usage: python app/scripts/verify_web.py <evidence-group>')
target = APP.parent / 'docs/quality/evidence' / sys.argv[1] / 'web-build.json'
npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
if not npm:
    raise SystemExit('Native npm is required.')
local = APP / '.local'
local.mkdir(exist_ok=True)
report_path = local / (sys.argv[1] + '-web-tests.json')
report_path.unlink(missing_ok=True)
sources = sorted(set([Path(__file__).resolve(), APP / 'package.json', APP / 'package-lock.json'] +
    [p for folder in ('web', 'packages/design', 'packages/contracts') for p in (APP / folder).rglob('*')
     if p.is_file() and not any(part in ('node_modules', 'dist', '.taro', '.swc') for part in p.relative_to(APP).parts)
     and p.suffix in ('.ts', '.tsx', '.css', '.json', '.html')]), key=lambda p: p.relative_to(APP).as_posix())


def digest():
    value = hashlib.sha256()
    for path in sources:
        value.update(path.relative_to(APP).as_posix().encode('utf-8') + b'\0' + path.read_bytes() + b'\0')
    return value.hexdigest()


started, before = datetime.now(timezone.utc).isoformat(), digest()
commands = []
for name, args in (
    ('test', ['run', 'test', '--workspace', '@copypaste/web', '--', '--reporter=json', '--outputFile=' + str(report_path)]),
    ('build:web', ['run', 'build:web']),
):
    began = time.monotonic()
    with (local / (sys.argv[1] + '-web-' + name.replace(':', '-') + '.log')).open('w', encoding='utf-8') as log:
        result = subprocess.run([npm, *args], cwd=APP, stdout=log, stderr=subprocess.STDOUT,
            creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    commands.append({'script': name, 'exit_code': result.returncode, 'elapsed_seconds': round(time.monotonic() - began, 3)})
    print(name + ': ' + ('passed' if result.returncode == 0 else 'failed'), flush=True)
    if result.returncode:
        break
tests = json.loads(report_path.read_text(encoding='utf-8')) if report_path.exists() else {}
after = digest()
passed = len(commands) == 2 and all(c['exit_code'] == 0 for c in commands) and before == after
passed = passed and tests.get('success', False) and tests.get('numTotalTests', 0) > 0 and tests.get('numPendingTests') == 0
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(json.dumps({'passed': passed, 'started_at': started, 'completed_at': datetime.now(timezone.utc).isoformat(),
    'environment': 'Native Windows Node/Vitest/TypeScript/Vite; no virtualization or browser',
    'commands': commands, 'tests': {k: tests.get(k) for k in ('numTotalTestSuites', 'numTotalTests', 'numPassedTests', 'numFailedTests', 'numPendingTests')},
    'source_file_count': len(sources), 'source_sha256': before, 'source_unchanged_during_run': before == after,
    'sources': [p.relative_to(APP).as_posix() for p in sources], 'production_deployment': False}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': passed, 'tests': tests.get('numTotalTests', 0), 'source_file_count': len(sources)}))
raise SystemExit(0 if passed else 1)
