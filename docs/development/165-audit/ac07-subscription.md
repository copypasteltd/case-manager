# AC-07 — 订阅、提醒与月报源码核对

> “订阅月付/年付、变价、取消” → “账期、预计/实际、提醒、历史快照与月报正确”
>
> 来源：`docs/06-decisions-risks-and-acceptance.md:97`，AC-07。

> “时区与提前提醒规则以账期快照为准，不能服务重启后重新推迟扣款日。”
>
> 来源：`docs/requirements/05-finance-and-subscriptions.md:52`，FIN-10。

**Verdict：partial；源码判断置信度：高。** 基线为 `3dc6deeba29298fbff101e818ac6b68e944db07a`。本项审查只读取源码、需求与 SQL；未运行测试、构建、浏览器或服务。报告文件是唯一写入。本结论不关闭 AC-07、GAP-03/18 或23个工作包的正式验收。

## 要求对应的具体性质

月付、年付必须从原始日历锚点生成半开账期；价格和账期修订保留历史；暂停、取消与供应商实际停费分别记录；预测不能制造实际费用或付款；真实费用只按规范记录计入发生额。提醒应服从准确账期的配置快照、当地业务日期与当前接收权限。报表应分开预测、发生额和现金，明确覆盖不足，并保留可复核的已完成导出／秘书月度快照。依据为 FIN-08～12（`docs/requirements/05-finance-and-subscriptions.md:44–70`）。

## 已定位的实施差异

| 编号 | 结论 | 具体路径和影响 |
| --- | --- | --- |
| AC07-S1 | **需要修复；主代理及另一审查者已独立确认** | 对已生成账期，更新订阅的 `reminder_days` 会使后台采用新规则补排／停止补排提醒，而不是沿用账期固定配置。新提醒日期、所有者和事件虽然固定，选择提前天数的依据已经变成最新配置；FIN-10 的快照约束未成立。 |

证据链：

1. `subscription_period_mutations.go:24–41` 将 `configuration_revision_id`、价格、时区和负责人固定到不可变账期修订；已有数据足以读取其原 `reminder_days`，无须重写历史。
2. `subscription_mutations.go:275–298` 允许更新 active/paused 订阅；第284～289行只禁止修改价格与固定日历。`subscription_content.go:176–187` 的日历摘要不包含提醒天数。Web 的 `SubscriptionEditor.tsx:20` 和共享移动端 `pages/finance/subscriptions.tsx:51–54` 都提交可编辑的提醒天数，故此路径在四端可到达。
3. `subscription_jobs.go:192–212` 读取并核对最新订阅配置后调用 `queueSubscriptionReminders`；第231～233行的 `unnest($3::integer[])` 实际参数为 **`sub.ReminderDays`**，未从 `r.configuration_revision_id` 取规则。
4. SQL 不会拦截这个差异：`0038_subscriptions_and_forecasts.sql:328–340` 检查租约、固定日期、账期负责人和发件箱，但第335行同样使用 **`cfg.id=s.current_revision_id`** 检查 `reminder_days`。源码搜索未找到后续替换该函数的迁移。
5. 提醒记录本身受到 `(org_id,period_id,days_before)` 唯一约束和禁止修改／删除保护（同迁移第87～92、327行）；问题是创建哪些提醒时读取了错误版本，并非旧提醒行被覆盖。

静态例子：账期按 `[7,0]` 规则生成，尚未到提醒日时把当前订阅改为 `[3]`。在原7天提醒日执行后台任务，第233行已不枚举7；在3天提醒日却可创建原账期从未固定过的3天提醒。既有配置／账期历史仍在，但通知行为与其不一致。此例未执行。

修复边界：后台选择和数据库 INSERT guard 必须共同使用账期准确配置。保留既有迁移、账期修订、提醒和通知证据；采用追加迁移，不通过改写旧行“补成通过”。时区已经在启用后固定，实际缺口集中在可变的提前天数。

## 已走查的调用与约束

下表表示读过的具体实现和对应约束，不代表这些路径已重新运行通过。

