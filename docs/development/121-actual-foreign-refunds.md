# 增量121：异币退款实收 / Actual foreign-currency refunds

本轮接通真实异币退款的草稿、人工登记、原付款本金上限、现金报表和全额冲销。Web、微信小程序、抖音小程序和 Android 共用相同协议。资金只记录已经发生的事实，不发起银行转账。

This increment records an actual refund received in a currency different from the original payment. All four clients support drafting, human confirmation, original-principal limits, actual-currency cash reporting and full reversal. Recording never initiates a bank transfer.

## 业务边界 / Business boundary

| 编号 | 已实现约束 / Implemented constraint | 例子 / Example |
| --- | --- | --- |
| RF-01 | 实收金额与退回原本金分别固定 / Pin actual cash and original principal separately | 原付780 HKD、实收100 USD；不会相加为880 |
| RF-02 | 汇率方向为每1原付款币兑换实收币 / Rate is cash units per original principal unit | 780 × 0.1282051282 → 100 USD |
| RF-03 | 原本金和实收各按币种精度检查；只在实收精度舍入一次 / Exact currency precision and one cash rounding | JPY无小数，HKD／USD为两位；不使用浮点数 |
| RF-04 | 同币／异币退款共用原付款本金上限 / Same and foreign refunds share one principal cap | 已退390 HKD后，最多再扣390 HKD本金 |
| RF-05 | 当前仅使用原付款尚未分配且未预留的本金 / Use only unallocated, unreserved original principal | 已占用的费用分配须另经明确释放，不能重复退款 |
| RF-06 | 异币退款不能新建费用或合同归属分配 / Foreign refunds cannot create expense or contract attribution | 提交退款分配明确拒绝；详情移除关联分配按钮 |
| RF-07 | 固定完整原付款身份、范围、版本与实际凭证 / Pin source identity, scope, version and evidence | 不接受其他供应方、员工或项目的原付款 |
| RF-08 | 每次登记重新人工确认、当前权限与近期MFA / Fresh human review, current authority and recent MFA | 保存草稿不代表批准登记 |
| RF-09 | 原付款事务锁下重算余额；幂等回放不重复记账 / Lock source and recompute; exact replay adds no cash | 两笔400 HKD本金并发退款，原款780时仅一笔成功 |
| RF-10 | 冲销新增相反现金事实并恢复原本金 / Reversal appends inverse cash and restores principal | 冲销100 USD实收，原款恢复780 HKD可用本金 |
| RF-11 | 凭证、原修订、旧账本和已完成导出不改写 / Preserve immutable evidence and completed exports | 旧同币与增量116双币分配继续有效 |
| RF-12 | 审计写入失败即整体回滚 / Audit failure rolls back all changes | 不留下已登记退款或本金扣减 |

费用回转可能同时涉及实收、原付款、费用三种币种，当前协议没有把三种单位混进原双币分配表。异币退款仅完成未分配资金闭环，**费用／合同异币归属、公司汇差会计制度和完整业务验收仍属于 GAP-03／18**。正常同币退款的原分配回转继续沿用增量116。

Expense attribution may involve three currencies. This protocol therefore completes unallocated cash refunds while explicitly rejecting foreign refund allocations. Expense/contract attribution, adopted accounting treatment for exchange differences and complete business acceptance remain open under GAP-03/18. Existing same-currency refund inverses keep their increment116 behavior.

## 使用步骤 / User flow

1. 打开未冲销的实际付款，选择「登记收到退款」；移动端为「起草收到退款」。收付款对象和项目范围继承原付款。
2. 原币填写**实际收到的币种**，金额填写实际实收金额。币种不同后，填写退回原付款本金、实际汇率和退款汇率凭证来源，并选择准确的实收凭证文件版本。
3. 保存草稿。系统检查原本金 × 汇率按实收精度舍入后等于实收金额；无效报价不能保存。
4. 「登记实际资金」重新读取原付款及当前余额，明确人工核实，完成需要的MFA后提交。原付款余额不足时保留草稿，重新核对后再操作。
5. 实收进入相应币种的现金汇总；原付款「有效实际退款」使用原付款币种。本轮没有项目费用分配，项目份额报表不会把未分配退款自动归入某项目。
6. 若登记错误，打开退款执行全额冲销；系统保留原记录、汇率来源和凭证，并新建相反实收币种记录。真实的新退款须另建事实。

Open the recorded outgoing payment, start a refund draft and choose the currency actually received. For a different currency, enter original principal, rate and statement provenance, then select exact evidence versions. Save the draft, reopen human confirmation, review current source availability and submit with current MFA. Cash reports use the received currency; the original payment's refunded balance uses original principal units. A full reversal preserves evidence and restores the principal. Unallocated cash is not automatically assigned to project expense shares.

