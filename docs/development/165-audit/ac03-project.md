# AC-03 项目闭环：单项代码对照审查

审查基线：`3dc6deeba29298fbff101e818ac6b68e944db07a`（main，开始时工作区干净）。本项为静态只读审查；唯一新增内容是本报告。未执行测试、构建、浏览器、服务或数据库命令，未调用 Docker、WSL、虚拟机或模拟器。

## 1. 原始需求与结论

原文：`docs/06-decisions-risks-and-acceptance.md:93`，AC-03／U-17：

> 项目从立项到归档：任务/依赖、里程碑、交付验收、风险变更、文件合同与成本能闭环追踪。

细化依据为 `docs/requirements/03-projects-and-knowledge.md:9–52`（PM-01～09、项目状态及 PM-05 结项约束），以及 `docs/design/17-project-closeout-and-archive.md`、`docs/design/18-mobile-project-collaboration.md`。当前范围保留项目、四端及普通财务／文件能力；不把设备、网络性能、历史迁移／容灾、AI 费用控制恢复为门槛。

**单项完整合规结论：undecidable。已走读的主闭环未确认新的可行动代码偏差。** 下表说明实际读到的执行约束及其边界，而非把整个 AC-03 或全部 23 包标记为完成。当前公司规则、真实配置和完整业务样本／正式验收仍不足，且本次没有运行验证来证明所有分支。

## 2. 已走读的实际执行链

| 编号 | 必须成立的事实 | 实际调用链与约束证据 | 本次判断 |
| --- | --- | --- | --- |
| P03-01 | 立项使用准确模板修订；项目／成员／目录／任务／里程碑不能半完成 | `httpapi/projects.go:44` 的创建入口到 `project/service.go:149–251`；`Commands.Do` 内验证有效成员，`FOR SHARE` 锁定已发布模板，保存准确 `template_version`，创建项目、成员、标准目录后在同一事务调用 `templates.go:9–46` 初始化任务与里程碑。`template_management.go:26–30` 的结构仅含目录、任务、里程碑。 | 读到事务闭环。没有把模板文件字节生成视为已实现，见边界。 |
| P03-02 | 任务依赖必须同项目、无环、前置完成后才执行；结束项目不能继续普通写入 | `httpapi/projects.go:57–74` 到 `tasks.go:164–208,219–329,361–458,469–565`；先锁项目，再锁当前任务，检查预期版本、有效父任务／里程碑／参与人；依赖加入验证前置读取权、同项目、递归环；开始／审核／完成检查前置状态；父完成检查未取消直接子任务；项目锁使同项目依赖编辑串行。`0002_projects_knowledge.sql:42–55` 补充同项目外键及非自父约束。 | 已读路径没有前端绕过的依赖写入；没有声称已穷尽所有重开／并发组合。 |
| P03-03 | 里程碑与交付必须基于实际完成／独立验收，不用任务完成代替交付验收 | `httpapi/collaboration.go:17–35` 到 `milestones.go:41–45,122–198`：派生逾期，达成要求存在实际关联且全部任务完成／交付 accepted。`httpapi/deliverables.go:11–44` 到 `deliverables.go:176–223,249–310`、`deliverable_revisions.go:138–189`：固定修订和文件，按项目／交付版本重检，验收固定 `revision_id/content_hash`、文件与准则确认、真实近期 MFA，禁止提交人自验收，决定和交付状态同事务。`0044_project_deliverables.sql:66,91,112` 另有结束阶段和固定来源约束。 | 已读到实际验收约束；历史决定不因重提／文件头变化而自动换依据。 |
| P03-04 | 风险实现必须留下问题；批准变更不能自动等同实施，也不能绕普通编辑更改活动项目范围 | `httpapi/collaboration.go:36–52` 到 `concerns.go:175–294`：realized 另需 issue.create，创建问题与风险引用在同事务，问题解决／关闭需说明。`httpapi/project_changes.go:15–57` 到 `change_mutations.go:335–405`：固定批准修订、审批请求修订、内容摘要、项目／预算基础、当前权限与近期 MFA；`0045_project_changes_and_budgets.sql:201–235` 校验后由触发器原子应用范围／日期和预算版本；活动／暂停项目普通更新不能绕过。 | 读到风险→问题及批准→人工实施的区分；未把公司变更阈值／例外规则视为已确认。 |
| P03-05 | 文件／合同／费用等应能由项目追踪，且预算、审批、实际发生和现金不能混为一谈 | `projects.go:16–26` 的业务上下文／概况入口；`overview_reads.go:20–118` 使用稳定 ID、当前权限过滤和完整可读计数／5 个样本，费用经 `finance.ProjectIncurredInTx`，预算独立列出。文件要求有效目录链；费用／现金通过跨项目分摊和合同返回关联纳入。`0046_project_closeout_and_archive.sql:2–19` 收集直接／分摊／合同／采购／报销／订阅费用及退款／反向现金家族；最新 `0122_closeout_refund_units.sql:46–97` 核查 13 类实际事实，付款分摊使用费用单位 `cost_amount`，退款使用原本金单位。 | 已读到跨业务实际来源及单位区分；本报告不重复替代财务 AC 的完整审查。 |
| P03-06 | 归档需完整当前权限、准确事实、零阻塞、人为复盘、近期 MFA 与固定附件；不能普通 transition 绕过 | `httpapi/project_closeout.go` 的 GET closeout／archives、POST archive 到 `closeout_reads.go:21–127`、`closeout_mutations.go:17–139`。缺权类别显示 restricted，隐藏计数／指纹／行动；超过 5000 事实拒绝完整核查，不能截断归档。归档锁定项目与相关费用／现金，重检项目版本、13 类快照指纹、附件、近期真实人会话、两项明确确认，再同事务写复盘、固定引用、状态和事件。`service.go:353–419` 普通 archive 直接拒绝；`0046...sql:96–123` 在 SQL 层强制真实会话、全部当前权限、准确快照、无阻塞、归档和附件同提交及不可修改。 | 未发现普通状态入口绕过。Go 条件之外的 SQL 约束已纳入判断，没有仅依据 handler 认定缺口。 |
| P03-07 | 归档只读但保留历史，历史内容仍按当前权限读取；四端不能用旧结果冒充当前事实 | `closeout_reads.go:129–259` 读取归档时重新核查历史项、当前读取权和固定文件；`service.go:421–445` 重开保留原档案并要求 project.reopen。`finance/content.go:41–68`、`files/folders.go:51–71` 在实际写入锁下拒绝 closing／archived／cancelled。Web `Projects.tsx:72–79`、`ProjectCloseout.tsx:44–109` 隐藏失败缓存，分开 restricted／空集合，固定提交依据，显式重读保留文字并清空确认；共享移动端 `project/index.tsx:59–61`、`resources.tsx:33–35`、`closeout.tsx:32–87,136–176,186–207` 同样只读、重读当前权限并验证归档回执／历史。 | 已读 Web／共享 Taro 调用到同一后端；未执行浏览器、三平台构建或真机验收。 |

