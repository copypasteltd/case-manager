"""Publish increment112 documentation using completed, explicitly scoped evidence."""
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/file-recovery'
def read(path):
    return json.loads(path.read_text(encoding='utf-8'))
def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
def prepend(name, text):
    path = ROOT / name
    before = path.read_text(encoding='utf-8')
    assert '增量112' not in before[:500] and 'Increment 112' not in before[:500], name
    heading, rest = before.split('\n', 1)
    path.write_text(heading + '\n\n' + text.strip() + '\n\n' + rest.lstrip(), encoding='utf-8')

def main():
    backend = read(E.parent / 'file-recovery-backend/result.json')
    assert backend['passed']
    top, count = backend['discovered_top_level_tests'], len(backend['checks'])
    client = read(E / 'client-checks.json')
    assert client['passed'] and len(client['checks']) == 8
    counts = {v['name']: v['test_count'] for v in client['checks'] if v.get('test_count') is not None}
    assert counts == {'web-tests': 219, 'mobile-tests': 313, 'runtime-tests': 55}
    jvm = read(E / 'android-test-scope.json')
    assert jvm['passed']
    jvm_cn = '来自Gradle缓存' if jvm['gradle_cached'] else '本轮实际执行'
    jvm_en = 'came from Gradle cache' if jvm['gradle_cached'] else 'executed in this increment'
    for name in ('media.json', 'development-project-resources.json', 'development-update.json', 'linux-build.json', 'android-feature-content.json'):
        assert read(E / name)['passed'], name
    for target in ('web', 'mobile'):
        assert read(E.parent / ('file-recovery-' + target) / 'result.json')['passed']
    cn = '增量112已实现四端秘书准确草稿恢复：员工选定已结束请求中的准确草稿，核对差异和摘要后明确创建新任务；固定原目标、基础版本和来源，重新上传、校验与扫描，再通过原有授权计划正式提交。支持文件更新与新文件创建；原任务与回执保持历史事实。依赖变化、扫描隔离、会话撤销、取消及冲突会阻断执行。GAP-04／12中的该子流程已实现，更多格式、全文差异、完整OA附件和真实模型／Skill验收仍开放；23项（14 P0／9 P1）及5项移出不变。仅移出AI费用控制，秘书、八项Skill、TG、隔离、授权、审计和技术限制保留。'
    en = 'Increment 112 adds exact saved-draft recovery across Web and the shared WeChat, Douyin and Android secretary pages. A human selects an ended request’s exact draft, reviews its diff and digest, and explicitly creates a new run. Original target, base and provenance are fixed; execution uploads, verifies and scans fresh bytes before the existing authorized formal commit. Updates and new-file drafts are supported. Dependency changes, quarantine, revoked sessions, cancellation and conflicts stop execution. This completes a subfeature of GAP-04/12; wider formats, full-text diffs, complete OA attachments and real model/Skill acceptance remain open. All 23 packages (14 P0/9 P1) and 5 exclusions remain. Only AI fee controls are excluded; secretary, eight Skills, TG, isolation, authorization, audit and technical limits remain required.'
    usage_cn = '操作：本人私密秘书会话 → 选择文件任务 → 选择原请求与准确草稿 → 核对差异／摘要并勾选声明 → 加入请求 → 明确发送 → 核对新任务来源及真实执行状态。接单不等于上传、扫描或正式提交；来源或基础版本变化时，须人工核对现行内容后发起新编辑。'
    usage_en = 'Usage: open your own private secretary conversation, choose the file task, select the original request and exact draft, review and confirm the diff/digest, add the recovery and explicitly send. Inspect new-run provenance and actual execution state. Admission does not mean upload, scanning or formal commit. Changed sources or bases require human review and a new edit request.'
    verify_cn = f'本轮源码核验：后端{top}个顶层测试／{count}条结果，Web219、共享移动313、运行时55；八项客户端命令全部执行，双小程序各20页。Web与Taro H5各5组真实页面／API检查；新增7 PNG、2静音MP4、2 GIF，244份旧媒体保留原字节。Android开发APK通过18＋4项静态检查，26条JVM{jvm_cn}；普通schema86→87升级、全耐久原生临时PG及2个Linux交叉构建已核对。无真机、模拟器、真实云／模型／TG或生产验收声明。下方旧增量的统计保留原执行日期与适用范围。'
    verify_en = f'Current-source checks: {top} backend top-level tests / {count} records, Web 219, shared mobile 313 and runtime 55; all eight client commands execute and each miniapp retains 20 pages. Web and Taro H5 each pass 5 actual page/API groups. New media: 7 PNGs, 2 silent MP4s and 2 GIFs; 244 previous media retain original bytes. A development APK passes 18 + 4 static checks; 26 JVM results {jvm_en}. A normal schema 86 to 87 upgrade, fully durable temporary native PG and 2 Linux cross-builds are verified. No device, emulator, live cloud/model/TG or production acceptance is claimed. Older increment statistics below retain their original dates and scope.'
    for name, prefix, language in (('README.md', 'docs/', 'both'), ('docs/README.md', '', 'both'), ('docs/README.zh-CN.md', '', 'cn'), ('docs/README.en.md', '', 'en')):
        links = f'[设计与使用 / Design and usage]({prefix}development/112-secretary-file-recovery.md) · [验证与来源 / Verification]({prefix}quality/112-secretary-file-recovery-verification.md) · [截图／录屏／GIF / Media]({prefix}media/README.md) · [全部未完成项 / Remaining work]({prefix}delivery/03-goal-gap-assessment.md)'
        paras = [cn, en, usage_cn, usage_en, verify_cn, verify_en] if language == 'both' else ([cn, usage_cn, verify_cn] if language == 'cn' else [en, usage_en, verify_en])
        prepend(name, '\n\n'.join(paras + [links]))
    intro = '增量112：GAP-04／12的准确草稿跨运行恢复已接通四端、实际API、不可变PG来源和专用MCP工具；旧草稿重建后须重新上传、校验、扫描和授权提交。更多格式、全文差异、完整OA附件与真实模型验收继续开放；23 active（14 P0／9 P1）／5 excluded不变。见[设计与使用](../development/112-secretary-file-recovery.md)及[验证](../quality/112-secretary-file-recovery-verification.md)。'
    for name in ('docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md', 'docs/delivery/04-current-scope-and-acceptance.md'):
        prepend(name, intro)
    scope_path = ROOT / 'docs/delivery/04-current-scope.json'
    scope = read(scope_path)
    changes = {'GAP-04': '补更多格式的读取／预览／创建／编辑与全文差异；用PDF、Office、OCR及复杂DOCX样本核对实际支持范围。',
        'GAP-12': '补更多项目工具、文件格式与跨范围整理、完整OA正文附件、更多申请类型与模板写作、跨订阅完整审阅计划和人员／模板目录；评测异常判断质量。'}
    gap_path = ROOT / 'docs/delivery/03-goal-gap-assessment.md'
    text = gap_path.read_text(encoding='utf-8')
    for row in scope['items']:
        if row['id'] not in changes:
            continue
        assert row['remaining'] in text
        text = text.replace(row['remaining'], changes[row['id']])
        row['remaining'] = changes[row['id']]
        row.setdefault('completed_adaptations', []).append('FILE-RECOVERY-112')
        row['completed_detail'] = (row.get('completed_detail', '') + ' 增量112已实现人工选定准确草稿、新运行不可变来源、原目标／基础版本重查、依赖展开、新上传与扫描及正式提交；支持更新和新文件，整个工作包继续active。').strip()
    gap_path.write_text(text, encoding='utf-8')
    save(scope_path, scope)
    prepend('docs/CHANGELOG.md', '\n\n'.join(['## 增量112 / Increment 112 · 秘书准确草稿恢复 / Exact saved-draft recovery', cn, en, verify_cn, verify_en,
        '新增迁移87、不可变恢复来源、`file_recovery`接单字段、`recover_file_draft`专用工具及14个共享边界算例。保留325路径／401操作，模型增至713；原模型仅三处增加恢复字段。早期夹具失败与源码变更导致的失效回归分别保留，最终检查使用完整当前源码。',
        '[实现与使用](development/112-secretary-file-recovery.md) · [验证、原始来源与尝试](quality/112-secretary-file-recovery-verification.md)']))
    media = '''增量112 / Increment112：实际认证后的合成Web／Taro H5准确草稿恢复，新增7 PNG、2静音MP4及2 GIF；244份旧媒体保持原字节。录制从认证后开始，无密码、MFA输入或真实业务记录。新任务接单演示不代表真实模型执行完成。

| 客户端 | 核对差异 | 已选内容 | 接单来源 | 录屏 | GIF |
|---|---|---|---|---|---|
| Web | [PNG](screenshots/file-recovery-web-review.png) | [PNG](screenshots/file-recovery-web-selected.png) | [PNG](screenshots/file-recovery-web-accepted.png)、[深色](screenshots/file-recovery-web-dark.png) | [静音MP4](videos/file-recovery-web.mp4) | [GIF](gifs/file-recovery-web.gif) |
| 共享移动 | [PNG](screenshots/file-recovery-mobile-review.png) | [PNG](screenshots/file-recovery-mobile-selected.png) | [PNG](screenshots/file-recovery-mobile-accepted.png) | [静音MP4](videos/file-recovery-mobile.mp4) | [GIF](gifs/file-recovery-mobile.gif) |

[准确帧与编码来源](../quality/evidence/file-recovery/media.json) · [本轮核验](../quality/112-secretary-file-recovery-verification.md)。'''
    prepend('docs/media/README.md', media)
    quality = f'''# 秘书准确草稿恢复验证 / Exact draft recovery verification

增量112完成GAP-04／12的准确草稿跨运行恢复子流程，整个工作包继续开放。[设计、使用、协议与约束](../development/112-secretary-file-recovery.md)。

{verify_cn}

{verify_en}

| 编号 | 核验 | 实际证据与边界 |
|---|---|---|
| FR-V01 | 全部原生后端 | {top}个顶层／{count}条结果；全部发现项执行，无skip。每批至多20个名称，串行隔离库；[结果](evidence/file-recovery-backend/result.json)与[准确私有日志摘要](evidence/file-recovery-backend/raw-log-archive.json)。 |
| FR-V02 | 实际恢复业务 | 13组API／PG／MCP场景：原任务终态、准确选择与确认、过期草稿重建、严格工具字段、不可变来源、旧草稿／计划拒绝、二次恢复新扫描、正式提交原子回滚与幂等、新文件BOM／CRLF、隔离拒绝、上传中取消、工具依赖变化、后续隔离、会话撤销与新分片中止。使用受控原生证明、对象后端与扫描夹具，不是真实模型或云。 |
| FR-V03 | 跨端源码与构建 | [八项命令](evidence/file-recovery/client-checks.json)实际执行，Web219、移动313、运行时55；Web和移动各执行同一组14个共享边界算例，不能解释为28个不同业务场景；双小程序各20页。 |
| FR-V04 | 真实页面 | [Web](evidence/file-recovery-web/result.json)与[Taro H5](evidence/file-recovery-mobile/result.json)各5组：实际登录与MFA、原请求／差异读取、选择不自动发送、明确确认、新任务接单与持久来源、篡改摘要拒绝、原任务保持及页面宽度。无API响应替换。页面演示未执行真实模型。 |
| FR-V05 | Android | 当前源码生成新的Metro／Gradle开发APK，18项普通包检查＋4项恢复内容静态检查；26条JVM{jvm_cn}，保留4份原XML与实际Gradle任务状态。不使用真机、虚拟机或模拟器；不声明正式签名／上架。 |
| FR-V06 | 媒体 | 新增7 PNG、2静音MP4、2 GIF；实际认证后合成页面像素，保留准确帧、时间及编码输入；[媒体证明](evidence/file-recovery/media.json)与[目视核对](evidence/file-recovery/visual-review.json)。原244份媒体保留原字节及适用范围。 |
| FR-V07 | 原生持久化 | 原开发实例正常schema86→87升级；历史AI费用／技术用量行数与内容摘要保留；新表只读／追加权限及5项恢复约束已核对；只替换核对过归属的API及2个Worker，外部接入关闭，不声明生产部署。 |
| FR-V08 | API与Linux | 325路径／401操作／713模型；全部旧路径语义保持，旧模型仅RunCreate及两类文件回执增加恢复字段。2个当前源码Linux amd64 ELF交叉编译，未在Linux执行或部署。 |
| FR-V09 | 资源释放 | 两端浏览器上下文、页面、临时服务与代理、隔离业务数据库结束后关闭；独立全耐久原生临时PG在全部检查后关闭。原开发PG与用户无关页面／进程保留。 |

早期页面准备尝试未给合成员工授予无业务动作的MFA要求角色，直接请求注册导致422；13个业务场景已通过，但尚未打开浏览器。该失败与准确源码ZIP／私有原始日志分别保留在[attempt01](evidence/file-recovery/attempts/01-browser-mfa-role/attempt.json)，没有放宽MFA。修正仅对显式启用的浏览器夹具生效。

修正浏览器夹具期间，首轮完整后端回归的源码发生变化；辅助器将第4批标记为失效并拒绝后续批次。前三批只对旧源码有效，没有并入最终结果。[attempt02](evidence/file-recovery/attempts/02-backend-source-refresh/attempt.json)保留原批次记录和准确日志；旧源码来自attempt01的源码ZIP，修正后当前源码另存ZIP。最终完整回归从头重新发现并执行所有批次，全部绑定当前源码。

当前源码回归在第5批及Android打包期间又发生外部进程中断；原句柄消失，Windows进程检查确认对应测试／构建／PG均已停止。[attempt03](evidence/file-recovery/attempts/03-process-interruption/attempt.json)保留准确中断日志与原辅助器。已经完整通过的第1～4批绑定相同当前源码与准确日志，因此保留原执行事实；第5批从头重跑，其余批次继续执行，没有把中断片段拼成通过。Android从头构建新的完整产物，原中断bundle不作成功依据。

原生PG按准确原目录恢复，没有重新初始化；开发服务恢复使用独立脚本，不重跑schema86→87，也不覆盖原升级证明。最初恢复辅助器遗漏显式端口参数，开发PG临时启动在默认loopback5432，被断言拒绝；核对准确原生进程、目录、端口后只停止该自有进程，再以14660启动。应用角色无权读取`data_directory`的拒绝结果保持，未扩权；该辅助失败另行保留。本轮最终检查均在恢复后的实际资源上重新核对。

早期局部开发日志保留在忽略目录：编译夹具、取消状态断言、严格字段错误码及实际扫描枚举修正不计入最终通过。已通过的局部试跑不代替最终全量回归。依赖专项沿用增量108实际日期的完整审计0项及既有安全结果，本轮不声称重新检索公告或重新审计；根manifest／锁及依赖字段不变，仅新增contracts源码导出和移动测试选项。

真实Codex接入、Linux隔离验收、八项Skill正式发布／评测、实际OSS／R2／TG、正式Android与小程序发布、生产部署及公司完整业务旅程继续见[23项稳定编号](../delivery/03-goal-gap-assessment.md)。设备验证、网络性能／实网120GB、历史数据迁移／容灾和AI费用控制保持移出；不把取消范围记为通过。
'''
    (ROOT / 'docs/quality/112-secretary-file-recovery-verification.md').write_text(quality, encoding='utf-8')
    evidence_readme = '''# Increment112 evidence / 增量112证据

本目录记录准确草稿恢复的本轮实际检查与明确复用。总体说明见[验证文档](../../112-secretary-file-recovery-verification.md)，设计见[开发文档](../../../development/112-secretary-file-recovery.md)。

| 记录 | 内容 |
|---|---|
| client-checks.json及三份build摘要 | 八项实际命令、当前源码摘要、准确私有日志散列；摘要适配不重复执行测试。 |
| ../file-recovery-backend | 全部发现Go测试、每批结果及准确原始日志保全；真实原生PG。 |
| ../file-recovery-web、../file-recovery-mobile | 实际页面/API、完整捕获源码清单、截图和资源释放；移动使用Taro H5。 |
| build.json、apk-inspection.json、android-feature-content.json | 新开发APK、当前包与业务内容的静态证据；不执行设备。 |
| android-test-scope.json、android-unit-results | 26条JVM原始XML、Gradle任务状态及缓存／新执行范围。 |
| media.json、两份capture-manifest、visual-review.json | 实际认证页面帧、静音编码及目视核对；无密码或MFA录制。 |
| native-cluster.json、native-cluster-closed.json | 独立全耐久本机原生PG，未改原开发集群，结束后0测试库并关闭。 |
| development-update.json、development-project-resources.json | 普通schema86→87升级、历史费用数据保留、归属核对后的API／Worker替换、只读检查。 |
| linux-build.json | 两个Linux ELF交叉构建，不执行／部署Linux。 |
| reused-evidence.json | 原244份媒体及依赖证据按准确已发布字节保留；没有重新审计依赖。 |
| attempts | 失败／失效尝试独立保留；准确源码和原始日志在忽略目录，由公开摘要绑定。 |
| process-restoration.json、development-service-restoration.json | 已确认进程退出后的原生PG与3项自有服务恢复；不重新初始化、不重跑结构升级；中断片段不记为通过。 |
| final-source-check.json、scope-check.json、repository-check.json | 当前源码、范围与最终发布候选的分层核对。 |

凭据、连接串、私有原始日志、数据库、源码试跑ZIP、原始帧和APK不入Git。所有示例均为隔离合成数据。生产就绪、真实模型／云／TG以及正式发布继续开放，当前23项待完成、5项移出不变。
'''
    (E / 'README.md').write_text(evidence_readme, encoding='utf-8')
    agents = ROOT / 'AGENTS.md'
    agents.write_text(agents.read_text(encoding='utf-8').rstrip() + '\n\n- Increment112 adds explicitly selected exact file draft recovery into a new run, immutable provenance, current dependency/session checks and fresh upload/scan/commit evidence. Keep wider formats, full-text diffs, complete OA attachments and real model/Skill acceptance open under GAP-04/12/13; all23 packages and five exclusions remain. See `docs/development/112-secretary-file-recovery.md`.\n', encoding='utf-8')
    baseline_path = ROOT / 'docs/quality/current-functional-baseline.json'
    baseline = read(baseline_path)
    assert baseline['increment'] == 111
    baseline.update(increment=112, evidence_group='file-recovery', mobile_tests=313,
        verification_script='verify_file_recovery_sources.py', backend_evidence_group='file-recovery-backend',
        backend_top_level_tests=top, backend_result_records=count, web_evidence_group='file-recovery',
        runtime_evidence_group='file-recovery', web_tests=219, runtime_tests=55,
        development_verification_script='verify_file_recovery_development.py', interaction_kind='exact_file_draft_recovery',
        interaction_sources='file_recovery_sources.py', retained_evidence_record='docs/quality/evidence/file-recovery/reused-evidence.json',
        planning_scope_verification_record='docs/quality/evidence/file-recovery/scope-check.json',
        planning_publication_record='docs/quality/evidence/file-recovery/repository-check.json')
    changed = subprocess.check_output(['git', 'diff', '--name-only', '-z'], cwd=ROOT).decode().split('\0')
    added = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    # The publication report is an output: it must not be required before its first run.
    future = ['docs/quality/evidence/file-recovery/' + n for n in ('final-source-check.json', 'scope-check.json', 'acceptance-applicability-check.json')]
    candidates = set(baseline['publication_files'] + future + [n for n in changed + added if n and (ROOT / n).is_file()])
    # This list applies text-layout rules to code and guides. Media and exact raw
    # logs still undergo the verifier's full candidate/index, secret and link checks.
    baseline['publication_files'] = sorted(n for n in candidates if Path(n).suffix.lower() not in {'.png', '.gif', '.mp4', '.webm', '.log'})
    save(baseline_path, baseline)
    print(json.dumps({'passed': True, 'increment': 112, 'backend_top_level': top, 'backend_records': count, 'active': 23, 'excluded': 5}))

if __name__ == '__main__':
    main()
