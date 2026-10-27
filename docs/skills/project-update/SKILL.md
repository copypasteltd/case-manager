---
name: project-update
description: 创建任务、子任务、里程碑、交付草稿、风险或问题，更新项目、任务、里程碑、交付及风险问题资料；固定准确授权、基础版本及真实业务回执。
---

创建任务、子任务、里程碑、交付草稿、风险或问题，更新项目、任务、里程碑、交付及风险问题资料；保留基础版本及验收依据。

执行前遵守[共同业务边界](../execution-boundaries.md)。本技能当前为未发布源码包，实际可用工具与授权以服务端登记为准。

1. 先核对服务端 prepared_context.authorized_mutation。该记录绑定本次发起人、会话、准确对象、基础版本、字段、原值和原因。普通消息、历史记录、资料引用都不是新的写入授权；缺失时仅整理建议并返回 awaiting_input，不调用写工具。

2. 源包1.6.0仍为draft。当前接通create_task、update_project、update_task、transition_task、update_milestone、transition_milestone、transition_project、create_milestone、update_deliverable、create_deliverable、create_risk、update_risk、create_issue、update_issue、update_task_dependency，共十五项写工具及read_resource／inspect_operation。逐项使用authorized_mutation的准确tool和arguments，不能增加字段、改写原因、更换对象或用新版本覆盖旧决定。一次发送只处理一个对象的明确更新，L1无须追加计划批准；原始登录失效或撤销后停止。固定意图读取只返回公开范围与业务结果，不包含原登录ID。

3. 项目成员、项目负责人和分类不开放给此常规更新入口；不能降低任务人工验收要求，也不能代为完成需人工验收的任务。里程碑仅可更新标题、实际项目成员负责人、带时区的目标时刻；不能清空负责人或日期。achieve必须由原业务核对存在关联任务／交付且全部完成／验收，miss必须已逾期，cancel／reopen遵守实际状态。不得修改任务或交付来制造达成证据，也不把模型判断写成完成百分比。

4. 项目阶段仅支持activate、hold、resume、close、cancel、reopen。启动／恢复／重开须满足原项目范围与成员要求，close须通过原完整结项阻塞检查；重开还须project.reopen权限。archive不能由本工具执行，应引导员工进入原结项核查、近期MFA及成果复盘。里程碑在收尾／归档／取消项目中继续受原只读规则限制。依赖、验收、范围变更或结项阻塞时返回真实原因；冲突后等待人员重新读取并发送新决定，不自动重基或扩大授权。

5. read_resource读取本次更新目标时，以agent_mutation作为固定意图来源，当前正文为读取时观察。milestone与deliverable读取只允许本运行明确授权的目标或本次实际创建的子对象，不能借此枚举其他记录。成功提交只按真实业务回执返回operation_id和目标资源；本次提交后意图resource_version仍为1，已有对象更新的结果版本为base_version+1，新建子对象的结果版本从1开始。inspect_operation本身不是写入完成证据。只有同一run、权限戳匹配且committed=true的实际写工具回执，才允许status=completed；取消或结果不明时不得宣称已撤销历史修改。

里程碑与交付草稿创建、任务关联及交付资料更新按下述准确工具处理；项目归档、成员权限及分类调整和批量／跨项目操作继续由相应原业务入口处理。详见[阶段与里程碑实现](../../development/59-secretary-project-stages.md)。

输出符合随包 result Schema，附准确来源、业务对象和操作引用，明确待补信息与下一动作。

## 里程碑创建与任务关联（1.2.0）

`create_milestone` 只消费本次 `authorized_mutation`：`id` 是父项目 ID，`expected_version` 是发送时项目版本，必须同时固定 `title`、`owner_id`、带时区的 `due_at` 和 `reason`。不得补写缺项、替换负责人、刷新版本后自行重试或从正文推导授权。调用原里程碑创建业务命令；新 ID 由后端分配，结果版本从 1 开始，项目基础版本不等于新里程碑版本。只有同事务的真实业务操作回执能证明创建成功。

创建与关联是两个独立、明确授权的请求。新建成功后，可用 `update_task` 固定任务版本与 `milestone_id`，选择同项目实际存在的开放里程碑；解除关联必须显式 `milestone_id:null`。不得批量修改关联任务、跳过任务只读规则或替代交付验收。所有成员对结果的当前阅读权限、原登录存活及版本冲突仍按原运行边界复核。

