# 合同到期提醒策略 / Contract expiry reminder policies

增量111完成GAP-08中的合同提醒策略配置子功能：组织管理员明确授权经办人，经办人以近期MFA发布不可变策略，通知Worker按当前组织规则调度。Web、微信、抖音和Android使用同一后端契约；移动端沿用现有通知页面，不新增路由。更广的工作台聚合、实际存储复制故障来源与跨渠道送达仍在GAP-08中开放。

Increment111 implements explicit organization contract reminder policies. An independently authorized human publishes a versioned rule; the real notification scheduler uses that rule, including an explicit disable. Web and the shared WeChat, Douyin and Android page use the same contract. This is a completed subfeature, not completion of the whole notification package or production acceptance. Only AI fee controls are excluded; secretary, eight Skills, TG, authorization, isolation, audit and technical limits remain required.

## 使用 / Usage

1. 在“成员与授权”把“合同提醒策略管理人”按组织范围授予经办人。仅需查询的成员使用“合同提醒策略查阅人”。新角色不会自动绑定给任何员工，原通知运维和重试角色也不会自动获得策略管理权。
2. 打开“通知中心 → 合同到期提醒策略”，核对当前已发布版本和发布历史。没有组织策略时，页面明确提示Worker可能仍使用运维明确配置的部署规则；页面不会假定整个提醒系统已停用。
3. 点击“修改提醒策略”，明确选择启用或停用，填写业务时区，例如`Asia/Hong_Kong`。启用时填写1至12个不重复的0至365天整数；`0`表示合同到期当日。例子`30, 7, 0`只是输入示例，不是自动采用的公司制度。
4. 填写变更原因，勾选范围核对声明，完成近期MFA，再点击“确认发布提醒策略”。打开表单、填写字段和身份验证都不会自动发布。
5. 核对新版本、开关、提前量、业务时区、发布业务日、时间及经办人。页面重新读取数据库当前状态和不可变历史。若权限、版本或请求结果不确定，先刷新策略并核对历史，再决定下一步；不要凭旧页面覆盖并发变更。

Grant the organization-scoped Policy Manager role explicitly, or the Reader role for inspection. Open Notifications, review the published policy, choose enable/disable and an explicit timezone, and enter thresholds and a reason. Confirm the organizational scope and provide recent MFA before publishing. Review the persisted current version and history. No company reminder policy is silently selected for the user.

修改开关、时区、提前量或变更原因后，核对勾选会清除，必须按最后的表单内容重新确认；近期身份验证仍独立检查。Changing any policy input clears the confirmation checkbox; the final inputs must be confirmed again. Recent identity verification is checked separately.

重复提前量、无效时区和不完整规则等本地输入错误会显示具体修改提示，保留已填内容且不提交请求；移动端把这类输入错误显式适配为受控错误，不使用通用读取失败提示。Local input errors retain the form and show an actionable message before any publication request; the mobile form explicitly adapts validation errors to its controlled error renderer.

停用允许保留已选提前量，也允许明确提交空列表。停用仍须明确业务时区和变更原因。已有规则可以重新发布一个启用版本，但历史版本不允许编辑、删除或直接回滚覆盖；采用旧规则时也是一次新的人工发布。

Disabling may retain thresholds or explicitly store an empty list. A timezone and reason remain mandatory. Reusing an older choice means publishing a new version; old versions are never rewritten or deleted.

## 调度行为 / Scheduling semantics

