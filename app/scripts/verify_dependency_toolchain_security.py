"""Exercise actual CJS consumers and record exact source, logs and installed tree."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/dependency-toolchain'
INPUTS = ['package.json', 'package-lock.json', 'scripts/dependency_security.test.mjs',
          'scripts/dependency_maintenance.test.mjs', 'scripts/dependency_remediation.test.mjs', 'scripts/dependency_toolchain.test.mjs',
          'scripts/verify_dependency_toolchain_security.py', 'mobile/native/babel-primitives.test.cjs', 'mobile/config/index.ts']
for folder in ['archive-extraction', 'image-size-compat', 'latest-version-compat', 'template-download', 'template-minifier', 'dev-server-compat']:
    INPUTS += [str(p.relative_to(APP).as_posix()) for p in (APP / 'packages' / folder).rglob('*')
               if p.is_file() and 'node_modules' not in p.relative_to(APP).parts]
INPUTS = sorted(set(INPUTS))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest():
    value = hashlib.sha256()
    for name in INPUTS:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def run_logged(command, output, timeout, errors=subprocess.STDOUT):
    # npm.cmd owns descendants on Windows. Killing only its shell leaves test
    # servers alive; stop this exact process tree on a verifier deadline.
    with subprocess.Popen(command, cwd=APP, stdout=output, stderr=errors,
                          creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0,
                          start_new_session=os.name != 'nt') as process:
        try:
            return process.wait(timeout=timeout), False
        except subprocess.TimeoutExpired:
            if os.name == 'nt':
                subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=30)
            else:
                os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=30)
            return 124, True


def main():
    E.mkdir(parents=True, exist_ok=True)
    target = E / 'security-checks.json'
    assert not target.exists(), 'Archive any earlier result before running again.'
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    assert npm
    before = digest()
    checks = []
    for label, args, expected in [
        ('security-tests', ['run', 'test:dependency-security'], 40),
        ('native-js-tests', ['run', 'test:native-js', '--workspace', '@copypaste/mobile'], 2),
    ]:
        raw_log = APP / '.local' / ('dependency-toolchain-' + label + '.log')
        with raw_log.open('xb') as output:
            status, timed_out = run_logged([npm, *args], output, 900)
        raw = raw_log.read_bytes()
        text = raw.decode('utf-8', errors='replace')
        counts = {key: int(re.search(r'^# ' + key + r' (\d+)$', text, re.M)[1])
                  if re.search(r'^# ' + key + r' (\d+)$', text, re.M) else None
                  for key in ('tests', 'pass', 'fail', 'skipped', 'cancelled')}
        passed = status == 0 and not timed_out and counts['tests'] == counts['pass'] == expected
        passed = passed and all(counts[key] == 0 for key in ('fail', 'skipped', 'cancelled'))
        # Preserve original bytes privately; public logs omit this machine's paths.
        public = text.replace(str(APP), '<app>').replace(APP.as_posix(), '<app>')
        (E / (label + '.log')).write_text(public, encoding='utf-8')
        checks.append({'name': label, 'passed': bool(passed), 'exit_code': status, 'tests': counts,
                       'timed_out': timed_out, 'technical_deadline_seconds': 900,
                       'raw_log_sha256': sha(raw_log), 'public_log_sha256': sha(E / (label + '.log'))})
    tree_command = [npm, 'ls', 'image-size', '@copypaste/image-size-parser', 'download-git-repo',
                           'latest-version', '@copypaste/latest-version-upstream', 'got', 'git-clone',
                           'decompress', 'html-minifier', 'html-minifier-terser', '@copypaste/html-minifier-upstream', 'webpack-dev-server', '@copypaste/dev-server-upstream', 'webpack-dev-middleware', 'postcss', 'vite', '@vitejs/plugin-react', '--all', '--json']
    tree_log = APP / '.local/dependency-toolchain-tree.json'
    tree_errors = APP / '.local/dependency-toolchain-tree-errors.log'
    with tree_log.open('xb') as output, tree_errors.open('xb') as errors:
        tree_status, tree_timeout = run_logged(tree_command, output, 300, errors)
    try:
        tree_data = json.loads(tree_log.read_bytes())
    except (json.JSONDecodeError, UnicodeDecodeError):
        tree_data = {'problems': ['The tree command did not produce valid JSON; original output remains in the private raw log.']}
    (E / 'selected-dependency-tree.json').write_text(json.dumps(tree_data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    valid = tree_status == 0 and not tree_timeout and not tree_data.get('problems')
    result = {'passed': all(row['passed'] for row in checks) and valid and before == digest(),
              'checked_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': before,
              'sources': INPUTS, 'source_unchanged': before == digest(), 'checks': checks,
              'node_version': subprocess.check_output([shutil.which('node'), '--version']).decode().strip(),
              'npm_version': subprocess.check_output([npm, '--version']).decode().strip(),
              'selected_dependency_tree_valid': valid, 'selected_dependency_tree_exit_code': tree_status,
              'selected_dependency_tree_timed_out': tree_timeout, 'selected_dependency_tree_deadline_seconds': 300,
              'selected_dependency_tree_raw_sha256': sha(tree_log), 'selected_dependency_tree_stderr_sha256': sha(tree_errors),
              'selected_dependency_tree_sha256': sha(E / 'selected-dependency-tree.json'),
              'scope': 'Native synthetic consumer/security checks, actual Taro and Metro entry points. No device, emulator, network performance or production acceptance.'}
    target.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'checks': [(c['name'], c['tests']) for c in checks], 'tree_valid': valid}), flush=True)
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
