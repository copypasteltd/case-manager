"""Seal the current native server package; inspect it without Linux or deployment."""
import argparse
import json
from pathlib import Path
import subprocess
import sys
from development_wave import APP, ROOT, Wave, sha, write_once
from build_production_release import build
from catalog import RELEASE_TOOLS, SERVICES, render, unit


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--attempt', required=True)
    args = parser.parse_args()
    import re
    if not re.fullmatch('[a-z0-9-]{1,48}', args.attempt):
        raise ValueError('Use a safe immutable attempt name.')
    wave = Wave(168)
    target = wave.evidence / ('production-' + args.attempt + '.json')
    output = APP / '.local/batch-round-168' / ('production-' + args.attempt)
    if target.exists() or output.exists():
        raise ValueError('Preserve the earlier package and evidence; use a new attempt.')
    # The builder archives and checks all its actual source consumers before/after.
    report = build(output)
    package = output / 'package'
    manifest = json.loads((package / 'release.json').read_text(encoding='utf-8'))
    tools = package / 'metadata/release-tools'
    program = ('import runpy,sys;sys.path.insert(0,sys.argv[1]);'
               'sys.argv=[sys.argv[1]+"/controller.py","--help"];'
               'runpy.run_path(sys.argv[0],run_name="__main__")')
    result = subprocess.run([sys.executable, '-I', '-B', '-c', program, str(tools)],
                            cwd=output, capture_output=True, text=True, timeout=30)
    modules = {name: sha(tools / name) for name in RELEASE_TOOLS}
    matched = all(value == sha(APP / 'deploy/release' / name) for name, value in modules.items())
    units = all((package / 'systemd' / unit(name)).read_text(encoding='utf-8') == render(name) for name in SERVICES)
    migrations = sorted((APP / 'backend/migrations').glob('*.sql'))
    exact_schema = (len(manifest['schema']) == len(migrations) == 136 and
                    manifest['schema'] == [{'name': p.name, 'sha256': sha(p)} for p in migrations])
    elf = (package / 'bin/platform').read_bytes()[:4] == b'\x7fELF'
    binding = {row['path']: row['sha256'] for row in report['source_files']}
    current = all((ROOT / name).is_file() and sha(ROOT / name) == value for name, value in binding.items())
    passed = report['passed'] and result.returncode == 0 and 'activate' in result.stdout and 'recover' in result.stdout and matched and units and exact_schema and elf and current
    record = {'passed': passed, 'schema': 136, 'build': report, 'source_file_sha256': binding,
              'installed_cli_exit_code': result.returncode, 'isolated_import_without_repository': True,
              'installed_modules': modules, 'all_controlled_units_match': units,
              'artifact_file_count': len(manifest['files']), 'native_linux_executed': False,
              'production_deployed': False, 'android_js_built': False, 'apk_built': False,
              'virtualization_used': False}
    write_once(target, record)
    print(json.dumps({'passed': passed, 'schema': 136, 'installed_modules': len(modules),
                      'artifact_files': len(manifest['files'])}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
