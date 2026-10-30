"""Inspect Office text extraction and exact provenance in the APK; no device execution."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/office-text'
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
 {'name':'Known Office extraction identity and limitations are packaged', 'passed':all(contains(v) for v in ('office-v1','Office 正文为文字提取','公式不计算','包含隐藏内容'))},
 {'name':'Both exact-version comparison provenance and eligible Office formats are packaged','passed':all(contains(v) for v in ('mixed','spreadsheetml','presentationml','比较两个内容版本','comparison_sha256'))},
 {'name':'Exact search preview and secretary version selection use the same APK bundle','passed':same and all(contains(v) for v in ('查阅此版本正文','讲者备注','text_sha256','XLSX','PPTX'))},
]
record = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
    'checks': checks, 'source_sha256': build['source_sha256'],
    'bundle_sha256': sha(bundle.read_bytes()), 'apk_sha256': sha(apk.read_bytes()),
    'artifact_executed': False, 'real_device': False, 'verification_script_sha256': sha(Path(__file__).read_bytes())}
assert record['bundle_sha256'] == build['artifacts']['bundle']['sha256'] and record['apk_sha256'] == build['artifacts']['apk']['sha256']
(E / 'android-feature-content.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': record['passed'], 'checks': len(checks), 'device_executed': False}))
raise SystemExit(0 if record['passed'] else 1)
