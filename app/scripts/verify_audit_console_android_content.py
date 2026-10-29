"""Inspect the actual JS bundle and APK without running a device or emulator."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/audit-console'
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
    {'name': 'Current portable bundle is byte-identical inside the newly built APK',
     'passed': same and sha(bundle.read_bytes()) == build['artifacts']['bundle']['sha256'] and sha(apk.read_bytes()) == build['artifacts']['apk']['sha256']},
    {'name': 'Organization audit metadata and separate query/export permissions are included',
     'passed': all(contains(v) for v in ('organization_metadata_v1', 'audit.read', 'audit.export', '/audit/events', '/audit/exports'))},
    {'name': 'Explicit filters, human purpose, sealed history and current authorization are included',
     'passed': all(contains(v) for v in ('筛选尚未应用', '导出用途', '按当前条件生成 CSV', 'authorization_changed', 'csv_sha256', 'chunk_sha256'))},
    {'name': 'The settings entry and verified native CSV save path are included',
     'passed': all(contains(v) for v in ('section=audit', '查询审计记录与固定导出', '校验并打开 CSV', 'audit-'))},
]
result = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
          'checks': checks, 'source_sha256': build['source_sha256'],
          'bundle_sha256': build['artifacts']['bundle']['sha256'], 'apk_sha256': build['artifacts']['apk']['sha256'],
          'artifact_executed': False, 'real_device': False, 'verification_script_sha256': sha(Path(__file__).read_bytes())}
(E / 'android-feature-content.json').write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': result['passed'], 'checks': len(checks), 'device_executed': False}))
raise SystemExit(0 if result['passed'] else 1)
