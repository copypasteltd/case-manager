"""Update bilingual guides and the current baseline only from completed actual evidence."""
from pathlib import Path
import json
import subprocess

ROOT=Path(__file__).resolve().parents[2]
E=ROOT/'docs/quality/evidence/contract-reminders'
def read(path):return json.loads(path.read_text(encoding='utf-8'))
def save(path,value):path.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
def prepend(name,body):
    path=ROOT/name;text=path.read_text(encoding='utf-8')
    if body.split('\n')[0] in text:return
    header,rest=text.split('\n',1)
    path.write_text(header+'\n\n'+body.rstrip()+'\n\n'+rest.lstrip('\n'),encoding='utf-8',newline='\n')
def preserve_markdown_endings():
    names=subprocess.check_output(['git','diff','--name-only','-z','--','*.md'],cwd=ROOT).decode().split('\0')
    for name in filter(None,names):
        previous=subprocess.check_output(['git','show','HEAD:'+name],cwd=ROOT)
        if previous.count(b'\n') and previous.count(b'\n')==previous.count(b'\r\n'):
            path=ROOT/name;path.write_bytes(path.read_bytes().replace(b'\r\n',b'\n').replace(b'\n',b'\r\n'))

def main():
    backend=read(E.parent/'contract-reminders-backend/result.json')
    proof,media,web,mobile,runtime,android=[read(E/n) for n in ('final-source-check.json','media.json','web-build.json','mobile-build.json','runtime-build.json','android-test-scope.json')]
    assert backend['passed'] and proof['passed'] and media['passed'] and web['passed'] and mobile['passed'] and runtime['status']=='passed'
    tops,records=backend['discovered_top_level_tests'],len(backend['checks'])
    nw,nm,nr=web['tests']['numPassedTests'],mobile['tests']['pass'],runtime['tests']['pass']
    cached=android['gradle_cached']
    zh='增量111已实现四端合同到期提醒策略：独立读／管理授权、近期MFA、准确版本发布、不可变历史和真实Worker调度。组织策略（包括停用）覆盖部署规则；新策略不补发早于发布业务日的历史阈值，已有通知事实保留。规则修改后须重新确认。GAP-08仍待更多聚合、实际复制故障、跨渠道送达及公司规则选用；全部23项（14 P0／9 P1）与5项移出不变。AI仅移出费用控制，秘书、八项Skill、TG、隔离、授权、审计和技术限制保留。'
    en='Increment 111 adds contract expiry reminder policies across Web and the shared WeChat, Douyin and Android pages: separate read/manage authority, recent MFA, fixed-version publication, immutable history and real worker scheduling. An organization policy, including disable, overrides deployment fallback. New rules do not backfill thresholds before their publication business date; existing notification facts remain. Editing a rule requires confirmation again. GAP-08 still includes wider aggregation, real storage failures, cross-channel delivery and actual company rule adoption. All 23 packages (14 P0/9 P1) and 5 exclusions remain. Only AI fee control is excluded; secretary, eight Skills, TG, isolation, authorization, audit and technical limits remain required.'
    cncheck=f'当前源码核验：后端{tops}个顶层测试／{records}条结果，Web{nw}、共享移动{nm}、运行时{nr}；最终八项客户端命令全部重新执行，微信／抖音各20页。Web与Taro H5各5组真实页面／API检查；新增7 PNG、2静音MP4、2 GIF，原233份媒体保留准确字节。Android开发APK通过18＋4项静态检查，26条JVM来自'+('Gradle缓存' if cached else '本轮执行')+f'。另有本地schema85→86升级、全耐久原生临时PG及2个Linux交叉构建；不声明真机、真实云／模型／TG或生产验收。'
    encheck=f'Current-source verification: {tops} backend top-level tests / {records} records, {nw} Web, {nm} shared mobile and {nr} runtime tests. All eight final client commands reran; each miniapp retains 20 pages. Web and Taro H5 each pass 5 actual page/API groups. New media: 7 PNGs, 2 silent MP4s and 2 GIFs; 233 previous media retain exact bytes. The development APK passes 18 + 4 static checks; 26 JVM results came from '+('Gradle cache' if cached else 'this execution')+'. A local schema 85 to 86 upgrade, a fully durable temporary native PG cluster and 2 Linux cross-builds also pass. No device, live cloud/model/TG or production acceptance is claimed.'
    links='[设计与使用 / Design and usage](docs/development/111-contract-reminder-policies.md) · [验证与来源 / Verification](docs/quality/111-contract-reminder-policies-verification.md) · [全部未完成项 / All remaining work](docs/delivery/03-goal-gap-assessment.md)'
    prepend('README.md',zh+'\n\n'+en+'\n\n'+links+'\n\n'+cncheck+'\n\n'+encheck+'\n\n![合同提醒策略 / Contract reminder policies](docs/media/gifs/contract-reminders-web.gif)')
    usage='操作：管理员明确授予组织级策略角色 → 通知中心查看当前规则 → 填写开关、时区、提前量及原因 → 重新勾选核对声明并完成MFA → 明确发布 → 核对持久版本与历史。未发布组织可能仍使用运维显式配置的旧部署规则，页面会提示这一边界。'
    usage_en='Usage: explicitly grant an organization policy role, open Notifications, review the rule, choose enable/disable, timezone and lead days, enter a reason, confirm the final inputs and complete MFA, then publish and inspect persisted current/history records. An unpublished organization may still use an explicitly configured deployment fallback; the page discloses that boundary.'
    for name,body in [('docs/README.zh-CN.md',zh+'\n\n'+usage+'\n\n'+cncheck),('docs/README.en.md',en+'\n\n'+usage_en+'\n\n'+encheck),('docs/README.md',zh+'\n\n'+en+'\n\n'+cncheck+'\n\n'+encheck)]:
        prepend(name,body+'\n\n[策略设计与使用 / Policy design and usage](development/111-contract-reminder-policies.md) · [验证 / Verification](quality/111-contract-reminder-policies-verification.md)')
    ledger='增量111：GAP-08的版本化合同提醒策略已接通四端、实际API、PG不可变发布、调度边界和Worker。更多聚合、实际复制故障、跨渠道送达与公司规则选用继续开放；23 active（14 P0／9 P1）／5 excluded不变。见[设计](../development/111-contract-reminder-policies.md)及[验证](../quality/111-contract-reminder-policies-verification.md)。'
    for name in ('docs/delivery/03-goal-gap-assessment.md','docs/delivery/04-current-scope-and-acceptance.md'):prepend(name,ledger)
    prepend('docs/CHANGELOG.md','## 增量111 / Increment 111 · 合同提醒策略 / Contract reminder policies\n\n'+zh+'\n\n'+en+'\n\n'+cncheck+'\n\n'+encheck+'\n\n新增迁移86、组织策略读／管理角色、当前策略／发布／签名分页历史接口、发布日边界及调度并发重查。保留原323路径／705模型语义，新计325路径／401操作／711模型。移动默认测试命令接入新10个跨端边界算例；此前未选中它们的八项命令通过记录单独保留，不能替代最终299项移动结果。\n\n[实现](development/111-contract-reminder-policies.md) · [原始验证与尝试](quality/111-contract-reminder-policies-verification.md)')
    prepend('docs/media/README.md','增量111媒体来自真实认证后的合成Web／Taro H5页面；新增7张PNG、2段静音MP4和2个GIF，原233份媒体准确保留。\n\n| 端 | GIF | 录屏 | 截图 |\n|---|---|---|---|\n| Web | [策略发布](gifs/contract-reminders-web.gif) | [静音MP4](videos/contract-reminders-web.mp4) | [核对](screenshots/contract-reminders-web-review.png)、[已发布](screenshots/contract-reminders-web-published.png)、[历史](screenshots/contract-reminders-web-history.png)、[深色](screenshots/contract-reminders-web-dark.png) |\n| 共享移动 | [策略发布](gifs/contract-reminders-mobile.gif) | [静音MP4](videos/contract-reminders-mobile.mp4) | [核对](screenshots/contract-reminders-mobile-review.png)、[已发布](screenshots/contract-reminders-mobile-published.png)、[历史](screenshots/contract-reminders-mobile-history.png) |\n\n真实像素、逐帧来源与实际GIF倍率见[媒体记录](../quality/evidence/contract-reminders/media.json)。不录制登录、密码、MFA秘密或真实业务资料；无真机、模拟器或生产接入声明。')
    prepend('docs/contributing.md','增量111的合同提醒策略采用原生PG、真实HTTP、签名合同及租约Worker检查。运行入口、发布日边界、旧部署兼容和失败尝试来源见[设计](development/111-contract-reminder-policies.md)及[验证](quality/111-contract-reminder-policies-verification.md)。最新汇总脚本为`app/scripts/verify_contract_reminders_sources.py`；默认移动测试须继续包含提醒策略算例，不能用未选中新算例的旧结果替代当前验证。')
    prepend('docs/requirements/02-identity-and-permissions.md','实现补充（增量111）：合同提醒策略采用独立组织级`notification.policy.read`／`notification.policy.manage`，写操作要求近期MFA和准确版本；新角色不自动分配，Agent／TG执行会话不得使用人工发布入口。见[策略权限与调度边界](../development/111-contract-reminder-policies.md)。')
    report=f'''# 合同提醒策略验证 / Contract reminder policy verification

增量111完成GAP-08的策略配置子功能；整个工作包及完整Goal继续开放。[设计、操作、协议和检查命令](../development/111-contract-reminder-policies.md)。

{cncheck}

{encheck}

| 编号 | 核验 | 实际结果与边界 |
|---|---|---|
| CRP-V01 | 完整原生后端 | {tops}个顶层／{records}条结果，全部发现项执行，无skip；[结果](evidence/contract-reminders-backend/result.json)和[准确原始日志摘要](evidence/contract-reminders-backend/raw-log-archive.json)。另一个全耐久原生PG承担隔离测试，不修改原开发PG、fixture存活或清理期限。 |
| CRP-V02 | 新策略业务 | 12个真实API／PG子场景：角色独立、明确发布、严格字段、时区／提前量、MFA、幂等、并发冲突、不可变历史、游标绑定、撤权重放、数据库与审计回滚；合同经过创建、独立审批、固定文件签署和生效，再由实际调度及租约Worker写入责任人通知。 |
| CRP-V03 | 发布边界 | 实际验证组织规则覆盖UTC／365天旧部署输入；7天合同准确关联策略；发布前阈值的旧合同不入队；重复扫描不重复；停用覆盖旧部署且保留既有送达事实。 |
| CRP-V04 | 最终客户端 | [八项命令](evidence/contract-reminders/client-checks.json)全部重新执行；Web{nw}、移动{nm}、运行时{nr}。Web及移动各执行同一组10个共享边界算例，不能解释为20个不同业务场景。 |
| CRP-V05 | 微信／抖音 | 实际构建各20页；共享提醒页与类型契约接入，不代表真实AppID身份或正式发布。 |
| CRP-V06 | 实际页面 | [Web](evidence/contract-reminders-web/result.json)和[Taro H5](evidence/contract-reminders-mobile/result.json)各5组：独立登录、MFA、无自动提交、重复提前量拒绝、编辑后重新确认、真实停用发布、持久读取、旧版本冲突、不可变历史与页面宽度。无响应替换。 |
| CRP-V07 | Android | 本轮原生Metro／Gradle开发APK，18项包检查＋4项策略内容检查；26条JVM{'来自缓存' if cached else '本轮执行'}，保留4个原XML和实际Gradle任务状态。不使用设备或模拟器。 |
| CRP-V08 | 媒体 | 新增7 PNG、2静音MP4、2 GIF，来自认证后真实合成页面像素；准确帧、实际速度与编码来源见[媒体证明](evidence/contract-reminders/media.json)，目视核对记录见[visual-review](evidence/contract-reminders/visual-review.json)。全部233份旧媒体保留原字节、执行日期和原适用范围。 |
| CRP-V09 | 持久升级 | 原开发实例普通schema85→86升级；旧费用记录行数及摘要不变；策略表仅追加、4项新触发器与两个独立角色；只替换已核对归属的自有API／2个Worker。测试库0，外部接入保持关闭，不声明生产部署。 |
| CRP-V10 | API与产物 | 原323路径／705模型逐项语义不变，现325路径／401操作／711模型。2个当前源码Linux amd64 ELF交叉构建，未在Linux运行或部署。 |

所有浏览器上下文、页面、临时服务、转发端口和测试数据库完成后关闭；独立临时PG在完整阶段结束后关闭，私有数据不入Git。原开发PG及不属本轮的进程和页面保留。

早期尝试分别保留在[attempts](evidence/contract-reminders/attempts/)：首次账号测试把已有MFA员工再次送入首次注册辅助流程，得到CHALLENGE_INVALID；修正为独立只读员工的真实四渠道会话，没有放宽MFA。第二次撤权测试正确拒绝重放，但恢复夹具时误插重复绑定，触发唯一约束；改为恢复准确原绑定的有效期。二者仍记为失败，准确源码ZIP和原始日志保留在忽略目录，由公开摘要绑定。完整最终后端重新执行全部发现项。

第一次全量发现辅助器未请求Go JSON，解析不到测试，因此在执行任何批次前拒绝空集合；保留原辅助器与失败说明后补上JSON参数。未把零测试记为通过。第一次八项客户端命令虽通过，但默认移动命令的显式清单遗漏新10个策略算例；该整轮和准确输入／八段日志已保留为范围不完整。接入默认清单并增加编辑后重新确认行为后，最终八项客户端命令全部重跑，以299项共享移动结果为准。

实际移动页面还发现一项反馈问题：重复提前量已被客户端拒绝，但通用未知错误渲染器掩盖了具体输入提示。该失败页面、准确源码ZIP、原始日志和修正前通过的八项客户端命令均保留在attempt05，仍记为页面检查失败。策略表单现把本地输入错误转换为受控移动错误，员工能够看到应修改的具体内容；修正后再次执行全部八项命令及实际页面流程，最终记录绑定当前源码。

首次Web页面的四组业务检查通过，末尾宽度辅助器误把Page对象用于DOM节点回调而失败；attempt06保留该完整失败、当时通过的移动检查、7张截图、准确源码及私有原始帧／日志。修正为实际HTML节点定位，并把历史截图滚动到历史区域；应用实现未变更。两端均在最终公共辅助器版本重新检查、重新采集，旧移动通过记录没有冒充最终版本证明。

本轮未重新查询依赖公告，也不重新执行依赖安全专项；增量108完整npm审计0项和既有安全专项保留实际原日期。根manifest、锁文件、contracts manifest及全部workspace依赖字段不变，移动manifest仅在测试命令追加新算例。正式生产依赖与产物安全验收仍属GAP-26。

Actual company approver/policy adoption, real model/Codex/Skill execution, Linux isolation acceptance, actual OSS/R2 and Telegram, signed Android release, real miniapp identity/release, production deployment and complete applicable business journeys remain in the [numbered register](../delivery/03-goal-gap-assessment.md). Device checks, network performance/real120GB exercises, historical migration/DR and AI fee controls remain excluded. Local evidence does not establish production readiness.
'''
    (ROOT/'docs/quality/111-contract-reminder-policies-verification.md').write_text(report,encoding='utf-8',newline='\n')
    (E/'verification.md').write_text('# Increment 111 verification\n\nSee [verification and provenance](../../111-contract-reminder-policies-verification.md) and [design and usage](../../../development/111-contract-reminder-policies.md). Local source-bound evidence does not complete the full goal or production acceptance.\n',encoding='utf-8')
    path=ROOT/'docs/quality/current-functional-baseline.json';baseline=read(path)
    baseline.update(increment=111,evidence_group='contract-reminders',mobile_tests=nm,verification_script='verify_contract_reminders_sources.py',
        backend_evidence_group='contract-reminders-backend',backend_top_level_tests=tops,backend_result_records=records,
        web_evidence_group='contract-reminders',web_tests=nw,runtime_evidence_group='contract-reminders',runtime_tests=nr,
        development_verification_script='verify_contract_reminders_development.py',interaction_kind='organization_contract_reminder_policy',
        interaction_sources='contract_reminders_sources.py',retained_evidence_record='docs/quality/evidence/contract-reminders/reused-evidence.json',
        new_browser_capture=True,new_backend_test_execution=True,full_goal_complete=False,production_ready=False,
        planning_scope_verification_record='docs/quality/evidence/contract-reminders/scope-check.json',
        planning_publication_record='docs/quality/evidence/contract-reminders/repository-check.json')
    changed=subprocess.check_output(['git','diff','--name-only','-z'],cwd=ROOT).decode().split('\0')
    added=subprocess.check_output(['git','ls-files','--others','--exclude-standard','-z'],cwd=ROOT).decode().split('\0')
    kinds={'.md','.py','.go','.sql','.ts','.tsx','.mjs','.js','.json','.xml','.css','.html','.yaml','.yml','.ps1'}
    baseline['publication_files']=sorted(set(baseline['publication_files']+[name for name in changed+added if name and (ROOT/name).is_file() and Path(name).suffix.lower() in kinds]))
    save(path,baseline);preserve_markdown_endings()
    print(json.dumps({'passed':True,'increment':111,'backend_top_level':tops,'backend_records':records,'remaining':23,'full_goal_complete':False}))
if __name__=='__main__':main()
