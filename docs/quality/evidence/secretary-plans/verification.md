# 项目计划、本人批准及真实创建验证

验证日期：2026-10-25（Asia/Taipei）。本阶段推进 GAP-11／12 的首条业务链：本人公司私密会话拟定固定项目计划，经本人 MFA 批准后，通过既有项目服务原子创建项目并取得真实业务回执。代码、契约、Web 和 Taro 公共页面均已实现，协议见[项目计划开发文档](../../../development/48-secretary-project-plans.md)。

## 1. 固定来源验证

| 验证 | 结果 | 证据及实际范围 |
| --- | --- | --- |
| 完整后端 | 13批，103个自动发现的顶层测试，638条结果，577个叶场景；无失败或跳过 | [完整结果](backend-result.json)、[逐批账本](backend-batch-progress.json)；原生 Windows Go／PostgreSQL，不是单个 Go 进程运行全部测试 |
| 项目计划及 MCP | 拟定不创建、准确摘要／版本／人员、近期 MFA、失效／撤销、模板变更、事务失败回滚、并发单次创建、真实回执、跨轮准确计划引用、取消及历史撤权通过 | 上述完整后端中的 `TestAgentPlans`、`TestAgentProjectTools`；后者使用实际本地 HTTP／MCP 协议及受控原生证明验证器，未调用真实模型或实际隔离启动器 |
| Web | 26项测试及生产构建通过；7条实际页面／API链通过 | [构建](web-build.json)、[页面操作](../secretary-plans-web/result.json)及[截图](../secretary-plans-web/plans.png)；含批准前拒绝、两请求只创建一次、撤销、项目跳转、仅截止日期与准确有效时刻展示 |
| 移动 | 58项测试、类型、微信／抖音编译通过；4条实际 Taro H5／API链通过 | [编译](mobile-build.json)、[页面操作](../secretary-plans-mobile/result.json)及[项目截图](../secretary-plans-mobile/mobile-plan-project.png)；含拟定、本人批准、真实提交与项目跳转 |
| 运行时 | 20项检查及 Skill 包校验通过 | [运行时结果](runtime-build.json)；验证 pending 计划引用不被当作创建完成，工具提交凭证与真实 operation ID 对应；八个实际 manifest 仍为 draft |
| Linux | linux/amd64 交叉编译通过 | [构建记录](linux-build.json)；未在 Linux 执行或部署该产物 |
| 开发环境 | 49项迁移及摘要匹配，10条健康／拒绝探测与6条新计划未认证拒绝探测通过；6项计划数据库守卫有效 | [执行环境核对](development-runtime.json)、[计划保护核对](development-plans.json)、[自有服务记录](development-services.json)；应用身份不能删除计划或改写事件，实际活动运行配置0 |
| 最终核对 | 14项全部通过 | [来源与清理](final-source-check.json)；当前源码与后端／运行时／Web／移动／Linux及两组页面证据匹配，两份 OpenAPI 相同，隔离库0、私有夹具不存在、5189释放，原5178页面／就绪代理及18080 API就绪均200 |

最终后端 SHA-256：`4842354c2e267a3fcb841babe6f3698d9c4d9cde6070023162a273acffafcd65`。各前端、运行时与页面的独立来源摘要保存在相应 JSON 中。接口契约现有234个路径、297个操作、493个 Schema；端点数量仅用于追溯，不作为需求完成率。

## 2. 修正与失败记录

- [最初清理超时](attempts/01-backend-batch-5-cleanup-timeout.json)：`TestDurableJobLeases` 的失败发生在测试库 DROP 清理，已纠正早先将其称为连接建立超时的记录。该批原来源重跑通过，后来因代码修正不再计入最终来源的验证。
- [MCP 参数修正](attempts/02-mcp-context-binding/finding.json)：原工具要求模型提供执行上下文未包含的会话版本。改为由 Go 从已绑定任务读取准确版本，负责人省略时拟定为本人。原来源部分批次全部归档，最终13批从新来源重新运行。
- [原生检查点超时](attempts/03-native-checkpoint-timeout/finding.json)：全部5个业务子场景通过，但一次56.438秒的原生 PostgreSQL 落盘检查点超过45秒 DROP 清理限时。仅将夹具清理预算改为2分钟，浏览器所属夹具按短间隔等待释放；未关闭 fsync 或修改生产持久化设置。
- [准确遗留库清理](owned-database-cleanup.json)：依据本工作区原生集群、准确 DROP 日志、OID、所有者及零连接核对，移除该失败清理遗留对象；未选择应用库或其他活动夹具。原始私有日志留在忽略目录，失败结构记录按实际保留范围描述，不宣称原日志已全部归档。
- 移动 H5 首次来源清单不一致的尝试保存在移动证据的 `attempts/` 中；后续按同一完整清单重跑通过。历史尝试不作为当前源码已通过的证据。

## 3. 操作入口与限制

本地入口为 `http://localhost:5178/`。管理员使用已有角色管理为指定员工绑定 `agent_plan_operator`，并保留其会话运行及项目创建权限；从“公司秘书 → 本人公司私密会话 → 项目创建计划”人工验证。角色没有自动授予员工。项目会话、共享会话及 TG 群不提供这项批准入口。

MCP 本阶段只为 `project-create` 增加 `propose_project` 和 `create_project`，连同原3项只读工具组成5项准确集合。模型没有批准工具；项目、目录初始化、模板任务／里程碑、幂等记录、计划、审计及 outbox 在同一事务提交。人工 UI 可以直接提交批准计划。跨轮受控测试显式提供准确 `plan_id`，当前尚无计划目录或自动恢复历史计划上下文；不能据此宣称自然语言长会话记忆已经实现。

本阶段未发布实际 Skill、启动实际秘书运行配置、调用真实模型、部署生产、调用真实云／TG或完成微信／抖音／Android真机验收；未调用 Docker、WSL、虚拟机或模拟器。项目更新、文件／目录／OA等其余工具、计划发现与预授权、Skill 发布生命周期、实际原生隔离启动器／签名服务、模型、TG、Android及120GB／2TB双云与恢复仍按[28项差异](../../../delivery/03-goal-gap-assessment.md)继续实施。完整平台与四端同批首发目标没有缩小。
