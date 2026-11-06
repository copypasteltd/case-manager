# GAP-01 — 项目从立项到归档的当前代码核对

审查基线：`930cdde97b9d5d2e85d892664b0857a10a811739`。本项仅只读源码和规格，不执行测试、构建、浏览器、服务或数据库命令，不改应用代码；未使用 Docker、WSL、虚拟机或模拟器。范围是 GAP-01，不借此签署其他工作包或正式验收。

## Requirement

> 本规划将“完整”具体化为：项目立项与范围、负责人/成员、任务与子任务、优先级/期限、任务依赖、里程碑、文件/日志、合同关联、项目预算与实际成本、风险与问题、范围变更、交付清单、验收记录、归档与复盘。
>
> 项目详情提供列表/看板及基础时间线。完成任务不自动意味着交付验收通过；关闭项目需检查未完成任务、交付文件、验收结论及未处理合同/费用。复杂资源优化排程与自动工时不属于当前“完整”的默认含义。
>
> — `docs/01-requirements-and-scope.md:125–127`，细化为 `docs/requirements/03-projects-and-knowledge.md:9–52` 的 PM-01～09/11。

**Verdict: undecidable（完整正式旅程）；列明的当前实现链未确认新的可行动代码偏差。** 此结论区分源码中已经存在的流程与尚无公司项目/制度/真实环境证据的正式验收，不将“静态未发现缺陷”改写为全场景通过。

## Where enforcement lives / Paths walked

| 路径 | 实际读到的约束与调用方 | 判断 |
|---|---|---|
| 立项及初始化 | `httpapi/projects.go:44` → `project/service.go:149–252`：`Commands.Do` 内核对组织创建/成员管理权限、有效用户；锁定已发布模板及准确修订；项目、成员、根目录/标准子目录、模板任务/里程碑同事务创建。Web `Projects.tsx`、移动 `project/setup.tsx:24–59` 发出实际创建/编辑命令。 | 已有普通入口与事务实现；未把模板文件字节异步生成算作已经实现。 |
| 成员及负责人 | `project/members.go:49–145`：项目版本、可写阶段、独立成员管理权限；不得由本人变更本人资格，不得直接移除项目负责人；移除须处理未完成的责任事项，并撤销相关授权、提高用户/组织授权版本。移动 `project/members.tsx:15–39` 连接普通命令。 | 已有源约束；公司角色分配仍是配置。 |
| 任务/子任务/期限 | `project/tasks.go:110–209,219–328`：同项目有效负责人/参与人/里程碑；不可将子任务挂到已结束父任务；日期顺序、父链循环、实际版本、项目只读状态均在写事务内检查。移动 `project/task-forms.tsx:18–45,49–141` 连接普通读取/表单；Web `Projects.tsx:93–129` 连接原任务详情和动作。 | 已有普通来源和人类操作。 |
| 依赖与完成 | `project/tasks.go:361–457,469–564`：同项目且可读前置任务、项目锁、循环检查；开始/送验/完成检查前置 done，完成检查未结束子任务，需要复核的任务另外要求 `task.review`。任务 done 不写交付验收记录。 | 已有依赖和任务验收边界。 |
| 里程碑 | `project/milestones.go:122–217`：版本、项目写权限和状态；achieve 必须有关联任务或交付，所有相关事项完成/accepted；已达成或取消的里程碑须先重开才允许关联变更。 | 已有受控关联与阶段检查。 |
| 风险→问题 | `project/concerns.go:175–295`：风险转问题另外核对 `issue.create`；项目锁/可写状态、版本、有效负责人、状态机；生成关联问题和风险更新同事务，问题解决/关闭需说明。 | 已有可追踪转化，非仅状态标签。 |
| 范围变更与预算 | `project/change_mutations.go:335–403`：准确批准修订、请求修订、内容摘要和项目/预算基线；当前人类会话及近期 MFA；再次锁定和重读后插入实施凭据。`project/budget_comparison.go:49–174` 把预算、原币实际费用口径和带依据的分页分别处理。Web `ProjectChanges.tsx` / `ProjectBudgetComparison.tsx` 与移动 `project/change*.tsx` / `project/budget-comparison.tsx` 使用原协议。 | 已有审批和实施分离；本项未签署全部财务业务。 |
| 文件/日志及关联业务 | `project/overview_reads.go:20` 起逐类查询实际记录并按 `can_access` 过滤；固定关联为稳定 ID。`project/overview.go:93–118` 对依赖标题/状态另做当前可读检查。Web `ProjectOverview.tsx` / `ProjectBusinessHub.tsx` 与移动 `project/overview.tsx` / `project/business.tsx` 连接实际来源。 | 项目上下文和普通业务可追踪；合同节点精细关联属于 GAP-02 独立核对。 |
| 固定交付与独立验收 | `project/deliverables.go:176–222,249–311`：当前项目/里程碑可写、准确头版本、固定正式文件版本和会话检查；提交创建独立不可变修订。`project/deliverable_revisions.go:138–183` 对准确修订/hash、文件和准则声明、近期 MFA、人类会话再次核对，拒绝提交人自行 accept。Web `ProjectDeliverables.tsx` 与移动 `pages/deliverable/index.tsx` 是实际入口。 | 不因任务完成自动验收；新提交不覆盖旧文件依据。 |
| 结项、复盘及归档 | `project/service.go:353–418` 拒绝普通 archive 捷径；进入 closing 先核查阻塞。`project/closeout_mutations.go:27–134` 要求准确版本/fingerprint、完整可读依据、业务/财务逐项声明、复盘和近期 MFA；锁定来源、重算快照并拒绝变化或任何阻塞，固定附件/复盘/归档状态在同一事务。`closeout_reads.go:96–126` 使用只读重复读快照，仅完整可见者可得完整指纹。Web `ProjectCloseout.tsx` 和移动 `project/closeout.tsx:30–203` 连接核查、进入 closing、归档和历史原接口。 | 已有完整普通归档入口；未执行实际归档验证。 |
| 当前阻塞全集及只读 | 最新 `migrations/0131_closeout_subscription_calendar.sql:2–45` 的核查包含任务、里程碑、独立交付验收、风险/问题、范围变更、预算、合同、采购、报销、费用、资金及订阅，并按订阅原时区判断取消生效。`project/tasks.go:50–54` 的 `projectWritable` 被已走读普通写入口调用，归档/取消/结项中不能普通写入。归档本身不产生付款、取消订阅或删除文件。 | 源码保留166日历修复；历史归档不回填。 |

