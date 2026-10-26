"""Build both mini-program targets with a deliberately non-production origin."""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time
from datetime import datetime, timezone

APP = Path(__file__).resolve().parents[1]
group = sys.argv[1] if len(sys.argv)==2 else 'mobile-foundation'
if len(sys.argv)>2 or not group.replace('-','').isalnum():
    raise SystemExit('Usage: python app/scripts/verify_mobile.py [evidence-group]')
REPORT = APP.parent / 'docs/quality/evidence' / group / ('result.json' if group=='mobile-foundation' else 'mobile-build.json')
LOCAL = APP / '.local'


def digest():
    files = [APP / 'package.json', APP / 'package-lock.json', APP / 'scripts/sync_design.py', Path(__file__).resolve()]
    for directory, directories, names in os.walk(APP / 'mobile'):
        directories[:] = [name for name in directories if name not in {'node_modules', 'dist', '.taro', '.swc', 'android', 'native', '.gradle', 'build'}]
        files += [Path(directory) / name for name in names]
    files += list((APP / 'packages/contracts/src').rglob('*.ts'))
    files += list((APP / 'packages/design/src').glob('*'))
    result = hashlib.sha256()
    for path in sorted(set(files)):
        result.update(path.relative_to(APP.parent).as_posix().encode() + b'\0' + path.read_bytes() + b'\0')
    return result.hexdigest(), len(set(files))


def main():
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    if not npm:
        raise SystemExit('Native npm is required.')
    LOCAL.mkdir(exist_ok=True)
    before, count = digest()
    started = datetime.now(timezone.utc).isoformat()
    commands = []
    tests = None
    environment = {**os.environ, 'CP_MOBILE_BUILD_PROFILE': 'local', 'CP_MOBILE_API_ORIGIN': 'http://127.0.0.1:18080', 'CI': 'true', 'TARO_CHECK_UPDATE': 'false'}
    log_path = LOCAL / 'mobile-verification.log'
    flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
    with log_path.open('w', encoding='utf-8') as log:
        for script in ('test', 'typecheck', 'build:weapp', 'build:tt'):
            print('Running native mobile ' + script, flush=True)
            log.write('\n' + script + '\n')
            log.flush()
            offset = log.tell()
            began = time.monotonic()
            status, timed_out = 1, False
            # A file avoids inherited pipe handles keeping communicate() open
            # after an npm/CLI process has already exited on native Windows.
            process = subprocess.Popen([npm, 'run', script, '--workspace', '@copypaste/mobile'], cwd=APP,
                env=environment, stdout=log, stderr=subprocess.STDOUT, creationflags=flags)
            try:
                status = process.wait(timeout=480)
            except subprocess.TimeoutExpired:
                timed_out = True
                if os.name == 'nt':
                    subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, creationflags=flags, timeout=15)
                else:
                    process.kill()
                process.wait(timeout=15)
                status = 124
            log.flush()
            with log_path.open('r', encoding='utf-8', errors='replace') as recorded:
                recorded.seek(offset)
                combined = recorded.read()
            commands.append({'script': script, 'exit_code': status, 'timed_out': timed_out,
                             'elapsed_seconds': round(time.monotonic() - began, 3)})
            print(script + ': ' + ('passed' if status == 0 else 'failed'), flush=True)
            if script == 'test':
                tests = {name: sum(int(value) for value in re.findall(r'^# ' + name + r' (\d+)\s*$', combined, re.MULTILINE)) for name in ('tests', 'pass', 'fail', 'cancelled', 'skipped')}
                tests['suites'] = len(re.findall(r'^# tests (\d+)\s*$', combined, re.MULTILINE))
            if status:
                # Windows GBK consoles must not prevent saving a failed report.
                print(combined[-5000:].encode(sys.stdout.encoding or 'utf-8', errors='backslashreplace').decode(sys.stdout.encoding or 'utf-8'), flush=True)
                break
    after, _ = digest()
    artifacts = {}
    for target in ('weapp', 'tt'):
        root = APP / 'mobile/dist' / target
        files = list(root.rglob('*')) if root.exists() else []
        files = [p for p in files if p.is_file()]
        artifacts[target] = {'files': len(files), 'bytes': sum(p.stat().st_size for p in files), 'app_json_exists': (root / 'app.json').is_file(), 'status_page_exists': (root / 'pages/status/index.js').is_file()}
    passed = len(commands) == 4 and all(item['exit_code'] == 0 for item in commands) and before == after and tests is not None and tests.get('tests', 0) > 0 and tests.get('tests') == tests.get('pass') and tests.get('cancelled') == 0 and tests.get('fail') == 0 and tests.get('skipped') == 0 and all(item['app_json_exists'] and item['status_page_exists'] for item in artifacts.values())
    result = {'started_at': started, 'completed_at': datetime.now(timezone.utc).isoformat(), 'commands': commands, 'tests': tests, 'source_sha256': before, 'source_files': count, 'source_unchanged_during_run': before == after, 'artifacts': artifacts, 'environment': 'Native Windows Node/TypeScript/Taro; no Docker, WSL or emulator', 'taro_version': '4.3.1', 'api_origin': environment['CP_MOBILE_API_ORIGIN'], 'build_profile': 'local', 'actual_mini_program_runtime_invoked': False, 'actual_android_runtime_invoked': False, 'actual_cloud_invoked': False, 'scope': 'Portable memory-only session, shared draft budget, intent-key, fixed-content reader and business-input validation/serialization tests; TypeScript and two mini-program compilation targets. Excludes real devices, native document/file-system execution, platform submission, Android native transfer and end-to-end acceptance', 'passed': passed}
    REPORT.parent.mkdir(parents=True, exist_ok=True)
    REPORT.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(result, ensure_ascii=False))
    raise SystemExit(0 if passed else 1)


if __name__ == '__main__':
    main()
