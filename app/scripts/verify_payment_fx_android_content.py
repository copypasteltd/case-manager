"""Inspect reviewed double-currency UI in the built APK without executing a device."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/payment-fx'
build = json.loads((E / 'build.json').read_text(encoding='utf-8'))
assert build['passed']
bundle, apk = [APP / build['artifacts'][key]['path'] for key in ('bundle', 'apk')]
sha = lambda data: hashlib.sha256(data).hexdigest()
text = bundle.read_text(encoding='utf-8')

def contains(value):
    escaped = ''.join('\\u%04x' % ord(c) if ord(c) > 127 else c for c in value)
    return value in text or escaped.lower() in text.lower()

with zipfile.ZipFile(apk) as archive:
    same = archive.read('assets/index.android.bundle') == bundle.read_bytes()
checks = [
    {'name':'Exact dual units and quotation guards are packaged without the observed broken BigInt power lowering','passed':all(contains(v) for v in ('remaining_cost_amount','cost_currency','payment_currency','cost_shares','cost_amount','quoted_on')) and not re.search(r'\bMath\.pow\(\s*(?:[0-9]+n\b|BigInt\()', text)},
    {'name':'Manual settlement quotation and precise currency direction UI are packaged','passed':all(contains(v) for v in ('费用原币核销金额','实际汇率（每1','结算汇率凭证来源','本次付款币本金'))},
    {'name':'Release and refund review MFA and exact APK bundle remain packaged','passed':same and all(contains(v) for v in ('本次回转：','原分配当前双币余额','已核对原事实、凭证、双币金额、汇率方向与所选依据','验证器六位验证码'))},
]

record = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
    'source_sha256': build['source_sha256'], 'bundle_sha256': build['artifacts']['bundle']['sha256'], 'apk_sha256': build['artifacts']['apk']['sha256'],
    'artifact_executed': False, 'real_device': False, 'verification_script_sha256': sha(Path(__file__).read_bytes())}
(E / 'android-feature-content.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': record['passed'], 'checks': len(checks), 'device_executed': False}))
raise SystemExit(0 if record['passed'] else 1)
