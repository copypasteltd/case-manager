# 86 · 秘书月度Token额度与用量账本 / Monthly secretary token budgets

本增量推进 GAP-14 的接单预留、范围额度、失败结算和可核对报表。它管理 Token 数量，不把 Token 换算成货币，不代表供应商账单，也不能在 SDK 尚未报告用量时阻止已经发生的供应商计费。完整四端平台目标继续有效。首发仍面向香港3位员工、境内服务器、最大120GB原件及2TB存量；真实云、模型、Telegram与真机验收尚未完成。

This increment advances GAP-14 with admission reservations, scoped allowances, failure accounting and reviewable reports. It accounts for tokens without converting them into money. It is neither a provider invoice nor a provider-side spending limit. The full four-client platform remains the delivery goal.

## 1. 操作流程 / Workflow

1. 管理员使用真实交互会话登录，在 Web「秘书额度」或移动端「我的工作空间 → 秘书额度与用量」打开账本。具备 `policy.manage` 且两步验证仍在有效窗口内，才可发布策略或核对未知用量。
2. 首次发布时填写组织、每位员工、每个项目、每项 Skill 的四个**完整整数额度**及原因。四项均须填写，不从其他值推断。`0` 表示该范围不再接新运行；非负整数上限为 `1,000,000,000,000`。生产迁移不赠送默认额度。
3. Web、微信、抖音、Android 和 TG 的运行接单共用 Go 后端。每次预留所选运行配置的 `max_input_tokens + max_output_tokens`。同时满足组织、发起员工、所属项目（如有）和 Skill 的剩余额度，才创建运行、用户消息、预留和单次执行队列任务。
4. 查看本人账本无需管理权限。管理员可按整个组织、员工 UUID、项目 UUID 或 Skill ID 查询。月份使用 `YYYY-MM`；留空为数据库当前 UTC 月。详情通过签名游标分页，范围或月份变更后旧游标不可复用。
5. 正常接收 SDK 的 `turn.completed` 用量时，运行时先向 Go 控制面提交 `run.usage`，之后才判断 Token 越限、回复内容或最终结果。即使结果随后被拒绝，已记录的实际用量仍然存在。
6. 已终止且用量未知的运行显示「用量待核对」。管理员确认外部用量证据后，明确填写输入、输出 Token、核对凭据编号和原因，再执行核对。只有有凭据确认确无用量时，才明确填 `0`。页面展示核对结果，不修改原始预留。

An interactive administrator opens **Secretary budget** on Web or **My workspace → Secretary tokens** on mobile. Publishing or reconciling requires `policy.manage` and fresh MFA. Publish all four integer allowances and a reason; migrations supply no production allowance. All clients and Telegram share admission accounting. A run reserves its profile's full input-plus-output ceiling against every applicable scope before any durable work is queued.

Every member can inspect their own ledger. Administrators may filter by organization, user UUID, project UUID or Skill ID. Months are UTC calendar months. Detailed records use signed, scope-bound pagination. When the SDK reports a completed turn, the runtime submits usage before accepting or rejecting the final answer. Unknown terminal usage requires explicit counts and an external reference before human reconciliation; the original reservation remains immutable.

## 2. 额度语义 / Allowance semantics

| 维度 / Dimension | 聚合键 / Account | 检查 / Check |
| --- | --- | --- |
| 组织 / Organization | `org_id + UTC admission month` | 所有员工、项目、Skill、渠道合计 / All channels and scopes combined |
| 员工 / User | `org_id + actor_id + month` | 发起人的运行合计；非当前聊天成员合计 / Initiating actor, across conversations |
| 项目 / Project | `org_id + project_id + month` | 所有员工在同一项目会话的合计 / All actors in that project |
| Skill | `org_id + skill_id + month` | 同一 Skill 的所有版本及配置合计 / All versions and profiles of that Skill |

- 每位员工、项目和 Skill 使用策略中对应的共同上限，拥有独立累计账户。本增量没有个别员工／项目的差异化覆盖值。
- 不属于项目的个人会话不消耗项目账户，仍受组织、员工和 Skill 上限约束。项目 ID 来自后端的会话记录，不能由模型或工具参数指定为另一个项目。
- 月份在**接单时**固定；跨月完成或核对仍归原接单月。没有结转余额，也不在月末伪造免费结算。
- 当前策略控制下一次接单。降低额度不会撤销已接单预留，也不会通过自动取消假定产生退款。可用数可能为负，后续接单会被拒绝。历史月份页面显示当前策略参考上限；各运行保存原始策略 ID。
- `input_tokens + output_tokens` 是本账本的数量定义。缓存输入已经包含在输入中，推理输出已包含在输出中，不重复相加。供应商缓存写入等独立价格语义不在本次数量换算范围。

