"""Check the current notification delivery documents and preserved goal IDs."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
EVIDENCE = ROOT / 'docs/quality/evidence/notifications'


def main():
    files = [ROOT / name for name in ('docs/README.md', 'docs/development/53-in-app-notifications.md', 'docs/requirements/08-status-notifications-and-nfr.md', 'docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md', 'docs/quality/evidence/notifications/verification.md')]
    missing, links, sources = [], 0, []
    for file in files:
        content = file.read_text(encoding='utf-8')
        sources.append({'path': file.relative_to(ROOT).as_posix(), 'sha256': hashlib.sha256(file.read_bytes()).hexdigest()})
        for raw in re.findall(r'\[[^\]]+\]\(([^)]+)\)', content):
            target = raw.strip('<>')
            parsed = urlsplit(target)
            if parsed.scheme or target.startswith('#'):
                continue
            path = unquote(parsed.path)
            if not path:
                continue
            links += 1
            resolved = (file.parent / path).resolve()
            if not resolved.exists() and resolved != (EVIDENCE / 'documentation-check.json').resolve():
                missing.append({'document': file.relative_to(ROOT).as_posix(), 'target': target})
    gap = files[4].read_text(encoding='utf-8')
    ids = re.findall(r'^\| (GAP-\d{2}) \|', gap, re.M)
    expected = [f'GAP-{number:02}' for number in range(1, 29)]
    backend = json.loads((EVIDENCE / 'backend-result.json').read_text(encoding='utf-8'))
    final = json.loads((EVIDENCE / 'final-source-check.json').read_text(encoding='utf-8'))
    development = json.loads((EVIDENCE / 'development-notifications.json').read_text(encoding='utf-8'))
    ledger = files[3].read_text(encoding='utf-8')
    checks = [
        {'name': 'All selected local document links resolve', 'passed': not missing},
        {'name': 'All 28 distinct goal gap IDs remain in order', 'passed': ids == expected and len(set(ids)) == 28},
        {'name': 'Index and implementation ledger link increment 53', 'passed': 'development/53-in-app-notifications.md' in files[0].read_text(encoding='utf-8') and '## 27. ' in ledger and '53-in-app-notifications.md' in ledger},
        {'name': 'Current backend source is recorded in ledger and gap baseline', 'passed': backend['passed'] and backend['source_sha256'] in ledger and backend['source_sha256'] in gap},
        {'name': 'Current source and owned development checks pass', 'passed': final['passed'] and development['passed']},
        {'name': 'Notification gap remains explicit and mobile page count is current', 'passed': re.search(r'^\| GAP-08 \|.*53-in-app-notifications.*@', gap, re.M) is not None and re.search(r'^\| GAP-18 \|.*13', gap, re.M) is not None},
    ]
    report = {'passed': all(check['passed'] for check in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'document_count': len(files), 'local_links_checked': links, 'missing_links': missing, 'gap_ids': ids, 'checks': checks, 'documents': sources}
    (EVIDENCE / 'documentation-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'documents': len(files), 'links': links, 'gap_ids': len(ids), 'missing_links': missing, 'failed': [check['name'] for check in checks if not check['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
