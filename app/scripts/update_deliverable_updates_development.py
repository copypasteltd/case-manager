"""Load the reviewed backend into the exact owned native development API."""
import importlib.util
from pathlib import Path

APP = Path(__file__).resolve().parents[1]


def main():
    spec = importlib.util.spec_from_file_location('deliverable_update_runtime', APP / 'scripts/update_project_templates_development.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.EVIDENCE = APP.parent / 'docs/quality/evidence/deliverable-updates'
    return module.main()


if __name__ == '__main__':
    raise SystemExit(main())
