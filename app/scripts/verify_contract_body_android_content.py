"""Inspect exact full-body review and separate human adoption in the APK; no device execution."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/contract-body'
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
    {'name': 'Explicit same-project contract basis and exact original revision are packaged',
     'passed': all(contains(v) for v in ('选择合同正文依据', '将准确合同依据加入请求', 'base_revision_id', 'file_intent_sha256'))},
    {'name': 'Complete file text, independent byte/hash verification and clean first-version guards are packaged',
     'passed': all(contains(v) for v in ('准确文件的完整正文', 'content_sha256', 'sha256', 'size_bytes', 'contract-body-full-text', 'scan_state'))},
    {'name': 'Separate explicit reviewed human adoption and immutable receipt are in the exact APK bundle',
     'passed': same and all(contains(v) for v in ('/contract-body-adoption', '我已核对合同原始资料与完整正文，确认采用此准确文件版本', '采用完整合同正文', 'draft_file_version_id', 'operation_id'))},
]
record = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
    'checks': checks, 'source_sha256': build['source_sha256'],
    'bundle_sha256': sha(bundle.read_bytes()), 'apk_sha256': sha(apk.read_bytes()),
    'artifact_executed': False, 'real_device': False, 'verification_script_sha256': sha(Path(__file__).read_bytes())}
assert record['bundle_sha256'] == build['artifacts']['bundle']['sha256'] and record['apk_sha256'] == build['artifacts']['apk']['sha256']
(E / 'android-feature-content.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': record['passed'], 'checks': len(checks), 'device_executed': False}))
raise SystemExit(0 if record['passed'] else 1)
