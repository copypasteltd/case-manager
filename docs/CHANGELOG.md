# 更新记录 / Changelog

## 增量98 · 依赖维护 / Dependency maintenance · 2026-10-28

- 移除原生主机构建未使用的Taro RN runner；Doctor glob10固定10.5.0，旧代码生成器真实加载micromatch4.0.8，并移除旧解析器分支。
- 当前完整审计91→80项、high20→13；全部剩余记录开放，不忽略依赖范围或告警。新增真实CLI参数与代码变换排除规则检查。
- Removes an unused native runner and patches actual glob/matcher consumers. Fourteen dependency and two native JS checks pass; affected clients rebuild with the current lock. Framework versions and native boundaries remain.
- 中英指南、台账及GAP-26同步。Go与UI原证据、97PNG／35MP4／35GIF按原字节复用；Android／JVM实际范围见[验证记录](quality/98-dependency-maintenance-verification.md)。完整四端Goal及真实环境验收仍开放。

## 增量97 · 小程序身份 / Mini-program identity · 2026-10-28

- 新增微信／抖音服务端交换、单次code记录、本人密码与适用MFA、两分钟会话绑定证明及明确确认；平台身份只查找已有员工。
- 新增Web与共享Taro本人绑定管理、历史和解绑确认；解绑覆盖MFA设置／验证／恢复、刷新轮换及并发，新增验证器也阻止旧平台会话续期。
- Added fixed-endpoint provider adapters, explicit employee binding, platform login and provenance across MFA/refresh. Revocation invalidates all derived sessions and pending challenges without rewriting binding history.
- 迁移78保留原77；新执行、缓存、实际截图／MP4／GIF与外部验收范围见[验证记录](quality/97-mini-program-identity-verification.md)。完整四端Goal及真实平台、设备、云、模型、TG和生产验收继续开放。

同时隔离既有状态桥 DNS 拒绝测试的客户端与解析结果，并使用原子命中计数；修正前20次重复出现3次失败，修正后20次新执行全部通过，运行时传输实现保留原发布字节。浏览器故障注入改为等待实际响应拦截完成，原失败记录保留。

Also makes the existing DNS denial fixture deterministic with an independent client/resolver and atomic handler counts: the original20-run probe failed3 times; the corrected20-run probe passes. Runtime transport retains its published bytes. Browser fault injection awaits actual interception completion, with prior failures retained.

## 增量96 · 原生文件迁移 / Native file migration · 2026-10-28

- 新增六个原生CLI命令、逐项来源映射、固定计划与近期MFA批准；复用普通权限、配额、扫描、正式提交及双端复制流程。
- 原上传对账与明确失败重试保持稳定来源／逻辑文件；源改变需要准确新计划及当前基础版本；旧提交重放同样检查迁移权限。
- Added a native inventory/plan/import/report operator, immutable manifests, explicit retries and current provenance checks. Per-row OSS/R2 evidence remains separate from restore and cutover approval.
- 新检查、开发环境与历史页面媒体复用范围见[完整验证](quality/96-native-file-import-verification.md)；真实2TB、120GB、云、设备、模型、TG与生产验收仍分列。

## 2026-10-28 · Increment95 / 任务讨论、固定附件与明确提及

Web和共享Taro新增真实任务讨论：新建／本人修订、准确正式版本附件、明确成员选择、不可变历史及站内提及通知。省略列表保留完整原关联，完整替换需要再次核对；无权旧关联不会被过滤后静默删除。通知沿真实outbox／租约／幂等管线，文字修改不重复投递，移除后再提及不复活旧通知；打开准确当前修订并自动滚动，超过首20条也可定位。冲突、已提交但丢失／缺失回执保留文字、阻止重复，并允许人工核对后同意图幂等对账。

Added actual task discussion and explicit eligible-member mentions to Web/Taro, immutable revisions, fixed-version attachments and exact inbox routing. Delivery and reads recheck live authority; reading never completes a task. No external sender is added. Migration76 adds immutable mention metadata; the prior75 files, dependencies and eight draft Skills remain unchanged. OpenAPI:286 paths/355 operations/625 schemas.

新执行原生后端171顶层／1133结果、9批；Web135、运行时51、移动238、类型／构建及双小程序各20页；Linux交叉编译但未执行／部署。当前Android开发APK、18项包检查，26项JVM明确为Gradle UP-TO-DATE缓存。实际Web／H5各8组流程与8PNG／两套原速MP4和GIF；媒体总90／33／33。原94入口／应用／后端证据、9次脚本或被替代采集及完整来源保留，失败不改写为通过。本机迁移升级76并核对归属API／通知及报表worker、零隔离库；云／模型／TG关闭。

Fresh local checks and source-bound artifacts are separate from JVM cache reuse, historical evidence and external acceptance. The unchanged lock retains the original90 audit (86 findings,20 high); no new scan is claimed. Four-client release, real devices/platforms/cloud/model/TG,120GB/2TB, network, recovery and production security remain open.

[方案 / Workflow](development/95-task-discussion-and-mentions.md) · [验证 / Verification](quality/95-task-discussion-verification.md) · [媒体 / Media](media/README.md) · [编号差异 / Gaps](delivery/03-goal-gap-assessment.md)

