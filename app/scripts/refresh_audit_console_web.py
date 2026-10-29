"""Recheck a Web-only layout correction, preserving unaffected command provenance."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/audit-console'
ORIGINAL = E / 'attempts/03-before-web-layout'
CHANGED = ['web/src/AuditConsole.tsx', 'web/src/App.tsx']


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def digest(names, prior=False):
    h = hashlib.sha256()
    for name in names:
        file = ORIGINAL / Path(name).name if prior and name in CHANGED else APP / name
        h.update(name.encode() + b'\0' + file.read_bytes() + b'\0')
    return h.hexdigest()


def main():
    target = E / 'client-checks.json'
    assert not ORIGINAL.exists(), 'Archive this refresh attempt before trying again.'
    old = json.loads(target.read_text(encoding='utf-8'))
    assert old['passed'] and old['source_unchanged'] and len(old['checks']) == 8
    assert digest(old['source_files']) == old['source_sha256']
    ORIGINAL.mkdir(parents=True)
    shutil.copyfile(target, ORIGINAL / 'client-checks.json')
    for name in CHANGED:
        shutil.copyfile(APP / name, ORIGINAL / Path(name).name)
    file = APP / CHANGED[0]
    source = file.read_text(encoding='utf-8')
    needle = 'className="button secondary" disabled={!!busy} onClick={() => void download(item)}'
    assert source.count(needle) == 1
    file.write_text(source.replace(needle, 'style={{ alignSelf: \'center\' }} ' + needle), encoding='utf-8', newline='\n')
    file = APP / CHANGED[1]
    source = file.read_bytes()
    needle = b"location.pathname === '/administration' ?"
    assert source.count(needle) == 1
    file.write_bytes(source.replace(needle, "location.pathname.startsWith('/audit') ? '审计记录' : ".encode() + needle))
    assert digest(old['source_files'], True) == old['source_sha256']
    for row in old['checks']:
        assert row['passed'] and row['exit_code'] == 0 and sha(APP / '.local' / Path(row['raw_log']).name) == row['raw_log_sha256']
    before = digest(old['source_files'])
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    assert npm
    replacement = {}
    for name, command in [('typecheck', ['run', 'typecheck']), ('web-tests', ['run', 'test', '--workspace', '@copypaste/web']), ('web-build', ['run', 'build:web'])]:
        log = APP / '.local' / ('audit-console-web-refresh-' + name + '.log')
        started = datetime.now(timezone.utc).isoformat()
        print('Refreshing ' + name, flush=True)
        with log.open('x', encoding='utf-8') as output:
            code = subprocess.run([npm, *command], cwd=APP, env={**os.environ, 'CI':'true', 'NO_COLOR':'1'},
                stdout=output, stderr=subprocess.STDOUT, timeout=900,
                creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0).returncode
        count = None
        if name == 'web-tests':
            match = re.search(r'Tests\s+(\d+) passed \((\d+)\)', log.read_text(encoding='utf-8'))
            assert match and match[1] == match[2]
            count = int(match[1])
        replacement[name] = {'name':name, 'passed':code == 0, 'exit_code':code, 'executed_at':started,
            'test_count':count, 'command':'npm ' + ' '.join(command), 'raw_log_sha256':sha(log),
            'raw_log':'ignored app/.local/' + log.name, 'web_layout_refresh_executed':True}
        assert code == 0 and digest(old['source_files']) == before
    old.update(source_sha256=before, source_after_sha256=digest(old['source_files']),
        checks=[replacement.get(row['name'], {**row, 'same_increment_execution_retained':True}) for row in old['checks']],
        web_refresh={'original_record':(ORIGINAL / 'client-checks.json').relative_to(APP.parent).as_posix(),
            'original_record_sha256':sha(ORIGINAL / 'client-checks.json'),
            'original_source_sha256':json.loads((ORIGINAL / 'client-checks.json').read_text())['source_sha256'],
            'changed_files':CHANGED, 'previous_files':[{ 'file':(ORIGINAL / Path(name).name).relative_to(APP.parent).as_posix(), 'source':name, 'sha256':sha(ORIGINAL / Path(name).name)} for name in CHANGED],
            'fresh_commands':list(replacement), 'retained_commands':[row['name'] for row in old['checks'] if row['name'] not in replacement],
            'original_input_digest_reconstructed':True, 'verification_script_sha256':sha(Path(__file__))},
        scope='All eight original commands passed. Only Web download-button alignment and its audit breadcrumb changed; typecheck, Web tests and Web build ran again. Five unaffected mobile/runtime/miniapp commands retain their original execution and exact inputs. No device, live cloud/model or production acceptance.')
    target.write_text(json.dumps(old, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed':True, 'fresh_web_commands':3, 'retained_commands':5}))


if __name__ == '__main__':
    main()
