# GAP-09 — 真实 Codex 秘书接入代码链路

> “基于 Codex 的在线公司秘书，从管理端会话访问”
> — `docs/01-requirements-and-scope.md:23`，U-07。
>
> “Codex 接入资格与 Runtime PoC 是放行前提（Q-02），不是已接通事实。”
> — `docs/requirements/07-secretary-and-channels.md:9`。

**Verdict:** undecidable（真实模型/目标环境验收）；已追踪的集成代码路径 implemented，未确认新增可直接修复的代码缺口。基线 `930cdde`；168代码阶段只读检查，不执行测试、构建、浏览器、服务、模型或虚拟化。

**What this demands of an implementation:** 已认证普通会话能够选择固定 Skill/runtime/config、排队生成任务，由独立原生执行器调用固定 Codex SDK/CLI；模型只能通过当前 Go 权限与持久回执使用业务工具，公开进度及准确最终结果写回公司会话。TG沿用同一任务体系；实际准入和配置未提供时不能宣称模型已接通。AI费用控制移出不删除技术资源/权限/隔离边界。

---

**Where enforcement lives / Paths walked:**

- **会话→实际队列**：`app/backend/internal/httpapi/agent_runs.go:87–113` 将消息读取、创建任务及事件读取接到 agent 服务；`agent/runs.go:425–441` 检查会话与 active 且 `agent_profile_release_enabled` 的精确配置，`:743–797` 在普通事务中写 queued run、原始消息、固定授权对象并调用 `enqueueRun`。`agent/admission.go:33–45` 建立去重 `agent.execute.v1` job 与 run/job 关系，非只返回虚假 run ID。
- **队列→原生执行器**：`agentworker/runner.go:20–49` reconcile、claim、IssueExecution、LeaseForCredential，按任务有效期启动 Executor；`:54–88` 续租/失权/截止取消并等待进程完成，`:92–107` 在独立有界上下文核对最终状态。`cmd/platform/agent_worker.go:36–60,72–105` 消费明确模型、原生 launcher/Node/entry/repository/证明公钥和固定 loopback 地址；native 模式拒绝把供应商密钥放入 worker，仅传 broker 占位凭据。
- **启动→实际 SDK**：`agentworker/process.go:50–65` 将绑定 run/settings 送 launcher stdin，受控进程退出不会假报完成。`nativeruntime/broker.go:64–98` ValidateRun/Plan/容量检查后调用 Host Prepare/Run；`nativeruntime/linux_client.go:154–170` 建立私有 company/model 转发并启动 Node。`agent-runtime/src/worker.ts:14–22` 核对准确运行器与 Skill 摘要后实例化适配器，正式 CLI 不接受测试 factory；`codex-adapter.ts:123–159` 默认 `new Codex`、先 current/confinement 核对再 `startThread().runStreamed()`，没有模拟成功分支。
- **供应商请求**：`nativeruntime/gateway.go:20–60,70–83,113–138` 把固定 run 的私有 `/model/v1/responses` 映射至已配置 ModelOrigin、校验固定模型并由 broker 注入真正密钥，拒绝重定向/直接泄露供应商错误正文。`codex-adapter.ts:98–119` 固定 SDK config、只读 sandbox、无 shell/联网/浏览及受限 company MCP 工具；配置摘要绑定这些实际选项。
- **工具/进度→独立业务证据**：`agentworker/bridge.go:76–118` 检查受控来源/任务凭证/current/receipts；`mcp.go:424–438` 对 `tools/call` 严格解码，仅把固定参数送 `Service.ExecutionTool`，返回 Go 业务输出而非模型自报。`bridge.go:168–201` 把 started、usage、tool observations、completed 分别交给 Go 服务。`codex-adapter.ts:160–199` 限制事件/工具、要求完整流，读可信 receipts 后 `verifyResult` 再发 completed；`:201–211` 中断保留需对账语义。
- **终态→会话和当前客户端**：`agent/execution_results.go:71–105` 在当前执行事务中校验状态/技术用量及可信 receipts；`:195–229` 合并所有被观察来源，原子写 assistant message、result、succeeded 事件及 job completion。`web/src/ConversationMessages.tsx:67–89,137–142` 实际发送并轮询消息/事件；`mobile/src/pages/secretary/index.tsx:151–155,202–214` 使用同一消息/运行事件入口，并清理自己的计时器与 scope。
- **TG沿用相同执行链**：`agent/telegram.go:212–239` 固定当前 audience/source/config，写 telegram run/message 后调用同一 `enqueueRun`。`telegram/delivery.go:243–258` 默认发送准确 portal 收件入口，只有 reviewed_result 才调独立审阅的正文投递；这是公开渠道披露限制，不能把普通提示当作模型正文已群发。真实 Bot、群成员和投递验收仍属 GAP-15；本次未代替该项审查。

---

**Searched:**

- `Codex/startThread/runStreamed/assertNativeConfinement/run.completed`：实际构造、执行及边界调用见上述 TS 文件，非仅接口/测试命中。
- `ProcessExecutor/worker-agent/NativeSettings/CP_AGENT_MODEL`：找到平台 worker 启动、settings 消费和原生进程传输入口。
- `ExecutionJobKind/enqueueRun/agent_runs/agent_run_results/conversation_messages`：找到 admission、IssueExecution 及原子终态写入路径。
- `tools/call/ExecutionTool/events/receipts`：找到真正 MCP 调用、进度上报与 Go ledger 读写入口。
- 初次猜测的 `agent/messages.go` 和 `telegram/dispatch.go` 不存在；随后改用目录/文件发现定位 `runs.go`、`agent_runs.go`、`agent/telegram.go`，未把错误路径搜索当作能力缺失证据。

**How the verdict was reached:** 固定供应商配置、SDK、native broker、实际任务调度、MCP/Go回执、终态持久化及当前客户入口都有可执行源码；没有确认另一个仅声明未实现的集成步骤。真实模型/CLI/平台兼容性依赖尚未提供的资料和目标环境，源码存在不证明成功互通，不能关闭 GAP-09。八项 Skill draft 与真实隔离/安装验收分别保持 GAP-10/13。

**Open questions / Unread boundaries:** 未调用真实供应商、未在目标 Linux运行CLI/隔离/工具会话、未校验私密部署配置、未执行任何测试或重放旧证明；不宣称所有SDK事件/供应商扩展/全部业务工具实现已穷尽审查。原生内核隔离详细安全性、独立Skill发布和TG披露策略由其各自编号另审。公司制度、真实配置及正式业务验收保持开放。
