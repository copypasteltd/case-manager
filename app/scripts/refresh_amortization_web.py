"""Recheck the Web-only CSS import after all original client commands finish.

The original complete input digest is reconstructed in memory using the saved
single changed file. Unchanged mobile/runtime/mini-program results keep their
own original command records; this does not reclassify them as rerun commands.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/cost-amortization'
ORIGINAL = E / 'attempts/03-before-cost-style'
CHANGED = 'web/src/CostAmortization.tsx'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(names, previous=None):
    h = hashlib.sha256()
    for name in names:
        raw = previous if name == CHANGED and previous is not None else (APP / name).read_bytes()
        h.update(name.encode() + b'\0' + raw + b'\0')
    return h.hexdigest()


def main():
    record_path = E / 'client-checks.json'
    if not ORIGINAL.exists():
        old = json.loads(record_path.read_text(encoding='utf-8'))
        assert old['passed'] and old['source_unchanged'] and len(old['checks']) == 8
        assert digest(old['source_files']) == old['source_sha256']
        ORIGINAL.mkdir(parents=True)
        shutil.copyfile(record_path, ORIGINAL / 'client-checks.json')
        shutil.copyfile(APP / CHANGED, ORIGINAL / 'CostAmortization.tsx')
        before_text = (APP / CHANGED).read_text(encoding='utf-8')
        assert "import './financial-reports.css';" not in before_text
        (APP / CHANGED).write_text("import './financial-reports.css';\n" + before_text, encoding='utf-8')
    old = json.loads((ORIGINAL / 'client-checks.json').read_text(encoding='utf-8'))
    previous = (ORIGINAL / 'CostAmortization.tsx').read_bytes()
    assert old['passed'] and old['source_unchanged'] and len(old['checks']) == 8
    assert digest(old['source_files'], previous) == old['source_sha256'], 'An unrelated input changed; a targeted refresh is not valid.'
    expected = b"import './financial-reports.css';\r\n" + previous
    assert (APP / CHANGED).read_bytes() == expected
    for item in old['checks']:
        assert item['passed'] and item['exit_code'] == 0 and sha(APP / '.local' / Path(item['raw_log']).name) == item['raw_log_sha256']
    names = old['source_files']
    before = digest(names)
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    assert npm
    replacement = {}
    for label, args in [('typecheck', ['run', 'typecheck']), ('web-tests', ['run', 'test', '--workspace', '@copypaste/web']), ('web-build', ['run', 'build:web'])]:
        print('Refreshing Web: ' + label, flush=True)
        log = APP / '.local' / ('cost-amortization-web-refresh-' + label + '.log')
        started = datetime.now(timezone.utc).isoformat()
        with log.open('w', encoding='utf-8') as output:
            status = subprocess.run([npm, *args], cwd=APP, env={**os.environ, 'CI': 'true', 'NO_COLOR': '1', 'TARO_CHECK_UPDATE': 'false'},
                stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0, timeout=1200).returncode
        content = log.read_text(encoding='utf-8')
        count = None
        if label == 'web-tests':
            match = re.search(r'Tests\s+(\d+) passed \((\d+)\)', content)
            assert match and match[1] == match[2]
            count = int(match[1])
        replacement[label] = {'name': label, 'passed': status == 0, 'exit_code': status, 'executed_at': started, 'test_count': count,
                              'command': 'npm ' + ' '.join(args), 'raw_log_sha256': sha(log), 'raw_log': 'ignored app/.local/' + log.name,
                              'web_style_refresh_executed': True}
        assert status == 0 and digest(names) == before, 'The fresh Web check or unchanged-source guard failed.'
    checks = [replacement.get(v['name'], {**v, 'web_style_refresh_executed': False, 'same_increment_execution_retained': True}) for v in old['checks']]
    value = {'passed': True, 'source_sha256': before, 'source_after_sha256': digest(names), 'source_files': names, 'source_unchanged': True, 'checks': checks,
             'web_refresh': {'original_record': (ORIGINAL / 'client-checks.json').relative_to(APP.parent).as_posix(), 'original_record_sha256': sha(ORIGINAL / 'client-checks.json'),
                             'original_source_sha256': old['source_sha256'], 'changed_file': CHANGED, 'previous_file': (ORIGINAL / 'CostAmortization.tsx').relative_to(APP.parent).as_posix(),
                             'previous_file_sha256': sha(ORIGINAL / 'CostAmortization.tsx'), 'only_change': "import './financial-reports.css';", 'original_input_digest_reconstructed': True,
                             'fresh_commands': list(replacement), 'retained_commands': [v['name'] for v in old['checks'] if v['name'] not in replacement],
                             'verification_script_sha256': sha(Path(__file__))},
             'scope': 'All eight original increment106 commands completed. Only the Web CSS import changed afterward; typecheck/Web tests/build reran. Five unaffected mobile/runtime/mini-program command records retain their original106 execution with identical relevant inputs. No device, model, cloud or production acceptance.'}
    record_path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'fresh_web_commands': 3, 'retained_same_increment_commands': 5}))


if __name__ == '__main__':
    main()
