"""Source binding for the post-freeze receivable foreign return increment."""
import hashlib
from pathlib import Path
import subprocess
APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
CLIENT_SCOPES = ['web', 'mobile', 'packages', 'agent-runtime', 'package.json', 'package-lock.json']
SCOPES = [*CLIENT_SCOPES, 'backend', 'skills', 'scripts/build_openapi.py',
 'scripts/receivable_foreign_returns_sources.py', 'scripts/run_receivable_foreign_returns_checks.py',
 'scripts/test_receivable_foreign_returns_native.py', 'scripts/contract_body_native_cluster.py',
 'scripts/run_receivable_foreign_returns_browser.py', 'scripts/check_receivable_foreign_returns.mjs',
 'scripts/receivable_foreign_returns_browser_common.mjs', 'scripts/receivable_foreign_returns_recording.mjs',
 'scripts/render_receivable_foreign_returns_media.py', 'scripts/verify_receivable_foreign_returns.py']
def sources(scopes=None):
 names = subprocess.check_output(['git','ls-files','-z','--cached','--others','--exclude-standard','--',*(scopes or SCOPES)],cwd=APP).decode('utf-8').split('\0')
 return sorted({n for n in names if n and (APP/n).is_file()})
def digest(names=None):
 h=hashlib.sha256()
 for n in names or sources(): h.update(n.encode()+b'\0'+(APP/n).read_bytes()+b'\0')
 return h.hexdigest()
