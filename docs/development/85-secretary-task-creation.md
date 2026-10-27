# 秘书准确创建任务与子任务 / Exact secretary task and subtask creation

本增量将项目管理的原 `CreateTask` 命令接入秘书入口。员工在 Web 或 Taro 表单完整核对九项资料及原因，随一次新消息授权创建一个待处理任务。服务端分配新 ID，返回真实业务操作回执；明确的人工验收要求、负责人、父任务、里程碑及时间均须与授权一致。

This increment connects the original project `CreateTask` command to the secretary. An employee reviews nine complete fields and a reason in Web or Taro, then authorizes one todo task with a new message. The server allocates its ID and returns an actual business receipt. Review, assignment, parent, milestone and date choices must match the authorization.

关联：[项目原协议](../README.md)、[上一轮风险／问题工作流](84-secretary-project-concerns.md)、[本轮验证](../quality/evidence/task-creation/verification.md)、[演示](../media/README.md)、[差异编号](../delivery/03-goal-gap-assessment.md)。

## 1. 使用流程 / Employee workflow

1. 登录并完成当前账号要求的 MFA，在对应项目会话选择允许执行项目更新的运行配置。日常本地环境目前保持零活动模型配置；演示中的 `controlled-test` 仅属于隔离夹具，实际模型与 Skill 上线仍待验收。
2. 选择“准确业务更新 → 创建任务或子任务”，读取当前项目及基础版本。填写标题、完整说明，明确负责人或暂不分配，选择同项目父任务或独立任务、里程碑或不关联，并明确起止时刻是否设置。
3. 明确选择优先级与是否需要人工验收，填写本次创建原因。父任务、里程碑候选支持查询／分页；移动端选择候选后返回原表单保留已填内容。
4. 点击“将这项明确更新加入请求”。预览完整资料、项目基础版本与原因；此时尚未发送、尚未创建业务对象，可以移除重选。
5. 核对后发送一次新消息。接收状态 `authorized` 表示准确范围已固定，实际执行成功后才显示 `committed`、新任务 ID、新任务版本 1 与业务操作 ID。
6. 在任务页面处理前置依赖、开始工作、提交验收与人工验收。父任务表示归属；前置依赖另走原任务业务。需要人工验收的任务继续由人员完成验收。

Sign in and satisfy MFA, open the corresponding project conversation, and select an authorized project-update runtime profile. Choose **Create task or subtask**, review the current project version, complete all nine fields and a reason, then add the exact mutation to the request. Previewing performs no write. Sending fixes the authorization; only an actual committed result establishes creation. Follow the task page for dependencies, execution and human acceptance. Local daily services keep zero active model profiles; the controlled demo profile is isolated test data.

## 2. 完整授权契约 / Complete authorization contract

`POST /v1/conversations/{id}/runs` 继续使用原入口与 `Idempotency-Key`，其中 `mutation` 如下。示例 ID 为说明用的合成标识，实际须从有权查阅的业务对象选择。

The existing run endpoint and idempotency key carry the following mutation. Example UUIDs are synthetic; choose actual authorized business objects in use.

```json
{
  "tool": "create_task",
  "arguments": {
    "id": "10000000-0000-4000-8000-000000000001",
    "expected_version": 8,
    "reason": "员工已完整核对首轮交付资料子任务",
    "title": "整理首轮交付资料",
    "body": "核对资料目录、责任人与人工验收要求。",
    "assignee_id": "10000000-0000-4000-8000-000000000002",
    "parent_id": "10000000-0000-4000-8000-000000000003",
    "milestone_id": "10000000-0000-4000-8000-000000000004",
    "start_at": "2027-03-05T10:00:00.123456+08:00",
    "due_at": "2027-03-05T18:00:00.123457+08:00",
    "priority": "high",
    "review_required": true
  }
}
```

