# GAP-03 — 订阅生命周期与独立财务口径源代码核对

基线：`930cdde97b9d5d2e85d892664b0857a10a811739`。本次为增量168代码盘点阶段的独立规格／代码核对；没有运行测试、构建、浏览器、服务、数据库或云接口，没有修改应用代码。既有执行记录不作为本次执行。

## 被核对的要求

> “按账期生成预计开支，人工或经核准接口登记实际扣款、票据和异常。支持月付/年付等周期、价格变更生效日期、续费提醒、取消与历史记录。月报区分预计、已发生、已处理，按项目/类别/供应商汇总。”
>
> “如同时需要‘实际现金支出’和‘按月摊销成本’，应分开显示；年费不能同时按全额支付和每月摊销重复计入同一个口径。多币种保留原币，汇总时使用经公司确认的记账币种、汇率来源与记账日期。”
>
> — `docs/01-requirements-and-scope.md:119`、`:121`，§5.4。

后续可执行细化为 `docs/requirements/05-finance-and-subscriptions.md:23–30`（FIN-01—05、永久来源去重）、`:36–40`（十进制原币／明确汇率）、`:44–52`（FIN-08—10、日历／价格／生命周期／对账）、`:66–70`（FIN-12、独立报告、筛选与导出）。本文核对这些普通路径的当前代码，不把尚待采纳的公司财务制度转为默认规则。

**Verdict：implemented，限于以下已经逐路径阅读的普通订阅与报表代码。确认的新代码背离：0项。整个GAP-03／正式财务验收仍开放。**

## Where enforcement lives / 执行与入口

| 约束 | 当前执行证据及调用路径 |
|---|---|
| 可维护订阅、准确供应方／负责人／日历／首价 | `app/backend/internal/finance/subscription_content.go:32–163` 校验固定资料、自然日、时区、周期、价格、供应方核对时间与文件；启用完整性在`:160`。`subscription_mutations.go:157–206` 从Create进入普通命令、交互会话、角色、项目锁及引用检查；创建仅为draft，没有费用／付款插入。 |
| 草稿与正式变更的当前权限／准确基线 | `subscription_mutations.go:208–254` 普通写入口二次复查当前可读／动作权限、原人工会话及正式状态MFA，按项目→订阅加锁后再次检查，核对准确版本／修订／摘要；`:275–298` 已启用日历和原价格不能静默改写。 |
| 月底／闰年／自定义周期不漂移 | `subscription_calendar.go:33–106` 每次由原始锚点计算月／年日期，月底截断不覆盖原锚点；`:111–184` Next／Windows均从原点计算，单窗口1–366自然日。`0038_subscriptions_and_forecasts.sql:115–130` 独立校验原日历和账期结束日。 |
| 价格版本及历史固定 | `subscription_prices.go:20–49` 明确有效区间、非重叠、币种精度与已知零；`:62–80` 新价格只截断新修订中最后窗口。`subscription_mutations.go:300–342` 不回填过去、追加不可变配置；`subscription_period_mutations.go:24–42` 账期固定配置／价格／分类／负责人／供应方／时区，已有账期不原位跟随调价。 |
| 启用／暂停／恢复／取消／归档与供应方实际状态分离 | `subscription_mutations.go:344–451` 显式状态机，取消生效日按订阅时区、归档前固定供应方取消证据及已有账期核对；恢复仅返回原取消状态。`subscription_reads.go:58–108` 显示取消待生效、供应方停费未核对、价格未知等投影，不制造外部停订／付款事实。 |
| 预测不制造实际成本／现金；不重复账期 | `subscription_period_mutations.go:83–119` 只生成固定预测，并分别列缺价格；自然唯一键在`0038_subscriptions_and_forecasts.sql:43–44`，即使作废也不删除。`:121–143` 人工生成走普通权限/MFA/原子命令；`:320–351` 仅明确重估未来且从未关联实际费用的预测。 |
| 实际费用唯一，关联及复核可失效 | `subscription_period_mutations.go:354–406` 固定费用版本／修订／摘要，核对confirmed/expense、币种／供应方／项目／分类；一项费用已归属任何订阅账期即拒绝再关联。`:408–479` 核对固定费用集合指纹，真实费用至少一项，差额需明确承认及固定凭证；有历史费用关联不能免付／作废掩盖支出。`0036_actual_costs.sql:9` 永久`(org_id,source_key)`唯一，`0038...:132–134` 指纹包含费用ID／修订／状态。 |
| 不变历史／原子回执 | `subscription_mutations.go:68–109,256–273` 配置／价格／供应方凭证及事件追加；`subscription_period_mutations.go:293–318` 账期头／订阅事件／账期事件／提醒排队在同一普通命令事务内。`0038...:207–219,277–289` 核对不可变版本数量和完整事件头。 |
| 实际任务围栏、固定提醒及投递术语 | `subscription_jobs.go:71–154` 每日固定前向／补账任务、配置及控制版本去重；`:186–256` 复查真实任务租约／围栏、原配置／控制版本，失效任务跳过，队列完成与预测／事件同事务；`:258–270` 提醒读取准确账期配置和时区、去重包含账期版本、原日期及上限，未制造付款。 |
| 独立报表与避免多业务重复加总 | `report_query.go:64–113` 明确incurred/cash/forecast/amortization及期间、项目／公司、类别、供应方、经办／负责人、币种，分别分派；`report.go:27–32,75–85` 发生额仅从规范费用及分摊聚合，不join业务关联重复相乘；confirmed/reversed原事实仍贡献原期间，反向记录独立带符号。`cash_report.go:26–30,74–84` 只计算outgoing/refund与准确分配/实际退款现金贡献，incoming不计作支出。`report_sources.go:20–42,77–125` 与汇总共用准确来源，在同一repeatable-read事务核对授权、分页、明细及折算。 |
| 未知预测与原币／FX | `forecast_report.go:27–46,68–153` 已持久预测与日历／缺价格／未补齐草稿／1000项覆盖上限分别计算，未知不补零；币种分组不跨币相加。`report_valuation.go:52–115,128–219` 只在明确选择FXBook后读取固定报价版本，精确有理数/PG numeric逐行舍入，缺报价保留missing_rate，覆盖不完整不返回全额；`:220–267` 冲销沿原事实报价日。 |
| 摊销独立可选，不叠加发生／现金 | `amortization_schedule.go:14–109` 明确人工monthly_equal/daily_weighted、日期与120月份上限，按原币最小单位累计舍入，逐项目／公司份额与总额守恒。`amortization.go:194–267` 原费用和计划基线、预览摘要、明确确认和发布/停用MFA；`amortization_report.go:13–32,40–60` 独立最新有效计划视图，冲销保留已确认月份并反向，不改原发生／现金事实。 |
| Web与共享移动可达 | Web`Subscriptions.tsx:15–79` 列表／详情／账期／固定修订及提醒，`SubscriptionActions.tsx:25–68` 明确固定打开基线，选择生成／价格／停订／供应方证据后调用同一HTTP协议。共享移动`pages/finance/subscriptions.tsx:19–38,58–69` 同状态操作、重新读当前对象和能力、固定请求与回执，`pages/finance/index.tsx:3–4,40,83` 订阅／账期及成本摊销入口。Web`FinancialReports.tsx:25–35` 与共享移动`pages/reports/index.tsx:22–43` 独立口径、准确筛选、读取失败隐藏旧结果；移动同页`:44–45` 提供月份／项目／公司／类别／供应方／负责人／币种及人工FX选择，走共享金融契约。 |