## 协议与状态 / Protocol and state

新增可选 `PaymentContent.refund_fx`，省略时保留旧协议及旧正文摘要：

```json
{
  "currency": "USD",
  "amount": "100.000000",
  "refund_fx": {
    "original_currency": "HKD",
    "original_amount": "780.000000",
    "rate": "0.1282051282",
    "source": "人工核对的实际退款银行凭证"
  }
}
```

上述仅为正文片段；完整建草稿／修订／登记字段见 [OpenAPI](openapi.json)。`POST /payments/{id}/record` 同时要求原付款当前 `expected_source_version`、自身版本／修订／摘要、`refund_allocations: []` 和本次 `refund_fx_acknowledged: true`。同币退款不接受该报价或确认。汇率日期由固定退款 `paid_on` 给出，冲销沿用原报价来源；冲销时间仅改变冲销现金生效日。

The snippet is partial content; the complete OpenAPI remains authoritative. Foreign confirmation adds `refund_fx_acknowledged: true` to the existing record command. Same-currency refunds reject a redundant quote or acknowledgement. The original refund's fixed cash date identifies quotation evidence; reversal retains that original quote while its own date identifies the correction date.

| 接口 / Endpoint | 本轮行为 / Behavior |
| --- | --- |
| `POST /payments`、`PATCH /payments/{id}` | 固定报价进入不可变修订；当前人类与原付款可读权限 / Immutable quote revision with human/source access |
| `POST /payments/{id}/record` | 人工确认、原付款锁、当前本金重算、审计同事务 / Reviewed locked principal admission and atomic audit |
| `GET /payments/{id}`、修订与事件历史 | 实收币种与本金依据并列展示 / Display received cash and principal evidence |
| `POST /payments/{id}/refund-allocations` | 异币退款409拒绝，保留同币协议 / Reject foreign attribution; retain same-currency behavior |
| `POST /payments/{id}/reverse` | 固定实收币种的全额反向事实及本金恢复 / Full inverse cash with principal restoration |
| `GET /reports/cash` 与现有导出 | 原实际币种现金、未分配份额；历史完成导出保持固定 / Actual cash currency and unallocated scope; completed exports stay fixed |

`refund_fx_acknowledged` 不是费用控制。秘书的文件操作授权、隔离、审计和技术资源限制继续保留；本轮不新增AI写财务工具，仍仅移出AI费用／token额度／计费控制。

## 数据约束与升级 / Storage and upgrades

- 迁移 `0093_actual_foreign_refunds.sql` 增加只读生成列 `payment_revisions.refund_original_amount`；旧行等于原实收金额，新异币行取固定原本金。原有payload、摘要、费用和现金金额不回填改写。
- `payment_events.refund_fx_acknowledged` 新增默认为false，仅登记事件允许为true。异币退款必须有明确确认的登记事件及原有人工MFA证据。
- 保留原不可变修订／事件／分配触发器、排序锁、幂等和当前权限检查；新增数据库报价与禁止异币归属约束，防止直接SQL绕过接口的单位边界。
- 原付款资金上限、合同未归属退款检查改用原付款本金单位；当前现金汇总仍使用退款自身实际金额，避免用报价替代银行事实。
- 正常92→93结构升级以已发布增量120源码产生的真实合成财务旅程验证，逐项比较15组原事实；新增派生列与默认确认列单独验证，不冒充历史迁移或容灾。

Migration93 adds generated original principal and a default-false human confirmation field. Legacy payloads, hashes, amounts, allocations and historical receipts stay unchanged. Source funding and contract unattributed-refund checks use principal units; cash reporting continues to use actual bank cash. Published schema92 finance fixtures validate the additive upgrade separately from current-source regressions. This is a normal schema upgrade, not a historical migration/DR exercise.

## 后续与验收 / Remaining acceptance

真实汇差会计处理需要公司的功能币种和制度选择；本轮报价是实际退款事实的人工依据，不自动生成汇兑损益、手续费或修改报表估值汇率簿。[IAS 21官方说明](https://www.ifrs.org/issued-standards/list-of-standards/ias-21-the-effects-of-changes-in-foreign-exchange-rates/)用于区分交易事实、功能／列报币种与会计处理，未据此代替公司政策审批。

Remaining: three-currency expense/contract refund attribution, adopted company accounting policy and full four-client business journeys. Real Codex/Skills, Linux isolation, TG, object storage, platform approval, production database and formal releases remain separate open packages. Real-device, network-performance, live120GB专项, historical migration/DR and AI fee-control requirements remain excluded.

实际执行范围、源码绑定和演示媒体见 [增量121验证](../quality/121-refund-verification.md)；全部编号见 [差距清单](../delivery/03-goal-gap-assessment.md)。
