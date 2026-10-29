"""Inspect current policy code inside the actual APK; never execute a device."""
from datetime import datetime,timezone
import hashlib
import json
from pathlib import Path
import re
import zipfile

APP=Path(__file__).resolve().parents[1]
E=APP.parent/'docs/quality/evidence/contract-reminders'
build=json.loads((E/'build.json').read_text(encoding='utf-8'))
assert build['passed']
bundle=APP/build['artifacts']['bundle']['path'];apk=APP/build['artifacts']['apk']['path']
sha=lambda data:hashlib.sha256(data).hexdigest()
text=bundle.read_text(encoding='utf-8')
def contains(value):
    escaped=''.join('\\u%04x'%ord(c) if ord(c)>127 else c for c in value)
    return value in text or escaped.lower() in text.lower()
with zipfile.ZipFile(apk) as archive:
    same=archive.read('assets/index.android.bundle')==bundle.read_bytes()
checks=[
    {'name':'Current portable bundle is byte-identical inside the new development APK','passed':same and sha(bundle.read_bytes())==build['artifacts']['bundle']['sha256'] and sha(apk.read_bytes())==build['artifacts']['apk']['sha256']},
    {'name':'Independent policy read entry, actual routes and server-derived publication authority are packaged','passed':all(contains(v) for v in ('notification.policy.read','/notification-contract-policy','/notification-contract-policy-history?limit=10','合同到期提醒策略','can_publish')) and re.search(r'\.can_publish&&',text) is not None},
    {'name':'Explicit fixed-version input, timezone, thresholds and receipt identity checks are packaged','passed':all(contains(v) for v in ('expected_version','days_before','effective_on','created_by','业务时区','提前天数','策略变更原因','本次策略输入尚未固定。'))},
    {'name':'Human MFA, confirmation, actionable input feedback and publication history are packaged','passed':all(contains(v) for v in ('已通过本次两步验证，请继续核对并确认操作。','确认发布提醒策略','已核对组织范围、时区、提前量及历史提醒处理方式。','策略发布历史','刷新提醒策略','INVALID_POLICY_INPUT','提前量须为互不重复的0至365天。'))},
]
record={'passed':all(v['passed'] for v in checks),'checked_at':datetime.now(timezone.utc).isoformat(),'checks':checks,
    'source_sha256':build['source_sha256'],'bundle_sha256':build['artifacts']['bundle']['sha256'],'apk_sha256':build['artifacts']['apk']['sha256'],
    'artifact_executed':False,'real_device':False,'verification_script_sha256':sha(Path(__file__).read_bytes())}
(E/'android-feature-content.json').write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'passed':record['passed'],'checks':len(checks),'device_executed':False}))
raise SystemExit(0 if record['passed'] else 1)
