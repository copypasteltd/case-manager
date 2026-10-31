# v0.1.0-rc.3 · 冻结交付 / Frozen delivery

本版固定当前业务143／结构109，包含138–143最新功能。服务器、Web、秘书运行器、微信／抖音和Android开发包重新生成；附完整应用源码／开发文档与全部既有演示媒体。按用户要求精简验证，完成八组主流程和产物必要检查；原两个冻结候选保持不可变。

This candidate fixes business source `e1c7a671cc4c7f52888a1e6ed72d7ce1a9c042d3`, schema109. All four client artifacts are rebuilt, with complete application source/docs and a separate exact media archive. Selected validation is deferred by the user; eight primary HTTP/PG suites and artifact integrity checks are current. Production integration and all23 remaining work packages stay open.

[固定源码 / Source](https://github.com/copypasteltd/case-manager/tree/v0.1.0-rc.3) · [下载目录 / Assets](https://github.com/copypasteltd/case-manager/tree/codex/release-v0.1.0-rc.3/docs/releases/v0.1.0-rc.3) · [全部23项 / Remaining work](../../delivery/05-functional-closure-144.md)

## 1. 下载与摘要 / Downloads and integrity

| 编号 | 下载 |
|---|---|
| F144-01 | [Linux amd64服务器／Web／秘书运行器](https://github.com/copypasteltd/case-manager/raw/codex/release-v0.1.0-rc.3/docs/releases/v0.1.0-rc.3/case-manager-v0.1.0-rc.3-linux-amd64.tar.gz) |
| F144-02 | [微信／抖音小程序](https://github.com/copypasteltd/case-manager/raw/codex/release-v0.1.0-rc.3/docs/releases/v0.1.0-rc.3/case-manager-v0.1.0-rc.3-miniapps.zip) |
| F144-03 | [Android开发APK](https://github.com/copypasteltd/case-manager/raw/codex/release-v0.1.0-rc.3/docs/releases/v0.1.0-rc.3/case-manager-v0.1.0-rc.3-android-debug.apk) |
| F144-04 | [完整应用源码／开发文档](https://github.com/copypasteltd/case-manager/raw/codex/release-v0.1.0-rc.3/docs/releases/v0.1.0-rc.3/case-manager-v0.1.0-rc.3-source.zip) |
| F144-05 | [完整演示媒体](https://github.com/copypasteltd/case-manager/raw/codex/release-v0.1.0-rc.3/docs/releases/v0.1.0-rc.3/case-manager-v0.1.0-rc.3-media.zip) |

[SHA256SUMS](SHA256SUMS.txt) · [准确文件大小与清单 / Manifest](release-manifest.json) · [服务器构建回执 / Build receipt](server-build.json)。先核对压缩包自身SHA-256，再检查服务器**内层清单**；二者摘要用途不同。内层清单摘要：

`119922952bffdad7fca1c53a58c9123ddd3a61ac14842113f11c2c16baeff2b8`

源码ZIP包含指定提交的全部应用源码、测试、锁文件、迁移及开发文档；docs/media另包，较大的docs/quality/evidence历史证明保留完整Git标签。打包字节逐文件与原Git blob对应。完整仓库及证据使用 `git clone --branch v0.1.0-rc.3 git@github.com:copypasteltd/case-manager.git`。本轮封装工具和发版记录在main144，业务标签和源码ZIP准确固定143。

The source ZIP excludes only docs/media and historical docs/quality/evidence. All application code is included; the exact media complement has its own ZIP. Use the frozen Git tag for full historical evidence. No secrets, databases, installed dependencies, local tools, caches or signing private keys are included.

## 2. 主功能与操作 / Business and usage

项目／任务／里程碑／交付→合同、采购、报销草稿与审批→人工确认实际费用→实际收付款与核销→退款／合同财务核对→结项。审批、费用、资金及合同归属分别保存。文件、版本、博客、工作台和服务状态共用后端权限。秘书计划仍需授权及审计；八项Skill仍draft，真实模型／Linux隔离／TG待实际接入。

Newly packaged138–143: completed uncommitted upload cleanup, organization TG delivery/inbound operations, runtime-profile retirement, project people/template catalogs and complete subscription portfolio review. See the [Chinese guide](../../README.zh-CN.md), [English guide](../../README.en.md) and [delivery design](../../development/144-frozen-candidate-delivery.md).

1. 下载所需包及SHA256SUMS，核对准确文件名及摘要。源码和媒体ZIP可解压到同一新目录，不覆盖现有开发目录。
2. 使用可信源码检查服务器包：`python3 -B app/deploy/release/bundle.py inspect --archive PACKAGE.tar.gz --manifest-sha256 FULL_INNER_MANIFEST_SHA256 --output app/.local/inspection-UNIQUE`。
3. 按[原生发布手册](../../operations/03-native-release-runbook.md)配置目标Linux身份、持久PG、HTTPS和私有密钥，完成暂存、正常结构升级及激活。开发库仍100，运行新后端前正常应用101–109；全新安装应用1–109。
4. 员工从Web登录／MFA，按角色进入项目、OA、文件、财务和服务状态。云存储、模型、TG和平台身份分别配置及验收。
5. 当前小程序／APK使用明确本地API `http://127.0.0.1:18080`，不会自动连接公司服务器。部署使用前设置实际HTTPS API、真实AppID和合法域名并重建；APK当前为开发签名。

Verify the asset hashes and trusted inner manifest, then follow the native runbook to provision the target and apply ordinary upgrades. Mobile artifacts require the real HTTPS API/platform identities and a rebuild before server use. Formal Android signing/publication remains separate. No production deployment or development-service replacement was performed.

## 3. 当前检查与延期 / Checks and deferrals

本轮服务器792项包文件、109项结构；两小程序151项输出；新APK10项实际内容检查、183个源码映射模块一致。八组HTTP／耐久PG主流程共87个子场景，加8个组汇总为95条通过记录，无失败／跳过；对象传输为测试替身，不代表真实OSS／R2。

Android原生单元任务为 `> Task :app:testDebugUnitTest UP-TO-DATE`，28项XML来自缓存，本轮没有新执行；单独JVM未重跑。143选定业务／运行时56项、136完整Web452／共享移动537项、121完整后端／运行器及各轮实际媒体按原出处复用，不声称本轮重跑。见[验证与精确范围](../../quality/144-frozen-candidate-verification.md)。

Deferred: full regression, extra UI journeys/recordings, development PG upgrade, real Linux/CI/HTTPS/PG/cloud/model/TG/platform acceptance and formal Android release. Excluded by the user: real devices, network performance/live120GB special, historic migration/disaster recovery and AI fee control. AI authorization/isolation/audit/resource limits, ordinary file recovery, persistent PG and OSS＋R2 remain required. All23 packages and eight draft Skills stay open.

GitHub Releases网页本轮未创建；实际交付使用已授权SSH推送的新源码标签及独立下载分支，不覆盖原版本或强推。No GitHub Releases page is claimed; the source tag and downloadable artifact branch are the publication mechanism.
