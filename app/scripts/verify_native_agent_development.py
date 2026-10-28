"""Source-bound read-only development inspection, using the unchanged schema82 checks."""
import hashlib
import importlib.util
import json
from pathlib import Path

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/native-agent-runtime'


def main():
    helper = APP / 'scripts/verify_multipart_cleanup_development.py'
    spec = importlib.util.spec_from_file_location('native_agent_owned_inspection', helper)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.E = E
    status = module.main()
    path = E / 'development-project-resources.json'
    value = json.loads(path.read_text(encoding='utf-8'))
    value.update(wrapper_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                 delegated_implementation_sha256=hashlib.sha256(helper.read_bytes()).hexdigest(),
                 native_controller_started=False, real_model_invoked=False)
    path.write_text(json.dumps(value, indent=2) + '\n', encoding='utf-8')
    return status


if __name__ == '__main__':
    raise SystemExit(main())