## 2026-10-28 · Increment94 / 移动文件与目录整理

共享Taro接入真实文件工作区：按目录层级与名称筛选，目录／文件独立分页；新建目录、同项目／分类改名移动、文件回收与原目录恢复；分页核对正式版本和准确历史阅读。源修订、目标、原因和影响先预览确认，完整回执才显示成功；文字草稿只驻内存，冲突和不确定结果停止重复提交。导航替换当前目录页面以控制原生页面栈；保留Android原生上传与原固定版本读者。

Added actual shared mobile folder/file commands, version history and explicit confirmation. Exact receipts preserve content identity and scope. Text-only drafts survive conflicts and uncertain results; destination selection and confirmation are never restored. Existing backend business rules remain authoritative. No API, migration, dependency or Skill changes.

新执行移动233项、类型、微信／抖音各20页；当前Android开发APK及18项检查，26项JVM按Gradle实际缓存范围保留。真实本机API／隔离PG的9组流程、8项H5布局及4PNG／两套MP4与GIF；当前媒体82／31／31。三次脚本诊断保留失败状态，另一次全通过但过快的录制保留原通过状态与媒体后重新采集；完整来源均归档。后端167顶层／1129结果、Web130、运行时51及Linux按完整来源未变复用93，原90审计86项／20high未重新执行。原93的89份证据／媒体及10份入口快照保持原字节。

Fresh mobile/build/browser evidence is separate from unchanged-source reuse and Gradle caches. Recordings use authenticated synthetic H5 pixels. Actual platforms/devices/cloud/model/TG,120GB/2TB, retention cleanup, cross-scope moves, security and production recovery remain open; the full four-client goal is active.

[方案 / Workflow](development/94-mobile-file-workspace.md) · [验证 / Verification](quality/evidence/mobile-files/verification.md) · [媒体 / Media](media/README.md) · [编号差异 / Gaps](delivery/03-goal-gap-assessment.md)

## 2026-10-28 · Increment93 / 秘书正文检索与固定DOCX资料

原 `search_resources` 接入项目／任务／知识实际正文及当前正式文件的就绪索引，按会话共同权限、分类和准确文件依赖筛选；字面大小写匹配、Unicode片段和绑定查询／运行／授权的游标沿用有界候选保护。`read_resource` 与Web／Taro预览支持固定DOCX派生正文，绑定原件／正文SHA-256、提取器、索引时刻与字符位置；重新核对撤权、回收、扫描、租约、取消、预算及持久回执，保留原UTF-8独立读取路径。八项Skill协议递增仍draft，不增加写授权。

Existing secretary MCP tools now discover permitted body matches and read immutable fixed-version DOCX text. Current authority and original/derived evidence remain bound through preview, Unicode continuation, durable receipts and result withdrawal. The original raw-text reader remains in place. All eight updated Skill packages remain draft; discovery and snippets do not grant mutations.

无新迁移，原75项与已发布92原字节一致；OpenAPI285路径／353操作／622 Schema。当前后端167顶层／1,129结果／1,027末级、9批，Web130、运行时51、移动222及类型、双小程序各19页；Linux只编译。原生开发APK通过18项包检查，26项JVM明确为Gradle缓存；Web／H5各8项实际API／页面流程，新增4PNG及两套MP4／GIF，全目录78／29／29。已发布92完整来源重建，91份证据／媒体、10份入口快照保留原字节。

Fresh results bind unchanged current source bytes. The native fixture's earlier UUID cast failure, three browser observation failures and one limited development-inspection failure retain original status and exact available sources/reports; no failed report was reclassified. The unchanged lock reuses the original90 audit of86 findings, including20 high. Actual cloud/model/TG/device,120GB/2TB, recovery, security and the complete four-client production launch remain open.

[协议 / Protocol](development/93-secretary-indexed-search.md) · [准确验证 / Verification](quality/evidence/agent-indexed-search/verification.md) · [素材 / Media](media/README.md) · [28项差异 / Gaps](delivery/03-goal-gap-assessment.md)

## 2026-10-28 · Increment92 / 统一搜索与固定版本正文

实现统一搜索项目、任务、目录、可见知识及文件；权限过滤先于分页，游标绑定本人／组织／授权版本及查询条件。当前正式文件版本可按独立只读凭据提取UTF-8文本／Markdown／CSV／JSON与DOCX主文档，复核冻结的云版本、ETag、字节数及SHA-256；IO期间不持有业务锁，发布前重检版本、扫描、权限范围和队列租约。派生正文入PostgreSQL并按固定版本分块预览；Web与Taro新增入口。PDF、OCR、复杂Office及超限文件只支持本轮已实现的元数据检索，不虚报正文支持。

Implemented unified permission-filtered search, query-bound keyset cursors, independently credentialed native extraction and immutable version-derived text. Cloud reads release database locks and publication rechecks source identity, clean scan state and the fenced lease. Web and Taro expose exact-version, bounded plaintext previews with explicit unsupported/failed states. The restricted DOCX parser never executes document content.

