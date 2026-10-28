# 秘书技能包、结果核验与 Codex 事件适配

增量105更新：仓库现已提供[Linux原生控制器与独立发布目录](105-native-agent-runtime.md)，原文中缺少签发器的实现缺口已推进为目标服务器验收；真实模型／Skill发布仍开放。AI费用控制已按[修订99](../delivery/04-current-scope-and-acceptance.md)移出，技术资源限制仍保留。

更新：2026-10-25。本文记录技能加载、Codex事件适配与Go执行控制链路，关联 [秘书需求](../requirements/07-secretary-and-channels.md) 与 [工具协议](07-agent-tools-and-skills.md)。实际模型、服务器原生隔离和Telegram仍待配置验收；下列受控测试不代表这些外部链路已上线。

## 1. 实际实现范围

| 部分 | 当前状态 | 实现位置 |
| --- | --- | --- |
| 八项技能说明 | 已创建，全部draft，未发布 | docs/skills/各技能目录 |
| Manifest及输入输出Schema | 已创建并校验 | app/skills/ |
| 加载、摘要与发布状态检查 | 已实现，离线验证 | app/agent-runtime/src/skills.ts |
| 业务凭证与模型结果引用核验 | 已接Go持久化只读凭证；业务写入工具待实现 | app/agent-runtime/src/evidence.ts、Go agent/execution_results.go |
| Codex SDK事件适配 | 已实现并使用受控SDK工厂验证 | app/agent-runtime/src/codex-adapter.ts |
| 公司会话/任务/事件持久化 | 已接HTTP执行边界、任务队列、租约、回复与收尾 | Go agent/、agentworker/、迁移0047～0048 |
| Go工具网关及MCP服务 | 已提供项目／任务／知识文本读取、范围检索及本次任务核对；其余工具待实现 | agentworker/bridge.go、mcp.go |
| 原生工具与OS隔离 | 已实现签名证明核验和子进程停止；真实服务器隔离尚未实测，无默认放行 | agentworker/proof.go、process*.go，Q-02仍开放 |
| 真实Codex/模型、TG | 未调用、未投递 | 后续对应工作包 |

SDK固定为 `@openai/codex-sdk@0.164.0`，其他依赖及完整性由app/package-lock.json固定。已提供Go控制器、Node执行入口与短期任务凭证；尚未配置真实模型凭据或服务器隔离实现，不能仅设置enabled后跳过准入检查。

## 2. 八项来源包与文档布局

说明源统一放在docs/skills/，执行清单和Schema放在app/skills/；不会把文档搬入服务器任意目录或自动安装到用户全局技能目录。

| 技能 | 等级 | 完成结果所需的业务工具 |
| --- | --- | --- |
| [project-create](../skills/project-create/SKILL.md) | L1 | create_project |
| [project-update](../skills/project-update/SKILL.md) | L1 | update_project或transition_task |
| [file-update](../skills/file-update/SKILL.md) | L1 | commit_file_update，草稿生成不足以证明正式更新 |
| [information-organize](../skills/information-organize/SKILL.md) | L2 | commit_organization；固定计划与逐项结果仍须业务网关实现 |
| [oa-assist](../skills/oa-assist/SKILL.md) | L1 | draft_request或submit_request，须按实际意图及业务证据解释 |
| [subscription-review](../skills/subscription-review/SKILL.md) | L0 | 实际成功读取及可核对来源 |
| [project-briefing](../skills/project-briefing/SKILL.md) | L0 | 实际成功读取及可核对来源 |
| [incident-summary](../skills/incident-summary/SKILL.md) | L0 | 实际成功读取及可核对来源 |

各包引用[共同执行边界](../skills/execution-boundaries.md)，限定身份、范围、来源、不可信内容、授权和提交凭证。owner_role是维护职责，不是模型执行身份。八项包均不默认提供向群聊发送消息的工具。

## 3. 加载与不可变摘要

加载器核对规范包名、Manifest Schema、技能身份、YAML前置信息、工具目录、输入输出Schema和共同边界引用。路径在解析前后均检查是否位于允许的来源根目录；拒绝逃逸路径、符号链接包目录、超限文件和无法解析的结构。

