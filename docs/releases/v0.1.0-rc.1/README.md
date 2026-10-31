# v0.1.0-rc.1 · 首个冻结发布 / First frozen release

这是首个可下载的冻结候选版本。源码以Git标签 `v0.1.0-rc.1` 固定；发布包和SHA-256清单位于独立产物分支 `codex/release-v0.1.0-rc.1`。按照用户明确要求保留主业务必要检查，延期扩展验证；并不宣布全部23工作包或生产验收完成。

This first frozen candidate pins source with the Git tag. Downloadable artifacts and exact hashes live on the dedicated artifact branch. The user explicitly authorized reduced validation; deferred checks and external integrations remain open.

[冻结源码 / Source](https://github.com/copypasteltd/case-manager/tree/v0.1.0-rc.1) · [发布产物 / Downloads](https://github.com/copypasteltd/case-manager/tree/codex/release-v0.1.0-rc.1/docs/releases/v0.1.0-rc.1) · [完整双语入口 / Guide](../../../README.md)

## 包含内容 / Included

| 编号 | 产物 | 使用逻辑 |
|---|---|---|
| R-01 | Linux amd64服务器包 | 三个原生ELF、Web、独立秘书运行器／依赖、八项draft Skill、100项结构清单、systemd模板和固定发布工具 |
| R-02 | 微信／抖音构建产物 | 当前源码编译结果；需配置真实AppID、合法域名和平台主体后由各平台发布；不能把本地回环配置直接提审 |
| R-03 | Android开发包 | 本轮43,908,022字节APK，320文件输入／真实源码映射一致、19项内容检查通过；本地API配置，连接服务器须重建；正式签名／应用商店发布仍待完成 |
| R-04 | SHA256SUMS与release-manifest | 校验准确下载字节、构建源码提交、结构100和每项产物状态；没有完成的产物不写成成功 |
| R-05 | 冻结源码／使用文档 | 四端、Go／PG、OA／合同／财务、项目／文件／监控及秘书工具代码和编号余项 |

公司业务主线：项目建立与任务／交付→合同／采购／报销申请与审批→实际费用→实际收付款与核销→合同财务归属／退款／核对→完成或终止／结项。审批不等于费用发生，费用不等于银行付款，合同归属不产生第二份资金。

The business flow separates approval, actual expense, cash, allocation and contractual reconciliation. This freeze adds the complete shared mobile contract finance center and reviewed foreign-refund principal attribution. A780 HKD payment and100 USD refund retain independent cash and principal units; partial release and source reversal preserve immutable history.

## 部署与访问 / Deployment and access

1. 从上方产物分支下载服务器包、`SHA256SUMS.txt` 与 `release-manifest.json`，先核对文件SHA-256。
2. 在可信源码中执行 `python3 -B app/deploy/release/bundle.py inspect --archive PACKAGE.tar.gz --manifest-sha256 FULL_MANIFEST_SHA256 --output app/.local/inspection-UNIQUE`；完整清单SHA取release-manifest，区别于压缩包自身SHA。
3. 按[原生部署运行手册](../../operations/03-native-release-runbook.md)准备独立Linux身份、持久PG、HTTPS及私有配置，再暂存、正常升级和激活。包内没有数据库密码、OSS／R2、模型、TG、平台密钥或Android私钥。
4. 员工从Web登录并完成MFA；按当前角色进入项目、文件、OA、合同、财务和状态。小程序／Android通过同一API和权限体系访问。
5. 秘书／八项Skill、TG、OSS＋R2必须接入真实配置后分别验收；运行器代码和服务模板的存在不表示这些服务已经在线。

Verify artifact hashes, inspect with trusted release tools, then follow the native runbook for target identities, persistent PostgreSQL, HTTPS and private configuration. Staff authenticate through Web or a configured client using the same API and authority. No production activation, real cloud/model/TG/platform connection or credential is bundled.

两小程序和Android开发产物使用本地API构建配置，供冻结留档和本地联调；它们不会自动指向贵司服务器。部署后须设置真实HTTPS API、真实小程序AppID／合法域名并重新构建。APK仅使用开发签名，未附带签名私钥。

Miniapp and Android artifacts retain the local API profile for local integration and archive purposes. Set the actual HTTPS API and platform identity, then rebuild for server use. The APK carries a development signature; its private signing key is excluded.

## 已执行与延期 / Executed and deferred

五组实际HTTP／PG业务74条、Web428项、共享移动513项、类型、Web／微信／抖音构建及原生JS基础检查已通过。开发PG99→100保持原历史。Web真实页面完成195 HKD部分解除及持久回执查询。Android包按实际构建／内容记录发布；原生测试缓存状态保留原时间，不冒充重跑。

延期：完整后端／秘书运行时回归重跑、更广四端场景、修正定位器后的移动浏览器重跑／移动视频、真实服务与正式平台／Android／生产验收。移动首次定位器失败和所有早期失败均保留，不计为通过。用户先前移出的真机、网络性能、实网120GB专项、历史迁移／容灾和AI费用控制继续不作为门槛。

See [the exact reduced checks](../../quality/133-contract-finance-verification.md), [source-bound proof](../../quality/evidence/contract-refunds/final-source-check.json) and [all23 remaining packages](../../delivery/05-functional-closure-133.md). A candidate or development APK is not production readiness.

## 后续冻结规则 / Freeze rules

- 不移动或覆盖本标签、原包、清单及原始证据；修改以新提交和新版本交付。
- 后续开发仍同步双语README、相关文档和CHANGELOG；只运行与变更有关的必要检查。
- GitHub发布API的本机已保存账号返回403“account was suspended”；SSH访问正常。因此以Git标签和独立可下载产物分支交付，没有伪称GitHub Releases页面已创建。

The tag and artifacts are immutable. Future changes receive a new version. GitHub’s stored API account is suspended; SSH remains available, so this delivery uses a source tag and downloadable artifact branch instead of claiming a created GitHub Releases page.
