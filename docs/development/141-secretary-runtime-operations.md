# 秘书运行配置台账与人工停用 / Secretary runtime operations

增量141，主线结构107。推进GAP-07／18；提供真实部署前后的日常配置核对与停用能力，完整工作包仍开放。

## 使用流程

Web主导航“秘书运行配置”；微信／抖音／Android共享入口“公司秘书→秘书运行配置”。查阅人可按八种任务和活动／停用状态筛选并完整分页。详情展示不可变Skill／运行时／门户及可选TG配置摘要、安装身份、实际技术资源限制、状态历史，以及排队／执行／等待批准／提交核对四类当前任务数量。

停用人重新读取详情，核对配置编号与版本、当前任务数量和不可重新启用的影响，填写1–200字原因；以最近10分钟内完成MFA的本人会话确认。任务已增加、结束、变更版本／状态或取消意图，服务器返回409 PROFILE_IMPACT_CHANGED，必须重新读取；原配置已停用则返回版本冲突或PROFILE_ALREADY_RETIRED。重复相同幂等意图返回原回执；当前会话和角色始终先于重放检查。

配置状态“可供任务选择”仅说明数据库存在活动安装记录，不是模型在线、隔离有效或正式发布证明。组织台账不提供员工会话、请求、回复或引用，不展示安装／停用自由文本备注、密钥、服务器目录或模型端秘密。详细原因保存在原审计事实内，组织元数据仅返回经办身份。

## 接口与权限

| 接口 | 输入 | 结果 |
|---|---|---|
| GET /v1/agent/runtime-profiles | state、skill_id、cursor、limit | 安装配置元数据，最多100条；游标绑定组织／当前用户授权、筛选条件 |
| GET /v1/agent/runtime-profiles/{id} | 准确配置UUID，不接受查询参数 | 配置、当前活动任务计数／摘要及完整历史 |
| POST /v1/agent/runtime-profiles/{id}/retire | expected_version、active_basis_sha256、reviewed_impact=true、reason | 停用后的配置、当前任务影响和实际历史 |

独立动作agent.runtime.read／agent.runtime.retire，对应agent_runtime_reader／agent_runtime_manager；系统管理员具备这两项配置动作，但不因此取得会话正文权限。迁移107补既有组织，Bootstrap补新组织。入口限Web／WeChat／Douyin／Android实际人类会话；AI执行身份与TG入站身份不能调用。Cookie写入继续检查Origin和CSRF。

## 数据、并发与安装边界

正常迁移0107保留agent_runtime_profiles既有字段和身份保护，不修改迁移1–106。agent_profile_events由固定search_path的SECURITY DEFINER观察触发器自动记录安装／实际停用；既有行以迁移观察时间记retained_baseline，不虚构旧安装事件。应用只有SELECT，不能伪造历史。

agent_profile_retirements为不可变的人类决策，绑定准确配置、前后历史版本、当前活动任务摘要、会话／经办人、明确核对和原因。专用触发器持组织及配置锁，重新检查人类会话、MFA、read／retire实时权限、配置活动状态及任务摘要，才执行唯一的active→retired变化。应用仍无配置表INSERT／UPDATE／DELETE权；没有新建、编辑、恢复活动或绕过部署核验的接口。新版本／回滚版本需另行经过发布和安装核验，不能复活旧配置。

摘要由实际活动任务按UUID稳定排序，绑定每个run的ID、版本、状态、是否已请求取消。API停用持组织排他锁；运行创建、执行／提交和普通取消的组织锁与之串行。详情持组织共享锁，使用一次查询读取一致的任务计数／摘要。所有回执、配置变化、自动历史、审计、outbox及幂等结果同事务提交；审计失败整体回滚。审计与网络／模型调用不混在事务内。

## 执行含义

停用阻止新门户任务及TG专用配置调度。原执行器的续租、工具、结果／提交权限检查读取活动配置，因此后续失去授权。停用接口不直接改任务状态、不伪造进程退出、不重跑模型，也不撤销已完成业务。执行中的进程退出与未决提交仍由原执行控制器确认；已经完成的任务和业务事实保留。

输入／输出Token边界、调用次数、超时均为技术资源限制，仍在当前范围；没有重新引入AI费用控制。

## 验证和部署边界 / Evidence and deployment

Fresh focused native HTTP/PG checks exercise separate roles, all four human channels, exact review/MFA/version, changing real task impact, actual execution authority after retirement, installation privilege preservation, journal immutability, idempotent replay and transactional audit failure. The existing selected execution-controller suite is rerun with controlled confinement/provider doubles, without invoking Codex. Actual authenticated synthetic Web inventory→impact review→retirement→reload is separately captured.

客户端类型及Web／微信／抖音构建为本轮源码；Android仅共享源码更新，沿用APK137（业务136／结构103）。完整回归、新移动UI旅程、新APK及正式Linux／模型／真实TG验收延期。开发PG仍100，不能直接运行结构107新后端；目标持久PG需正常升级并实际验收。两冻结标签不移动，八Skill仍draft，不声称真实模型已上线。

[验证](../quality/141-secretary-runtime-verification.md) · [全部余项](../delivery/05-functional-closure-141.md)
