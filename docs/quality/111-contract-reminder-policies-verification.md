# 合同提醒策略验证 / Contract reminder policy verification

增量111完成GAP-08的策略配置子功能；整个工作包及完整Goal继续开放。[设计、操作、协议和检查命令](../development/111-contract-reminder-policies.md)。

当前源码核验：后端230个顶层测试／1396条结果，Web205、共享移动299、运行时55；最终八项客户端命令全部重新执行，微信／抖音各20页。Web与Taro H5各5组真实页面／API检查；新增7 PNG、2静音MP4、2 GIF，原233份媒体保留准确字节。Android开发APK通过18＋4项静态检查，26条JVM来自Gradle缓存。另有本地schema85→86升级、全耐久原生临时PG及2个Linux交叉构建；不声明真机、真实云／模型／TG或生产验收。

Current-source verification: 230 backend top-level tests / 1396 records, 205 Web, 299 shared mobile and 55 runtime tests. All eight final client commands reran; each miniapp retains 20 pages. Web and Taro H5 each pass 5 actual page/API groups. New media: 7 PNGs, 2 silent MP4s and 2 GIFs; 233 previous media retain exact bytes. The development APK passes 18 + 4 static checks; 26 JVM results came from Gradle cache. A local schema 85 to 86 upgrade, a fully durable temporary native PG cluster and 2 Linux cross-builds also pass. No device, live cloud/model/TG or production acceptance is claimed.

| 编号 | 核验 | 实际结果与边界 |
|---|---|---|
| CRP-V01 | 完整原生后端 | 230个顶层／1396条结果，全部发现项执行，无skip；[结果](evidence/contract-reminders-backend/result.json)和[准确原始日志摘要](evidence/contract-reminders-backend/raw-log-archive.json)。另一个全耐久原生PG承担隔离测试，不修改原开发PG、fixture存活或清理期限。 |
| CRP-V02 | 新策略业务 | 12个真实API／PG子场景：角色独立、明确发布、严格字段、时区／提前量、MFA、幂等、并发冲突、不可变历史、游标绑定、撤权重放、数据库与审计回滚；合同经过创建、独立审批、固定文件签署和生效，再由实际调度及租约Worker写入责任人通知。 |
| CRP-V03 | 发布边界 | 实际验证组织规则覆盖UTC／365天旧部署输入；7天合同准确关联策略；发布前阈值的旧合同不入队；重复扫描不重复；停用覆盖旧部署且保留既有送达事实。 |
| CRP-V04 | 最终客户端 | [八项命令](evidence/contract-reminders/client-checks.json)全部重新执行；Web205、移动299、运行时55。Web及移动各执行同一组10个共享边界算例，不能解释为20个不同业务场景。 |
| CRP-V05 | 微信／抖音 | 实际构建各20页；共享提醒页与类型契约接入，不代表真实AppID身份或正式发布。 |
| CRP-V06 | 实际页面 | [Web](evidence/contract-reminders-web/result.json)和[Taro H5](evidence/contract-reminders-mobile/result.json)各5组：独立登录、MFA、无自动提交、重复提前量拒绝、编辑后重新确认、真实停用发布、持久读取、旧版本冲突、不可变历史与页面宽度。无响应替换。 |
| CRP-V07 | Android | 本轮原生Metro／Gradle开发APK，18项包检查＋4项策略内容检查；26条JVM来自缓存，保留4个原XML和实际Gradle任务状态。不使用设备或模拟器。 |
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
