# 增量73：秘书交付资料与里程碑关联

员工可在项目秘书会话中选择实际交付，完整核对名称、验收准则、负责人及里程碑关联后随消息发送。秘书复用原 `UpdateDeliverable` 命令，返回实际业务回执；提交和验收继续由员工在原交付页面办理。代码在 `app/`，文档和验证记录在 `docs/`。

## 1. 行为与边界

| 编号 | 行为 | 实现约束 |
| --- | --- | --- |
| DU-01 | 选择准确交付 | 仅对应项目会话中的真实记录；使用读取时交付版本，不拿项目版本替代 |
| DU-02 | 核对完整资料 | `name`、`acceptance_criteria`、`owner_id`、`milestone_id` 必须全部存在；保持原值的字段也须在预览中核对 |
| DU-03 | 明确关联或解除 | 使用同项目实际开放里程碑 ID；不关联或解除必须显式 `null`，缺项不能代替清空 |
| DU-04 | 准确人工授权 | 完整对象、版本、资料和原因随正式发送固定；普通聊天、模型推断、历史回执和预览不授予写权限 |
| DU-05 | 复用原业务 | 交付写权限为既有 `deliverable.create`，同时继续核对 AI 分类、项目阶段、有效成员、里程碑与当前共同受众 |
| DU-06 | 保留人工流程 | 只修改协议1的草稿或退回交付资料；旧协议、已提交、已验收及只读项目拒绝，原关联已关闭也须先按原业务重开 |
| DU-07 | 固定正式修订 | 不改变交付状态、正式修订 ID、固定文件、修订正文或人工决定；退回后资料修改不会改写旧提交 |
| DU-08 | 幂等与真实回执 | 同一调用重放或同一授权换调用键，复用原操作；原命令、审计、事件、不可变提交及工具回执同事务完成 |
| DU-09 | 受控阅读 | `read_resource` 的交付读取仅开放本次授权对象；使用本次 `agent_mutation` 引用并重新核对受众、版本及原登录，不开放其他交付枚举 |
| DU-10 | 冲突和中断 | 取消、原登录撤回、版本变化、权限／受众变化会阻止后续执行；不能刷新版本后自行扩大原决定 |

## 2. 接口与事务

新增受控工具 `update_deliverable`，复用既有运行创建接口的 `mutation`，没有新增公开业务端点。工具参数固定为以下七项，不接受权限、状态、修订或文件字段：

```json
{
  "tool": "update_deliverable",
  "arguments": {
    "id": "交付的小写 UUID",
    "expected_version": 3,
    "name": "首轮完整交付",
    "acceptance_criteria": "核对完整固定资料与约定结果。",
    "owner_id": "实际项目成员的小写 UUID",
    "milestone_id": null,
    "reason": "按已核对资料更新，并明确不关联里程碑"
  }
}
```

准备阶段固定交付投影、基础版本、完整字段、会话修订、受众和原登录。执行前复核当前投影及权限，经原命令提交后核对实际新版本与投影摘要。返回值沿用原业务 `DeliverableReceipt` 的 `id`、`version`、`revision_id`，不是凭空拼出的完整业务对象；版本必须为基础版本加一，正式修订 ID 保持原样。

迁移 [0067](../../app/backend/migrations/0067_agent_deliverable_updates.sql) 扩展准确工具和 `deliverable` 范围，保留里程碑创建的父子版本规则、原登录、不可变与延迟证据约束。新增交付提交校验逐项比较实际完整资料、原状态与正式修订，且必须匹配原成功幂等操作。该迁移不补造旧协议交付的创建人或验收事实。

追加迁移 [0068](../../app/backend/migrations/0068_agent_deliverable_text_whitespace.sql) 让名称与验收准则的首尾空白核对匹配 Go `strings.TrimSpace`，覆盖换行、制表及 Unicode 空白。0067 已在本地开发应用，因此保留原迁移摘要，以追加迁移修正；原始人工输入仍保留在固定授权中，不偷偷改写。

## 3. 代码与多端

| 层 | 位置 | 作用 |
| --- | --- | --- |
| Go | [mutations.go](../../app/backend/internal/agent/mutations.go)、[context.go](../../app/backend/internal/agent/context.go) | 完整授权、准确交付投影、原业务命令、原登录与真实提交 |
| MCP | [mcp.go](../../app/backend/internal/agentworker/mcp.go) | 严格七项参数和受限读取；原生隔离证明准确包含十项工具 |
| 契约 | [mutation.ts](../../app/packages/contracts/src/mutation.ts) | 完整构造、显式空关联、版本及正式修订回执核对 |
| Web | [ConversationMutation.tsx](../../app/web/src/ConversationMutation.tsx) | 实际交付分页选择、完整资料表单、成员及同项目里程碑分页、发送前预览 |
| Taro | [mutations.tsx](../../app/mobile/src/pages/secretary/mutations.tsx) | 相同完整表单、关联选择／解除与回执，供微信、抖音和Android共用 |
| Skill | [project-update](../skills/project-update/SKILL.md) | 源码1.3.0，八项准确写工具及读取／操作核对；继续保持 draft |

## 4. 验证与未完成范围

[原生专项](../../app/backend/internal/httpapi/agent_deliverable_updates_integration_test.go) 包含九项集成场景：完整参数与普通聊天、原命令和真实事务、两种重试、显式解除、错误项目／里程碑／负责人、版本及项目阶段、合成旧协议、实际人工提交／退回／重新提交／验收、取消／不可变历史及原登录撤回。旧协议记录仅由隔离测试库的 owner 事务构造，运行进程不具备该身份或触发器绕过能力。

Web 测试另外验证完整字段构造、空关联与错误修订回执；运行时测试验证严格 Schema 与同运行的真实写入证据。两端浏览器通过真实登录、原生API和业务历史核对完整表单、关联与解除预览、准确发送、重试及取消。浏览器发送后的运行不会交给真实模型执行；实际写入由受控 MCP 专项证明。

结果、来源与资源回收见[验证记录](../quality/evidence/deliverable-updates/verification.md)。本轮推进 GAP-01／12／13／18；交付创建、更多项目工具及跨范围能力、真实模型评测与 Skill 发布、真实云／TG／平台／设备、120GB／2TB和生产部署／恢复等仍按完整 Goal 推进，不缩减四端首发范围。

增量73结束时待补既有里程碑标题等字段的 Unicode 首尾空白，以及秘书交付创建。[增量74](74-secretary-deliverable-creation.md)已复现原两个里程碑场景并用追加迁移0069修正，同时接通交付草稿创建；其余项目操作继续按当前差异清单推进。