| 路径 | 实际执行依据与观察 |
| --- | --- |
| HTTP → 业务服务 | `httpapi/subscriptions.go:30–140` 的订阅、配置／事件／提醒／账期历史、生成／异常／重估／关联费用／核对及预测报表入口均进入 `read` 或 `command`；`server.go:245–289` 获取当前身份并解码约束。报告入口见 `finance_reports.go:53–58、78–147`；完整下载独立调用当前身份和同一下载服务。 |
| 月付／年付 → 半开区间 | `subscription_calendar.go:68–106、143–192` 每次从原始首次收费年月和锚点计算，月底截短不改变后续锚点；时区用于真实日期转换。SQL `0038:115–131、303–310` 独立检查原日历、起止日及固定配置。 |
| 调价 → 历史预测／显式重估 | `subscription_prices.go:20–78` 排序、币种小数精度和区间校验；`subscription_mutations.go:300–342` 追加新配置，禁止回填过去价格。`subscription_period_mutations.go:315–347` 仅对未关联真实费用的合格预测，按准确当前配置新增修订；SQL `0038:178、298、303–310` 重复检查资格和快照。已生成账期不自动重算，与 `docs/development/37-subscriptions-billing-and-forecast.md:17–19` 一致。 |
| 暂停／恢复／取消 → 重复生成 | `subscription_mutations.go:344–453` 记录真实业务日期、内部状态及单独供应方证据。`0038:136–140` 按暂停／恢复事件顺序和取消生效日判断收费资格。`subscription_period_mutations.go:83–119` 在完整自然账期键上查重；SQL `0038:44` 唯一约束兜底。未找到重复生成或越过取消生效日新建正常预测的差异。 |
| 持久调度 → 固定窗口 | `subscription_jobs.go:39–140、154–224` 将业务日期、前后窗口、配置和启用／恢复控制版本固定入队；事务内重查当前配置、项目锁、租约和围栏，生成／提醒／事件与任务完成共同提交。过期配置跳过，不把恢复时刻改成新的收费锚点。 |
| 预测 → 实际费用关联／核对 | `subscription_period_mutations.go:349–474` 读取准确已确认规范费用，核对币种、供应商、项目、分类和重复关联；关联与费用／账期事件原子追加。确认固定费用集合指纹和凭证，金额差异需声明；预测生成器不创建费用／付款。`subscription_period_reads.go:31–108` 动态呈现差额、缺票、冲销、停订后扣款和过期确认。 |
| 授权 → 读写／通知／重试 | `subscription_reads.go:38–46` 与 SQL `0038:109–112` 要求当前订阅及全部关联费用读取权。`subscription_mutations.go:208–254`、`subscription_period_mutations.go:200–291` 重新检查当前授权、实际人工会话／MFA和准确头版本。通知 `notification/jobs.go:132–140、194–231、298–341` 检查准确事件版本、固定负责人、资格及当前有效接收人／读取权；`notification/access.go:115–137` 对四端读通知再次核对。`notification/retries.go:173–210` 只对授权失败任务追加固定原事件的子任务，仍走上述普通分发器。 |
| 月报 → 规范记录／覆盖 | `report.go:26–98` 按规范费用与冲销各计一次，不把来源关联 join 进汇总；`forecast_report.go:15–25、73–155` 在一致读取事务内聚合已存预测并独立检查缺账期、缺价格、草稿及1000项覆盖上限。`report_sources.go:20–43、77–125` 让当前报表汇总与来源使用同一事务和预测资格口径。 |
| 不可变历史／已完成快照 | SQL `0038:303–327` 检查精确配置／价格并禁止历史修订、事件及凭证改写。`report_export.go:150–231` 在一致读取事务封存汇总、明细、原币、来源和哈希；`0073_financial_report_exports.sql:34–65` 核对准确任务、固定筛选和行数，禁止修改及未到期删除。`report_csv.go:99–158` 下载时重新检查所有来源权限并复算内容哈希。`finance/agent_summary.go:96–181` 分开月度 forecast/actual/cash；`agent/finance_summary.go:13–63、104–129` 固定会话、受众、内容、来源与摘要，读取旧秘书快照时重查当前权限。 |
| Web／共享移动端 → 同一普通业务命令 | `web/src/SubscriptionActions.tsx:33–46` 与 `mobile/src/pages/finance/subscriptions.tsx:61–69` 分别进入生成、异常、价格和生命周期普通接口；`web/src/SubscriptionPeriods.tsx:41–52` 与 `mobile/src/pages/subscription-period/index.tsx:34–44` 固定账期、配置、费用和实际指纹。当前报表使用 `packages/contracts/src/finance-reports.ts:43–62` 的筛选与响应检查，由 Web `FinancialReports.tsx:29` 及移动端 `pages/reports/index.tsx` 共用。共享移动端覆盖微信／抖音／Android源码，不等于实际平台验收。 |

