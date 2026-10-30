"""Publish the current Office feature guide without rewriting historical bytes."""
from pathlib import Path
import json
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/office-text'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def save(path, data):
    raw = path.read_bytes() if path.exists() else b''
    newline = '\r\n' if b'\r\n' in raw else '\n'
    path.write_bytes((json.dumps(data, ensure_ascii=False, indent=2) + '\n').replace('\n', newline).encode())


def prepend(name, body):
    path = ROOT / name
    raw = path.read_bytes()
    assert b'Increment120' not in raw[:1200] and '增量120'.encode() not in raw[:1200], name
    first, rest = raw.split(b'\n', 1)
    newline = b'\r\n' if first.endswith(b'\r') else b'\n'
    path.write_bytes(first + b'\n' + newline + body.strip().encode().replace(b'\n', newline) + newline + newline + rest.lstrip(b'\r\n'))


def main():
    backend = read(E.parent / 'office-text-backend/result.json')
    clients = read(E / 'client-checks.json')
    assert backend['passed'] and clients['passed']
    counts = {c['name']: c['test_count'] for c in clients['checks']}
    jvm = read(E / 'android-test-scope.json')
    overview = '''增量120已将 XLSX／PPTX 接入四端的固定版本正文预览、当前版本检索、全文比较和秘书资料预览。Excel保留单元格坐标、隐藏标识、公式文字和可能过期的文件缓存；PPT包含幻灯片文字和讲者备注。公式不计算，排版／图像查原件。原件8MiB、派生正文2MiB有界，不改120GB普通文件上传设计上限。

Increment120 adds exact-version XLSX/PPTX text previews, current-head search, complete text comparisons and selected secretary context across all four clients. XLSX retains cell coordinates, hidden-content labels, inert formulas and potentially stale cached values; PPTX includes slide text and speaker notes. Original files remain intact. Text extraction is bounded to8MiB originals/2MiB output; the ordinary120GB upload design is separate.

使用：正常上传并扫描通过 → 等待正文提取 → 搜索工作资料／查阅此版本正文；文件详情明确选择两个版本后比较；秘书资料明确选择准确版本后预览加入。微信、抖音和Android共用Taro页面；不同提取方式分别固定原件／正文摘要，历史提取终态保留。

Usage: upload and pass scanning, wait for indexing, then search and open the exact-version text. Choose two formal versions in file details to compare; select an exact version in a secretary conversation to preview and bind it. WeChat, Douyin and Android share Taro pages. Every derivative retains its original and text digests; prior terminal evidence remains immutable.

本轮重新核对完整后端、客户端和实际Web／H5页面，执行91→92正常结构升级并保留原记录。Android仅开发包／内容检查，JVM缓存状态如实记录；受控对象与资料读取不代替真实云或Codex／Skill验收。旧316份已发布媒体及原证据保留。23项待完成（14 P0／9 P1）、5项移出和八项draft Skill不变；仅AI费用控制移出，秘书、TG、授权、审计、隔离和技术限制保留。

Fresh checks cover current backend/client sources and real Web/Taro H5 pages, plus a populated91→92 normal schema upgrade. Android proof is a development artifact inspection, with JVM cache status disclosed. Controlled objects and source retrieval do not establish live cloud/model/Skill acceptance. All316 prior media retain their published bytes. Increment119 release archives retain their historical source/schema91 binding; deployment of this source requires a new schema92 package. All23 packages (14 P0/9 P1), five exclusions and eight draft Skills remain open. Only AI fee controls are excluded.'''
    jvm_kind = 'Gradle缓存复用 / cached results' if jvm['gradle_cached'] else 'JVM本轮执行 / freshly executed'
    overview += (f"\n\n本轮验证 / Checks: Go {backend['discovered_top_level_tests']}个顶层用例、{len(backend['checks'])}条结果；"
        f"Web {counts['web-tests']}；共享移动端 / shared mobile {counts['mobile-tests']}；"
        f"运行器 / runtime {counts['runtime-tests']}；JVM {jvm['tests']}（{jvm_kind}）。"
        'Android使用披露的单worker Metro调用，保留完整原生构建／包内检查。 / Android uses a disclosed single-worker Metro invocation with native build and artifact checks intact.')
    for name in ('README.md', 'docs/README.md', 'docs/README.zh-CN.md', 'docs/README.en.md'):
        prefix = 'docs/' if name == 'README.md' else ''
        links = '\n\n[设计与使用 / Design and usage](' + prefix + 'development/120-office-text-extraction.md) · [本轮验证 / Verification](' + prefix + 'quality/120-office-text-verification.md) · [全部未完成项 / Remaining](' + prefix + 'delivery/03-goal-gap-assessment.md)'
        preview = '\n\n![Excel 正文 / XLSX preview](' + prefix + 'media/screenshots/office-text-web-xlsx.png)\n\n![实际页面演示 / Actual UI demo](' + prefix + 'media/gifs/office-text-web.gif)\n\n[Web MP4](' + prefix + 'media/videos/office-text-web.mp4) · [移动端演示 / Mobile demo](' + prefix + 'media/videos/office-text-mobile.mp4)'
        prepend(name, overview + links + preview)
    short = '增量120：GAP-04／18增加XLSX／PPTX准确版本提取、当前正文检索、全文比较和秘书资料读取，保留原件与旧提取证据。PDF／OCR、复杂DOCX、Office编辑／版式及完整业务验收继续开放。仍23 active（14 P0／9 P1）、5 excluded。见[设计](../development/120-office-text-extraction.md)及[验证](../quality/120-office-text-verification.md)。'
    for name in ('docs/delivery/04-current-scope-and-acceptance.md', 'docs/delivery/03-goal-gap-assessment.md', 'docs/delivery/02-implementation-ledger.md'):
        prepend(name, short)
    for name in ('docs/requirements/06-files-and-search.md', 'docs/development/92-file-text-search.md'):
        prepend(name, '增量120当前补充：XLSX／PPTX已接通有界准确版本正文预览／检索／比较与秘书资料读取；Excel公式与缓存、隐藏内容、PPT备注及明确原件边界见[Office正文设计](../development/120-office-text-extraction.md)。PDF/OCR、Office编辑和复杂DOCX仍开放；范围按[修订99](../delivery/04-current-scope-and-acceptance.md)。')
    scope_path = ROOT / 'docs/delivery/04-current-scope.json'
    scope = read(scope_path)
    for row in scope['items']:
        if row['id'] in ('GAP-04', 'GAP-18'):
            row.setdefault('completed_adaptations', []).append('OFFICE-TEXT-120')
            row['completed_detail'] = row.get('completed_detail', '') + ' 增量120接通XLSX／PPTX准确正式版本正文提取、当前版本搜索、逐侧提取依据比较及秘书资料读取；四端显示格式限制，旧终态不改写。PDF／OCR、复杂DOCX、Office编辑与版式及完整旅程仍开放。'
            if row['id'] == 'GAP-04':
                row['remaining'] = '继续补PDF／OCR、复杂DOCX、Office创建／编辑与原件排版；逐格式核对真实业务样本。增量120已接通XLSX／PPTX文字提取、当前版本检索、比较和秘书读取，公式不计算、缓存可能过期，完整格式／业务验收仍开放。'
    save(scope_path, scope)
    # Replace just the two current table rows; preserve the historical register.
    gap = ROOT / 'docs/delivery/03-goal-gap-assessment.md'
    raw = gap.read_bytes()
    lines = raw.decode().splitlines(keepends=True)
    for index, line in enumerate(lines):
        for row in scope['items']:
            if row['id'] == 'GAP-04' and line.startswith('| GAP-04 |'):
                # The remaining-work column follows ID/name/priority/kind.
                old_scope = json.loads(subprocess.check_output(['git', 'show', 'HEAD:docs/delivery/04-current-scope.json'], cwd=ROOT))
                old = next(r['remaining'] for r in old_scope['items'] if r['id'] == row['id'])
                assert old in line
                lines[index] = line.replace(old, row['remaining'])
    gap.write_bytes(''.join(lines).encode())
    prepend('docs/CHANGELOG.md', '## Increment120 · XLSX／PPTX正文\n\n- 新增有界OOXML关系解析、单元格／公式缓存／隐藏标识、幻灯片文字和讲者备注；禁止宏／外部读取，失败不发布部分正文。\n- 迁移0092增加独立office-v1，保留旧结果；同步真实队列、搜索、秘书资料和逐侧正文比较，以及四端提示。\n- 新增完整源码回归、正常升级、实际页面／截图／录像／GIF和Android开发包检查；初轮与中断记录保留，缓存及实际云／模型／生产边界分别披露。')
    prepend('docs/media/README.md', '''增量120实际Web／Taro H5合成演示：XLSX隐藏工作表与公式缓存、PPTX讲者备注、准确版本比较。录制在登录后开始，不包含密码／MFA或真实业务记录。

| 页面 | Web | Taro H5 |
| --- | --- | --- |
| Excel正文 | [截图](screenshots/office-text-web-xlsx.png) | [截图](screenshots/office-text-mobile-xlsx.png) |
| PowerPoint正文 | [截图](screenshots/office-text-web-pptx.png) | [截图](screenshots/office-text-mobile-pptx.png) |
| 版本比较 | [截图](screenshots/office-text-web-comparison.png)、[深色](screenshots/office-text-web-dark.png) | [截图](screenshots/office-text-mobile-comparison.png) |
| 录屏 | [MP4](videos/office-text-web.mp4) · [GIF](gifs/office-text-web.gif) | [MP4](videos/office-text-mobile.mp4) · [GIF](gifs/office-text-mobile.gif) |

新素材对应最终源码；初轮素材单独归档，旧316份已发布媒体字节保留。[验证](../quality/120-office-text-verification.md)。''')
    agents = ROOT / 'AGENTS.md'
    agents.write_bytes(agents.read_bytes() + b'\r\n- Increment120 adds bounded XLSX/PPTX extraction with immutable office-v1 derivations, exact current-head search, per-version comparisons and secretary source reads across four clients, schema92. Preserve formula/cache/hidden-content/layout limitations and all legacy text-v1 evidence. PDF/OCR, complex DOCX, Office editing/layout and full GAP-04/18 remain open. See `docs/development/120-office-text-extraction.md`.\r\n')
    baseline_path = ROOT / 'docs/quality/current-functional-baseline.json'
    baseline = read(baseline_path)
    assert baseline['increment'] == 118
    baseline.update(increment=120, evidence_group='office-text', mobile_tests=counts['mobile-tests'],
        verification_script='verify_office_text_sources.py', backend_evidence_group='office-text-backend',
        backend_top_level_tests=backend['discovered_top_level_tests'], backend_result_records=len(backend['checks']),
        web_evidence_group='office-text', runtime_evidence_group='office-text',
        web_tests=counts['web-tests'], runtime_tests=counts['runtime-tests'],
        development_verification_script='verify_office_text_development.py', interaction_kind='office_text_extraction',
        interaction_sources='office_text_sources.py', retained_evidence_record='docs/quality/evidence/office-text/reused-evidence.json',
        dependency_audit_record='docs/quality/evidence/office-text/dependency-audit.json',
        planning_scope_verification_record='docs/quality/evidence/office-text/scope-check.json',
        planning_publication_record='docs/quality/evidence/office-text/repository-check.json')
    changed = subprocess.check_output(['git', 'diff', '--name-only', '-z'], cwd=ROOT).decode().split('\0')
    new = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    names = set(baseline['publication_files'] + [n for n in changed + new if n and (ROOT / n).is_file()])
    names.update('docs/quality/evidence/office-text/' + n for n in
        ('final-source-check.json', 'scope-check.json', 'acceptance-applicability-check.json'))
    baseline['publication_files'] = sorted(n for n in names if Path(n).suffix.lower() not in {'.png', '.gif', '.mp4', '.webm', '.log'} and n != baseline['planning_publication_record'])
    save(baseline_path, baseline)
    print(json.dumps({'passed': True, 'increment': 120, 'active': 23, 'excluded': 5}))


if __name__ == '__main__':
    main()
