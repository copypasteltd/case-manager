"""Inspect reviewed-transfer UI in the built APK without executing a device."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/scope-transfer'
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
    {'name':'Human fixed scope-transfer routes and complete exact plan guards are packaged','passed':all(contains(v) for v in ('/file-scope-transfers','plan_hash','current_basis_matches','visibility_changes','expected_target_version'))},
    {'name':'Explicit full inventory and actual target selection UI are packaged','passed':all(contains(v) for v in ('跨项目与分类整理','分析跨范围移动','本人的固定计划','打开移动后的资料','移动计划确认原因'))},
    {'name':'Acknowledgement, MFA, blockers and exact APK bundle remain packaged','passed':same and all(contains(v) for v in ('确认并执行跨范围移动','我已核对全部清单、读取权限变化及本次操作影响','BUSINESS_REFERENCE','CLASSIFICATION_DOWNGRADE','验证器六位验证码'))},
]

record = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
    'source_sha256': build['source_sha256'], 'bundle_sha256': build['artifacts']['bundle']['sha256'], 'apk_sha256': build['artifacts']['apk']['sha256'],
    'artifact_executed': False, 'real_device': False, 'verification_script_sha256': sha(Path(__file__).read_bytes())}
(E / 'android-feature-content.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': record['passed'], 'checks': len(checks), 'device_executed': False}))
raise SystemExit(0 if record['passed'] else 1)
