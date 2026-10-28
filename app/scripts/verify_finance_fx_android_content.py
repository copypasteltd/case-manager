"""Inspect the actual bundle and APK; no Android device or emulator is started."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/finance-fx'
build = json.loads((E / 'build.json').read_text())
assert build['passed']
bundle = APP / build['artifacts']['bundle']['path']
apk = APP / build['artifacts']['apk']['path']
sha = lambda data: hashlib.sha256(data).hexdigest()
text = bundle.read_text(encoding='utf-8')

def contains(value):
    escaped = ''.join('\\u%04x' % ord(c) if ord(c) > 127 else c for c in value)
    return value in text or escaped.lower() in text.lower()

with zipfile.ZipFile(apk) as archive:
    same = archive.read('assets/index.android.bundle') == bundle.read_bytes()
checks = [
    {'name':'Current portable bundle is byte-identical inside the newly built APK', 'passed':same and sha(bundle.read_bytes()) == build['artifacts']['bundle']['sha256'] and sha(apk.read_bytes()) == build['artifacts']['apk']['sha256']},
    {'name':'Quote policy, exact book and report selection protocols are included', 'passed':all(contains(v) for v in ('/finance/fx-policy','/finance/fx-books','fx_book_id','scope_line_half_up'))},
    {'name':'Human publication, explicit selection and source review controls are present', 'passed':all(contains(v) for v in ('发布人工汇率版本','用于当前报表','人工汇率明细','汇率发布依据','确认基础币种、来源与舍入口径'))},
    {'name':'Incomplete coverage is explicit and report CSV remains pinned', 'passed':all(contains(v) for v in ('source_coverage_complete','missing_native_amount','完整合计待补齐','fx_date','/finance/report-exports'))},
]
result = {'passed':all(c['passed'] for c in checks), 'checked_at':datetime.now(timezone.utc).isoformat(),
    'checks':checks, 'source_sha256':build['source_sha256'], 'bundle_sha256':build['artifacts']['bundle']['sha256'],
    'apk_sha256':build['artifacts']['apk']['sha256'], 'artifact_executed':False, 'real_device':False,
    'verification_script_sha256':sha(Path(__file__).read_bytes())}
(E / 'android-feature-content.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'passed':result['passed'],'checks':len(checks),'device_executed':False}))
raise SystemExit(0 if result['passed'] else 1)
