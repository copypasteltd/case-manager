# 84. 秘书管理项目风险与问题 / Secretary risk and issue workflows

## 1. 范围 / Scope

在原 Go/PostgreSQL 项目风险与问题模块上增加四个准确授权工具：`create_risk`、`update_risk`、`create_issue`、`update_issue`。Web 与 Taro 使用同一份字段、状态、参数和回执规则；微信、抖音及 Android 继续使用现有16个注册页面。`project-update` 源码包版本为1.5.0，包含13个业务写工具和2个受控读取／操作检查工具，仍为 draft。

Four explicit tools extend the original Go/PostgreSQL risk and issue business commands. Web and Taro share field, state, argument and receipt rules. Both mini-program targets and Android continue to use the existing sixteen registered pages. The `project-update` source package is version 1.5.0 with thirteen business write tools and two bounded read/operation tools. It remains draft.

八个 Skill 均未发布，真实模型和 Telegram 不因本次修改自动启用。隔离夹具提供可核验的原业务结果；不代表真实模型、手机、小程序平台或生产验收。

All eight Skills remain unpublished. This increment does not activate a model or Telegram. Controlled fixtures provide verifiable original business results; they do not establish real model, device, mini-program platform or production acceptance.

## 2. 员工使用流程 / Employee workflow

1. 进入项目会话，选“更新项目”任务，再选“选择准确业务更新”。创建时读取父项目的实际版本；修改时从当前可见的风险或问题中明确选择对象，再读取其实际版本。
2. 核对完整标题、说明、实际有效项目成员负责人、严重程度、截止时刻。风险补充概率和缓解措施；问题补充解决说明。未改动字段同样属于本次请求。
3. 截止时刻明确设置或清空。设置须含时区，并保留最多六位小数。风险概率明确为低／中／高或未评估／清空，不能以漏字段代替清空。
4. 修改默认保持原状态；选择新状态时遵守原状态机。风险发生将新增关联问题，须勾选确认这一影响；缺少问题创建权限时后端拒绝整个请求。
5. “加入请求”仅生成预览。核对字段、对象、版本及原因后，“确认发送”固定本次授权。模型不能补字段、换负责人、换对象、刷新版本重试或从普通消息推导写权限。
6. 只有原业务提交、审计、outbox、幂等操作和运行回执一起成功，才展示已提交。创建结果是实际新 ID 与版本1；修改结果为原事项版本加一。风险转问题同时返回实际关联问题引用。
7. 原登录失效、共同受众失去阅读权限、对象版本改变或任务取消后，停止后续执行。历史已提交结果保留；取消待执行请求不会撤销此前真实提交。

Open the project conversation, select the project update task, and choose an exact operation. Review every field against the actual project members and the current parent/child version. Explicitly set or clear nullable fields. Keep state unchanged or choose an original permitted transition; realizing a risk requires confirmation that it creates a linked issue. Preparing a preview does not write business data. Sending fixes the exact instruction. Only an atomic original business commit and a matching execution receipt establish success. Later cancellation or changed authority stops subsequent execution while preserving committed history.

## 3. 参数与版本 / Parameters and versions

| 工具 / Tool | 目标和版本 / Target and version | 必填完整资料 / Required complete metadata |
| --- | --- | --- |
| `create_risk` | 父项目 ID／版本；新风险 ID 由业务生成，version=1 / Parent project; business allocates a version-one child | 共通字段＋`probability`、`mitigation` / Common fields plus probability and mitigation |
| `update_risk` | 风险自身 ID／版本，结果 version=base+1 / Risk ID/version, result base+1 | 同上，可选明确 `state` / Same, with optional explicit state |
| `create_issue` | 父项目 ID／版本；新问题 ID／version=1 / Parent project, new version-one issue | 共通字段＋`resolution` / Common fields plus resolution |
| `update_issue` | 问题自身 ID／版本，结果 version=base+1 / Issue ID/version, result base+1 | 同上，可选明确 `state` / Same, with optional explicit state |

共通字段为 `id`、`expected_version`、`reason`、`title`、`description`、`owner_id`、`severity`、`due_at`。创建不接收 `state`；更新不提供 `state` 表示保持原状态。跨类型字段、未知字段、缺项以及不允许的 null 均拒绝。风险的 `due_at`、`probability` 和问题的 `due_at` 必须显式设置或为 null。标题和原因最多200个 Unicode 字符，说明／缓解／解决说明各最多20,000个字符；完整参数最多64KiB。输入中的非配对代理字符拒绝，标题按 Go 的 Unicode 空白规则规范化。

Common fields are `id`, `expected_version`, `reason`, `title`, `description`, `owner_id`, `severity` and `due_at`. Creation accepts no state; omitted update state preserves the existing state. Cross-kind/unknown fields, omitted required fields and invalid nulls are rejected. Titles/reasons allow 200 Unicode characters; long text fields allow 20,000 each, within the 64KiB total argument limit. Unpaired surrogates are rejected; titles follow Go Unicode whitespace normalization.

示例 / Example:

```json
{
  "tool": "update_risk",
  "arguments": {
    "id": "10000000-0000-4000-8000-000000000002",
    "expected_version": 3,
    "reason": "员工已核对影响及负责人",
    "title": "交付依赖供应延误",
    "description": "已核对最新实际情况。",
    "owner_id": "10000000-0000-4000-8000-000000000003",
    "severity": "high",
    "due_at": "2027-03-05T18:00:00.123456+08:00",
    "probability": "medium",
    "mitigation": "检查替代供应方案并记录负责人。",
    "state": "realized"
  }
}
```

示例 ID 仅用于解释结构，实际请求必须从当前有权访问的业务记录选择。时刻按微秒完整核对，不以 JavaScript 毫秒截断后的相等代替实际相等。

