# OA 正文起草、核对与采用 / OA prose drafting and adoption

增量101补充 GAP-12 的合同、采购、报销正文字段起草。员工固定业务修订和字段，秘书保存不可变建议，员工逐字段核对后采用为业务新修订，再使用原有申请和审批流程。代码位于 `app/`；[验证记录](../quality/101-oa-prose-verification.md)区分真实本地执行、受控工具夹具、缓存结果和待接入服务。

## 1. 支持范围

| 业务 | 可选起草字段 | 单字段上限 | 采用后的结果 |
| --- | --- | --- | --- |
| 合同 | 续约条款 `renewal_terms`、终止条款 `termination_terms`、备注 `notes` | 各20,000字符 | 普通合同草稿新修订 |
| 采购 | 用途 `purpose`、备注 `notes` | 用途4,000、备注20,000字符 | 普通采购草稿新修订 |
| 报销 | 用途 `purpose`、备注 `notes` | 用途4,000、备注20,000字符 | 普通报销草稿新修订 |

本次字段白名单不包括金额、币种、负责人、供应商、付款计划、费用明细和附件引用。采用接口从服务器保存的原始业务内容构造更新，仅替换选中的正文；正常业务校验、修订、审计和关联申请状态处理继续生效。用途沿用业务规则去除首尾空白，返回保存后的建议供员工核对。

合同正式正文附件、Office/PDF 正文生成、任意表单自动填写、更多申请类型和真实模型质量评估仍在 GAP-04／12／13。八项 Skill 保持 `draft`；该增量未安装或启用真实模型。

## 2. 四端使用逻辑

1. 在公司秘书中进入本人私密会话，选择与业务相同的公司／项目范围。
2. 选择“协助业务申请”，打开“选择业务申请操作”。
3. 选择合同、采购或报销，以及已保存、本人可编辑的准确业务草稿。
4. 选择“起草业务正文”，勾选本次起草字段，填写操作说明和本次消息，再确认发送。
5. 秘书读取授权依据并保存建议。任务结果为“草稿待处理”，此时业务版本尚未改变。
6. 在任务进度内对照“原文／正文建议”，勾选完整核对声明，填写采用依据，点击“采用正文草稿”。
7. 查看返回的业务新修订和操作记录。随后另行选择“起草或修订申请”，核对送审预览后提交审批。

Web 使用双栏原文对照，窄屏自动纵向排列。微信、抖音及 Android 共用 Taro 纵向核对组件。字段选择和发送不自动采用正文；记录结果不明时先刷新任务和业务修订，页面不自动重发写操作。

## 3. 协议与数据约束

`RunCreate.oa_request.tool = draft_business_text` 的 `arguments` 必须包含 `kind`、`business_id`、`expected_business_version`、`business_revision_id`、`business_content_hash`、`fields`、`reason`。每次仅授权一项业务操作，不能同时携带文件、项目或知识写入意图。

模型工具 `draft_business_text(run_id, intent_sha256, fields)` 只能使用原授权字段；正文必须完整、非空、有效 UTF-8，且符合业务长度限制。同一任务保存一份不可变建议；不同正文需要新任务。相同调用通过工具台账保持幂等。工具返回 `committed=false`、空业务操作号；任务必须以 `draft` 返回，不能将建议声明为业务完成。

`POST /v1/runs/{id}/oa-text-adoption` 的请求为：

```json
{
  "draft_sha256": "<已核对建议的64位SHA-256>",
  "reviewed": true,
  "reason": "本人已核对全部正文建议"
}
```

需要常规登录、当前会话／业务权限、合法 `Idempotency-Key`，Cookie 写请求还需 CSRF。只允许原员工在相同私密会话范围采用已成功返回 `draft` 的任务；取消、失败、未完成、正文摘要不匹配或业务原文变化时拒绝采用。模型和 TG 没有此工具。

采用时重新锁定任务并核对原业务版本、修订、内容摘要、生命周期和当前权限。建议包含起草前实际读取的来源快照；显式选中和工具额外读取的资料均重新验证。来源失效后任务正文及采用能力一起撤回，避免通过旧建议继续暴露资料。

采用协调器与正常 `contracts.Update`／`spend.Update` 使用同一数据库事务。业务修订、业务幂等记录、采用回执、采用审计和 outbox 一起成功或回滚。相同幂等请求返回当前可见回执；新键重复采用返回冲突。采用不会代替准备申请、送审、员工审批、签署、下单、记账或付款。

## 4. 数据库与实现位置

正常结构升级新增迁移80，保留原1—79字节：扩展 OA 意图工具枚举，新增 `agent_oa_text_drafts` 和 `agent_oa_text_adoptions`。建议保存准确 UTF-8 JSON、SHA-256 和来源快照；采用表固定正常业务操作号及结果。两表都有数据库不可改写触发器、租户／任务外键和插入绑定校验，应用角色仅可读取、追加。

| 层 | 实现 |
| --- | --- |
| 后端 | [oa_text.go](../../app/backend/internal/agent/oa_text.go)、现有 OA 意图与来源复核 |
| MCP | [mcp.go](../../app/backend/internal/agentworker/mcp.go)、受限工具集合与执行隔离握手 |
| 共享输入与回执 | [oa-text.ts](../../app/packages/contracts/src/oa-text.ts) |
| Web | [ConversationOAText.tsx](../../app/web/src/ConversationOAText.tsx) |
| 微信／抖音／Android | [oa-text.tsx](../../app/mobile/src/pages/secretary/oa-text.tsx) |
| Skill | [OA 助手说明](../skills/oa-assist/SKILL.md)，包版本1.3.0、状态 `draft` |

## 5. English usage and boundaries

Select an editable contract, purchase or reimbursement in your own private secretary conversation. Choose **Draft business prose**, select the prose fields and send the exact business revision. The tool saves an immutable suggestion and returns a draft outcome. Review every original/proposed field, confirm the review, provide a reason and adopt it as a new business revision. Prepare and submit an approval separately.

The adoption endpoint uses current human permissions, session/CSRF protection, source revalidation, optimistic business versions and normal business commands in one transaction. It never accepts commercial fields from model output or the adoption request. Suggestions retain all sources observed before proposal; changed or inaccessible sources withdraw access. Actual Codex, production Skill installation, cloud/platform configuration and wider business workflows remain open. See the [current numbered gap register](../delivery/03-goal-gap-assessment.md).
