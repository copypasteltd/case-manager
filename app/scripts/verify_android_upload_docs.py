"""Check current upload documentation links and preserve the original release scope."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
FILES = [ROOT / name for name in (
    'docs/README.md', 'docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md',
    'docs/development/65-android-native-foundation.md', 'docs/development/66-android-resumable-uploads.md',
    'docs/quality/evidence/android-native/verification.md', 'docs/quality/evidence/android-upload/verification.md',
)]


def main():
    errors, links, value = [], 0, hashlib.sha256()
    for file in FILES:
        text = file.read_text(encoding='utf-8')
        value.update(file.relative_to(ROOT).as_posix().encode() + b'\0' + file.read_bytes() + b'\0')
        for raw in re.findall(r'\[[^\]]*\]\(([^)]+)\)', text):
            target = urlsplit(raw.strip('<>'))
            if target.scheme or not target.path: continue
            links += 1; path = (file.parent / unquote(target.path)).resolve()
            if not path.is_relative_to(ROOT) or not path.exists(): errors.append(file.relative_to(ROOT).as_posix() + ': ' + raw)
    gap = FILES[2].read_text(encoding='utf-8')
    ids = re.findall(r'^\| (GAP-\d+) \|', gap, re.MULTILINE)
    if ids != [f'GAP-{number:02d}' for number in range(1, 29)]: errors.append('Original gap IDs must remain stable.')
    row = next((line for line in gap.splitlines() if line.startswith('| GAP-17 |')), '')
    if not all(word in row for word in ('核心代码已实现', '实体设备', '真实120GB', '不替代实测')): errors.append('GAP-17 must retain device and real-capacity acceptance.')
    implementation = FILES[4].read_text(encoding='utf-8')
    for prefix, count in [('ANU-', 7), ('ANU-AC-', 5)]:
        expected = [f'{prefix}{number:02d}' for number in range(1, count + 1)]
        actual = re.findall(r'^\| (' + re.escape(prefix) + r'\d+) \|', implementation, re.MULTILINE)
        if actual != expected: errors.append('Missing implementation or acceptance traceability: ' + prefix)
    ledger = FILES[1].read_text(encoding='utf-8')
    if '## 40.' not in ledger or '66-android-resumable-uploads.md' not in ledger: errors.append('Implementation ledger is missing this functional increment.')
    report = {'passed': not errors, 'checked_at': datetime.now(timezone.utc).isoformat(), 'documents': len(FILES), 'local_links_checked': links,
              'stable_gap_ids': len(ids), 'source_sha256': value.hexdigest(), 'errors': errors}
    output = ROOT / 'docs/quality/evidence/android-upload/document-check.json'
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'documents': len(FILES), 'links': links, 'errors': errors}, ensure_ascii=False))
    return 0 if report['passed'] else 1


if __name__ == '__main__': raise SystemExit(main())