新增迁移75并保留原74项原字节；后端全量165顶层／1,112结果／1,012末级、9批，Web126、运行时49、移动218及类型、微信／抖音各19页编译。Linux只编译；当前Android开发包、18项包检查及26项JVM缓存范围单独记录。真实原生API＋隔离合成资料的Web／H5各12项检查；新增4PNG、两套MP4／GIF，目录74／27／27。原91完整来源重建、164份已发布证据／媒体及10份原入口文档快照保留。

Fresh native checks and actual authenticated browser pixels are source-bound. Earlier failed and superseded attempts retain their original status; fixture-start and concurrent-edit diagnostics do not claim complete old-source reconstruction. Original91 evidence stays byte-identical. The unchanged lock reuses the original90 audit with86 findings, including20 high; no fresh audit or security release is claimed. Real cloud/device/model/TG,120GB/2TB, restore, platform distribution and four-client production acceptance remain open.

[设计 / Design](development/92-file-text-search.md) · [运行 / Operations](operations/file-text-search.md) · [准确验证 / Verification](quality/evidence/file-search/verification.md) · [演示 / Media](media/README.md) · [28项差异 / Gaps](delivery/03-goal-gap-assessment.md)

## 2026-10-27 · Increment91 / 秘书任务前置依赖

实现 `update_task_dependency`：员工在Web／Taro选择同项目两个实际任务，固定两个版本、明确新增／解除和原因，预览后发送。服务端复用原Dependency命令及项目锁、权限、循环／状态规则；数据库独立核对原关系、真实增删关系、未越权任务资料和实际回执。准确重试、失联回执恢复和取消保持一个业务操作。修正输入改正后的旧错误提示，以及会话轮询重建选项数组导致移动滚轮重置的问题。

Implemented exact secretary prerequisite changes with both actual task versions and an explicit add/remove decision. Shared business commands preserve cycle/state/permission rules and atomic audit/outbox/runtime evidence. Database guards verify the actual edge and unchanged task fields. Exact retries cannot duplicate a relationship or version; corrected inputs clear stale validation feedback, and stable option labels keep live polling from resetting an open mobile wheel.

新增迁移74，保留原73项迁移字节；Skill `project-update`1.7.0含15写／2读工具，八项Skill均draft。全量后端162顶层／1,080结果／982末级、9批，Web120、运行时49、移动212及类型、双小程序各18页编译；Linux仅编译，Android当前开发包与18项包检查另验，26项JVM如实记录Gradle缓存复用。两端各5组实际流程／3宽度，新增8截图和两套MP4／GIF，总目录70PNG／25MP4／25GIF。

Migration74 is additive; the earlier73 remain byte-identical. Fresh source-bound checks cover the native backend, Web, runtime, mobile and both mini-program builds. Current native APK inspection and cached JVM results have separate scope. Actual authenticated Web/H5 demonstrations use isolated synthetic records: UI sends are queued then cancelled; committed add/remove views come from original business commands through controlled MCP, without a live model.

已发布增量90的完整来源按e45c347重建，84份原证据／媒体保持原字节。失败与明确替代的通过尝试保留原状态、完整准确来源及清理结果；首轮临时原生用例修正未冻结各次完整旧来源，仅作为过程限制披露。相同锁文件沿用原90完整审计86项（20 high），本轮未重审计，不宣称安全放行。实际模型、设备、TG、云、120GB／2TB、恢复及四端生产首发继续开放。

Published90 sources reconstruct from Git; all84 original evidence/media files retain exact bytes. Failed and superseded attempts preserve status and source archives. Initial ad-hoc native fixture iterations lack complete pre-edit archives and are not presented as current passing evidence. The unchanged lock reuses its original90 audit; real integrations, devices, capacity and production acceptance remain open.

[设计／操作](development/91-secretary-task-dependencies.md) · [准确验证 / Verification](quality/evidence/task-dependencies/verification.md) · [演示 / Media](media/README.md) · [28项差异 / Gaps](delivery/03-goal-gap-assessment.md)

## 2026-10-27 · Increment90 / 依赖修复与多端兼容

以维护中的解包器11.1.4替换原无维护实现，提供实际CommonJS／Promise兼容入口；升级Swiper12.1.2、tar7.5.22、兼容webpack5.104.1及明确依赖版本，核对安装后真实依赖树。完整同次审计101→86，critical11→0；仍有high20／moderate64／low2，安全放行继续开放。历史114→101属于公告库变化，不算修复。

12项有界安全回归、2项原生JS、Web112、运行时47、移动204／类型／双小程序重新检查；真实页面与Android产物分别验证。后端161／1057及Linux只按完整相同来源复用原88证据；73迁移与接口未改。页面检查遵守现有MFA限流并等待实际窗口，不修改应用限制。新增4PNG／2MP4／2GIF，目录62／23／23，双语说明和28项差异同步维护。

The actual dependency tree now uses a maintained archive extractor behind a callable CommonJS adapter, plus explicit patched and compatible pins. A complete same-run audit drops from101 to86 findings and11 to0 critical;20 high findings remain. Bounded regressions, affected client/runtime builds and actual synthetic journeys are checked separately from device/cloud/production acceptance. Original89 capture sources and103 historical evidence/media files are preserved. Failed attempts retain their actual limits, including the first attempt’s missing original lock bytes. Full four-client acceptance remains open.

