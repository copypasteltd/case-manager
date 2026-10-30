"""Run only the native Go parsers; no DB, Office application or virtualization."""
import os
from pathlib import Path
import subprocess
import sys

APP=Path(__file__).resolve().parents[1]
args=sys.argv[1:]
assert args in ([],['-json'])
raise SystemExit(subprocess.call([str(APP/'.tools/go/bin/go.exe'),'test','./internal/files',
    '-count=1','-run','^(TestDOCXTextExtraction|TestOfficeTextExtraction|TestFileTextExtraction)$',*args],
    cwd=APP/'backend',env={**os.environ,'GOPROXY':'https://goproxy.cn,https://proxy.golang.org,direct','GODEBUG':'http2client=0'}))
