# 增量135 · 应收合同异币向外退回 / Receivable foreign returns

本轮补齐应收合同收到款项后，用另一币种实际向客户退回的财务归属。四端使用同一服务与授权；只解释已经登记的真实资金，不发起银行转账，也不自动记汇兑损益。首个冻结候选 `v0.1.0-rc.1` 的源码标签、产物分支和包保持固定；135为main后续源码。

## 操作闭环 / Workflow

1. 应收合同完成普通审批、签署、生效，节点币种和应收计划固定。
2. 在收付款台账登记实际收入，并将相同币种收入归属到合同节点。
3. 另行登记已向同一客户实际付出的资金；项目、往来方、分类、原件凭证和付款日期仍走普通规则。应收退回使用普通 `outgoing`，不冒充供应方退款 `refund_of`。
4. Web合同详情进入“合同财务归属”；微信／抖音／Android进入“合同财务归属与核对”。选择“归属实际退回款”，先固定本节点原收入归属，再选择实际向外付款。
5. 输入合同币种本金，以及本次实际付出归属金额、每1合同币实际兑换的付出币金额、凭证来源。逐项核对后明确确认，完成近期MFA再提交。
6. 在节点历史读取固定来源修订、合同本金、本条付出、整笔原资金和尚占用金额；可追加部分解除。节点人工核对仍需要固定版本凭证、条件及差额声明。
7. 资金全额冲销保留旧记录，当前合同净收付及现金报表按现有普通冲销规则恢复。原收入不能在尚有有效退回时先冲销。

The human pins an effective receivable milestone and its original incoming-cash attribution, then selects already recorded outgoing cash. Review contract principal, attributed actual cash, actual rate and evidence source; acknowledge and complete recent MFA. History and reports retain both units. A release detaches attribution only; it does not reverse the bank payment. Actual cash reversal preserves history and invalidates current reconciliation.

## 单位、金额与尾差 / Monetary invariants

| 编号 | 口径 | 规则 |
|---|---|---|
| 135-01 | 合同本金 | `amount`、节点净收付、原收入可退上限及解除输入始终使用合同币种 |
| 135-02 | 实际付出 | `return_fx.cash_amount`使用所选资金币种，按币种最小单位；本金乘汇率半入舍入后必须相等 |
| 135-03 | 原资金总额 | 原付款修订／摘要／实际日期及总额固定；可将一部分归属合同，剩余仍可按普通规则使用 |
| 135-04 | 资金占用 | `contract_cash_return_reserved`与项目现金份额按实际付出币金额计数；不能重复用于费用或其他退回 |
| 135-05 | 部分解除 | 按“本次解除本金 × 尚占用付出 ÷ 尚未解除本金”计算；最后一笔收尽准确尾差，可能有零付出小额解除 |
| 135-06 | 不可变核对 | 新的核对快照固定两种金额和汇率；解除、来源变化或冲销使当前核对过期，旧快照保持 |

算例：先将780 HKD收入归属应收节点，选100 USD向外付款。本金390 HKD、实际付出50 USD、汇率0.1282051282，节点净收390 HKD，资金尚占用50 USD、可用50 USD。再归属195 HKD／25 USD后，节点净收195 HKD，资金占用75 USD、可用25 USD。分三次解除原390 HKD归属、每次130 HKD，释放16.67、16.67、16.66 USD，共50 USD，不凭浮点运算丢失尾差。

Example:780 HKD received and100 USD paid out remain separate facts. Attribute390 HKD /50 USD, then195 HKD /25 USD. Contract net becomes195 HKD; outgoing cash reservation is75 USD and free cash25 USD. Three130 HKD releases of the first attribution release16.67,16.67 and16.66 USD exactly.

## 接口与持久化 / API and persistence

沿用 `POST /contracts/{id}/finance/entries`，在已有准确来源字段之外，为应收异币退回增加可选 `return_fx`。同币、费用、正向资金和应付退款不接受此输入。应付异币实收继续使用原 `refund_fx_acknowledged` 流程。

```json
{
  "amount": "195",
  "return_fx": {
    "cash_amount": "25",
    "rate": "0.1282051282",
    "source": "人工核对的银行退回凭证",
    "acknowledged": true
  }
}
```

以上为补充字段示例，实际命令仍需合同／财务版本、节点、来源ID／版本／修订／摘要、原收入事件和操作依据；不能单独提交这段摘要作为完整命令。

普通迁移102在 `contract_finance_events` 添加可空实际付出金额与不可变汇率／来源。应用方拒绝无用或不完整输入；数据库再次检查方向、币种、舍入、实际人类会话及金额上限。部分解除保存准确反向现金，不接受新报价。新 `return_cash` 输出区分本条付出、整笔原资金、原归属合同本金及尚占用付出；释放记录的尚占用值为空并明确指向原归属。历史同币及应付退款的核对快照保持原样，迁移1–101不改字节。迁移101的三币种费用退款约束保留。

Authorization and concurrency retain current contract/cash visibility, `contract.reconcile` and `payment.record`, recent interactive MFA, project/contract/source locks, exact revision/hash checks, immutable commands and same-transaction audit. The database guards both units again. Parameterized queries do not trust client-calculated reservation balances.

## 验证与发布边界 / Verification and release boundary

[实际验证及明确延期](../quality/135-receivable-foreign-returns-verification.md) · [全部23项余项](../delivery/05-functional-closure-135.md) · [冻结下载](../releases/v0.1.0-rc.1/README.md)

现有持久开发库／服务仍100，本轮临时数据库完整应用1–102。运行新后端前按既有正常升级工具依次应用101、102；不要让新后端指向尚未升级的库。没有新Android安装包或生产部署，133开发包和100结构的冻结服务器包保持原出处，不包含135。公司汇兑差额制度、完整四端旅程及正式模型／平台／云／目标Linux／生产验收仍开放。
