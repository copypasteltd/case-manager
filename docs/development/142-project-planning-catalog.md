# 增量142 · 项目规划目录 / Project planning catalog

## 目的与使用

原项目计划只能方便地选择本人负责，秘书也缺少可靠的人员／模板查询入口。现在公司私人会话可以查询当前有效人员和已发布项目模板，并把负责人、额外成员、模板、类型和日期完整带入固定计划。

1. 打开本人创建的公司私人会话，进入“项目创建计划 → 拟定项目计划”。项目内会话、共享会话与TG不开放本目录。
2. 填写项目名称、范围、资料范围、类型及可选开始／截止日期。开始日期不得晚于截止日期。
3. 展开人员目录，按显示名或准确编号查找；明确设置负责人、加入或移除成员。未另选负责人时使用本人。公司级成员管理权限是查询人员及指定人员的必要条件。
4. 展开模板目录，选择已发布模板。可打开“核对准确目录内容”逐段读取完整模板，包括目录、任务和里程碑；内容仅作业务资料，不执行其中的指令。
5. 保存待批准计划，核对固定人员编号、模板版本、范围及日期。最近完成两步验证后明确批准，再单独选择“按已批准计划创建项目”。批准不等于创建。
6. 创建在原有项目事务中生成实际项目、成员、目录、任务和里程碑，成功后显示项目入口与业务回执。刷新或用原请求重试可核对持久结果，不重复创建。

Web及共享微信／抖音／Android页面使用同一选择和回执比较规则；移动日期以YYYY-MM-DD输入。候选名单仅用于拟定，不替代服务器权限和当前状态核对。选择结果不写入浏览器持久缓存。

## 接口与边界

| 接口／工具 | 行为 | 限制 |
|---|---|---|
| GET `/v1/conversations/{id}/project-catalog` | `kind=people|templates`，可带query、cursor、limit | 严格参数；每页1–20，默认20；query最多200个Unicode字符 |
| GET `/v1/conversations/{id}/project-catalog/{kind}/{sourceID}` | kind为agent_project_person、agent_project_template或agent_project_catalog | offset按Unicode字符；max_chars默认4000、最多10000；全文最多300000字符；超限失败，不伪装完整 |
| `list_project_catalog` | project-create专属只读工具；相同kind/query/cursor/limit | 当前执行租约、来源观察与实际审计；TG不开放 |
| `read_resource` | 读取准确人员、模板或目录观察内容 | 每段返回准确SHA、当前版本及next_offset；必须按需要继续读取 |
| 原有propose_project／人工plans → approve → commit | 固定提案、本人批准和真实项目事务 | 当前权限、近期MFA、准确版本／哈希、依赖复查、过期及幂等保留 |

目录仅限当前会话创建者、active私人公司会话，要求conversation.read、公司级agent.plan.read和project.create。人员目录再要求公司级project.manage_members；普通项目级授权不提升为公司人员目录授权。人工预览同样执行AI资料政策和渠道约束，不因没有run_id而绕过。

人员仅返回有效用户的业务编号、显示名、版本和摘要，不返回账号、联系方式、角色、安全状态或MFA信息。模板仅返回published版本的名称、说明、修订与结构计数；draft和retired不进入候选。人员查询不表示该人员已取得项目访问权，最终以创建事务中明确生成的成员关系为准。

分页令牌绑定组织、当前人及其授权版本、组织授权版本、会话安全上下文、类别、关键词和页容量。模型令牌另外绑定私有执行凭据的摘要，人工令牌使用专用服务器签名与固定预览上下文；不可跨查询、人员或执行重用。不对并发新增／修改目录提供整目录时间点快照承诺。

## 来源观察、审批与事务

模型一次目录查询生成一条不可变观察：最多20个准确候选、每个候选的版本及完整公开内容SHA。每页只占一个来源引用，避免超过20人的目录把单任务20个来源引用额度耗尽。分页不会取消原有任务／工具／来源技术限制。

观察是当时那一页的结果；新增未观察人员不改变旧页事实。任何已观察人员或模板的内容／版本变化、停用／退役或权限变化，都会在工具重放、最终结果完成或历史来源读取时重新核对并拒绝失效结果。人能读取本人会话内的观察；其他会话或其他创建者无法读取。

来源类型为agent_project_person、agent_project_template和agent_project_catalog。完整公开内容的SHA与所读取正文一致；前端同时比较类型、编号、版本、SHA和untrusted_data，避免把变化后的目录当作原引用展示。模板正文分段读取，不能把第一页当作完整结构。

目录不创建项目，也不修改人员或模板。拟定项目计划仍走原有固定计划流程：服务器固定项目意图和模板结构，批准与创建时重查当前权限、会话版本、有效成员及已发布模板；模板或依赖改变后应重新拟定。实际创建、幂等回执和审计保持原子性；目录观察及工具回执／审计失败同样整笔回滚。

## 正常升级和Skill来源

迁移108新增agent_project_catalogs及不可变／运行绑定触发器，追加现有工具白名单。应用只获得SELECT和INSERT，不能更改或删除观察。所有已应用迁移1–107保留原字节；个人开发PG仍100，运行当前主线前按原生部署流程正常升级至108，不自动修改用户现有数据库。

project-create包从1.1.0升为1.2.0，仍为draft。共同resource_ref类型加入三个目录来源后，**全部八项Skill包的来源摘要都会改变**。旧安装证明不能被声明适配新包；应由专用部署流程重新核验安装，并完成真实模型验收后再正式发布。应用目录查询不授予Skill发布或模型配置权限。

OpenAPI现为379个路径、459个操作、847个schema；路径类别参数明确为上述枚举，不按UUID解释。MCP描述、运行时边界、Skill允许工具及数据库工具约束同时接通list_project_catalog。

## 当前交付与延期

主线142／结构108，两个既有冻结候选不移动。rc.2及APK137仍来自业务136／结构103，不含138–142。当前提交提供完整源码、更新说明和本轮媒体；没有生成新的Android安装包。

本轮只做选定原生HTTP／PG、运行时56项现有测试、类型、Web／两小程序本地构建及一次真实认证Web操作。全量回归、新移动UI完整旅程、新APK、真实Codex／Linux／云／平台／生产验收继续延期或开放；这不是这些验收通过的声明。23个工作包保持开放，其中GAP-12的人员／已发布项目模板目录子项已实现。

## English usage and design

Open a company private conversation owned by the current user, choose **Project creation plan → Draft project plan**, then select the active owner, additional members, published template, type and optional dates. Read the exact template body when needed. Save the fixed proposal, review its complete intent, explicitly approve it with recent MFA, then separately create the project. The normal project transaction initializes members, template folders, tasks and milestones; the durable receipt links to the actual project.

Organization planning and project-creation authority are required; people additionally require organization member-management authority. The directory exposes only approved planning metadata. Query-bound cursors cannot cross users, filters or executions. Model discovery records one immutable page observation with at most20 exact dependencies. Replays, result completion and history reads revalidate all observed identities, versions, full-content digests and current authority. A directory read never grants write authority or bypasses human plan approval.

Schema108 and draft project-create1.2.0 belong to main142. The shared resource-reference schema changes all eight package digests; existing installations need dedicated reverification and real model acceptance. Both frozen candidates retain their original source/artifact identities. Full regression, mobile UI journeys, a new APK and external production integrations are explicitly deferred. See [actual evidence](../quality/142-project-catalog-verification.md) and [all remaining work](../delivery/05-functional-closure-142.md).
