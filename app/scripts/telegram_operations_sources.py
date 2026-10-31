"""Source binding for organization TG delivery operations, excluding credentials."""
import hashlib, subprocess
from pathlib import Path
APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent
CLIENT_SCOPES=['web','mobile','packages','agent-runtime','package.json','package-lock.json']
SCOPES=[*CLIENT_SCOPES,'backend','skills','scripts/build_openapi.py','scripts/telegram_operations_sources.py',
 'scripts/test_telegram_operations_native.py','scripts/contract_body_native_cluster.py',
 'scripts/run_telegram_operations_checks.py','scripts/run_telegram_operations_browser.py',
 'scripts/check_telegram_operations.mjs','scripts/telegram_operations_browser_common.mjs',
 'scripts/telegram_operations_recording.mjs','scripts/render_telegram_operations_media.py',
 'scripts/verify_telegram_operations.py']
def sources(scopes=None):
 values=subprocess.check_output(['git','ls-files','-z','--cached','--others','--exclude-standard','--',*(scopes or SCOPES)],cwd=APP).decode('utf-8').split('\0')
 return sorted({n for n in values if n and (APP/n).is_file()})
def digest(names=None):
 h=hashlib.sha256()
 for n in names or sources():h.update(n.encode()+b'\0'+(APP/n).read_bytes()+b'\0')
 return h.hexdigest()
