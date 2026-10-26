# 资料目录整理、准确计划与原子业务执行

本增量推进 GAP-11／12／13／18，在既有项目、任务和文件正文工具上加入可实际执行的目录整理。代码位于 `app/`。适用于本人公司或项目私密会话；Web 可选择目录拟定计划，Web／Taro 可查看、批准、执行、撤销并读取回执。验收记录见[本次验证](../quality/evidence/secretary-organization/verification.md)，总体未完成项继续见[28项差异表](../delivery/03-goal-gap-assessment.md)。

## 1. 业务范围

| 能力 | 实现约束 |
| --- | --- |
| 盘点目录 | 指定一个实际有权目录；只列最多50个可见直接子项，不递归；数量超限显示尚未盘点完整目录 |
| 新建目录 | 在已有父目录下创建，固定父目录版本，继承其项目归属和分类 |
| 重命名 | 目录、文件均固定原对象及基础元数据版本，名称沿用原有存储名称校验 |
| 移动 | 原对象与目标目录同项目、同分类，固定目标版本；不能将项目根目录移动或形成目录环 |
| 多项整理 | 每份1至10项；父目录调整及依赖该父目录的动作拆成后续计划；新建目录不能以临时占位符供同批引用 |
| 内容与权限 | 本增量仅改命名空间；不产生文件内容修订，不删除原件，不改分类、成员、对象授权或保留策略 |

“归档资料”可作为目标目录名称，表示资料位置调整。合同、项目或文件生命周期的归档状态仍使用各自原业务流程。跨项目、跨分类、共享会话、TG、标签和批量删除尚未开放。

## 2. 准确计划状态

`proposed(v1) → approved(v2) → committed(v3)`；proposed／approved 可进入 revoked 终态。拟定与批准均不移动资料。

1. 服务端读取原对象、父路径与目标目录，固定 ID、名称、分类、归属和版本。请求不能自行提供预览替代真实资料。
2. 计划同时固定发起人、其认证版本、组织权限版本、会话修订、安全版本、可选原始运行以及30分钟有效期。以规范JSON计算摘要；数据库检查摘要及执行字段与原始意图一致。
3. 只有原发起人能查看和处理该计划。批准必须来自有效交互会话，最近10分钟完成MFA，且具有 `agent.plan.approve`。原 `agent_plan_operator` 角色提供计划动作；每一项仍要求原业务权限。
4. 提交要求准确计划版本和摘要，原批准事件存在，权限、路径、分类、对象版本和目标版本仍有效。来源任务中断、失败、取消或过期时不可继续使用其未执行计划。
5. 人工提交同样要求近期MFA。模型仅可消费本人已批准的准确计划，工具目录中没有批准接口。
6. 再次读取按当前权限检查全部相关对象；已失去权限或AI处理许可时隐藏旧计划数据，不能借已保存预览继续泄露资料。

计划意图、事件和目录观察受数据库不可变约束保护。计划头更新须具有连续的准确事件历史，批准事件须有真实会话MFA证据，提交事件须匹配每项实际成功业务回执。

## 3. 原子提交与幂等

所有项目资料命名空间变更沿用项目锁；公司资料沿用原命名空间锁。批准后重读全批原始依据，再在同一个PostgreSQL事务中调用 `CreateFolder`、`MoveFolder`、`MoveFile`。权限判断、乐观版本、名称冲突、目录环和项目只读规则继续由原业务服务执行。

内部业务路由使用 `/internal/agent-organization/{kind}`，每项幂等键为 `agent-organization:{plan_id}:{index}`。它们是服务内部协调标识，没有对应可绕过鉴权的公开接口。

任一命令或计划事件写入失败，全批目录修改、逐项幂等记录、审计、Outbox及计划状态一起回滚。失败的计划保留批准状态及空结果，不能显示部分完成。同名冲突应核对资料并重新拟定；网络结果不明则使用原请求核对回执。

计划 `operation_id` 是第一项真实业务回执，作为批次锚点；`results` 保存全部逐项 `operation_id` 与原业务返回数据。它并不是额外虚构的总操作。成功后重复提交准确已批准内容返回相同结果，不再次创建或移动资料。

## 4. API与秘书工具

