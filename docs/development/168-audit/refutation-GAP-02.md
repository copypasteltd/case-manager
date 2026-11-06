# GAP-02 candidate — 合同履约节点关联的独立反证

基线：`930cdde97b9d5d2e85d892664b0857a10a811739`。本报告由 GAP-01 审查者按主审另行委派，对另一作者提出的 GAP-02 候选做独立源码/规格反证；不是候选原作者自行复核。仅阅读源码/规格，未运行测试、构建、浏览器、数据库或服务，未更改应用代码，未使用本地虚拟化。

## Requirement

> 支持主合同、补充协议关联，变更保留版本与审批记录；签署件和工作草稿分开标记。履约节点关联任务、采购/报销和费用记录。合同审核由有权人员负责，秘书可提取字段和提醒，不将模型理解当作法律结论。电子签名服务不默认包含在首发范围。
>
> — `docs/01-requirements-and-scope.md:101`。

候选：现有合同节点主要是财务节点，未完成上述履约事项关联。反证问题：现有财务事件、费用来源和项目交付是否已经提供了真正的稳定关系，足以推翻“未关联”判断。

**Verdict: partial / narrowed-confirmed。** “合同节点没有费用、采购或报销关联”这一宽泛说法不成立：存在真实固定费用关联及采购/报销的间接来源链。**但没有找到合同准确节点与任务之间的可保存、可读回关系**；同一项目、自由文字条件、项目里程碑不是该关系。因此至少“任务关联”这一明确要求仍有可行动源码差异。此判断不要求新增任意通用关系框架，也不要求自动执行/自动完成任务。

## Independent paths walked

| 反证方向 | 当前源码事实 | 对候选的影响 |
|---|---|---|
| 合同节点是否只有名称描述而完全无稳定身份 | `contracts/models.go:117–142` 的 `PaymentPlanItem.ID`、`Milestone.ID/ContractID/RevisionID/PlanItemID` 有稳定合同及准确修订身份；`contracts/schedules.go:13–98` 校验计划、金额、币种、到期日、唯一节点，`Milestones:106–128` 实际从 `contract_milestones` 读取。 | 不能声称没有准确节点。它们目前表达收付款节点和文本条件。 |
| 节点→费用是否已经实现 | `finance/contract_finance_models.go:14–25` 有准确 `MilestoneID`、来源 ID/版本/修订/hash 与金额；`finance/contract_finance_mutations.go:188–231` 的 `LinkContractFinance` 接受 `kind=cost`，检查准确来源及余额，写入原财务事件。`migrations/0039_contract_financial_reconciliation.sql:17` 把事件节点外键指向 `contract_milestones`。 | 费用关联确实存在，必须保留，不能另造重复费用。 |
| 费用→采购/报销是否已经实现 | `finance/links.go:40–47,88–107,131–137` 使用已批准采购/报销的准确修订和 `incurred_for` / `reimbursed_via` 关系，保存 `cost_links`；`finance/business_cost.go:146` 起由这三种批准来源显式登记费用。 | 存在 `合同节点→费用→采购/报销准确修订` 的真实间接证据链；不能把这些来源一概写成 absent。是否另需提前计划阶段的直接采购/报销关系，规格没有在本句给出阶段，不能擅自扩大缺陷。 |
| 调用方是否可查阅真实费用 | Web `ContractFinance.tsx:33–36` 和共享移动 `business/contract-finance.tsx:58–64` 按准确节点读取财务事件，提供费用/资金来源跳转。 | 不只是数据库里不可达的关联；已有真实客户端路径。 |
| 项目任务的 `milestone_id` 是否就是合同节点 | `project/models.go:65,82,95` 和 `project/tasks.go:190,271–300` 将任务里程碑交给项目 `validMilestone`；`project/milestones.go:202–216` 读取 `milestones` 并核对同一项目；`migrations/0002_projects_knowledge.sql:54` 明确外键是 `milestones(org_id,project_id,id)`，不是 `contract_milestones`。 | 同名字段不能反证缺口；任务并没有以该字段关联合同节点。 |
| 项目交付是否提供另一条等价的准确关系 | `project/deliverable_models.go:18,30,72` 的里程碑同样是项目里程碑；`migrations/0010_project_collaboration.sql:14` 外键指向项目 `milestones`。交付修订固定文件版本和准则，不含合同节点目标。 | 项目交付/任务均属同项目不等于该交付或任务履行某个准确合同节点。 |
| 文本条件是否已实现任务链接 | `PaymentPlanItem.Conditions` 和 `Milestone.Conditions` 均为字符串；计划校验仅校验文字长度，当前模型和读取没有任务 ID 集合、目标版本、关系凭据或目标查询。 | 文本可以描述任务，却不能证明有稳定的普通业务关联，不能视为满足这句需求。 |

## Searched

- 在 `app/backend/internal/contracts` 和合同财务模型/实现检索 `task_id|task_ids|purchase_id|claim_id|related_|target_type|target_id|performance`：无当前字段/关系实现；再直接读计划/节点字段、校验和实际读取，避免仅凭0命中推断缺失。
- 在全部迁移检索 `REFERENCES .*milestones|FOREIGN KEY.*milestone`：任务及交付分别指向项目 `milestones`；合同财务事件指向 `contract_milestones`。未发现把任务记录连到合同节点的外键/关系表。
- 枚举 `app/backend/internal` 的 `*link*/*relation*/*association*`：现有通用命中为实际费用的 `finance/links.go`，该文件的全部允许来源为采购、报销、生效合同及订阅账期，不接受任务或合同节点作为费用关系的另一端。
- 正向读取两端模型、项目 `validMilestone`、合同节点 `Milestones`、财务 `LinkContractFinance`、费用 `sourceInfo`/`Link`，以及 Web/共享移动的节点事件调用方；未把某个漏搜名称当作反证。

## Result for implementation

保留已有合同节点→费用→采购/报销证据链和财务口径。确认需要补齐的是**可选择、保存、读回并追溯准确合同节点和任务的普通业务关系**；应沿用当前权限、合同/任务同范围、准确版本、幂等/审计与归档只读等约束。现有关系不应被新关系冒充为付款、任务完成或验收。是否另加提前计划阶段采购/报销直接关系、独立非收付款节点或统一关系引擎，不由本反证自动要求。

没有执行运行验证，也没有证明真实公司制度或正式合同旅程通过；此反证只确认上述狭义源差异，并推翻候选中可能过宽的“全部关联皆无”表述。
