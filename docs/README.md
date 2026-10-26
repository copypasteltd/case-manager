# CopyPaste 内部管理平台文档中心

**仓库使用入口 / Repository guides：[中文完整方案与使用](README.zh-CN.md) · [Complete English guide](README.en.md) · [实际截图、MP4与GIF / Actual media](media/README.md) · [更新记录 / Changelog](CHANGELOG.md) · [贡献与同步 / Maintenance](contributing.md)。**

**当前细化基线：v1.0-draft，2026-10-24；视觉设计基线：v1.0.0-draft。** 全部后续文档保存在 `docs/`，全部开发实现位于 `app/`。身份/权限、项目协作、知识修订、文件元数据和部分 Web 页面已进入本地实现与验证；完整平台仍在建设，尚未生产部署或完成首发验收。真实进度见[代码实施台账](delivery/02-implementation-ledger.md)。

确认输入：3名香港员工/3人同时在线；现有境内服务器；2TB原件/最大单文件120GB；Go+PostgreSQL、React/Vite、Taro微信/抖音/Android、Codex秘书与TG；OSS+R2；应用机仅临时数据。四端同批首发，120GB由Web/Android上传，小程序查阅/申请/审批。不做考勤/人事。

本轮功能文档：[秘书风险与问题完整工作流 / Secretary risk and issue workflows](development/84-secretary-project-concerns.md)。

## 1. 阅读路径

负责人先读产品需求、OA/费用、决策登记和工作包；开发读工程/数据/接口及负责模块；前端和设计读页面矩阵及风格设计体系；AI开发先读权限与工具；测试/运维读追踪、配置及部署恢复。用户已确认要求优先；细化设计优先于原v0.2举例。

## 2. 治理、范围与配置

| 文档 | 内容 |
| --- | --- |
| [基线与执行规则](governance/01-baseline-and-rules.md) | 确认/设计/假设/待决策、范围、不变量、变更与术语 |
| [决策与待确认登记](governance/02-decisions-and-open-questions.md) | Q-01—18、责任角色、可继续工作与放行点 |
| [配置目录](governance/03-configuration-catalog.md) | 候选默认值、边界、生效、缺失行为 |

## 3. 需求与业务规范

| 文档 | 内容 |
| --- | --- |
| [产品需求与用户旅程](requirements/01-product-and-journeys.md) | 目标、信息架构、十条旅程、首发组织 |
| [身份、权限、分类与审计](requirements/02-identity-and-permissions.md) | 登录/MFA/绑定/撤权、ACL、渠道、引用下载 |
| [项目与知识](requirements/03-projects-and-knowledge.md) | 任务/依赖/里程碑/预算/风险/变更/交付/归档、博客/SOP |
| [审批与OA](requirements/04-workflows-and-oa.md) | 修订/快照/状态、合同、采购、报销与异常 |
| [费用与订阅](requirements/05-finance-and-subscriptions.md) | 一次费用多处关联、分摊/付款/冲销、账期/变价/取消/月报 |
| [文件与搜索](requirements/06-files-and-search.md) | 目录/版本/120GB/扫描/检索/双云/保留 |
| [秘书与渠道](requirements/07-secretary-and-channels.md) | 会话、八个Skill、预授权、固定计划、TG、预算 |
| [状态、通知与非功能](requirements/08-status-notifications-and-nfr.md) | 观测/事件/维护、通知、性能/恢复/安全/环境 |

## 4. 开发契约

