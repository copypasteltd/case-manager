# v0.1.0-rc.5 · 冻结功能候选 / Frozen functional candidate

业务源码147：`4155ce9c19304b8c65f5636e09707a5eba1e21cd`；数据库结构111。四端重新构建并包含组织文件复制台账、详情及人工重试；当前Android开发包补齐上一版未包含147的差异。前四次冻结保持原引用和原字节。148只做封装、核对与交付，不新增业务逻辑或结构，不扩大业务回归。

This freeze includes all business source through147/schema111. Linux/Web/runtime, WeChat, Douyin and the development Android APK are rebuilt. Exact147 business checks and demonstrations are retained without replay. Packaging tools and current delivery guides live on main148; the source tag and compact source ZIP fix147. This is a frozen functional candidate with complete application source; formal production acceptance remains open.

## 1. 固定下载 / Fixed downloads

产物提交 / Artifact commit: `0f7a0c0158c7b86a8301a7f6d6798d6608f1951f`。链接固定提交，不依赖可移动分支。

| 编号 | 下载 / Download | 字节 / Bytes |
| --- | --- | ---: |
| F148-01 | [Linux服务器／Web／秘书运行器 · Linux server/Web/runtime](https://github.com/copypasteltd/case-manager/raw/0f7a0c0158c7b86a8301a7f6d6798d6608f1951f/docs/releases/v0.1.0-rc.5/case-manager-v0.1.0-rc.5-linux-amd64.tar.gz) | 31382266 |
| F148-02 | [微信＋抖音小程序 · WeChat/Douyin](https://github.com/copypasteltd/case-manager/raw/0f7a0c0158c7b86a8301a7f6d6798d6608f1951f/docs/releases/v0.1.0-rc.5/case-manager-v0.1.0-rc.5-miniapps.zip) | 1009897 |
| F148-03 | [Android开发安装包 · Development Android APK](https://github.com/copypasteltd/case-manager/raw/0f7a0c0158c7b86a8301a7f6d6798d6608f1951f/docs/releases/v0.1.0-rc.5/case-manager-v0.1.0-rc.5-android-debug.apk) | 43933472 |
| F148-04 | [完整应用源码／开发文档 · Complete app source/docs](https://github.com/copypasteltd/case-manager/raw/0f7a0c0158c7b86a8301a7f6d6798d6608f1951f/docs/releases/v0.1.0-rc.5/case-manager-v0.1.0-rc.5-source.zip) | 10509233 |
| F148-05 | [原始截图／MP4／GIF · Original demo media](https://github.com/copypasteltd/case-manager/raw/0f7a0c0158c7b86a8301a7f6d6798d6608f1951f/docs/releases/v0.1.0-rc.5/case-manager-v0.1.0-rc.5-media.zip) | 95879523 |

[SHA256SUMS](SHA256SUMS.txt) · [准确清单 / Manifest](release-manifest.json) · [服务器构建 / Server receipt](server-build.json)。先核对下载文件自身SHA-256，再核对服务器内层可信清单；内层清单摘要 / Trusted inner server manifest SHA-256:

`5dad7234bf70bb4f0779e50c3dd5a32dd4a28dfd31df279b7763e42d0c077222`

源码ZIP与媒体ZIP合计重建3289项原Git blob，包括全部应用实现、测试、锁文件、111项结构、需求／开发／使用文档和全部冻结docs/media。历史docs/quality/evidence留在完整Git标签，不在便利源码ZIP；媒体ZIP可与源码ZIP解压到同一个新目录。源码中的既有版本说明保持其历史语境，最新148交付说明在main。完整冻结仓库和历史证明：

```sh
git clone --branch v0.1.0-rc.5 git@github.com:copypasteltd/case-manager.git
```

The compact source ZIP includes all application source and documentation except docs/media and historical docs/quality/evidence. All frozen media is in the separate media ZIP; every packaged source/media blob matches Git147 exactly. Historical evidence remains available through the full Git tag. Private configuration, databases, dependency installations, caches, tools and signing private keys are excluded.

## 2. 使用及部署 / Usage and deployment

1. 下载所需包，核对SHA256SUMS。源码及媒体解压到新目录；完整历史证明使用冻结Git标签。
2. 用可信源码检查服务端归档：`python3 -B app/deploy/release/bundle.py inspect --archive PACKAGE.tar.gz --manifest-sha256 FULL_INNER_MANIFEST_SHA256 --output app/.local/inspection-UNIQUE`，摘要使用上方准确值。
3. 按[原生部署手册](../../operations/03-native-release-runbook.md)准备目标Linux、独立身份、持久PostgreSQL、HTTPS和私有配置，再暂存、正常升级、激活并核对就绪。新安装应用1–111；开发PG100连接本版前需正常升级101–111。本轮没有升级、重启或部署既有开发服务。
4. Web登录与MFA后按角色使用项目／任务／交付，合同／采购／报销，实际费用／付款／退款／订阅，文件／版本／博客和服务状态。审批、费用、现金分别记录；秘书建议须人工明确采用。
5. 获授权的组织复制读取员在Web `/storage-operations`、移动端“设置→复制运维”筛选状态及查看元数据。重试需组织重试权限、普通文件权限、近期MFA和准确版本／依据；旧依据须刷新复核。重试进入原工作器，排队回执不代表云端复制已完成。
6. 当前小程序／APK明确使用本地API `http://127.0.0.1:18080`。连接公司服务器前设置真实HTTPS API、平台AppID／合法域名并重建。APK为开发签名；正式签名、版本与平台发布配置另行完成。
7. OSS＋R2、真实Codex、原生Linux隔离、TG、监控和平台身份需要真实配置与验收。八项Skill仍draft。现有配置未齐，不据封包宣称真实服务已上线。

Verify hashes and the trusted inner manifest, follow the native Linux/PostgreSQL/HTTPS release runbook, apply ordinary migrations, activate and check readiness. Users enter current role-bound business workflows. Organization replica retries require explicit current authority, recent MFA and reviewed version/basis; queue acceptance and successful replication are separate states. Configure real HTTPS/platform identities and rebuild mobile artifacts before server use. Formal signing and actual integrations remain outstanding.

[中文完整指南](../../README.zh-CN.md) · [English guide](../../README.en.md) · [148方案／操作](../../development/148-frozen-release-delivery.md)

## 3. 本轮核对与复用 / Current checks and retained evidence

本轮：服务器1355项准确构建输入、793项包文件／111项结构；两小程序151项输出；Android实际APK10项内容检查、185项选定源码映射内容一致；五包5227项展开成员扫描及3289项源码／媒体重建。全部业务输入与冻结147一致，发版脚本／文档未提交导致的工作区dirty如实披露。

Gradle任务 `> Task :app:testDebugUnitTest UP-TO-DATE`，28项XML为缓存结果，本轮没有新执行，也未单独重跑JVM。147原生13业务子场景＋1未启用浏览器钩子＋1组汇总形成15条pass记录；钩子不计业务通过。147 Web7／共享移动7与真实Web3项检查、三PNG／2.2秒无声操作MP4／6秒截图导览GIF原字节复用；145运行器57、136完整Web452／共享移动537、121完整后端分别按原出处保留。本轮没有执行上述业务测试或重新录制。

Fresh packaging and artifact checks are distinguished from retained business evidence. There is no fresh full regression, business execution, browser capture or device test in148. Linux binaries are cross-built, not run on the target Linux server. DevelopmentPG remains100. Real Linux/CI/PG/cloud/model/TG/platform acceptance, formal Android release and company-rule/full-journey acceptance remain open in23 work packages; eight Skills remain draft.

用户移出：真机、网络性能／实网120GB专项、历史迁移／容灾、AI费用控制。四端、AI授权／隔离／审计／技术资源限制、普通恢复、正常结构升级、持久PG及OSS＋R2继续保留。

[准确检查／延期](../../quality/148-frozen-release-verification.md) · [本轮与全部23项](../../delivery/05-functional-closure-148.md) · [当前范围](../../delivery/04-current-scope-and-acceptance.md)

GitHub Releases API此前因现有凭据账号停用返回403，本轮未重试，也未创建Releases网页；源代码标签与固定产物已通过授权SSH交付。原始拒绝见[144记录](../../quality/evidence/release-candidate-144/github-release-api.json)。The fixed source tag and five artifact downloads are available independently of the unavailable GitHub Releases webpage. No production deployment is claimed.
