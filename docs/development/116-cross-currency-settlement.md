# 增量116：实际付款的双币核销 / Reviewed cross-currency settlement

增量121当前补充：异币退款已接通未分配、未预留原付款本金的实收登记和全额冲销；原分配的费用／合同异币回转仍待完成，不混用三种币种。普通同币退款沿用原协议，真实费用／现金／手续费依据不改写。详见[异币实收设计](../development/121-actual-foreign-refunds.md)；范围按[修订99](../delivery/04-current-scope-and-acceptance.md)。

## 业务约束 / Business rules

本轮把真实付款与原币费用连接起来，不执行银行转账。付款金额、现金报表及付款币项目份额使用付款币；费用、报销和关闭检查使用费用原币。原始事实、已完成导出和历史凭证不改写。

An explicit human allocation connects actual cash to an incurred cost. Cash stays in payment units; expenses and employee claims stay in cost units. This operation never initiates a bank transfer or rewrites original facts or completed exports.

- 同币种沿用原协议，不接受汇率。跨币种必须输入费用原币金额、实际汇率、凭证来源并明确确认；汇率方向固定为「每1费用币兑换多少付款币」，以付款日固定依据。
- `round(cost_amount × rate, payment_exponent) = amount`，十进制定点计算，最后舍入一次。付款本金必须匹配；手续费须有独立真实费用和现金依据，不自动把差额当手续费或汇兑损益。
- 每条核销保存两套金额、对齐的两套项目份额及原报价。费用原币余额与付款币余额各自守恒，禁止超付、超额核销、越权或使用陈旧版本。
- 释放或退款按原正向分配**当前**双币剩余额度等比例回转，在费用币精度舍入。最后一次用尽准确原币尾差；小额回转允许费用币舍入到零。客户端须展示并回传核对后的费用原币金额，服务端加锁后重新计算，变化时拒绝。
- 实收退款仍与原付款同币种。退款冲销及付款全额冲销逐条复制相反方向的双币份额及报价，保留历史。
- 员工收款、批准报销行、供应方、准确费用关联、项目／分类权限、真人会话、近期MFA、审计与幂等约束全部继续生效。

## 数据与接口 / Data and API

Schema89 在不可变核销账本及份额增加 `cost_amount`，在账本增加 `fx_rate / fx_source / fx_quoted_on`。旧同币记录按原额填充第二单位，其他字段原样保留。普通数据库升级属于当前范围。

`PaymentAllocate.fx` 包含 `cost_amount / rate / source / acknowledged`。跨币释放、退款分配及登记退款内的分配项额外传 `cost_amount`。现有路径和同币请求不变。新读取返回两种币种、精度、份额、剩余额和报价；为兼容不可改写的旧工具回执，OpenAPI中的新增响应字段为可选，跨币操作必须取得完整当前记录。

Schema89 adds the cost axis and immutable quotation to allocations, preserving original ledger fields. Existing endpoints remain. New response fields are optional only for historical archived receipts; current cross-currency writes require complete, freshly read dual-unit facts.

## 当前边界 / Remaining scope

本轮不选择公司报告汇率账簿、不计算会计汇兑损益、不自动登记手续费，也不支持退款实收币不同于原付款。公司财务口径采纳、费用／汇差完整链路和 GAP-03 的最终验收继续开放。四端入口均需实现并验证；本地检查不代表正式平台、云环境或生产验收。

Company accounting policy adoption, fees and exchange differences, refunds received in another currency, and full GAP-03 acceptance remain open. AI fee controls remain excluded; AI authorization and technical limits remain required.

## 操作与验收口径 / User workflow

| 编号 | 操作 | 页面及数据结果 |
|---|---|---|
| FX-01 | 登记实际付款，并确认准确费用 | 付款和费用各自有真实币种、金额、凭证版本及当前修订。登记付款不自动确认费用，也不发起银行转账。 |
| FX-02 | 在付款详情选择「分配费用结算」 | 通过有权限的实际记录选择器选定费用；员工收款先选择已经批准、符合收款人和费用关联的垫付行。不能凭手填ID跳过读取权限。 |
| FX-03 | 输入两种本金及汇率依据 | 付款币本金、费用原币核销金额、实际汇率、凭证来源均须核对。原币不同才显示汇率输入；同币种不接受额外汇率。 |
| FX-04 | 明确确认并提交 | 原有真人、近期MFA、幂等键及准确版本约束继续生效。表单先检查金额；服务端在当前权限及锁内重新核对，不信任客户端计算。 |
| FX-05 | 查看不可变核销流水 | 同时显示付款币金额、费用原币金额、两种剩余额、原汇率及两套对齐项目份额。原付款与费用内容修订不会因此改写。 |
| FX-06 | 释放或按原付款币登记退款 | 选择原正向分配，输入本次付款币金额，核对显示的原币回转额及原凭证来源，再确认。释放只撤销核销关系，不产生银行退款。 |
| FX-07 | 处理结果不明或余额变化 | 先刷新付款、原分配和相关费用，核对是否已经落账；使用当前依据重新确认。不得通过修改幂等键或盲目重试掩盖未知结果。 |

四端共享同一金额规则和服务端契约。微信、抖音小程序继续承担查阅、审批和适配的业务操作；本轮移动页面证据使用Taro H5，Android检查使用开发构建，不代表正式平台上架。

The four clients share the same decimal rules and backend contract. Select actual authorized records, review both amounts and provenance, then confirm with recent MFA. A release removes a settlement relationship; it does not move money. Refresh authoritative records before retrying an uncertain result.

