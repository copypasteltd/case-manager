"""Run the ordinary immutable-lock install and record effective npm options."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import time

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/dependency-integrity'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def save(name, data):
    with (E / name).open('x', encoding='utf-8') as output:
        output.write(json.dumps(data, ensure_ascii=False, indent=2) + '\n')


def main():
    assert not (E / 'clean-install.json').exists()
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    node = shutil.which('node')
    assert npm and node
    flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
    expected = {'ignore-scripts': 'false', 'force': 'false', 'legacy-peer-deps': 'false', 'omit': '', 'include': ''}
    actual = {key: subprocess.check_output([npm, 'config', 'get', key], cwd=APP, creationflags=flags).decode().strip()
              for key in expected}
    save('install-config.json', {'passed': actual == expected, 'exit_code': 0, 'effective_values': actual,
                                'checked_at': datetime.now(timezone.utc).isoformat()})
    assert actual == expected
    lock = sha(APP / 'package-lock.json'); manifest = sha(APP / 'package.json')
    command = [npm, 'ci', '--no-audit', '--no-fund']
    log = APP / '.local/dependency-integrity-clean-install.log'
    started = time.monotonic()
    print('Starting standard npm ci; installation scripts are enabled.', flush=True)
    with log.open('xb') as output:
        with subprocess.Popen(command, cwd=APP, stdout=output, stderr=subprocess.STDOUT,
                              creationflags=flags, start_new_session=os.name != 'nt') as process:
            try:
                code = process.wait(timeout=3600); timed_out = False
            except subprocess.TimeoutExpired:
                if os.name == 'nt':
                    subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=30)
                else:
                    os.killpg(process.pid, signal.SIGKILL)
                process.wait(timeout=30); code = 124; timed_out = True
    record = {'passed': code == 0 and not timed_out and lock == sha(APP / 'package-lock.json')
                       and manifest == sha(APP / 'package.json'),
              'checked_at': datetime.now(timezone.utc).isoformat(), 'command': ['npm', *command[1:]],
              'exit_code': code, 'timed_out': timed_out, 'technical_deadline_seconds': 3600,
              'elapsed_seconds': round(time.monotonic() - started, 2), 'package_lock_sha256': lock,
              'package_manifest_sha256': manifest, 'lock_unchanged': lock == sha(APP / 'package-lock.json'),
              'manifest_unchanged': manifest == sha(APP / 'package.json'),
              'install_scripts_enabled': True, 'force_or_legacy_peer_flags': False,
              'node_version': subprocess.check_output([node, '--version']).decode().strip(),
              'npm_version': subprocess.check_output([npm, '--version']).decode().strip(),
              'raw_log_sha256': sha(log), 'scope': 'Native clean install; actual dependency-tree and consumer checks run separately.'}
    public = E / 'clean-install.log'
    public.write_text(log.read_text(encoding='utf-8', errors='replace').replace(str(APP), '<app>'), encoding='utf-8')
    record['public_log_sha256'] = sha(public)
    save('clean-install.json', record)
    print(json.dumps({'passed': record['passed'], 'exit_code': code, 'lock_unchanged': record['lock_unchanged']}), flush=True)
    return 0 if record['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