| 文档/产物 | 内容 |
| --- | --- |
| [系统与工程](development/01-system-and-engineering.md) | 模块/依赖/未来目录、原生部署、事务/迁移/编码/构建 |
| [数据字典](development/02-data-dictionary.md) | 全域逻辑实体、字段、唯一/FK/校验/索引/删除 |
| [存储协议](development/03-storage-protocol.md) | 直传/源冻结/校验/正式提交/复制/恢复/清理 |
| [API目录与契约](development/04-api-contract.md) | 路径、输入、状态、幂等、错误、SSE、演进 |
| [OpenAPI](development/openapi.json) | 核心高风险机器契约；覆盖边界见x-contract-scope |
| [OpenAPI生成入口](development/build_openapi.py) | 兼容入口调用app/scripts；同源生成文档、后端嵌入契约与前端类型 |
| [事件、任务与并发](development/05-events-jobs-and-concurrency.md) | outbox、租约/fencing、锁、重试、业务去重 |
| [页面与客户端](development/06-clients-and-pages.md) | 19组页面、四端能力、表单/错误/弱网/Android原生 |
| [工具与Skill契约](development/07-agent-tools-and-skills.md) | Runtime、工具参数/权限/证据、manifest、发布/回滚 |
| [本地开发与验证](development/08-local-development.md) | 原生启动、凭证隔离、测试、浏览器关闭与当前边界 |
| [文件元数据与名称空间](development/09-file-metadata-and-namespace.md) | Unicode比较、跨类型名称互斥、移动/回收站/副本证据及实施边界 |
| [对象存储适配器](development/10-object-storage-adapters.md) | OSS/R2分片、签名范围、固定源读取、流式摘要与验证边界 |
| [上传会话与固定源](development/11-upload-control-and-freezing.md) | 容量预约、短期许可、云端对账、后台封存与竞态恢复 |
| [上传执行器与身份](development/12-upload-worker-and-cloud-identities.md) | 原生常驻进程、心跳/重试、失败事务、用途凭证与部署模板 |
| [完整性与扫描](development/13-upload-integrity-and-scanning.md) | 恢复检查点、权威摘要、ClamAV边界、不可变扫描证据与重扫 |
| [正式候选与提交](development/14-file-promotion-and-commit.md) | 可恢复复制、目标完整回读、基础版/名称冲突、正式版本及副本排队 |
| [扫描例外制度](development/15-scan-waivers-and-review.md) | 默认拒绝、明确制度、独立MFA审核、固定证据与waived状态 |
| [双云副本执行与恢复](development/16-cross-cloud-replication.md) | 分片/摘要恢复、纪元与租约、失败修复、独立云用途和验证边界 |
| [下载授权与流式读取](development/17-download-authorization-and-streaming.md) | 固定版本/副本、短签与鉴权代理、设备续期、范围/资源约束、Web保存及审计 |
| [上传队列与审核客户端](development/18-upload-queue-and-review-clients.md) | 全局传输/对账/正式提交、固定报告审核、制度修订与当前会话MFA |
| [文件历史内容恢复](development/19-history-restoration.md) | 固定来源/基础版、独立候选与显式提交、容量预留、恢复执行器和Web任务 |
| [保留、引用与清理评估](development/20-retention-and-reference-protection.md) | 独立保留角色、不可变锁/释放、业务版本引用及固定评估；物理清理待接入 |
| [OA 修订与审批协议](development/21-oa-approval-and-business-revisions.md) | 制度发布、业务修订与审批总协议；具体实施见合同与申请章节 |
| [往来资料与固定依据](development/22-business-parties.md) | 共用客户/供应方、不可变联系资料修订、权限、归档与 Web 管理 |
| [合同草稿与固定依据](development/23-contract-drafts-and-fixed-evidence.md) | 不完整草稿、不可变修订、准确往来方/正文版本、补充协议及 Web 管理 |
| [申请快照与顺序审批](development/24-request-snapshots-and-sequential-approvals.md) | 准确送审预览、人工顺序决定、重提/拒绝/撤回及 Web 工作台 |
| [合同签署与生效](development/25-contract-signature-and-activation.md) | 固定签署件、本人 MFA 登记、准确批准日期与独立生效事件 |
| [合同收付款计划](development/26-contract-payment-schedules.md) | 计划随业务修订审批、生效生成固定节点、旧版摘要兼容 |
| [秘书技能包与Codex适配](development/27-agent-skill-packages-and-runtime-adapter.md) | 八项草稿来源、发布摘要、实际凭证核验、事件/中断及验证边界 |
| [秘书会话与成员](development/28-agent-conversations-and-membership.md) | 个人/项目范围、不可变成员快照、上下文安全版本与撤权/并发 |
| [秘书资料与共同阅读范围](development/29-agent-context-and-source-projection.md) | AI 许可、全部成员交集、固定知识/文件版本、字符片段与来源摘要 |
| [分类与渠道策略发布](development/30-classification-policy-publication.md) | 不可变规则、准确前序、人工MFA、历史与组织授权同步 |
| [Taro移动客户端底座](development/31-mobile-client-foundation.md) | 内存会话/草稿、页面取消、固定正文与审批版本、两端编译与Android兼容边界 |
| [服务状态与签名观测](development/32-service-status-and-signed-observations.md) | 来源修订、精确字节签名、持久投影、新鲜度、维护覆盖与全部影响范围事件 |
| [采购报销与审批依据](development/33-purchase-and-claim-approval-drafts.md) | 不完整草稿、准确金额/垫付口径、固定供应商/凭证、商业修订和顺序审批 |
| [采购订单、验收与来源退货](development/34-purchase-orders-acceptance-and-returns.md) | 批准额度内多订单、分批验收/拒收、固定来源退货、取消余量及不可变履行记录 |
| [唯一实际费用与发生支出报表](development/35-canonical-actual-costs-and-incurred-reports.md) | 永久来源、准确分摊/确认/冲销、共同业务关联、原币月报与权限钻取 |
| [实际资金、退款与员工结算](development/36-payments-refunds-and-settlement.md) | 不可变付款分配、实时余额/并发、来源退款/冲销、员工垫付和原币现金月报 |
| [订阅、账期与预测](development/37-subscriptions-billing-and-forecast.md) | 日历原点、不可变价格/预测、停订、实际扣款对账及覆盖；本地验证，外部与多端待验收 |
| [合同节点财务归属](development/38-contract-financial-reconciliation.md) | 既有费用/资金归属、退款/解除、余额预留、固定核对及历史；本地验证，多端待验收 |
| [合同完成与终止](development/39-contract-completion-and-termination.md) | 法律阶段、固定未结事项与财务依据；完整后端／Web本地验证，多端与外部待验收 |
| [采购差异重审与执行基线](development/40-purchase-variance-approval-and-execution-baselines.md) | 完整差异、独立审批、人工启用与历史订单；后端／Web本地验证，移动共用快照编译，外部与全端待验收 |
| [采购完成与结算核对](development/41-purchase-completion-and-settlement-review.md) | 已有本地PG／完整后端及Web验证：完整交付、差异／未使用金额、实际结算、明确人工声明与不可变完成；完整四端及真实业务另验 |
| [项目交付与人工验收](development/42-project-deliverables-and-acceptance.md) | 固定文件修订、退回重提、独立人工验收及不可变历史；验证状态见协议 |
| [项目变更与预算修订协议](development/43-project-changes-and-budget-revisions.md) | 固定基线、原币预算修订、独立审批及明确实施；本地Web/API与移动审批快照、接口／事务及分端验收范围 |
| [项目结项核查与归档协议](development/44-project-closeout-and-archive.md) | 十三类业务／财务核查、准确依据、人工复盘、近期身份验证、不可变归档及受控重开；验证状态见协议 |
| [移动端项目协作与固定交付修订](development/45-mobile-project-collaboration.md) | 项目资料、任务讨论、固定版本提交、独立验收、内存草稿及并发恢复；本轮验证与原生端边界 |
| [移动端固定内容读取](development/46-mobile-fixed-content-reading.md) | 固定版本授权、1 MiB 有界 JSON 段、摘要校验、取消、暂存清理和原生查看器边界 |
| [移动业务草稿与送审](development/47-mobile-business-drafts-and-submission.md) | 合同、采购、报销完整输入、固定依据、共享内存草稿、明确送审、撤回重提与历史 |
| [秘书项目计划与准确批准](development/48-secretary-project-plans.md) | 固定意图、近期MFA、批准／撤销、原子项目创建、MCP真实回执、Web与Taro操作 |
| [秘书计划查询与明确引用](development/49-secretary-plan-discovery.md) | 有界计划查询、固定意图读取、新任务上下文、Web／Taro引用及历史查看 |
| [秘书常规项目／任务更新](development/50-secretary-routine-updates.md) | 明确 L1 范围、原业务校验、固定来源与真实提交回执 |
| [秘书指定文件版本正文读取](development/51-secretary-file-text.md) | 专用读取身份、完整字节校验、两阶段授权、Web／Taro准确版本及MCP回执 |
| [秘书准确文件更新](development/52-secretary-file-updates.md) | 固定范围、真实差异、隔离草稿、扫描、正式新版本与两端操作回执 |
| [站内通知与投递记录](development/53-in-app-notifications.md) | 实际业务事件、当前收件范围、未读／待处理、可靠投递、两端入口与异常核对 |
| [账号安全与密码恢复](development/54-account-security-and-recovery.md) | 本人设备会话、单设备撤销、修改密码、管理员单用恢复凭证及两端入口 |
| [秘书目录整理与准确计划](development/55-secretary-directory-organization.md) | 目录盘点、逐项计划、本人MFA批准、原子目录／文件变更与Web／Taro回执 |
| [秘书文本文件创建](development/56-secretary-file-creation.md) | 准确目录／名称授权、文本草稿、实际扫描与正式首版本、Web／Taro入口 |
| [秘书准确业务申请](development/57-secretary-oa-requests.md) | 原始登录会话绑定、合同／采购／报销申请起草／修订、真实预览送审及Web／Taro回执 |
| [秘书知识草稿与内部发布](development/58-secretary-knowledge-writes.md) | 有限正文生成、准确原文修订、完整内容核对后内部发布，来源分类与项目约束及Web／Taro回执 |
| [秘书里程碑与项目阶段](development/59-secretary-project-stages.md) | 准确里程碑资料／状态、项目阶段、原登录绑定及Web／Taro业务回执 |
| [TG请求调度与秘书会话](development/61-telegram-agent-dispatch.md) | 独立渠道上下文、实际运行队列、读取工具约束、Web／Taro结果与取消；真实模型／群外发仍待验收 |
| [Telegram本人绑定与群入口](development/60-telegram-binding-and-ingress.md) | 私聊证明与门户确认、准确群／话题、消息去重入库、本人请求和配置历史；执行已由增量61延伸，外发待接通 |