| 方法 | API | 用途 |
| --- | --- | --- |
| GET | `/v1/conversations/{id}/directories/{folderID}` | 当前会话有权资料的有限目录视图 |
| GET／POST | `/v1/conversations/{id}/organization-plans` | 最近50份本人计划／拟定准确计划 |
| GET | `/v1/organization-plans/{id}` | 按当前权限读取状态及逐项预览 |
| POST | `/v1/organization-plans/{id}/approve` | 本人MFA批准 |
| POST | `/v1/organization-plans/{id}/revoke` | 本人撤销 |
| POST | `/v1/organization-plans/{id}/commit` | 原子执行已批准内容 |

本次增加6条路径、7项操作。请求、响应及类型由[OpenAPI](openapi.json)与生成的共享合同约束。

| 工具 | 输入／结果 |
| --- | --- |
| `list_directory` | `folder_id`；返回有权直接子项、是否超限及当时观察记录 |
| `propose_organization` | `actions`、`reason`；服务端固定会话和真实预览，返回pending |
| `commit_organization` | `plan_id`、`expected_version`、`plan_hash`、`reason`；只能消费原准确批准 |
| `read_resource` | `type=agent_organization_plan`及准确ID；按字符分页读取原计划，并返回当前状态 |
| `inspect_operation` | 当前 `run_id`及可选 `plan_id`，核对计划和真实逐项回执 |

动作 kind 为 `folder.create`／`folder.move`／`file.move`。`id` 在新建时代表已有父目录，其他动作代表源对象；`expected_version` 是对应对象基础版本。`name` 指新名称；有 `target_folder_id` 时必须同时提供 `expected_target_version`。

目录观察单独存储不可变快照，来源类型 `agent_directory`，含观察时间与摘要；它证明当时列出的资料，不保证当前名称或位置。后续重放重新校验所有引用对象权限。计划来源 `agent_organization_plan` 在执行后仍可核对原意图；引用本身不构成批准或新的写入授权。

运行时、MCP工具目录、原生约束所需工具集合及观察事件白名单均接入 information-organize。只有本次运行内真实 `commit_organization` 成功凭证和对应操作引用，才能将结果标为completed；盘点、拟定、读取或批准均不足以证明执行完成。

## 5. 页面和运行

Web会话显示“资料整理计划”，可浏览当前范围内的根目录和子目录、选择资料、新名称与目标目录，最多加入10项，保存后查看真实原／目标路径和版本。未确认结果的请求保留原始内容及幂等键。

Taro会话接入同份计划的逐项查看、近期MFA批准、撤销、执行和回执刷新；公共页面数量仍为13个。首期手工计划编辑器在Web，移动端可请已配置秘书拟定后查看处理。微信／抖音编译、H5页面验证与Android真机验收分别记录。

原生开发库追加迁移 `0054_agent_organization.sql`，总计54项。新增 `agent_organization_plans`、`agent_organization_events`、`agent_directory_snapshots`；应用角色仅能追加事件／观察，不能更新或删除；计划头允许受约束更新，不能删除。

Skill源码包 information-organize 为 `1.1.0`、draft；实际SOP见[资料整理Skill](../skills/information-organize/SKILL.md)。启用真实模型仍需实际执行器约束、发布评测、准确版本安装及权限配置。本次没有将八个draft包直接改为已发布，也没有创建真实模型、TG或云存储授权。

## 6. 验证与剩余范围

针对本增量验证实际业务提交、同名冲突整批回滚、重复请求、旧版本／目标变化、最近MFA、摘要错配、他人访问、撤销、过期、来源中断、目录环、跨范围、父子依赖、50项盘点和10项计划上限、AI许可撤销及数据库不可变／连续历史约束。秘书链路使用真实MCP／控制HTTP和原生隔离PostgreSQL，约束签发器和云原件使用受控测试实现。

本地回归和真实页面结果、来源摘要、临时服务清理见[验证记录](../quality/evidence/secretary-organization/verification.md)。它们不代替真实模型、OSS／R2、TG、设备、120GB传输、2TB导入、境内到香港网络或生产恢复验收。GAP-11／12／13／18保持部分实现，下一步仍需补文件创建、其余业务工具、技能评测发布和真实运行接入。
