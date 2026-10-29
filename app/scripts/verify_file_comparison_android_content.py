"""Inspect recovery UI and provenance in the built APK without executing a device."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/file-comparison'
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
    {'name': 'Exact version selectors, explicit comparison and both page controls are packaged', 'passed': all(contains(v) for v in ('比较两个内容版本', '比较基础版本', '比较目标版本', '比较所选版本', '上一页正文差异', '下一页正文差异'))},
    {'name': 'Version, full-text and pinned-page guards are packaged', 'passed': all(contains(v) for v in ('text-comparison', 'base_version_id', 'target_version_id', 'comparison_sha256', 'base_offset', 'target_offset', 'text_sha256', 'file_version', 'bounded-replacement-v1'))},
    {'name': 'Literal text, context counts and byte-versus-text disclosure are packaged', 'passed': all(contains(v) for v in ('提取正文相同，但原件字节不同；BOM、换行、排版或其他原件信息可能不同。', '此行后无末尾换行', '省略', 'cp-diff-text', '差异显示范围', '全部正文'))},
]
record = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
    'source_sha256': build['source_sha256'], 'bundle_sha256': build['artifacts']['bundle']['sha256'], 'apk_sha256': build['artifacts']['apk']['sha256'],
    'artifact_executed': False, 'real_device': False, 'verification_script_sha256': sha(Path(__file__).read_bytes())}
(E / 'android-feature-content.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': record['passed'], 'checks': len(checks), 'device_executed': False}))
raise SystemExit(0 if record['passed'] else 1)
