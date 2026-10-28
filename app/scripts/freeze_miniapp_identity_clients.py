"""Adapt recorded fresh command results to the repository publication schemas.

This does not rerun or invent test cases. Every count comes from the saved
current-source command record and its byte-matching private raw log.
"""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import subprocess

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/miniapp-identity'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def module(name):
    spec = importlib.util.spec_from_file_location('mention_' + name, APP / 'scripts' / (name + '.py'))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


def save(name, data):
    data['derived_from'] = {'record': 'client-checks.json', 'sha256': sha(E / 'client-checks.json'),
        'summary_adapter_sha256': sha(Path(__file__)), 'adapter_reran_tests': False}
    (E / name).write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


record = json.loads((E / 'client-checks.json').read_text())
assert record['passed'] and record['source_unchanged'] and digest(record['source_files']) == record['source_sha256']
checks = {item['name']: item for item in record['checks']}
assert len(checks) == 8 and all(item['passed'] and item['exit_code'] == 0 for item in checks.values())
for item in checks.values():
    assert sha(APP / '.local' / Path(item['raw_log']).name) == item['raw_log_sha256']
stamp = datetime.now(timezone.utc).isoformat()
web_sources = sorted(set([APP / 'scripts/verify_web.py', APP / 'package.json', APP / 'package-lock.json'] +
    [p for folder in ('web', 'packages/design', 'packages/contracts') for p in (APP / folder).rglob('*')
     if p.is_file() and not any(part in ('node_modules', 'dist', '.taro', '.swc') for part in p.relative_to(APP).parts)
     and p.suffix in ('.ts', '.tsx', '.css', '.json', '.html')]), key=lambda p: p.relative_to(APP).as_posix())
names = [p.relative_to(APP).as_posix() for p in web_sources]
save('web-build.json', {'passed': True, 'checked_at': stamp, 'source_sha256': digest(names), 'sources': names,
    'tests': {'numPassedTests': checks['web-tests']['test_count'], 'numTotalTests': checks['web-tests']['test_count'], 'success': True},
    'commands': [checks[n] for n in ('typecheck', 'web-tests', 'web-build')], 'source_unchanged_during_commands': True})
mobile = module('verify_mobile')
log = (APP / '.local/miniapp-identity-mobile-tests.log').read_text()
for label in ('fail', 'cancelled', 'skipped'):
    values = re.findall(r'^# ' + label + r' (\d+)', log, re.M)
    assert values and not any(int(v) for v in values)
pages = {}
for target in ('weapp', 'tt'):
    config = APP / 'mobile/dist' / target / 'app.json'
    items = json.loads(config.read_text())['pages']
    assert len(items) == 20 and 'pages/detail/index' in items and 'pages/files/index' in items
    pages[target] = {'page_count': len(items), 'pages': items, 'app_json_sha256': sha(config)}
save('mobile-build.json', {'passed': True, 'checked_at': stamp, 'source_sha256': mobile.digest()[0], 'source_files': mobile.digest()[1],
    'tests': {'pass': checks['mobile-tests']['test_count'], 'fail': 0, 'cancelled': 0, 'skipped': 0},
    'commands': [checks[n] for n in ('typecheck', 'mobile-tests', 'wechat-build', 'douyin-build')], 'targets': pages,
    'source_unchanged_during_commands': True, 'api_origin': 'http://127.0.0.1:18080', 'profile': 'local', 'real_device': False})
runtime = module('verify_agent_runtime')
runtime_before = runtime.digest()[0]
skill_log = APP / '.local/miniapp-identity-skill-check.log'
with skill_log.open('w', encoding='utf-8') as output:
    skill = subprocess.run([shutil.which('npm.cmd' if os.name == 'nt' else 'npm'), 'run', 'check:skills', '--workspace', '@copypaste/agent-runtime'], cwd=APP,
        stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
assert skill.returncode == 0 and runtime_before == runtime.digest()[0]
save('runtime-build.json', {'status': 'passed', 'checked_at': stamp, 'source_sha256': runtime_before, 'source_files': runtime.digest()[1],
    'tests': {'pass': checks['runtime-tests']['test_count'], 'fail': 0, 'cancelled': 0, 'skipped': 0},
    'commands': [checks[n] for n in ('typecheck', 'runtime-tests', 'runtime-build')],
    'skills_check': {'passed': True, 'executed_now': True, 'raw_log_sha256': sha(skill_log)}, 'real_model': False})
assert digest(record['source_files']) == record['source_sha256']
print(json.dumps({'passed': True, 'web_tests': checks['web-tests']['test_count'], 'mobile_tests': checks['mobile-tests']['test_count'],
    'runtime_tests': checks['runtime-tests']['test_count'], 'adapted_summaries': 3, 'adapter_reran_tests': False}))
