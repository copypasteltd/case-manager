"""Record native runtime checks against fixed code, schemas and linked skill instructions."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time

APP = Path(__file__).resolve().parents[1]
group = sys.argv[1] if len(sys.argv)==2 else 'secretary-execution'
if len(sys.argv)>2 or not group.replace('-','').isalnum():
    raise SystemExit('Usage: python app/scripts/verify_agent_runtime.py [evidence-group]')
REPORT = APP.parent / 'docs/quality/evidence' / group / 'runtime-build.json'


def sources():
    files = [Path(__file__).resolve(), APP / 'package.json', APP / 'package-lock.json', APP / 'agent-runtime/package.json', APP / 'agent-runtime/tsconfig.json']
    files += list((APP / 'agent-runtime/src').rglob('*.ts'))
    files += list((APP / 'skills').rglob('*.json'))
    files += [p for p in (APP.parent / 'docs/skills').rglob('*') if p.is_file()]
    return sorted(set(files), key=lambda p: p.relative_to(APP.parent).as_posix())


def digest():
    value = hashlib.sha256()
    paths = sources()
    for path in paths:
        value.update(path.relative_to(APP.parent).as_posix().encode('utf-8') + b'\0' + path.read_bytes() + b'\0')
    return value.hexdigest(), len(paths)


def main():
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    if not npm:
        raise SystemExit('Native npm is required.')
    before, count = digest()
    started = datetime.now(timezone.utc).isoformat()
    commands = []
    tests = None
    for name in ('test', 'check:skills'):
        began = time.monotonic()
        log = APP / '.local' / ('agent-runtime-' + name.replace(':', '-') + '.log')
        with log.open('w', encoding='utf-8') as output:
            result = subprocess.run([npm, 'run', name, '--workspace', '@copypaste/agent-runtime'], cwd=APP, stdout=output, stderr=subprocess.STDOUT,
                                    creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
        command = {'name': name, 'exit_code': result.returncode, 'elapsed_seconds': round(time.monotonic() - began, 3)}
        commands.append(command)
        if name == 'test':
            raw = log.read_text(encoding='utf-8', errors='replace')
            tests = {key: int(re.search(r'^# ' + key + r' (\d+)$', raw, re.M).group(1)) if re.search(r'^# ' + key + r' (\d+)$', raw, re.M) else None
                     for key in ('tests', 'pass', 'fail', 'skipped', 'cancelled')}
        if result.returncode:
            break
    after, _ = digest()
    passed = len(commands) == 2 and all(c['exit_code'] == 0 for c in commands) and before == after and tests and tests['tests'] and tests['tests'] == tests['pass'] and tests['fail'] == tests['skipped'] == tests['cancelled'] == 0
    report = {'status': 'passed' if passed else 'failed', 'started_at': started, 'finished_at': datetime.now(timezone.utc).isoformat(),
              'source_sha256': before, 'source_after_sha256': after, 'source_files': count, 'commands': commands, 'tests': tests,
              'execution_mode': 'Controlled SDK clients and loopback HTTP only; no real provider invocation or production activation.'}
    REPORT.parent.mkdir(parents=True, exist_ok=True)
    REPORT.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'status': report['status'], 'tests': tests, 'source_unchanged': before == after}, ensure_ascii=False))
    raise SystemExit(0 if passed else 1)


if __name__ == '__main__':
    main()
