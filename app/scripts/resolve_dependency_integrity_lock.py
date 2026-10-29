"""Regenerate a portable candidate with npm in an isolated source checkout."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/dependency-integrity'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    local = (APP / '.local').resolve()
    task = local / 'dependency-integrity-resolution03'
    assert task.parent == local and not task.exists()
    project = task / 'project'
    project.mkdir(parents=True)
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', '.'], cwd=APP).decode().split('\0')
    source_files = []
    for name in sorted(set(v for v in names if v and (APP / v).is_file())):
        target = project / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes((APP / name).read_bytes())
        source_files.append({'file': name, 'sha256': sha(target)})
    original = sha(APP / 'package-lock.json')
    lock = json.loads((project / 'package-lock.json').read_text(encoding='utf-8'))
    removed = {}
    for name in list(lock['packages']):
        leaf = name.split('node_modules/')[-1]
        shadow = leaf in ['deepmerge', 'decode-uri-component'] and name != 'node_modules/' + leaf
        malformed = name.startswith('node_modules/') and '/packages/uri-decoder-compat' in name
        if shadow or malformed:
            removed[name] = lock['packages'].pop(name)
    assert removed
    (project / 'package-lock.json').write_text(json.dumps(lock, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'isolated_source_files': len(source_files), 'stale_candidate_entries': list(removed)}), flush=True)
    log = task / 'npm-resolution.log'
    command = [shutil.which('npm.cmd' if os.name == 'nt' else 'npm'), 'install', '--package-lock-only', '--no-audit', '--no-fund']
    started = time.monotonic()
    with log.open('xb') as output:
        with subprocess.Popen(command, cwd=project, env={**os.environ, 'CI': 'true', 'npm_config_prefer_offline': 'true'},
                              stdout=output, stderr=subprocess.STDOUT,
                              creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0) as process:
            try:
                code = process.wait(timeout=900); timed_out = False
            except subprocess.TimeoutExpired:
                if os.name == 'nt':
                    subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=30)
                else:
                    process.kill()
                process.wait(timeout=30); code = 124; timed_out = True
    generated = json.loads((project / 'package-lock.json').read_text(encoding='utf-8'))
    aliases = {'deepmerge': 'packages/deepmerge-safe', 'decode-uri-component': 'packages/uri-decoder-compat'}
    valid = all(row.get('link') and row.get('resolved') == aliases[name.split('node_modules/')[-1]]
                for name, row in generated['packages'].items() if name.split('node_modules/')[-1] in aliases)
    portable = str(task).encode() not in (project / 'package-lock.json').read_bytes()
    passed = code == 0 and not timed_out and valid and portable and sha(APP / 'package-lock.json') == original
    report = {'passed': passed, 'checked_at': datetime.now(timezone.utc).isoformat(), 'command': ['npm', *command[1:]],
              'exit_code': code, 'timed_out': timed_out, 'elapsed_seconds': round(time.monotonic() - started, 2),
              'previous_lock_sha256': original, 'candidate_lock_sha256': sha(project / 'package-lock.json'),
              'isolated_source_files': source_files, 'removed_stale_candidate_entries': removed,
              'aliases_portable_and_exact': valid and portable, 'raw_log_sha256': sha(log),
              'candidate_published_to_worktree': passed, 'force_or_legacy_peer_flags': False,
              'scope': 'Candidate generation only. A normal clean CI and actual installed tree are still required.'}
    if passed:
        (APP / 'package-lock.json').write_bytes((project / 'package-lock.json').read_bytes())
    with (E / 'resolution03.json').open('x', encoding='utf-8') as output:
        output.write(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({key: report[key] for key in ['passed', 'exit_code', 'candidate_lock_sha256', 'aliases_portable_and_exact']}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