## Paths walked / 判定与已排除的候选

- 配置保存→普通订阅命令→引用／项目锁→新配置／价格→事件与返回；正式状态／价格命令在同样事务复核原人工会话及MFA，不将前端按钮可见性当授权。
- 生成→原日历→准确价格→新账期及事件，或自然键已存在／价格未知；后台生成另经当前持有人／围栏、配置／控制版本复查。两条路径都没有创建actual_cost/payment。
- 费用关联→准确费用和项目锁→已有任意订阅账期去重→cost_links和费用/账期事件；复核→固定费用集合指纹→证据与差额承认；费用后来冲销时旧复核可失效。
- 财务请求→四种独立summary→同样的来源SQL→明细及可选固定FX；发生按规范费用、现金按实际资金与份额、预测按固定账期、摊销按准确独立计划，不合并为一个财务事实。
- “归档只看已有账期、不能证明未创建的历史不存在”不是本轮新缺陷：`docs/development/37-subscriptions-billing-and-forecast.md:27` 已明确规定此边界。当前归档入口检查与其一致；不能把未给定银行事实／未创建账期推定不存在。
- `FIN-12` 当前明确为按月份／项目／公司／类别／供应方／负责人／币种筛选并钻取（需求`:66`），不另发明必须增加任意多维透视表的首发要求。

## Searched / 检索记录

- `rg --files app/backend/internal -g '*subscription*' -g '*finance*' -g '*cost*' -g '*amort*' -g '*fx*' -g '*cash*'`：定位普通订阅／报表、摊销、FX及HTTP文件，随后阅读上表入口／执行／SQL。
- `rg -n '^func ' ...subscription_*.go`：定位normalize、读取、普通mutate、generate、reforecast、link、review、scheduler、leased job及提醒；读取各实际分支，未以函数名推定实现。
- `rg -n '^func ' ...report*.go ...cash_report.go ...forecast_report.go ...amortization*.go`：定位独立分派、聚合、来源、准确折算、摊销计划/发布；逐段阅读上表范围。
- `rg -n '^CREATE( OR REPLACE)? FUNCTION (can_read_subscription|subscription_charge_eligible|validate_subscription|validate_subscription_period|subscription_actual_fingerprint|validate_cost|guard_cost_amortization)' app/backend/migrations -g '*.sql'`：当前订阅权限／日历／指纹和订阅事件约束仍在0038；费用validate最新0041、双币结算最新0089，未把旧同名费用函数作为最新实现的完整证明。本次没有重审所有费用/退款/手续费写命令。
- `rg --files app/web/src -g '*Subscription*' -g '*Billing*' -g '*Report*'`及对应共享移动目录：找到并阅读上述当前维护、动作、列表和报表入口。长行／SQL曾被输出截断的内容，按更窄行段重读；未读取的尾段没有作为已执行证明。

## Open questions / 明确未证明的内容

1. 公司实际基础币种、报价来源／报价日、舍入和摊销制度、供应方停订证据与真实完整业务样本均待采纳／提供；代码存在人工明确选择能力不等于制度已生效。
2. 实际Codex/TG/云配置、worker目标安装和跨渠道送达、正式四端业务验收未在本次执行，也不由源码一致性证明。未重跑历史用例、APK、真实设备或性能专项。
3. 本次核对普通订阅和报告选定不变量；没有把财务超级用户任意篡改、未录入的外部扣款、全部退款/手续费/汇兑写入链或会计总账纳入已证明范围。可见报告的汇总/来源路径不等于外部经营数据已经正确。

下一步：当前没有确认需要加入168应用代码批次的GAP-03新差异；保留GAP-03开放状态及上述未给定制度／配置／正式验收。