单个来源文件上限64KiB，单次输入/输出JSON上限1MiB。参数不进行隐式类型转换；业务输入禁止额外身份、任意路径、对象键或URL字段。时间窗校验实际日期与起止顺序，日期不能借助JavaScript的自动进位接受2月30日或24点。

SHA-256绑定Manifest、共同Schema、Manifest Schema、输入输出Schema、SKILL.md及所有引用边界文档。任一来源改变都使旧发布摘要失效。发布状态必须为published，或显式试点时为pilot；draft、tested和disabled均不进入实际运行。工具还必须存在于网关实际可用集合中。

L0只接受读取工具且不能声明写入完成工具；批量整理提交和站外投递提交仅允许L2声明。这是包级检查，不替代Go对每次调用的对象权限和固定计划检查。

传给SDK的输出Schema会展开已知共同引用，不要求模型或CLI访问copypaste.hk下载Schema。此展开只支持本项目已知引用，其他来源或循环引用拒绝；实际模型端结构化输出兼容性仍需Q-02验证。

## 4. 模型结果与实际业务凭证

模型只能提出status、summary、resource_refs、source_refs、operation_ids、missing_fields及next_action；不能自行提供committed或TG送达状态。

执行凭证由独立可信边界取得，包含实际run、授权版本、工具、操作号、状态、committed和资源/来源引用。模型正文中的同名信息不能作为凭证导入。

核验器执行以下检查：

1. 只接受当前run、当前授权版本和本技能允许工具的凭证。
2. 结果中每个资源或来源引用必须匹配成功凭证，引用包含版本时须准确匹配版本。
3. 操作号须存在于实际凭证；正式完成时该操作必须成功且committed。
4. 写入完成至少匹配本技能声明的正式完成工具；L0完成须有实际读取和非空来源。
5. 存在missing_fields时不能标记completed。

这些检查验证结构、引用和基础提交事实。它们不证明自然语言的每个结论正确，不足以判断固定批量计划是否全部完成，也不能替代OA按「草稿/送审」实际意图核对结果。逐项计划完整性、业务对象范围、状态事实和当前披露权限须由Go业务层最终核验；在该层接入前不发布正式写入能力。

## 5. SDK适配与公开事件

每次公司run创建新的原生线程，使用已经重新筛选的授权上下文；不直接续接可能包含失权资料的旧原生线程。公司任务标识与原生线程标识分开，原生线程标识不进入公开事件。

SDK子进程只接收有限系统环境和本次短期任务凭据；不继承数据库、存储及其他业务服务凭证。短期工具凭据通过指定环境变量提供给MCP连接，不拼进模型任务输入。SDK所需API凭据由独立运行身份的部署配置提供，当前未配置或调用。

适配器设置read-only、never审批模式、关闭Web搜索、不添加额外目录，并显式关闭shell、Apps及code mode。MCP只声明company服务和当前技能工具清单。**这些配置不是已经有效隔离所有原生工具的证明。** `never`不代表业务无限授权；SDK的工具事件检查发生在事件被观察时，不能当作工具执行前的拦截器。

实际启动前必须通过assertNativeConfinement与当前授权检查。原生边界请求绑定SDK版本、部署模型、配置摘要、工作目录和预期工具集合；没有默认总是成功的验证器。服务器接入必须证明独立OS身份、配置来源、目录/网络/资源限制及实际可用工具与已验配置一致。

| 事件 | 对外内容 | 限制 |
| --- | --- | --- |
| run.started | 任务已开始 | 持久化成功后才继续启动SDK |
| tool.started | 允许公开的工具名称 | 不附原始参数、凭据或文件内容 |
| tool.finished | 工具调用已返回 | 不等于业务已提交、文件已复制或消息已送达 |
| run.completed | 经凭证核对的结构化结果 | 仅代表本次模型适配回合完成；业务解释以result.status与最终业务核验为准 |
| run.interrupted | 固定安全错误码、是否需核对结果 | 不暴露SDK原始异常、命令或连接信息 |

