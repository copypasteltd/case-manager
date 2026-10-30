"""Run all discovered current tests in unchanged batches against owned native PG.

After a recorded concurrent-build lease failure, remaining batches run with one
Go package at a time. Application leases, test cases and deadlines are unchanged."""
import sys
from pathlib import Path
import verify_backend_batches as base
from office_text_native_cluster import APP
base.command=lambda *args: [sys.executable,str(APP/'scripts/test_office_text_native.py'),'-json','-p','1',*args]
sys.argv += ['--evidence-group','office-text-backend']
raise SystemExit(base.main())