[修复方案 / Design](development/90-dependency-hardening.md) · [准确验证 / Verification](quality/evidence/dependency-hardening/verification.md) · [媒体 / Media](media/README.md)

## 2026-10-27 · Increment89 / 移动费用、资金、退款与员工结算

实现Taro费用／资金起草和修订、固定凭证与往来方、精确原币／香港时间／分摊、MFA确认与登记、批准业务依据、费用分配和释放、退款登记及后补分配、准确员工报销行和全额冲销；固定修订及不可变历史可分页核对。保留输入、版本／摘要与单次意图，真实响应丢失不重复提交。修正费用入口为后端实际cost.record权限。

204项移动测试、类型及双小程序各18页、9组真实页面流程和3种宽度通过；Android开发包42,921,174字节、18包检查，26项JVM本轮缓存复用。后端161／1057、Web112、运行时47和Linux编译只在完整声明来源相同后复用原88证据；全部73迁移、接口、依赖锁文件未改。新增4PNG／2MP4／2GIF，目录58／21／21。保留全部失败来源及被替代构建，按e87656f重建88完整捕获与80份旧证据，清理所有自建临时浏览器／API／库／服务。完整四端及真实设备／云／模型／TG／容量／安全／恢复／生产目标仍开放。

Shared Taro clients now implement expense/cash drafts, exact approved source links, payment allocations/releases, refunds, employee settlement and dated reversals with MFA, source-bound receipts and blocked uncertain writes. Current mobile/native builds and synthetic UI journeys pass. Unchanged backend/Web/runtime evidence is reused explicitly; cached JVM results are not called new executions. Bilingual guides, actual media and retained evidence are updated together.

[操作与设计 / Workflow](development/89-mobile-financial-operations.md) · [准确验证 / Verification](quality/evidence/mobile-finance-operations/verification.md) · [演示 / Media](media/README.md)

## 2026-10-27 · Increment88 / 财务筛选、规范来源与限时导出

发生、现金、订阅预测现支持完整业务筛选、原币独立合计、可读来源候选及规范明细签名分页。独立report.export授权允许本人排队、查看、取消和24小时内下载；实际worker固定快照，实时授权／围栏／不可变数据库约束保护生成与交付，过期清理派生内容。Web与Taro共用完整分段及SHA校验，原文件继续OSS／R2，CSV不长期落盘；Android／微信／抖音按实际能力交付或明确回退Web。

Adds complete business filters and canonical source pagination across incurred, cash and subscription forecasts. A separate report.export permission governs owner-only queued snapshots, cancellation and expiring retrieval. A native DB-only worker freezes complete authorized sources; fencing, immutable guards, expiry and fresh authority protect delivery. Clients verify the full CSV before saving or handing it off. Original documents retain the existing OSS/R2 path.

161顶层／1,057结果／960末级后端9批、Web112、运行时47、移动187、双小程序各18页通过；Android42,905,815字节开发包、18包检查、26项新执行JVM通过；Linux只编译。新增6截图／2MP4／2GIF，目录54／19／19；两端均实际校验27行CSV。新增73迁移，原72项字节不变。保留完整来源的失败及被替代捕获、修复旧秘书查询兼容问题后的全量回归；增量87按已发布73d994b重建，74份旧证据原字节不变。完整四端、真机／云／模型／TG、容量、安全和生产放行继续开放。

Source-bound local checks and actual media pass with the26 JVM tests newly executed. Failed and superseded attempts retain exact sources. Full regression caught and fixed an existing secretary-query compatibility issue before all nine batches passed. Published increment87 provenance and74 prior evidence files remain unchanged. No device, real integration or production acceptance is claimed.

[完整使用与设计 / Workflow](development/88-financial-report-filters-and-exports.md) · [验证 / Evidence](quality/evidence/finance-reports/verification.md) · [截图与录屏 / Media](media/README.md)

## 2026-10-27 · Increment87 / 项目预算与实际支出对比

新增完整当前预算与实际费用对比API及Web／Taro入口，按明确原币、类别、包含起止日的期间和本项目规范分摊计算；原费用与冲销各用自身发生日期，草稿单列。预算内／外来源支持状态筛选、签名分页与原记录跳转；严格检查逐笔读取权限和当前计划依据，不将可见部分冒充全项目余额。无预算与批准清空的零预算分开显示，无新增迁移。

Adds current implemented project-budget comparison against canonical allocation shares and dated reversals, with separate drafts and out-of-plan activity. Both clients expose complete plan lines, filtered signed pagination and original expense navigation. Exact currency, live record authority and stale-basis checks remain explicit; authorized totals do not establish whole-project balances. No migration is added.

159顶层／1,041结果／945末级后端、Web104、运行时47、移动187及双小程序各17页通过；Android开发包18项检查通过，25项JVM本轮使用缓存；Linux仅编译。新增4图／2短MP4／2GIF，目录48／17／17。保留捕获检查器失败和既有隔离库清理超时；相同来源恢复失败批后全部通过。增量86完整来源按已发布94081c0重建，99份旧证据原字节保留。完整28项GAP、真实设备／云／模型／TG、容量、安全、恢复与生产放行继续开放。

