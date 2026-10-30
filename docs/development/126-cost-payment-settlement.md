# Increment126 · 费用付款核销闭环 / Expense to actual payment settlement

已确认费用详情增加“登记付款并核销费用”。员工明确填写已经发生的付款时间、实际总额、币种、收款方、正式凭证和本次核销额，核对后一次提交，进入普通资金详情及不可变分配历史。费用原额保持，实际资金和费用结算分别核算。

## 操作流程

1. 打开未冲销、所在项目可写的已确认原费用，选择新入口。需要本人交互会话、费用读取及资金读取／登记权限。页面重新取得当前费用版本、修订和摘要；项目归属和资料分类由服务从费用确定。
2. 填写实际付款香港时间（精确到秒）、实际币种和付款总额，独立填写本次核销付款额。只预填描述性标题、费用类别及币种，不自动填写实际日期、总额或核销额。
3. 供应方付款须明确选择费用同一供应方的准确资料版本。员工垫付报销须选择本笔费用已关联的当前准确批准报销，再明确选一条供应方相符的员工垫付明细；收款员工来自该报销的创建人。
4. 异币核销另填费用原币核销额、每1费用原币兑换付款币种的实际汇率和来源。服务按付款币精度复核舍入结果，并分别核对费用、报销明细和付款余额。同币核销不接受多余汇率输入。
5. 选择至少一份正式付款凭证的准确版本，填写实际付款方式、可选外部流水、原因及永久资金来源标识。明确核对实际付款与本笔核销依据，并完成真实近期两步验证；改变输入后两个声明重新核对。
6. 提交后进入普通已登记资金详情，核对已分配、未分配资金、费用原币、汇率及三条普通资金历史。费用详情和报表读取新结算余额，原费用发生额与业务批准金额分别保留。

| 独立场景 | 费用原额 | 实际付款总额 | 本次付款币核销 | 费用原币核销 | 未分配付款 | 费用待结 |
|---|---:|---:|---:|---:|---:|---:|
| 供应方部分付款 | HKD100 | HKD60 | HKD40 | HKD40 | HKD20 | HKD60 |
| 人工核对异币 | HKD100 | USD13 | USD6.50 | HKD50（汇率0.13） | USD6.50 | HKD50 |
| 批准员工垫付 | HKD100 | HKD30 | HKD25 | HKD25 | HKD5 | HKD75 |

三个独立场景的费用仍合计HKD300；实际资金为HKD90和USD13。实际现金不能把两种币种直接相加，也不能把核销重复计成新增费用。员工明细余额与费用余额共同限制核销。

## 接口与数据约束

| 接口 | 当前契约 |
|---|---|
| `GET /v1/costs/{id}/payment-basis` | 只读当前已确认原费用及快照时间，重新检查本人身份、完整费用读取和可写项目；不接受查询参数、不建立资金、不预留余额。 |
| `POST /v1/payments/from-cost` | 普通实际付款内容、准确`cost`、可选准确`claim`、`allocation_amount`、可选人工`fx`、永久`source_key`、原因和两个明确确认。成功201，返回`payment`及`allocation`。不接受客户端项目／分类覆盖或退款来源。 |

`cost`固定`id/version/revision_id/content_hash`。员工`claim`还固定当前报销版本、准确批准修订／摘要及`line_id`。预览和提交均重新授权；预览不是未来写入许可或完整全局余额证明。公开OpenAPI与四端类型见[契约](openapi.json)。

## 事务、幂等和审计

协调器以已有`Commands.Tx`依次执行普通`CreatePayment`、`RecordPayment`、`AllocatePayment`，最外层一次提交。每个普通子命令继续检查原身份、权限、正式凭证、供应方／员工、币种精度、实际付款时间、永久来源及金额限制。保留一份资金修订、三条资金历史、一条分配及正式凭证引用；三个普通命令和一个组合结果各有成功审计及Outbox事件。费用不新增、不重写原事实。

锁顺序沿用排序项目、可选批准报销、费用、资金。先锁定相关项目及报销，再核对准确费用头；不同操作争用同一费用余额仍使用原核销限额。最后一步失败会撤销先前资金、历史、凭证、成功审计／Outbox和内部／外部幂等记录。被拒绝请求的独立安全审计继续记录，不能误认为业务部分成功。

内部幂等标识由外层路由、标识和固定步骤派生。相同请求重放返回原资金与原分配，不重新核销；换请求体被拒绝。真实会话、近期MFA和当前权限在重放前检查，回执原金额固定，显示资料按当前权限重新投影。普通后续释放、退款和更正继续使用原页面及命令。

## 页面与恢复

Web与共享微信／抖音／Android均从费用详情进入新表单。移动文字草稿只存有限白名单输入，按本人授权及费用ID、版本、修订、摘要隔离；供应方选择、报销选择、明细、正式文件、MFA和确认声明不恢复。新请求体只由当前准确依据与明确选择构建。

四端共同核对完整组合回执：资金版本3、已登记状态、实际金额与日期、收款人、固定凭证、归属、未分配金额，以及准确分配的费用修订、员工依据、双币金额、汇率和份额。无法核对时停止连续提交，并要求按永久来源标识查阅现有资金，不能更换标识重复登记。移动沿用既有命令状态机。

## 交付边界

本轮不发起银行转账、不推断实际支付、不自动完成合同／采购履行或结项。公司制度、手续费／汇差口径、异币退款归属、完整正常／异常业务旅程及正式平台／云／模型／生产验收继续开放。没有结构升级，继续schema94，原迁移与已发布证据／媒体按原字节保留。

完整23项与五项移出见[当前范围](../delivery/04-current-scope-and-acceptance.md)，真实执行和媒体见[本轮核验](../quality/126-cost-payment-verification.md)。没有启用Docker、WSL、虚拟机或模拟器。

## English design and usage

Open a confirmed, unreversed expense in a writable project and choose the explicit payment-and-settlement action. The page pins the current expense version/revision/hash, derives owning scope and classification on the server, and leaves actual cash date, total and allocation blank. A human enters actual Hong Kong payment time, currency, total cash and the separate amount assigned to this expense, then selects the matching supplier revision or an exact linked approved employee-advance line. Foreign settlement additionally requires the expense-currency amount, actual conversion rate and source.

Choose formal fixed evidence, actual payment method and reason, acknowledge both actual cash and settlement facts, and verify yourself with recent MFA. One PostgreSQL transaction composes the existing ordinary create, record and allocate commands. All existing authorization, project/claim/cost locks, expense/claim/cash ceilings, currency precision, permanent source identity, audit, Outbox, history and idempotency rules remain active. A late failure rolls back every business write; independent rejection audit remains. Extra cash stays unassigned on the ordinary payment rather than becoming another expense.

Web and shared WeChat/Douyin/Android validate the full compound receipt, including exact cash, payee, evidence, pinned cost/claim, dual-currency allocation and residual cash. Mobile drafts restore bounded text only, isolated by current authority and exact expense basis. Evidence, payee/claim selection, MFA and acknowledgements require fresh review. Unknown outcomes stop repeated creation and direct the human to the permanent source identity. No bank transfer, automatic business completion, new schema upgrade or formal production acceptance is implied.
