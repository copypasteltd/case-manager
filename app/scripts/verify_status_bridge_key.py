"""Exercise real key creation and exclusive overwrite refusal without publishing keys."""
import base64
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/status-bridge'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    build = json.loads((EVIDENCE / 'bridge-build.json').read_text(encoding='utf-8'))
    phase = next(p for p in build['phases'] if p['target'] == 'windows/amd64')
    binary = APP / phase['artifact']
    assert phase['passed'] and sha(binary) == phase['sha256']
    with tempfile.TemporaryDirectory(prefix='status-bridge-key-check-', dir=APP / '.local') as private:
        key = Path(private) / 'probe.seed'

        def invoke():
            return subprocess.run([str(binary), '--generate-key', str(key)], capture_output=True,
                encoding='utf-8', timeout=10, creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)

        first = invoke()
        assert first.returncode == 0 and not first.stderr
        assert len(base64.b64decode(first.stdout.strip(), validate=True)) == 32
        assert len(base64.b64decode(key.read_text().strip(), validate=True)) == 32
        saved = key.read_bytes()
        second = invoke()
        assert second.returncode == 2 and key.read_bytes() == saved
    assert not Path(private).exists()
    report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'binary_sha256': sha(binary),
        'verification_script_sha256': sha(Path(__file__)), 'seed_bytes': 32, 'first_exit_code': 0,
        'existing_file_exit_code': 2, 'existing_key_unchanged': True, 'private_material_published': False,
        'temporary_private_directory_removed': True}
    (EVIDENCE / 'key-generation.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'new_seed_and_overwrite_refusal_verified': True, 'private_directory_removed': True}))


if __name__ == '__main__':
    main()
