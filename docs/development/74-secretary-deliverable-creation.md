# 增量74：秘书创建交付草稿

员工可以在项目秘书会话中，核对名称、验收准则、实际项目成员负责人、里程碑关联及原因后，明确授权创建一份交付草稿。秘书执行原交付创建命令，返回真实新 ID 和版本；员工随后在原交付页面上传固定文件、提交及验收。代码位于 `app/`，本地验证记录位于 `docs/quality/evidence/`。

## 1. 需求与业务边界

| 编号 | 行为 | 约束 |
| --- | --- | --- |
| DC-01 | 明确创建意图 | 仅项目会话中的本次准确授权可创建；普通消息、历史结果及模型推断不能代替 |
| DC-02 | 核对完整资料 | 名称、验收准则、实际负责人、明确里程碑关联或 `null`，以及原因须同时固定；缺少关联字段不表示不关联 |
| DC-03 | 使用父项目版本 | 请求中的 `id` 和 `expected_version` 指向当前父项目；版本变化时等待员工重新核对 |
| DC-04 | 沿用原权限与状态 | 使用 `deliverable.create`；保持原项目只读阶段、有效成员、同项目开放／逾期里程碑与 AI 共同受众规则 |
| DC-05 | 创建真实草稿 | 原命令分配新交付 ID，协议为1、状态为 draft、版本为1；没有正式修订或文件 |
| DC-06 | 展示准确回执 | 返回原 `DeliverableReceipt` 的新 `id`、`version:1` 和 `revision_id:null`；父项目基础版本和新交付版本分别核对 |
| DC-07 | 重试不重复创建 | 同调用键或同一授权更换调用键，复用原业务操作、新 ID 及原回执 |
| DC-08 | 有限后续阅读 | 创建成功后只增加本次真实新交付的受控读取，不开放其他交付的枚举；每次重查当前受众和原登录 |
| DC-09 | 独立人工提交验收 | 创建不附带文件、修订或审批参数；后续资料更新须另发准确授权，正式提交与独立验收使用原人工流程 |
| DC-10 | 取消、撤权与回滚 | 取消或撤销原始交互登录后不得继续创建；回执落库失败时，交付、业务幂等、审计及事件一起回滚 |

同项目候选来自真实分页接口。界面只列出可用的实际成员与里程碑；服务端仍重新执行原业务检查。预览本身不创建记录，正式发送固定完整七项参数。

## 2. 严格契约与父子版本

新增受控工具 `create_deliverable`，使用既有运行创建接口中的 `mutation`，不增加公开业务端点：

```json
{
  "tool": "create_deliverable",
  "arguments": {
    "id": "父项目的小写 UUID",
    "expected_version": 8,
    "name": "首轮成果交付",
    "acceptance_criteria": "核对约定成果及其固定文件版本。",
    "owner_id": "实际项目成员的小写 UUID",
    "milestone_id": null,
    "reason": "根据本次完整核对建立交付草稿"
  }
}
```

七项参数全部必填，`milestone_id` 允许显式 `null`；未知字段、状态、文件、正文修订或验收参数均拒绝。执行前固定父项目投影、基础版本、会话修订、成员受众和原始交互登录，不能自动刷新基础版本或补齐缺项。原 `project.Service.CreateDeliverable` 完成写入，保持父项目版本，返回新交付的版本1。后续更新使用新交付自己的 ID 和版本，不能继续拿父项目版本替代。

实际成功结果只有三项：

```json
{"id":"真实新交付的小写 UUID","version":1,"revision_id":null}
```

该结果表示创建时的事实。若员工后来提交或验收，历史创建回执仍保留版本1和空正式修订；当前交付投影则显示后续真实状态，两者不能互相覆盖。

## 3. 事务、证据与数据库约束

追加迁移 [0069](../../app/backend/migrations/0069_agent_deliverable_creation.sql)，保留既有0067／0068摘要。三处工具白名单增加 `create_deliverable`，授权范围仍为父 `project`。提交绑定验证新 ID、版本1、原成功操作、创建人及同项目真实交付；会话校验另核对草稿／协议1、全部七项资料、没有正式修订记录，以及准确三项原业务回执。

