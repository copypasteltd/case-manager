# CopyPaste Case Manager · 企业内部管理平台

**[中文完整说明](docs/README.zh-CN.md) · [Complete English guide](docs/README.en.md) · [文档中心 / Documentation](docs/README.md) · [演示 / Demos](docs/media/README.md) · [更新记录 / Changelog](docs/CHANGELOG.md)**

面向 CopyPaste 的内部工作空间，把项目、文件、知识、合同、采购、报销、费用订阅、公司秘书和服务状态接入同一套身份、权限、业务版本与审计。目标为 Web、微信小程序、抖音小程序和 Android 同批首发，首期服务香港的 3 名员工；Web／Android 承担最大 120 GB 原件，2 TB 存量纳入 OSS 与 Cloudflare R2。

An internal workspace for CopyPaste that connects projects, files, knowledge, contracts, procurement, expense claims, subscriptions, the company secretary and service status through shared identity, authorization, business revisions and audit history. The intended first release covers Web, WeChat, Douyin and Android for three Hong Kong users, with 120 GB Web/Android uploads and 2 TB of originals managed across OSS and Cloudflare R2.

> **开发预览 / Development preview.** 已有可运行代码、原生数据库集成检查和真实页面流程；完整四端生产验收尚未完成。演示使用隔离合成数据，移动录屏为 Taro H5，共享界面不等同于真机验证。真实云、模型、TG、120 GB／2 TB、依赖安全和生产恢复仍有放行项，见[编号差异表](docs/delivery/03-goal-gap-assessment.md)。
>
> Working code and native database/browser checks are included. Full production acceptance remains open. Demos use isolated synthetic data; mobile recordings show the shared Taro H5 interface rather than a device. Real cloud/model/TG operation, large-file and migration acceptance, dependency security and production recovery still have release gates.

![实际项目业务概览 / Actual project business overview](docs/media/screenshots/web-overview.png)

最新功能 / Latest workflow: [财务筛选、明细与限时导出 / Financial filters, sources and expiring exports](docs/development/88-financial-report-filters-and-exports.md).

![实际财务查询与异步导出 / Actual financial queries and asynchronous exports](docs/media/gifs/financial-reports-web.gif)

[Web MP4](docs/media/videos/financial-reports-web.mp4) · [Mobile MP4](docs/media/videos/financial-reports-mobile.mp4) · [Mobile GIF](docs/media/gifs/financial-reports-mobile.gif). 合成数据／真实本机API；移动为Taro H5。Synthetic data and real native API; mobile is Taro H5.

财务报表现支持期间、项目／公司、类别、供应方、经办／负责人、原币和关键词筛选，附原始明细分页。导出使用独立权限与后台任务，固定快照保留24小时；下载前重新鉴权并核对完整摘要。参见[操作、API、worker和四端差异](docs/development/88-financial-report-filters-and-exports.md)。

Financial reports now include interval, project/company, category, supplier, recorder/owner, original-currency and keyword filters with paged canonical sources. Independently authorized exports run asynchronously, expire after24 hours and reauthorize before verified retrieval. See the [workflow, API, worker and client behavior](docs/development/88-financial-report-filters-and-exports.md).

## 方案与能力 / Architecture and capabilities

| 组成 / Component | 实现与职责 / Implementation and responsibility |
| --- | --- |
| 业务后端 / Business backend | Go + PostgreSQL；统一身份、MFA、范围权限、业务命令、准确版本、幂等、审计与 outbox / Identity, MFA, scoped permissions, versioned commands, idempotency, audit and outbox |
| Web 管理端 / Web admin | React + Vite；项目、OA、文件、财务、秘书、状态和管理入口 / Project, OA, file, finance, secretary, status and administration workflows |
| 移动端 / Mobile clients | Taro 微信／抖音公共页面及 Android 原生宿主；小程序侧重查阅和审批，Android 提供原生文件传输 / Shared WeChat/Douyin pages and an Android native host; mini-program reading/approval and native Android transfer |
| 文件与备份 / Files and replicas | OSS + R2 适配、分片、固定源、摘要、扫描、正式版本、复制及恢复；真实环境另验 / OSS/R2 adapters, multipart transfer, frozen sources, checksums, scanning, formal versions, replication and recovery; external acceptance remains required |
| 公司秘书 / Company secretary | Codex SDK 运行时、8 项草稿 Skill、准确业务授权与真实回执、门户及 TG 调度 / Codex SDK runtime, eight draft Skills, explicit business authorization/receipts, portal and TG orchestration |
| 服务监控 / Service monitoring | 独立 Go Prometheus／Blackbox 桥接、原采样时刻、持久签名投递与完整回执；既有状态／历史／通知 / Native bridge, original sample times, durable signed delivery and bound ACKs; existing status, history and notifications |
| 原生运维 / Native operations | 独立 API／Worker／Runtime、最小权限数据库身份和服务模板；不启动本地 Docker／WSL／虚拟机／模拟器 / Separate native processes and least-privilege identities; no local virtualization |

