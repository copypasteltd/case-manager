"""Check current functional documents, evidence links and all twenty-eight stable goal gaps."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
EVIDENCE = ROOT / 'docs/quality/evidence/milestone-creation'


def main():
    names = ('docs/README.md', 'docs/development/72-secretary-milestone-creation-and-task-association.md',
             'docs/requirements/03-projects-and-knowledge.md', 'docs/requirements/07-secretary-and-channels.md',
             'docs/development/07-agent-tools-and-skills.md', 'docs/skills/project-update/SKILL.md',
             'docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md',
             'docs/quality/evidence/milestone-creation/verification.md')
    missing, sources, links = [], [], 0
    for name in names:
        path = ROOT / name
        content = path.read_text(encoding='utf-8')
        sources.append({'path': name, 'sha256': hashlib.sha256(path.read_bytes()).hexdigest()})
        for target in re.findall(r'\[[^\]]+\]\(([^)]+)\)', content):
            target = target.strip('<>'); parsed = urlsplit(target)
            if parsed.scheme or target.startswith('#') or not parsed.path:
                continue
            links += 1
            resolved = (path.parent / unquote(parsed.path)).resolve()
            if not resolved.exists() and resolved != (EVIDENCE / 'documentation-check.json').resolve():
                missing.append({'document': name, 'target': target})
    read = lambda name: (ROOT / name).read_text(encoding='utf-8')
    gap = read('docs/delivery/03-goal-gap-assessment.md'); ledger = read('docs/delivery/02-implementation-ledger.md')
    ids = re.findall(r'^\| (GAP-\d{2}) \|', gap, re.M)
    backend = json.loads((EVIDENCE / 'result.json').read_text(encoding='utf-8'))
    final = json.loads((EVIDENCE / 'final-source-check.json').read_text(encoding='utf-8'))
    development = json.loads((EVIDENCE / 'development-milestone-creation.json').read_text(encoding='utf-8'))
    checks = [
        {'name': 'All selected local document links resolve', 'passed': not missing},
        {'name': 'All twenty-eight gap IDs remain distinct and in order', 'passed': ids == [f'GAP-{i:02}' for i in range(1, 29)]},
        {'name': 'Index and ledger identify increment72 and its functional implementation',
         'passed': 'development/72-secretary-milestone-creation-and-task-association.md' in read('docs/README.md') and '## 46. ' in ledger
                   and '72-secretary-milestone-creation-and-task-association.md' in ledger},
        {'name': 'Ledger and current gap baseline bind the actual newly executed backend source',
         'passed': backend['passed'] and backend['source_sha256'] in ledger and backend['source_sha256'] in gap},
        {'name': 'Current final source and sixty-six-migration development checks pass',
         'passed': final['passed'] and development['passed'] and development['migration_count'] == 66},
        {'name': 'Remaining real model and Skill publication work stays open with the correct draft package version',
         'passed': re.search(r'^\| GAP-12 \|.*部分实现.*里程碑', gap, re.M) is not None
                   and re.search(r'^\| GAP-13 \|.*draft.*1.2.0', gap, re.M) is not None
                   and '真实模型' in gap and not final['full_goal_complete']},
        {'name': 'Fresh Web, mobile and Android evidence remains distinct from device or production release claims',
         'passed': '77' in gap and '89' in gap and 'Android' in gap and '真实设备' in gap and '开发' in gap},
    ]
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
              'document_count': len(names), 'local_links_checked': links, 'gap_ids': ids, 'missing_links': missing,
              'documents': sources, 'checks': checks}
    (EVIDENCE / 'documentation-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'documents': len(names), 'links': links, 'gap_ids': len(ids),
                      'missing_links': missing, 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
