"""Publish increment113 guides only after the actual source-bound checks finish."""
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/file-comparison'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def prepend(name, body):
    path = ROOT / name
    text = path.read_text(encoding='utf-8')
    assert '增量113' not in text[:500] and 'Increment 113' not in text[:500], name
    header, rest = text.split('\n', 1)
    path.write_text(header + '\n\n' + body.strip() + '\n\n' + rest.lstrip(), encoding='utf-8')


def main():
    backend = read(E.parent / 'file-comparison-backend/result.json')
    proof, media, web, mobile, runtime, android = [read(E / n) for n in ('final-source-check.json', 'media.json', 'web-build.json', 'mobile-build.json', 'runtime-build.json', 'android-test-scope.json')]
    assert backend['passed'] and proof['passed'] and media['passed'] and web['passed'] and mobile['passed'] and runtime['status'] == 'passed'
    top, records = backend['discovered_top_level_tests'], len(backend['checks'])
    nw, nm, nr = web['tests']['numPassedTests'], mobile['tests']['pass'], runtime['tests']['pass']
    assert (nw, nm, nr) == (238, 332, 55)
    jvm_cn = '来自Gradle缓存' if android['gradle_cached'] else '本轮实际执行'
    jvm_en = 'came from Gradle cache' if android['gradle_cached'] else 'executed in this increment'
    zh = '增量113已实现四端文件版本正文比较：人工明确选定两个正式版本，完整正文行差异、前后文与全文分页、末尾换行核对、原件／正文摘要分离及固定比较依据。选择不自动读取；文件／权限／扫描依据变化后拒绝旧页并隐藏旧结果。支持文本、Markdown、CSV、JSON及DOCX主文档正文；更多格式、PDF／OCR、排版与复杂DOCX仍在GAP-04，完整四端业务旅程仍在GAP-18。23项（14 P0／9 P1）及5项移出保持不变；仅移出AI费用控制，秘书、八项Skill、TG、隔离、授权、审计及技术限制继续保留。'
    en = 'Increment 113 adds file version text comparison across Web and the shared WeChat, Douyin and Android pages: explicit exact-version selection, complete derived-text line diffs, context or all-text paging, final-newline checks, separate original/text hashes and pinned comparison identity. Selection alone does not read a diff. Changed file, authority or scan evidence rejects old pages and hides stale results. Text, Markdown, CSV, JSON and DOCX main-body text are supported; wider formats, PDF/OCR, layout and complex DOCX remain GAP-04, with full business journeys under GAP-18. All 23 packages (14 P0/9 P1) and 5 exclusions remain. Only AI fee controls are excluded; secretary, eight Skills, TG, isolation, authorization, audit and technical limits remain required.'
    usage = '操作：文件详情 → 比较两个内容版本 → 选择基础／目标准确版本 → 明确比较 → 选择差异前后文或全部正文并重新比较 → 按页核对。提取正文相同不表示原件相同；遇到依据变化，刷新文件并核对现行资料后重新比较。'
    usage_en = 'Usage: open file details, select exact base/target versions, explicitly compare, choose changes/context or all text, and review the pinned pages. Equal derived text does not imply equal original bytes. Refresh current file metadata and review the selection after a changed-basis conflict.'
    verify_cn = f'当前源码核验：后端{top}个顶层测试／{records}条结果，Web{nw}、共享移动{nm}、运行时{nr}；八项客户端命令实际执行，双小程序各20页。Web与Taro H5各6组实际页面／API检查；新增7 PNG、2静音MP4、2 GIF，255份旧媒体保留原字节。Android开发APK通过18＋4项静态检查，26条JVM{jvm_cn}；schema保持87，无新增或重跑迁移；独立全耐久原生PG及2个Linux交叉构建已核对。不声明真机、模拟器、真实云／模型／TG或生产验收。下方旧增量统计保留原日期与适用范围。'
    verify_en = f'Current-source checks: {top} backend top-level tests / {records} records, Web {nw}, shared mobile {nm} and runtime {nr}; all eight client commands execute and both miniapps retain 20 pages. Web and Taro H5 each pass 6 actual page/API groups. New media: 7 PNGs, 2 silent MP4s and 2 GIFs; 255 previous media retain original bytes. A development APK passes 18 + 4 static checks; 26 JVM results {jvm_en}. Schema remains 87 with no new or replayed migration. Full-durability native temporary PG and 2 Linux cross-builds are checked. No device, emulator, live cloud/model/TG or production acceptance is claimed. Older increment statistics below retain their original dates and scope.'
    for name, prefix, language in (('README.md', 'docs/', 'both'), ('docs/README.md', '', 'both'), ('docs/README.zh-CN.md', '', 'cn'), ('docs/README.en.md', '', 'en')):
        links = f'[设计与使用 / Design and usage]({prefix}development/113-file-version-comparison.md) · [验证与来源 / Verification]({prefix}quality/113-file-version-comparison-verification.md) · [截图／录屏／GIF / Media]({prefix}media/README.md) · [全部未完成项 / Remaining work]({prefix}delivery/03-goal-gap-assessment.md)'
        paras = [zh, en, usage, usage_en, verify_cn, verify_en] if language == 'both' else ([zh, usage, verify_cn] if language == 'cn' else [en, usage_en, verify_en])
        if name == 'README.md':
            paras.append('![文件版本正文比较 / File version text comparison](docs/media/gifs/file-comparison-web.gif)')
        prepend(name, '\n\n'.join(paras + [links]))
    intro = '增量113：GAP-04／18已加入四端准确正式版本的完整提取正文比较、固定摘要分页与过期依据拒绝。更多格式、原件排版、PDF／OCR、复杂DOCX和完整业务旅程继续开放；23 active（14 P0／9 P1）／5 excluded不变。见[设计与使用](../development/113-file-version-comparison.md)及[验证](../quality/113-file-version-comparison-verification.md)。'
    for name in ('docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md', 'docs/delivery/04-current-scope-and-acceptance.md'):
        prepend(name, intro)
    scope_path = ROOT / 'docs/delivery/04-current-scope.json'
    scope = read(scope_path)
    gap_path = ROOT / 'docs/delivery/03-goal-gap-assessment.md'
    gaps = gap_path.read_text(encoding='utf-8')
    for row in scope['items']:
        if row['id'] not in ('GAP-04', 'GAP-18'):
            continue
        if row['id'] == 'GAP-04':
            replacement = '补更多格式的读取／预览／创建／编辑与原件排版差异；用PDF、Office、OCR及复杂DOCX样本核对实际支持范围。已支持提取格式的完整正文比较见增量113。'
            assert row['remaining'] in gaps
            gaps = gaps.replace(row['remaining'], replacement)
            row['remaining'] = replacement
        row.setdefault('completed_adaptations', []).append('FILE-COMPARISON-113')
        row['completed_detail'] = (row.get('completed_detail', '') + ' 增量113已实现准确正式版本的完整正文行差异、原件／正文区分、有界完整替换降级、固定依据分页及四端入口；实际Web／H5核对全文还原和过期依据拒绝，整个工作包继续active。').strip()
    gap_path.write_text(gaps, encoding='utf-8')
    save(scope_path, scope)
    prepend('docs/CHANGELOG.md', '\n\n'.join(['## 增量113 / Increment 113 · 文件版本正文比较 / File version text comparison', zh, en, verify_cn, verify_en, '新增只读比较接口与4个模型，保留原325路径／713模型语义，现326路径／402操作／717模型。使用完整不可变提取正文，有界Myers及完整中段替换降级；19个共享边界算例由Web／移动分别执行，算法用独立还原和LCS最短编辑结果核对。没有数据库结构升级、云或模型调用。', '[实现与使用](development/113-file-version-comparison.md) · [原始验证与辅助尝试](quality/113-file-version-comparison-verification.md)']))
    prepend('docs/media/README.md', '''增量113 / Increment113：实际认证后的合成Web／Taro H5文件版本正文比较；新增7 PNG、2静音MP4及2 GIF，255份旧媒体保持原字节。录制在登录与MFA完成后开始；全文分页实际还原准确正文，脚本文字按普通文本显示。

| 客户端 | 截图 | 录屏 | GIF |
|---|---|---|---|
| Web | [前后文](screenshots/file-comparison-web-context.png)、[全文](screenshots/file-comparison-web-full.png)、[依据变化](screenshots/file-comparison-web-stale.png)、[深色](screenshots/file-comparison-web-dark.png) | [静音MP4](videos/file-comparison-web.mp4) | [GIF](gifs/file-comparison-web.gif) |
| 共享移动 | [前后文](screenshots/file-comparison-mobile-context.png)、[全文](screenshots/file-comparison-mobile-full.png)、[依据变化](screenshots/file-comparison-mobile-stale.png) | [静音MP4](videos/file-comparison-mobile.mp4) | [GIF](gifs/file-comparison-mobile.gif) |

[真实帧与编码来源](../quality/evidence/file-comparison/media.json) · [本轮核验](../quality/113-file-version-comparison-verification.md)。不录制凭据或真实业务数据；不声明真机、真实云、模型或生产验收。''')
    quality = f'''# 文件版本正文比较验证 / File version comparison verification

增量113推进GAP-04／18，整个工作包继续开放。[协议、使用和实现约束](../development/113-file-version-comparison.md)。

{verify_cn}

{verify_en}

| 编号 | 核验 | 证据与实际边界 |
|---|---|---|
| VC-V01 | 完整后端 | {top}顶层／{records}条结果；全部发现项执行，无skip。至多20个名称一批、串行原生隔离库；[完整结果](evidence/file-comparison-backend/result.json)、[准确私有日志摘要](evidence/file-comparison-backend/raw-log-archive.json)。 |
| VC-V02 | 算法 | 两个测试族覆盖307组固定／随机输入：独立还原两侧完整内容，以独立LCS动态规划核对小输入最少编辑；还测Unicode、空行、空正文、末尾换行、长行、上下文省略计数、资源拒绝与取消。600行变化触发完整中段替换降级，不截断冒充完整差异。 |
| VC-V03 | 实际API／PG | 11组：未就绪拒绝、逐页完整还原、前后文／方向、同版本／规范化相同与原件不同、无业务写入、越权／跨文件／不支持、严格参数与固定摘要、回收站／隔离／分类变更、元数据修订、会话撤销、CSV／JSON／DOCX实际提取。原件由受控对象读取器提供，经实际索引Worker及事务；不声明真实云。 |
| VC-V04 | 四端客户端 | [八项命令](evidence/file-comparison/client-checks.json)均实际执行；Web238、移动332、运行时55。Web／移动各执行同一19个共享边界用例，不能算38个不同业务场景。微信／抖音各20页构建；不表示平台提交成功。 |
| VC-V05 | 实际页面 | [Web](evidence/file-comparison-web/result.json)与[Taro H5](evidence/file-comparison-mobile/result.json)各6组：实际认证、选择不读取、准确历史版本、差异与省略、脚本文字、全文分页摘要还原、原件／正文区别、篡改固定摘要与真实改名后的旧页拒绝、清除旧结果、页面宽度。无响应替换、状态注入或直接数据库修改代替页面操作。 |
| VC-V06 | Android | 当前源码生成新Metro／Gradle开发APK，18项包检查＋4项业务内容静态检查；26条JVM{jvm_cn}，保留四份实际XML及任务状态。未启用实体设备、模拟器或虚拟机；正式签名／发版继续GAP-16。 |
| VC-V07 | 媒体 | 7 PNG、2静音MP4、2 GIF，实际认证后合成页面像素；[准确帧与编码](evidence/file-comparison/media.json)、[目视核对](evidence/file-comparison/visual-review.json)。255份历史媒体按已发布字节保留。 |
| VC-V08 | 持久化与服务 | schema仍87，无迁移新增或重跑。只替换核对归属的开发API和两项Worker，历史费用／用量资料摘要及行数保留；[更新](evidence/file-comparison/development-update.json)、[只读检查](evidence/file-comparison/development-project-resources.json)。未启用真实云、模型、TG或生产接入。 |
| VC-V09 | API与Linux | 326路径／402操作／717模型；原325路径／713模型内容不变。当前源码两个Linux amd64 ELF交叉构建；未执行目标Linux内核隔离或部署。 |
| VC-V10 | 资源释放 | 两端自有浏览器上下文、页面、HTTP服务和移动代理、隔离测试数据库均结束并关闭；所有后端检查后关闭独立全耐久原生临时PG，0残留测试库。原开发PG和用户其他进程／页面保留。 |

开发服务首次更新时，已核对归属的API终止命令返回非零，辅助器停止，未继续启动替代进程。后续独立检查确认原API已退出、18080端口已释放、两项原Worker仍运行。修正为“命令执行后重新核对实际归属与退出状态”，再启动／检查当前源码服务；不凭终止命令的返回值认定服务已就绪。[该辅助失败](evidence/file-comparison/attempts/01-development-stop-exit-code/attempt.json)保留准确原辅助器摘要，不重新标记成通过；没有改应用源码或复用失效业务测试。

运行入口：原生测试数据库由 `file_comparison_native_cluster.py prepare` 创建；`verify_file_comparison_backend.py` 执行完整发现项；`verify_file_comparison_clients.py` 执行八项命令；`run_file_comparison_browser.py` 分别检查Web与 `--mobile`；Android使用 `build_android.py --google-mirror --evidence-group file-comparison` 和两项包检查；Linux使用 `build_file_comparison_linux.py`。原生集群结束时 `file_comparison_native_cluster.py stop`；之后运行 `verify_file_comparison_sources.py`、`verify_acceptance_applicability.py`、`verify_current_scope.py` 与仓库候选／暂存区检查。凭据和准确私有日志不入Git。

依赖保留增量108实际日期的完整npm审计0项及安全结果，本轮不声称重新审计或更新公告。根manifest、锁与依赖字段不变，仅新增contracts源码导出和移动测试选择。真实Codex、Linux隔离、八项Skill正式发布／评测、实际OSS／R2／TG、正式Android与小程序、生产发布及完整公司旅程继续见[23项稳定编号](../delivery/03-goal-gap-assessment.md)。真机、网络性能／实网120GB、历史数据迁移／容灾及AI费用控制维持移出；取消范围不记成测试通过。
'''
    (ROOT / 'docs/quality/113-file-version-comparison-verification.md').write_text(quality, encoding='utf-8')
    (E / 'README.md').write_text('''# Increment113 evidence / 增量113证据

记录实际原生PG、完整后端、四端构建、实际Web／H5、开发APK、截图／录屏／GIF与明确复用；不把开发包或缓存结果称为正式发布。

| 记录 | 内容 |
|---|---|
| client-checks.json、三份build摘要 | 八项实际命令、准确源码及私有日志摘要；摘要适配不重跑测试。 |
| ../file-comparison-backend | 全部发现Go项、每批结果、准确原始日志保全。 |
| ../file-comparison-web、../file-comparison-mobile | 实际页面／API、全文还原、固定分页、准确源码、截图及资源释放。 |
| build.json、apk-inspection.json、android-feature-content.json | 新开发APK、包与业务内容静态检查，无设备执行。 |
| android-test-scope.json、android-unit-results | 26条JVM实际任务缓存范围及四份XML。 |
| media.json、capture-manifest、visual-review.json | 认证后实际像素、准确帧、静音编码和目视核对。 |
| native-cluster.json、native-cluster-closed.json | 独立全耐久原生测试PG，结束后0测试库并关闭。 |
| development-update.json、development-project-resources.json | schema87保持、归属核对后的服务更新、历史费用保留及当前只读检查。 |
| linux-build.json | 两个当前Linux交叉构建，没有执行或部署。 |
| reused-evidence.json | 255份媒体与依赖检查保留原已发布字节和日期。 |
| attempts | 首次服务终止命令非零的辅助失败，单独保留，不记成成功。 |
| final-source-check.json、scope-check.json、repository-check.json | 当前功能源码、范围与发布候选检查。 |

[验证与边界](../../113-file-version-comparison-verification.md) · [设计与使用](../../../development/113-file-version-comparison.md)。私有连接资料、数据库、工具、源码试跑归档、原始帧、APK和缓存不入Git。23个工作包仍待完成，5项移出。
''', encoding='utf-8')
    agents = ROOT / 'AGENTS.md'
    agents.write_text(agents.read_text(encoding='utf-8').rstrip() + '\n\n- Increment113 adds explicit exact-version full derived-text comparison across four clients, complete pinned pagination, byte/text equality separation and stale-authority rejection. Keep wider formats, original layout/PDF/OCR/complex DOCX and full business journeys open under GAP-04/18; all23 packages and five exclusions remain. See `docs/development/113-file-version-comparison.md`.\n', encoding='utf-8')
    baseline_path = ROOT / 'docs/quality/current-functional-baseline.json'
    baseline = read(baseline_path)
    assert baseline['increment'] == 112
    baseline.update(increment=113, evidence_group='file-comparison', mobile_tests=nm,
        verification_script='verify_file_comparison_sources.py', backend_evidence_group='file-comparison-backend',
        backend_top_level_tests=top, backend_result_records=records, web_evidence_group='file-comparison', runtime_evidence_group='file-comparison', web_tests=nw, runtime_tests=nr,
        development_verification_script='verify_file_comparison_development.py', interaction_kind='file_version_text_comparison', interaction_sources='file_comparison_sources.py',
        retained_evidence_record='docs/quality/evidence/file-comparison/reused-evidence.json', planning_scope_verification_record='docs/quality/evidence/file-comparison/scope-check.json', planning_publication_record='docs/quality/evidence/file-comparison/repository-check.json')
    changed = subprocess.check_output(['git', 'diff', '--name-only', '-z'], cwd=ROOT).decode().split('\0')
    added = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    future = ['docs/quality/evidence/file-comparison/' + n for n in ('final-source-check.json', 'scope-check.json', 'acceptance-applicability-check.json')]
    candidates = set(baseline['publication_files'] + future + [n for n in changed + added if n and (ROOT / n).is_file()])
    baseline['publication_files'] = sorted(n for n in candidates if Path(n).suffix.lower() not in {'.png', '.gif', '.mp4', '.webm', '.log'} and n != 'docs/quality/evidence/file-comparison/repository-check.json')
    save(baseline_path, baseline)
    print(json.dumps({'passed': True, 'increment': 113, 'backend_top_level': top, 'backend_records': records, 'active': 23, 'excluded': 5}))


if __name__ == '__main__':
    main()