八项秘书技能的说明源保存在以下目录；输入输出Schema与清单位于app/skills/。当前技能均为草稿，尚未发布，不代表真实模型、工具网关或业务执行链已接通。

| 技能说明 | 用途 |
| --- | --- |
| [共用执行边界](skills/execution-boundaries.md) | 身份、工具范围、可信来源与实际执行凭证 |
| [新建项目](skills/project-create/SKILL.md) | 依据明确范围生成项目创建操作 |
| [更新项目](skills/project-update/SKILL.md) | 固定对象与版本，更新项目或任务信息 |
| [更新文件](skills/file-update/SKILL.md) | 生成差异与草稿，经业务流程提交版本 |
| [整理资料](skills/information-organize/SKILL.md) | 有界整理范围及固定批量计划 |
| [OA协助](skills/oa-assist/SKILL.md) | 合同、采购、报销的草稿与送审协助 |
| [订阅核对](skills/subscription-review/SKILL.md) | 按账期及财务口径读取和核对 |
| [项目简报](skills/project-briefing/SKILL.md) | 基于实际可读来源形成项目摘要 |
| [事件摘要](skills/incident-summary/SKILL.md) | 基于服务观测与事件形成摘要 |

## 5. 官网风格与产品设计

| 文档/产物 | 内容 |
| --- | --- |
| [设计体系总入口](design/README.md) | 设计结论、证据/扩展等级、约束与阅读路径 |
| [风格设计总规范](design/00-style-design-specification.md) | 官网风格映射、色系/字体/布局、组件与业务界面、四端适配、交付约束及待定设计决策 |
| [官网风格实证](design/01-website-style-audit.md) | 当前色值/字体/Logo、取证截图和继承边界 |
| [视觉基础与令牌](design/02-visual-foundations.md) | 明暗色板、排版、密度、尺寸、无障碍与图表 |
| [组件与交互](design/03-components-and-patterns.md) | 表单/表格、审批、120GB与副本、秘书计划、状态 |
| [页面与四端](design/04-page-and-cross-platform-design.md) | 19组页面配方、响应式、微信/抖音/Android |
| [实现与设计验收](design/05-implementation-and-design-qa.md) | 令牌适配、交付规则、VD-01—14、工作包/门禁 |
| [本轮设计验证](design/06-design-verification.md) | 实际静态/浏览器检查和未验证边界 |
| [设计交接与组件契约](design/07-design-handoff-and-component-contracts.md) | CP-01—28组件、状态与权限分支、命名/交接模板 |
| [关键页面布局与任务流](design/08-page-layouts-and-flows.md) | 19组页面的重点布局、业务路径与异常示例 |
| [品牌应用与文案规范](design/09-brand-application-and-content-style.md) | 色彩使用顺序、模块风格、构图配方、术语和跨端输出 |
| [订阅与财务页面设计](design/10-subscriptions-and-financial-views.md) | 订阅维护、账期对账、三种月报、覆盖提示与提醒 |
| [合同节点与财务归属](design/11-contract-financial-reconciliation.md) | 固定计划与实际金额、来源选择、退款/解除、人工核对与历史 |
| [合同完成与终止设计](design/12-contract-completion-and-termination.md) | 法律阶段、未结事项、人工核实、固定结束凭证、冲突和历史变化 |
| [采购差异与执行基线设计](design/13-purchase-variance-and-execution-baselines.md) | 原批准与拟执行内容对照、独立重审、明确启用、历史订单与冲突 |
| [采购完成与结算核对设计](design/14-purchase-completion-and-settlement-review.md) | 完整固定集合、无应付人工核实、不可变历史、后续变化及受限状态 |
| [项目交付与人工验收设计](design/15-project-deliverables-and-acceptance.md) | 项目页签、准确修订、人工声明、历史与版本冲突保留 |
| [项目变更与预算修订设计](design/16-project-changes-and-budget-revisions.md) | 固定项目基线、完整提案与预算口径、独立审批、人工实施及历史；页面待验收 |
| [项目结项与归档复盘设计](design/17-project-closeout-and-archive.md) | 官网风格下的完整清单、权限未知、复盘表单、字段定位、冲突保留和历史；跨端边界独立说明 |
| [移动端项目协作与验收设计](design/18-mobile-project-collaboration.md) | 单列项目资料、准确文件版本、明确核实声明、评论与历史、权限和冲突状态 |
| [固定版本阅读页设计](design/19-mobile-fixed-content-reading.md) | 官网双主题、版本事实、文本／图片、文档降级、读取／取消／清除及实机边界 |
| [移动业务草稿与送审设计](design/20-mobile-business-drafts-and-submission.md) | 合同／采购／报销长表单、稳定明细、固定依据、完整送审预览、错误定位及历史 |
| [可交互风格样板](design/styleguide.html) | 本地查看明/暗主题与组件；演示数据，不连接业务 |
| [令牌JSON](design/tokens.json) / [CSS](design/tokens.css) | 两个主题各39个语义值及字体/尺寸等；JSON为源 |

