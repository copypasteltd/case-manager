"""Cross-compile current native backend targets; never execute Linux artifacts."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time

from verify_backend_batches import digest

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/refund-fx'


def main():
    source = digest()
    target = APP / '.local/refund_fx-linux'
    target.mkdir(parents=True, exist_ok=True)
    artifacts = []
    commands = []
    go = APP / '.tools/go/bin' / ('go.exe' if os.name == 'nt' else 'go')
    for name, package in [('agent-native', './cmd/agent-native'), ('platform', './cmd/platform')]:
        output = target / (name + '-linux-amd64')
        command = [str(go), 'build', '-o', str(output), package]
        began = time.monotonic()
        log = target / (name + '-linux-build.log')
        with log.open('wb') as stream:
            result = subprocess.run(command, cwd=APP / 'backend', env={**os.environ, 'GOOS': 'linux', 'GOARCH': 'amd64', 'CGO_ENABLED': '0'}, stdout=stream, stderr=subprocess.STDOUT)
        commands.append({'name': name, 'package': package, 'exit_code': result.returncode, 'elapsed_seconds': round(time.monotonic() - began, 3), 'raw_log_sha256': hashlib.sha256(log.read_bytes()).hexdigest()})
        if result.returncode:
            break
        raw = output.read_bytes()
        assert raw[:4] == b'\x7fELF' and raw[4] == 2 and raw[18:20] == b'\x3e\x00'
        artifacts.append({'name': name, 'file': output.relative_to(APP).as_posix(), 'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw), 'format': 'ELF64-x86-64'})
    report = {'passed': len(artifacts) == 2 and source == digest(), 'checked_at': datetime.now(timezone.utc).isoformat(),
              'source_sha256': source, 'source_after_sha256': digest(), 'commands': commands, 'artifacts': artifacts,
              'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'artifact_executed': False, 'kernel_boundary_executed': False, 'deployed': False}
    E.mkdir(parents=True, exist_ok=True)
    (E / 'linux-build.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'artifacts': len(artifacts), 'linux_execution': False}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
