"""Publish increment114 guides only after final source-bound evidence is complete."""
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/workbench'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def prepend(name, body):
    path = ROOT / name
    raw = path.read_bytes()
    assert '增量114'.encode() not in raw[:500] and b'Increment 114' not in raw[:500], name
    first, rest = raw.split(b'\n', 1)
    newline = b'\r\n' if first.endswith(b'\r') else b'\n'
    block = body.strip().encode().replace(b'\n', newline)
    path.write_bytes(first + b'\n' + newline + block + newline + newline + rest.lstrip(b'\r\n'))


def main():
    baseline_path = ROOT / 'docs/quality/current-functional-baseline.json'
    baseline = read(baseline_path)
    assert baseline['increment'] == 113, 'This guarded publication bootstrap runs once.'
    backend = read(E.parent / 'workbench-backend/result.json')
    proof, media, web, mobile, runtime, jvm = [read(E / n) for n in
        ('final-source-check.json', 'media.json', 'web-build.json', 'mobile-build.json', 'runtime-build.json', 'android-test-scope.json')]
    assert backend['passed'] and proof['passed'] and media['passed'] and web['passed'] and mobile['passed'] and runtime['status'] == 'passed'
    top, records = backend['discovered_top_level_tests'], len(backend['checks'])
    nw, nm, nr = web['tests']['numPassedTests'], mobile['tests']['pass'], runtime['tests']['pass']
    assert (top, records, nw, nm, nr) == (235, 1435, 257, 351, 55)
    jvm_cn = '来自Gradle缓存' if jvm['gradle_cached'] else '本轮实际执行'
    jvm_en = 'came from Gradle cache' if jvm['gradle_cached'] else 'executed in this increment'
    zh = '增量114已接通四端业务工作台：本人当前有效审批、本人负责的生效合同到期及已生成订阅账期待核对。三个分组共享原生PG事务快照，先授权计数再展示前5条，点击进入原业务详情。合同使用正式生效修订及香港日期；账期待核对不表示欠款或已扣费。刷新失败隐藏旧汇总，撤权后重新读取清除旧数量与标题。'
    en = 'Increment 114 adds a business workbench across Web and the shared WeChat, Douyin and Android home pages: current assigned approvals, owned effective-contract expiry and generated subscription periods requiring review. One native PG snapshot counts authorized matches before selecting five-item samples. Fixed links open ordinary business details. Contracts use the active revision and Hong Kong dates; period review does not imply debt or an actual charge. Failed refreshes hide old results; refreshed authority removes revoked counts and titles.'
    usage = '使用：登录 → 首页业务待办 → 核对数量与日期 → 点击具体请求、合同或账期 → 在原详情核对当前依据再处理。Web使用“刷新业务待办”，移动端使用“刷新工作台”；超过5条提示到对应业务列表继续查阅。'
    usage_en = 'Usage: sign in, review Business work on Home, open an exact request, contract or billing period, then inspect current facts before acting. Refresh the business desk on Web or the home page on mobile. Samples above five items disclose that additional matches exist; the business lists retain their own filters.'
    scope_zh = '仍有23项工作包（14 P0／9 P1）与5项移出；服务／文件异常等更多聚合、实际存储故障、跨渠道回执、公司规则选用与完整业务旅程继续开放。仅移出AI费用控制；秘书、八项Skill、TG、隔离、授权、审计及技术限制保留。'
    scope_en = 'All 23 packages (14 P0/9 P1) and 5 exclusions remain. Wider service/file aggregation, actual storage failures, cross-channel receipts, company policy adoption and complete business journeys stay open. Only AI fee controls are excluded; secretary, eight Skills, TG, isolation, authorization, audit and technical limits remain required.'
    validation_zh = f'当前源码核验：后端{top}个顶层测试／{records}条结果，Web{nw}、共享移动{nm}、运行时{nr}；八项客户端命令重新执行，微信／抖音各20页。Web与Taro H5各6组实际页面／API检查；新增6 PNG、2静音MP4、2截图轮播GIF，266份旧媒体保持原字节。Android开发APK通过18＋3项静态检查，26条JVM{jvm_cn}。schema保持87，无迁移新增或重跑；独立全耐久原生PG及2个Linux交叉构建已核对。不声明真机、模拟器、真实云／模型／TG或生产验收；旧增量记录保留原日期与范围。'
    validation_en = f'Current-source checks: {top} backend top-level tests / {records} records; Web {nw}, shared mobile {nm}, runtime {nr}. All eight client commands execute; WeChat/Douyin retain 20 pages each. Web and Taro H5 each pass 6 actual page/API groups. New media: 6 PNGs, 2 silent MP4 recordings and 2 screenshot-tour GIFs; 266 prior media keep exact bytes. A development APK passes 18 + 3 static checks; 26 JVM results {jvm_en}. Schema stays 87 with no new or replayed migration. Full-durability native test PG and 2 Linux cross-builds are checked. No device, emulator, live cloud/model/TG or production acceptance is claimed. Older records retain their original dates and scope.'
    for name, prefix, language in (('README.md', 'docs/', 'both'), ('docs/README.md', '', 'both'), ('docs/README.zh-CN.md', '', 'cn'), ('docs/README.en.md', '', 'en')):
        paragraphs = [zh, en, usage, usage_en, scope_zh, scope_en, validation_zh, validation_en] if language == 'both' else ([zh, usage, scope_zh, validation_zh] if language == 'cn' else [en, usage_en, scope_en, validation_en])
        links = f'[设计与使用 / Design]({prefix}development/114-business-workbench.md) · [验证 / Verification]({prefix}quality/114-business-workbench-verification.md) · [截图／录屏／GIF / Media]({prefix}media/README.md) · [全部未完成项 / Remaining work]({prefix}delivery/03-goal-gap-assessment.md)'
        prepend(name, '\n\n'.join(paragraphs + [links]))
    intro = '增量114：GAP-08／18已加入四端本人有效审批、生效合同到期与订阅账期待核对工作台、完整授权计数和准确业务详情入口。更多聚合、实际故障／跨渠道回执及完整业务旅程继续开放；23 active（14 P0／9 P1）／5 excluded不变。见[设计与使用](../development/114-business-workbench.md)及[验证](../quality/114-business-workbench-verification.md)。'
    for name in ('docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md', 'docs/delivery/04-current-scope-and-acceptance.md'):
        prepend(name, intro)
    scope_path = ROOT / 'docs/delivery/04-current-scope.json'
    scope = read(scope_path)
    gap_path = ROOT / 'docs/delivery/03-goal-gap-assessment.md'
    raw = gap_path.read_bytes()
    newline = b'\r\n' if b'\r\n' in raw else b'\n'
    for row in scope['items']:
        if row['id'] not in ('GAP-08', 'GAP-18'):
            continue
        if row['id'] == 'GAP-08':
            replacement = '补服务事件、文件／复制异常等更多业务聚合、实际存储复制失败来源与跨渠道送达回执；实际公司规则选用、真实集成及整体功能验收继续开放。本人审批、合同到期与订阅核对聚合见增量114，站内受控重试及版本化合同提醒见增量110／111。'
            assert row['remaining'].encode() in raw
            raw = raw.replace(row['remaining'].encode(), replacement.encode())
            row['remaining'] = replacement
        row.setdefault('completed_adaptations', []).append('BUSINESS-WORKBENCH-114')
        row['completed_detail'] = (row.get('completed_detail', '') + ' 增量114已实现三类本人业务汇总、完整当前授权计数、前5条样本、准确业务详情入口、刷新隐藏旧结果及实际撤权核对；四端源代码与Web／H5实操形成对应证据，整个工作包继续active。').strip()
    gap_path.write_bytes(raw)
    save(scope_path, scope)
    prepend('docs/CHANGELOG.md', '\n\n'.join(['## 增量114 / Increment 114 · 业务工作台 / Business workbench', zh, en, validation_zh, validation_en,
        '新增只读业务汇总接口及4个模型，原326路径／717模型语义保持，现327路径／403操作／721模型。审批队列与工作台共享完整当前轮次与权限查询。19个共享边界用例分别由Web／移动执行，不算38个不同场景。仅使用既有PG结构，无云、模型或通知副作用。早期测试初始化、Web类型包含配置、因测试源码修正失效的回归及移动刷新按钮定位失败均单独保留；修正检查脚本后Web及H5均重新实操。最终源码的后端12批覆盖全部测试；第9批测试前环境查询超时单独保留，按原时限重试，前8个完整批次校验源码和日志摘要后续跑。',
        '[设计与使用](development/114-business-workbench.md) · [验证与尝试记录](quality/114-business-workbench-verification.md)']))
    prepend('docs/media/README.md', '''增量114 / Increment 114：业务工作台来自认证后的隔离合成Web／Taro H5页面；新增6 PNG、2静音MP4和2截图轮播GIF，266份旧媒体保留原字节。MP4保留认证后首页概览与实际刷新录制；GIF按每张2秒轮播真实的汇总、窄屏与其他员工截图。GIF是截图导览，实际业务交互由页面检查覆盖；全部素材不含登录、密码或MFA输入。

| 端 | 汇总 | 窄屏 | 其他员工 | 录屏 | GIF截图导览 |
|---|---|---|---|---|---|
| Web | [PNG](screenshots/workbench-web-summary.png) | [PNG](screenshots/workbench-web-narrow.png) | [PNG](screenshots/workbench-web-other-member.png) | [静音MP4](videos/workbench-web.mp4) | [GIF](gifs/workbench-web.gif) |
| 共享移动 | [PNG](screenshots/workbench-mobile-summary.png) | [PNG](screenshots/workbench-mobile-narrow.png) | [PNG](screenshots/workbench-mobile-other-member.png) | [静音MP4](videos/workbench-mobile.mp4) | [GIF](gifs/workbench-mobile.gif) |

[实际帧、时长和编码来源](../quality/evidence/workbench/media.json) · [验证](../quality/114-business-workbench-verification.md)。媒体展示本地受控业务，不声明真机、真实云／模型／TG或生产验收。''')
    (ROOT / 'docs/quality/114-business-workbench-verification.md').write_text(f'''# 业务工作台验证 / Business workbench verification

增量114的核验对象是当前工作台源码及本轮构建产物。整体目标尚未完成，23个工作包仍active，5项移出。[设计与使用](../development/114-business-workbench.md) · [全部未完成项](../delivery/03-goal-gap-assessment.md)。

| 编号 | 检查 | 实际结果与边界 |
|---|---|---|
| BW-V01 | 完整后端回归 | 最终源码全部{top}个顶层测试／{records}条结果，12个原生批次覆盖全部测试；无跳过、替换或先前源码批次复用。第9批在Go测试前的环境查询按原30秒时限超时，失败原样保留；续跑校验同一源码与前8个完整批次的日志摘要后，重试第9批并执行余批。结果记录包含嵌套子用例，不等于{records}个不同业务场景。 |
| BW-V02 | 新业务接口 | 10组真实API／PG检查：严格查询、空可读范围、有效审批计数及队列一致性、香港日期和30天边界、已生成订阅预测与待核对口径、只读无副作用、已完成步骤消失、真实角色撤销、运行会话／TG／陈旧授权拒绝、真实撤销会话／取消读取及四个人类渠道。Web负责事项另由实际MFA浏览器覆盖；部分原生跨渠道断言使用无负责事项的真实普通成员会话。 |
| BW-V03 | 共享客户端边界 | Web{nw}、移动{nm}、运行时{nr}。Web和移动分别执行同一19个新共享用例，不算38个不同场景。检查日期、完整3分组、完整计数与5条样本、固定引用、状态、排序、文字及路由边界。 |
| BW-V04 | 八项客户端命令 | 类型检查、三组测试、Web／运行时构建、微信／抖音构建均重新执行，前后完整输入摘要一致。双小程序各20页；构建成功不代表真实平台提交／发布。 |
| BW-V05 | 实际页面 | Web及Taro H5各6组：真实登录与MFA、准确数量与有限样本、认证后录制、三类实际详情链接、刷新与窄屏、其他员工隔离、实际角色撤销后旧数量／标题清除。无API响应替换、组件状态注入或用数据库业务状态伪造代替业务页面操作。撤权通过仅测试编译的受控fixture入口实施实际角色变更。 |
| BW-V06 | Android | 当前源码生成开发bundle与APK，18项常规包检查＋3项业务内容静态检查。26条JVM{jvm_cn}，四份XML保留实际时间和任务状态。没有设备或模拟器执行，正式签名／发布仍GAP-16。 |
| BW-V07 | 媒体 | 6 PNG、2静音MP4、2截图轮播GIF来自实际认证后合成页面；视频帧来源和实际时长、GIF截图来源与每张2秒间隔、目视采样范围单独记录。266份旧媒体按已发布字节保留。 |
| BW-V08 | 原生持久化环境 | 独立临时PostgreSQL保持fsync、同步提交及完整页写入开启；检查结束0临时测试库并关闭自有集群。开发PG与用户无关进程保留；未改变用例时限或耐久配置。 |
| BW-V09 | 开发服务 | 仅替换核对归属的本地API及两项Worker。schema仍87，没有迁移新增或重跑；既有历史AI费用／用量四表的行数和内容摘要保留。真实云、模型、TG及平台凭据继续关闭。 |
| BW-V10 | API与Linux | 327路径／403操作／721模型；原326路径／717模型语义保持。两个当前Linux amd64 ELF交叉构建通过；没有目标Linux执行、内核隔离或生产部署声明。 |
| BW-V11 | 范围与验收适用性 | 保持23 active（14 P0／9 P1）、5 excluded、8项draft Skill。原30 AC／72 TC适用性校验不执行业务案例，正式业务案例通过数仍为0；GAP-28保持开放。 |
| BW-V12 | 发布 | 当前功能源码绑定、范围、适用性、文档链接、媒体、候选及实际暂存区检查按各自结果记录；提交和普通推送完成后以Git提交为准。 |

The final source passes the complete native backend run, all eight client commands and actual Web/H5 journeys. These are local source, artifact and controlled business checks. They do not establish production readiness, real cloud/model/TG integration, platform approval or company rule adoption. Cached JVM outputs and retained dependency/media evidence preserve their original execution scope.

早期尝试单独保留，均不改记为最终通过：首次初始化在没有MFA注册挑战时调用注册辅助器而停止；Web复合类型项目漏列共享文件，随后补齐；浏览器初次准备发现合成Alice没有录制所需MFA凭据，修正为临时授予注册角色、完成真实MFA注册后撤销该临时角色。该测试源码修正使先前回归结果失效，最终源码从第1批重新覆盖全量。移动浏览器首轮把普通刷新按钮误写为带行尾箭头的名称，等待响应超时；已保留失败及此前Web通过记录，改为实际按钮名并让点击与响应等待共同处理异常，随后两端均重新检查。该修正只改变检查脚本，不沿用此前Web通过记录作为最终源码通过。最终回归第9批的测试前原生PG环境查询曾超时，未运行的Go案例未计为通过；保留失败后按原时限续跑，核对同源码已完成批次的精确日志摘要。应用业务状态没有通过直接SQL伪造。

初版Web／H5功能检查已经通过，但目视采样发现滚动快于采样间隔，演示几乎静止。该轮页面结果和原始帧均保留为被替代记录；仅在检查脚本增加实际页面阅读停留、固定窄屏录制与真实刷新，再重新执行两端检查与录制。应用源码、业务流程及后端回归输入不因此变化。

补录一轮在登录、录制和三类详情链接通过后，返回首页等待标题超时，失败截图为空白；当时没有记录到准确导航位置，原因未确认，不归因为已修复的业务错误。保留该轮后增加失败栈和不含业务正文的页面结构诊断。曾尝试整页重载来重置首页，但这会清除H5有意只保存在内存中的会话，诊断确认正确跳回登录；该测试策略已改为实际浏览器“返回”，保留原会话并等待首页显示。普通详情仍通过实际点击访问，应用会话机制没有改动。

一次重试在H5准备构建的原720秒时限内未完成，浏览器和原生fixture均未启动，自有构建及代理已关闭；该结果不计业务失败或通过，单独保存并重试。没有修改业务用例时限或数据库耐久配置。

Web复核曾超过原150秒环境准备等待，fixture在退出等待期间才写出，旧检查器未重新读取其释放凭据，停止子进程后留下一个断开的合成库。已保留私有fixture、精确日志和完整合成库dump，核对独立PG归属、唯一库名、项目ID及零连接后，只清理该库；另有明确清理记录。检查器现于结束阶段重新识别自有子进程的迟到fixture并请求释放。仅将准备等待调整为300秒；既有150秒结束等待、12分钟fixture持有、业务检查时限和数据库耐久配置均保持。此前移动端通过记录保留为旧检查器来源，两端随后重新检查。

| 记录 | 链接 |
|---|---|
| 最终源码绑定 | [final-source-check.json](evidence/workbench/final-source-check.json) |
| 后端全量与原始日志 | [结果](evidence/workbench-backend/result.json)、[批次](evidence/workbench-backend/batch-progress.json)、[私有日志保全](evidence/workbench-backend/raw-log-archive.json) |
| 客户端命令 | [client-checks.json](evidence/workbench/client-checks.json) |
| 实际页面与资源释放 | [Web](evidence/workbench-web/result.json)、[Taro H5](evidence/workbench-mobile/result.json) |
| Android范围 | [构建](evidence/workbench/build.json)、[包检查](evidence/workbench/apk-inspection.json)、[业务内容](evidence/workbench/android-feature-content.json)、[JVM缓存范围](evidence/workbench/android-test-scope.json) |
| 媒体与目视核对 | [编码及来源](evidence/workbench/media.json)、[目视采样](evidence/workbench/visual-review.json) |
| 原生PG | [启动与耐久](evidence/workbench/native-cluster.json)、[关闭与零测试库](evidence/workbench/native-cluster-closed.json) |
| 开发服务 | [更新](evidence/workbench/development-update.json)、[只读核对](evidence/workbench/development-project-resources.json) |
| Linux | [交叉构建](evidence/workbench/linux-build.json) |
| 复用证据 | [266份媒体及原依赖结果](evidence/workbench/reused-evidence.json) |
| 早期尝试 | [MFA挑战](evidence/workbench/attempts/01-existing-mfa-fixture/attempt.json)、[浏览器准备及失效源码](evidence/workbench/attempts/02-browser-mfa-fixture/source.json)、[类型包含配置](evidence/workbench/attempts/03-web-composite-includes/attempt.json)、[移动刷新定位](evidence/workbench/attempts/04-mobile-refresh-selector/attempt.json)、[测试前环境超时](evidence/workbench/attempts/05-native-attestation-timeout/attempt.json) |
| 初版录屏替代记录 | [目视发现、历史通过及精确来源核对](evidence/workbench/attempts/06-recording-readable-holds/attempt.json) |
| 补录导航等待失败 | [失败、清理与辅助脚本来源核对](evidence/workbench/attempts/07-mobile-home-navigation/attempt.json) |
| H5构建准备超时 | [未启动浏览器／fixture的准备失败](evidence/workbench/attempts/08-h5-build-timeout/attempt.json) |
| 整页重载与内存会话 | [已确认的检查策略问题](evidence/workbench/attempts/09-h5-memory-session-reload/attempt.json) |
| Web迟到环境与受控清理 | [准备失败与私有快照](evidence/workbench/attempts/10-web-late-fixture/attempt.json)、[唯一合成库清理](evidence/workbench/attempts/10-web-late-fixture/controlled-cleanup.json) |

重复检查入口见 `app/scripts/` 下的 `workbench_native_cluster.py`、`verify_workbench_backend.py`、`verify_workbench_clients.py`、`run_workbench_browser.py`、`build_android.py`、`build_workbench_linux.py` 及对应摘要／包／最终来源检查器。测试数据库由本轮独立原生所有权记录管理；不要对已关闭的历史记录直接复用活动所有权。准确私有日志、数据库、凭据、原始帧、工具、APK及源码试跑归档不入Git。

Only AI fee controls are excluded. Secretary execution, all eight Skills, TG, isolation, authorization, audit and technical resource limits remain required. Device validation, network performance/actual-network 120 GB tests and historical migration/disaster recovery remain excluded without being counted as passed.
''', encoding='utf-8')
    agents = ROOT / 'AGENTS.md'
    original = agents.read_bytes()
    newline = b'\r\n' if b'\r\n' in original else b'\n'
    statement = '- Increment114 adds a read-only personal business workbench across four clients: exact active approvals, effective-contract expiry and generated subscription reviews, complete authorized counts and bounded samples. Preserve current human sessions, shared approval visibility, active-revision/date semantics and stale-result hiding. Wider aggregation, actual failures/cross-channel receipts and full business acceptance remain GAP-08/18; all23 packages and five exclusions remain. See `docs/development/114-business-workbench.md`.'
    agents.write_bytes(original.rstrip(b'\r\n') + newline + newline + statement.encode() + newline)
    (E / 'README.md').write_text('''# 增量114证据 / Increment 114 evidence

本目录记录当前业务工作台的实际验证。Web与共享移动测试各执行同一19个边界用例；没有设备、模拟器、云、模型、TG或生产验收声明。

| 记录 | 适用范围 |
|---|---|
| final-source-check.json | 当前源码与全部证据绑定；整体goal仍未完成。 |
| client-checks.json、web-build.json、mobile-build.json、runtime-build.json | 八项实际命令、完整输入摘要与私有原始日志摘要。 |
| ../workbench-backend | 最终全量原生后端发现项、每批结果及原始日志保全。 |
| ../workbench-web、../workbench-mobile | 实际登录、三类业务链接、刷新、其他员工与实际撤权；全部自有资源关闭。 |
| build.json、apk-inspection.json、android-feature-content.json | 当前开发APK及静态包／业务内容；没有设备执行。 |
| android-test-scope.json、android-unit-results | 26条JVM的实际任务状态，准确区分缓存和新执行。 |
| media.json、visual-review.json、capture-manifest | 真实认证后像素、静音编码、准确帧与目视采样范围。 |
| native-cluster.json、native-cluster-closed.json | 独立全耐久原生PG，最终0临时库并关闭。 |
| development-update.json、development-project-resources.json | 归属核对后的开发服务替换；schema87及历史费用／用量资料保留。 |
| linux-build.json | 两个当前Linux交叉构建；没有Linux执行或部署。 |
| reused-evidence.json | 266份历史媒体与原依赖检查保留准确字节和日期。 |
| attempts | 初始化、Web类型包含修正、失效回归、移动刷新定位、测试前环境超时及补录导航等待失败；初版录屏替代记录单独保存原通过与来源核对，不计当前通过。 |

私有日志、连接凭据、数据库、工具、APK、原始帧和源码试跑归档不入Git。[设计](../../../development/114-business-workbench.md) · [核验](../../114-business-workbench-verification.md)。
''', encoding='utf-8')
    baseline.update(increment=114, evidence_group='workbench', mobile_tests=nm,
        verification_script='verify_workbench_sources.py', backend_evidence_group='workbench-backend',
        backend_top_level_tests=top, backend_result_records=records, web_evidence_group='workbench', runtime_evidence_group='workbench', web_tests=nw, runtime_tests=nr,
        development_verification_script='verify_workbench_development.py', interaction_kind='human_business_workbench', interaction_sources='workbench_sources.py',
        retained_evidence_record='docs/quality/evidence/workbench/reused-evidence.json', planning_scope_verification_record='docs/quality/evidence/workbench/scope-check.json', planning_publication_record='docs/quality/evidence/workbench/repository-check.json')
    changed = subprocess.check_output(['git', 'diff', '--name-only', '-z'], cwd=ROOT).decode().split('\0')
    added = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    future = ['docs/quality/evidence/workbench/' + n for n in ('final-source-check.json', 'scope-check.json', 'acceptance-applicability-check.json')]
    candidates = set(baseline['publication_files'] + future + [n for n in changed + added if n and (ROOT / n).is_file()])
    baseline['publication_files'] = sorted(n for n in candidates if Path(n).suffix.lower() not in {'.png', '.gif', '.mp4', '.webm', '.log'} and n != 'docs/quality/evidence/workbench/repository-check.json')
    save(baseline_path, baseline)
    print(json.dumps({'passed':True,'increment':114,'backend_top_level':top,'backend_records':records,'active':23,'excluded':5}))


if __name__ == '__main__':
    main()
