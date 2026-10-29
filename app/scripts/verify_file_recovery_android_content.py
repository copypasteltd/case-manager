"""Inspect recovery UI and provenance in the built APK without executing a device."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/file-recovery'
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
    {'name': 'Current portable bundle matches the new development APK exactly', 'passed': same and sha(bundle.read_bytes()) == build['artifacts']['bundle']['sha256'] and sha(apk.read_bytes()) == build['artifacts']['apk']['sha256']},
    {'name': 'Actual original request, exact draft selection and human review controls are packaged', 'passed': all(contains(v) for v in ('选择旧草稿继续处理', '选择原文件请求', '本次恢复的准确草稿', '我已核对选定草稿的差异、内容摘要及原目标', '将准确草稿恢复加入请求'))},
    {'name': 'New request and immutable provenance identity checks are packaged', 'passed': all(contains(v) for v in ('file_recovery', 'source_run_id', 'draft_id', 'content_sha256', 'recovery_sha256', '移除本次草稿恢复', '原任务回执与选择不符。'))},
    {'name': 'Fresh execution disclosure and exact recovery receipt guards are packaged', 'passed': all(contains(v) for v in ('恢复来源', '层数', '选定内容摘要', 'reviewed', '原任务与准确草稿', '发送后重新核对原目标、基础版本、依赖资料和权限，并重新上传、校验和扫描。'))},
]
record = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
    'source_sha256': build['source_sha256'], 'bundle_sha256': build['artifacts']['bundle']['sha256'], 'apk_sha256': build['artifacts']['apk']['sha256'],
    'artifact_executed': False, 'real_device': False, 'verification_script_sha256': sha(Path(__file__).read_bytes())}
(E / 'android-feature-content.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': record['passed'], 'checks': len(checks), 'device_executed': False}))
raise SystemExit(0 if record['passed'] else 1)
