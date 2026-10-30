"""Build and seal native Linux server artifacts. Never read deployment secrets."""
import argparse
from datetime import datetime, timezone
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time
import zipfile

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
sys.path.insert(0, str(APP / 'deploy/release'))
from bundle import canonical, digest, file_sha, inventory, require, safe_name, write_archive
from catalog import SERVICES, nginx_site, render, unit


def inputs():
    scopes = ['app/backend', 'app/web', 'app/mobile', 'app/agent-runtime', 'app/skills', 'app/packages',
              'docs/skills', 'app/package.json', 'app/package-lock.json', 'app/deploy/release',
              'app/scripts/build_production_release.py', 'app/scripts/build_native_runtime_release.py',
              'app/scripts/native_ci_release.py', '.github/workflows/native-release.yml']
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--', *scopes], cwd=ROOT).decode().split('\0')
    return [{'path': name, 'sha256': file_sha(ROOT / name)} for name in sorted(set(names)) if name and (ROOT / name).is_file()]


def native_tool(name):
    local = APP / '.tools' / ('go/bin/go.exe' if name == 'go' and os.name == 'nt' else 'go/bin/go' if name == 'go' else 'node-v22.23.4-win-x64/npm.cmd')
    found = str(local) if local.is_file() else shutil.which(name + '.cmd' if name == 'npm' and os.name == 'nt' else name)
    require(found is not None, 'Install the declared native build tools first.')
    return found


def command(args, cwd, log, extra=None):
    env = {**os.environ, **(extra or {})}
    if os.name == 'nt':
        env['PATH'] = str(APP / '.tools/node-v22.23.4-win-x64') + os.pathsep + env.get('PATH', '')
    began = time.monotonic()
    with log.open('xb') as output:
        result = subprocess.run(args, cwd=cwd, env=env, stdin=subprocess.DEVNULL, stdout=output, stderr=subprocess.STDOUT)
    require(result.returncode == 0, 'Native artifact build failed; retain its private log.')
    return {'command': [Path(args[0]).name, *args[1:]], 'exit_code': result.returncode,
            'elapsed_seconds': round(time.monotonic() - began, 3), 'raw_log_sha256': file_sha(log)}


def copy_public_tree(source, destination):
    source = Path(source)
    require(source.is_dir() and not source.is_symlink(), 'Compiled component directory missing.')
    for path in sorted(source.rglob('*')):
        require(not path.is_symlink(), 'Compiled component symlink refused.')
        if path.is_dir():
            continue
        relative = safe_name(path.relative_to(source).as_posix())
        require(not path.name.startswith('.env') and path.suffix not in ('.key', '.pem', '.pfx', '.jks', '.keystore'), 'Private compiled input refused.')
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(path, target)


