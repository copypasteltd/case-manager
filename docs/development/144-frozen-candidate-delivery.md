# 增量144 · 当前主线冻结交付 / Current-main frozen delivery

按用户要求，优先冻结已有主功能，精简重复验证。本轮不扩展业务模块，交付 `v0.1.0-rc.3`，准确固定业务源码 `e1c7a671cc4c7f52888a1e6ed72d7ce1a9c042d3`、数据库结构109。此前首冻rc.1及rc.2的标签、产物和证明保持不变。

This increment packages existing business functionality with selected validation deferred. The new candidate fixes business source143/schema109; packaging tools and delivery records belong to increment144 on main. Earlier frozen candidates remain immutable. A frozen candidate is a reviewable delivery, while production integration and the complete remaining work register stay open.

## 1. 主功能及操作顺序

| 编号 | 已实现的主流程 | 使用逻辑 |
|---|---|---|
| M144-01 | 身份、角色、项目、任务与交付 | 登录／MFA→按当前权限进入项目→任务与里程碑→交付及结项；变更权限后按最新授权重新判断 |
| M144-02 | 合同、采购、报销与审批 | 建立业务草稿→核对资料和版本→提交→逐级审批→履行；实际费用和现金单独登记 |
| M144-03 | 公司财务及订阅 | 批准业务→人工确认实际费用→实际付款／核销→退款、汇兑、费用分摊与报表；订阅逐账期核对 |
| M144-04 | 文件、版本与项目博客 | 上传→内容校验／扫描→明确提交→查看／关联／正文检索；版本恢复、回收站和受控清理保留原记录 |
| M144-05 | 工作台、服务状态与通知 | 查看本人待办、监控事件、存储待办和站内通知→按权限处置→刷新核对持久回执 |
| M144-06 | 秘书与TG业务代码 | 会话→选择资料／Skill→核对固定计划→授权执行→查看结果及审计；真实Codex、Linux隔离、Bot和Skill正式发布仍待实际接入 |

审批不生成银行交易；模型建议不自动成为正式业务记录；文件删除／清理仍遵守独立授权、观察期、保留和引用保护。AI费用控制移出，授权、隔离、审计及技术资源约束保留。

新增入包的138–143包括：已完成但未提交的上传清理、TG出入站组织运维、秘书运行配置停用、项目人员／模板规划目录、完整跨订阅审阅。四端共用Go后端及PostgreSQL权限与业务规则。

## 2. 五项交付

| 编号 | 产物 | 范围 |
|---|---|---|
| F144-01 | Linux amd64服务器包 | 三个Go程序、Web、独立秘书JS运行器／依赖、八项draft Skill、systemd模板及109项结构升级清单 |
| F144-02 | 微信／抖音小程序ZIP | 两个平台分别实际构建，本地API配置；实际AppID、合法域名和发布另行准备 |
| F144-03 | Android开发APK | 当前143业务源码重新打包；实际JS映射、DEX、签名、权限和包内容检查，不启动设备或模拟器 |
| F144-04 | 完整应用源码／开发文档ZIP | 指定提交的全部app源码、测试、迁移、锁文件及开发文档；每项与Git原始字节对应 |
| F144-05 | 演示媒体ZIP | 同一冻结提交的完整docs/media；历史验证证据仍可在完整Git标签查阅，不重复装入便利源码ZIP |

源码和媒体ZIP可解压至同一个新目录；完整仓库及历史证据使用 `git clone --branch v0.1.0-rc.3 git@github.com:copypasteltd/case-manager.git`。本轮新封装工具在main的144提交，业务源码ZIP和标签准确保持143；没有把后来编写的工具声称为较早构建输入。

## 3. 部署和访问

1. 从[下载页](../releases/v0.1.0-rc.3/README.md)取得产物及SHA256SUMS，核对准确文件名与压缩包摘要。
2. 用可信源码中的 `app/deploy/release/bundle.py inspect` 核对服务器包及**内层清单**摘要；内层摘要与压缩包自身摘要用途不同。
3. 按[原生部署手册](../operations/03-native-release-runbook.md)准备目标Linux身份、持久PostgreSQL、HTTPS和私有配置，暂存、正常升级、激活并检查就绪。开发库仍100；新后端需正常应用101–109，全新安装应用1–109。
4. Web作为员工入口，登录／MFA后按角色进入业务。微信／抖音侧重查阅、审批；Android包含原生上传及共享业务页面。
5. 当前移动包明确使用 `http://127.0.0.1:18080` 本地配置。面向公司服务器使用前，设置实际HTTPS API、真实平台身份及合法域名并重建；开发APK不是正式签名发布包。
6. 单独配置及验收OSS＋R2、Codex／隔离、TG、监控和平台身份。没有这些外部身份时，不宣称真实云文件、模型执行或群回复已经交付。

No production deployment or existing development-service restart is performed by packaging. Secrets, database files, installed dependencies, local tools, caches and signing private keys stay outside Git and convenience archives.

## 4. 必要验证和延期

本轮重新构建服务器、Web、秘书运行器、两小程序和开发APK；执行实际产物、完整源码字节、展开私密资料、Git对象／引用检查。只选八组原生HTTP／持久PG主业务检查，不重跑全部后端／运行器／前端回归，也不新增页面录制。各组执行名称和准确条数以[验证记录](../quality/144-frozen-candidate-verification.md)为准。

143的完整跨订阅审阅／运行时56项、136的完整Web452／共享移动537项及各轮实际截图／MP4／GIF按原出处复用；121完整后端／运行器回归继续保留历史身份。Gradle任务缓存状态、原XML时间和准确字节单独记录，缓存结果不写成新测试执行。

延期：完整回归、额外四端页面旅程、开发库升级、真实Linux／云／模型／TG／生产PG／平台接入、正式Android签名发布和当前范围完整验收。真机、网络性能／实网120GB专项、历史数据迁移／容灾、AI费用控制继续按用户要求移出，不计测试通过。禁止Docker、WSL、虚拟机和模拟器。

仍23个工作包（14 P0／9 P1），八项Skill仍draft。GitHub Releases页面不作为本轮已创建内容；实际交付使用新源码标签和独立产物分支，拒绝覆盖旧引用、强推或重置用户索引。

[全部23项待办](../delivery/05-functional-closure-144.md) · [验证与延期](../quality/144-frozen-candidate-verification.md) · [下载](../releases/v0.1.0-rc.3/README.md)
