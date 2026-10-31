"""Source binding for organization secretary runtime queue operations, excluding credentials."""
import hashlib, subprocess
from pathlib import Path
APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent
CLIENT_SCOPES=['web','mobile','packages','agent-runtime','package.json','package-lock.json']
SCOPES=[*CLIENT_SCOPES,'backend','skills','scripts/build_openapi.py','scripts/agent_runtime_ops_sources.py',
 'scripts/test_agent_runtime_ops_native.py','scripts/contract_body_native_cluster.py',
 'scripts/run_agent_runtime_ops_checks.py','scripts/run_agent_runtime_ops_browser.py',
 'scripts/check_agent_runtime_ops.mjs','scripts/agent_runtime_ops_browser_common.mjs',
 'scripts/agent_runtime_ops_recording.mjs','scripts/render_agent_runtime_ops_media.py',
 'scripts/verify_agent_runtime_ops.py']
def sources(scopes=None):
 values=subprocess.check_output(['git','ls-files','-z','--cached','--others','--exclude-standard','--',*(scopes or SCOPES)],cwd=APP).decode('utf-8').split('\0')
 return sorted({n for n in values if n and (APP/n).is_file()})
def digest(names=None):
 h=hashlib.sha256()
 for n in names or sources():h.update(n.encode()+b'\0'+(APP/n).read_bytes()+b'\0')
 return h.hexdigest()