reasoning、原始MCP参数/返回体、原生错误文本不作为公开事件保存。结构化消息在验证前不直接推送给用户；当前实时反馈是任务与工具进度，不宣称已经实现逐token安全输出。

## 6. 取消、超时、撤权与预算

每次状态读取与公开结果前检查当前授权；接口接入方必须支持请求取消，并将实际撤权、预算与到期判断接回Go。每次工具执行前的授权、限额和幂等必须由Go网关执行，不能只依赖适配器观察事后SDK事件。

同一原生工具调用ID的更新只计一次，新调用受技能与run两者较小次数限制。Token用量按SDK回合终结数据记录并检查输入/输出边界；这属于已发生用量检查，不是供应商计费的硬上限。AI费用预留／配额／结算不再属于当前发布要求；技术用量和执行限制按修订99及增量100处理。

超时或取消通过AbortSignal传入SDK，并清理监听器、计时器与迭代器。适配器已开始调用SDK后，丢失连接、撤权或中断均置reconciliation_required；不能提示「所有写入已撤销」。可信控制器应查询操作账本与未决提交，恢复同一操作结果，不重建幂等键盲目执行。

## 7. 实际验证与后续放行

[当前运行时证据](../quality/evidence/secretary-execution/runtime-build.json)记录19项场景通过，包含HTTP执行边界、MCP读取、工作进程入口、准确配置摘要与会话意图Schema。先前14项离线结果保留在[原始证据](../quality/evidence/agent-runtime/result.json)，属于历史来源。测试使用临时复制包的受控published状态和受控SDK工厂；仓库八项实际Manifest仍保持draft，未调用原生Codex或真实模型，未发送消息。

场景覆盖身份/路径字段注入、日期窗口、输出Schema、未发布/摘要漂移、L0工具边界、真实凭证匹配、伪造操作号、原生旁路工具事件、撤权、取消、停滞流超时、调用次数、已发生用量、持久化失败、错误信息过滤及不完整事件流。八项SKILL.md另通过skill-creator的quick_validate，Windows下使用Python UTF-8模式读取中文。

复核命令（在app/运行）：

```text
npm run build:runtime
npm run check:skills --workspace @copypaste/agent-runtime
C:\Python313\python.exe scripts/verify_agent_runtime.py
```

尚待完成：写入工具、固定计划与批准、逐项业务结果、技能发布管理、真实服务器原生隔离PoC、模型端资格和调用、Android宿主／真机及Telegram授权与去重。会话和任务已接Web与Taro页面；四端原生首发验收继续保留。上述缺口不能因适配库测试通过而自动放行。

## 8. 已核实的上游依据

