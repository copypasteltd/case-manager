# 增量106：费用摊销计划 / Versioned cost amortization

本轮推进 GAP-03 的可选费用分期能力。Web、微信、抖音与 Android 共用原费用、不可变计划版本和独立摊销报表。公司仍需明确选用规则；跨币种核销、手续费／汇差、完整财务旅程及生产接入继续开放。当前23个工作包、14 P0／9 P1及5项范围排除保持不变。

This increment adds optional, explicitly published expense allocation schedules across four clients. Each plan pins a confirmed expense and conserves its original project/company allocations. Cross-currency settlement, exchange fees/differences, company policy adoption and full financial workflow acceptance remain open.

## 1. 使用流程 / Human workflow

| 步骤 | 操作与约束 / Operation and constraint |
| --- | --- |
| 1 | 正常升级数据库至迁移83；在权限管理中向适合的员工授予“费用摊销管理人”（finance_amortization_manager）及实际业务范围。基础权限为cost.read、cost.amortize和report.read；项目访问及报表导出沿用独立授权。升级不自动向员工授予角色，不为旧费用生成计划。 |
| 2 | 打开已确认、未冲销的原费用，选择“查阅摊销计划”→“编制或停用摊销计划”。draft仅查阅，冲销记录本身不可编制；原费用被冲销后可查阅旧计划并停用。 |
| 3 | 明确选择月份均分或按实际服务天数分配，填写服务起始日、结束日和操作依据。日期为香港业务自然日，结束日不含；没有默认规则、期间或币种。 |
| 4 | 选择“预览月度摊销”，检查确认日期、原币金额、公司与项目份额。预览只记录审计，不发布计划、不改原费用版本。修改输入后须重新预览。 |
| 5 | 确认重列当前计划口径的声明，完成最近十分钟内的两步验证，再选择“确认发布摊销计划”。请求固定费用版本、修订、内容摘要、当前计划版本和预览SHA-256。 |
| 6 | 在不可修改的历史中查阅作者、时间、原因、SHA-256及完整月份／份额。发布修订需重新预览、核实；停用也新增版本并保留此前内容。 |
| 7 | 在财务报表明确选择“费用摊销”，选择期间、项目或公司范围、币种和其他筛选。可另选固定人工汇率版本折算；导出沿用独立权限、异步任务、24小时有效期和重新鉴权。 |
| 8 | 费用、权限、项目或计划冲突时刷新费用与计划后重新核对。结果不确定时输入锁定，原样重试沿用同一幂等标识；也可刷新历史核对，不据超时判断失败。 |

Open a confirmed original expense, inspect its plan, explicitly choose a method and half-open service interval, preview every share, complete recent MFA, acknowledge the report basis, then publish. Revisions and stopping create immutable versions. Choose **费用摊销** in reports; source links return to the original expense. Reconcile uncertain responses using the same request or version history.

## 2. 报表口径边界 / Meaning of this view

实现的是**按最新有效计划生成的费用分期视图**，basis=latest_active_schedules。没有计划或最新版本为stopped的费用不进入此视图。发布修订或停用会重列当前视图的整个服务期间；它不是冻结的会计凭证账簿，也不生成付款、冲销原发生额或替公司选择财务制度。

历史计划及其月份／份额不可修改。已完成导出固定请求、筛选、计划／原费用来源和快照，后续计划变化不改文件；下载仍重新检查授权和有效期。每次读取用一致事务生成汇总和本页明细，翻页按当时最新计划查询；需要跨页固定结果应生成导出。

The live report is a restatable schedule view, not a posted ledger. Replacing or stopping a plan changes its current service-period view. Immutable history and completed export bytes stay fixed. Each page is internally consistent; pagination reads current facts again. Use an export for a frozen result.

## 3. 日期、金额与舍入 / Calendar and arithmetic

| 规则 | 定义 |
| --- | --- |
| 服务区间 | 左闭右开；1900至2200年内有效ISO日期、跨度不超过3660天、最多120个相交月份。 |
| 月度分段 | 与自然月边界相交的连续区间；每段确认日期为结束日前一天。不足月的服务期仍占一段。 |
| monthly_equal | 所有相交月份同权，包括首尾不足月；员工须明确接受该规则。 |
| daily_weighted | 按每段实际服务天数加权，闰年和不足月使用真实日历。 |
| 原分摊 | 每个原项目／公司份额分别计算，保留原币定义的0至6位精度；全部月份合计恰等于该份额。 |
| 累计舍入 | 原份额最小单位数为A，截至第k段累计权重Wk，总权重W；累计额=floor(A×Wk/W+1/2)，本段额=本次累计额−上次累计额。只用整数／有理数。 |
| 月度总额 | 当月各原份额已舍入金额求和；不另算总额再强塞尾差。允许零金额月份，其空份额列表保留。 |

例：HKD100分为项目60、公司40，1月1日至4月1日。月均法项目每月20，公司依次13.33、13.34、13.33，整笔每月33.33、33.34、33.33。改为按日法须重新预览、发布；规则不会自动替换。

All arithmetic uses integer minor units and exact rational weights. Cumulative half-up rounding runs separately for each original allocation. Monthly totals sum those rounded shares. This prevents a negative final-month adjustment and preserves every project's amount over120 months.

## 4. 费用冲销与汇率 / Reversal and FX

原费用在R日全额冲销时，保留确认日期严格早于R的月份，并在R日逐期生成负向报表项；R日及其后的月份不再计入。负项保留原计划版本、月份和原确认日期。确认日恰为R的月份不先生成正负两条，没有先前月份时不虚构负额。

