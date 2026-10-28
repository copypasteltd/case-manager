"""Cross-compile the current backend without running or deploying the artifact."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import verify_backend_batches as backend

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/oa-prose'
E.mkdir(parents=True, exist_ok=True)
source = backend.digest()
target = APP / '.local/oa-prose-platform-linux-amd64'
log = APP / '.local/oa-prose-linux-build.log'
with log.open('w', encoding='utf-8') as output:
    status = subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'build', '-trimpath', '-o', str(target), './cmd/platform'],
        cwd=APP / 'backend', env={**os.environ, 'GOOS':'linux', 'GOARCH':'amd64', 'CGO_ENABLED':'0',
            'GODEBUG':'http2client=0'}, stdout=output, stderr=subprocess.STDOUT,
        creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0).returncode
artifact = target.read_bytes() if status == 0 else b''
result = {'passed': status == 0 and source == backend.digest() and artifact[:4] == b'\x7fELF',
    'checked_at':datetime.now(timezone.utc).isoformat(), 'source_sha256':source,
    'source_after_sha256':backend.digest(), 'exit_code':status,
    'artifact':{'file':'ignored app/.local/' + target.name, 'sha256':hashlib.sha256(artifact).hexdigest(), 'bytes':len(artifact)},
    'raw_log_sha256':hashlib.sha256(log.read_bytes()).hexdigest(),
    'verification_script_sha256':hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
    'artifact_executed':False, 'deployed':False, 'virtualization_used':False}
(E / 'linux-build.json').write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed':result['passed'],'artifact_executed':False,'deployed':False}))
raise SystemExit(0 if result['passed'] else 1)
