"""Check the Android increment's documentation links and stable gap IDs."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
DOCS = [ROOT / name for name in (
    'docs/README.md', 'docs/delivery/02-implementation-ledger.md',
    'docs/delivery/03-goal-gap-assessment.md', 'docs/development/65-android-native-foundation.md',
    'docs/quality/evidence/android-native/verification.md',
)]


def main():
    errors, count, value = [], 0, hashlib.sha256()
    for file in DOCS:
        text = file.read_text(encoding='utf-8')
        value.update(file.relative_to(ROOT).as_posix().encode() + b'\0' + file.read_bytes() + b'\0')
        for raw in re.findall(r'\[[^\]]*\]\(([^)]+)\)', text):
            url = urlsplit(raw.strip('<>'))
            if url.scheme or not url.path: continue
            count += 1
            target = (file.parent / unquote(url.path)).resolve()
            if not target.is_relative_to(ROOT) or not target.exists(): errors.append(file.relative_to(ROOT).as_posix() + ': ' + raw)
    gap = DOCS[2].read_text(encoding='utf-8')
    ids = re.findall(r'^\| (GAP-\d+) \|', gap, re.MULTILINE)
    if ids != [f'GAP-{number:02d}' for number in range(1, 29)]: errors.append('The twenty-eight stable gap IDs changed.')
    implementation = DOCS[3].read_text(encoding='utf-8')
    if not all(word in implementation for word in ['GAP-17', '尚未实现', '实体设备', '正式签名', '不启动 Docker']): errors.append('Android release and capacity limits must remain explicit.')
    ledger = DOCS[1].read_text(encoding='utf-8')
    if '## 39.' not in ledger or '65-android-native-foundation.md' not in ledger: errors.append('The implementation ledger must include this functional increment.')
    report = {'passed': not errors, 'checked_at': datetime.now(timezone.utc).isoformat(), 'documents': len(DOCS),
              'local_links_checked': count, 'stable_gap_ids': len(ids), 'source_sha256': value.hexdigest(), 'errors': errors}
    target = ROOT / 'docs/quality/evidence/android-native/document-check.json'
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'documents': len(DOCS), 'links': count, 'errors': errors}, ensure_ascii=False))
    return 0 if report['passed'] else 1


if __name__ == '__main__': raise SystemExit(main())
