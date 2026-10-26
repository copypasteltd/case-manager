"""Run the existing OA UI/API flows while preserving historical evidence."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess
import sys

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
ORIGINAL = ROOT / 'docs/quality/evidence/mobile-oa'
ARCHIVE = ROOT / 'docs/quality/evidence/mobile-status/oa-regression'


def main():
    if (APP / '.local/business-browser-fixture.json').exists():
        raise SystemExit('An owned fixture is live; wait for its owner to finish.')
    if ARCHIVE.exists():
        raise SystemExit('An archive already exists; preserve it before running again.')
    original = {path.name: path.read_bytes() for path in ORIGINAL.iterdir() if path.is_file()}
    nested = {path: hashlib.sha256(path.read_bytes()).hexdigest() for path in ORIGINAL.rglob('*') if path.is_file() and path.parent != ORIGINAL}
    status = 1
    try:
        status = subprocess.run([sys.executable, str(APP / 'scripts/run_mobile_browser.py'), '--oa'], cwd=ROOT).returncode
        ARCHIVE.mkdir(parents=True)
        for path in ORIGINAL.iterdir():
            if path.is_file() and (path.name == 'result.json' or original.get(path.name) != path.read_bytes()):
                (ARCHIVE / path.name).write_bytes(path.read_bytes())
    finally:
        for name, data in original.items():
            (ORIGINAL / name).write_bytes(data)
        for path in ORIGINAL.iterdir():
            if path.is_file() and path.name not in original:
                # Only an explicitly owned new file in this exact directory.
                path.resolve().relative_to(ORIGINAL.resolve())
                path.unlink()
    restored = all((ORIGINAL / name).read_bytes() == data for name, data in original.items())
    restored = restored and all(path.is_file() and hashlib.sha256(path.read_bytes()).hexdigest() == digest for path, digest in nested.items())
    report = {'executed_at': datetime.now(timezone.utc).isoformat(), 'runner_exit_code': status,
              'historical_files_preserved_byte_for_byte': restored, 'historical_files': len(original) + len(nested),
              'scope': 'Actual existing mobile OA UI/API regression after shared draft and candidate controls changed; native-device acceptance remains separate',
              'passed': status == 0 and restored}
    (ARCHIVE.parent / 'oa-regression-archive.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(report, ensure_ascii=False))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
