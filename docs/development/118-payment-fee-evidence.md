# 付款手续费依据 / Actual payment fee evidence

本轮在 GAP-03／18 下增加手续费凭证整理。公司经营费用仍属于当前范围；用户移出的是 **AI 费用控制**，不影响报销、采购、订阅费用或银行手续费。整个工作包继续开放。

## 业务含义

手续费是一笔真实发生的费用。先通过现有费用流程确认发生支出，通过资金流程登记实际付款并完成费用分配，再从原付款关联这条准确分配。可以选择同一付款中的分配，也可以选择另一笔实际手续费付款。

关联与撤销只增加依据历史，不新增费用、付款、汇率、费用分摊或现金分配，不改变原金额及报表。撤销误关联不等于退款，也不冲销真实费用。退款、释放分配和资金冲销继续使用原财务命令。

This increment classifies an already confirmed incurred cost and an already recorded cash allocation as fee evidence for an original outgoing payment. The fee may be an allocation of the same payment or a separate actual payment. Association and revocation append evidence only: they create no extra monetary facts or inferred FX differences. Ordinary financial commands continue to handle actual refunds, allocation release and reversals.

## 使用流程

1. 在费用发生中登记并确认实际手续费，固定原凭证；在付款与结算中登记真实资金并核销费用。异币核销按增量116人工核对实际结算汇率。
2. 打开原支出付款详情 → 查阅手续费依据 → 关联手续费依据。
3. 从实际资金列表选择手续费付款，或选择“使用本笔付款中的分配”，再选择整条原分配。候选列表支持分页；资金列表支持标题查找。相同标题的记录须核对金额和完整原资料。
4. 阅读原付款、手续费费用、实际资金、原分配的双币金额、核对时有效余额、准确修订及摘要。查阅源记录和原凭证，填写操作依据，明确确认并完成近期两步验证。
5. 保存后查看有效关联与当前原分配余额。若关联有误，明确撤销并填写依据；旧关联、旧摘要及撤销记录保留。重新关联会生成新的关联 ID。
6. 资料、权限或关联版本发生冲突时重新读取付款与历史。结果未知时先查历史；Web 保留原样重试同一请求，移动端采用现有未确认命令锁定机制，避免换内容重复提交。

Web has a dedicated detail panel; WeChat, Douyin and Android share the Taro finance page. Both use actual authorized candidate lists and identical basis/receipt validation. Mobile requires explicit MFA inside each sensitive form. A development APK or H5 recording does not establish platform publication or production acceptance.

## 固定依据与动态余额

| 对象 | 固定内容 | 读取时核对 |
|---|---|---|
| 原付款 | ID、版本、修订、内容摘要、标题、范围、币种、金额 | 当前付款读取权限；保存时原版本完全一致 |
| 手续费实际资金 | 同上 | 当前读取权限；新关联时必须为已登记支出付款 |
| 手续费费用 | 同上 | 当前读取权限；新关联时必须为已确认正向费用 |
| 整条原分配 | 根 ID、源费用修订、双币本金／精度、两套项目份额、实际汇率及凭证来源、核对时余额 | 读取实际当前双币余额；新关联时两币余额均须为正 |
| 关联／撤销 | 人类用户、会话、依据、记录时刻、独立递增版本 | 当前会话及来源资料权限；不可变历史 |

全部固定依据按服务器完整 JSON 字节计算 SHA-256，最多128 KiB。共享客户端重新计算 UTF-8 摘要，并核对来源、原分配、项目份额与双币语义。客户端不能把哈希校验当成来源授权。

当前余额与不可变快照分开显示。原分配被正常释放、退款或冲销后，历史仍保留，界面按当前双币余额显示“有效关联”或“原分配已无有效余额”；明确撤销始终显示“关联已撤销”。记录为空不代表没有手续费。

## API 与持久化约束

| 方法 | 路径（前缀 `/v1`） | 用途 |
|---|---|---|
| GET | `/payments/{id}/fees?limit=20&cursor=…` | 当前授权历史、动态双币余额、独立版本及签名分页 |
| GET | `/payments/{id}/fees/basis?fee_payment_id=…&allocation_id=…` | 读取完整当前依据及 SHA-256 |
| POST | `/payments/{id}/fees` | 明确人工关联，返回不可变关联收据 |
| POST | `/payments/{id}/fees/{fee}/revoke` | 明确人工撤销，返回原关联及新增撤销收据 |

写入请求包括原付款准确版本／修订／摘要、当前手续费历史版本、完整依据摘要、`reviewed=true` 和操作依据。创建额外指定准确手续费资金和原分配。遵循现有幂等键、当前权限、近期 MFA、审计和事务性事件规则。

Schema91新增 `payment_fee_links`、`payment_fee_revocations` 两张仅追加表；四个触发器约束历史不可改、准确来源、真实人类会话、近期 MFA、版本顺序和未重复关联的根分配。应用角色仅有 SELECT／INSERT。关联版本独立于原付款版本，避免整理依据导致原资金记录虚假变化。 `payment.fee.linked`／`payment.fee.revoked`事件按原付款分组，事件中的`resource_version`／`aggregate_version`指手续费台账序号；准确原付款版本位于不可变完整依据中。

三名员工的当前规模下，这类短元数据写入使用组织排他锁，与现有组织共享写锁串行，避免与项目／费用／付款锁交叉死锁。禁止同一组织同一根分配同时关联多笔原付款。明确撤销后可以重新关联；并发新建只能有一条成功。

历史分页的签名游标绑定当前用户、组织权限版本、原付款及手续费历史版本。换付款、换权限或新增／撤销历史后，旧游标失效，需从第一页重新读取。每页1至100条，默认20条。

## 权限和失败行为

- 仅当前有效人类交互会话；TG／模型执行会话不能执行此操作。写入和幂等回放先检查当前会话及近期 MFA。
- 每次读取及写入均检查原付款、手续费资金、费用的当前权限；管理还要求 `payment.record`、`cost.record` 及可写业务范围。三者必须处于同一管理项目、同一分类。
- 预览后来源版本、余额或关联版本变化会拒绝提交；失败不追加关联。审计写入失败会回滚关联和正常业务事务。
- 收据只固定原操作事实。再次读取历史才提供当前余额，避免幂等回放把旧余额当成实时结果。
- 不提供手工输入业务 ID 的产品流程，不允许凭空补费用、把汇率差额视为手续费或自动推导公司会计口径。

## 验收边界

当前源码检查覆盖实际关联／撤销、同笔付款、双币余额变化、完整摘要、近期 MFA、并发重复关联、分页过期、撤权及审计回滚；Web／H5核对实际页面和 API，构建两小程序及 Android 开发包。正常90→91升级检查使用已发布117的实际双币财务旅程，并核对原费用、现金、分配、项目份额及历史 AI 记录不变。

公司财务制度采纳、汇兑差额的确认／结转、异币实收退款、完整报表及四端长流程仍属 GAP-03／18。真实身份／云／平台／正式 Android／生产验收仍按原工作包推进。本轮不包含真机、网络性能、实网120GB、历史数据导入、容灾或 AI 费用控制。

[本轮核验](../quality/118-payment-fee-verification.md) · [全部未完成项](../delivery/03-goal-gap-assessment.md) · [接口定义](openapi.json)