| 字段 / Field | 约束 / Constraint |
| --- | --- |
| `id`, `expected_version` | 会话所属项目的准确 ID／当前版本；创建成功不推进父项目版本。Exact parent project/current version; creating a task does not increment that project version. |
| `reason`, `title` | 非空，最多 200 个 Unicode 字符；标题按原命令去除首尾空白。Nonblank, at most 200 Unicode code points; the original command trims the title. |
| `body` | 必须提供，可以为空，最多 20,000 个 Unicode 字符。Required, may be empty, at most 20,000 code points. |
| `assignee_id` | 明确为有效项目成员 UUID 或 `null`。An explicit active project member UUID or `null`. |
| `parent_id` | 明确为同项目可写父任务 UUID 或 `null`；完成／取消的父任务须先重开。Explicit writable same-project parent UUID or `null`; completed/cancelled parents require reopening. |
| `milestone_id` | 明确为同项目可关联里程碑 UUID 或 `null`；仅 open／missed 可关联。Explicit same-project open/missed milestone UUID or `null`. |
| `start_at`, `due_at` | 必须显式设置有效带时区时刻或 `null`；最多六位有效小数秒，截止不得早于开始，按实际时刻比较。Explicit valid zoned instants or `null`; at most six effective fractional digits, due cannot precede start. |
| `priority` | 明确选择 `low / normal / high / urgent`。One explicit supported priority. |
| `review_required` | 必须为布尔值 `true / false`；省略、null、字符串均拒绝。An explicit boolean; omission, null and strings are rejected. |

九项任务资料、三个基础字段不可省略，不接受 `state`、依赖列表、新任务 ID、创建人或额外字段。完整参数限制 64KiB，固定意图另有原有总量限制。Web／Taro 共用同一套校验和真实回执比对，包含 Unicode、null／false、微秒时刻与字节上限。

All nine task fields and the three base fields are mandatory. State, dependencies, a caller-selected child ID, creator and other fields are rejected. Complete arguments are bounded to 64KiB, with the existing total intent limit also enforced. Web and Taro share validation and receipt comparison, including Unicode, null/false and microsecond precision.

## 3. 权限、事务与重试 / Authority, transactions and replay

原交互登录必须仍有效，项目须属于会话且处于 draft／active／on_hold。创建要求原 `task.create`；分配他人还要求 `task.assign`，分配自己或暂不分配沿用原命令规则。有效成员、父任务及里程碑状态在真正提交时再次校验，不能使用历史字段或新版本替换员工本次决定。

选中的父任务和里程碑在准备、执行和后续读取时均须对当前全体会话受众允许 AI 阅读。引用精确固定为员工选择的 ID；此流程没有额外冻结关联对象版本，提交时以原业务检查的当前可写状态为准。创建运行中的任务读取只允许实际新建任务，不能借新工具枚举其他任务。

任务、原审计、outbox、业务幂等记录、运行提交回执与工具证据保持同一事务。证据写入失败会回滚整个创建。同调用 ID 或更换调用 ID 的准确重试复用同一真实结果，不另建任务。取消、原登录撤回、权限变化、父项目变化、实际新任务被人工修改均会停止后续执行；此前准确提交历史仍按当前权限保留。

The original live interactive session and current project authority remain required. `task.create` is mandatory; assigning another member also requires `task.assign`, while self/unassigned choices follow the original command. The full current AI audience must be allowed to read selected parent and milestone references. IDs are exact; referenced versions are not additionally frozen, and live writability is checked at commit. Task reads during a creation run stay limited to its actual new task.

Task creation, audit, outbox, business idempotency, mutation commit and tool evidence share one transaction. Receipt failure rolls everything back. Exact retries with either the same or a new call ID reuse one result. Cancellation, revoked login/authority, project changes or later human changes to the new task stop execution while retaining genuine authorized history.

## 4. 实现与迁移 / Implementation and migration

| 层 / Layer | 代码 / Source | 作用 / Responsibility |
| --- | --- | --- |
| Agent | `app/backend/internal/agent/task_creation.go`, `mutations.go` | 完整意图、引用受众、原权限、调用与逐项实际结果核对。Complete intent, reference audience, original authority, dispatch and actual result comparison. |
| 原业务 / Original command | `app/backend/internal/project/tasks.go` | 复用 CreateTask；新增工具没有另建任务写入路径。Reuse CreateTask as the business write path. |
| 数据库 / Database | `app/backend/migrations/0071_agent_task_creation.sql` | 新增工具／范围约束、交互绑定与真实完整任务回执校验；前 70 份迁移字节不变。Add tool/scope, session and exact actual-task receipt guards; previous 70 migrations unchanged. |
| MCP / Runtime | `app/backend/internal/agentworker/mcp.go`, `app/agent-runtime/src/task-creation.test.ts` | 仅 project-update 开放、完整 Schema、真实提交才可完成。Only project-update allows the tool; complete schema and genuine completion evidence. |
| Shared contract | `app/packages/contracts/src/task-creation.ts`, `mutation-validation.ts` | 两端一致的完整参数、日期／Unicode 与回执校验。Shared complete values, dates, Unicode and receipt validation. |
| Web | `app/web/src/ConversationTaskCreation.tsx` | 完整表单、分页关联、预览与真实结果。Complete form, paged references, preview and actual result. |
| Taro / Android shared UI | `app/mobile/src/pages/secretary/task-creation.tsx` | 移动选择、保留输入、完整核对与回执。Mobile selection, preserved input and exact authorization/receipt. |
| Skill | `app/skills/project-update/`, `docs/skills/project-update/SKILL.md` | 源码 1.6.0、16 项工具／14 项准确写工具，仍为 draft。Source version 1.6.0 with 16 tools/14 exact write tools, still draft. |