Current source-bound checks and actual media pass with cached JVM scope recorded honestly. Retained attempts document capture-checker assumptions and an isolated-database cleanup timeout, followed by same-source batch recovery. Published increment86 provenance reconstructs without rewriting99 old evidence files. The full production goal remains open.

[计算与使用 / Workflow](development/87-project-budget-comparison.md) · [验证 / Evidence](quality/evidence/budget-comparison/verification.md) · [截图与录屏 / Media](media/README.md)


## 2026-10-27 · Increment86 / 秘书Token额度与用量

新增Go／PostgreSQL月度额度账本：组织、员工、项目、Skill共用原子接单预留；未知用量保守保留，已知失败、取消及越限用量不被抹除。管理员通过Web／Taro发布完整策略、按范围查账，并凭据核对未知终态用量。迁移72不预设生产额度；SDK用量先记账再校验回复，不将Token数量视为金额账单。

Adds monthly token accounts across organization, user, project and Skill scopes, atomic admission reservations, observed failure/cancellation accounting and evidenced human reconciliation of unknown terminal usage. Web and Taro expose the same policy and reporting workflows. Migration72 supplies no production allowance. Observed usage is metered before reply validation; provider monetary billing and the full production goal remain open.

[规则 / Workflow](development/86-secretary-token-budgets.md) · [检查 / Evidence](quality/evidence/token-budgets/verification.md) · [截图与录屏 / Media](media/README.md)


本文件记录可交付改动、验证范围和仍未放行的事项。完整逐轮历史见[实施台账](delivery/02-implementation-ledger.md)，所有未完成项保留[稳定 GAP 编号](delivery/03-goal-gap-assessment.md)。

This log records deliverable changes, verification scope and open release gates. See the [implementation ledger](delivery/02-implementation-ledger.md) for increment history and the [stable gap register](delivery/03-goal-gap-assessment.md) for unfinished work.

## 2026-10-27 · 秘书准确创建任务与子任务 / Exact secretary task and subtask creation · increment85

- 新增 `create_task`：完整九项资料及原因，显式负责人／父任务／里程碑／日期、优先级与人工验收要求；复用原 CreateTask 权限及事务，真实新 ID／todo／版本1回执，两种准确重试不重复创建。
- Web／Taro完整表单、分页关联与保留输入、缺少验收选择及微秒日期倒序校验、准确预览／发送／取消同步；新Android开发包42,888,501字节、17项检查，25项JVM结果明确来自Gradle缓存。
- 迁移71增量接入，前70份迁移不变；157顶层／1,014结果／920末级、Web95、运行时45、移动187、两小程序各16页、两端各7流程／3布局。失败尝试保留，包括原生临时库删除超时及未冻结编译缓存导致的历史复现限制。
- 新增4 PNG、2 MP4、2 GIF，媒体目录40／13／13；完整中英文说明、GAP编号、来源检查同步。八项Skill仍draft，真实模型／云／TG、设备／容量／安全／恢复／生产仍待验收。

Added complete `create_task` authorization, original business authority/atomic receipts and exact replay; shared Web/Taro forms preserve all nine decisions and explicit review. Additive migration71, current native regression/build checks, actual seven-flow captures per client, four screenshots and two MP4/GIF pairs are included. Cache and historical provenance limits are explicit; all Skills remain draft and real release gates remain open.

[方案 / Workflow](development/85-secretary-task-creation.md) · [验证 / Verification](quality/evidence/task-creation/verification.md) · [演示 / Media](media/README.md)

## 2026-10-27 · 秘书风险与问题 / Secretary risks and issues · increment84

- 增加四个准确工具，完整核对字段、父项目／事项版本和真实回执；项目成员、阶段、共同受众及原登录继续约束每一步。风险转问题要求双权限并原子提交，失败回滚，准确重试不重复创建。
- Web／Taro增加完整表单、清空语义、状态选择和风险发生确认；普通PATCH可显式清空截止时刻／概率。Skill1.5.0仍为draft，四个工具不进入其他技能。
- Fresh backend156 top-level/1,001 results/908 leaf checks, runtime43, Web88, mobile183, both 16-page mini-program builds and eight actual flows/three layouts per browser client passed. The 42,885,880-byte Android debug APK passed 17 inspections;25 JVM results retain their actual Gradle cache marker.
- 新增4张截图、两对实际MP4／GIF；原生开发API更新至70项迁移，10项只读探针通过，外部集成未启用。增量83的805份采集原Git来源保留，失败尝试不覆盖。

[方案与使用 / Workflow](development/84-secretary-project-concerns.md) · [准确验证 / Evidence](quality/evidence/project-concerns/verification.md) · [实际演示 / Media](media/README.md)。真实设备、云、模型、TG、安全、120GB／2TB及生产验收继续开放。Real devices/cloud/model/TG, security, capacity and production acceptance remain open.

## 2026-10-27 · 移动知识与项目博客 / Mobile knowledge and project blog · increment83

