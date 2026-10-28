"""Exact inputs of the native execution boundary, independent of retained UI media."""
import hashlib
from pathlib import Path

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent


def sources():
    paths = [p for p in (APP / 'backend').rglob('*') if p.is_file() and (p.suffix in {'.go', '.sql'} or p.name in {'go.mod', 'go.sum'} or p.relative_to(APP / 'backend').as_posix() == 'internal/contract/openapi.json')]
    paths += list((APP / 'agent-runtime/src').rglob('*.ts'))
    paths += list((APP / 'skills').rglob('*.json'))
    paths += [p for p in (ROOT / 'docs/skills').rglob('*') if p.is_file()]
    paths += [APP / p for p in ('package.json', 'package-lock.json', 'agent-runtime/package.json', 'agent-runtime/tsconfig.json',
             'scripts/build_native_runtime_release.py', 'scripts/test_native_runtime_release.py', 'scripts/verify_native_agent_package.py', 'scripts/verify_native_agent_linux_build.py', 'scripts/native_agent_sources.py')]
    paths += [APP / 'deploy/native' / p for p in ('agent-native.example.json', 'agent-worker.env.example', 'agent-native-launch.sh', 'agent-native-probe.sh', 'copypaste-agent-native.service', 'copypaste-agent-worker.service')]
    return sorted({p.relative_to(ROOT).as_posix() for p in paths})


def digest():
    value = hashlib.sha256()
    for name in sources():
        value.update(name.encode() + b'\0' + (ROOT / name).read_bytes() + b'\0')
    return value.hexdigest()