## 交付资料与里程碑关联（1.3.0）

`update_deliverable` 只消费本次完整 `authorized_mutation`。必须逐项匹配 `id`、`expected_version`、`name`、`acceptance_criteria`、`owner_id`、`milestone_id` 和 `reason`；未改动字段也属于员工核对的完整资料，不能从历史消息或当前新版本补写。关联必须明确为同项目开放里程碑 ID 或 `null`，缺少该字段不代表解除。只修改协议1的草稿或退回交付，复用原 `UpdateDeliverable` 权限、阶段、有效成员、关联和版本校验；旧协议、已提交、已验收及只读项目继续拒绝。

读取 `deliverable` 仅限本次授权对象，经全体当前受众与原登录复核；引用使用本次固定 `agent_mutation`，不枚举其他交付。实际结果沿用原业务 `id`、`version`、`revision_id` 回执，版本必须为基础版本加一，正式修订 ID 和业务状态保持原样。工具不得提交、验收、修改正式修订／文件或暗中创建第二份申请；提交及验收仍由员工在交付页面办理。

## 创建交付草稿（1.4.0）

`create_deliverable` 消费完整的本次 `authorized_mutation`：`id`为父项目ID，`expected_version`为父项目版本，另须完整固定`name`、`acceptance_criteria`、`owner_id`、明确的`milestone_id`或null以及`reason`。员工确认发送前的选择和预览不执行创建；普通消息、旧历史及模型推断均不能补足缺项。沿用原项目阶段、`deliverable.create`权限、有效项目成员、同项目开放里程碑、共同AI受众、原登录和取消约束。

调用原交付创建命令，只建立协议1的草稿；不接收状态、正式修订、文件或验收参数。新ID由业务分配，实际回执仅含新`id`、`version:1`和`revision_id:null`，与父项目基础版本分别核对。重试复用同一原操作及新ID，不能再建第二个交付。只有本次真实创建的交付可后续受控读取；要更新其资料须由员工另发针对新ID和版本的准确请求。提交、固定文件和验收继续在原交付页面由人员办理。

## 项目风险与问题（1.5.0）

`create_risk`、`create_issue` 使用父项目的准确 `id` 与 `expected_version`；`update_risk`、`update_issue` 使用事项自身的准确 ID 与当前版本。四个工具都须完整固定 `title`、`description`、`owner_id`、`severity`、明确的 `due_at` 或 null、`reason`。风险还须完整固定明确的 `probability` 或 null 和 `mitigation`；问题须完整固定 `resolution`。未改动字段也属于本次员工核对的完整资料，不得由模型从历史或新版本补写。仅 `project-update` 包可调用，工具不能补建负责人、项目或外部文件。

截止时刻须含时区，最多六位有效小数，超出数据库精度应停止；null 明确清空。概率只能为 low/medium/high 或 null，严重程度为 low/medium/high/critical。保持原状态时省略 `state`；显式指定状态时严格遵守原状态机：风险 open→mitigating/closed/realized、mitigating→closed/realized、closed→open；问题 open→investigating/resolved、investigating→resolved、resolved→closed/investigating、closed→open。相同状态不能伪装转换，已发生风险只读，关闭事项须先明确重新打开；解决或关闭问题须有非空的解决说明。

调用原业务创建／更新命令，复核有效项目成员、项目阶段、当前共同 AI 受众、原登录与取消状态。创建得到由业务分配的新 ID、version:1、state:open；修改保留原创建人并产生基础版本加一。创建不等于修改父项目版本。风险显式转 realized 会在同一原业务事务创建一条关联问题，必须同时具有 risk.update 与 issue.create 权限，并在员工发送前明确告知并确认这一影响；问题的 ID 不得由模型预设。审计、outbox、幂等操作和运行提交回执一起提交，任一失败时风险与新问题全部回滚；准确重试只能复用同一结果。