通用授权／事务并非仅靠页面按钮：`platform/commands.go:22–126` 在幂等回执前锁定组织／用户授权版本并执行授权；`deliverable_revisions.go:40–60` 的重复读事务再次核对组织与用户状态／授权版本；归档 SQL 另外固定真实会话与 MFA。这里没有把普通读到数据当作写授权或验收。

## 3. 查找与反证过程

| 查找范围／模式 | 结果及用于排除的误判 |
| --- | --- |
| 权威需求中 `AC-03`、`PM-01`～`PM-11`、`Q-18`、`归档`、`模板`、`子任务`、`前置` | 把必须的闭环与明确后续能力区分；先前猜测的需求路径不存在，改由 `rg --files docs/requirements` 找到实际 `03-projects-and-knowledge.md`，未把错误路径当缺功能证据。 |
| HTTP 实际 `HandleFunc`：projects、collaboration、deliverables、project_changes、project_closeout | 确认 GET/POST/PATCH 的真实 handler 与 service；普通状态入口、独立归档入口及交付决定入口分别追踪。 |
| 项目代码中的 `FOR UPDATE`、`dependencies`、`parent`、`cycle`、`CurrentRevision`、`RequireInteractive`、`projectWritable` | 追到锁、递归检查、固定修订和近期人会话，而非仅靠函数名或测试名称判断。 |
| 全部 SQL 中 `project_closeout_items`、`project_closeout_snapshot`、`project_closeout_readable_groups`、`CREATE TRIGGER`、`configuration` | 核对 0046 的定义及 0089／0122 后续覆盖，实际采用最新 0122 的关闭事实；排除仅看旧 migration 漏掉外币修复的误判。 |
| Web／共享移动端 `closeout`、`archive`、`visibility`、`fingerprint`、`revision`、`readOnly` | 确认真实用户入口、只读控制、固定请求和重读流程；未以单元测试或旧媒体推断当前界面通过。 |

本次没有新的 AC-03 候选发现交给独立反证，因此没有伪造“发现已复核”。另受主审委派独立反证的 FIN-10 提醒快照候选已单独回报；它不被混入 AC-03 的结论。

## 4. 有界能力、剩余不确定性

1. `docs/development/67-project-template-management.md:19` 明示模板仅生成逻辑结构，不含模板文件字节、自动日期、模板依赖或跨项目应用器；源结构和应用器与该边界一致。PM-02 中更广模板文件异步生成仍应作为未实现后续能力记录，不能因当前 `initialization_state=complete` 把文件生成计为完成。本次未证明其属于 AC-03 首发路径必选输入，不将其升级为新的运行缺陷。
2. PM-04 明确首发不含高级甘特拖拽／资源优化；Q-18 业务例外未冻结，不能提供“忽略全部”的归档通道。缺少这些能力不等于本次发现代码与首发约定冲突。
3. 未穷尽所有任务重开／层级组合、所有服务与评论写入口、任意权限撤回并发、全部财务家族、所有 agent 固定草案入口和全部界面错误恢复分支；不能从本次读过的路径推导任意组合皆正确。相关普通模块／授权 AC 还需独立审查。
4. 未以真实公司项目、合同／采购履约、月度账期和跨币种全旅程验证；也未重新执行 SQL、HTTP 或客户端测试。保留既有证据的原来源与延期说明，不将静态推理记成运行成功。

本报告只支持“上述实际路径已读、未确认新的 AC-03 代码偏差”，不支持 AC-03 正式通过、全 23 包完成或生产可用结论。