已核对[Codex SDK官方说明](https://learn.chatgpt.com/docs/codex-sdk)及安装包的TypeScript类型：本适配器使用Codex、startThread、runStreamed、outputSchema与AbortSignal。版本变化须重新核对事件与配置兼容性。

[官方配置参考](https://learn.chatgpt.com/docs/config-file/config-reference)说明了developer_instructions、MCP工具允许清单与相关开关；尤其unified_exec用户关闭选项存在限制，不能靠单个用户开关声称原生命令工具已完全不可用。history.persistence=none也不能推导没有任何原生会话缓存。临时目录与身份环境的实际保存内容必须在服务器PoC审计，业务持久化仍回到PostgreSQL和对象存储。

## 9. 持久执行控制链路（2026-10-25）

`0048_agent_execution_and_receipts.sql` 将消息受理与 `agent.execute.v1` 队列原子绑定；历史queued任务迁入相同队列。每个任务最多一次执行尝试。租约绑定准确任务、工作进程、fencing token、授权版本与短期随机凭证，数据库只保存凭证摘要。模型提示、公开事件和日志不包含凭证。

Go的 `worker-agent` 创建本地控制入口，领取任务，向原生启动器传递JSON标准输入，并每15秒核验授权与续租。`worker-agent-reconcile` 可独立运行，回收过期或失联任务；失联记录为中断，不重跑模型。正常取消必须等启动器退出后确认；committing状态保留正式提交核对义务。续租默认90秒，业务任务仍受原定到期时间约束。

控制入口只绑定固定的 `127.0.0.1` 端口，核对Host、Origin与任务Bearer；拒绝身份注入、任意路径和查询参数。`/internal/v1/agent-control/` 下提供current、confinement、receipts、events；`/internal/v1/agent-mcp` 提供initialize、ping、tools/list、tools/call。采用无会话的JSON Streamable HTTP，GET不建立SSE。协议协商及工具结果格式依据[MCP生命周期](https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle)、[传输规范](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)和[工具规范](https://modelcontextprotocol.io/specification/2025-06-18/server/tools)。

只读工具复用所有会话成员的共同业务权限和AI许可。搜索固定在会话项目／公司知识范围内，游标绑定任务、查询、类型、页大小与签名；单次最多20条，候选范围超过10000条则要求缩小范围。读取支持有限字符分页；整次任务最多20个资料依赖，每份资料只允许一个准确版本。旧调用标识只能重放原参数和当前仍有效的凭证，不能变更调用内容。

结果中的每项引用必须来自本次Go读取账本。回复包含已观察的全部资料依赖，即使模型省略某条引用。最后核验与回复提交期间持有短时组织锁，与现有业务命令互斥；事务内不调用模型或其他网络服务。首期3人并发按此保守实现，扩容时再拆为细粒度依赖锁。回复、结果、任务事件、审计、outbox、队列成功同事务提交。后续资料变化或丧失阅读权限时，历史回复隐藏原文与引用。

`state=succeeded` 表示本次秘书响应已保存；新增 `outcome` 区分草稿、待处理、待补资料、完成、冲突、拒绝和事项失败。当前只读网关仅允许有真实来源的资料汇总报告completed，不能制造项目创建、审批、付款或文件更新成功。Web和Taro按outcome展示业务进展。

原生启动条件仍明确保留：已发布的准确技能摘要、齐备的工具、当前权限、准确SDK配置摘要、专用模型凭据，以及外部原生隔离服务签发的短期Ed25519证明。证明绑定随机挑战、run、授权摘要、模型、目录、SDK和工具清单；不接受布尔“已隔离”。外部启动器必须在整个子进程生命周期落实原生工具、文件、网络与进程树限制。签名核验不能替代真实OS隔离；本仓库未部署该启动器或签名服务。

部署需明确配置 `CP_AGENT_LAUNCHER`、`CP_AGENT_NODE`、`CP_AGENT_ENTRY`、`CP_AGENT_REPOSITORY`、`CP_AGENT_WORK_DIRECTORY`、`CP_AGENT_PROOF_COMMAND`、`CP_AGENT_PROOF_PUBLIC_KEY`、`CP_AGENT_MODEL`、`CP_AGENT_OPENAI_API_KEY`；可选 `CP_AGENT_BRIDGE_ADDRESS`，默认127.0.0.1:18081。前六个路径必须为运维控制的绝对路径。Node入口为编译后的 `agent-runtime/dist/worker.js`。模型、数据库、会话及存储凭据不通过继承环境传给启动器；专用模型凭据和本次任务凭证仅通过受控标准输入传递。启动器转发到已隔离的Node进程，不能将输入写入普通日志。

`npm run profile:describe --workspace @copypaste/agent-runtime` 从标准输入读取skill_id、profile_id、model、working_directory、mcp_endpoint，输出准确技能／SDK配置摘要与缺失工具，不发布技能、不修改数据库或调用模型。部署身份先完成真实隔离验收和技能发布，再安装不可变运行配置；没有任何默认活动配置。当前八项Manifest仍为draft。

验证见[运行时检查](../quality/evidence/secretary-execution/runtime-build.json)、[完整后端回归](../quality/evidence/secretary-execution/backend-result.json)与[最终来源核对](../quality/evidence/secretary-execution/final-source-check.json)。Go受控夹具直接使用真实数据库和公司工具HTTP入口；Node使用受控SDK客户端。本轮不代表真实Codex进程、模型、TG、Android或云存储已验收。
