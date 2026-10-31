# v0.1.0-rc.8 · 同版冻结交付 / Matching frozen delivery

业务153、结构113固定于 `160d78f8032be21c9eab72fc28415fda598fb7df`。本轮154停止业务扩展，交付同版服务器、Web、秘书运行器、微信、抖音和新Android开发APK，包含153个人TG业务提醒。七个既有冻结版本保持原字节。

All application changes through153 are frozen at this commit. Five downloads provide matching server/client artifacts, complete application source/docs and all public media. Packaging154 helpers and handover remain on main. This is an internal candidate; formal signing, actual deployment and external-service acceptance remain open.

## 固定下载 / Fixed downloads

| 编号 / No. | 内容 / Artifact | 获取 / Download |
| --- | --- | --- |
| 1 | Linux服务器＋Web＋运行器 / Server, Web and runtime | [case-manager-v0.1.0-rc.8-linux-amd64.tar.gz](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.8/docs/releases/v0.1.0-rc.8/case-manager-v0.1.0-rc.8-linux-amd64.tar.gz?raw=true) |
| 2 | 微信＋抖音本地包 / WeChat and Douyin local builds | [case-manager-v0.1.0-rc.8-miniapps.zip](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.8/docs/releases/v0.1.0-rc.8/case-manager-v0.1.0-rc.8-miniapps.zip?raw=true) |
| 3 | Android开发安装包 / Development APK | [case-manager-v0.1.0-rc.8-android-debug.apk](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.8/docs/releases/v0.1.0-rc.8/case-manager-v0.1.0-rc.8-android-debug.apk?raw=true) |
| 4 | 完整应用源码、依赖锁和文档 / Complete application source, locks and docs | [case-manager-v0.1.0-rc.8-source.zip](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.8/docs/releases/v0.1.0-rc.8/case-manager-v0.1.0-rc.8-source.zip?raw=true) |
| 5 | 全部公开截图、MP4和GIF / All public media | [case-manager-v0.1.0-rc.8-media.zip](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.8/docs/releases/v0.1.0-rc.8/case-manager-v0.1.0-rc.8-media.zip?raw=true) |

[外层校验 / Checksums](SHA256SUMS.txt) · [固定清单 / Manifest](release-manifest.json) · [固定源码 / Source tag](https://github.com/copypasteltd/case-manager/tree/v0.1.0-rc.8)

## 使用流程 / Use and deployment

1. 核对外层SHA256、固定源提交及结构版本，再按可信摘要检查服务器包的内层清单。完整应用源码／规格／使用指南固定为153，历史核验证据在注释源码标签，全部媒体独立成包；154封包工具和交接文档在main。
2. 按[原生Linux发布手册](../../development/119-native-production-release.md)准备目标Linux、持久PostgreSQL、HTTPS和真实身份密钥。现有本机开发PG100／API原样保留；部署新后端须正常升级至113，不以重建库代替升级。正式文件使用OSS＋R2，服务器只作受控临时处理／缓存。
3. 小程序和开发APK使用本地API构建配置。连接实际服务器前配置真实HTTPS、平台AppID／域名并重新构建；Android正式签名、升级、目标SDK／原生桥接及平台发布另行完成。
4. 登录后按角色进入项目／OA／财务／文件／秘书／状态／通知。主要业务以正式回执和独立审批／验收为准；AI计划、聊天、已读和排队不替代业务完成。
5. 个人TG提醒：先在账户设置完成双向绑定，再在“通知”本人选Bot、十类业务事件及MFA确认后开启；站外只有固定登录提示。未知结果由本人核对准确消息、明确重试或终止。停用保留站内通知与历史。部署须启用独立个人通知工作器，详见[153使用说明](../../development/153-personal-telegram-notifications.md)。
6. 真实Codex、Linux隔离、八项Skill和股东TG配置继续按未完成工作包实施。真实服务器／云／模型／TG／平台资料按用户答复后续提供；不声称当前包已连接真实服务。

Verify outer hashes and the trusted inner server manifest. Prepare native Linux, persistent PG and HTTPS and upgrade normally to113. Set real platform/API identities and rebuild the clients before connecting them to that server. Use current business roles; exact AI authorization never replaces independent approval or acceptance. Personal TG notices require confirmed binding, explicit owner consent, recent MFA and the dedicated worker. Only a fixed sign-in prompt is sent; uncertain delivery requires human reconciliation.

## 核验界限 / Validation boundaries

154新执行服务器／Web／运行器交叉构建、两小程序构建、Android开发包构建和实际映射检查、五包展开及私密检查、完整应用Git字节重建和固定引用核对。Linux没有在目标主机执行或部署。JVM任务实际缓存／新执行状态、28条XML的原时间见[构建范围](../../quality/evidence/release-candidate-154/android-build-scope.json)，不把缓存称为新测试。

153业务证明按原字节保留：原生HTTP／PG十四个子场景加一条汇总，传输适配器十八条通过记录，Web4／共享移动4和三项实际Web交互及PNG／无声MP4／六秒截图导览GIF。154不重跑业务或重录媒体；运行器58项来自151、完整后端121／完整客户端136也仅保留。详见[准确核验与延期](../../quality/154-frozen-release-verification.md)。

Fresh artifact construction and inspection are separate from retained business proof. Broader regression and extra mobile UI journeys are deferred for speed. All seven earlier freezes remain immutable. The existing GitHub Releases API credential previously returned403/suspended; delivery uses authorized SSH source/artifact refs without claiming a Releases webpage. No unchanged credential retry was performed.

用户移出真机、网络性能／实网120GB专项、历史迁移／容灾和AI费用控制。保留四端、真实秘书／TG、AI授权／隔离／审计／技术限制、普通恢复、正常升级、PG及OSS＋R2。全部23项（14 P0／9 P1）继续开放，八项Skill仍draft，见[完整未完成表](../../delivery/05-functional-closure-154.md)。

[中文使用](../../README.zh-CN.md) · [English guide](../../README.en.md) · [方案与操作](../../development/154-frozen-release-delivery.md)
