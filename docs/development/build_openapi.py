"""Compatibility entry point; contract implementation lives under app/."""
from pathlib import Path
import runpy

if __name__ == "__main__":
    runpy.run_path(str(Path(__file__).resolve().parents[2] / "app/scripts/build_openapi.py"), run_name="__main__")
