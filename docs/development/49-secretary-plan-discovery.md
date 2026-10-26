# 秘书既有计划查询与明确引用

本增量延续[项目计划批准协议](48-secretary-project-plans.md)，解决新任务无法取得前一任务计划标识的问题。后端代码在 `app/backend/internal/agent/plan_discovery.go`，客户端入口在公司秘书的本人公司私密会话。数据库仍为0049，无新增迁移，也未自动发布技能或安装实际运行配置。

## 1. 用户流程

1. 员工在既有计划卡片点击“引用这份计划”，把准确计划加入本次上下文；客户端选择当前可用的新建项目任务，过滤其他技能。移除引用后恢复任务选择。引用不等于批准，也不自动发送请求。
2. 明确发送消息后，`RunCreate.context_refs` 携带 `agent_plan` 的准确 ID。服务器检查本人、会话、当前权限、资料分类及技能，再保存不可变意图摘要。文字草稿不会自动恢复未核对的资料引用。
3. 没有明确引用时，秘书可在当前会话查询可见计划；多个候选必须让员工明确选择。新任务不会继承其他任务的游标、凭证或隐含批准。
4. 用准确标识读取固定内容，再检查当前计划版本、状态及阻断原因。只有既有批准仍然有效，才能通过原来的创建工具消费批准；创建继续复用实际项目服务与原子回执。
5. 移动端历史消息中的“查看当前计划”在当前页面读取对应计划，核对准确摘要及会话，展示只读卡片；关闭后回到原消息，不跳转不存在的通用资料路由。

没有可用的 `project-create` 运行配置时，不显示引用执行入口；既有人工拟定、批准、撤销和提交入口仍按原协议授权。实际开发环境仍有0个活动配置，测试中的配置只存在于隔离数据库。

## 2. 受控工具契约

准确工具名称仍为5项；此次扩展既有读取和查询参数，不增加模型批准工具。

| 工具 | 参数 | 返回与边界 |
| --- | --- | --- |
| `search_resources` | `resource_types:["agent_plan"]`、`query`；可选 `plan_states`、`limit`、`cursor` | 标题关键词或准确ID；允许空查询，查询词最多200字符；每页1—20项，状态只允许 proposed／approved／committed／revoked |
| `read_resource` | `type:"agent_plan"`、准确 `id`；可选 `offset`、`max_chars` | 固定规范JSON的有限字符片段；偏移0—200000，每次最多10000字符；不接受 `version_id` |
| `inspect_operation` | 当前 `run_id`、准确 `plan_id` | 当前计划、版本、摘要、实际回执及阻断原因；只观察，不批准、不恢复失效计划 |
| `propose_project` | 原项目参数和原因 | 固定意图，pending、未提交；AI分类拒绝时整笔拟定回滚 |
| `create_project` | 准确计划、批准版本、摘要和原因 | 再次核验当前授权与批准；成功后真实项目、operation ID及 `committed=true` |

计划必须单独查询，不能与 project／file／post 混合。其他技能保持原读取范围，不能通过直接参数、上下文引用或工具定义取得创建计划。人员与模板目录仍未提供，不得猜测相关ID。

查询返回准确ID、名称、不可变 `plan_hash`／`source_sha256`、`resource_version:1`、当前 `plan_version`、状态、到期时间及阻断原因。正文读取和查询附 `observed_at`；状态是读取时的观察，批准后仍需重新检查。不可变资料修订与当前状态版本不能混用。

当前固定单动作协议的人工批准版本为2，提交后的状态版本为3。已提交计划重查同一业务回执时，`create_project.expected_version` 仍绑定原批准版本2；不能直接复制查询结果的状态版本3作为批准。SOP明确该规则，服务器继续核对准确摘要、本人及当前项目读取权限，不产生第二个项目。仅查询到历史已完成事实不等于本次任务完成创建。

## 3. 权限、分页和资料边界

范围固定为当前组织、当前本人、当前公司私密会话。每一条候选重新核验读取权限；计划本身的目标分类也检查最新 AI 许可及渠道政策，尚未创建项目的意图同样受约束。创建后的计划还检查实际项目当前权限和 AI 读取范围。人工可读不代表可交给模型处理。

游标采用签名的UUID键集位置，绑定任务、授权版本、会话、原查询、状态和页大小；更换筛选、篡改或跨任务使用会被拒绝。每次最多扫描10000条候选，超限返回受控错误，要求缩小范围。隐藏项不计入结果，不返回隐藏数量，也不因它们暴露续页位置。分页不是全量快照；期间状态或权限变化时应重新查询。

来源摘要只对应不可变计划意图。批准、撤销、提交的状态变化不改写意图，也不把旧观察变成当前授权。已取消来源任务、权限变化、过期等阻断仍由原提交路径重新核对。已成功查询、读取或检查的工具回执不能冒充项目创建完成凭证。

## 4. 验证方式

`TestAgentProjectTools` 使用实际本地HTTP/MCP、原生PostgreSQL和受控原生证明，覆盖分页、空查询及状态筛选、跨用户／跨会话隔离、签名和跨任务游标拒绝、有界正文读取、查询恢复准确计划后的实际提交、已取消来源阻断、禁止AI分类的列表／直接读取／上下文隔离，以及拒绝拟定的事务回滚。没有调用真实模型。

`skills.test.ts` 验证查询／读取／检查成功仍不能声明业务已完成或捏造 operation ID。Web和Taro H5浏览器检查使用上述隔离夹具，通过真实登录及MFA、准确引用、实际任务受理、固定来源持久化和取消状态核对；Taro另检查历史引用只读查看。两套脚本都关闭所属页面、上下文、浏览器和临时服务，原生测试随后删除隔离数据库。

| 命令 | 用途 |
| --- | --- |
| `python app/scripts/run_agent_browser.py --plan-discovery` | Web引用任务与服务端来源核对 |
| `python app/scripts/run_mobile_browser.py --plan-discovery` | Taro H5历史引用、任务与来源核对 |
| `python app/scripts/verify_backend_batches.py` | 固定来源下的完整后端回归；已有记录须先保留再重跑 |
| `python app/scripts/verify_agent_runtime.py secretary-discovery` | 运行时测试与八项技能包校验 |
| `python app/scripts/verify_mobile.py secretary-discovery` | 移动测试、类型及微信／抖音编译 |
| `python app/scripts/verify_secretary_plans_development.py secretary-discovery` | 自有开发库、路由拒绝与运行配置检查 |
| `python app/scripts/verify_secretary_plans_sources.py secretary-discovery` | 当前来源、证据与临时资源清理汇总 |

执行结果以 `docs/quality/evidence/secretary-discovery/`、`secretary-discovery-web/` 和 `secretary-discovery-mobile/` 的记录为准。H5和小程序编译不能替代微信／抖音真机或Android验收；本增量不证明真实SDK原生隔离、模型、TG、双云、大文件或生产部署通过。
