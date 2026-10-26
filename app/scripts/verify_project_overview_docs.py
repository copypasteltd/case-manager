"""Check project-overview documentation links, stable gaps and acceptance traceability."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
from urllib.parse import unquote, urlsplit

ROOT=Path(__file__).resolve().parents[2]
FILES=[ROOT/name for name in (
 'docs/README.md','docs/delivery/02-implementation-ledger.md','docs/delivery/03-goal-gap-assessment.md',
 'docs/requirements/03-projects-and-knowledge.md','docs/development/69-project-business-overview.md',
 'docs/quality/evidence/project-overview/verification.md',
)]


def main():
    errors=[];links=0;digest=hashlib.sha256()
    output=ROOT/'docs/quality/evidence/project-overview/document-check.json'
    for file in FILES:
        text=file.read_text(encoding='utf-8');digest.update(file.relative_to(ROOT).as_posix().encode()+b'\0'+file.read_bytes()+b'\0')
        for raw in re.findall(r'\[[^\]]*\]\(([^)]+)\)',text):
            target=urlsplit(raw.strip('<>'))
            if target.scheme or not target.path:continue
            links+=1;path=(file.parent/unquote(target.path)).resolve()
            # This invocation creates its own report after checking the inputs.
            if not path.is_relative_to(ROOT) or (path!=output and not path.exists()):errors.append(file.relative_to(ROOT).as_posix()+': '+raw)
    gap=FILES[2].read_text(encoding='utf-8');ids=re.findall(r'^\| (GAP-\d+) \|',gap,re.MULTILINE)
    if ids!=[f'GAP-{number:02d}' for number in range(1,29)]:errors.append('Original 28 gap IDs must remain stable.')
    row=next(line for line in gap.splitlines() if line.startswith('| GAP-01 |'))
    if not all(word in row for word in ('增量67','增量68','增量69','聚合','跨OA')):errors.append('GAP-01 must record actual project aggregation and financial progress and preserve remaining project requirements.')
    row=next(line for line in gap.splitlines() if line.startswith('| GAP-17 |'))
    if not all(word in row for word in ('实体设备','真实120GB','不替代实测')):errors.append('Original Android real-device/120GB gates must stay open.')
    for file,prefix,count in ((FILES[4],'POV-',8),(FILES[5],'POV-AC-',8)):
        actual=re.findall(r'^\| ('+re.escape(prefix)+r'\d+) \|',file.read_text(encoding='utf-8'),re.MULTILINE)
        if actual!=[f'{prefix}{n:02d}' for n in range(1,count+1)]:errors.append('Missing traceability '+prefix)
    ledger=FILES[1].read_text(encoding='utf-8')
    if '## 43.' not in ledger or '69-project-business-overview.md' not in ledger:errors.append('Ledger must include this functional increment.')
    if '执行中' in FILES[5].read_text(encoding='utf-8') or '待回归结束' in FILES[5].read_text(encoding='utf-8'):errors.append('Final verification document still contains pending execution status.')
    report={'passed':not errors,'checked_at':datetime.now(timezone.utc).isoformat(),'documents':len(FILES),'local_links_checked':links,'stable_gap_ids':len(ids),'source_sha256':digest.hexdigest(),'sources':[file.relative_to(ROOT).as_posix() for file in FILES],'errors':errors}
    output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':not errors,'documents':len(FILES),'links':links,'errors':errors},ensure_ascii=False))
    return 0 if not errors else 1


if __name__=='__main__':raise SystemExit(main())