## 6. 质量、运维与实施

| 文档 | 内容 |
| --- | --- |
| [测试策略与场景](quality/01-test-strategy-and-cases.md) | 权限/业务/并发/大文件/AI/TG/多端/恢复用例 |
| [追踪、验收与门禁](quality/02-traceability-and-release.md) | U/AC→需求→工作包→TC、G0—G5 |
| [原需求/开发文档验证快照](quality/03-document-verification.md) | 设计文档新增前的检查；新设计验证见上节 |
| [部署与日常运维](operations/01-deployment-and-operations.md) | 原生服务、最小身份、配置、发布/回滚、告警/处置 |
| [备份、迁移与恢复](operations/02-backup-migration-and-recovery.md) | 一致快照、2TB迁移、重建/epoch、演练/清理 |
| [工作包与完成标准](delivery/01-work-packages-and-gates.md) | WP-01—14、依赖、Ready/Done、估算边界 |
| [代码实施与验证台账](delivery/02-implementation-ledger.md) | 实际实现、测试证据、未完成范围；全量目标不缩减 |
| [当前Goal差异评估](delivery/03-goal-gap-assessment.md) | GAP-01—28、当前代码与验证基线、缺失交付物、外部依赖及推进顺序 |
| [记录模板](templates/01-record-templates.md) | 需求变更、ADR、开发评审、缺陷/测试、发布/恢复 |

