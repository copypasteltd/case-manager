# v0.1.0-rc.4 · 冻结功能候选 / Frozen functional candidate

业务源码145：`294bf0509a7c87f63d9c829d5317fe33fb76a6f3`；数据库结构110。包含采购／报销逐明细文案选择、原文核对及人工采用，当前Android开发包与两小程序均已重新构建。旧三次冻结不移动。当前发版工具／说明在main146，业务标签及便利源码ZIP准确固定145。

The source tag fixes the exact145 business snapshot. All current client artifacts are rebuilt. This is a functional freeze; real integrations and formal production acceptance remain open. Packaging tools and release guides live on main146. The independent artifact branch holds the five sealed assets and machine-readable metadata; this download guide is maintained on main.

## 1. 固定下载 / Fixed downloads

产物提交 / Artifact commit: `590280062262f8099dcca861c5086e7765b87d52`。所有链接固定该提交，不使用可移动分支作为包下载地址。

| 编号 | 下载 / Download | 字节 / Bytes |
| --- | --- | ---: |
| F146-01 | [Linux服务器／Web／秘书运行器 · Linux server/Web/runtime](https://github.com/copypasteltd/case-manager/raw/590280062262f8099dcca861c5086e7765b87d52/docs/releases/v0.1.0-rc.4/case-manager-v0.1.0-rc.4-linux-amd64.tar.gz) | 31346105 |
| F146-02 | [微信＋抖音小程序 · WeChat/Douyin](https://github.com/copypasteltd/case-manager/raw/590280062262f8099dcca861c5086e7765b87d52/docs/releases/v0.1.0-rc.4/case-manager-v0.1.0-rc.4-miniapps.zip) | 1003319 |
| F146-03 | [Android开发包 · DevelopmentAPK](https://github.com/copypasteltd/case-manager/raw/590280062262f8099dcca861c5086e7765b87d52/docs/releases/v0.1.0-rc.4/case-manager-v0.1.0-rc.4-android-debug.apk) | 43928997 |
| F146-04 | [完整应用源码／开发文档 · Complete app source/docs](https://github.com/copypasteltd/case-manager/raw/590280062262f8099dcca861c5086e7765b87d52/docs/releases/v0.1.0-rc.4/case-manager-v0.1.0-rc.4-source.zip) | 10400640 |
| F146-05 | [原始截图／MP4／GIF · Original demo media](https://github.com/copypasteltd/case-manager/raw/590280062262f8099dcca861c5086e7765b87d52/docs/releases/v0.1.0-rc.4/case-manager-v0.1.0-rc.4-media.zip) | 94948853 |

[SHA256SUMS](SHA256SUMS.txt) · [准确清单 / Manifest](release-manifest.json) · [服务器构建 / Server receipt](server-build.json)。先核对下载文件自身SHA-256，再检查服务器内层清单；二者用途不同。Inner server manifest SHA-256:

`707bcefdb72c4498cf80647de501bcd1665b16809d640b5f0d5568ff3903aef5`

源码ZIP包含全部应用实现、测试、锁文件、110项结构和需求／开发／使用文档；全部docs/media另包，较大的历史docs/quality/evidence保留完整Git标签。每项打包源码和媒体与原Git blob字节对应；两个ZIP可解压至同一个新目录。完整仓库／历史证明使用：

```sh
git clone --branch v0.1.0-rc.4 git@github.com:copypasteltd/case-manager.git
```

All application source is included. Only docs/media and historical docs/quality/evidence are omitted from the compact source ZIP: media has its own complete ZIP, and all historical evidence remains in the frozen Git tag. Secrets, databases, installed dependencies, tools, caches and signing private keys are excluded. No virtualized environment or device is used.

## 2. 使用及部署 / Usage and deployment

1. 核对文件摘要和可信内层清单。源码／媒体解压到新目录，不覆盖现有开发目录。
2. 使用可信源码检查服务器包：`python3 -B app/deploy/release/bundle.py inspect --archive PACKAGE.tar.gz --manifest-sha256 FULL_INNER_MANIFEST_SHA256 --output app/.local/inspection-UNIQUE`。
3. 按[原生部署手册](../../operations/03-native-release-runbook.md)配置目标Linux、独立身份、持久PostgreSQL、HTTPS及私有配置；暂存、正常结构升级、激活、核对就绪。全新安装应用1–110；既有开发PG100需正常应用101–110。本轮没有升级或重启既有开发服务。
4. Web登录／MFA后按角色使用项目／任务／交付、合同／采购／报销审批、实际费用／现金／核销／退款／订阅、文件／版本／博客及服务状态工作台。审批不生成现金，文案建议需人工明确采用。
5. 小程序／APK当前为明确本地API `http://127.0.0.1:18080`，正式连接公司服务器前设置真实HTTPS API、AppID／合法域名并重建；APK当前为开发签名，正式签名与发布单独完成。
6. OSS＋R2、Codex、原生Linux隔离、TG、监控及平台身份分别配置并接入；八项Skill仍draft，不能据本地构建声称实际模型／云端已可用。

Verify the asset hashes, inspect the server's trusted inner manifest, provision native Linux/PostgreSQL/HTTPS/private identities, apply ordinary upgrades and activate using the native runbook. Employees use current role-bound business actions; approvals, expense facts and actual cash stay separate. Configure real HTTPS/platform identities and rebuild mobile packages before server use. Formal Android signing and real integrations remain outstanding.

[中文完整指南](../../README.zh-CN.md) · [English guide](../../README.en.md) · [本次方案与操作](https://github.com/copypasteltd/case-manager/blob/main/docs/development/146-frozen-release-delivery.md)

## 3. 检查与延期 / Checks and deferrals

本轮服务器792项包文件、110项结构，两小程序151项输出，当前APK10项内容检查和183个实际源码映射模块一致。三组原生HTTP／耐久PG：45子场景＋3组汇总，共48条通过，无失败／跳过。首个服务器封存因Node版本读取不一致被拒，已保留失败并以Node22.23.4重建；最终每项实际服务器输入均与冻结145对应，发版脚本／文档待提交造成的dirty标志如实保留。

Android原生任务为 `> Task :app:testDebugUnitTest UP-TO-DATE`；28项XML为缓存结果，本轮没有新执行，单独JVM未重跑。145选定原生32条／Web11／共享移动11／运行器57及真实Web媒体、136完整Web452／共享移动537、121完整后端回归按原出处复用，未新增录制。精选HTTP检查会覆盖部分相同业务，但原证明文件保持原字节，不将原全部测试重新计作本轮执行。

Deferred: full regression, extra UI journeys/recordings, existing development PG upgrade, real Linux/CI/HTTPS/production PG/cloud/model/TG/platform acceptance and formal Android release. Excluded by the user: devices, network performance/live120GB special, historical migration/disaster recovery and AI fee control. AI authorization/isolation/audit/resource limits, ordinary file recovery, persistent PG and OSS＋R2 remain required. All23 work packages and eight draft Skills remain open.

[准确核验／延期](https://github.com/copypasteltd/case-manager/blob/main/docs/quality/146-frozen-release-verification.md) · [本轮与全部23项编号](https://github.com/copypasteltd/case-manager/blob/main/docs/delivery/05-functional-closure-146.md)

GitHub Releases API此前因现有凭据账号停用返回403，本轮未重试、未创建Releases网页；源码标签和上述固定产物通过已授权SSH交付。原始拒绝见[144记录](../../quality/evidence/release-candidate-144/github-release-api.json)。The webpage limitation is separate from source-tag and artifact-branch delivery. No production deployment is claimed.
