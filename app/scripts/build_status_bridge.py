"""Build native monitoring artifacts at one complete Go/SQL/contract digest."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
from verify_backend_batches import digest

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/status-bridge'
ARTIFACTS = APP / '.local/status-bridge/bin'


def main():
    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    before, started = digest(), datetime.now(timezone.utc).isoformat()
    phases = []
    for target, name in [('windows', 'status-bridge.exe'), ('linux', 'status-bridge-linux-amd64')]:
        path = ARTIFACTS / name
        log = APP / '.local' / ('status-bridge-build-' + target + '.log')
        with log.open('wb') as output:
            r = subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'build', '-trimpath', '-o', str(path), './cmd/status-bridge'],
                cwd=APP / 'backend', env={**os.environ, 'GOOS': target, 'GOARCH': 'amd64', 'CGO_ENABLED': '0', 'GOFLAGS': '-p=1', 'GOMAXPROCS': '2'},
                stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
        phases.append({'target': target + '/amd64', 'passed': r.returncode == 0 and path.is_file(), 'exit_code': r.returncode,
            'artifact': path.relative_to(APP).as_posix(), 'sha256': hashlib.sha256(path.read_bytes()).hexdigest() if path.is_file() else None,
            'bytes': path.stat().st_size if path.is_file() else 0, 'executed_by_build': False})
        print(json.dumps({'target': phases[-1]['target'], 'passed': phases[-1]['passed']}), flush=True)
        if r.returncode:
            break
    report = {'passed': len(phases) == 2 and all(p['passed'] for p in phases) and digest() == before,
        'started_at': started, 'completed_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': before,
        'source_unchanged_during_build': digest() == before, 'phases': phases, 'cgo_enabled': False,
        'build_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'deployed': False}
    (EVIDENCE / 'bridge-build.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'artifacts': len(phases)}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