| 编号 | 行为 | 确定规则 |
|---|---|---|
| CRP-01 | 策略优先级 | 一旦存在组织发布记录，最新数据库版本优先于部署环境变量；停用版本同样优先。 |
| CRP-02 | 未发布组织 | 保留现有部署兼容：只有Worker显式提供组织、时区和提前量才使用旧规则。未配置环境变量不会启用默认策略。 |
| CRP-03 | 候选合同 | 当前生效的合同、当前生效修订及明确到期日；合同当前责任人及其当前访问权仍由通知服务重查。 |
| CRP-04 | 发布边界 | 新策略只考虑计划提醒日期不早于该策略发布业务日的提前量。发布不会补发更早的历史阈值。 |
| CRP-05 | 调度延迟 | 若Worker暂时停顿，发布边界内已经到达的阈值可在后续轮次处理；多个已到达阈值只选择距离到期最近的一项。 |
| CRP-06 | 去重 | 同组织、合同、生效修订、到期日及提前量唯一；改策略或重复扫描不会再次生成同一提醒事实。 |
| CRP-07 | 停用后已有事实 | 阻止新的调度，不删除已入队事件、失败记录、送达回执或既有站内信。队列继续按其固定业务事实和当前接收人权限处理。 |
| CRP-08 | 并发 | 发布取得组织排他锁；逐合同提交取得共享锁并重新读取当前策略。旧策略或旧部署候选不能在新策略发布后提交。 |
| CRP-09 | 技术上限 | 每组织每次至多100个候选合同；组织遍历每页至多100个，并保留下一组织游标，末页后循环。Worker每分钟扫描，延迟不构成实时送达保证。 |

Policies take effect for subsequent scheduling after commit. Their activation boundary is a business date in the chosen timezone, computed by PostgreSQL from the publication timestamp. New rules do not backfill thresholds before that date. A delayed worker can process an eligible threshold reached after publication; it selects only the nearest reached threshold. Deduplication retains the original contract/revision/date/threshold key across policy changes. Existing queued events and deliveries remain facts; a disable does not recall them.

The scheduler paginates organizations and bounds each contract scan. Every contract transaction checks the latest policy under the same organization lock that publication updates exclusively. A stale candidate cannot commit after a newer publication. PostgreSQL also rejects a due event whose policy, version, timezone, threshold or activation boundary does not match the current enabled policy.

发布回执说明规则已入库，不表示Worker已扫描，也不表示站内信已写入、员工已读或TG已经发送。通知异常继续使用增量110的技术详情与受控重试；本轮不改变接收人权限、失败重试或外部渠道协议。

A publication receipt confirms a committed policy. It does not claim a scheduler scan, inbox write, employee read or Telegram send. Notification failure handling continues to use the controlled retry feature from increment110.

运维兼容说明：现有Worker启动日志字段`contract_reminders_enabled`记录部署级环境变量是否配置，不能用它判断所有组织策略的当前开关；数据库策略应从上述当前策略接口核对。The retained startup flag describes deployment fallback only; inspect the organization policy endpoint to determine its published state.

## 权限 / Authority

| 操作 | 必须条件 |
|---|---|
| 读取当前策略、发布历史 | 当前人类交互会话；组织级`notification.policy.read` |
| 发布启用或停用版本 | 同时具备组织级`notification.policy.read`和`notification.policy.manage`；最近10分钟MFA；准确`expected_version`；有效规则和原因 |
| 生成提醒事实 | 原生通知Worker内部调度；当前组织策略、当前生效合同、当前修订、激活边界及去重键；不继承经办人的合同正文权限 |
| 投递与读取通知 | 既有租约／fencing、业务事件固定输入、当前责任人与当前资料权限 |

组织策略权限允许读取显式组织规则和变更原因，不授予合同、附件或项目正文权限。Agent运行会话和TG执行会话不能使用人工策略接口。幂等重放先检查当前组织、账号、会话、角色和近期MFA；撤权后的原请求不会绕过授权。

Policy authority is separate from contract-content authority. The policy endpoints reject runtime and TG execution sessions. Idempotent replay still checks current authority and recent MFA before returning the original receipt. Stale versions and competing writers cannot overwrite another publication.

## API与持久化 / API and persistence

沿用现有`/v1`、JSON信封、CSRF、`Idempotency-Key`、错误协议和签名分页。

| 方法 | 路径 | 固定内容 |
|---|---|---|
| GET | `/notification-contract-policy` | `current`为最新不可变记录或`null`；`can_publish`为当前明确管理权。 |
| POST | `/notification-contract-policy` | 只接受`expected_version`、`enabled`、`timezone`、`days_before`、`reason`；201返回完整已发布记录。 |
| GET | `/notification-contract-policy-history` | 只接受`limit`、`cursor`；默认20条，最多50条；签名键集分页绑定组织、人员、渠道和授权版本。 |

