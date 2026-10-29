"""Check recovery code in the exact APK; never run a device or emulator."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/notification-retry'
build = json.loads((E / 'build.json').read_text(encoding='utf-8'))
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
    {'name': 'Portable bundle is byte-identical inside the newly built APK',
     'passed': same and sha(bundle.read_bytes()) == build['artifacts']['bundle']['sha256'] and sha(apk.read_bytes()) == build['artifacts']['apk']['sha256']},
    {'name': 'Notification read entry, detail/history and server-derived retry eligibility branch are packaged',
     'passed': all(contains(v) for v in ('notification.delivery.read', '/notification-delivery-jobs/', '/notification-delivery-retries?limit=10', '投递异常与重试', 'can_retry', 'retry_block', 'not_authorized')) and re.search(r'\.can_retry&&', text) is not None},
    {'name': 'Fixed revisions, strict receipts and explicit human confirmation are packaged',
     'passed': all(contains(v) for v in ('expected_revision', 'source_job_id', 'business_event_id', 'created_by', '人工重试原因', '确认创建重试', '已核对失败原因；按原事件重新检查当前接收人，不修改原始业务。'))},
    {'name': 'Recent MFA, terminal-source links and actual in-app results are packaged',
     'passed': all(contains(v) for v in ('已通过本次两步验证，请继续核对并确认操作。', '查看原失败', '查看已创建的后续任务', '已写入站内信', 'already_retried'))},
]
result = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
    'checks': checks, 'source_sha256': build['source_sha256'],
    'bundle_sha256': build['artifacts']['bundle']['sha256'], 'apk_sha256': build['artifacts']['apk']['sha256'],
    'artifact_executed': False, 'real_device': False, 'verification_script_sha256': sha(Path(__file__).read_bytes())}
(E / 'android-feature-content.json').write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': result['passed'], 'checks': len(checks), 'device_executed': False}))
raise SystemExit(0 if result['passed'] else 1)
