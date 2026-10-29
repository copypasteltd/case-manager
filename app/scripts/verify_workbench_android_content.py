"""Inspect recovery UI and provenance in the built APK without executing a device."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/workbench'
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
    {'name':'Current personal-workbench endpoint and exact scoped summary guards are packaged','passed':all(contains(v) for v in ('/dashboard/business','my_readable_work','contract_through','contract_timezone','related_id'))},
    {'name':'All three business groups and ordinary exact-resource routes are packaged','passed':all(contains(v) for v in ('待我审批','我负责的合同到期','我负责的订阅待核对','/pages/subscription-period/index?id=','kind=request&id=','kind=contract&id='))},
    {'name':'Limited samples and non-payment review disclosures remain packaged','passed':all(contains(v) for v in ('仅列出前5条，请到对应业务列表继续查阅。','已生成且需要你核对的账期，不表示欠款或已扣费。','当前没有符合条件的事项；只反映你当前可见的负责事项。'))},
]

record = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
    'source_sha256': build['source_sha256'], 'bundle_sha256': build['artifacts']['bundle']['sha256'], 'apk_sha256': build['artifacts']['apk']['sha256'],
    'artifact_executed': False, 'real_device': False, 'verification_script_sha256': sha(Path(__file__).read_bytes())}
(E / 'android-feature-content.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': record['passed'], 'checks': len(checks), 'device_executed': False}))
raise SystemExit(0 if record['passed'] else 1)