Every user, project and Skill has a separate account using its dimension's common ceiling; individual overrides are not implemented. Personal conversations omit only the project account. Admission fixes the UTC month and exact policy ID. Cross-month completion stays in that original account. New policies affect future admissions and preserve existing reservations. Historical reports show the current policy as a reference ceiling, while entries retain their admission policy ID. Token accounting adds input and output once, without double counting cached inputs or reasoning outputs.

## 3. 结算状态 / Accounting states

| 状态 / Status | 运行预留 / Reserved | 已记用量 / Charged | 待核对 / Pending |
| --- | --- | --- | --- |
| `reserved` | 尚在队列／执行中的完整预留；已知越限用量高于预留时取较大值 / Full active reservation, or higher observed usage | 0 | 0 |
| `released` | 0 | 未启动即取消／失败为 0 / Zero when no execution started | 0 |
| `settled` | 0 | SDK 已知输入 + 输出，包含失败、取消和越限后的实际数量 / Observed input + output, including failed/cancelled turns | 0 |
| `pending` | 0 | 以原始完整预留保守记账 / Conservatively retain the estimate | 同一估算值，是已记用量的子集 / Same estimate, already included in charged |
| `reconciled` | 0 | 管理员凭证核对的输入 + 输出 / Explicitly reconciled counts | 0 |

`可用额度 = 当前策略上限 - 运行预留 - 已记用量`。不要再次减去「待核对」，否则会重复计算。未启动与已启动由持久化 `run.started` 事件区分；页面关闭、取消请求或模型文字均不能证明用量为零。

Settlement is a database view over immutable reservations, usage, reconciliation records and durable execution state. Completion, cancellation and controller reconciliation therefore have no separate refund-job crash window. A still-running cancellation request keeps its reservation until the trusted controller ends execution. Accounting after cancellation may record usage under a valid lease, but grants no business authority and cannot revive the run.

核对只允许 `failed / cancelled / interrupted` 且已启动、尚无用量的准确终态版本。SDK 已知用量不能用人工核对覆盖；一次核对记录不可编辑、删除或再核对。相同幂等键可读取原回执；不同键再次核对会冲突。后续若需更正已核对凭据，应设计独立的补偿账目流程，不能直接修改历史。

Reconciliation requires the exact terminal run version, a started execution and absent observed usage. Known SDK usage cannot be overwritten. A reconciliation is immutable and accepted once. Replaying its original idempotency key returns the receipt; a new key conflicts. A future correction workflow would need explicit compensating records rather than editing history.

## 4. 接口 / API

| 方法 / Method | 路径 / Path | 用途 / Purpose |
| --- | --- | --- |
| GET | `/v1/agent-budget` | 策略参考、范围汇总、可用数、待核对数、升级前未记账运行计数 / Policy, totals, available tokens, pending usage and legacy count |
| GET | `/v1/agent-budget/entries` | 分页运行账本 / Paginated run ledger |
| POST | `/v1/agent-budget/policies` | 发布完整策略修订；初始 `expected_version=0` / Publish a complete policy, initially at version zero |
| POST | `/v1/agent-budget/entries/{run_id}/reconcile` | 凭证核对未知用量 / Reconcile unknown terminal usage |
| 控制面 / Controller | `run.usage` event | 仅持有准确执行凭据的本机受控运行时可报告 / Exact authenticated execution lease only |

查询参数：`month`、`scope=mine|organization|user|project|skill`、必要时 `scope_id`；详情另有 `cursor`、`limit=1..100`。重复范围参数、无效月份和无效 UUID 均拒绝。`mine` 固定当前用户。其他人的数据和组织聚合要求组织级管理授权；不可见范围沿用原有隐藏资源语义。

业务写入沿用现有 CSRF／设备会话、幂等键、实时授权、审计与 outbox。策略修订使用组织行锁，运行接单使用组织级事务锁；两个同时到达的请求无法消费同一份剩余额度。额度不足返回 `429 AGENT_BUDGET_EXCEEDED`；未配置返回 `409 AGENT_BUDGET_UNCONFIGURED`。旧版无预留运行不获得新的受控执行权限，报表显示其历史数量而不补造估算。