启用须有1至12个不重复的0至365天提前量；停用可为空。服务端按升序存储，客户端也构造排序后的固定输入。`expected_version=0`表示首次发布，其后须等于最新版本。未知字段、重复查询字段、无效编码、错误时区、缺失列表及无效版本均拒绝。策略详情和发布命令不接受查询字段。

迁移86增加`notification_contract_policies`，以`(org_id,version)`唯一并按连续版本追加。数据库约束固定组织、经办人、时区、提前量、发布业务日和时间；禁止修改／删除；延迟触发器要求同事务审计和Outbox。`notification_due_events.policy_id`按组织外键关联准确策略；旧部署记录保留空值，新策略记录必须匹配当前已启用版本。应用账号只获得策略表的SELECT／INSERT。

Migration86 adds immutable, consecutively versioned organization policy rows, atomic audit/outbox guards and an organization-scoped policy reference on due events. The application role has only SELECT/INSERT on policy history. Existing deployments can perform a normal schema85-to86 upgrade; this is not a historical data migration or disaster recovery task.

## 开发与验证入口 / Development and verification

| 层次 | 实现 |
|---|---|
| 数据库 | `app/backend/migrations/0086_contract_reminder_policies.sql` |
| 规则、发布和历史 | `app/backend/internal/notification/contract_policies.go` |
| 逐合同与分页调度 | `app/backend/internal/notification/scheduler.go`及`cmd/platform/notification_worker.go` |
| HTTP与契约 | `internal/httpapi/notifications.go`、`app/scripts/build_openapi.py`及生成的两份OpenAPI／TypeScript契约 |
| Web | `app/web/src/ContractReminders.tsx`，由通知中心接入 |
| 共享移动 | `app/mobile/src/pages/notifications/reminders.tsx`，由已有通知页接入 |
| 跨端校验 | `app/packages/contracts/src/notifications.ts`及共享边界算例 |

```powershell
$env:PYTHONUTF8='1'
$env:PATH=(Join-Path (Get-Location) 'app/.tools/node-v22.23.4-win-x64')+';'+$env:PATH
python app/scripts/contract_reminders_native_cluster.py prepare
python app/scripts/verify_contract_reminders_backend.py
python app/scripts/freeze_contract_reminders_backend.py
python app/scripts/verify_contract_reminders_clients.py
python app/scripts/freeze_contract_reminders_clients.py
python app/scripts/run_contract_reminders_browser.py
python app/scripts/run_contract_reminders_browser.py --mobile
python app/scripts/encode_contract_reminders_media.py
python app/scripts/contract_reminders_native_cluster.py stop
```

以上命令用于新的隔离核验环境。辅助器发现已有结果或自有夹具时会拒绝覆盖；重新验证前须先核对并归档准确输入、原始日志及旧结果，再准备新环境。已关闭的临时集群数据仍在本地私有临时目录中保留，不应据此重启或改写原开发实例。The commands intentionally refuse to overwrite existing evidence or fixtures. Inspect and archive exact inputs, logs and results before preparing a fresh owned verification context; retained private data does not authorize changing another development instance.

测试使用自有、独立、全耐久的原生Windows PostgreSQL和随机隔离数据库，保留原开发实例。H5先构建后启动固定存活时间的fixture，使用仅限本机的HTTP转发连接实际API，不替换响应。每轮完成后关闭所有自建浏览器上下文、页面、临时服务和测试库；完整测试阶段结束后关闭临时PG。原始失败与准确输入单独归档，不覆盖成通过。

Native Windows PostgreSQL, actual HTTP writes and leased notification processing verify local behavior. H5 is a local shared-page check; miniapp identity/release, signed Android release, real model/Skill/TG/cloud and production acceptance remain separate. No device, emulator, virtualization, network performance, real120GB transfer, historical migration/DR or AI fee control is introduced.

最终执行结果与来源见[验证记录](../quality/111-contract-reminder-policies-verification.md)。全部未完成项继续保留在[编号台账](../delivery/03-goal-gap-assessment.md)；子功能完成不把整个GAP-08或整个Goal记为完成。
