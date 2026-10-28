"""Check the real minimal release with native Node and retain exact inventory hashes."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import build_native_runtime_release as builder

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/native-agent-runtime'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    release = APP / '.local/native-agent/runtime-release-v2'
    manifest = json.loads(release.with_name(release.name + '.manifest.json').read_text(encoding='utf-8'))
    assert manifest['release_sha256'] == builder.release_digest(release)
    assert all(sha(release / item['file']) == item['sha256'] == sha(ROOT / item['file']) for item in manifest['files'])
    commands = []
    log = APP / '.local/native-agent/package-checks.log'
    with log.open('wb') as output:
        result = subprocess.run([sys.executable, str(APP / 'scripts/test_native_runtime_release.py')], stdout=output, stderr=subprocess.STDOUT)
        commands.append({'name': 'packaging-boundaries', 'exit_code': result.returncode, 'tests': 4})
    value = subprocess.run(['node', str(release / 'app/agent-runtime/dist/check-skills.js')], capture_output=True, text=True, encoding='utf-8')
    commands.append({'name': 'packaged-skills-cli', 'exit_code': value.returncode})
    skills = json.loads(value.stdout) if value.returncode == 0 else {}
    request = {'skill_id': 'project-briefing', 'profile_id': '00000000-0000-4000-8000-000000000000', 'model': 'operator-selected-model',
               'working_directory': '/var/lib/copypaste/agent-work', 'mcp_endpoint': 'http://127.0.0.1:18081/internal/v1/agent-mcp', 'api_base_url': 'http://127.0.0.1:18082/v1'}
    value = subprocess.run(['node', str(release / 'app/agent-runtime/dist/describe-profile.js')], input=json.dumps(request), capture_output=True, text=True, encoding='utf-8')
    commands.append({'name': 'packaged-profile-cli', 'exit_code': value.returncode})
    profile = json.loads(value.stdout) if value.returncode == 0 else {}
    names = [item['file'] for item in manifest['files']]
    safe = all(not any(part in {'tests', '.local', '.tools', '.git', 'vendor'} or part.startswith('.env') for part in Path(name).parts)
               and Path(name).suffix not in {'.exe', '.dll', '.so', '.node'} and not name.endswith('.test.js') for name in names)
    passed = all(v['exit_code'] == 0 for v in commands) and len(names) == manifest['file_count'] and safe and skills.get('count') == 8 and all(s['state'] == 'draft' for s in skills['skills']) and not profile.get('missing_tools') and profile.get('skill_state') == 'draft' and profile.get('ready_for_controlled_installation') is False
    report = {'passed': passed, 'checked_at': datetime.now(timezone.utc).isoformat(), 'commands': commands,
              'release_sha256': manifest['release_sha256'], 'inventory': manifest['files'], 'packages': manifest['packages'], 'file_count': len(names), 'bytes': manifest['bytes'],
              'packager_sha256': sha(APP / 'scripts/build_native_runtime_release.py'), 'tests_sha256': sha(APP / 'scripts/test_native_runtime_release.py'),
              'verification_script_sha256': sha(Path(__file__)), 'package_lock_sha256': sha(APP / 'package-lock.json'),
              'all_packaged_bytes_match_current_inputs': True, 'private_or_native_paths_excluded': safe, 'packaged_skill_count': skills.get('count'),
              'all_skills_remain_draft': manifest['all_skills_remain_draft'], 'profile_configuration_sha256': profile.get('profile', {}).get('configuration_sha256'),
              'linux_ownership_validated': False, 'kernel_boundary_executed': False, 'real_model_invoked': False}
    E.mkdir(parents=True, exist_ok=True)
    (E / 'package-checks.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': passed, 'files': len(names), 'packaged_skills': skills.get('count')}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
