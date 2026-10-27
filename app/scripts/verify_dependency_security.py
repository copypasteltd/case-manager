"""Run bounded security/compatibility checks and retain exact logs and source bytes."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/dependency-hardening'
SOURCES = ['package.json', 'package-lock.json', 'mobile/package.json', 'mobile/native/babel-primitives.test.cjs',
    'packages/archive-extraction/package.json', 'packages/archive-extraction/index.cjs',
    'scripts/dependency_security.test.mjs', 'scripts/verify_dependency_security.py']


def digest():
    result = hashlib.sha256()
    for name in sorted(SOURCES):
        result.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return result.hexdigest()


def main():
    E.mkdir(parents=True, exist_ok=True)
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    assert npm
    before = digest()
    checks = []
    for label, args, expected in [
        ('security-tests', ['run','test:dependency-security'], 12),
        ('native-js-tests', ['run','test:native-js','--workspace','@copypaste/mobile'], None),
    ]:
        run = subprocess.run([npm, *args], cwd=APP, capture_output=True, timeout=90)
        raw = run.stdout + run.stderr
        target = E / (label + '.log')
        target.write_bytes(raw)
        text = raw.decode('utf-8', errors='replace')
        counts = {key: int(re.search(r'^# ' + key + r' (\d+)$', text, re.M).group(1))
            if re.search(r'^# ' + key + r' (\d+)$', text, re.M) else None
            for key in ('tests','pass','fail','skipped','cancelled')}
        passed = run.returncode == 0 and counts['tests'] and counts['tests'] == counts['pass']
        passed = passed and all(counts[k] == 0 for k in ('fail','skipped','cancelled'))
        passed = passed and (expected is None or counts['tests'] == expected)
        checks.append({'name':label, 'passed':bool(passed), 'exit_code':run.returncode,
            'tests':counts, 'log_sha256':hashlib.sha256(raw).hexdigest()})
    tree = subprocess.run([npm, 'ls', 'webpack','decompress','@xhmikosr/decompress','tar','swiper',
        'adm-zip','lodash-es','serialize-javascript','--all','--json'], cwd=APP, capture_output=True, timeout=60)
    assert tree.returncode == 0, 'Selected overridden dependency tree contains invalid versions.'
    json.loads(tree.stdout)
    (E / 'selected-dependency-tree.json').write_bytes(tree.stdout)
    report = {'passed': all(c['passed'] for c in checks) and before == digest(),
        'checked_at':datetime.now(timezone.utc).isoformat(), 'source_sha256':before,
        'sources':sorted(SOURCES), 'source_unchanged':before == digest(), 'checks':checks,
        'selected_dependency_tree_valid':True, 'selected_dependency_tree_sha256':hashlib.sha256(tree.stdout).hexdigest(),
        'scope':'Bounded synthetic native filesystem/network cases and actual upstream CJS download caller. No resource-exhaustion payload, device, cloud or production security acceptance.'}
    (E / 'security-checks.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':report['passed'],'checks':[(c['name'],c['tests']['pass']) for c in checks]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