## 补充交错边界：尚待独立定性

未来账期已有排队提醒之后，用户合法显式重估：`finishSubscriptionPeriod` 增加账期版本（`subscription_period_mutations.go:293–302`）；旧事件在通知分发时因版本不符被抑制（`notification/jobs.go:138`）。后续后台任务又因已有相同账期／提前天数而不创建替代事件（`subscription_jobs.go:233`、SQL `0038:90`）。这是读取到的交错结果，已交主代理独立复核；**不能声称仅改为固定配置就覆盖了该边界**。处理时应区分原提醒已送达、尚在队列、已抑制、已失败以及明确的重新固定，保持原通知和决策历史，避免自动重复发送。

## 统一测试阶段建议覆盖的边界

以下仅为待执行清单，没有新增或执行测试。

| 编号 | 输入／操作 | 必须核对的结果 |
| --- | --- | --- |
| S1 | 生成账期后，将当前规则 `[7,0]` 改为 `[3]` | 原账期仍按原7天、0天日期产生候选；不新增原快照没有的3天提醒；新生成账期采用新规则。 |
| S2 | 原规则为空，后续新增；原规则非空，后续清空 | 两种方向都不反向改变旧账期的规则；已有提醒字节保持固定。 |
| S3 | 原提醒日当天、前一天、后一天／重启补排 | 当天能排，提前一天不排，补排仍保留原提醒日期，不把截止日向后推。 |
| S4 | 香港日期与 UTC 日期跨日、月末／二月／闰年年费 | 按账期固定时区判定，固定原始锚点；不按进程时区或上一次截短日期漂移。 |
| S5 | 显式重估未到收费日、无费用的预测 | 准确新配置／新修订生效；保留旧修订；分别观察已排队与未排队的同天数、不同天数提醒及上述版本交错。 |
| S6 | 暂停、恢复、取消待生效／已生效后重复跑任务 | 正常资格按原事件日期判断；自然账期、提醒和事件不重复；实际扣款历史保留。 |
| S7 | 接收人停用、撤权、周期已核对、人工重试 | 普通分发和重试继续抑制失效接收／状态，不因修正选规则绕过当前授权。 |
| S8 | 人工确认、费用冲销、重新查月报与旧导出 | 新读取展示过期确认和独立冲销口径；已封存历史快照／CSV内容及来源不被重算覆盖。 |

## 搜索与未覆盖边界

- `rg -n 'AC-07|提醒|快照|重预测|暂停|取消|归档'`：定位原 AC-07、FIN-08～12 和增量37明确约定；归档仅要求取消日前已存账期核对是已声明设计，不作为新缺陷。
- `rg -n -g '*.sql' 'verify_subscription_reminder|subscription_charge_eligible|reminder_days' app/backend/migrations`：提醒规则 INSERT guard 和资格函数命中0038；未找到后续重定义。没有因为某个文件未找到就断言功能缺失。
- `rg -n -g '*.go' 'subscription.reminder' app/backend`：除已跳过的测试外，实际排队在 finance，分发在 notification；随后逐段读取这些实现、SQL和重试调用链。
- `rg -n 'reminder_days|fixedCalendar|reforecast'`：定位 Web 和共享移动端实际提交与可编辑字段；继续读取相关命令调用。
- `rg -n 'reportForecastPredicate|subscription_period'`、`rg -n 'finance.AgentSummary|ValidateAgentSummary'`：追踪汇总、来源、不可变导出和秘书月度快照，排除了“JSONB重排导致快照哈希必失败”的假设，原因是0073迁移第59行在数据库内固定实际规范文本哈希。
- 本报告不是全目录每条语句审计；未穷尽复杂组合、长时间大数据分页、并发调度的所有交错、每种 FX／分摊组合、全部秘书审阅工具或各客户端所有展示分支。没有重新执行任何既有用例或引用测试名称作为通过依据。
- 公司财务制度采纳、真实供应方核对证据、云／模型／TG配置、正式平台验收仍按原登记保持开放。本审查不重新引入真机、网络性能、历史迁移与容灾或 AI 费用控制门槛。