共享移动财务操作在构造本地请求时，将刻意抛出的输入校验消息转换为页面可显示的财务输入错误。例如汇率7.7与HKD780不符时，员工能看见具体金额关系提示。此转换只发生在同步财务输入边界；未知运行时错误仍使用通用提示，网络错误继续由既有请求路径处理。

Taro实际构建会把BigInt的幂运算改写为不兼容BigInt的`Math.pow`。共享计算因此使用有界整数乘积构造币种步长，保持整数精度，并以实际编译后的H5业务流程检查报价、写入及回转。Android额外检查包内没有本次发现的错误改写形式；此静态检查仍不能替代设备执行。

## 金额实例与尾差 / Amount examples

| 编号 | 原始事实／本次操作 | 付款轴 | 费用轴 |
|---|---|---|---|
| FX-E01 | USD100费用，HKD780付款，实际汇率7.8 | 核销HKD780 | 核销USD100 |
| FX-E02 | 基于E01释放HKD390 | 剩余HKD390 | 回转USD50，剩余USD50 |
| FX-E03 | 再释放剩余HKD390 | 剩余0 | 使用准确剩余USD50，剩余0 |
| FX-E04 | 将E01的汇率误填7.7 | HKD770与输入HKD780不符，拒绝 | 费用已付额不变 |
| FX-E05 | 极小部分回转在费用精度舍入为0 | 仍记录实际付款币回转额 | 明确记录0，后续最终回转取尽准确尾差 |

反向核销使用当前剩余双币比例，不重新套用原始汇率逐笔转换。这样在多次部分释放、独立退款及整数币种舍入后，两套累计余额仍分别守恒。费用原币可能先归零而仍有付款币余额；存在有效结算关系时继续禁止冲销该费用。

项目份额分别按各自币种精度分配，使用确定的最大余数规则。两套数组按相同项目与顺序对齐；某项目在一种币上舍入为0、在另一种币上为正时保留该行，避免丢失原币项目归属。全额冲销复制原行的两套准确份额。

## 请求与回执约束 / Request and receipt contract

下例只展示核销请求中新加入的报价片段；实际请求还须包含当前付款／费用修订、金额、操作依据等既有必填字段，以及正常鉴权和幂等头。

```json
{
  "amount": "780.00",
  "fx": {
    "cost_amount": "100.00",
    "rate": "7.8",
    "source": "已核对的实际付款凭证",
    "acknowledged": true
  }
}
```

- 金额以十进制字符串传输；实际币种精度由服务端币种定义决定。当前表单提供HKD、CNY、USD、EUR、GBP及JPY，不能用浮点数累计金额。
- 汇率必须为正，最多14位整数、10位小数；方向固定为付款币／费用币。来源须满足正常名称长度及字符规则，日期取实际付款的既有付款日。
- `amount / shares / remaining_amount`始终为付款币；`cost_amount / cost_shares / remaining_cost_amount`始终为费用原币；`fx`保存固定汇率、来源及`quoted_on`。
- 反向请求的`cost_amount`是本次重新核对后的回转额，不能省略跨币依据。服务端按锁内当前余额重算，不一致返回`SETTLEMENT_BASIS_CHANGED`；超过付款币余额返回`SETTLEMENT_BALANCE_EXCEEDED`。
- 历史不可改写工具回执可能没有新增字段；响应契约保留兼容性。客户端不得把缺失字段当作0、默认同币或伪造当前报价，须重新取得完整实际记录。
- 既有幂等回放仍须通过当前会话、权限及近期MFA。独立退款可能改变原分配余额而不改变原付款内容，必须重读当前根分配并核对双币，不能只比较付款版本。

## 实现位置与维护 / Implementation map

| 编号 | 位置 | 维护责任 |
|---|---|---|
| FX-D01 | `app/backend/internal/finance/payment_fx.go` | 报价与反向金额精确计算；使用整数／有理数，最后在目标币精度舍入一次。 |
| FX-D02 | `payment_allocations.go`、`payment_fx_shares.go`、`payment_shares.go` | 根分配锁、双币项目份额、当前余额复核、不可变账本。 |
| FX-D03 | `app/backend/migrations/0089_cross_currency_settlement.sql` | 正常schema升级、历史同币补齐、数据库侧金额／报价／根引用／份额约束及原币投影。 |
| FX-D04 | `app/packages/contracts/src/payment-fx.ts` | 四端共用BigInt规则、完整当前依据检查与实际回执校验。 |
| FX-D05 | `app/web/src/PaymentActions.tsx`、移动`pages/finance/operations.tsx` | 人工选择、明确确认、MFA、双币回转预览及结果核对。 |
| FX-D06 | `app/scripts/verify_payment_fx_*`与`run_payment_fx_browser.py` | 当前源码绑定、正常升级、实际页面及资源关闭检查；证据保存在`docs/quality/evidence/payment-fx*`。 |

发布前应保持旧迁移内容不变，并通过新迁移升级普通数据库。数据库升级校验分别比较原现金、费用及只读历史AI费用事实；不能以空新库建表成功代替已有账本升级检查。开发者不得在源码绑定检查运行时修改参与计算的来源文件。

Backend rational arithmetic, shared client BigInt calculations and independent database constraints protect the same two-unit invariant. Normal upgrades preserve populated prior ledgers. Current-source tests, prior-version upgrade checks, reused media and cached JVM results remain separately labelled.