## Searched

- 在 `app/backend/internal/project` 枚举实际 `.go`，从 HTTP 项目、任务、变更及结项路由进入服务和相关文件；没有以函数名存在替代上述执行分支阅读。
- `project_closeout_items/snapshot/cost_ids/cash_ids` 在迁移目录找到46、89、122、131修订；实际采用131最后定义，不以旧定义推导当前日期/退款口径。
- 在 `app/mobile/src` 检索 `看板|时间线|timeline|TaskBoard|taskLanes|projectSchedule|board`：项目任务只找到列表组件，时间线命中是状态事件；未找到移动项目看板/时间线。确切任务入口为 `pages/list/index.tsx:18–20` → `project/task-forms.tsx:24–46`。Web `ProjectTaskViews.tsx:34–81` 实有列表/看板/时间线。
- 对照 `docs/development/68-project-task-board-and-timeline.md:17`，该增量明确只实现 Web；`docs/development/06-clients-and-pages.md:14–16` 又把移动项目资料“查阅”和任务本人动作列为必须，未明确要求三个视觉视图逐端相同。因此移动缺少两个视图是**现存能力边界/范围疑问**，不能仅凭共同首发就直接断定是新的必选代码缺陷；应交独立规格反证后决定是否纳入实现。

## How the verdict was reached

核心普通链条有实际调用方、事务和状态/版本/权限校验，未发现必须另起一套项目框架的证据。当前无新确认缺陷交付实现；完整正式项目旅程仍不能由本次只读分析证明。PM-02模板文件异步生成在增量67已有明确未支持边界，不能凭 `initialization_state=complete` 把它误记为文件生成成功；是否首发必选模板文件仍缺明确业务输入。本报告保留此边界，不重定义“全部功能完成”。

## Open questions / Not executed

- 移动看板/基础时间线的首发范围应独立核对；尚非已确认缺陷。高级甘特、自动工时和资源优化仍不是本项首发要求。
- 没有重跑 HTTP、SQL、客户端、完整财务/合同/归档链，未穷尽任意权限撤回并发、每一种任务重开/依赖组合以及任意业务样本；现有测试/媒体不能据此记为本轮新通过。
- 公司项目模板、角色/审批/结项制度与真实业务样本/正式验收仍未提供；它们不是本次新确认的代码缺陷，也不关闭 GAP-01 或其余22包。
