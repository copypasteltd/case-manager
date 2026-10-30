# 增量117：完整合同正文起草与人工采用 / Complete contract bodies

本轮解决秘书只能提出部分 OA 字段建议、无法在同一任务里阅读全文并固定为合同正文附件的问题。员工选择一份准确的可编辑合同和一个准确的新文件目录，秘书通过现有文件创建流程产生完整文本；员工阅读全文并明确采用后，系统保存正常合同新修订，只替换其草稿正文附件。

这是 GAP-04／12／18 的局部实现。真实 Codex、八项 Skill 正式发布、Office／PDF／OCR／复杂 DOCX 和完整 OA 附件体系继续开放；本轮的受控模型夹具不证明真实模型写作质量。

## 使用流程 / Usage

1. 打开本人私有的项目秘书会话，选择“新建或更新文件”任务。
2. 选择新建文件位置、准确目录修订、文件名和文本格式。支持 UTF-8 `text/plain`、`text/markdown`，完整文件上限 48 KiB。
3. 点击“选择合同正文依据”，选择同项目、同分类、可编辑的草稿合同，填写起草说明，加入本次请求。公司范围使用 `project_id=null`，同样要求三个对象范围一致。
4. 填写任务说明并发送。消息和任务固定合同 ID、原修订、内容哈希、新文件授权摘要。提交请求不会改变合同资料。
5. 文件经真实工具协议创建草稿、扫描、提议和提交后，在任务进度中打开“完整合同正文与人工采用”。页面显示合同事实、准确文件版本、字节数、SHA-256 和完整正文，包括差异摘要之外的全文。
6. 核对商业条件、缺失事项及正文全文，勾选本人确认，填写采用依据，再点击“采用完整合同正文”。当前人类会话需要近期 MFA；过期时按现有账号流程重新验证后重新读取。
7. 查看保存后的合同修订及准确文件版本。送审、签署、生效仍走各自正常业务操作。

Web、微信、抖音及 Android 共用相同接口和收据校验逻辑。移动端全文为可选择文本。浏览器中的 Taro H5 检查验证共享页面，不冒充小程序容器或 Android 设备执行。

## 业务边界

| 编号 | 约束 | 实现结果 |
|---|---|---|
| CB-01 | 原合同明确 | `contract_id`、`expected_version`、`base_revision_id`、`content_hash` 全部固定；合同处于可编辑草稿状态。 |
| CB-02 | 人与范围明确 | 本人私有会话，且合同、会话、新文件目录项目一致、分类完全一致；执行期间继续检查原人类会话和当前授权。 |
| CB-03 | 文件授权独立 | 只允许 `file-update` Skill 搭配一个 `file_create`。不能混入更新旧文件、恢复文件、业务字段修改、申请操作或知识发布。 |
| CB-04 | 生成不等于采用 | 秘书只有新文件工具。没有合同采用、送审、签署或生效工具。文件生成成功也不自动改合同。 |
| CB-05 | 全文按实际文件核对 | 完整正文来自不可变的单编辑新建草稿，必须与已提交首版的 SHA、UTF-8 字节数、名称、格式、干净扫描和范围一致。 |
| CB-06 | 人工采用准确版本 | 必须任务成功结束、未请求取消、明确 `reviewed=true`，且输入准确文件版本和完整正文 SHA；近期 MFA 和当前合同更新权限缺一不可。 |
| CB-07 | 采用只改附件 | 在同一事务内调用正常合同更新流程。只将原可信内容的 `draft_file_version_id` 换成准确新文件，保留原金额、币种、相对方、日期及其他条件。 |
| CB-08 | 原资料变化即停止 | 执行阶段原合同变更，阻止后续文件写入；文件已生成后原合同变化，阻止采用并保留已生成文件及其历史。 |
| CB-09 | 历史与权限并行 | 不可变授权、原资料、采用回执和正常业务修订保留。读取任务及事件流仍检查当前合同、附件和会话权限；撤权后不能借历史阅读全文。 |
| CB-10 | 技术限制保留 | 48 KiB 正文、输入输出上限、工具次数、执行时间、隔离和审计继续适用；AI 费用预算、费用配额和计费控制不在当前范围。 |

未知条款须明确标为待本人确认。正文内容属于待核对草稿，系统不会据此推导付款、批准、签署或承诺，也不会自动生成缺失商业事实。

## 接口与数据 / API and data

`RunCreate.contract_body` 为可选输入，与 `file_create` 一起提交：