## 7. 原立项论证与来源

| 文档 | 用途 |
| --- | --- |
| [00 可行性评估](00-feasibility-assessment.md) | 总体结论与前提 |
| [01 原需求范围](01-requirements-and-scope.md) | U-01—20、原R编号与假设 |
| [02 原架构方案](02-architecture-and-data.md) | 架构选型论证 |
| [03 原秘书/TG方案](03-ai-secretary-and-telegram.md) | Codex路线与依据 |
| [04 原存储恢复方案](04-storage-backup-and-recovery.md) | 提供商能力与传输模型 |
| [05 工作量与预算](05-delivery-plan-and-budget.md) | 186–276人日规划区间与费用模型；非报价 |
| [06 原决策与验收](06-decisions-risks-and-acceptance.md) | ADR-01—14、AC-01—30 |
| [07 官方核实记录](07-research-sources.md) | 2026-10-22来源；上线前重核易变条件 |

## 8. 当前状态

| 最近增量 | 已实现内容 |
| --- | --- |
| [原生Prometheus／Blackbox桥接](development/81-native-prometheus-status-bridge.md) | 原采样时刻、受限读取、持久序号及签名正文、准确回执、既有Web／Taro故障／恢复与历史验证 |
| [移动知识与项目博客完整工作流](development/83-mobile-knowledge-workflows.md) | 四类知识、准确附件、正文／评论修订、发布／恢复、权限与回执约束 |
| [移动项目变更与预算修订](development/80-mobile-project-changes-and-budgets.md) | 完整项目／币种基线、固定提案、原申请修订及独立审批、本人MFA明确实施、实际项目／预算证明及不可变历史 |
| [移动项目结项与固定归档](development/79-mobile-project-closeout-and-archive.md) | 十三类完整核查、阶段操作、固定复盘、两项声明与本人MFA、冲突保留、实际项目／历史证明及受控重开 |
| [移动任务、子任务与前置依赖](development/78-mobile-task-editing-and-dependencies.md) | 手工新建／编辑、实际候选、空值解除和依赖回读；124项移动测试、9组真实流程、6项布局，新增2张截图及任务MP4／GIF |
| [仓库整理、双语指南与实际演示](development/77-repository-publication-and-bilingual-readmes.md) | 双语完整README、15张初始截图、3段MP4／3个GIF、来源绑定与提交检查、后续同步推送约束；不改变生产放行状态 |
| [TG群通知、回执与对账](development/64-telegram-delivery.md) | 原群阶段登录链接、持久准备、共享限流、未知投递人工对账及 Web／Taro 本人查询 |
| [Android原生宿主与业务接入](development/65-android-native-foundation.md) | Taro／RN宿主、原生请求与私有阅读；实际调试APK、11项Android测试及14项包检查通过 |
| [Android原生分片上传与恢复](development/66-android-resumable-uploads.md) | SAF原件授权、流式分片、前台通知／暂停、本人恢复队列、准确候选提交；25项Android测试及17项包检查通过 |
| [项目模板管理与固定修订初始化](development/67-project-template-management.md) | 中性结构草稿、准确发布／停用／新修订、只增历史、项目选择和同事务初始化；实际API浏览器链验证 |
| [项目任务看板与基础时间线](development/68-project-task-board-and-timeline.md) | 三视图、实际筛选／分页、准确日期／未排期、可读前置及原状态约束；68项Web测试、4条实际浏览器链和6项布局检查通过 |
| [项目业务概览与财务关联](development/69-project-business-overview.md) | 16类当前可读记录、完整计数／最近预览、本项目精确费用分摊与独立预算、稳定入口及读取失败保护；实际API／浏览器验证 |
| [移动项目概览与业务查阅](development/70-mobile-project-overview-and-business-reading.md) | 同一16类聚合、费用／付款／订阅／变更原记录、预算分页准确定位；89项移动测试、6条实际浏览器链、18项布局及新Android开发APK通过 |
| [秘书订阅逐笔审阅](development/71-secretary-subscription-detail-review.md) | 真实订阅分页、完整日期窗口、费用／资金原记录和共同权限快照；14项新业务集成场景、36项运行时测试及 Web／Taro 实际读取验证 |
| [秘书创建里程碑与任务关联](development/72-secretary-milestone-creation-and-task-association.md) | 完整创建授权、新里程碑真实回执、任务关联／显式解除；10项新集成场景、37项运行时、77项Web、89项移动及新Android开发APK验证 |
| [秘书交付资料与关联](development/73-secretary-deliverable-updates.md) | 完整交付资料、同项目里程碑关联／解除、原业务真实回执及人工修订保留；Web／Taro入口和Android开发APK |
| [秘书创建交付草稿](development/74-secretary-deliverable-creation.md) | 完整创建授权、真实新ID／版本、原人工提交验收；同步修正里程碑标题Unicode空白，Web／Taro和新Android开发APK |
| [移动项目立项与成员管理](development/75-mobile-project-setup-and-members.md) | 新建立项、准确模板、资料编辑、角色／移除及负责人移交；99项移动测试、7条真实流程和新Android开发APK |
| [移动里程碑、风险与问题](development/76-mobile-project-milestones-risks-and-issues.md) | 三类事项新建／编辑、明确状态、风险转实际问题及冲突恢复；114项移动测试、8条真实流程及新Android开发APK |

增量83历史功能见[移动知识与项目博客](development/83-mobile-knowledge-workflows.md)：完整手工编辑／发布／恢复、准确附件和评论历史，修复隐藏评论附件保留。新155顶层／986结果后端、Web81、运行时39、移动178项、两小程序编译、12组实际流程、9项布局、新Android开发APK及17项包检查通过；25项JVM结果按Gradle缓存准确记录。[25项来源核验](quality/evidence/mobile-knowledge/final-source-check.json)通过，新增3张截图、2.8秒MP4及17帧GIF。详见[准确验证](quality/evidence/mobile-knowledge/verification.md)及[28项差异](delivery/03-goal-gap-assessment.md)。

[增量84：秘书风险与问题](development/84-secretary-project-concerns.md)提供四个完整工具、准确字段／版本、原状态机、风险转问题和真实回执；156顶层／1,001结果、Web88、运行时43、移动183、两小程序编译、新Android包与17项检查，以及两端实际流程通过。25项JVM结果按Gradle缓存记录，真实平台／云／模型验收另行完成。见[验证](quality/evidence/project-concerns/verification.md)。