不包含考勤或人事管理。Neither attendance nor HR management is in scope.

## 实际操作演示 / Actual interaction demos

GIF 为轻量预览，较长片段适度加速；MP4 保留完整屏幕操作。详情、数据来源、录制范围和重建命令见[演示目录](docs/media/README.md)。

GIFs are lightweight previews, with modest acceleration for the longer clip. MP4 recordings retain the complete interaction. See the [media catalog](docs/media/README.md) for scope, provenance and reproduction.

秘书创建任务／子任务：核对负责人、归属、起止时间、优先级与人工验收要求 → 完整授权预览。Secretary task creation: review assignment, parent, dates, priority and human acceptance, then preview the complete authorization.

![准确创建任务 / Exact task creation](docs/media/gifs/secretary-task-creation-web.gif)

[Web MP4](docs/media/videos/secretary-task-creation-web.mp4) · [移动MP4 / Mobile MP4](docs/media/videos/secretary-task-creation-mobile.mp4) · [移动GIF / Mobile GIF](docs/media/gifs/secretary-task-creation-mobile.gif) · [完整方案 / Workflow](docs/development/85-secretary-task-creation.md) · [完整预览截图 / Preview screenshot](docs/media/screenshots/secretary-task-creation-mobile-preview.png)

秘书风险与问题：核对完整资料 → 确认风险发生会新增问题 → 准确预览。Secretary concerns: review complete metadata, explicitly confirm the linked-issue effect, then preview the exact instruction.

![Web秘书风险与问题 / Web secretary concerns](docs/media/gifs/secretary-concerns-web.gif)

[Web MP4](docs/media/videos/secretary-concerns-web.mp4) · [移动MP4 / Mobile MP4](docs/media/videos/secretary-concerns-mobile.mp4) · [移动GIF / Mobile GIF](docs/media/gifs/secretary-concerns-mobile.gif) · [完整方案 / Workflow](docs/development/84-secretary-project-concerns.md) · [页面预览 / Screenshot](docs/media/screenshots/secretary-concerns-web-preview.png)

移动 TG 入口：本人数字身份绑定、管理员准确群配置和完整历史。录屏仅展示核对后的配置与历史，不录制私聊能力或 MFA。Mobile TG access adds own-identity binding, exact channel configuration and complete history. The recording excludes private proof capabilities and MFA inputs.

移动知识操作：准确附件 → 草稿修订 → 人工确认发布 → 历史恢复及修订讨论。Mobile knowledge: explicit attachments, draft revisions, confirmed publication, restoration and revision-bound discussion.

![移动知识编辑与发布 / Mobile knowledge edit and publication](docs/media/gifs/mobile-knowledge-access.gif)

[完整 MP4 / Full MP4](docs/media/videos/mobile-knowledge-access.mp4) · [使用、接口与约束 / Workflow, API and constraints](docs/development/83-mobile-knowledge-workflows.md) · [编辑页面 / Editor screenshot](docs/media/screenshots/mobile-knowledge-editor.png)

![移动 TG 入口与完整历史 / Mobile TG access and complete history](docs/media/gifs/mobile-telegram-access.gif)

[TG 操作 MP4 / TG interaction MP4](docs/media/videos/mobile-telegram-access.mp4) · [方案和使用逻辑 / Protocol and workflow](docs/development/82-mobile-telegram-access.md) · [入口截图 / Channel screenshot](docs/media/screenshots/mobile-telegram-channel-config.png) · Taro H5

实际原生状态桥接：失败阈值 → 服务异常 → 恢复阈值 → 原始签名核验。采样由受控 Prometheus 协议端提供；程序、API和页面均实际执行。Native bridge demo: outage threshold → service failure → recovery → original-signature verification, using controlled metric samples and the actual CLI/API/UI.

