"""Exact native import interaction inputs; browser media remains scoped to 95."""
import hashlib
from pathlib import Path

APP = Path(__file__).resolve().parents[1]


def source_files():
    files = [p for p in (APP/'backend').rglob('*') if p.is_file() and (p.suffix in ('.go','.sql') or p.name in ('go.mod','go.sum') or p.relative_to(APP).as_posix() == 'backend/internal/contract/openapi.json')]
    files += [APP/'packages/contracts/src/api.d.ts', APP/'scripts/build_openapi.py',APP/'scripts/verify_file_import_cli.py',Path(__file__)]
    return sorted({p.relative_to(APP).as_posix() for p in files})


def digest():
    result=hashlib.sha256()
    for name in source_files():
        result.update(name.encode()+b'\0'+(APP/name).read_bytes()+b'\0')
    return result.hexdigest()
