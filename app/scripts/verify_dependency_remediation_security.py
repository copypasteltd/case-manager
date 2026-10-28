"""Exercise actual CJS consumers and record exact source, logs and installed tree."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/dependency-remediation'
INPUTS = ['package.json', 'package-lock.json', 'scripts/dependency_security.test.mjs',
          'scripts/dependency_maintenance.test.mjs', 'scripts/dependency_remediation.test.mjs',
          'scripts/verify_dependency_remediation_security.py', 'mobile/native/babel-primitives.test.cjs']
for folder in ['archive-extraction', 'image-size-compat', 'latest-version-compat', 'template-download']:
    INPUTS += [str(p.relative_to(APP).as_posix()) for p in (APP / 'packages' / folder).glob('*') if p.is_file()]
INPUTS = sorted(set(INPUTS))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest():
    value = hashlib.sha256()
    for name in INPUTS:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def main():
    E.mkdir(parents=True, exist_ok=True)
    target = E / 'security-checks.json'
    assert not target.exists(), 'Archive any earlier result before running again.'
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    assert npm
    before = digest()
    checks = []
    for label, args, expected in [
        ('security-tests', ['run', 'test:dependency-security'], 31),
        ('native-js-tests', ['run', 'test:native-js', '--workspace', '@copypaste/mobile'], 2),
    ]:
        raw_log = APP / '.local' / ('dependency-remediation-' + label + '.log')
        with raw_log.open('xb') as output:
            run = subprocess.run([npm, *args], cwd=APP, stdout=output, stderr=subprocess.STDOUT, timeout=180)
        raw = raw_log.read_bytes()
        text = raw.decode('utf-8', errors='replace')
        counts = {key: int(re.search(r'^# ' + key + r' (\d+)$', text, re.M)[1])
                  if re.search(r'^# ' + key + r' (\d+)$', text, re.M) else None
                  for key in ('tests', 'pass', 'fail', 'skipped', 'cancelled')}
        passed = run.returncode == 0 and counts['tests'] == counts['pass'] == expected
        passed = passed and all(counts[key] == 0 for key in ('fail', 'skipped', 'cancelled'))
        # Preserve original bytes privately; public logs omit this machine's paths.
        public = text.replace(str(APP), '<app>').replace(APP.as_posix(), '<app>')
        (E / (label + '.log')).write_text(public, encoding='utf-8')
        checks.append({'name': label, 'passed': bool(passed), 'exit_code': run.returncode, 'tests': counts,
                       'raw_log_sha256': sha(raw_log), 'public_log_sha256': sha(E / (label + '.log'))})
    tree = subprocess.run([npm, 'ls', 'image-size', '@copypaste/image-size-parser', 'download-git-repo',
                           'latest-version', '@copypaste/latest-version-upstream', 'got', 'git-clone',
                           'decompress', '--all', '--json'], cwd=APP, capture_output=True, timeout=60)
    tree_data = json.loads(tree.stdout)
    (E / 'selected-dependency-tree.json').write_bytes(tree.stdout)
    valid = tree.returncode == 0 and not tree_data.get('problems')
    result = {'passed': all(row['passed'] for row in checks) and valid and before == digest(),
              'checked_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': before,
              'sources': INPUTS, 'source_unchanged': before == digest(), 'checks': checks,
              'selected_dependency_tree_valid': valid, 'selected_dependency_tree_exit_code': tree.returncode,
              'selected_dependency_tree_sha256': sha(E / 'selected-dependency-tree.json'),
              'scope': 'Native synthetic consumer/security checks, actual Taro and Metro entry points. No device, emulator, network performance or production acceptance.'}
    target.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'checks': [(c['name'], c['tests']) for c in checks], 'tree_valid': valid}), flush=True)
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