![原生监控与实际状态切换 / Native monitor and actual status transitions](docs/media/gifs/native-service-health-workflow.gif)

[状态桥接 MP4 / Status bridge MP4](docs/media/videos/native-service-health-workflow.mp4) · 6.0 s · [部署与协议 / Deployment and protocol](docs/development/81-native-prometheus-status-bridge.md) · [实际状态截图 / Actual status screenshot](docs/media/screenshots/native-status-bridge-down.png)

![Web 项目创建与任务闭环 / Web project and task workflow](docs/media/gifs/web-project-flow.gif)

[Web 项目操作 MP4 / Web project MP4](docs/media/videos/web-project-flow.mp4) · [业务台账浏览 MP4 / Operations tour MP4](docs/media/videos/web-operations-tour.mp4) · [移动风险与问题 MP4 / Mobile risk/issue MP4](docs/media/videos/mobile-risk-issue.mp4)

移动任务操作：任务新建 → 子任务 → 里程碑关联 → 前置依赖及实际回读。Mobile task flow: create a task, create a subtask, select a milestone and reconcile a prerequisite through actual reads.

![移动任务、子任务与前置依赖 / Mobile task, subtask and prerequisite](docs/media/gifs/mobile-task-workflow.gif)

[移动任务 MP4 / Mobile task MP4](docs/media/videos/mobile-task-workflow.mp4) · [具体操作与约束 / Workflow protocol](docs/development/78-mobile-task-editing-and-dependencies.md) · 6.7 s · Taro H5

移动结项操作：完整清单 → 明确结项 → 复盘／双项核实／MFA → 归档及实际回读 → 受控重开。Closeout flow: complete checklist → closing → retrospective, attestations and MFA → archive with actual proof → controlled reopen.

![移动项目结项与复盘 / Mobile project closeout and retrospective](docs/media/gifs/mobile-project-closeout-workflow.gif)

[移动结项 MP4 / Mobile closeout MP4](docs/media/videos/mobile-project-closeout-workflow.mp4) · [操作与约束 / Workflow protocol](docs/development/79-mobile-project-closeout-and-archive.md) · Taro H5

移动项目变更：明确采用项目／币种预算基线 → 完整提案与固定文件 → 原申请修订及独立审批 → 双项核实／MFA → 明确实施及实际回读。Mobile changes: accept the exact project/currency basis → full proposal and fixed files → original request revision and independent approval → two attestations/MFA → explicit implementation with actual source proof.

![移动完整变更与预算提案 / Mobile full change and budget proposal](docs/media/gifs/mobile-project-change-workflow.gif)

[移动变更 MP4 / Mobile change MP4](docs/media/videos/mobile-project-change-workflow.mp4) · [完整操作约束 / Complete workflow](docs/development/80-mobile-project-changes-and-budgets.md) · Taro H5；录制展示初次提案，审批与实施结果见验证记录 / Capture shows initial drafting; approval/implementation are documented in the verification record.

## 使用与开发 / Usage and development

1. 按[中文指南](docs/README.zh-CN.md)或 [English guide](docs/README.en.md)准备原生工具、安装依赖、迁移数据库并初始化开发身份。
2. 在 Web 完成项目与角色配置，按“草稿 → 固定依据 → 明确提交／批准 → 原业务执行 → 历史核对”使用业务流程。Start with projects and roles, then follow draft → fixed evidence → explicit submission/approval → original business execution → history review.
3. 按需要编译两小程序或 Android 开发包；实际平台账号、域名、签名、设备、存储和模型单独配置及验收。Build client packages and independently configure/accept platform accounts, domains, signing, devices, storage and model access.
4. 服务监控按[桥接指南](docs/development/81-native-prometheus-status-bridge.md)在独立监控机配置准确来源修订、公钥、标签及端点；先升级 API，再检查一次采集和受控服务。For monitoring, configure the exact source revision/key/labels/endpoints on a dedicated monitor host; upgrade the API before enabling its bridge.


工具已准备后，在仓库根目录执行以下本地启动步骤。After preparing native prerequisites, run these commands from the repository root:

```powershell
$env:PYTHONUTF8='1'
npm ci --prefix app
python app/scripts/dev_database.py start
python app/scripts/run_backend.py migrate
python app/scripts/seed_development.py
```

