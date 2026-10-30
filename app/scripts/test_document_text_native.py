"""Execute actual native parser checks with explicit pinned local dependencies."""
import os
from pathlib import Path
import subprocess
APP=Path(__file__).resolve().parents[1]
profile=Path(os.environ.get('CP_DOCUMENT_TEXT_TEST_PROFILE',str(APP/'.local/document-tools/profile.json'))).resolve(strict=True)
fixtures=Path(os.environ.get('CP_DOCUMENT_TEXT_TEST_FIXTURES',str(APP.parent/'docs/quality/fixtures/document-text'))).resolve(strict=True)
env={**os.environ,'CP_DOCUMENT_TEXT_TEST_PROFILE':str(profile),'CP_DOCUMENT_TEXT_TEST_FIXTURES':str(fixtures)}
raise SystemExit(subprocess.call([str(APP/'.tools/go/bin/go.exe'),'test','./internal/documenttext','-count=1','-json'],cwd=APP/'backend',env=env))
