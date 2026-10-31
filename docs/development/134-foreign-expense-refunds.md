# 增量134：异币退款费用回转 / Foreign-currency expense refund attribution

本轮把已登记的异币实收退款接回原费用结算，支持登记时一次组合回转，也支持登记后分次追加。Web、微信、抖音和Android共享协议与准确金额计算。首个冻结候选`v0.1.0-rc.1`仍固定在增量133；本轮是其后的main开发源码，不修改标签或既有安装包。

This increment attributes recorded foreign-currency refunds back to their original expense settlement, either atomically when recording cash or in later partial commands. All four clients share exact monetary rules. The first frozen candidate remains at increment133; this is subsequent main development, with no replacement of its tag or artifacts.

## 业务金额 / Monetary facts

三种币种可以同时存在。原实际费用、原付款及实收退款保持各自原始事实，回转只追加不可变分配。

| 事实 / Fact | 示例 / Example | 金额用途 / Purpose |
|---|---|---|
| 原费用 / Expense | 200 EUR；项目150、公司50 | 费用及员工报销余额 |
| 原付款 / Outgoing cash | 1,000 HKD；其中780 HKD核销200 EUR | 付款本金与可用余额 |
| 原核销依据 / Original settlement | 每1 EUR兑换3.9 HKD | 保留准确原分配、日期与凭证 |
| 实收退款 / Actual refund | 100 USD；固定退回原本金780 HKD | 银行实收原币现金 |
| 第一次费用回转 / First inverse | 560 HKD →143.59 EUR；实收份额71.79 USD | 三种单位分别守恒 |
| 最后一笔 / Final inverse | 220 HKD →56.41 EUR；实收份额28.21 USD | 准确耗尽本金、费用与实收尾差 |

首次回转后原费用未结算额为143.59 EUR；全部回转后恢复为200 EUR。原费用仍为200 EUR，银行原付款仍为1,000 HKD，整笔实收仍为100 USD。完整现金项目份额为75 USD与公司25 USD，不把780 HKD当作780 USD。

The expense remains200 EUR, original cash1,000 HKD and actual refund100 USD. Reversal allocations change settlement balances only. Project cash reports ultimately attribute75 USD to the project and25 USD to company scope. No amount combines different currencies.

## 四端操作 / Four-client operation

1. 从原付款查看准确的原正向费用分配，确认付款币、费用原币及剩余双币金额。
2. 登记真实异币退款：固定原付款、实收金额／币种、退回原本金、实际汇率及凭证来源。
3. 可在登记时选择原分配并输入**原付款本金**；有已分配资金时，原付款剩余额度要求相应费用回转与实收登记同事务完成。
4. 后续在退款详情选择“关联退款分配”；选择准确原分配，输入原付款本金，并核对费用原币回转与实收现金份额。
5. 人工声明、当前权限与最近MFA通过后提交。页面重新读取持久流水及回执，金额或来源不一致时要求重新核对。
6. 资金事实有误时执行原有全额冲销；追加反向资金及三币种分配，原历史保留。不会发起银行转账。

On the original payment pin the exact positive expense allocation. Record actual received cash and its reviewed principal quotation. Select original allocations at record time or use “关联退款分配” later; enter original-payment principal and review the expense/cash preview. Current human authority and recent MFA are mandatory. Inspect durable history, or use full cash reversal to append inverse evidence.

## 金额算法与存储 / Exact arithmetic and persistence

- `payment_allocations.amount`、`shares[].amount`继续使用原付款本金；费用保持`cost_amount`及`cost_shares`，原核销汇率不改写。
- 新增可空`refund_cash_amount`及不可变`refund_fx_acknowledged`；新外币腿的`refund_cash`响应提供本条实收份额、整笔实收、固定本金／实际汇率和对应项目份额。
- 下一条实收份额=本次原本金×剩余实收现金÷剩余原本金，按实收币种精度四舍五入；最后一条取得准确剩余量。支持显式零最小单位现金份额。
- 项目实收份额按本条原本金项目权重使用最大余数法，余数相同按原位置；项目数、顺序、范围和金额须匹配。费用／本金份额仍遵循原核销与原回转算法。
- 付款余额按本金、费用余额按费用原币、现金报表及其估值／导出按实际实收份额核对。没有新汇率推断、自动汇差或自动手续费记账。
- `refund_allocated_principal`明确提供已回转本金；历史幂等回执同时保留当时本金与现金快照，不能混入后来的冲销余额。
- 迁移101增加字段、替换明确需要的原守卫并移除旧一律拒绝外币费用回转的触发器；迁移1–100及既有账本字节不变。已发布旧响应的新增可选字段不变为必填。

Allocation principal and expense units retain their original meanings. New immutable optional cash columns identify actual refund cash separately. Exact rational arithmetic rounds each remaining-ratio cash attribution; the final principal consumes the exact cash remainder. Project cash shares use largest remainders. Old journal rows are not rewritten; nullable cash columns preserve legacy calculations. Migration101 replaces the prior blanket rejection with precise principal/cost/cash constraints.

## 授权与一致性 / Authority and atomicity

继续使用项目→员工报销→费用→资金的既有排序锁，核对组织、准确原付款／正向分配、原修订、供应方／员工、分类及访问范围。原分配剩余本金、费用余额、退款固定本金和实收各自限额；并发命令、重放和审计失败不产生半条业务。数据库校验人工声明、原币精度、准确比例、项目份额和冲销复制。秘书未新增财务写入工具，AI费用控制仍移出。

Existing ordered locks, current organization/scope authority, human sessions, MFA, idempotency and same-transaction audit remain required. Database checks independently enforce quotation, minor units, exact shares and reversal copies. No new secretary financial write tool is introduced.

## 本轮交付边界 / Delivery boundary

实际完成选定原生HTTP／PG64条、Web436项、共享移动521项及Web／两小程序构建；仅Web重录本轮实际操作。保留旧冻结包与Android开发APK，不把它们称为本轮新构建。持久开发库仍schema100；部署本轮源码前须正常应用101并重启对应服务。

Receivable-contract foreign outgoing returns, adopted company FX/difference policies and full business journeys remain open. The persistent development database remains schema100; apply101 normally before running this new backend. No fresh Android APK, full backend/runtime regression or mobile browser acceptance is claimed. See [检查与延期](../quality/134-foreign-expense-refunds-verification.md) and [全部23项余项](../delivery/05-functional-closure-134.md).
