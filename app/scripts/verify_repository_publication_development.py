"""Reuse read-only native probes in a new publication evidence group; never rewrite old evidence."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/repository-showcase'


def main():
    filename = APP / 'scripts/verify_mobile_project_resources_development.py'
    spec = importlib.util.spec_from_file_location('publication_development_probe', filename)
    probe = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(probe)
    probe.EVIDENCE = EVIDENCE
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    code = probe.main()
    report = {'passed': code == 0, 'checked_at': datetime.now(timezone.utc).isoformat(),
              'read_only': True, 'services_started': False, 'source_script': filename.relative_to(APP).as_posix(),
              'source_script_sha256': hashlib.sha256(filename.read_bytes()).hexdigest(),
              'wrapper_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'scope': 'Fresh owned native development probes only; no production or external service activation.'}
    (EVIDENCE / 'development-check.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    return code


if __name__ == '__main__':
    raise SystemExit(main())