业务创建、幂等操作、审计、outbox、不可变提交和工具回执在同一事务完成。失败不能留下交付孤儿记录。现有追加写权限、原登录存活和延迟证据触发器继续生效。`read_resource` 仅接受当前授权对象或本次真实创建的子对象，来源使用固定 `agent_mutation`；来源仍要通过当前权限与共同受众复核。

0069同时修正既有里程碑创建和更新的标题首尾空白比较，使 SQL 与 Go `strings.TrimSpace` 一致。包含换行、制表、NBSP 和全角空格的真实指令，在68项迁移上曾因数据库二次比较失败而返回 `INTERNAL_ERROR`；保留[复现记录](../quality/evidence/deliverable-creation/whitespace-reproduction.json)，原两个业务场景在69项迁移上重新验证。没有改写员工固定的原始输入或已应用迁移。

## 4. 实现位置

| 层 | 位置 | 作用 |
| --- | --- | --- |
| Go 执行与阅读 | [mutations.go](../../app/backend/internal/agent/mutations.go)、[execution_tools.go](../../app/backend/internal/agent/execution_tools.go) | 严格授权、原命令、真实父子回执与有限新对象阅读 |
| MCP 与原生证明 | [mcp.go](../../app/backend/internal/agentworker/mcp.go)、[bridge.go](../../app/backend/internal/agentworker/bridge.go) | 七项参数 Schema、十一项工具白名单及隔离证明核对 |
| SDK | [http-boundary.ts](../../app/agent-runtime/src/http-boundary.ts)、[skills.ts](../../app/agent-runtime/src/skills.ts) | 创建边界、真实写入回执及完成证据 |
| 公共契约 | [mutation.ts](../../app/packages/contracts/src/mutation.ts) | 完整构造、新交付 ID、版本1及空正式修订校验 |
| Web | [ConversationMutation.tsx](../../app/web/src/ConversationMutation.tsx) | 空白创建表单、实际成员及里程碑选择、完整预览、准确发送与结果 |
| Taro | [mutations.tsx](../../app/mobile/src/pages/secretary/mutations.tsx) | 微信／抖音／Android共用创建入口与父子版本展示 |
| Skill | [project-update](../skills/project-update/SKILL.md) | 源码1.4.0，九项准确写工具及读取／操作核对；八项包继续 draft |

## 5. 验证与后续范围

[原生集成测试](../../app/backend/internal/httpapi/agent_deliverable_creation_integration_test.go)覆盖十项场景：完整父项目授权与显式关联、准确新草稿／事务证据／两种重试、显式不关联、无效成员及异项目／关闭里程碑、父项目版本冲突、只读项目、回执失败回滚、真实人工提交及独立验收、取消及不可变历史、撤销原登录。测试使用隔离原生 PostgreSQL 和受控 MCP，不调用真实模型或外部服务。

首次专项中，“只读项目”测试夹具尝试把含未完成事项的项目直接转入结项，被原结项约束正确拦截。夹具改为原规则允许的取消项目后重新通过；没有绕过业务约束。完整回归还包含原交付资料更新和两个真实里程碑空白字符场景。

Web 和 Taro 浏览器分别通过真实 MFA 登录，核对既有创建回执及后续人工验收；实际操作完整表单、显式空关联、实际里程碑选择、固定七项发送、幂等重试及取消。浏览器发送后的运行保持受控授权态并取消，实际写入由原生 MCP 场景验证。H5 操作不代替微信／抖音／Android实体设备验收。

构建、完整回归、源码摘要及浏览器回收证据见[本轮验证记录](../quality/evidence/deliverable-creation/verification.md)，完整开放事项仍按[28项差异清单](../delivery/03-goal-gap-assessment.md)推进。更多项目和跨域工具、真实模型评测与 Skill 发布、实际 OSS／R2／TG／平台、120GB／2TB、设备、安全、生产部署及恢复尚未因本轮增量而完成。
