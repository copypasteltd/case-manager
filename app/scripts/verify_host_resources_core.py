"""Run native monitoring unit checks and cross-build the actual collector."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import verify_backend_batches as sources

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/host-resources'


def main():
    before = sources.digest()
    folder = APP / '.local/host-resources'
    folder.mkdir(parents=True, exist_ok=True)
    go = str(APP / '.tools/go/bin/go.exe')
    command = [go, 'test', './internal/servicestatus', './internal/statusbridge', '-count=1', '-json']
    log = folder / 'core-tests.jsonl'
    with log.open('wb') as output:
        result = subprocess.run(command, cwd=APP / 'backend', stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW)
    if result.returncode:
        return result.returncode
    records = [json.loads(line) for line in log.read_text(encoding='utf-8').splitlines() if line.startswith('{')]
    passed = [row for row in records if row.get('Action') == 'pass' and row.get('Test')]
    assert passed and not [row for row in records if row.get('Action') in ('fail', 'skip')]
    artifacts = []
    for target, suffix in [('windows', '.exe'), ('linux', '')]:
        file = folder / ('copypaste-status-bridge-' + target + suffix)
        command = [go, 'build', '-trimpath', '-o', str(file), './cmd/status-bridge']
        with (folder / (target + '-build.log')).open('wb') as output:
            result = subprocess.run(command, cwd=APP / 'backend', env={**os.environ, 'GOOS': target, 'GOARCH': 'amd64', 'CGO_ENABLED': '0'}, stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW)
        if result.returncode:
            return result.returncode
        artifacts.append({'target': target + '/amd64', 'file': file.relative_to(APP).as_posix(), 'bytes': file.stat().st_size, 'sha256': hashlib.sha256(file.read_bytes()).hexdigest(), 'executed_on_target': False})
    report = {'passed': before == sources.digest(), 'checked_at': datetime.now(timezone.utc).isoformat(), 'backend_source_sha256': before, 'passed_test_records': len(passed), 'skipped': 0, 'test_log_sha256': hashlib.sha256(log.read_bytes()).hexdigest(), 'artifacts': artifacts, 'real_prometheus_executed': False, 'target_host_executed': False, 'virtualization_used': False, 'scope': 'Actual native Go unit suites and Windows/Linux collector compilation. Controlled Prometheus HTTP responses are exercised in TestHostResources; no target Linux/node_exporter or production installation claim.'}
    (E / 'host-core.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'test_records': len(passed), 'native_builds': len(artifacts)}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
