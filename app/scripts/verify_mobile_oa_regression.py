"""Run existing mobile/project UI regression and preserve its historical record.

Uses the owned native runner only. This command archives the current run under
mobile-oa/projects-regression, then restores the original historical evidence.
"""
from datetime import datetime, timezone
import json
from pathlib import Path
import subprocess
import sys

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
ORIGINAL = ROOT / 'docs/quality/evidence/mobile-projects'
ARCHIVE = ROOT / 'docs/quality/evidence/mobile-oa/projects-regression'


def main():
    if (APP / '.local/business-browser-fixture.json').exists():
        raise SystemExit('An owned fixture is active; wait for its owner to finish.')
    if ARCHIVE.exists():
        raise SystemExit('A regression archive already exists; preserve it before rerunning.')
    original = {path.relative_to(ORIGINAL): path.read_bytes() for path in ORIGINAL.rglob('*') if path.is_file()}
    status = 1
    try:
        status = subprocess.run([sys.executable, str(APP / 'scripts/run_mobile_browser.py'), '--projects'], cwd=ROOT).returncode
        ARCHIVE.mkdir(parents=True)
        for path in ORIGINAL.rglob('*'):
            if path.is_file():
                relative, data = path.relative_to(ORIGINAL), path.read_bytes()
                if relative.name != 'result.json' and original.get(relative) == data:
                    continue
                destination = ARCHIVE / relative
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_bytes(data)
    finally:
        for relative, data in original.items():
            (ORIGINAL / relative).write_bytes(data)
    restored = all((ORIGINAL / relative).read_bytes() == data for relative, data in original.items())
    report = {'executed_at': datetime.now(timezone.utc).isoformat(), 'runner_exit_code': status,
              'historical_files_preserved_byte_for_byte': restored, 'historical_files': len(original),
              'scope': 'Existing Taro H5 mobile and project API/UI regression; no native-device or production claim',
              'passed': status == 0 and restored}
    (ARCHIVE.parent / 'projects-regression-archive.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(report))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