分别在两个终端启动 API 和 Web。Start API and Web in separate terminals:

```powershell
python app/scripts/run_backend.py api
```

```powershell
npm run dev --prefix app
```

Web：`http://localhost:5178`。开发账号随机生成并保存在已忽略的 `app/.local/development-accounts.json`，不设通用默认密码。The development accounts are randomly generated in that ignored file; there is no universal default password. [详细前提与启动逻辑 / Prerequisites and startup logic](docs/README.en.md#native-local-development).

## 仓库导航 / Repository map

```text
app/
  backend/         Go API, domain modules, migrations and native workers
  web/             React/Vite admin
  mobile/          Taro clients and Android native host
  agent-runtime/   Codex SDK runtime
  packages/        Generated API contracts and shared design tokens
  skills/          Eight versioned draft Skill packages
  deploy/native/   Native worker service templates
  scripts/         Setup, checks, recording and publication tools
docs/
  README.zh-CN.md  Complete Chinese solution and usage guide
  README.en.md     Complete English solution and usage guide
  requirements/   Business requirements and invariants
  development/    Implementation protocols and OpenAPI
  operations/     Deployment, migration, backup and recovery
  quality/        Traceability and source-bound evidence
  delivery/       Implementation ledger and 28 stable gap IDs
  media/          Screenshots, MP4 recordings and GIF previews
  CHANGELOG.md    Bilingual update notes
```

## 验证与后续更新 / Verification and maintenance

当前功能基线（增量87）新增项目预算与实际支出对比：在“项目 → 预算”明确选择已配置币种，逐项核对当前已实施的完整预算、原费用的本项目份额、冲销及预算外发生。可按明细／状态分页查来源并打开规范原记录；未确认费用单列。缺失预算与批准清空的零预算不同，历史预算不会冒充当前依据。见[完整使用及接口规则](docs/development/87-project-budget-comparison.md)与[验证记录](docs/quality/evidence/budget-comparison/verification.md)。

本轮后端159顶层、1,041结果／945末级、8批，72项既有迁移保持原字节；Web104、运行时47、移动187项及两小程序各17页通过。Web／Taro各6组实际流程、3项布局；Android开发包18项检查通过，25项JVM测试本轮使用缓存；Linux仅交叉编译。后端隔离库清理超时与捕获检查器的失败记录、相同来源恢复过程均保留。

演示目录共48 PNG、17 MP4、17 GIF；本轮新增四张预算对比截图和Web／Taro各一对实际短录屏。增量86完整启动及两端捕获来源按已发布94081c0重建，99份旧证据保持原字节；更早84的未冻结编译缓存限制仍保留。对比始终只代表当前授权费用，不宣称全部项目余额或可用资金。保留全部28项原编号；完整四端、云、模型／TG、120 GB／2 TB、安全、恢复与生产验收仍开放。

The current functional baseline (increment87) adds project budget versus actual expenditure. Open **Project → Budget**, select an explicitly configured currency, and compare the complete current implemented plan against this project's canonical expense shares, reversals and out-of-plan activity. Filter and page sources, then open the original record. Drafts remain separate; an absent plan differs from an explicitly cleared zero plan. See the [workflow and API rules](docs/development/87-project-budget-comparison.md) and [verification](docs/quality/evidence/budget-comparison/verification.md).

This round passes159 backend top-level tests,1,041 results/945 leaves in eight source-bound batches, preserving all72 existing migrations;104 Web,47 runtime and187 mobile tests, with17 pages per mini-program. Each Web/Taro demonstration checks six actual flows and three layouts. The development APK passes18 inspections;25 JVM results are cached this round. Linux evidence establishes cross-compilation only. Retained failures document a disposable-database cleanup timeout and capture-checker assumptions, including the source-checked recovery.

The catalog contains48 PNGs,17 MP4s and17 GIFs, including four new comparison screenshots and an actual short recording/GIF pair per client. Increment86 complete startup and both captures reconstruct from published94081c0;99 historical evidence files retain exact bytes. Earlier increment84 cache limitations remain explicit. Comparisons cover authorized records only and do not establish total project balances or available funds. All28 gap IDs remain in the register; full device/cloud/model/TG/capacity/security/recovery/production acceptance remains open.