Writes retain existing CSRF/session, idempotency, live authorization, audit and outbox rules. Policy changes serialize against admissions, and admissions serialize across channels. Missing policy returns `409`; insufficient capacity returns `429`. Rejected admissions roll back the run, message, reservation and queue work together. Legacy runs are counted separately, without invented usage or new execution authority.

## 5. 实现与升级 / Implementation and upgrade

| 文件 / File | 职责 / Responsibility |
| --- | --- |
| `app/backend/migrations/0072_agent_token_budgets.sql` | 四张不可变表、租户／运行绑定校验、动态结算视图、应用角色权限 / Immutable records, tenant/run guards, settlement view and database grants |
| `app/backend/internal/agent/budget.go` | 策略发布、原子接单、可信用量记录 / Policy publication, atomic reservation and trusted usage |
| `app/backend/internal/agent/budget_reports.go` | 授权查询、签名分页、人工核对 / Authorized reporting, pagination and reconciliation |
| `app/backend/internal/httpapi/agent_budget.go` | 四个业务接口 / Four business endpoints |
| `app/agent-runtime/src/codex-adapter.ts` | 在结果校验前提交 SDK 已知用量 / Meter observed usage before result rejection |
| `app/packages/contracts/src/budget.ts` | Web／Taro 共用完整整数与凭据校验 / Shared explicit input validation |
| `app/web/src/AgentBudget.tsx` | Web 账本、策略、核对 / Web account workflows |
| `app/mobile/src/pages/budget/index.tsx` | 微信／抖音／Android 共享 Taro 页面 / Shared mobile workflows |

先停止接新秘书运行并等待现有运行终结，再备份数据库、应用第72项增量迁移、更新 Go／运行时／客户端，最后由管理员发布真实额度。原有前71项迁移不得改写。本地开发检查仅操作已验证归属的原生服务，不启动 Docker、WSL、虚拟机或模拟器。所有临时浏览器、上下文和页面检查服务在退出时关闭。

Before upgrading a live environment, stop new secretary admissions and let existing runs settle, back up PostgreSQL, apply migration72, update services/clients and have an administrator publish actual allowances. The preceding71 migration files stay byte-for-byte unchanged. Local verification uses native tools and releases every owned browser context and review server.

## 6. 验证与实际边界 / Verification and remaining work

14项原生集成场景覆盖：无策略拒绝、权限与策略版本、消息／命令重放、组织额度并发、员工／项目／Skill独立上限、零额度、缓存不重复计数、失败及越限用量、取消后的记账、未知用量核对、不可变历史与游标，以及预留失败的整笔回滚。SDK 用量传输通过真实本机 Go 控制面检查，不调用供应商。前端共享校验和运行时中断场景另有单元检查。

Fourteen native scenarios cover missing-policy denial, authority/revisions, replay, concurrent admission, all scoped quotas, zero limits, cache accounting, known failures and overshoots, cancellation accounting, unknown usage reconciliation, immutable history/cursors and atomic rollback. Actual local Go HTTP metering is exercised without a provider. Browser checks use real authenticated pages, native API and synthetic records; mobile recordings are Taro H5.

结果、源文件摘要、构建范围、浏览器资源释放、截图和录屏见[本增量验证记录](../quality/evidence/token-budgets/verification.md)。这些检查不等同于真机、供应商金额账单或生产验收。

GAP-14仍未关闭：价格版本、货币、供应商账单导入与金额差额、个别范围覆盖值、供应商侧费用硬限制、多段执行中的费用控制和补偿更正流程仍须继续实现／联调。SDK 在一次 turn 结束后才报告总用量；此前不能据此准确逐 Token 扣款或撤回已发生的费用。也不把本次合成用量当作真实模型联通证据。所有 Skill 仍保持 draft，真实模型和 TG仍保持禁用。

GAP-14 remains open for versioned prices/currencies, provider invoice import and monetary reconciliation, individual overrides, provider-side spending limits, segmented execution cost control and compensating corrections. SDK usage arrives at turn completion; this ledger cannot preempt costs that already occurred. Synthetic accounting proves neither live model connectivity nor production readiness. Skills remain draft and real model/Telegram integrations remain disabled.
