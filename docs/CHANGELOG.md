# 更新记录 / Changelog

本文件记录可交付改动、验证范围和仍未放行的事项。完整逐轮历史见[实施台账](delivery/02-implementation-ledger.md)，所有未完成项保留[稳定 GAP 编号](delivery/03-goal-gap-assessment.md)。

This log records deliverable changes, verification scope and open release gates. See the [implementation ledger](delivery/02-implementation-ledger.md) for increment history and the [stable gap register](delivery/03-goal-gap-assessment.md) for unfinished work.

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
