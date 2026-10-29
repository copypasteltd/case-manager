"""Publish increment115 guides from completed current-source evidence, once."""
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/scope-transfer'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def prepend(name, body):
    path = ROOT / name
    raw = path.read_bytes()
    assert '增量115'.encode() not in raw[:500] and b'Increment 115' not in raw[:500], name
    first, rest = raw.split(b'\n', 1)
    newline = b'\r\n' if first.endswith(b'\r') else b'\n'
    block = body.strip().encode().replace(b'\n', newline)
    path.write_bytes(first + b'\n' + newline + block + newline + newline + rest.lstrip(b'\r\n'))


def main():
    baseline_path = ROOT / 'docs/quality/current-functional-baseline.json'
    baseline = read(baseline_path)
    assert baseline['increment'] == 114
    backend = read(E.parent / 'scope_transfer-backend/result.json')
    proof, media, web, mobile, runtime, jvm = [read(E / n) for n in
        ('final-source-check.json', 'media.json', 'web-build.json', 'mobile-build.json', 'runtime-build.json', 'android-test-scope.json')]
    assert backend['passed'] and proof['passed'] and media['passed'] and web['passed'] and mobile['passed'] and runtime['status'] == 'passed'
    top, records = backend['discovered_top_level_tests'], len(backend['checks'])
    nw, nm, nr = web['tests']['numPassedTests'], mobile['tests']['pass'], runtime['tests']['pass']
    assert (top, nw, nm, nr) == (236, 281, 375, 55)
    retained = len(read(E / 'reused-evidence.json')['media'])
    jvm_cn = '来自Gradle缓存' if jvm['gradle_cached'] else '本轮实际执行'
    jvm_en = 'came from Gradle cache' if jvm['gradle_cached'] else 'executed in this increment'
    zh = '增量115已接通四端受控人工跨范围文件整理：员工从实际目录选择跨项目／分类目标，先分析完整清单及普通／AI读取策略变化，再以准确计划摘要、原因、明确声明和近期MFA确认执行。目录树逐项重检，正式内容历史保留；正式业务引用、保留锁、在途任务、分类降级或陈旧依据阻止执行。最终决定及审计不可改写，重复提交返回同一回执。'
    en = 'Increment 115 adds human reviewed file and folder-tree transfers across Web, WeChat, Douyin and Android. Select an actual cross-project/classification destination, analyze the complete fixed inventory and changed ordinary/AI read-policy combinations, then confirm the exact plan with a reason, acknowledgement and recent MFA. Execution rechecks every item and preserves immutable content history. Business references, retention holds, in-flight work, classification downgrades and stale facts block execution. Final decisions and audit evidence remain immutable; idempotent retries return one receipt.'
    usage = '使用：文件与资料 → 实际文件或非根目录 →“拟定跨范围移动”→ 选择实际目标 →“分析跨范围移动”→ 核对全部清单及读取权限变化 → 明确声明、原因及MFA → 确认执行。移动端从回执“打开移动后的资料”；未知结果先刷新本人计划历史，不重复创建。'
    usage_en = 'Usage: open a file or non-root folder in Files, choose 拟定跨范围移动, select an actual destination and analyze. Review the full inventory and read-policy changes, provide a reason and acknowledgement, then confirm with current MFA. On mobile, open the new location from the receipt. Refresh owner history before retrying an uncertain result.'
    scope_zh = '仍有23项工作包（14 P0／9 P1）和5项移出。正式业务引用的跨范围协同、大树／批量策略及完整四端旅程继续GAP-18；AI跨范围工具、预授权及TG披露继续GAP-11。本功能不授予AI新工具。仅排除AI费用控制，秘书、8项Skill、TG、隔离、授权、审计和技术限制保留。'
    scope_en = 'All 23 packages (14 P0/9 P1) and 5 exclusions remain. Transfers involving formal business references, larger/bulk policies and complete four-client journeys remain GAP-18. AI scope-changing tools, preauthorization and TG disclosure remain GAP-11. Only AI fee controls are excluded; secretary, eight Skills, TG, isolation, authorization, audit and technical limits remain required.'
    validation_zh = f'当前源码：后端{top}顶层／{records}条结果，Web{nw}、共享移动{nm}、运行时{nr}；八项客户端命令通过，双小程序各20页。Web／Taro H5各6组实际页面与API检查；6 PNG、2静音MP4、2截图轮播GIF，{retained}份旧媒体原样保留。Android开发APK通过18＋3静态检查，26条JVM{jvm_cn}。原生开发schema87→88，旧AI费用／用量资料保留；独立全耐久测试PG关闭且无临时库，两个Linux交叉构建通过。不声明真机、模拟器、真实云／模型／TG或生产验收。'
    validation_en = f'Current source: {top} backend top-level tests / {records} records; Web {nw}, shared mobile {nm}, runtime {nr}. Eight client commands pass; both miniapps retain 20 routes. Web/Taro H5 each pass 6 actual UI/API groups. New media: 6 PNGs, 2 silent MP4s and 2 screenshot-tour GIFs; {retained} older media retain their bytes. A development APK passes 18 + 3 static checks; 26 JVM results {jvm_en}. Native development schema upgrades87→88 while legacy AI fee/usage records stay fixed. The full-durability test PG closes without temporary databases; 2 Linux targets cross-build. No device, emulator, live cloud/model/TG or production acceptance is claimed.'
    for name, prefix, language in (('README.md', 'docs/', 'both'), ('docs/README.md', '', 'both'), ('docs/README.zh-CN.md', '', 'cn'), ('docs/README.en.md', '', 'en')):
        paragraphs = [zh, en, usage, usage_en, scope_zh, scope_en, validation_zh, validation_en] if language == 'both' else ([zh, usage, scope_zh, validation_zh] if language == 'cn' else [en, usage_en, scope_en, validation_en])
        links = f'[设计与使用 / Design]({prefix}development/115-file-scope-transfers.md) · [验证 / Verification]({prefix}quality/115-file-scope-transfers-verification.md) · [截图／录屏／GIF / Media]({prefix}media/README.md) · [全部未完成项 / Remaining work]({prefix}delivery/03-goal-gap-assessment.md)'
        prepend(name, '\n\n'.join(paragraphs + [links]))
    intro = '增量115：GAP-18新增四端人工跨项目／分类文件与目录树固定计划、完整权限影响核对、近期MFA明确执行和不可变回执。正式业务引用联动、大树／批量策略及完整旅程继续开放；AI跨范围工具与预授权仍GAP-11。仍23 active（14 P0／9 P1）、5 excluded。见[设计与使用](../development/115-file-scope-transfers.md)及[验证](../quality/115-file-scope-transfers-verification.md)。'
    for name in ('docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md', 'docs/delivery/04-current-scope-and-acceptance.md'):
        prepend(name, intro)
    scope_path = ROOT / 'docs/delivery/04-current-scope.json'
    scope = read(scope_path)
    gap_path = ROOT / 'docs/delivery/03-goal-gap-assessment.md'
    raw = gap_path.read_bytes()
    for row in scope['items']:
        if row['id'] != 'GAP-18': continue
        replacement = '按四端矩阵核验项目／OA／财务旅程、普通附件、账期跨域核对；补正式业务引用文件及超限目录树的跨范围协同、批量策略编辑器和保留执行器；核对上传／暂停／重试／凭据过期等功能。受控人工跨范围计划见增量115。'
        assert row['remaining'].encode() in raw
        raw = raw.replace(row['remaining'].encode(), replacement.encode())
        row['remaining'] = replacement
        row.setdefault('completed_adaptations', []).append('HUMAN-FILE-SCOPE-TRANSFER-115')
        row['completed_detail'] = (row.get('completed_detail', '') + ' 增量115加入四端人工跨范围文件／目录树整理、全项权限变化分析、固定摘要与近期MFA确认、原子版本更新及不可变回执；正式业务引用／超限树、AI工具及完整工作包仍未验收。').strip()
    gap_path.write_bytes(raw)
    save(scope_path, scope)
    prepend('docs/CHANGELOG.md', '\n\n'.join(['## 增量115 / Increment 115 · 跨范围文件整理 / Reviewed file scope transfers', zh, en, validation_zh, validation_en,
        '新增6路径／6操作／8模型，现333路径／409操作／729模型，原327路径／721模型语义保留。schema88增加固定计划与最终决定守卫，不自动授予权限。13组新增API／PG检查和24个共享边界用例形成当前回归；Web和移动执行同一24例，不算48个不同场景。早期上传夹具、类型目标及浏览器目标选择失败单独保留，未记成最终通过。', '[设计与使用](development/115-file-scope-transfers.md) · [验证与尝试](quality/115-file-scope-transfers-verification.md)']))
    prepend('docs/media/README.md', '''增量115 / Increment 115：受控人工跨范围文件整理。真实认证后的隔离合成页面，6 PNG、2静音MP4、2截图导览GIF。视频录制固定计划的实际阅读与刷新；确认／MFA输入阶段不录制，实际执行由页面检查和执行后截图覆盖。GIF按每张2秒展示计划、窄屏与最终回执，属于截图导览。

| 端 | 固定计划 | 窄屏 | 执行回执 | 录屏 | GIF导览 |
|---|---|---|---|---|---|
| Web | [PNG](screenshots/scope-transfer-web-plan.png) | [PNG](screenshots/scope-transfer-web-narrow.png) | [PNG](screenshots/scope-transfer-web-receipt.png) | [静音MP4](videos/scope-transfer-web.mp4) | [GIF](gifs/scope-transfer-web.gif) |
| 共享移动 | [PNG](screenshots/scope-transfer-mobile-plan.png) | [PNG](screenshots/scope-transfer-mobile-narrow.png) | [PNG](screenshots/scope-transfer-mobile-receipt.png) | [静音MP4](videos/scope-transfer-mobile.mp4) | [GIF](gifs/scope-transfer-mobile.gif) |

[帧、时长与来源](../quality/evidence/scope-transfer/media.json) · [验证](../quality/115-file-scope-transfers-verification.md)。旧媒体保留原字节和执行范围；不声明真实云／模型／TG或生产验收。''')
    verification = f'''# 跨范围文件整理核验 / File scope transfer verification

增量115核验当前源码、原生PG、四端构建及实际Web／Taro H5操作。[设计与使用](../development/115-file-scope-transfers.md) · [完整未完成清单](../delivery/03-goal-gap-assessment.md)。

| 编号 | 检查 | 结果与边界 |
|---|---|---|
| FT-V01 | 全量后端 | {top}个顶层测试／{records}条结果，12个原生串行批次；全部发现用例执行，无跳过。结果包含嵌套子用例，不代表{records}个不同业务场景。 |
| FT-V02 | 新API／PG | 13组：完整内容历史与权限增减、真实人类身份／伪造渠道、严格输入／本人历史、近期MFA及准确摘要、分页、陈旧目标与撤销、引用／保留／上传阻塞、授权变更、伪造最终回执拒绝、真实权限时间到期、分类降级与跨类型同名冲突、原子执行及幂等、完整目录树与后代目标拒绝。 |
| FT-V03 | 共享边界 | Web{nw}、移动{nm}、运行时{nr}；Web／移动分别执行同一24个新共享边界用例，不算48个不同场景。 |
| FT-V04 | 客户端 | 八项类型、测试与构建命令针对同一源码运行；微信／抖音各20页，正式平台审批另验收。 |
| FT-V05 | 真实页面 | 两端各6组：真实登录与MFA、实际目标选择及不移动的分析、完整计划及响应布局录制、明确声明及MFA后实际执行、真实新位置导航、旧项目成员刷新失去文件。没有API响应替换或组件状态注入。 |
| FT-V06 | Android | 当前开发APK通过18＋3静态检查；26条JVM{jvm_cn}，保留4份XML和准确任务状态。未启动设备或模拟器，正式签名／升级发布仍GAP-16。 |
| FT-V07 | 媒体 | 6 PNG、2静音MP4、2截图导览GIF。录屏只覆盖固定计划阅读与刷新，不包括MFA输入；实际执行另有API检查与回执截图。GIF每张2秒共约6秒，不称连续操作录屏。{retained}份旧媒体按已发布字节保留。 |
| FT-V08 | 数据库与服务 | 全耐久独立临时PG在检查后关闭，0临时测试库；开发PG87→88正常结构升级，原AI费用／用量四表内容保持，仅替换归属核对的API与两项Worker。外部集成保持关闭。 |
| FT-V09 | 合约与Linux | 333路径／409操作／729模型，原327路径／721模型语义保留；两个Linux amd64 ELF交叉构建，不等于目标Linux执行或隔离验收。 |
| FT-V10 | 范围／发布 | 23 active（14 P0／9 P1）、5 excluded、8项draft Skill保持；30 AC／72 TC适用性分类不执行业务案例。当前源码、范围、适用性、链接、媒体及候选／实际暂存区检查分别记录；普通提交推送以Git提交为准。 |

Early fixture attempts consumed an aborted upload's pending initiation before later content helpers. Processing that single real lifecycle job resolved the fixture; assuming an extra provider-abort job was incorrect because no provider multipart upload existed. Both failed logs remain private with published hashes. The earlier successful targeted run predates the additional permission-expiry case and is not final-source full regression evidence.

早期类型检查分别发现记录类型收窄、故意错误枚举的测试类型，以及移动目标不支持Object.hasOwn；已修正，最终八项命令独立记录。前两次浏览器目标按钮等待超时，未提交分析／执行，实际目标目录已显示；检查脚本误用按钮可见文字，忽略了用于区分目标的实际可访问名称。失败截图、资源关闭、私有日志摘要及校验过的原源码归档均保留。检查脚本改用实际可访问按钮名称，并以服务器回执的目标ID及版本核对实际选择。旧失败或被替代记录均不记成当前通过。

首次归档与检查脚本编辑发生竞争；未通过源码绑定的初始压缩包只在私有目录保留。该次发布引用的归档按原清单恢复两行检查脚本并逐文件核对，最终整体摘要与原失败检查的清单一致；归档记录明确标注重建过程。此后归档完成后再编辑。

The checks establish this bounded human workflow using controlled local data. They do not establish production readiness, real cloud/model/TG integration, platform submission or company rule adoption. Cached JVM results and retained evidence preserve their original scope.

| 证据 | 链接 |
|---|---|
| 当前源码绑定 | [结果](evidence/scope-transfer/final-source-check.json) |
| 全量后端 | [结果](evidence/scope_transfer-backend/result.json)、[批次](evidence/scope_transfer-backend/batch-progress.json)、[原始日志摘要](evidence/scope_transfer-backend/raw-log-archive.json) |
| 客户端命令 | [八项命令](evidence/scope-transfer/client-checks.json) |
| 实际页面 | [Web](evidence/scope-transfer-web/result.json)、[Taro H5](evidence/scope-transfer-mobile/result.json) |
| Android | [构建](evidence/scope-transfer/build.json)、[静态检查](evidence/scope-transfer/apk-inspection.json)、[内容检查](evidence/scope-transfer/android-feature-content.json)、[JVM范围](evidence/scope-transfer/android-test-scope.json) |
| 媒体 | [编码与来源](evidence/scope-transfer/media.json)、[目视核对](evidence/scope-transfer/visual-review.json) |
| PG与服务 | [原生PG](evidence/scope-transfer/native-cluster.json)、[关闭](evidence/scope-transfer/native-cluster-closed.json)、[升级](evidence/scope-transfer/development-update.json)、[只读核对](evidence/scope-transfer/development-project-resources.json) |
| Linux及复用 | [交叉构建](evidence/scope-transfer/linux-build.json)、[保留证据](evidence/scope-transfer/reused-evidence.json) |
| 早期尝试 | [定向检查](evidence/scope-transfer/attempts/targeted-checks.json)、[首次浏览器](evidence/scope-transfer/attempts/01-target-selector/attempt.json)、[第二次浏览器](evidence/scope-transfer/attempts/02-target-button-name/attempt.json) |
'''
    (ROOT / 'docs/quality/115-file-scope-transfers-verification.md').write_text(verification, encoding='utf-8')
    (E / 'README.md').write_text('''# 增量115证据 / Increment115 evidence

当前源码与原生PG、人类页面操作、四端构建和真实像素分别记录。final-source-check.json核对准确输入与证据；client-checks.json保留八项命令和私有日志摘要；Android记录准确缓存状态；native-cluster及关闭记录证明耐久配置及自有资源释放；development记录schema87→88及原资料保留。media与visual-review绑定录制、截图和目视采样；reused-evidence保留旧媒体与依赖检查的原日期和范围。attempts里的旧失败和被替代检查不计当前通过。

凭据、数据库、工具、APK、原始帧及私有源码归档不入Git。[设计](../../../development/115-file-scope-transfers.md) · [核验](../../115-file-scope-transfers-verification.md)。
''', encoding='utf-8')
    baseline.update(increment=115, evidence_group='scope-transfer', mobile_tests=nm,
        verification_script='verify_scope_transfer_sources.py', backend_evidence_group='scope_transfer-backend',
        backend_top_level_tests=top, backend_result_records=records, web_evidence_group='scope-transfer', runtime_evidence_group='scope-transfer', web_tests=nw, runtime_tests=nr,
        development_verification_script='verify_scope_transfer_development.py', interaction_kind='human_file_scope_transfer', interaction_sources='scope_transfer_sources.py',
        retained_evidence_record='docs/quality/evidence/scope-transfer/reused-evidence.json', planning_scope_verification_record='docs/quality/evidence/scope-transfer/scope-check.json', planning_publication_record='docs/quality/evidence/scope-transfer/repository-check.json')
    changed = subprocess.check_output(['git', 'diff', '--name-only', '-z'], cwd=ROOT).decode().split('\0')
    added = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    future = ['docs/quality/evidence/scope-transfer/' + n for n in ('final-source-check.json', 'scope-check.json', 'acceptance-applicability-check.json')]
    candidates = set(baseline['publication_files'] + future + [n for n in changed + added if n and (ROOT / n).is_file()])
    baseline['publication_files'] = sorted(n for n in candidates if Path(n).suffix.lower() not in {'.png', '.gif', '.mp4', '.webm', '.log'} and n != 'docs/quality/evidence/scope-transfer/repository-check.json')
    save(baseline_path, baseline)
    print(json.dumps({'passed': True, 'increment': 115, 'backend_top_level': top, 'backend_records': records, 'active': 23, 'excluded': 5}))


if __name__ == '__main__':
    main()
