"""Inspect the actual bundle and APK; no Android device or emulator is started."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/multipart-cleanup'
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
    {'name':'Portable bundle bytes match the new APK asset and build hashes','passed':same and sha(bundle.read_bytes()) == build['artifacts']['bundle']['sha256'] and sha(apk.read_bytes()) == build['artifacts']['apk']['sha256']},
    {'name':'Candidate, plan, decision and receipt protocols are bundled','passed':all(contains(v) for v in ('/upload-cleanup-candidates','/upload-cleanup-plans','/upload-cleanup-items/','snapshot_sha256','expected_version'))},
    {'name':'Human observation, approval, cancellation and retry controls are included','passed':all(contains(v) for v in ('盘点与清理','建立固定清单并进入观察期','核对并批准清理','撤销尚未执行项','明确重试此项'))},
    {'name':'Started, confirmed absence and retry receipts have distinct presentations','passed':all(contains(v) for v in ('逐次执行回执','已确认分片不存在','retry_scheduled','confirmed_absent','generation'))},
]
result = {'passed':all(c['passed'] for c in checks), 'checked_at':datetime.now(timezone.utc).isoformat(),
    'checks':checks, 'source_sha256':build['source_sha256'], 'bundle_sha256':build['artifacts']['bundle']['sha256'],
    'apk_sha256':build['artifacts']['apk']['sha256'], 'artifact_executed':False, 'real_device':False,
    'verification_script_sha256':sha(Path(__file__).read_bytes())}
(E / 'android-feature-content.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'passed':result['passed'],'checks':len(checks),'device_executed':False}))
raise SystemExit(0 if result['passed'] else 1)
