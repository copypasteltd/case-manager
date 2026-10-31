# 增量133：合同财务闭环与异币退款本金归属 / Contract finance and foreign refund principal

本轮补齐共享移动端的合同财务中心，并让已登记的异币退款按原付款本金归属至应付合同。Web、微信、抖音及 Android 使用同一协议。合同归属说明现有资金的用途；操作不会发起银行付款，不生成第二份实际费用。

This increment adds the shared mobile contract finance center and attributes a recorded foreign-currency refund to a payable contract in original-principal units. Web, WeChat, Douyin and Android share the protocol. Attribution explains existing facts; it does not transfer money or create another expense.

| 编号 | 可使用功能 / Available behavior | 约束 / Constraint |
|---|---|---|
| CF-01 | 生效／完成／终止合同的节点概览 / Milestone overview | 计划、费用、净收付、差额及核对状态分别展示；权限不足显示未知 |
| CF-02 | 真实费用、实际收付款归属 / Expense and cash attribution | 固定来源版本、修订和摘要；费用保留生效合同依据 |
| CF-03 | 原本金退款归属 / Refund principal attribution | 先固定该节点正向资金归属，再选准确退款；供应方、项目、分类和原付款保持一致 |
| CF-04 | 异币人工确认 / Explicit foreign-unit review | 额外声明只归属合同币种本金；现金和实际汇率保留原修订 |
| CF-05 | 部分解除与历史 / Partial release and history | 追加记录，不修改银行事实和原归属；未解除余额及当前有效金额分别展示 |
| CF-06 | 固定凭证节点核对 / Evidence-bound reconciliation | 人工确认条件、差额和未归属退款；来源变化后原核对变为须重新核对 |
| CF-07 | 并发、重放、权限与回滚 / Concurrency and atomicity | 合同、来源排序锁、双重上限、幂等、当前权限、近期MFA和同事务审计 |
| CF-08 | 全额现金冲销 / Full cash reversal | 原归属和旧核对保留，当前有效退款归属归零，净合同收付恢复；不会补记新现金 |

## 金额逻辑 / Units

示例：原实际付款 **780 HKD**，银行实际退回 **100 USD**，已登记凭证固定退回本金780 HKD及实际汇率。可分别将195 HKD本金归属到合同，最多合计780 HKD；每条归属仍明确显示**整笔实收100 USD**，不会声称这195 HKD对应100 USD部分实收。

Example: a780 HKD payment receives100 USD. The immutable refund fixes780 HKD principal and the reviewed actual rate. Four195 HKD contract attributions exhaust that principal. Each retains the entire100 USD cash evidence, clearly labelled as a whole fact, rather than inventing per-attribution cash conversions.

节点`amount`、归属`amount`、`remaining_amount`和`effective_amount`始终使用合同币种。`refund_cash`提供整笔准确实收，内含原`refund_fx`、现金币种／金额／业务日期。费用原币核销和项目现金份额继续独立；合同归属不会把未分配现金自动计入项目费用份额。

Milestone and attribution amounts always use the contract currency. Optional `refund_cash` retains whole received currency, amount, business date and the immutable refund quote. Expense settlement and project cash shares remain independent.

## 操作顺序 / Operation

1. Web打开合同详情的“合同财务归属”；移动端打开生效、已完成或已终止合同，选择“合同财务归属与核对”。
2. 先“归属实际收付款”，选择相同范围和往来方的已登记付款并输入合同币种金额。
3. 在原付款详情登记真实异币退款；固定实收凭证、原本金和实际汇率，人工核实后登记。
4. 回到合同选择“归属实际退回款”，选择本节点原正向资金，再选择退款；输入**合同原币本金**。异币时须额外确认金额单位。
5. 打开“查看节点归属历史”查阅整笔实收和归属本金。若范围分配有误，可“解除部分归属”；修改银行事实须另走资金冲销。
6. “核对节点财务”选择准确凭证，明确检查条件、计划差额和未归属退款，完成本人验证后提交。新退款、解除或冲销会使旧核对不再代表当前情况。

On Web use the contract’s finance section; on mobile enter the contract finance center. Pin outgoing cash, record the actual foreign refund from its payment, then select the original attribution and refund. Enter contract principal and explicitly acknowledge the units. Inspect history, append partial releases when attribution is incorrect, and reconcile the current milestone with fixed file versions. Source changes invalidate current reconciliation while preserving the original review.

## 接口、数据库与授权 / Protocol, storage and authority

- 延用`POST /contracts/{id}/finance/entries`；仅异币退款增加`refund_fx_acknowledged: true`。旧同币协议及旧核对快照不增加必填字段。
- 迁移100仅添加默认false的不可变声明字段、受控放开合同本金归属，并更新总量检查。原93号迁移及全部历史账本不改写；费用异币退款的独立拒绝约束继续保留。
- 合同按原付款本金核对“退款剩余总量”和“原正向归属可退总量”。解除仍按合同币种执行，不进行重复汇率舍入。
- 固定历史额外提供`refund_cash`；存在异币来源的新核对快照提供`attribution_currency`和整笔实收。没有异币来源的旧快照表达保持相同。
- 继续要求当前`contract.reconcile`、来源读取／记录权限、人类会话及近期MFA。配置管理员权限不等同于公司业务读取权限，秘书没有新增财务写工具。
- 跨币费用回转、收款合同的异币向外退回、汇兑损益／手续费会计制度仍开放。这里使用已登记的原本金事实，不选定公司会计政策。

The additive schema100 preserves all historic migration bytes and journals. Current human authority, recent MFA, exact source revisions, ordered locks, idempotency and atomic audit remain mandatory. Foreign expense inverses, foreign outgoing returns on receivable contracts, adopted exchange-difference/fee accounting and full company acceptance remain open.

检查、源码及演示见[本轮验证](../quality/133-contract-finance-verification.md)。[全部23项余项](../delivery/05-functional-closure-133.md)继续区分功能代码与真实集成／正式发布；五项用户移出内容不重新成为门槛，八项Skill仍draft。