Example IDs illustrate the structure. Actual requests select current authorized business records. Receipts compare complete microsecond timestamps, rather than JavaScript millisecond truncation.

## 4. 原状态机 / Original state machines

| 类型 / Kind | 当前状态 / Current | 允许的明确转换 / Permitted explicit transitions |
| --- | --- | --- |
| 风险 / Risk | `open` | `mitigating`, `closed`, `realized` |
| 风险 / Risk | `mitigating` | `closed`, `realized` |
| 风险 / Risk | `closed` | `open` |
| 风险 / Risk | `realized` | 只读，进入关联问题跟进 / Read only; continue in the linked issue |
| 问题 / Issue | `open` | `investigating`, `resolved` |
| 问题 / Issue | `investigating` | `resolved` |
| 问题 / Issue | `resolved` | `closed`, `investigating` |
| 问题 / Issue | `closed` | `open` |

关闭事项须先明确重新打开；问题解决或关闭须有非空解决说明。相同状态不能伪装为转换。负责人、分类和阶段继续使用原业务权限，不引入模型专用的绕过通道。

Closed concerns require explicit reopening. Resolving or closing an issue requires nonblank resolution text. Repeating the same state is not a transition. Original owner, classification and project stage rules remain authoritative.

## 5. 后端与数据约束 / Backend and data constraints

迁移 `0070_agent_project_concerns.sql` 在不改动已应用迁移的前提下扩展准确授权、提交及工具证据约束。数据库核对工具／类型／项目、事项 ID、版本、原创建人、完整字段、实际状态、业务操作和实际结果；风险发生还核对真正新建问题的字段、创建人、初始版本和关联。它不接收模型指定的新 ID。

Migration 0070 extends exact intent, commit and tool evidence constraints without editing applied migrations. It binds tool/type/project, child ID, version, original creator, full fields, actual state, operation and business result. Risk realization additionally binds the actual new issue, its fields, creator and initial version. The model never supplies the new child ID.

复用原 `CreateConcern`／`UpdateConcern` 命令及原事务。风险发生同时要求 `risk.update` 和 `issue.create`；回执或事件失败时风险修改与问题创建一起回滚。相同运行的准确重试复用原操作和结果，避免重复创建。普通手工 PATCH 同时改为区分截止时刻／概率的“省略”和“显式 null”，让清空真正可执行。

Original `CreateConcern`/`UpdateConcern` transactions retain business authority. Realization needs both `risk.update` and `issue.create`; receipt/event failure rolls back both the risk update and linked issue creation. Exact retries reuse the original operation/result. Ordinary manual PATCH also distinguishes omitted nullable fields from explicit null so that clearing works correctly.

工具读取限于本次授权对象、实际新建对象和本次真正生成的关联问题；不因此开放项目内其他事项枚举。每次复核原登录、权限、共同 AI 受众及版本。关联问题发生后续版本变更时停止执行，已提交历史仍可阅读。

Reads are restricted to the authorized concern, actual created child and actual linked issue from this run. Each read rechecks origin login, current authority, common AI audience and versions. A later linked issue version stops further execution while committed history remains readable.

## 6. 实现入口 / Implementation map

| 内容 / Area | 路径 / Path |
| --- | --- |
| 原业务 / Original business | [concerns.go](../../app/backend/internal/project/concerns.go) |
| 准确工具与回执 / Exact tools and receipts | [concern_mutations.go](../../app/backend/internal/agent/concern_mutations.go)、[mutations.go](../../app/backend/internal/agent/mutations.go) |
| 数据库约束 / Database constraints | [0070 migration](../../app/backend/migrations/0070_agent_project_concerns.sql) |
| 跨端规则 / Shared client rules | [concern-mutation.ts](../../app/packages/contracts/src/concern-mutation.ts) |
| Web／Taro 表单 / Forms | [Web](../../app/web/src/ConversationConcern.tsx)、[Taro](../../app/mobile/src/pages/secretary/concerns.tsx) |
| Skill 流程 / Skill procedure | [project-update](../skills/project-update/SKILL.md) |
| 原生集成检查 / Native integration checks | [TestAgentProjectConcerns](../../app/backend/internal/httpapi/agent_project_concerns_integration_test.go) |
| 实际页面与录屏 / Actual browser capture | [runner](../../app/scripts/run_project_concerns_browser.py)、[checks](../../app/scripts/check_project_concerns.mjs) |

## 7. 验证与上线约束 / Verification and release boundaries

本轮验证汇总及精确源绑定见 [verification.md](../quality/evidence/project-concerns/verification.md)。失败的迁移语法、测试夹具和旧数量断言按真实执行保留在该组 `attempts/`，不以最终通过结果覆盖。Web 与移动演示只调用隔离原生 API；页面发送／精确重试／取消与夹具中已经提交的原业务结果分别核对，不把预览或排队宣称为新业务写入成功。

The verification report records actual runs and exact source bindings. Failed schema/test fixture/count attempts are retained. Browser demonstrations use an isolated native API and distinguish prepared/sent/cancelled instructions from already committed fixture business results. A preview or queued run is never presented as a successful business write.

首发仍须完成 OSS/R2 的120GB真实大文件和2TB迁移恢复、境内服务器与香港用户网络、微信／抖音实机、Android实机／签名／分发、真实模型与 Telegram、依赖安全和生产运维验收。完整28项稳定编号继续见 [目标差异评估](../delivery/03-goal-gap-assessment.md)。

Launch still requires real OSS/R2 120GB transfer and 2TB migration/restore, mainland server/Hong Kong access, both mini-program device reviews, Android device/signing/distribution, real model/Telegram integration, dependency security and production operations. The complete scope remains in the stable 28-item gap register.
