"""Check the current mobile setup documents, evidence links and twenty-eight stable gaps."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
EVIDENCE = ROOT / 'docs/quality/evidence/mobile-project-setup'


def main():
    names = ('docs/README.md', 'docs/development/75-mobile-project-setup-and-members.md',
             'docs/requirements/03-projects-and-knowledge.md', 'docs/development/06-clients-and-pages.md',
             'docs/development/31-mobile-client-foundation.md', 'docs/development/45-mobile-project-collaboration.md',
             'docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md',
             'docs/quality/evidence/mobile-project-setup/verification.md')
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
    backend = json.loads((EVIDENCE.parent / 'deliverable-creation/result.json').read_text(encoding='utf-8'))
    final = json.loads((EVIDENCE / 'final-source-check.json').read_text(encoding='utf-8'))
    development = json.loads((EVIDENCE / 'development-project-setup.json').read_text(encoding='utf-8'))
    mobile = json.loads((EVIDENCE / 'mobile-build.json').read_text(encoding='utf-8'))
    checks = [
        {'name': 'All selected local document links resolve', 'passed': not missing},
        {'name': 'All twenty-eight gap IDs remain distinct and in order', 'passed': ids == [f'GAP-{i:02}' for i in range(1, 29)]},
        {'name': 'Index and ledger identify increment75 and its functional implementation',
         'passed': 'development/75-mobile-project-setup-and-members.md' in read('docs/README.md') and '## 49. ' in ledger
                   and '75-mobile-project-setup-and-members.md' in ledger},
        {'name': 'Ledger and gap baseline identify unchanged backend source and explicit evidence reuse',
         'passed': backend['passed'] and backend['source_sha256'] == final['backend_source_sha256']
                   and backend['source_sha256'] in ledger and backend['source_sha256'] in gap
                   and '准确复用增量74' in ledger and '本轮未重跑' in gap},
        {'name': 'Final source and fresh sixty-nine-migration development checks pass',
         'passed': final['passed'] and development['passed'] and development['migration_count'] == 69},
        {'name': 'Mobile setup and member entry are implemented while remaining mobile coverage stays open',
         'passed': re.search(r'^\| GAP-18 \|.*增量75.*99.*7条.*更多项目后续操作', gap, re.M) is not None
                   and '按各端矩阵补项目立项／成员' not in gap},
        {'name': 'Real model and Skill publication remain open with the unchanged draft package version',
         'passed': re.search(r'^\| GAP-12 \|.*部分实现.*交付', gap, re.M) is not None
                   and re.search(r'^\| GAP-13 \|.*draft.*1.4.0', gap, re.M) is not None
                   and '真实模型' in gap and not final['full_goal_complete']},
        {'name': 'Fresh mobile and Android evidence is distinct from cached tests, device and production release claims',
         'passed': mobile['passed'] and mobile['tests']['pass'] == 99 and '99' in gap and '81' in gap
                   and 'UP-TO-DATE' in gap and '真实设备' in gap and '开发' in gap},
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