- 补齐四类公司／项目知识的手工创建、编辑、安全Markdown、准确附件、正文修订、独立发布／归档／退回草稿、历史恢复和修订讨论。Added complete shared mobile knowledge workflows with explicit attachment modes and immutable history.
- 文字评论修改默认保留服务器原附件；明确替换先核对全部旧／新附件，新回执和幂等重放按当前权限过滤。Text-only edits preserve original pins; explicit replacement rechecks every original/replacement and receipts reapply current visibility.
- 新增按固定版本ID解析元数据，复用当前文件ACL，不替换历史版本、不暴露存储坐标。Added exact version-ID resolution through current file authorization.
- 新完整后端155顶层／986结果／894末级、Web81、运行时39、移动178项、双小程序16页编译、12组真实流程和9项布局通过。Fresh source-bound backend, Web, runtime, mobile and actual UI checks passed.
- 新42,881,123字节Android开发APK及17项检查通过；25项JVM结果本次为Gradle缓存，四份XML及原日志保留。Fresh debug APK inspected; cached JVM scope is explicit.
- 新增3张截图、2.8秒MP4／17帧GIF；演示素材目录合计32／9／9。25项最终复算、805份当前采集来源及793份旧TG原Git来源可追溯。Added actual safe media, current capture provenance and exact historical reconstruction.
- 保留类型、页面定位和数据库清理失败；同源重跑后完整通过，两个确认为本轮残留且无连接的测试库已清理；69项迁移、10项只读探针和集成关闭状态核对通过。Retained failures, exact-source reruns and scoped fixture cleanup; daily native services were inspected without enabling integrations.
- 同步双语README、接口契约、操作／开发约束、差异表和媒体目录；发布检查按当前基线选择新的运行时证据组。Updated bilingual guides, contract, constraints, gap register and publication checks together. Real devices/cloud/model/TG, large-file capacity and production acceptance remain open.

详见[方案与使用](development/83-mobile-knowledge-workflows.md)、[准确验证](quality/evidence/mobile-knowledge/verification.md)和[实际演示](media/README.md)。See the linked workflow, evidence and actual media.
- 提交前恢复四份Go文件原LF，重新完整验证受影响的后端、Linux和页面；修正检查器等待页面归档状态的时序。旧完整证据与5份媒体准确保留，[来源沿革](quality/evidence/mobile-knowledge/source-normalization.json)可重构原始摘要。Restored original LF conventions, reran affected checks and fixed a browser assertion race; exact earlier proofs/media remain retained.

## 2026-10-27 · 移动TG身份与准确入口 / Mobile TG identity and exact channels · increment82

- 补齐移动本人私聊证明、原登录数字身份确认、取消和撤销；保留原请求及投递回执。Added own-identity challenge/confirmation/cancellation/revocation with existing requests and delivery receipts.
- 管理员完整配置Bot/群/话题、可见项目范围、允许/默认任务与阶段通知，新增始终暂停，完整回执及不可变历史双重核对。Added complete scoped configuration, paused creation and receipt/history corroboration.
- 新增有界Android前台敏感剪贴板复制；不读剪贴板、不持久化私聊能力。Added bounded foreground sensitive proof copying without clipboard reads or persistent capability storage.
- 新167项移动测试/双小程序编译、9组实际流程/9项布局、新Android开发APK/17项检查及新执行25项JVM测试通过；23项最终复算通过。Fresh client/build/browser/JVM checks passed with exact-source binding.
- 新增3张截图、4.9秒MP4/29帧GIF；媒体总计29/8/8。同步双语指南、开发约束、失败历史和787份旧状态捕获Git来源。Added actual safe media, bilingual workflow docs and exact historical capture reconstruction.
- 后端154顶层/984结果、Web81、运行时39及Linux产物按完整未变来源复用；未启动真实TG/云/模型、真机或生产部署。Unchanged proofs are retained explicitly; external/device/production gates remain open.

[方案 / Protocol](development/82-mobile-telegram-access.md) · [准确验证 / Verification](quality/evidence/mobile-telegram/verification.md) · [演示 / Demo](media/videos/mobile-telegram-access.mp4)

## 2026-10-27 · 原生Prometheus状态桥接 / Native Prometheus status bridge · increment81

- 新增独立Go桥接，读取成熟Prometheus／Blackbox的完整标签与原采样时间；复用原签名观测、投影、权限、历史和站内通知。Added a separate native bridge from mature monitoring to the original signed-ingress/status/history path.
- 增加私密配置、端点／IP许可、禁代理／重定向、独立读令牌、独占持久journal、准确重放与完整回执绑定；来源变更须新修订。Added bounded networking, independent credentials, durable exclusive journals, exact retries and bound ACKs; rebinding requires a new source revision.
- 后端新154顶层／984结果／892末级、Web81、移动154及双小程序编译通过；Windows实际桥接与Linux编译通过。Fresh backend/Web/mobile tests and builds passed; Windows bridge executed, Linux compile only.
- 六组实际Web／H5流程、12次CLI、9项布局及25项最终来源核验通过；所有所属浏览器与夹具清理。Six actual flows, 12 CLI invocations,nine layouts and 25 final source checks passed, with owned-resource cleanup.
- 新增3张截图、6秒MP4／36帧GIF、双语使用／部署协议、运维及失败历史。Added affected captures and bilingual usage, deployment, operations and honest attempt history.
- Android在当前来源下重新编译并通过17项检查，APK与80字节相同；25项原生结果为UP-TO-DATE。39项运行时按不变来源准确复用。Android build/inspection is fresh; its APK bytes are unchanged, native tests are cached and runtime proof is exact-source reuse.
- API须先升级再启用桥接；真实Prometheus／Blackbox安装、主机指标、独立告警、真机／云／模型／TG、容量／安全／恢复继续开放。Upgrade the API first; actual deployment, independent alerts and full production acceptance remain open.

