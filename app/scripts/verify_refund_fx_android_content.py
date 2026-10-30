"""Inspect Actual foreign refund principal and exact provenance in the APK; no device execution."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/refund-fx'
build = json.loads((E / 'build.json').read_text(encoding='utf-8'))
assert build['passed'] and build['source_unchanged_during_build']
bundle, apk = [APP / build['artifacts'][key]['path'] for key in ('bundle', 'apk')]
sha = lambda data: hashlib.sha256(data).hexdigest()
text = bundle.read_text(encoding='utf-8')


def contains(value):
    escaped = ''.join('\\u%04x' % ord(c) if ord(c) > 127 else c for c in value)
    return value in text or escaped.lower() in text.lower()


with zipfile.ZipFile(apk) as archive:
    same = archive.read('assets/index.android.bundle') == bundle.read_bytes()
checks = [
 {'name':'Actual foreign cash and immutable principal UI are packaged', 'passed':all(contains(v) for v in ('异币退款实收依据','退回原付款本金','退款汇率凭证来源'))},
 {'name':'Exact monetary bridge and explicit human confirmation protocol are packaged','passed':all(contains(v) for v in ('refund_fx_acknowledged','original_currency','original_amount','原付款本金乘实际汇率'))},
 {'name':'Actual inspected APK retains the same business bundle and original principal scope limit','passed':same and all(contains(v) for v in ('当前仅使用原付款未分配本金','不回转费用或合同归属','原本金'))},
]

record = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
    'checks': checks, 'source_sha256': build['source_sha256'],
    'bundle_sha256': sha(bundle.read_bytes()), 'apk_sha256': sha(apk.read_bytes()),
    'artifact_executed': False, 'real_device': False, 'verification_script_sha256': sha(Path(__file__).read_bytes())}
assert record['bundle_sha256'] == build['artifacts']['bundle']['sha256'] and record['apk_sha256'] == build['artifacts']['apk']['sha256']
(E / 'android-feature-content.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': record['passed'], 'checks': len(checks), 'device_executed': False}))
raise SystemExit(0 if record['passed'] else 1)