def build(output):
    output = Path(output).absolute()
    require(not output.exists() and (APP / '.local').resolve() in output.resolve().parents, 'Use a new output directory within app/.local.')
    output.mkdir(parents=True, mode=0o700)
    source = inputs()
    source_sha = digest(canonical(source))
    (output / 'source-before.json').write_bytes(canonical(source))
    with zipfile.ZipFile(output / 'source-before.zip', 'x', zipfile.ZIP_DEFLATED) as saved:
        for row in source:
            saved.write(ROOT / row['path'], row['path'])
    base_commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip()
    dirty = bool(subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=all'], cwd=ROOT))
    package, logs = output / 'package', output / 'private-logs'
    package.mkdir()
    logs.mkdir(mode=0o700)
    (package / 'bin').mkdir()
    commands = []
    go, npm = native_tool('go'), native_tool('npm')
    version_env = {**os.environ, 'PATH': str(APP / '.tools/node-v22.23.4-win-x64') + os.pathsep + os.environ.get('PATH', '')} if os.name == 'nt' else None
    versions = {name: subprocess.check_output(cmd, env=version_env).decode().strip() for name, cmd in
                [('go', [go, 'version']), ('npm', [npm, '--version']), ('node', ['node', '--version'])]}
    for name in ('platform', 'agent-native', 'status-bridge'):
        # Fixed inputs; no local absolute paths or dirty VCS marker embedded in binaries.
        commands.append(command([go, 'build', '-trimpath', '-buildvcs=false', '-o', str(package / 'bin' / name), './cmd/' + name],
                                APP / 'backend', logs / (name + '.log'), {'GOOS': 'linux', 'GOARCH': 'amd64', 'CGO_ENABLED': '0'}))
        raw = (package / 'bin' / name).read_bytes()
        require(raw[:5] == b'\x7fELF\x02' and raw[18:20] == b'\x3e\x00', 'Unexpected native server architecture.')
    for name in ('web', 'runtime'):
        commands.append(command([npm, 'run', 'build:' + name], APP, logs / (name + '.log')))
    copy_public_tree(APP / 'web/dist', package / 'web')
    specification = importlib.util.spec_from_file_location('production_runtime_packaging', APP / 'scripts/build_native_runtime_release.py')
    runtime_packaging = importlib.util.module_from_spec(specification)
    specification.loader.exec_module(runtime_packaging)
    runtime = runtime_packaging.create_release(output / 'runtime-input')
    copy_public_tree(output / 'runtime-input', package / 'runtime')
    metadata = package / 'metadata'
    metadata.mkdir()
    # Normalize the ancillary packaging clock; artifact identity is byte/content based.
    (metadata / 'runtime.json').write_bytes(canonical({k: v for k, v in runtime.items() if k != 'created_at'}))
    (metadata / 'source-inputs.json').write_bytes(canonical(source))
    tools = metadata / 'release-tools'
    tools.mkdir()
    for name in ('bundle.py', 'catalog.py', 'controller.py'):
        shutil.copyfile(APP / 'deploy/release' / name, tools / name)
    (metadata / 'nginx.conf.template').write_text(nginx_site('case.example.invalid'), encoding='utf-8', newline='\n')
    units = package / 'systemd'
    units.mkdir()
    for name in SERVICES:
        (units / unit(name)).write_text(render(name), encoding='utf-8', newline='\n')
    for name, subcommand in [('agent-native-launch', 'launch'), ('agent-native-probe', 'probe')]:
        (package / 'bin' / name).write_text('#!/bin/sh\nexec /opt/copypaste/current/bin/agent-native ' + subcommand +
            ' --socket /var/lib/copypaste/agent-native/control.sock "$@"\n', encoding='utf-8', newline='\n')
    schema = [{'name': p.name, 'sha256': file_sha(p)} for p in sorted((APP / 'backend/migrations').glob('*.sql'))]
    manifest = {'format': 1, 'target': 'linux-amd64', 'source': {'base_commit': base_commit, 'inputs_sha256': source_sha, 'working_tree_dirty': dirty},
                'schema': schema, 'files': inventory(package)}
    (package / 'release.json').write_bytes(canonical(manifest))
    require(inputs() == source, 'Build inputs changed during packaging; do not publish this attempt.')
    archive = output / 'copypaste-linux-amd64.tar.gz'
    receipt = write_archive(package, archive)
    report = {'passed': True, 'created_at': datetime.now(timezone.utc).isoformat(), **receipt,
              'archive': archive.relative_to(ROOT).as_posix(), 'source': manifest['source'], 'source_files': source,
              'files': len(manifest['files']), 'bytes': sum(p['bytes'] for p in manifest['files']), 'migrations': len(schema),
              'commands': commands, 'all_skills_remain_draft': runtime['all_skills_remain_draft'],
              'tool_versions': versions, 'private_source_archive': (output / 'source-before.zip').relative_to(ROOT).as_posix(),
              'source_archive_sha256': file_sha(output / 'source-before.zip'),
              'linux_executed': False, 'systemd_executed': False, 'deployed': False, 'production_ready': False}
    (output / 'build.json').write_bytes(canonical(report))
    (output / 'manifest.sha256').write_text(receipt['manifest_sha256'] + '\n', encoding='ascii')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    report = build(args.output)
    print(json.dumps({k: report[k] for k in ('passed', 'release_id', 'manifest_sha256', 'files', 'migrations', 'production_ready')}))


if __name__ == '__main__':
    main()