[完整方案 / Protocol](development/81-native-prometheus-status-bridge.md) · [验证与失败历史 / Verification and attempts](quality/evidence/status-bridge/verification.md) · [实际演示 / Actual media](media/README.md)

## 2026-10-26 · 移动完整变更与预算 / Mobile full changes and budgets · increment80

- 完整项目／币种基线、固定文件、不可变提案、原申请修订与独立审批、两项核实／本人MFA明确实施，成功须读回实际项目和预算。Full basis, fixed files, immutable proposals, original request revision/independent approval and separate MFA implementation require actual source proof.
- 明确区分保持null、替换明细和清空[]；十进制字符串、真实日期及原币口径沿用原后端，预算不能代替费用或付款。Keep/null, replacement and clear/[] retain exact decimals and original-currency semantics; budgets do not create expenses/payments.
- 409、403或不完整回执清除不再可读的事实，保留可编辑文字并阻止重放；显式重读重置基线采用和声明。Conflicts, rejected reads and incomplete receipts protect current facts, preserve text and block replay; reread resets acceptance/attestations.
- 154项移动测试、类型、双小程序各16页、10组实际API／H5流程和9项布局通过，5个所属上下文、临时服务及隔离数据库均回收。154 tests, types, both 16-page builds, ten actual flows and nine layouts passed; all owned resources were cleaned up.
- 新开发APK 42,858,139字节、17项包检查；25项原生测试为UP-TO-DATE保留结果，本轮未新执行。New 42,858,139-byte APK passed 17 inspections; 25 cached native results were not re-executed.
- 新3张截图、7.3秒MP4、44帧GIF及真实逐帧来源，双语指南、使用逻辑和稳定GAP同步更新；22项最终来源检查通过。Adds three PNGs, a 7.3-second MP4, 44-frame GIF, provenance and synchronized bilingual guides; 22 source checks passed.
- 后端146顶层／919条结果、运行时39、Web81及Linux按未变来源准确复用；8项Skill仍draft，真实模型／云／TG／平台／设备、120GB／2TB、安全和生产恢复继续开放。Unchanged proofs are reused at exact sources; real integrations and full production acceptance remain open.

[完整方案与命令 / Protocol and commands](development/80-mobile-project-changes-and-budgets.md) · [执行及失败历史 / Verification and attempts](quality/evidence/mobile-changes/verification.md)

## 2026-10-26 · 移动结项与固定归档 / Mobile closeout and immutable archive · increment79

- 接通原项目阶段、十三类完整核查、复盘、两项声明／本人MFA、可选正式文件、实际项目及历史证明、分页与受控重开。Connects original lifecycle, complete checks, retrospective, separate attestations/MFA, optional formal files, actual project/history proof, pagination and controlled reopen.
- 并发409、响应丢失或历史证明不完整时保留文字、阻止重放；显式重读重新核对权限／依据并重置声明与MFA。Concurrent, lost or incomplete results retain input and block replay; explicit reread refreshes authority/facts and resets attestations/MFA.
- 137项移动测试、类型、双小程序各16页、9组实际API／H5流程及9项布局通过，5个所属上下文、临时服务与隔离数据库均清理。137 mobile tests, types, both 16-page builds, nine actual flows and nine layouts passed, with owned browser/server/database cleanup.
- 新开发APK 42,842,779字节、17项包检查；原生25项为UP-TO-DATE保留结果，本轮未新执行。New 42,842,779-byte APK passed 17 inspections; 25 native results were retained, not re-executed.
- 新增3张实际截图、3.6秒MP4及22帧GIF；21项最终来源核对通过，旧媒体保留历史来源。Adds three actual screenshots, a 3.6-second MP4 and 22-frame GIF; 21 source checks passed, with historical media preserved.
- 后端146顶层／919条结果、运行时39、Web81与Linux产物按未变完整来源复用；8项Skill仍draft，真实集成及完整生产验收继续开放。Unchanged backend/runtime/Web/Linux evidence is reused at exact source digests; Skills remain draft and real integrations/full release remain open.

协议与命令见[增量79](development/79-mobile-project-closeout-and-archive.md)，准确执行／失败历史见[验证记录](quality/evidence/mobile-closeout/verification.md)。[Protocol](development/79-mobile-project-closeout-and-archive.md) and [verification](quality/evidence/mobile-closeout/verification.md).

## 2026-10-26 · 移动任务、子任务与依赖 / Mobile tasks, subtasks and prerequisites · increment78

