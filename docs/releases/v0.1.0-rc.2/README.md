# v0.1.0-rc.2 · 冻结候选 / Frozen candidate

本候选封存134–136最近三轮已实现功能；首个`v0.1.0-rc.1`的原标签和下载保持不变。本次固定业务源码`40cd118634117204e2e310395fac32b59af5cecc`、结构103，提供新的服务器、两小程序和Android开发产物。按用户要求精简重复验证，23个未完成包及正式生产验收保持开放。

This candidate seals the last three implemented increments at source `40cd118634117204e2e310395fac32b59af5cecc`, schema103. The first candidate remains immutable. Native server, miniapps and a new development APK are freshly built; selected validation is deferred by the user. All23 remaining packages and production acceptance stay open.

[固定源码 / Source](https://github.com/copypasteltd/case-manager/tree/v0.1.0-rc.2) · [下载目录 / Assets](https://github.com/copypasteltd/case-manager/tree/codex/release-v0.1.0-rc.2/docs/releases/v0.1.0-rc.2) · [全部余项 / Remaining work](https://github.com/copypasteltd/case-manager/blob/main/docs/delivery/05-functional-closure-137.md)

## 下载与校验 / Downloads and integrity

| 编号 | 下载 | 字节 |
|---|---|---:|
| F137-01 | [Linux amd64服务器／Web／秘书运行器](https://github.com/copypasteltd/case-manager/raw/codex/release-v0.1.0-rc.2/docs/releases/v0.1.0-rc.2/case-manager-v0.1.0-rc.2-linux-amd64.tar.gz) | 31,152,401 |
| F137-02 | [微信／抖音小程序](https://github.com/copypasteltd/case-manager/raw/codex/release-v0.1.0-rc.2/docs/releases/v0.1.0-rc.2/case-manager-v0.1.0-rc.2-miniapps.zip) | 979,923 |
| F137-03 | [Android开发APK](https://github.com/copypasteltd/case-manager/raw/codex/release-v0.1.0-rc.2/docs/releases/v0.1.0-rc.2/case-manager-v0.1.0-rc.2-android-debug.apk) | 43,911,771 |
| F137-04 | [完整应用源码／开发文档](https://github.com/copypasteltd/case-manager/raw/codex/release-v0.1.0-rc.2/docs/releases/v0.1.0-rc.2/case-manager-v0.1.0-rc.2-source.zip) | 9,933,002 |
| F137-05 | [演示媒体目录](https://github.com/copypasteltd/case-manager/raw/codex/release-v0.1.0-rc.2/docs/releases/v0.1.0-rc.2/case-manager-v0.1.0-rc.2-media.zip) | 89,097,550 |

[SHA256SUMS](SHA256SUMS.txt) · [机器可读清单 / Manifest](release-manifest.json) · [原生构建回执 / Build receipt](server-build.json)。先校验压缩包自身SHA-256，再执行服务器检查。服务器**内层清单**SHA-256（不是压缩包摘要）：

`b7ac9be38c5a456db91dd969a4a01589991ce9ddd86d39cad7733bfb5cbfd5b3`

The source ZIP includes all2280 application files, tests, lockfiles and development guides. It excludes only `docs/media/` and `docs/quality/evidence/`; published media has its own ZIP, while large historical evidence remains in the complete frozen Git tag. Every packaged file matches the frozen commit. For the full repository including history/evidence use `git clone --branch v0.1.0-rc.2 git@github.com:copypasteltd/case-manager.git`. No credentials, database, installed dependencies, local tools, caches or signing keys are included. Packaging tools and publication records are retained in increment137 on main; business source archives deliberately remain the exact136 snapshot.

## 主业务与访问逻辑 / Business and access

项目与任务／交付→合同、采购、报销等申请与审批→真实费用→实际收付款与核销→合同财务归属、退款与核对→结项。审批、费用、现金和合同归属分别记账。Web为入口，微信／抖音侧重查阅与审批，Android保留原生上传及共享业务页面，四端使用同一后端权限体系。

Included updates: foreign expense-refund attribution, receivable-contract foreign outgoing returns and explicit reauthorization of a stopped original-cleanup remainder. Actual cash and principal units stay separate, immutable receipts/history are retained and authority is checked again before execution. See the full [bilingual solution guide](../../README.en.md) and [Chinese guide](../../README.zh-CN.md).

## 操作步骤 / Usage

1. 下载所需包及SHA256SUMS，核对准确文件名和SHA-256。源码与媒体ZIP可直接解压到同一新目录；不覆盖已有开发目录。
2. 使用可信源码中的发布检查器：`python3 -B app/deploy/release/bundle.py inspect --archive PACKAGE.tar.gz --manifest-sha256 FULL_INNER_MANIFEST_SHA256 --output app/.local/inspection-UNIQUE`。
3. 按[原生发布手册](../../operations/03-native-release-runbook.md)准备Linux身份、持久PostgreSQL、HTTPS和私有配置，完成暂存、正常结构升级和激活。开发服务仍结构100；新后端须升级101–103，全新安装正常应用1–103。
4. 员工通过Web登录和MFA，按角色访问项目、OA、文件、合同、财务与服务状态。云存储、秘书、TG和平台必须配置真实身份后分别验收。
5. 小程序及APK当前是明确的**本地API配置**`http://127.0.0.1:18080`。它们不会自动连接公司服务器；设置实际HTTPS API、真实AppID／合法域名并重建后，才能用于部署环境。

Verify the exact asset hash, inspect with trusted release tools, provision native target identities/PG/HTTPS and apply the ordinary schema upgrades before activation. Mobile artifacts use a local API profile and require rebuilding for the real server. Android carries a development signature; formal signing, version/publication configuration and platform acceptance remain open. No target server or production service was changed by this delivery.

## 检查与延期 / Checks and deferrals

本轮：791项服务器包文件、103结构清单，Web／秘书JS／两小程序实际构建，新Android开发APK10项内容检查、173个实际源码映射模块，完整源码还原和展开私密资料检查。Android原生任务为`> Task :app:testDebugUnitTest`；本次Android原生28项实际执行通过，保留准确XML及执行时间。单独JVM套件没有重跑，其他缓存任务不记为新执行。

Retained, not rerun: increment136 Web452/shared-mobile537 rules, seven native HTTP/PG suites with42 passing records, and actual screenshots/video/GIFs. Full backend/runtime regression remains at121. The Node version-probe retry and interrupted cold Metro index are preserved in [the verification record](https://github.com/copypasteltd/case-manager/blob/main/docs/quality/137-frozen-candidate-verification.md).

延期完整回归、额外浏览器旅程／录制、开发库100→103更新、真实Linux／生产PG／OSS＋R2／Codex／TG／平台接入、正式Android签名发布及当前范围完整验收。八项Skill仍draft。真机、网络性能与实网120GB专项、历史迁移／容灾、AI费用控制按用户要求移出，不计为通过。

GitHub Releases网页本轮未创建；此前保存API账号返回403的历史记录保留，本轮未重试，使用已授权SSH推送源码标签和独立下载分支。No GitHub Releases page is claimed; delivery uses the source tag and downloadable artifact branch. No force push, virtualization, emulator, new browser capture or production deployment was used.
