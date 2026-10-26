# 更新记录 / Changelog

本文件记录可交付改动、验证范围和仍未放行的事项。完整逐轮历史见[实施台账](delivery/02-implementation-ledger.md)，所有未完成项保留[稳定 GAP 编号](delivery/03-goal-gap-assessment.md)。

This log records deliverable changes, verification scope and open release gates. See the [implementation ledger](delivery/02-implementation-ledger.md) for increment history and the [stable gap register](delivery/03-goal-gap-assessment.md) for unfinished work.

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
