"""Exercise real dependency consumers and preserve every attempt with exact inputs."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/dependency-integrity'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def sources():
    scopes = ['packages', 'package.json', 'package-lock.json', 'mobile/native/babel-primitives.test.cjs',
              'scripts/dependency_security.test.mjs', 'scripts/dependency_maintenance.test.mjs',
              'scripts/dependency_remediation.test.mjs', 'scripts/dependency_toolchain.test.mjs',
              'scripts/dependency_integrity.test.mjs', 'scripts/verify_dependency_integrity_security.py']
    raw = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', *scopes], cwd=APP)
    return sorted(set(name for name in raw.decode().split('\0') if name and (APP / name).is_file()))


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def run(command, output, errors=None, deadline=900):
    with subprocess.Popen(command, cwd=APP, stdout=output, stderr=errors or subprocess.STDOUT,
                          creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0,
                          start_new_session=os.name != 'nt') as process:
        try:
            return process.wait(timeout=deadline), False
        except subprocess.TimeoutExpired:
            if os.name == 'nt':
                subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=30)
            else:
                os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=30)
            return 124, True


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--focused', action='store_true')
    parser.add_argument('--label', required=True)
    args = parser.parse_args()
    assert re.fullmatch(r'[a-z0-9-]{1,40}', args.label)
    folder = E / 'attempts' / args.label
    folder.mkdir(parents=True, exist_ok=False)
    names = sources(); before = digest(names)
    with zipfile.ZipFile(folder / 'source-inputs.zip', 'w', zipfile.ZIP_DEFLATED) as archive:
        for name in names:
            archive.writestr(name, (APP / name).read_bytes())
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    node = shutil.which('node')
    assert npm and node
    commands = [('consumer-tests', [node, '--test', '--test-concurrency=1', 'scripts/dependency_integrity.test.mjs'], 16)] if args.focused else [
        ('security-tests', [npm, 'run', 'test:dependency-security'], 56),
        ('native-js-tests', [npm, 'run', 'test:native-js', '--workspace', '@copypaste/mobile'], 2),
    ]
    checks = []
    for label, command, expected in commands:
        log = APP / '.local' / f'dependency-integrity-{args.label}-{label}.log'
        with log.open('xb') as output:
            code, timed_out = run(command, output)
        text = log.read_text(encoding='utf-8', errors='replace')
        counts = {key: int(match[1]) if (match := re.search(r'^# ' + key + r' (\d+)$', text, re.M)) else None
                  for key in ['tests', 'pass', 'fail', 'skipped', 'cancelled']}
        public = folder / (label + '.log')
        public.write_text(text.replace(str(APP), '<app>').replace(APP.as_posix(), '<app>'), encoding='utf-8')
        passed = code == 0 and not timed_out and counts == {'tests': expected, 'pass': expected, 'fail': 0, 'skipped': 0, 'cancelled': 0}
        checks.append({'name': label, 'passed': passed, 'exit_code': code, 'timed_out': timed_out, 'deadline_seconds': 900,
                       'tests': counts, 'raw_log_sha256': sha(log), 'public_log_sha256': sha(public)})
    tree = APP / '.local' / f'dependency-integrity-{args.label}-tree.json'
    errors = APP / '.local' / f'dependency-integrity-{args.label}-tree-errors.log'
    with tree.open('xb') as output, errors.open('xb') as error_output:
        tree_code, tree_timeout = run([npm, 'ls', '--all', '--json'], output, error_output, 300)
    try:
        graph = json.loads(tree.read_bytes())
    except (ValueError, UnicodeError):
        graph = {'problems': ['Invalid tree JSON; exact original bytes remain in the private log.']}
    tree_file = folder / 'installed-tree.json'
    tree_file.write_text(json.dumps(graph, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    valid = tree_code == 0 and not tree_timeout and not graph.get('problems')
    report = {'passed': all(row['passed'] for row in checks) and valid and before == digest(names),
              'checked_at': datetime.now(timezone.utc).isoformat(), 'label': args.label, 'focused': args.focused,
              'source_sha256': before, 'sources': names, 'source_unchanged': before == digest(names),
              'source_archive_sha256': sha(folder / 'source-inputs.zip'), 'checks': checks,
              'node_version': subprocess.check_output([node, '--version']).decode().strip(),
              'npm_version': subprocess.check_output([npm, '--version']).decode().strip(),
              'tree_valid': valid, 'tree_exit_code': tree_code, 'tree_timed_out': tree_timeout,
              'tree_raw_sha256': sha(tree), 'tree_errors_sha256': sha(errors), 'tree_sha256': sha(tree_file),
              'full_goal_complete': False, 'production_ready': False}
    (folder / 'result.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    if report['passed'] and not args.focused:
        with (E / 'security-checks.json').open('x', encoding='utf-8') as output:
            output.write(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'passed': report['passed'], 'label': args.label, 'checks': [row['tests'] for row in checks], 'tree_valid': valid}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
