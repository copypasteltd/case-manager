"""Verify the actual native dispatcher command refuses an unconfigured launch."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/telegram-dispatch'


def main():
    values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    if values.get('CP_TELEGRAM_DISPATCH_BOTS', '') not in ('', 'disabled'):
        raise SystemExit('The owned dispatcher must remain disabled for this startup check.')
    spec = importlib.util.spec_from_file_location('dispatch_backend_digest', APP / 'scripts/verify_backend_batches.py')
    backend = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(backend)
    before = backend.digest()
    flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
    raw = APP / '.local/telegram-dispatch-startup.log'
    environment = {**os.environ, 'CP_TELEGRAM_DISPATCH_BOTS': 'disabled'}
    with raw.open('w', encoding='utf-8') as output:
        child = subprocess.Popen([sys.executable, str(APP / 'scripts/run_backend.py'), 'worker-telegram'], cwd=APP,
                                 env=environment, stdout=output, stderr=subprocess.STDOUT, creationflags=flags)
        timed_out = False
        try:
            status = child.wait(timeout=60)
        except subprocess.TimeoutExpired:
            timed_out = True
            if os.name == 'nt':
                subprocess.run(['taskkill', '/PID', str(child.pid), '/T', '/F'], capture_output=True, creationflags=flags)
            else:
                child.kill()
            status = child.wait(timeout=10)
    matched = 'worker-telegram requires explicit CP_TELEGRAM_DISPATCH_BOTS' in raw.read_text(encoding='utf-8', errors='replace')
    passed = not timed_out and status != 0 and matched and child.poll() is not None and backend.digest() == before
    files = [Path(__file__).resolve(), APP / 'scripts/run_backend.py']
    report = {'passed': passed, 'checked_at': datetime.now(timezone.utc).isoformat(), 'backend_source_sha256': before,
              'command': 'python app/scripts/run_backend.py worker-telegram', 'configuration': 'explicitly disabled',
              'exit_code': status, 'timed_out': timed_out, 'expected_configuration_error_observed': matched,
              'owned_process_exited': child.poll() is not None,
              'sources': {file.relative_to(APP).as_posix(): hashlib.sha256(file.read_bytes()).hexdigest() for file in files},
              'scope': 'Actual native command startup only; no dispatcher loop, model, Telegram or production activation.'}
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    (EVIDENCE / 'dispatcher-startup.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': passed, 'expected_configuration_error_observed': matched, 'owned_process_exited': child.poll() is not None}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
