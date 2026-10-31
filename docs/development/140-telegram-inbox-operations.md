# 增量140 · 组织TG入站调度 / Organization TG inbound operations

管理员现在可以统一查看组织已接收的TG请求、排队原因、当前任务状态和完整分页历史，并明确撤回尚未创建秘书任务的请求。此流程补足GAP-07／18的入站台账与人工干预，不授予查看员工请求正文或秘书结果的权限。

Administrators can inspect accepted inbound requests across their organization, diagnose waiting configuration/capacity, read the immutable request journal and withdraw an exact unstarted request. Separate read and withdrawal permissions preserve private request/model prose and existing requester-only access.

## 入口与使用 / Usage

1. 管理员在账号权限中授予“TG入站台账查阅人”或“TG入站排队经办人”，明确组织范围。系统管理员拥有此元数据管理能力，业务正文仍遵循原权限。
2. Web：Telegram接入→组织TG入站调度，地址 `/telegram?view=inbox`。微信／抖音／Android共享移动源码：本人TG请求→组织TG入站调度，地址 `/pages/telegram/index?section=inbox`。
3. 按入站状态、任务类型和准确群入口UUID筛选；列表以不可改的接收时间／ID排序。点击详情核对原群、话题、入口修订、请求人编号、下次调度时间及30分钟排队有效期。
4. “指定任务配置：已安装”只表示数据库中存在有效的该任务TG执行配置，不证明调度器存活、模型可用或Skill正式发布。等待原因取实际调度结果；缺少配置、容量不足和上下文忙碌分别展示。
5. 经办人核对准确请求，完成最近10分钟MFA，确认停止后续调度并填写原因。提交带准确台账版本和稳定幂等标识。若调度器已经处理或创建任务，返回冲突，需重新读取；已创建任务须沿原任务取消流程处理。
6. 撤回后原请求正文保留给本人，状态历史保留原事实。此请求不会新建秘书任务；已经发出的群通知不召回。尚未发送的通知沿既有发送器当前授权检查停止；实际Bot送达仍须正式接入。

Read-only operators use `telegram.inbox.read`; withdrawal additionally requires `telegram.inbox.withdraw`, a current human Web/WeChat/Douyin/Android session, recent MFA and explicit review. A metadata role cannot access another employee's `/me/telegram/requests` body. Withdrawal accepts no replacement text, destination, task, model or runtime profile.

## 接口契约 / API

所有路径均加 `/v1`，由既有认证、CSRF、命令幂等和OpenAPI校验处理。

| 方法／路径 | 输入与结果 |
|---|---|
| GET `/telegram/inbox-operations` | 可选state、channel_id、skill_id、cursor、limit；仅已准入request，排除绑定证明、忽略消息和成员事件 |
| GET `/telegram/inbox-operations/{id}` | 当前准确元数据及首20项历史；详情不接受查询参数 |
| GET `/telegram/inbox-operations/{id}/history` | 完整分页历史；仅cursor、limit，默认20、最多100 |
| POST `/telegram/inbox-operations/{id}/withdraw` | expected_version、reviewed_request=true、1–200字reason；返回撤回后的元数据 |

拒绝未知／重复参数、错误任务、越界分页和非UUID入口。游标绑定当前员工／组织授权、筛选条件；历史游标另绑定准确请求。列表按接收时间、历史按实际观察时间稳定分页，工作进程更新不会把列表顺序改成最近修改顺序。

组织响应为白名单路由／状态：请求ID、台账版本、Bot、原群／话题、当前入口名称与版本、内部请求人ID、任务类型、状态／原因代码、接收／调度／有效期时间、任务ID与当前状态、范围匹配和配置是否存在。没有请求正文、Telegram私人用户号、绑定证明、会话ID／结果、凭据或自由文本人工备注。人工原因保存在独立回执和原审计体系，由已有授权入口查阅。

## 数据与事务 / Persistence

正常升级为 `0106_telegram_inbox_operations.sql`；不改1–105迁移或既有telegram_inbox不可变身份约束。

| 对象 | 约束 |
|---|---|
| telegram_request_events | 每个实际接收、状态／等待原因／调度时间变化，同事务自动追加准确修订；禁止更新／删除，应用账号仅SELECT |
| observe_telegram_request | 固定search_path的数据库触发器，由迁移身份执行写入；不接收模型／客户端传入的历史。应用不能直接INSERT伪造历史 |
| retained_baseline | 已有请求在启用台账时保存一次当前状态，时间为实际观察时间；不补造过去的接收／重试事件 |
| telegram_request_withdrawals | 准确前后修订、操作者、原会话、原因；独立不可变回执和准确历史FK |
| deferred manual receipt constraint | manual_queue_withdrawn必须有对应人工回执，缺失时整个事务失败 |
| roles／索引 | 单独查阅／经办角色、系统管理员明确action、组织授权版本失效及请求／状态／入口分页索引 |

所有既有入口变更、成员变化、本人绑定撤销、实际调度等待／启动／失效都会经同一父行事务留下请求历史，工作进程无需绕开或仿造台账。重复Webhook沿既有去重协议，不产生第二次接收历史。修订号由父请求锁串行生成。

人工命令先检查当前组织、用户、会话及角色，再允许幂等重放；锁住准确请求后核对当前台账修订和无run link。调度器原有组织锁与该父行锁使启动／撤回串行：人工先提交则调度器不再接单；调度器先改版／启动则旧人工命令冲突。请求状态、自动历史、人工回执、审计、outbox和幂等响应同事务提交，任一失败全部回滚。

Legacy requests without `requested_skill_id` remain readable and report `scope_current=false`. They acquire no new task authority and remain withdrawn. Baseline observations are distinct from actual acceptance events; no historical migration/DR project is added.

## 交付边界 / Delivery boundary

本轮main140／结构106。个人开发PG仍结构100，使用最新后端前需正常升级101–106；本轮原生隔离PG从空库安装到106，不声称已升级个人开发库或生产库。两冻结标签不动，rc.2及APK137仍业务136／结构103，不包含138–140。

当前验证聚焦实际HTTP／耐久PG、既有TG调度关键回归、四端会话、客户端类型和Web／两小程序构建、实际认证Web提交与持久读回。全量后端／运行器／客户端回归、完整移动UI旅程、新Android包、目标Linux、真实Bot／模型／云／平台和生产验收延期。保留原136客户端完整回归和137开发包的准确出处。

剩余TG运行配置、更多任务、群正文披露／写入预授权、真实模型和八项Skill正式发布、真实Bot／群及完整业务仍在GAP-07／09–13／15内。23工作包（14 P0／9 P1）、5项移出和八项draft Skill保持。

[关键验证与延期](../quality/140-telegram-inbox-verification.md) · [本轮及全部余项](../delivery/05-functional-closure-140.md)
