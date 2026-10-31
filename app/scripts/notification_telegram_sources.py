"""Source binding for personal Telegram business notifications, excluding credentials."""
import hashlib, subprocess
from pathlib import Path
APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent
CLIENT_SCOPES=['web','mobile','packages','agent-runtime','skills','../docs/skills','package.json','package-lock.json']
SCOPES=[*CLIENT_SCOPES,'backend','skills','scripts/build_openapi.py','scripts/notification_telegram_sources.py','scripts/test_notification_telegram_native.py','scripts/contract_body_native_cluster.py','scripts/run_notification_telegram_checks.py','scripts/run_notification_telegram_browser.py','scripts/check_notification_telegram.mjs','scripts/notification_telegram_browser_common.mjs','scripts/notification_telegram_recording.mjs','scripts/render_notification_telegram_media.py','scripts/verify_notification_telegram.py']

def sources(scopes=None):
 values=subprocess.check_output(['git','ls-files','-z','--cached','--others','--exclude-standard','--',*(scopes or SCOPES)],cwd=APP).decode('utf-8').split('\0')
 return sorted({n for n in values if n and (APP/n).is_file()})
def digest(names=None):
 h=hashlib.sha256()
 for n in names or sources():h.update(n.encode()+b'\0'+(APP/n).read_bytes()+b'\0')
 return h.hexdigest()
