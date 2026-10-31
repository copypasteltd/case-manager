"""Source binding for complete conversation subscription portfolios, excluding credentials."""
import hashlib, subprocess
from pathlib import Path
APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent
CLIENT_SCOPES=['web','mobile','packages','agent-runtime','package.json','package-lock.json']
SCOPES=[*CLIENT_SCOPES,'backend','skills','scripts/build_openapi.py','scripts/subscription_portfolio_sources.py','scripts/test_subscription_portfolio_native.py','scripts/contract_body_native_cluster.py','scripts/run_subscription_portfolio_checks.py','scripts/run_subscription_portfolio_browser.py','scripts/check_subscription_portfolio.mjs','scripts/subscription_portfolio_browser_common.mjs','scripts/subscription_portfolio_recording.mjs','scripts/render_subscription_portfolio_media.py','scripts/verify_subscription_portfolio.py']

def sources(scopes=None):
 values=subprocess.check_output(['git','ls-files','-z','--cached','--others','--exclude-standard','--',*(scopes or SCOPES)],cwd=APP).decode('utf-8').split('\0')
 return sorted({n for n in values if n and (APP/n).is_file()})
def digest(names=None):
 h=hashlib.sha256()
 for n in names or sources():h.update(n.encode()+b'\0'+(APP/n).read_bytes()+b'\0')
 return h.hexdigest()
