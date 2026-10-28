"""Inspect the actual current bundle/APK; this never runs an Android device."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/oa-prose'
build = json.loads((E / 'build.json').read_text())
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
    {'name':'Exact current bundle is inside the new APK', 'passed':same and sha(bundle.read_bytes()) == build['artifacts']['bundle']['sha256'] and sha(apk.read_bytes()) == build['artifacts']['apk']['sha256']},
    {'name':'Draft tool and human-only adoption endpoint are present', 'passed':all(contains(v) for v in ('draft_business_text','oa-text-adoption'))},
    {'name':'Actual shared review, confirmation and adoption controls are present', 'passed':all(contains(v) for v in ('业务正文草稿','正文建议','采用正文草稿','采用正文的依据'))},
    {'name':'Selected field and source/digest protocol is included', 'passed':all(contains(v) for v in ('renewal_terms','termination_terms','purpose','draft_sha256','source_refs'))},
]
result = {'passed':all(c['passed'] for c in checks), 'checked_at':datetime.now(timezone.utc).isoformat(),
    'checks':checks, 'source_sha256':build['source_sha256'], 'bundle_sha256':build['artifacts']['bundle']['sha256'],
    'apk_sha256':build['artifacts']['apk']['sha256'], 'artifact_executed':False, 'real_device':False,
    'verification_script_sha256':sha(Path(__file__).read_bytes())}
(E / 'android-feature-content.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'passed':result['passed'],'checks':len(checks),'device_executed':False}))
raise SystemExit(0 if result['passed'] else 1)
