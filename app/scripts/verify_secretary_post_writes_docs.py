"""Check the knowledge-write increment's documentation, evidence and 28 stable gaps."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
EVIDENCE = ROOT / 'docs/quality/evidence/secretary-post-writes'


def main():
    names = ('docs/README.md', 'docs/development/58-secretary-knowledge-writes.md',
        'docs/requirements/07-secretary-and-channels.md', 'docs/development/07-agent-tools-and-skills.md',
        'docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md',
        'docs/quality/evidence/secretary-post-writes/verification.md', 'docs/skills/project-briefing/SKILL.md', 'docs/quality/01-test-strategy-and-cases.md',
        'docs/requirements/03-projects-and-knowledge.md')
    missing, links, sources = [], 0, []
    for name in names:
        file = ROOT / name
        content = file.read_text(encoding='utf-8')
        sources.append({'path': name, 'sha256': hashlib.sha256(file.read_bytes()).hexdigest()})
        for raw in re.findall(r'\[[^\]]+\]\(([^)]+)\)', content):
            target = raw.strip('<>')
            parsed = urlsplit(target)
            if parsed.scheme or target.startswith('#') or not parsed.path:
                continue
            links += 1
            resolved = (file.parent / unquote(parsed.path)).resolve()
            if not resolved.exists() and resolved != (EVIDENCE / 'documentation-check.json').resolve():
                missing.append({'document': name, 'target': target})
    gap = (ROOT / names[5]).read_text(encoding='utf-8')
    ledger = (ROOT / names[4]).read_text(encoding='utf-8')
    ids = re.findall(r'^\| (GAP-\d{2}) \|', gap, re.M)
    backend = json.loads((EVIDENCE / 'result.json').read_text(encoding='utf-8'))
    final = json.loads((EVIDENCE / 'final-source-check.json').read_text(encoding='utf-8'))
    development = json.loads((EVIDENCE / 'development-post-writes.json').read_text(encoding='utf-8'))
    checks = [
        {'name': 'All selected local document links resolve', 'passed': not missing},
        {'name': 'All 28 goal gap IDs remain distinct and in order', 'passed': ids == [f'GAP-{i:02}' for i in range(1,29)] and len(set(ids)) == 28},
        {'name': 'Index and ledger identify increment 58', 'passed': 'development/58-secretary-knowledge-writes.md' in (ROOT / names[0]).read_text(encoding='utf-8') and '## 32. ' in ledger and '58-secretary-knowledge-writes.md' in ledger},
        {'name': 'Current backend source appears in ledger and baseline', 'passed': backend['passed'] and backend['source_sha256'] in ledger and backend['source_sha256'] in gap},
        {'name': 'Current source and owned development checks pass', 'passed': final['passed'] and development['passed']},
        {'name': 'Knowledge writes remain partial scope and the 13-page mobile baseline is explicit', 'passed': re.search(r'^\| GAP-12 \|.*58-secretary-knowledge', gap,re.M) is not None and re.search(r'^\| GAP-18 \|.*13',gap,re.M) is not None},
    ]
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'document_count': len(names), 'local_links_checked': links, 'missing_links': missing, 'gap_ids': ids,
        'checks': checks, 'documents': sources}
    (EVIDENCE / 'documentation-check.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':report['passed'],'documents':len(names),'links':links,'gap_ids':len(ids),'missing_links':missing,'failed':[c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