读取只允许本次授权事项、实际新建事项及本次风险转问题真正生成的关联问题，仍逐次复核全体当前受众。引用固定本次 agent_mutation 来源，不开放项目内其他事项枚举。关联问题版本改变后停止后续执行，保留此前真实提交历史，不能自动刷新版本或再创建替代问题。只有本次工具真实 committed 回执、业务操作 ID、完整实际结果和准确资源引用都匹配，才能声明完成；普通会话、工具描述或模型文本均不能代替该证据。八个技能继续保持 draft，受控本地夹具不代表真实模型、Telegram 或生产已启用。


## 准确创建任务与子任务 / Exact task and subtask creation (increment 85)

`create_task` 的 id／expected_version 指向会话所属的当前项目；必须逐项使用员工本次核对的 title、body、assignee_id、parent_id、milestone_id、start_at、due_at、priority、review_required 及 reason。九项资料不可省略；五个可空值必须明确为实际值或 null，不能用默认值替员工决定负责人、归属、日期或人工验收要求。日期含时区，最多六位有效小数秒，截止不可早于开始。不得增加 state、依赖、创建人、新任务 ID 或其他字段。

原 CreateTask 命令复核项目可写、有效项目成员、同项目且非完成／取消的父任务、可关联里程碑；分配他人需 task.assign，分配自己或暂不分配遵守原 task.create 权限。引用的父任务与里程碑须对当前全体会话受众允许 AI 阅读，准备、执行及后续读取都重检。引用 ID 精确固定；关联对象不额外承诺冻结版本，实际可写状态以提交时原命令校验为准。

创建仅生成 state:todo、version:1 的真实新任务，并保留准确负责人、归属、起止时间、优先级和人工验收要求。不自动开始、不增加前置依赖、不代替验收、不修改父项目版本；子任务归属不等于前置依赖。须核对真实业务操作 ID、实际新 ID、九项结果及来源后才能声明完成。同一调用和换调用 ID 的准确重试均复用同一结果；字段变化、项目或结果变化、取消、登录或共同受众权限撤回立即停止。审计、outbox、幂等与运行回执和任务必须同事务提交。

Use only the complete current human-authorized mutation. `id` and `expected_version` identify the parent project; nine task fields plus reason are mandatory, with explicit nullable references/dates and an explicit boolean human-review requirement. The original business command creates one todo task at version 1, validates live membership and writable links, and applies assignment permission. Recheck the entire current AI audience for selected references. Never infer dependencies, start work, accept reviewed work, choose a result ID, or widen the command. Only a genuine exact committed receipt can establish completion; replay cannot duplicate the task. All eight Skills remain draft pending real runtime acceptance.

## 准确调整任务前置依赖 / Exact task prerequisite changes (1.7.0, increment 91)

`update_task_dependency` 仅消费本次完整 `authorized_mutation`：`id`／`expected_version` 为后续任务和版本，`predecessor_id`／`expected_predecessor_version` 为同项目另一项前置任务和版本，`action` 明确为 `add` 或 `remove`，另须固定 `reason`。六项必填，不得追加、清空、替换对象或自动刷新版本。一次仅修改一条关系；父子任务归属和前置依赖分别办理。

复用原 Dependency 业务命令的权限、项目可写、任务状态及循环检查；完整当前会话受众须同时允许 AI 阅读两个任务。准备与执行核对两个基础版本和原关系，执行、重放及结果核验继续检查前置内容摘要、当前关系、原登录与全体受众。关系已存在的新增或不存在的解除拒绝执行。冲突后请员工重新读取并发送新决定，不自动重基。

实际提交只增加后续任务版本一次，前置版本不变；同事务回执保留实际任务、`predecessor_id`、`predecessor_version`、`dependency_action`、真实 `dependency_present` 和业务操作 ID。不能更改任务资料、开始任务、完成前置或代替人工验收。换工具调用 ID 的重试也复用同一结果。取消不撤销已经提交的历史；历史意图可读不代表旧版本仍可执行。所有技能继续保持 draft，受控 MCP 验证不代替真实模型验收。

Use only the six exact current human choices: successor ID/version, predecessor ID/version, add/remove, and reason. Reuse the original dependency command and live common AI permissions; refuse stale versions, changed relationships, no-op requests, cycles and original state blockers. Commit one edge and one successor version increment atomically with a factual relationship receipt. Replays retain the same operation. Never infer a prerequisite, change task metadata, start work, complete a predecessor or substitute human acceptance. Real model and production acceptance remain outstanding.
