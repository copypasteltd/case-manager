"""Generate/review a complete private native bundle; never installs or starts services."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import sys

from config import PlanError, load, require
from render import build


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--config', type=Path, required=True)
    parser.add_argument('--output', type=Path)
    parser.add_argument('--check-only', action='store_true')
    args = parser.parse_args()
    try:
        require(args.check_only != (args.output is not None), 'OUTPUT_MODE')
        plan = load(args.config)
        files = build(plan)
        encoded = {name: text.encode('utf-8') for name, text in files.items()}
        manifest = {'schema': 1, 'component_lock_sha256': hashlib.sha256(Path(__file__).with_name('components.lock.json').read_bytes()).hexdigest(),
                    'files': {name: hashlib.sha256(data).hexdigest() for name, data in sorted(encoded.items())},
                    'operator_declarations_only': True, 'services_started': False, 'real_delivery_verified': False}
        if args.output:
            parent = args.output.resolve().parent
            require(parent.is_dir() and not args.output.exists() and not args.output.is_symlink(), 'NEW_OUTPUT_REQUIRED')
            # Refuse symlink ancestors so an operator cannot accidentally write elsewhere.
            require(not any(p.is_symlink() for p in [args.output.parent, *args.output.parent.parents]), 'OUTPUT_SYMLINK')
            args.output.mkdir(mode=0o700)
            for name, data in encoded.items():
                target = args.output / name
                target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
                with target.open('xb') as handle:
                    os.chmod(target, 0o600)
                    handle.write(data)
            with (args.output / 'manifest.json').open('x', encoding='utf-8', newline='\n') as handle:
                os.chmod(handle.name, 0o600)
                json.dump(manifest, handle, ensure_ascii=False, indent=2)
                handle.write('\n')
        print(json.dumps({'passed': True, 'files': len(files), 'check_only': args.check_only, 'services_started': False}))
        return 0
    except (PlanError, OSError) as error:
        # Never echo plan values, paths, keys or a configuration exception body.
        print(json.dumps({'passed': False, 'code': str(error) if isinstance(error, PlanError) else 'FILESYSTEM'}))
        return 2


if __name__ == '__main__':
    raise SystemExit(main())
