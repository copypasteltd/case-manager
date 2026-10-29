"""Update bilingual delivery notes only from completed, source-bound checks."""
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/notification-retry'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def prepend(name, text):
    path = ROOT / name
    old = path.read_text(encoding='utf-8')
    if text.splitlines()[0] not in old:
        title, rest = old.split('\n', 1)
        path.write_text(title + '\n\n' + text.rstrip() + '\n\n' + rest.lstrip('\n'), encoding='utf-8', newline='\n')


def preserve_existing_markdown_endings():
    # Current guides predate this increment and use CRLF. Avoid rewriting every
    # unchanged line when adding a small delivery note; leave new files in LF.
    names = subprocess.check_output(['git', 'diff', '--name-only', '-z', '--', '*.md'], cwd=ROOT).decode().split('\0')
    for name in filter(None, names):
        previous = subprocess.check_output(['git', 'show', 'HEAD:' + name], cwd=ROOT)
        if previous.count(b'\n') and previous.count(b'\n') == previous.count(b'\r\n'):
            path = ROOT / name
            path.write_bytes(path.read_bytes().replace(b'\r\n', b'\n').replace(b'\n', b'\r\n'))


def main():
    backend = read(E.parent / 'notification-retry-backend/result.json')
    final = read(E / 'final-source-check.json')
    media = read(E / 'media.json')
    web, mobile, runtime = [read(E / name) for name in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    assert backend['passed'] and final['passed'] and media['passed'] and web['passed'] and mobile['passed'] and runtime['status'] == 'passed'
    tops, records = backend['discovered_top_level_tests'], len(backend['checks'])
    nw, nm, nr = web['tests']['numPassedTests'], mobile['tests']['pass'], runtime['tests']['pass']
    cached = read(E / 'android-test-scope.json')['gradle_cached']
    shots = sum(len(read(E.parent / ('notification-retry-' + client) / 'capture.json')['screenshots']) for client in ('web', 'mobile'))
    zh = '增量110已接通四端站内通知失败的人工重试：独立组织授权、近期MFA、固定原事件、明确确认、不可变父子任务和可追踪结果。原失败不重置，实际Worker重查当前接收人，等待中详情自动刷新。更多工作台聚合、提醒策略、实际复制故障及跨渠道送达仍属GAP-08；完整23项（14 P0／9 P1）和5项移出保持不变。AI仅移出费用控制，秘书、八项Skill、TG、授权、审计、隔离和技术限制继续保留。'
    en = 'Increment 110 adds controlled human retries for failed in-app notifications across Web and the shared WeChat, Douyin and Android pages. Separate organization authority, recent MFA, a fixed original event, explicit confirmation and immutable parent/child receipts preserve the failure history. The real worker checks current recipients; pending details refresh automatically. Dashboard aggregation, reminder configuration, actual replication failures and cross-channel delivery remain open under GAP-08. All 23 packages (14 P0/9 P1) and 5 exclusions remain. Only AI cost control is excluded; secretary, eight Skills, TG, authorization, audit, isolation and technical limits remain required.'
    validation = f'当前源码检查：后端{tops}个顶层测试／{records}条结果，Web{nw}、共享移动{nm}、运行时{nr}；八项客户端命令全部重新执行通过，微信／抖音各20页。Android开发APK通过18＋4项静态检查，26条JVM结果来自' + ('Gradle缓存' if cached else '本轮执行') + f'。Web与Taro H5各5组实际浏览器／API／租约Worker操作通过；新增{shots} PNG、2段静音MP4、2个GIF，原222份媒体字节保留。另有2个Linux交叉构建及本地普通schema84→85升级；不声明真机、真实云／模型／TG或生产验收。'
    validation_en = f'Current-source checks: {tops} backend top-level tests / {records} records, {nw} Web, {nm} shared mobile and {nr} runtime tests. All eight client commands reran and passed; both miniapps retain 20 pages. A development APK passes 18 + 4 static checks; its 26 JVM records came from ' + ('Gradle cache' if cached else 'this execution') + f'. Web and Taro H5 each pass 5 actual browser/API/leased-worker check groups. New media: {shots} PNGs, 2 silent MP4s and 2 GIFs; all 222 historical media retain exact bytes. Two Linux cross-builds and a local schema84-to85 upgrade also pass. No device, live cloud/model/TG or production acceptance is claimed.'
    links = '[设计与使用 / Design and usage](docs/development/110-notification-retry.md) · [验证与来源 / Verification](docs/quality/110-notification-retry-verification.md) · [全部待办 / All remaining work](docs/delivery/03-goal-gap-assessment.md)'
    prepend('README.md', zh + '\n\n' + en + '\n\n' + links + '\n\n' + validation + '\n\n' + validation_en + '\n\n![通知异常与受控重试 / Notification recovery](docs/media/gifs/notification-retry-web.gif)')
    for name, text in [('docs/README.zh-CN.md', zh + '\n\n' + validation), ('docs/README.en.md', en + '\n\n' + validation_en), ('docs/README.md', zh + '\n\n' + en + '\n\n' + validation + '\n\n' + validation_en)]:
        prepend(name, text + '\n\n' + links.replace('(docs/', '('))
    prepend('docs/CHANGELOG.md', '## 增量110 · 通知失败人工重试 / Controlled notification recovery · 2026-10-29\n\n- ' + zh + '\n- ' + en + '\n- ' + validation + '\n- ' + validation_en + '\n- [设计与使用](development/110-notification-retry.md) · [验证及早期尝试](quality/110-notification-retry-verification.md)。')
    table = '| 客户端 / Client | 原失败 / Failure | 实际结果 / Result | 历史 / History | 录屏 / Recording | GIF |\n|---|---|---|---|---|---|\n'
    for client in ('web', 'mobile'):
        prefix = 'notification-retry-' + client
        table += '| ' + ('Web' if client == 'web' else 'Taro H5') + ' | ' + ' | '.join('[PNG](screenshots/' + prefix + '-' + suffix + '.png)' for suffix in ('failure', 'result', 'history')) + ' | [MP4](videos/' + prefix + '.mp4) | [GIF](gifs/' + prefix + '.gif) |\n'
    prepend('docs/media/README.md', f'增量110 / Increment110：{shots} PNG、2静音MP4、2 GIF来自认证后的隔离合成业务和实际浏览器操作；不录制密码或MFA输入。原222份媒体保留原始字节与来源。\n\n' + table + '\n[验证与来源 / Verification](../quality/110-notification-retry-verification.md)。')
    scope_path = ROOT / 'docs/delivery/04-current-scope.json'
    scope = read(scope_path)
    gap = next(v for v in scope['items'] if v['id'] == 'GAP-08')
    old_remaining = gap['remaining']
    remaining = '补更多业务聚合、实际存储复制失败来源、合同提醒策略配置及跨渠道送达回执；站内失败的受控人工重试已完成，真实集成与整体功能验收继续开放。'
    gap['remaining'] = remaining
    gap['completed_adaptations'] = sorted(set(gap.get('completed_adaptations', []) + ['NOTIFICATION-RETRY-110']))
    gap['completed_detail'] = '增量110实现站内失败技术详情、独立重试角色、近期MFA、固定revision、不可变父子回执、同事务审计、真实Worker接收人重查及四端页面；不代表TG／云或整个GAP-08通过。'
    save(scope_path, scope)
    path = ROOT / 'docs/delivery/03-goal-gap-assessment.md'
    if old_remaining != remaining:
        text = path.read_text(encoding='utf-8')
        assert old_remaining in text
        path.write_text(text.replace(old_remaining, remaining), encoding='utf-8', newline='\n')
    note = '增量110：GAP-08的站内通知失败人工重试已接通四端页面、真实API、PG不可变依据和租约Worker；更多工作台聚合、提醒策略、实际复制故障及跨渠道送达继续开放。仍为23 active（14 P0／9 P1）／5 excluded，子功能完成不等于整个工作包验收。见[设计与使用](../development/110-notification-retry.md)及[验证](../quality/110-notification-retry-verification.md)。'
    for name in ('docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md', 'docs/delivery/04-current-scope-and-acceptance.md'):
        prepend(name, note)
    path = ROOT / 'docs/requirements/02-identity-and-permissions.md'
    authority = '\n## 通知失败人工重试 / Human notification recovery · Increment110\n\n组织级`notification.delivery.read`只读技术元数据，`notification.delivery.retry`需独立明确授予；现有只读角色不自动升级。人工重试只接受有效人类交互会话及最近10分钟MFA，固定原始业务事件和准确任务revision，追加不可变子任务、理由、审计和Outbox。运维权不授予业务正文权限；实际Worker独立重查当前接收人。Agent运行和TG执行会话不得使用人工运维入口。见[使用与状态约束](../development/110-notification-retry.md)。\n\nRead authority does not imply retry or business-content authority. Retries require explicit human confirmation and recent MFA. Fixed source input, immutable evidence and current recipient access remain mandatory; creating a child does not establish delivery or employee read status.\n'
    if 'Human notification recovery · Increment110' not in path.read_text(encoding='utf-8'):
        path.write_text(path.read_text(encoding='utf-8').rstrip() + '\n' + authority, encoding='utf-8', newline='\n')
    rule = '- Increment110 adds controlled in-app notification retries across four clients. Keep explicit organization read/retry authority, human sessions, recent MFA, fixed input, immutable parent/child evidence and current-recipient checks. Dashboard aggregation, reminder configuration, actual storage failures and cross-channel delivery remain open under GAP-08. See `docs/development/110-notification-retry.md`.'
    path = ROOT / 'AGENTS.md'
    if rule not in path.read_text(encoding='utf-8'):
        path.write_text(path.read_text(encoding='utf-8').rstrip() + '\n\n' + rule + '\n', encoding='utf-8', newline='\n')
    report = f'''# 通知失败人工重试验证 / Notification recovery verification

增量110完成GAP-08的站内人工恢复子功能；完整23项工作包继续开放。AI仅移出费用控制。[设计、权限、使用与命令](../development/110-notification-retry.md)。

{validation}

{validation_en}

| 编号 | 核验 | 结果和边界 |
|---|---|---|
| NTR-110-01 | 全量原生后端 | {tops}个顶层／{records}条结果，全部发现项执行、无skip。[结果](evidence/notification-retry-backend/result.json)／[原始日志摘要](evidence/notification-retry-backend/raw-log-archive.json)。按单个批次进程顺序执行；未修改PG耐久设置、断言或清理期限。 |
| NTR-110-02 | 人工恢复业务 | 新增1个顶层、14个子场景：独立读写权、正文404、MFA、严格字段、准确revision、幂等及并发唯一子任务、原失败不变、叶节点异常列表、真实Worker／去重、游标绑定、四渠道读取、拒绝Agent／TG执行会话、DB固定输入／证据、撤权接收人、过时事件、自动退避不绕过、审计失败回滚和不可变历史。 |
| NTR-110-03 | 客户端 | [八项最终命令](evidence/notification-retry/client-checks.json)全部重新执行；Web{nw}、移动{nm}、运行时{nr}。新增Web10／移动8项契约和来源检查；不以静态断言代替真实业务验证。 |
| NTR-110-04 | 微信／抖音 | 两端各20页完成构建；共享页接入重试和结果。Taro H5与四渠道fixture读取不等于真实AppID登录或正式发布。 |
| NTR-110-05 | Android | 原生Metro＋Gradle开发APK；18项包检查＋4项功能内容检查。26条JVM为{'缓存' if cached else '新执行'}，保留实际任务状态与4个XML。无真机／模拟器。 |
| NTR-110-06 | 真实页面操作 | [Web](evidence/notification-retry-web/result.json)与[Taro H5](evidence/notification-retry-mobile/result.json)各5组；独立人类登录／MFA、明确提交、原失败不变、实际租约Worker站内写入、等待中详情刷新、历史追溯和页面宽度。无响应替换。自建浏览器、服务及隔离DB均关闭清理。 |
| NTR-110-07 | 媒体来源 | {shots} PNG、2段静音MP4、2个GIF，来自认证后的合成页面像素；已[目视核对](evidence/notification-retry/visual-review.json)7张最终截图及两个GIF的首／中／末帧。GIF变速以[编码记录](evidence/notification-retry/media.json)实际倍率为准；未包含密码或MFA输入。原222份媒体按[原日期来源](evidence/notification-retry/reused-evidence.json)字节保留。 |
| NTR-110-08 | 普通持久升级 | 本地schema84→85，历史费用表行数及内容摘要不变；新增重试表、独立角色和3项触发约束。只替换核对身份的自有API及2个Worker；临时测试库0，外部接入仍关闭。不是生产部署。 |
| NTR-110-09 | 契约与源码 | 原320路径／698模型逐项语义保留；现323路径／398操作／705模型。{len(final['checks'])}组最终来源核对；2个Linux amd64 ELF交叉构建，未执行Linux隔离边界。 |

投递技术回执不展示业务正文、自由原因、payload或会话秘密；组织运维权不授予项目内容权。创建成功只表示新任务入队。只有实际Worker结果表示站内写入或略过；不表示员工已读或TG外发。原失败从叶节点列表消失也不表示投递成功。

早期尝试均保留在[attempts](evidence/notification-retry/attempts/)：首次原生专项发现全新Bootstrap缺少新角色，已补全而未放宽权限；第二次测试将现有400 INVALID_CURSOR错误期望为422，已按既有协议修正但仍要求拒绝。首次客户端类型检查发现composite工程缺少显式共享文件入口，已修正配置。最终失败叶节点列表与等待中刷新分别增加后，旧专项／客户端通过记录保留各自原源码与日志；本轮最终全量后端及全部八项客户端命令以新源码重新执行，不把旧通过记录当作最终来源。

首次H5浏览器准备在固定存活的业务fixture内执行构建，耗尽该存活时间，fixture退出后浏览器因描述文件不存在而未启动。14个业务子场景虽通过，整轮仍记为失败。保留原生／构建／浏览器日志及准确源码，未延长Go fixture期限或清理期限。修正为先构建H5，再启动隔离业务，通过仅限本机的临时HTTP转发连接实际API；不替换响应。Web和H5均按最终辅助脚本重新核验采集，前一次Web成功只作为原日期历史记录保留。

随后Web五组实际页面操作已通过，但最终原生测试库DROP超过独立两分钟期限；同一时段全量后端的一批也出现相同清理超时。仅将失败的顶层集成测试组在原PG上串行重跑，清理仍然超时；原生PG日志记录到与失败窗口重叠的132.018秒checkpoint，其中同步128.318秒，超过原120秒清理期限。保留准确两行日志的[诊断来源](evidence/notification-retry/native-cleanup-diagnostic.json)，不将其描述为网络或容量测试。三次相关完整尝试继续保留为失败。

当前Go／SQL／契约和业务断言保持不变，仅将该失败顶层组在C盘临时目录的另一套原生Windows PostgreSQL18.6上重跑，得到11条通过结果；其fsync、synchronous_commit、full_page_writes全部开启，原fixture／清理期限均未修改。首次initdb因非ASCII程序路径失败，服务尚未启动；保留原错误及源码后沿用仓库现有ASCII路径适配器，不修改PG设置。最终后端汇总保留228个原成功顶层组及该1个重新执行的顶层组，12批原始来源通过13段日志精确绑定；同一批中原成功组不重复执行。最终Web与H5在后端结束后依次使用该原生临时PG运行，完成后关闭临时PG，库内无遗留合成测试库。

原开发PG的2个已标记无效、无活动连接的遗留合成测试库，分别按准确名称、失败时间窗口和归属核对后额外清理；未使用FORCE、未重启原PG。保留[第一次清理](evidence/notification-retry/fixture-cleanup.json)及[第二次清理](evidence/notification-retry/attempts/10-second-owned-fixture-cleanup/fixture-cleanup.json)的计划和回执。无效库不能连接，不声称读取其库内资料；原开发库及服务保留。

首次按新准备流程执行的H5在定位失败行按钮时超时，未执行任何浏览器业务组；原生14个子场景及隔离库清理均通过，整轮页面检查仍为失败。保留[准确源码、六段原始日志及前一版Web证明](evidence/notification-retry/attempts/11-h5-locator/attempt.json)，修正辅助脚本在行内重复限定页面根的定位方式，改为固定任务ID并断言唯一行。未修改应用业务代码或跳过实际点击；最终H5与Web均以修正后的辅助脚本重新采集。

首次并行的Android打包长时间停在文件发现阶段，未生成本轮业务bundle；归档准确输入和日志后只停止已核对创建时间、命令和PID的自有构建树。该次记为interrupted，不记为通过或JVM失败。最终Android记录来自后续单独执行的构建。

最终APK的18项基础检查通过后，第一次功能内容检查错误地要求包内包含仅服务端使用的`notification.delivery.retry`字符串。客户端实际以服务器核验后的`can_retry`控制重试表单；后端独立组织授权已有真实数据库场景覆盖。保留[失败断言与准确脚本](evidence/notification-retry/attempts/12-android-content-predicate/attempt.json)，修正为核对读入口、详情／历史路由、状态契约及实际`can_retry`界面分支。相同APK通过修正后的4项内容核对；应用源码及APK均未因断言修正而重建。

本轮不重新声明依赖公告查询或依赖安全专项：增量108的完整npm审计0项及56＋2专项保留原执行日期，锁文件、根manifest和workspace依赖字段未变。正式生产产物安全仍在GAP-26。

Real Codex/model/Skill execution, actual OSS/R2 and TG, formal Android signing/release, real miniapp identity/release, production deployment and complete applicable business journeys remain in the [numbered register](../delivery/03-goal-gap-assessment.md). Device verification, network performance/real120GB exercise, historical migration/disaster recovery and AI cost control remain excluded. No local pass is promoted to production readiness.
'''
    (ROOT / 'docs/quality/110-notification-retry-verification.md').write_text(report, encoding='utf-8', newline='\n')
    (E / 'verification.md').write_text('# Increment110 verification\n\nSee [verification and provenance](../../110-notification-retry-verification.md) and [design and usage](../../../development/110-notification-retry.md). Local source-bound checks do not close the full goal or production acceptance.\n', encoding='utf-8')
    path = ROOT / 'docs/quality/current-functional-baseline.json'
    baseline = read(path)
    baseline.update(increment=110, evidence_group='notification-retry', verification_script='verify_notification_retry_sources.py',
        backend_evidence_group='notification-retry-backend', backend_top_level_tests=tops, backend_result_records=records,
        web_evidence_group='notification-retry', web_tests=nw, mobile_tests=nm, runtime_evidence_group='notification-retry', runtime_tests=nr,
        development_verification_script='verify_notification_retry_development.py', interaction_kind='organization_notification_retry',
        interaction_sources='notification_retry_sources.py', retained_evidence_record='docs/quality/evidence/notification-retry/reused-evidence.json',
        new_browser_capture=True, new_backend_test_execution=True, full_goal_complete=False, production_ready=False,
        planning_scope_verification_record='docs/quality/evidence/notification-retry/scope-check.json',
        planning_publication_record='docs/quality/evidence/notification-retry/repository-check.json')
    changed = subprocess.check_output(['git', 'diff', '--name-only', '-z'], cwd=ROOT).decode().split('\0')
    added = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    kinds = {'.md', '.py', '.go', '.sql', '.ts', '.tsx', '.mjs', '.js', '.json', '.xml', '.css', '.html', '.yaml', '.yml', '.ps1'}
    baseline['publication_files'] = sorted(set(baseline['publication_files'] + [name for name in changed + added if name and (ROOT / name).is_file() and Path(name).suffix.lower() in kinds]))
    save(path, baseline)
    preserve_existing_markdown_endings()
    print(json.dumps({'passed': True, 'increment': 110, 'backend_top_level': tops, 'backend_records': records, 'screenshots': shots, 'remaining': 23, 'full_goal_complete': False}))


if __name__ == '__main__':
    main()
