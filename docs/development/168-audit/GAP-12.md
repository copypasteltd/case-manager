# GAP-12 — 八项 Skill 业务工具代码独立核对

审查日期：2026-11-06。对象为增量168代码阶段工作区；只读源码／规范核对，没有测试、构建、浏览器、服务或应用代码修改。本文不是发布、真实模型或八项 Skill 正式验收凭证。

## 要求与结论

原始要求为 `docs/01-requirements-and-scope.md:24` 的 U-08（纳管文件／目录阅读、新建、更改、写入）和 `:25` 的 U-09（文件、项目、资料整理 Skill）。`docs/03-ai-secretary-and-telegram.md:121` 明确首发至少覆盖项目新建／更新、文件更新、资料整理、合同／报销／采购辅助、订阅汇总，基础简报／状态汇总同轮完成，并要求复用普通业务流程。该文 `:168` 明确 L3 权限／永久清除／资金／生产变更首期不开放；`:102` 说明早期工具名称是设计示意，不能将示意名称逐字作为新增接口要求。

**结论：本次沿已支持普通业务能力的规范→包清单→运行时→网关→普通命令→回执链，未确认新增必须开发的 GAP-12 代码差异。** 这是限定路径的代码存在性与约束追踪，可信度中等；不代表全实现正确性证明，GAP-12 和全部八项 draft Skill 继续开放。真实 provider／目标安装／公司业务样本及正式工具验收未执行。

## 八项能力逐项映射

| Skill | 当前规范锚点 | 实际实现及普通业务／完成证据 |
| --- | --- | --- |
| file-update 1.7.0 | `docs/skills/file-update/SKILL.md:8`；共同边界 `docs/skills/execution-boundaries.md:38` | `app/backend/internal/agent/execution_tools.go:194` 读取准确授权版本及派生文本；`file_update_tools.go:239` 固定文件、基版、草稿、创建位置／名称与服务端计划摘要；`:298` 通过普通 `files.UploadService.CommitAgentDraft` 提交，核对真实版本 SHA／operation 后保存 `agent_file_commits`。新建、更新、恢复、受控 Office 工作区分别有包清单与网关分支，不用任意本地路径替代。 |
| project-create 1.2.0 | `docs/skills/project-create/SKILL.md:10` | `app/backend/internal/agent/plan_tools.go:58` 拟定／提交准确计划；`plan_mutations.go:20` 核对计划摘要、已批准版本及本人批准事件，`:51` 复用普通 `project.Service.Create` 同事务保存项目和计划回执。人员／模板发现由 `execution_tools.go:164` 的目录入口提供，目录发现不授予创建权。 |
| project-update 1.11.0 | `docs/skills/project-update/SKILL.md:12` | `app/backend/internal/agent/mutations.go:49` 列出固定项目、任务、依赖、里程碑、交付物、风险／问题工具；`:582` 比较工具名和原授权完整规范参数；`:602` 复用普通项目命令；`:753` 仅凭真实 operation 返回 committed。 |
| information-organize 1.3.0 | `docs/skills/information-organize/SKILL.md:10` | `app/backend/internal/agent/organization_tools.go:97` 私密会话有界目录快照／拟定；`organization_plans.go:379` 核对固定计划及本人批准事件，`:405`–`:425` 逐项调用普通目录创建／目录移动／文件移动，在同一事务保存真实结果。范围转移和通知另有准确固定人类计划工具，不等于开放任意分类／权限修改。 |
| oa-assist 1.6.0 | `docs/skills/oa-assist/SKILL.md:12`、`:61`、`:63`（后两处为当前扩展，旧前段不是完整能力清单） | `app/backend/internal/agent/business_drafts.go:232`、`business_revisions.go:346` 只接受原 run／intent SHA，固定完整合同／采购／报销／订阅资料，复用普通合同、支出、订阅创建／修订命令；`oa_requests.go:367` 固定申请参数并复用普通 Create／Revise／SubmitRequest；`oa_text.go:243` 人工采用选定正文。草稿不冒充审批、生效、费用或付款。 |
| project-briefing 1.2.0 | `docs/skills/project-briefing/SKILL.md:10` | `app/backend/internal/agent/execution_tools.go:413`、`:544` 提供共同授权资源读取／检索；`post_writes.go:336` 区分固定 draft_post 与精确 publish_post，`:375` 复用普通知识创建／更新／发布，`:416` 返回准确修订及 operation。无写授权时按真实读取来源完成只读简报。 |
| incident-summary 1.2.0 | `docs/skills/incident-summary/SKILL.md:10` | `app/backend/internal/agent/service_summary.go:63` 固定服务和有界时间窗，逐项应用当前共同读权限，调用普通 `servicestatus.AgentSummary`；`:112` 固定真实 service/report/hash 来源。当前 Skill 为只读状态汇总，不凭早期示意表推导必须增加生产修复或无固定授权的故障写入。 |
| subscription-review 1.4.0 | `docs/skills/subscription-review/SKILL.md:10` | `app/backend/internal/agent/finance_summary.go:47` 分币种预测／发生／现金汇总；`:66` 普通订阅发现、逐账期核对、完整组合审阅、真实关联财务原记录；`:104` 保存不可变财务快照。普通订阅草稿由 oa-assist 固定业务输入承担，不将审阅提升为资金动作。 |