例如1月31日计划33.33，2月15日冲销：1月保留+33.33，2月15日记录−33.33，2月底、3月底的未来计划排除。

折算需明确选择不可变人工汇率版本。正项用原确认日期，负项沿用相同日期选报价；即使冲销日没有报价，也准确抵回。缺少原月份报价仍显示缺口。项目／公司筛选先确定贡献，再按既有逐条折算规则舍入；不同范围折算相加可能有尾差，原币份额始终守恒。

摊销明细的 from 是本次确认或冲销的业务日期，用于报表期间筛选；to 保留原服务段的结束日。冲销发生在服务段结束后时，to 可以早于 from。完整服务段以 amortization.service_from／service_to 为准，原确认及选报价日期以 amortization.original_date 为准；接口消费者与CSV使用者应按这些字段各自的含义读取。

For amortization sources, from is the recognition/reversal business date and to retains the original service-segment end, which may precede a later reversal. Use amortization.service_from/service_to for the service interval and amortization.original_date for the original recognition and FX quotation date.

## 5. 持久化与接口 / Persistence and API

| 对象 | 内容与保护 |
| --- | --- |
| cost_amortization_versions | org/cost/version唯一；active或stopped；规范化内容文本及SHA-256；员工、交互会话、实际MFA时间、原因和创建时间。计划不递增原费用版本。 |
| cost_amortization_entries | 1至120段；独立正项／冲销项ID；服务区间、确认日、金额。 |
| cost_amortization_shares | 每段最多100个正金额项目／公司份额；零额份额省略。 |
| 数据库保护 | 三表禁止更新／删除；子记录仅在创建父版本的同一事务写入；延迟约束检查完整月份、内容、连续日期、总额及原分摊守恒。发布绑定准确已确认费用，停用保留此前内容。 |
| API保护 | 当前身份、业务可见性、cost.amortize、人工会话、MFA、项目可写状态；项目→费用顺序加锁；预览摘要与版本控制；数据／审计／Outbox／幂等回执原子提交。 |

下列路径以 **/v1/costs/{id}/amortization** 为前缀。

| 方法／后缀 | 返回与用途 |
| --- | --- |
| GET 无后缀 | 当前版本、完整内容和can_manage。读取失败时不继续把旧缓存展示为当前事实。 |
| GET /versions?limit=10&cursor=... | 有界签名历史分页，游标绑定身份／授权及费用。 |
| GET /versions/{version-id} | 指定不可变版本及完整内容；参数为版本UUID。 |
| POST /preview | 费用Base＋expected_plan_version＋method＋service_from/to；返回预览版本和摘要。 |
| POST 无后缀 | 同预览输入＋preview_sha256＋acknowledged=true；最近MFA确认后发布。 |
| POST /stop | 费用Base＋expected_plan_version＋reason＋acknowledged=true；最近MFA确认后新增停用版本。 |

GET拒绝未知／重复参数，POST沿用严格OpenAPI、1MiB上限、CSRF和幂等标识。读取、写入、历史与回执重放都重新检查权限。AI运行或TG身份不能调用这些人工命令；本轮不扩展秘书工具集。

## 6. 四端与验证 / Clients and validation

Web在原费用详情提供月度表格、展开份额、历史弹窗和人工表单。Taro在原财务页内提供同一流程，逐期展开份额；微信、抖音、Android共用输入与回执校验。没有新增页面路由，没有启用模拟器。

计算测试覆盖闰月、单日、不足月、120个月、零月额、不同货币精度及逐项目守恒。原生PG／HTTP测试覆盖权限、MFA与重放、审计回滚、预览、并发发布、不可变记录、延迟完整性约束、历史、停用、范围报表、导出固定和冲销报价日。客户端检查篡改／跨费用／过期预览、来源定位和精确金额。

实际计数、源码摘要、截图、静音录屏／GIF和Android产物范围见[本轮验证](../quality/106-cost-amortization-verification.md)。Web／Taro H5合成演示及开发APK不代表平台审核或生产上线。不做真机、模拟器、网络性能、实网120GB、历史迁移／容灾或AI费用控制。

## 7. 关联源码 / Source map

- 计算与服务位于app/backend/internal/finance/amortization系列文件；迁移为0083_cost_amortization.sql；路由为internal/httpapi/finance_amortization.go。
- 共享校验位于app/packages/contracts/src/amortization.ts；Web为CostAmortization.tsx；Taro为pages/finance/amortization.tsx。
- 报表／导出沿用report系列Go文件和共享finance-reports.ts；合约在[OpenAPI](openapi.json)，由app/scripts/update_amortization_contract.py维护并生成类型。
- [财务需求](../requirements/05-finance-and-subscriptions.md) · [当前范围](../delivery/04-current-scope-and-acceptance.md) · [全部未完成项](../delivery/03-goal-gap-assessment.md)。

## 编译目标兼容 / Compiled-target compatibility

真实Taro H5页面发现旧目标把BigInt幂运算转换为Math.pow，导致已成功返回的预览在客户端校验失败。最终实现使用整数乘法构造精度单位；回归将完整校验模块编译至ES2015后，在0至6位币种精度下实际执行，并重新检查四端构建及Web／H5页面。失败页面、旧源码和主动停止的未完成Android构建保存在本轮attempts/06证据中，不计为最终通过。

The receipt validator uses integer multiplication because older compilation targets lower exponentiation to Math.pow, which rejects BigInt. A regression executes the full ES2015-compiled validator at every supported currency precision; final browser and build evidence is captured after the fix.
