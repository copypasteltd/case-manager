"""Exact source binding for completed, uncommitted human-upload retirement."""
import hashlib,subprocess
from pathlib import Path
APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent
CLIENT_SCOPES=['web','mobile','packages','agent-runtime','package.json','package-lock.json']
SCOPES=[*CLIENT_SCOPES,'backend','skills','scripts/build_openapi.py',
 'scripts/completed_upload_retirement_sources.py','scripts/test_completed_upload_retirement_native.py',
 'scripts/contract_body_native_cluster.py','scripts/run_completed_upload_retirement_checks.py',
 'scripts/inspect_completed_upload_client_tests.py','scripts/run_completed_upload_retirement_browser.py',
 'scripts/check_completed_upload_retirement.mjs','scripts/completed_upload_retirement_browser_common.mjs',
 'scripts/completed_upload_retirement_recording.mjs','scripts/render_completed_upload_retirement_media.py',
 'scripts/verify_completed_upload_retirement.py']
def sources(scopes=None):
 values=subprocess.check_output(['git','ls-files','-z','--cached','--others','--exclude-standard','--',*(scopes or SCOPES)],cwd=APP).decode('utf-8').split('\0')
 return sorted({n for n in values if n and (APP/n).is_file()})
def digest(names=None):
 h=hashlib.sha256()
 for n in names or sources():h.update(n.encode()+b'\0'+(APP/n).read_bytes()+b'\0')
 return h.hexdigest()
