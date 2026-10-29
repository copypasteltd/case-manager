"""Execute client checks once, with complete input digests and private raw logs."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys

APP = Path(__file__).resolve().parents[1]
TARGET = APP.parent / 'docs/quality/evidence/payment-fx/client-checks.json'
if TARGET.exists():
    raise SystemExit('Archive the previous client attempt and exact logs before a fresh run.')


def sources():
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--',
        'web', 'mobile', 'packages', 'agent-runtime', 'package.json', 'package-lock.json', 'scripts/verify_payment_fx_clients.py'], cwd=APP).decode().split('\0')
    return sorted(set(name for name in names if name and (APP / name).is_file()))


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


names = sources()
before = digest(names)
npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
assert npm
flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
checks = []
commands = [
    ('typecheck', ['run', 'typecheck']),
    ('web-tests', ['run', 'test', '--workspace', '@copypaste/web']),
    ('mobile-tests', ['run', 'test', '--workspace', '@copypaste/mobile']),
    ('runtime-tests', ['run', 'test', '--workspace', '@copypaste/agent-runtime']),
    ('web-build', ['run', 'build:web']),
    ('runtime-build', ['run', 'build:runtime']),
    ('wechat-build', ['run', 'build:weapp']),
    ('douyin-build', ['run', 'build:tt']),
]
for label, command in commands:
    log = APP / '.local' / ('payment_fx-client-' + label + '.log')
    started = datetime.now(timezone.utc).isoformat()
    print('Running ' + label, flush=True)
    timed_out = False
    with log.open('x', encoding='utf-8') as output:
        with subprocess.Popen([npm, *command], cwd=APP,
            env={**os.environ, 'CI': 'true', 'NO_COLOR': '1', 'TARO_CHECK_UPDATE': 'false', 'CP_MOBILE_BUILD_PROFILE':'local', 'CP_MOBILE_API_ORIGIN':'http://127.0.0.1:18080'},
            stdout=output, stderr=subprocess.STDOUT, creationflags=flags,
            start_new_session=os.name != 'nt') as process:
            try:
                status = process.wait(timeout=900)
            except subprocess.TimeoutExpired:
                timed_out = True
                if os.name == 'nt':
                    subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=30)
                else:
                    os.killpg(process.pid, signal.SIGKILL)
                process.wait(timeout=30)
                status = 124
    text = re.sub(r'\x1b\[[0-9;]*m', '', log.read_text(encoding='utf-8'))
    counts = None
    if label == 'web-tests':
        found = re.search(r'Tests\s+(\d+) passed', text)
        counts = int(found[1]) if found else None
    elif label in ('mobile-tests', 'runtime-tests'):
        totals = [int(v) for v in re.findall(r'^# tests (\d+)', text, re.M)]
        passed = [int(v) for v in re.findall(r'^# pass (\d+)', text, re.M)]
        failed = [int(v) for v in re.findall(r'^# fail (\d+)', text, re.M)]
        counts = sum(totals) if totals and totals == passed and failed and not any(failed) else None
    if label.endswith('-tests') and not counts:
        status = status or 1
    checks.append({'name': label, 'passed': status == 0, 'exit_code': status, 'executed_at': started, 'test_count': counts,
        'timed_out': timed_out, 'technical_deadline_seconds': 900,
        'command': 'npm ' + ' '.join(command), 'raw_log_sha256': hashlib.sha256(log.read_bytes()).hexdigest(), 'raw_log': 'ignored app/.local/' + log.name})
    result = {'passed': all(c['passed'] for c in checks) and digest(names) == before and len(checks) == len(commands),
        'source_sha256': before, 'source_after_sha256': digest(names), 'source_files': names, 'source_unchanged': digest(names) == before,
        'checks': checks, 'node_version': subprocess.check_output([shutil.which('node'), '--version']).decode().strip(), 'scope': 'Fresh local test/build commands; no real-device, store, model, cloud or production acceptance.'}
    TARGET.parent.mkdir(parents=True, exist_ok=True)
    TARGET.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    if status:
        print(text[-2500:], flush=True)
        raise SystemExit(status)
print(json.dumps({'passed': result['passed'], 'checks': len(checks), 'source_unchanged': result['source_unchanged']}), flush=True)
raise SystemExit(0 if result['passed'] else 1)
