"""Source binding for fixed deliverable secretary submission, excluding credentials."""
import hashlib, subprocess
from pathlib import Path
APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent
CLIENT_SCOPES=['web','mobile','packages','agent-runtime','skills','../docs/skills','package.json','package-lock.json']
SCOPES=[*CLIENT_SCOPES,'backend','skills','scripts/build_openapi.py','scripts/deliverable_submission_sources.py','scripts/test_deliverable_submission_native.py','scripts/contract_body_native_cluster.py','scripts/run_deliverable_submission_checks.py','scripts/run_deliverable_submission_browser.py','scripts/check_deliverable_submission.mjs','scripts/deliverable_submission_browser_common.mjs','scripts/deliverable_submission_recording.mjs','scripts/render_deliverable_submission_media.py','scripts/verify_deliverable_submission.py']

def sources(scopes=None):
 values=subprocess.check_output(['git','ls-files','-z','--cached','--others','--exclude-standard','--',*(scopes or SCOPES)],cwd=APP).decode('utf-8').split('\0')
 return sorted({n for n in values if n and (APP/n).is_file()})
def digest(names=None):
 h=hashlib.sha256()
 for n in names or sources():h.update(n.encode()+b'\0'+(APP/n).read_bytes()+b'\0')
 return h.hexdigest()
