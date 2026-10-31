"""Source binding for the post-freeze foreign expense refund increment."""
import hashlib
from pathlib import Path
import subprocess
APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
CLIENT_SCOPES = ['web', 'mobile', 'packages', 'agent-runtime', 'package.json', 'package-lock.json']
SCOPES = [*CLIENT_SCOPES, 'backend', 'skills', 'scripts/build_openapi.py',
 'scripts/foreign_expense_refunds_sources.py', 'scripts/run_foreign_expense_refunds_checks.py',
 'scripts/test_foreign_expense_refunds_native.py', 'scripts/contract_body_native_cluster.py',
 'scripts/run_foreign_expense_refunds_browser.py', 'scripts/check_foreign_expense_refunds.mjs',
 'scripts/foreign_expense_refunds_browser_common.mjs', 'scripts/foreign_expense_refunds_recording.mjs',
 'scripts/render_foreign_expense_refunds_media.py', 'scripts/verify_foreign_expense_refunds.py']
def sources(scopes=None):
 names = subprocess.check_output(['git','ls-files','-z','--cached','--others','--exclude-standard','--',*(scopes or SCOPES)],cwd=APP).decode('utf-8').split('\0')
 return sorted({n for n in names if n and (APP/n).is_file()})
def digest(names=None):
 h=hashlib.sha256()
 for n in names or sources(): h.update(n.encode()+b'\0'+(APP/n).read_bytes()+b'\0')
 return h.hexdigest()
