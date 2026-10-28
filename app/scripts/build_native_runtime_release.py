"""Build a dedicated, inspectable JS/Skill release; never package the checkout.

No npm lifecycle scripts, downloads, model calls, containers or secret files are
used. Native Node/Codex/broker executables are pinned separately by Linux policy.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess

APP = Path(__file__).resolve().parents[1]
OMIT = {'node_modules', 'test', 'tests', '__tests__', 'fixtures', 'coverage', '.github', '.git', 'vendor', '.cache'}
TEXT = {'.js', '.mjs', '.cjs', '.json', '.md', '.txt', '.ts', '.yml', '.yaml'}


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def inside(base, path):
    return path == base or base in path.parents


def copy_file(source, destination, allowed):
    resolved = source.resolve(strict=True)
    if not inside(allowed.resolve(), resolved) or not resolved.is_file() or resolved.stat().st_size > 8 << 20:
        raise ValueError('Refusing an outside, special or oversized release input.')
    if destination.exists():
        if sha(destination) != sha(resolved):
            raise ValueError('Conflicting dependency content at one destination.')
        return
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(resolved, destination)
    destination.chmod(0o644)


def copy_tree(source, destination, allowed, package=False, ancestors=()):
    resolved = source.resolve(strict=True)
    if resolved in ancestors or not inside(allowed.resolve(), resolved):
        raise ValueError('Refusing a cyclic or outside package path.')
    for child in sorted(source.iterdir(), key=lambda p: p.name):
        if child.name in OMIT or child.name.startswith('.env') or child.name in {'.local', '.tools'}:
            continue
        if child.is_dir():
            copy_tree(child, destination / child.name, allowed, package, (*ancestors, resolved))
        elif not package or child.suffix in TEXT or child.name.upper().startswith(('LICENSE', 'LICENCE', 'NOTICE', 'COPYING')):
            copy_file(child, destination / child.name, allowed)


def resolve_package(name, start, modules):
    if not name or '..' in name.split('/') or len(name.split('/')) not in (1, 2) or name.startswith('/'):
        raise ValueError('Invalid package name.')
    current = start
    while inside(modules.parent, current):
        candidate = current / 'node_modules' / name
        if (candidate / 'package.json').is_file():
            if not inside(modules, candidate.resolve()):
                raise ValueError('Runtime dependency resolves outside installed modules.')
            return candidate
        if current == modules.parent:
            break
        current = current.parent
    raise ValueError('A required installed runtime dependency is missing.')


def release_digest(root):
    value = hashlib.sha256()

    def visit(directory):
        for path in sorted(directory.iterdir(), key=lambda p: p.name):
            if path.is_symlink():
                raise ValueError('Generated releases contain no symlinks.')
            if path.is_dir():
                visit(path)
            elif path.is_file():
                value.update(path.relative_to(root).as_posix().encode() + b'\0file\0')
                value.update(path.read_bytes() + b'\0')
            else:
                raise ValueError('Special release file refused.')
    visit(root)
    return value.hexdigest()


def create_release(output, app=APP):
    app = app.resolve()
    modules = (app / 'node_modules').resolve(strict=True)
    output = output.resolve()
    if output.exists() or output.with_name(output.name + '.manifest.json').exists() or inside(app.parent, output) and not inside(app / '.local', output):
        raise ValueError('Use a new dedicated output directory outside source and dependencies.')
    output.parent.mkdir(parents=True, exist_ok=True)
    output.mkdir(mode=0o755)
    for name in ('package.json', 'package-lock.json', 'agent-runtime/package.json'):
        copy_file(app / name, output / 'app' / name, app)
    compiled = app / 'agent-runtime/dist'
    if not (compiled / 'worker.js').is_file():
        raise ValueError('Build the runtime before creating its release.')
    for path in sorted(compiled.glob('*.js')):
        if not path.name.endswith('.test.js'):
            copy_file(path, output / 'app/agent-runtime/dist' / path.name, compiled)
    copy_tree(app / 'skills', output / 'app/skills', app / 'skills')
    copy_tree(app.parent / 'docs/skills', output / 'docs/skills', app.parent / 'docs/skills')
    pending = [(name, app / 'agent-runtime') for name in json.loads((app / 'agent-runtime/package.json').read_text(encoding='utf-8'))['dependencies']]
    seen, packages = set(), []
    while pending:
        name, start = pending.pop()
        source = resolve_package(name, start, modules)
        relative = source.relative_to(app)
        if relative.as_posix() in seen:
            continue
        seen.add(relative.as_posix())
        if len(seen) > 100:
            raise ValueError('Runtime dependency closure exceeds the packaging bound.')
        metadata = json.loads((source / 'package.json').read_text(encoding='utf-8'))
        if metadata.get('name') != name:
            raise ValueError('Installed dependency identity differs from its request.')
        copy_tree(source, output / 'app' / relative, modules, package=True)
        packages.append({'name': name, 'version': metadata['version'], 'path': 'app/' + relative.as_posix()})
        # Platform-specific optional Codex binaries are deliberately excluded;
        # CP_AGENT_CODEX_EXECUTABLE always identifies a separately pinned binary.
        pending.extend((dependency, source) for dependency in metadata.get('dependencies', {}) if dependency not in metadata.get('optionalDependencies', {}))
    files = [{'file': p.relative_to(output).as_posix(), 'sha256': sha(p), 'bytes': p.stat().st_size}
             for p in sorted(output.rglob('*')) if p.is_file()]
    if len(files) > 10000 or sum(p['bytes'] for p in files) > 256 << 20:
        raise ValueError('Release exceeds its bounded packaging inventory.')
    report = {'created_at': datetime.now(timezone.utc).isoformat(), 'release_sha256': release_digest(output),
              'package_lock_sha256': sha(app / 'package-lock.json'), 'packages': sorted(packages, key=lambda p: p['path']),
              'files': files, 'file_count': len(files), 'bytes': sum(p['bytes'] for p in files),
              'native_executables_included': False, 'secrets_included': False,
              'all_skills_remain_draft': all(json.loads(p.read_text(encoding='utf-8'))['state'] == 'draft' for p in (output / 'app/skills').glob('*/manifest.json')),
              'linux_ownership_validated': False, 'kernel_boundary_executed': False, 'real_model_invoked': False}
    manifest = output.with_name(output.name + '.manifest.json')
    with manifest.open('x', encoding='utf-8') as stream:
        json.dump(report, stream, indent=2, ensure_ascii=False)
        stream.write('\n')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, default=APP / '.local/native-agent/runtime-release')
    args = parser.parse_args()
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
    if not npm:
        raise SystemExit('Native npm is required.')
    result = subprocess.run([npm, 'run', 'build', '--workspace', '@copypaste/agent-runtime'], cwd=APP,
                            creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    if result.returncode:
        raise SystemExit('Runtime compilation failed; no release created.')
    report = create_release(args.output)
    print(json.dumps({k: report[k] for k in ('release_sha256', 'file_count', 'bytes', 'native_executables_included', 'all_skills_remain_draft')}))


if __name__ == '__main__':
    main()