表中仅首个文件为完整路径时，同格其余 Go 文件均位于 `app/backend/internal/agent/`；源码行号对应本次读取工作区。

## 协议、身份与消费链

- 全部八个 `app/skills/<id>/manifest.json` 的 allowed_tools／completion_tools 与 `docs/skills/<id>/SKILL.md`、随包输入／输出 Schema 已列入核对；源码包均为 draft。`app/agent-runtime/src/skills.ts:89` 拒绝未发布包／摘要变化／工具缺失；`:96` 绑定 manifest、输入／结果／共同 Schema、指令及引用文档摘要。这里只证明准入代码，未用 draft 冒充实际安装。
- `app/backend/internal/agentworker/mcp.go:415` 提供按 Skill／TG 渠道筛选的工具清单，`:434` 路由至 ExecutionTool；运行时 `app/agent-runtime/src/codex-adapter.ts:91` 同样按 manifest／TG 授权限制工具，`http-boundary.ts:5` 列出公司网关工具。未发现所列首发普通业务只能依赖 shell 或任意 URL 才能实现的情形。
- `app/backend/internal/agent/execution.go:73` 建立事务、锁定组织／用户／会话／运行，检查当前授权版本、状态、期限、取消、队列租约及共同范围；`execution_tools.go:86` 严格拒绝未知 JSON 字段／额外对象，`:104` 进入当前执行权限，`:123` 固定调用键与规范输入并在重放时复核来源，`:389` 将来源、authority stamp、工具输出与审计同事务保存。原始人类会话的门户快照／取消问题由 GAP-11 独立审计，本条不重复提出或声称其已解决。
- `app/agent-runtime/src/evidence.ts:17` 只接受当前 run／authority／允许工具的真实来源与 operation；completed 写操作必须具有成功、committed 的规定完成工具回执。只读简报有明确分支，订阅审阅必须引用真实财务快照；服务端 `app/backend/internal/agent/execution_results.go:124` 亦有 OA 结果核对，未仅以模型句子判完成。
- 门户路径已读 `app/web/src/ConversationMessages.tsx:98`（固定业务草稿发送结果核对）、`ConversationBusinessDraft.tsx:13`（真实业务回执入口）、`app/mobile/src/pages/secretary/index.tsx:183`（准确资料／计划选择），并检索对应共享移动准备、文件、项目、OA、资料整理、财务、服务结果页面。Web 与 Taro 共用业务协议；这里没有浏览器或 Android／小程序实测结论。

## 检索与审查边界

执行过的只读检索包括：`rg --files app/web/src app/mobile/src` 的 Agent／secretary 路径筛选；`rg -n` 搜索 `read_office_workspace|create_business_draft|propose_scope_transfer|read_subscription_portfolio|tools/list`、`allowed_tools|completion_tools|assertRunnable`、普通 `Commands`／Create／Revise／Submit／operation／committed；逐段阅读上述执行消费者、计划提交、OA、知识、目录、汇总及完成判定实现。最初在 `internal/nativeruntime` 和 `internal/httpapi` 的工具名称检索无结果，随后实际定位 `internal/agentworker/mcp.go`／bridge，未将前述零结果当作缺失证据。一次 PowerShell 文件通配路径检索报错，随后改为目录加 `-g 'execution*.go'`，未据失败搜索得出结论。

复杂 Office 布局／任意编辑、无人授权自由商业字段变更与 AI 权限／资金／生产执行，均不能据原用户“全量调度”字样新造为当前受支持能力；格式边界仍须如实展示，不能宣称任意文件完整编辑。未穷尽普通 finance／files／project 内部全部分支、所有 UI 状态、MCP 网络行为和所有并发交错；其他包与 GAP-11 身份问题由对应审计负责。实际 Linux／模型／云存储、TG、平台及公司制度验收保留未验证，不作为本次代码通过项。
