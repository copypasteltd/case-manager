"""Read-only overview route/manifest check; preserve earlier evidence groups."""
from datetime import datetime, timezone
import importlib.util
import json
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/project-overview'


def main():
    spec = importlib.util.spec_from_file_location('overview_development_base', APP / 'scripts/verify_project_templates_development.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.EVIDENCE = EVIDENCE
    if module.main():
        return 1
    route = '/v1/projects/00000000-0000-4000-8000-000000000000/overview'
    try:
        with urlopen('http://127.0.0.1:18080' + route, timeout=5) as response:
            status = response.status
    except HTTPError as error:
        status = error.code
        error.close()
    report = {'passed': status == 401, 'checked_at': datetime.now(timezone.utc).isoformat(),
              'route': route, 'anonymous_status': status, 'expected_status': 401,
              'read_only_probe': True, 'real_business_data_seeded': False,
              'real_cloud_or_model': False, 'production_deployment': False}
    (EVIDENCE / 'development-overview.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'overview_route_registered_and_protected': report['passed']}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