```json
{
  "contract_id": "<accurate-contract-id>",
  "expected_version": 1,
  "base_revision_id": "<accurate-revision-id>",
  "content_hash": "<sha256>",
  "reason": "依据已选合同资料起草，缺项由本人确认"
}
```

| 接口／来源 | 用途 |
|---|---|
| `POST /conversations/{id}/runs` | 固定合同原资料与新文件授权，不修改合同。 |
| `read_resource(type=agent_contract_body)` | 只读本任务已授权合同依据，按现有分页字符限制返回；不泄露原会话 ID。 |
| `GET /runs/{id}` | 返回当前有权读取的准确合同正文视图。 |
| `GET /runs/{id}/event-log` | 返回相同全文视图及真实任务事件，供四端任务进度使用。 |
| `POST /runs/{id}/contract-body-adoption` | 独立的人类采用命令；幂等请求，事务内正常更新合同及追加采用回执。 |

采用输入为 `intent_sha256`、`version_id`、`content_sha256`、`reviewed`、`reason`，同时使用现有 `Idempotency-Key`。状态 `authorized` 表示只有授权；`file_committed` 表示实际文件已生成但未采用；`adopted` 表示已保存正常合同新修订和不可变采用回执。这些正文状态与实际任务执行状态共同判断，失败或取消任务不能采用。

Schema90 新增 `agent_contract_body_intents` 和 `agent_contract_body_adoptions`，均只允许应用追加及读取。插入约束绑定原任务、文件创建摘要、合同快照、真实文件提交及正常人工合同操作。既有合同、财务事实和历史 AI 费用资料不回填、不改写。正常89→90升级属于版本升级，不属于已移出的历史数据迁移或容灾。

接口总数为 334 路径、410 操作、736 模型。只扩展原 `AgentSourceSnapshot`、`RunCreate`、`Run`，新增五个正文相关模型和一个人工采用路径；原路径保持兼容。

## 失败、重试与恢复

| 情况 | 用户操作与系统行为 |
|---|---|
| 尚未生成／扫描中 | 查看实际任务和文件状态，不能采用。 |
| 文件生成成功、第二副本仍排队 | 依据现有文件详情显示真实复制状态；不将排队表述为两地备份成功。 |
| 合同资料、会话或当前权限变化 | 重新读取并重新选择准确依据；旧任务不能覆盖新资料。 |
| 任务已取消或失败 | 保留已产生的正常文件和历史，不提供采用权限。 |
| 无近期 MFA、未勾选核对、版本或哈希不符 | 服务端拒绝写入，合同原修订保持。 |
| 提交结果未知 | 先重新读取任务和合同；保留原幂等键核对原命令，不换键盲目再提交。 |
| 审计或正常合同更新事务失败 | 正常合同修订、附件引用、操作回执及采用记录一起回滚。 |
| 已采用后再次执行新命令 | 拒绝重复采用；同一个原幂等请求只返回原操作，不追加合同修订。 |

## 实现与核验入口

- 后端：`app/backend/internal/agent/contract_bodies.go`、`run_reads.go`，迁移 `0090_agent_contract_bodies.sql`。
- 共享校验：`app/packages/contracts/src/contract-body.ts`，独立 UTF-8、SHA-256、完整版本和采用回执校验。
- 页面：`ConversationContractBody.tsx`、共享移动端 `pages/secretary/contract-bodies.tsx`。
- Skill：`file-update` 1.4.0，仍为 draft；八项 Skill 均未因本地夹具检查自动发布。
- [本轮核验](../quality/117-contract-body-verification.md)、[媒体](../media/README.md)、[全部未完成项](../delivery/03-goal-gap-assessment.md)。

## English guide

Select an editable contract and an exact new text-file destination in a private secretary conversation. All three objects must share the same project and classification. The task pins the original contract revision and file authorization, generates a complete UTF-8 text/Markdown file through the ordinary staging, scanning and commit pipeline, and exposes its complete bytes in both run and event-log views. The 48 KiB limit applies to the complete file.

A separate human action requires current permissions, a recent MFA session, explicit review, the precise file version and SHA-256. It saves a normal contract revision that changes only the draft-body attachment. Generating the file grants no authority to adopt, approve, sign or activate a contract. Changed original facts, cancelled tasks, revoked access or a mismatched file block adoption. The normal business command and immutable adoption receipt commit or roll back together.

The feature is shared by Web, WeChat, Douyin and Android. Current local browser/native/build evidence has its stated scope; real Codex, formal Skills, wider document formats, cloud integrations and production/platform acceptance remain open. Only AI fee controls are excluded; technical limits, authorization, audit and isolation remain required.