- 补齐移动任务手工创建、完整资料编辑、子任务、父／里程碑候选及明确解除；按香港时间保留六位微秒。Added manual mobile task creation/editing, subtasks, real paginated parent/milestone selection and explicit clearing, with six-digit Hong Kong schedule precision.
- 前置添加／移除独立确认原版本，实际任务与关系列表回读后才完成；并发、未知响应或证明不完整保留输入并阻止重放。Prerequisite changes use the original version and succeed only after actual task/edge reads; conflicts, unknown responses and incomplete proof retain input and block replay.
- 124项移动测试、类型检查、微信／抖音各16页构建、9组实际API／H5流程及6项布局通过，5个浏览器上下文和临时夹具均清理。124 mobile tests, types, both 16-page mini-program builds, nine actual API/H5 flows and six layouts passed; all five contexts and temporary fixtures were cleaned up.
- 新增2张实际截图、6.7秒MP4和GIF，同步中英指南、编号差异与更新记录；旧媒体保留准确历史提交来源。Added two actual screenshots and a 6.7-second MP4/GIF pair, updated both guides, gap IDs and notes, and retained exact historical provenance for older media.
- 新Android开发APK的17项检查通过；25项原生测试为Gradle保留结果。实际执行范围、未改后端／运行时／Web的来源复用见[验证记录](quality/evidence/mobile-tasks/verification.md)。The new APK passed 17 inspections; 25 native results were retained by Gradle. Full execution scope and unchanged backend/runtime/Web reuse are recorded in the [verification report](quality/evidence/mobile-tasks/verification.md). Production, real platform/device/cloud/model/TG and security gates remain open.

协议与实现见[增量78](development/78-mobile-task-editing-and-dependencies.md)。Protocol and implementation: [increment78](development/78-mobile-task-editing-and-dependencies.md).

## 2026-10-26 · 仓库初始发布 / Initial repository publication · increment77

- 整理现有 `app/` 实现与 `docs/` 协议、设计和验证历史，建立仓库双语入口、完整中英文方案／使用指南、维护与同步规则。Organized implementation and documentation with bilingual landing/full guides and maintenance/synchronization rules.
- 发布 14 张本次实际页面截图、1 张带来源的历史秘书计划裁剪、3 段实际操作 MP4 和 3 个 GIF；合成数据、无真实模型／云／TG 外发，移动为 Taro H5。Added 14 new real-page screenshots, one attributed historical plan crop, three actual-interaction MP4s and three GIFs; synthetic data, Taro H5, no real external operation.
- 新增来源绑定采集、原始帧校验编码、仓库敏感信息／链接／媒体检查及只读开发环境核对工具。Added source-bound capture, verified-frame encoding, repository/media/link/secret preflight and read-only development inspection.
- 将后续完成更新同步中英说明、受影响媒体、验证与更新记录，并连同代码提交推送的约定写入仓库规则。Recorded the standing rule to update both language guides, affected media, evidence and notes and push them with corresponding code.
- 本次不改变业务源码、依赖、迁移、Skill 状态或生产配置。来源相同的功能证据保留原执行范围；四端生产、真实模型／云／TG、120 GB／2 TB、设备、安全与恢复门槛继续开放。Business sources, dependencies, schema, Skill states and production configuration are unchanged; all production/external acceptance gates remain explicit.

媒体、检查和发布组织细节见[增量77](development/77-repository-publication-and-bilingual-readmes.md)与[媒体目录](media/README.md)。Media, checks and publication structure: [increment77](development/77-repository-publication-and-bilingual-readmes.md) and [media catalog](media/README.md).

## 2026-10-26 · 移动里程碑、风险与问题 / Mobile milestones, risks and issues · increment76

- 在既有 16 个 Taro 页面中补齐三类事项创建、编辑、状态预览、版本核对与内存草稿；风险发生返回真实问题并沿原记录继续处理。Added creation/editing/transitions, exact version checks and in-memory drafts within the existing 16 pages; realized risks link actual issues.
- 移动单测 114 项、双小程序构建、8 条实际 API/H5 流程、9 项布局、新 Android 开发 APK 的 17 项检查通过。114 mobile tests, both mini-program builds, eight actual API/H5 flows, nine layouts and 17 new-development-APK inspections passed.
- 25 项 Android 单测是 Gradle 保留的通过结果；后端 146 顶层／919 结果、运行时 39、Web 81 按未改完整来源复用增量74。The 25 Android unit results were retained by Gradle; backend/runtime/Web evidence is reused only against complete unchanged source digests.

协议与失败尝试见[增量76](development/76-mobile-project-milestones-risks-and-issues.md)和[验证记录](quality/evidence/mobile-project-resources/verification.md)。See the protocol and scoped verification, including failed attempts.

## 初始功能基线 / Included functional baseline · increments1–75

包含 Go/PostgreSQL 权限与版本化业务、Web 业务入口、Taro 公共页面及 Android 原生宿主、合同采购报销、费用与订阅、项目模板及协作／交付／变更／结项、文件版本与双云适配、知识、通知、安全、服务状态、受控秘书工具与 TG 渠道实现。增量75加入移动立项、成员与负责人管理。所有能力的精确范围、历史证据和未完成工作以对应协议与差异表为准。

Includes versioned Go/PostgreSQL business and authorization, Web workflows, shared Taro pages and Android host, contracts/procurement/claims, costs/subscriptions, project templates/collaboration/deliverables/changes/closeout, file versions and dual-cloud adapters, knowledge, notifications, security, status, controlled secretary tools and TG channel implementation. Increment75 added mobile project/member/owner management. Exact scope, evidence and remaining work are defined by the relevant protocols and gap register.
