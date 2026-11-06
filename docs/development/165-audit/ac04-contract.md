# AC-04 合同单项源码核对 / Contract source audit

本报告只核对 AC-04。基线为 `3dc6deeba29298fbff101e818ac6b68e944db07a`，2026-11-06 本地源码；采用 `spec-to-code-compliance` 的单项需求、调用链与反证流程。没有运行测试、构建、浏览器、数据库、Worker 或临时服务，没有修改应用实现。结论是静态实现判断，不是四端业务验收或真实配置验收。

## 1. 需求与判定

原始要求：

> AC-04：合同建立、变更、签署、履约、到期提醒；不同版本与补充协议关系、审批、签署件和节点记录完整。

来源：[原始验收表](../../06-decisions-risks-and-acceptance.md#L94)。[当前适用性](../../quality/04-current-acceptance-applicability.md#L25)保留本项，[TC-OA-05](../../quality/04-current-acceptance-applicability.md#L75)保留“草稿→批准→签署→生效→补充→终止，版本与签署件固定、未结节点可见”。细化约束见 [OA-07 固定正文及主／补充合同](../../requirements/04-workflows-and-oa.md#L25)、[OA-08 已签内容变更与关闭](../../requirements/04-workflows-and-oa.md#L31)。

**判定：部分实现（partial），置信度中高。** 后端有完整的普通合同状态与固定证据实现，Web 有现存补充协议的主合同入口；共享移动端现有合同的阅读页面遗漏主合同关系展示／入口，见 F-AC04-01。其余已走读路径未发现需要新增的确定性源码缺口。公司制度、实际往来／合同样本、对象存储及正式四端验收仍不能由本报告判断。

## 2. F-AC04-01：共享移动端遗漏现有补充协议的主合同关系

| 项目 | 证据及影响 |
|---|---|
| 需求 | AC-04 要求补充协议关系完整；[OA-07](../../requirements/04-workflows-and-oa.md#L27)明确支持主合同／补充协议关系。四端仍在当前范围中。 |
| 触发 | 用户在微信、抖音或 Android 打开一个已创建的补充协议，返回的 `Contract.parent_contract_id` 非空；用户拥有该补充协议的普通读取权。 |
| 当前行为 | 普通业务详情把合同交给 [PayloadSnapshot](../../../app/mobile/src/pages/business/index.tsx#L51)。[currentBusinessPayload](../../../app/mobile/src/platform/business-input.ts#L203)构造的合同投影不包含主合同字段；[合同快照渲染](../../../app/mobile/src/pages/detail/snapshot.tsx#L53)展示合同字段、项目、正文与计划，但不展示主合同。其余阅读／履约分支也没有主合同入口。 |
| 排除反证 | [businessInput](../../../app/mobile/src/platform/business-input.ts#L106)确实读取 `parent_contract_id`，但用于编辑输入。唯一“补充协议归属／选择主合同”UI 在 [editor.tsx:97](../../../app/mobile/src/pages/business/editor.tsx#L97)的 `!value` 新建分支，已有记录连编辑页也不展示此关系。[生效表单](../../../app/mobile/src/pages/business/contract-execution.tsx#L20)仅提示核对主合同，没有其标识或入口。 |
| 平台差异 | Web [Contracts.tsx:41](../../../app/web/src/Contracts.tsx#L41)已明确显示“此记录为补充协议”并链接准确 `parent_contract_id`。后端 [validateParent](../../../app/backend/internal/contracts/service.go#L373)也保存并校验关系，故这不是缺少服务器资料或公司制度。 |
| 后果 | 移动用户能够新建、批准、签署及使用补充协议，却无法从既有补充协议的普通阅读页面辨认或进入其主合同；需另行查找。影响三个共享移动交付面，属于阅读闭环缺口。 |
| 建议 | 在已有合同的普通详情按原始 `Contract.parent_contract_id` 展示关系并提供现有合同路由。不能推断或展示未授权主合同的名称；进入时仍走主合同当前普通读取权。无需修改历史审批快照、合同 hash、数据库结构或文件格式边界。 |
| 优先级／状态 | P2 源码缺口；主审独立检索移动详情、快照和创建分支后确认成立。主轮次将单独修复当前关系阅读区；本报告未修改代码、未执行回归。 |

## 3. 后端实际执行点

| 业务性质 | 已发现的实现证据 | 静态判定 |
|---|---|---|
| 建立及普通权限 | [contractRoutes](../../../app/backend/internal/httpapi/contracts.go#L12)提供列表／创建／更新／历史／签署／生效／节点接口。[Create](../../../app/backend/internal/contracts/service.go#L319)在命令事务内检查分类、项目与 `contract.create`、`contract.read`，再写合同头和首个修订；[validateParent](../../../app/backend/internal/contracts/service.go#L373)校验当前可读、未回收、同项目同分类主合同。 | 已实现源码路径；不等同已接受公司分类政策。 |
| 不可变修订与防覆盖 | [contentHash / insertRevision](../../../app/backend/internal/contracts/service.go#L288)把合同 ID、项目、主合同、分类与完整内容纳入 schema2 hash；[update](../../../app/backend/internal/contracts/mutations.go#L69)检查 `expected_version`、`base_revision_id`，仅普通可编辑草稿可追加新修订，关联申请回到草稿／未送审。[historyAccess](../../../app/backend/internal/contracts/history.go#L21)对历史保持当前 `contract.read` + `contract.update`。 | 已实现。历史读取较普通详情需要更高授权，属于明确行为而非默认对所有读者开放。 |
| DB 的独立约束 | [0027_contract_revisions.sql](../../../app/backend/migrations/0027_contract_revisions.sql#L49)禁止修订更新／删除并保护范围；[0029](../../../app/backend/migrations/0029_contract_signatures.sql#L64)约束确切批准、签署与生效；后续 [0040](../../../app/backend/migrations/0040_contract_closure.sql#L100)替换生命周期 guard，允许受证据约束的完成／终止。因此不能把旧 0029 只允许前两次转移误报为缺少关闭。 | 已沿后续覆盖定义反查。 |
| 完整送审依据 | [ApprovalDraft](../../../app/backend/internal/contracts/approval.go#L14)对送审检查完整商业字段、有效负责人、确切往来修订、正式正文和收付计划；[contractSubject](../../../app/backend/internal/workflow/request_business.go#L28)固定业务修订、hash、正文证据描述。 | 已实现；制度模板本身仍待公司采纳。 |
| 人工审批与精确版本 | [Decide](../../../app/backend/internal/workflow/decisions.go#L28)检查当前分配人、申请／步骤版本、当前申请修订及 hash、业务修订及 hash、当前步骤与实例、独立人工会话及制度 MFA。[approvalAuthority](../../../app/backend/internal/workflow/authority.go#L73)重查业务、往来及每个附件的当前授权和完整描述；[setBusinessState](../../../app/backend/internal/workflow/request_business.go#L164)只在 approved 后固定批准修订并进入 pending_signature。 | 已实现；秘书不能借普通审批路径取代人工决定。 |
| 签署及生效分离 | [RegisterSignature](../../../app/backend/internal/contracts/signatures.go#L75)要求确切批准申请／修订／实例、正文、签署件、人工说明及不晚于香港当日的真实签署日期；[Activate](../../../app/backend/internal/contracts/signatures.go#L133)再次核对确切签署、已达到批准生效日期、当前正文／往来／签署件及主合同有效状态。[mutate](../../../app/backend/internal/contracts/mutations.go#L14)对 `contract.execute` 要求当前交互会话与近期 MFA，并在锁内复查。 | 已实现。登记签署不会自动生效，生效不会自动产生实际费用／现金。 |
| 补充协议与已签内容 | 已批准头不可普通改写，需另建关联合同；[0040 的 lifecycle guard](../../../app/backend/migrations/0040_contract_closure.sql#L100)和 [contract_activation_evidence](../../../app/backend/migrations/0029_contract_signatures.sql#L77)保持原生效修订与有效主合同。 | 后端与 Web 有实现；移动阅读缺口见 F-AC04-01。 |
| 收付里程碑及履约记录 | [normalizeSchedule](../../../app/backend/internal/contracts/schedules.go#L13)区分未知、明确无收付与分期；检查节点唯一身份、日期、原币精度、正金额及合计。 [materialize_contract_milestones](../../../app/backend/migrations/0030_contract_payment_schedules.sql#L91)从准确 active 修订生成不可变完整节点。[mutateContractFinance](../../../app/backend/internal/finance/contract_finance_mutations.go#L74)对合同、费用／现金资料当前授权、人工近期 MFA、合同版本和 finance 版本双重核对，原事实与归属／核对独立。 | 已实现；未把审批、计划或归属自动宣称为真实付款。 |
| 完成／终止 | [contractClosureBlockers](../../../app/backend/internal/finance/contract_closure_reads.go#L35)检查未知计划、节点差额、待归属退款、核对新鲜度、未确认条件、未结费用及补充协议；[CloseContract](../../../app/backend/internal/finance/contract_closure_mutations.go#L73)核对当前权限／真实人工 MFA、完整依据指纹、版本、真实日期与固定文件。完成不接受未结项，终止对允许保留的未结项要求显式承担和后续处理，生效补充协议须先处理。 | 已实现；无重新开启／级联自动关闭承诺。 |
| 关闭 DB 与后续更正 | 最新 [contract_closure_snapshot 定义](../../../app/backend/migrations/0089_cross_currency_settlement.sql#L248)含当前有效修订、节点、费用及未回收子协议；[0040](../../../app/backend/migrations/0040_contract_closure.sql#L109)约束不可变关闭事实／附件与合同头一致。财务接口明确允许 completed／terminated 后按当前授权更正独立事实。 | 已反查后来覆盖的快照，未停留在原 0040 版本。 |
| 到期规则与排程 | [policyAuthority / PublishContractPolicy](../../../app/backend/internal/notification/contract_policies.go#L56)用独立组织读／管理权限、人工 MFA、乐观版本与不可变发布；[ScheduleContracts](../../../app/backend/internal/notification/scheduler.go#L73)和 [scheduleContract](../../../app/backend/internal/notification/scheduler.go#L111)只取 effective 合同的 active 修订及明确到期日，保留阈值去重／发布业务日，并在锁内重查启用及准确策略。 | 已实现；没有替公司默选制度。 |
| 当前收件人与提醒有效性 | [recipients](../../../app/backend/internal/notification/jobs.go#L141)固定当前负责人／创建人、版本和准确到期事件；[eligible](../../../app/backend/internal/notification/jobs.go#L193)重查账号及四渠道当前普通可见性；[view](../../../app/backend/internal/notification/access.go#L100)确认当前 effective／owner／activeRevision／日期。 | 已实现排程／应用通知源码；真实渠道发送另属配置验收。 |

## 4. 固定正文、签署件与回执

1. [fileref.Resolve](../../../app/backend/internal/fileref/resolve.go#L41)按传入 `versionID` 读取，检查版本可用／扫描、当前文件读取权、活跃完整目录链、项目及分类；不会替换成当前文件 head。[ForApproval](../../../app/backend/internal/fileref/resolve.go#L24)再验证真实业务证据。合同保存时用固定版本 ID，签署和审批时另查精确证据。
2. 历史修订及普通详情通过 [references](../../../app/backend/internal/contracts/references.go#L18)按当前权限解析描述；不可读附件／往来不会因合同可读而自动公开名称或对象键。[History / Revision](../../../app/backend/internal/contracts/history.go#L31)不改变原始引用 ID。
3. [合同 replay](../../../app/backend/internal/contracts/replay.go#L11)保留原操作修订回执，同时重新读取当前普通权限、重新解析固定正文／签署件，并对不再匹配的当前能力清零。[Commands.Do](../../../app/backend/internal/platform/commands.go#L64)在幂等读取之前执行当前授权，不因旧 key 自动绕过当前检查。
4. Web [ContractDisplay](../../../app/web/src/ContractDisplay.tsx#L22)和 [ContractExecution](../../../app/web/src/ContractExecution.tsx#L21)下载分别传准确 `version_id`、seq、hash、size。相邻“查看文件记录／查看签署文件”是普通文件记录入口，存在单独的准确版本下载；本次没有把它推定成“固定版本被错误替换”。
5. 移动 [PinnedFile](../../../app/mobile/src/pages/detail/snapshot.tsx#L19)使用 `goVersion(file_id, version_id)`；[FixedEvidence](../../../app/mobile/src/pages/business/execution-controls.tsx#L12)使用 `readVersion(file_id, version_id)`。共享 [MobileContractExecution](../../../app/mobile/src/pages/business/contract-execution.tsx#L19)向签署命令提交固定业务版本／修订／hash 与独立签署文件版本；关闭提交完整依据指纹和固定证据。
6. DOCX／文本正文生成与采纳是明确有界格式能力。OA-07 的固定正文附件语义没有规定“必须把 XLSX／PPTX 作为合同散文正文生成或采纳”；本报告不将这种未指定扩展当成 AC-04 缺口，也不把有界 Office 能力等同全部真实合同格式验收。

## 5. 已走读的成功及失败路径

| 路径 | 关键反向情形 |
|---|---|
| 创建→新修订→准备申请→送审→步骤决定 | 不完整字段、错误／不可读往来或文件、失效项目、旧业务版本／修订／hash、非当前审批人、未激活步骤、禁止自审、权限撤回、旧幂等回执。 |
| 批准→登记签署→人工生效 | 非准确批准修订、未来签署日、未到生效日、缺失／不可读签署件、主合同尚未生效、非人工会话、缺少近期 MFA、并发改变。 |
| 生效→里程碑归属／人工核对→完成或终止 | 计划未知、节点未结、实际付款与费用独立、退款未归属、核对依据变化、未履行条件、未结费用、未结束或仍有效的补充协议、旧 finance 版本／完整指纹、错误日期或证据。 |
| 发布／停用到期规则→排程→应用通知 | 无组织管理权、旧策略版本、发布后旧候选、无 activeRevision／无明确到期日、重复阈值、已结束合同、非当前负责人、账号或普通权限失效。 |
| 共享移动现有补充协议阅读 | 固定正文与签署件有版本入口；主合同关系无阅读入口。已在创建／编辑／履约／快照／平台输入层反查，保留 F-AC04-01。 |

这些是阅读源码中的分支，未执行上述业务用例。

## 6. 检索记录及反证

实际主要检索（PowerShell 原生命令；通配筛选以 `-g` 传入 `rg`）：

```text
rg -n 'AC-04|OA-07|OA-08|TC-OA-05' docs/requirements/04-workflows-and-oa.md docs/06-decisions-risks-and-acceptance.md docs/quality/04-current-acceptance-applicability.md
rg --files app/backend/internal/contracts app/backend/internal/finance app/mobile/src/pages/business
rg -n 'contract|active_revision|approved_revision' app/backend/internal/workflow app/backend/internal/notification
rg -n 'parent_contract|parentContract|主合同|补充协议' app/mobile/src -g '*.ts' -g '*.tsx'
rg -n 'draft_file|signed_file|readVersion' app/mobile/src -g '*.tsx'
rg -n 'DraftFile|Attachments|VersionID' app/backend/internal/workflow/request_business.go
rg -n '^func |ForApproval|authz.Require|expected|total|sum|end_date|start_date|due' app/backend/internal/contracts/schedules.go app/backend/internal/finance/contract_finance_mutations.go app/backend/internal/finance/contract_finance_reads.go
rg -n 'CREATE( OR REPLACE)? FUNCTION (preserve_contract_scope|preserve_contract_lifecycle|contract_activation_evidence|validate_contract_schedule_revision|materialize_contract_milestones|contract_closure_snapshot|can_read_contract_closure|contract_closure_has_unsettled|check_contract_closure_consistency|validate_contract_signature)' app/backend/migrations
```

已逐段读取实际路由、合同服务／修订／历史／签署、工作流决定及证据授权、文件引用 helper、合同财务与关闭、最新 SQL 覆盖定义、到期排程／收件人授权、Web 和共享移动调用者。对检索命中打开具体代码，而非仅由函数名或单个关键词判定。

“未发现”的边界：本报告没有对所有路由、所有迁移、所有前端及秘书工具做全仓合规证明；只对上述 AC-04 调用链反查。路径通配首次在 Windows 直接展开失败，后以 `rg -g` 或准确文件路径重查；没有把检索失败当成不存在实现的证据。

## 7. 推理与仍不可决定事项

AC-04 没有要求电子签名供应商、自动银行扣款、AI 法律判决或全格式 Office 正文自动编辑。当前实现的人工固定签署、显式生效、独立收付事实与补充协议链符合细化需求方向；需补的确定阅读行为是 F-AC04-01，而不是扩大这些未指定能力。

公司制度的实际审批人数、免签条件、税／法定模板、哪些结束日期代表商业到期等需要公司采纳；真实对象存储、Linux Worker、TG／模型及正式四端配置也不能由静态源码确认。它们未被重新当成本轮新增源码 TODO。用户移出的真机、网络性能／实网 120GB、历史迁移／容灾及 AI 费用控制不在此审计中恢复为发布门槛。

本报告没有执行或声称通过 AC-04／TC-OA-05，没有生成新 APK、测试证据、截图或录屏，也没有改写任何冻结版本或既有证明。F-AC04-01 修复后的统一验证及最终裁定由主轮次记录。
