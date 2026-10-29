"""Inspect the actual JS bundle and APK without running a device or emulator."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/cost-amortization'
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
    {'name': 'Explicit service methods, source revision, plan version and preview digest are present',
     'passed': all(contains(v) for v in ('monthly_equal', 'daily_weighted', 'expected_plan_version', 'preview_sha256', 'service_from', '/amortization'))},
    {'name': 'Human preview, publication, stopping, fresh MFA and immutable history controls are present',
     'passed': all(contains(v) for v in ('预览月度摊销', '确认发布摊销计划', '确认停用当前计划', '不可修改的计划版本', '确认本人操作', '原样重试计划操作'))},
    {'name': 'Independent report basis, original expense source and original-month quotation evidence are present',
     'passed': all(contains(v) for v in ('latest_active_schedules', 'cost_amortization', 'original_date', '费用摊销', '/finance/report-exports'))},
]
result = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
          'checks': checks, 'source_sha256': build['source_sha256'],
          'bundle_sha256': build['artifacts']['bundle']['sha256'], 'apk_sha256': build['artifacts']['apk']['sha256'],
          'artifact_executed': False, 'real_device': False, 'verification_script_sha256': sha(Path(__file__).read_bytes())}
(E / 'android-feature-content.json').write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': result['passed'], 'checks': len(checks), 'device_executed': False}))
raise SystemExit(0 if result['passed'] else 1)
