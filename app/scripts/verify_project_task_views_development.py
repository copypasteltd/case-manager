"""Read-only current development inspection; preserve increment67 evidence."""
import importlib.util
from pathlib import Path

APP = Path(__file__).resolve().parents[1]


def main():
    source = APP / 'scripts/verify_project_templates_development.py'
    spec = importlib.util.spec_from_file_location('task_view_development', source)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.EVIDENCE = APP.parent / 'docs/quality/evidence/project-task-views'
    module.EVIDENCE.mkdir(parents=True, exist_ok=True)
    return module.main()


if __name__ == '__main__':
    raise SystemExit(main())
