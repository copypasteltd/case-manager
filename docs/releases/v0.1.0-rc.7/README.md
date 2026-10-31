# v0.1.0-rc.7 · 内部冻结交付 / Internal frozen delivery

业务151、结构112固定于 `59102225bc0c82bba0d838fdb8a219d06f958bbf`。本轮152停止业务扩展，只将同一源码的服务器、Web、秘书运行器、微信、抖音及Android开发包封装交付；六个既有冻结版本保持原字节。新增准确授权的固定交付提交，含原版本／原修订、正式附件、独立验收、幂等和审计约束。

All application changes through151 are fixed at the commit above. This freeze packages matching server/Web/runtime, WeChat, Douyin and development Android outputs, complete application source/docs and all public media. It is an internal candidate; formal signing, actual production deployment and external-service acceptance remain open.

## 固定下载 / Fixed downloads

| 编号 / No. | 内容 / Artifact | 获取 / Download |
|---|---|---|
| 1 | Linux服务器＋Web＋运行器 / Server, Web and runtime | [case-manager-v0.1.0-rc.7-linux-amd64.tar.gz](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.7/docs/releases/v0.1.0-rc.7/case-manager-v0.1.0-rc.7-linux-amd64.tar.gz?raw=true) |
| 2 | 微信＋抖音本地包 / WeChat and Douyin local builds | [case-manager-v0.1.0-rc.7-miniapps.zip](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.7/docs/releases/v0.1.0-rc.7/case-manager-v0.1.0-rc.7-miniapps.zip?raw=true) |
| 3 | Android开发安装包 / Development APK | [case-manager-v0.1.0-rc.7-android-debug.apk](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.7/docs/releases/v0.1.0-rc.7/case-manager-v0.1.0-rc.7-android-debug.apk?raw=true) |
| 4 | 完整应用源码、依赖锁和文档 / Complete application source, locks and docs | [case-manager-v0.1.0-rc.7-source.zip](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.7/docs/releases/v0.1.0-rc.7/case-manager-v0.1.0-rc.7-source.zip?raw=true) |
| 5 | 全部公开截图、MP4和GIF / All public media | [case-manager-v0.1.0-rc.7-media.zip](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.7/docs/releases/v0.1.0-rc.7/case-manager-v0.1.0-rc.7-media.zip?raw=true) |

[外层校验 / Checksums](SHA256SUMS.txt) · [固定清单 / Release manifest](release-manifest.json) · [源码](https://github.com/copypasteltd/case-manager/tree/v0.1.0-rc.7)

## 使用流程 / Use and deployment

1. 先核对外层SHA256和清单中的固定源码、结构版本、产物类型，再核对服务器包内层清单。源码压缩包包含全部app实现、锁文件和规格／使用文档；历史证据保留在源码标签，媒体独立打包。
2. 按[原生Linux发布手册](../../development/119-native-production-release.md)准备目标Linux、持久PostgreSQL、HTTPS及真实身份密钥。服务器本地正式文件只作临时处理／缓存；OSS＋R2按已配置业务管理。原开发PG100未被升级，使用新后端前须按正常流程升级至112；不以重建库或历史迁移代替升级。
3. 小程序和开发APK使用本地API构建配置。部署到实际服务器前，填写真实HTTPS API、平台AppID／域名并重新构建；Android正式签名、升级与平台发布须另行配置。本地包不声称已经提审或可直接连接生产。
4. 登录后按角色进入项目／OA／财务／文件／秘书／服务状态。秘书交付：选择同项目可提交交付、固定正式附件、说明、原因并勾选核对后发送；实际回执生成待验收修订，再由独立人员办理验收。旧修订保留；已读、排队或自然语言回复不等于业务成功。
5. 真实Codex／八项Skill／TG配置与正式验收仍按独立工作包处理。受控演示只展示真实Web与原生PG业务边界，没有真实模型执行。

Verify outer checksums and the trusted inner server manifest before deployment. Prepare native Linux, persistent PostgreSQL, HTTPS and real provider identities. Upgrade normally to schema112. Configure real API/platform identities and rebuild clients for server use; the included APK is a development build. Use current project roles for OA, finance, files and status. A secretary submission authorizes an exact pending revision; independent human acceptance remains separate.

## 核验界限 / Validation boundaries

151新执行3组原生HTTP／PG检查，共36条pass记录（33个子场景＋3组汇总），Web4、移动共享4及运行器58项；类型与Web／微信／抖音构建、实际Web3项和三张截图／无声像素录屏／六秒截图导览均单列。152保留这些准确源码证据，只重新封包和核对产物；不再次执行全量业务回归或录制。完整后端121和完整客户端136仍属原出处。

当前Android包重新从151构建；原生JVM任务究竟缓存还是新执行以[构建范围](../../quality/evidence/release-candidate-152/android-build-scope.json)为准，不能把缓存XML说成新执行。Linux产物是本机交叉构建，没有在目标Linux执行或部署。[准确核验](../../quality/152-frozen-release-verification.md)记录产物、源绑定与延期。

All six older freezes remain immutable. Reduced validation is explicit. The existing GitHub Releases API account previously returned403/suspended; this release is delivered through authorized SSH source/artifact refs, without claiming a GitHub Releases webpage. No unchanged credential retry or production deployment was performed.

用户移出真机、网络性能／实网120GB专项、历史迁移／容灾和AI费用控制。保留四端、真实秘书／TG、AI授权／隔离／审计／技术限制、普通文件恢复、正常结构升级、PG及OSS＋R2。全部23项（14 P0／9 P1）见[完整未完成表](../../delivery/05-functional-closure-152.md)。

[中文使用](../../README.zh-CN.md) · [English guide](../../README.en.md) · [源码发布说明](../../development/152-frozen-release-delivery.md)