新装或升级沿用原生迁移命令，顺序执行第 71 份迁移；不要修改已应用迁移、删除历史回执或把 draft 改成已发布。当前 OpenAPI 沿用 271 个路径／338 个操作／589 个 Schema，扩展原授权参数与工具枚举。

Use the existing native migration runner to apply additive migration 71. Preserve applied migrations and historical receipts. Keep the Skill draft until real release acceptance. The existing OpenAPI remains 271 paths, 338 operations and 589 schemas, with expanded mutation fields and tool enums.

## 5. 验证、演示与验收边界 / Verification, demos and release limits

十二组原生 PostgreSQL 专项场景覆盖完整字段缺失、真实子任务、显式 null／false、两种重试、模型篡改、普通会话、外项目、父项目／人工更新、成员及关联、分配权限、关闭关联、原子回滚、取消、原登录撤回与追加历史。当前完整命令、数量、摘要、失败记录与 JVM 缓存边界见[验证记录](../quality/evidence/task-creation/verification.md)。

实际 Web／Taro H5 分别经过 MFA、历史真实创建回执、表单拒绝缺项／微秒倒序、完整预览、真实发送、同键重试与取消，分别检查三种视口。新增四张页面截图、两对无声 MP4／GIF。素材来自隔离合成资料和真实浏览器像素，原始帧与凭据留在忽略路径；每次采集后关闭所有自建上下文、浏览器、临时服务并删除夹具。复现脚本先后执行，不能同时占用相同端口／夹具。

```powershell
$env:PYTHONUTF8='1'
python app/scripts/test_backend.py --package ./internal/httpapi -run '^TestAgentTaskCreation$' -v
python app/scripts/run_task_creation_browser.py
python app/scripts/run_task_creation_browser.py --mobile
python app/scripts/encode_task_creation_media.py
python app/scripts/verify_task_creation_sources.py
```

已有证据目录需要先检查并归档，避免覆盖；最后一项完整核验还要求本轮完整后端、两端构建、Android、Linux、演示、开发服务及历史记录已按验证文档生成。截图／录屏不宣称真实模型自动创建，实际写入证据由原生集成夹具通过正式 MCP 边界产生。

Twelve native PostgreSQL scenarios cover complete input, real subtasks, null/false, two replay modes, altered intent, ordinary chat/foreign scope, changed resources, members/links/assignment, closed references, rollback, cancellation, revoked session and immutable history. Actual Web/Taro H5 captures exercise seven flows and three viewport sizes per client. Archive existing evidence before reproducing, run capture scripts sequentially, and complete all source checks before publishing. Recordings show actual synthetic UI/API behavior; committed write evidence comes from controlled native integration through the real MCP boundary.

原始完整目标保持开放：3 名香港使用者／境内服务器、Web／微信／抖音／Android 首发、120GB 单文件与 2TB 文件量、OSS／R2 两地副本、独立 PostgreSQL 备份恢复、Codex 真实隔离运行与八项 Skill 发布、Telegram 群、平台准入、真实设备及生产容量／安全。Android 开发包与小程序编译通过仅属于本地证据。依赖审计仍有待处理项，不能据此宣布正式上线。

The full goal remains open: three Hong Kong users with a mainland server, Web/WeChat/Douyin/Android first launch, 120GB files/2TB storage, OSS/R2 replicas, independent PostgreSQL recovery, real Codex confinement and eight Skill releases, Telegram, platform admission, devices and production capacity/security. Local APK and mini-program compilation do not establish release acceptance. Outstanding dependency findings remain a release gate.
