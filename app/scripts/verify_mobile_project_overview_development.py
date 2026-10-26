"""Inspect the owned development environment without restarting any service."""
import importlib.util
from pathlib import Path

APP = Path(__file__).resolve().parents[1]


def main():
    spec = importlib.util.spec_from_file_location('mobile_overview_development_base', APP / 'scripts/verify_project_overview_development.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.EVIDENCE = APP.parent / 'docs/quality/evidence/mobile-project-overview'
    return module.main()


if __name__ == '__main__':
    raise SystemExit(main())
