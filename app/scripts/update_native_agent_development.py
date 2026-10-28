"""Refresh only attested native development services; do not enable model execution."""
import hashlib
import importlib.util
import json
from pathlib import Path

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/native-agent-runtime'


def main():
    helper = APP / 'scripts/update_multipart_cleanup_development.py'
    spec = importlib.util.spec_from_file_location('native_agent_owned_update', helper)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    E.mkdir(parents=True, exist_ok=True)
    module.E = E
    status = module.main()
    path = E / 'development-update.json'
    value = json.loads(path.read_text(encoding='utf-8'))
    value.update(wrapper_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                 delegated_implementation_sha256=hashlib.sha256(helper.read_bytes()).hexdigest(),
                 native_controller_started=False, real_model_invoked=False)
    path.write_text(json.dumps(value, indent=2) + '\n', encoding='utf-8')
    return status


if __name__ == '__main__':
    raise SystemExit(main())
