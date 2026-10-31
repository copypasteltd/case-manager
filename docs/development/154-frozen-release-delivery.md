# 增量154：同版冻结交付 / Matching frozen delivery

`v0.1.0-rc.8` 固定业务153源码 `160d78f8032be21c9eab72fc28415fda598fb7df`，正常数据库结构升级目标为113。此轮停止业务扩展，将最新个人TG提醒及既有项目、OA、财务、文件、秘书、状态功能放入同版服务器／Web／运行器、微信、抖音和Android开发产物，另附完整应用源码、文档和演示媒体。

This candidate fixes application source153 at the commit above and schema113. Packaging helpers, checks and this handover are maintained separately on main. No business implementation or earlier freeze is modified by154. All seven earlier source tags and artifact branches remain immutable.

## 交付约束 / Delivery contract

| 编号 / No. | 内容 / Content | 边界 / Boundary |
| --- | --- | --- |
| 1 | Linux amd64服务器、Web、秘书运行器 | 本机原生交叉构建；没有在目标Linux执行或部署 |
| 2 | 微信、抖音构建包 | 本地API配置；实际平台身份、域名、审核发布待配置 |
| 3 | Android APK | 从153重新构建的开发包；不等于正式签名、升级及商店发布 |
| 4 | 完整应用源码与规格／使用文档 | 固定153 Git字节；历史核验证据保留在源码标签，媒体另包 |
| 5 | 全部公开截图、无声MP4、GIF | 固定153已有媒体；154不改变页面、不重新录制 |
| 6 | 摘要和来源清单 | 外层SHA256、服务器内层清单、源提交、结构版本及独立产物分支 |

Source153 and all application blobs are reconstructed exactly from Git. The convenience source ZIP excludes historical evidence and media; the annotated source tag retains every tracked evidence file and the separate media ZIP retains every tracked public media blob. Private databases, credentials, dependency installations and local caches are excluded. The main branch contains the independent154 packaging implementation and handover.

## 获取与使用 / Obtain and use

1. 从[固定下载入口](../releases/v0.1.0-rc.8/README.md)选择五项产物，先核对`SHA256SUMS.txt`、固定源提交及结构版本，再核对服务器包的内层清单。不要将不同候选版的客户端、后端或配置混用。
2. 按[原生发布手册](119-native-production-release.md)准备实际Linux、持久PostgreSQL、HTTPS和身份配置。现有本机PG100与API进程原样保留；实际部署按正常结构升级流程到113，不能用重建数据库代替升级。
3. OSS作为主存储、R2作为副本的配置和后台工作器随服务器包交付。正式文件落在对象存储，服务器只保留受控临时处理／缓存。真实桶、身份、CORS、签名、复制与恢复仍需后续实际环境验收。
4. 小程序和APK使用本地开发API配置。连接实际服务器前设置真实HTTPS入口、平台AppID／域名并重新构建；Android按公司正式签名及升级流程出包，不将开发APK作为正式包。
5. 员工按当前角色使用项目、合同／采购／报销、付款／月开支、文件版本与恢复、站内通知及服务状态。OA批准、付款、交付验收等以各自正式回执为准；聊天回复、已读与排队不替代业务完成。
6. 本人完成TG双向绑定后，在“通知”选择Bot及通知类型，进行近期MFA确认后主动开启。系统只发送固定登录提示，合同名称、金额、文件与正文继续在系统内查阅。结果未知时由本人核对准确消息或明确重试／终止；停用保留站内通知和历史。需启动独立个人通知工作器，详见[153说明](153-personal-telegram-notifications.md)。
7. 秘书仍需真实Codex、Linux隔离环境、八项Skill正式发布和TG配置。安装由服务器侧受控完成；现有运行配置查看／停用入口不允许绕过安装与版本证明。

Verify outer checksums and the trusted inner server manifest before use. Prepare native Linux, persistent PG and HTTPS, then apply ordinary schema upgrades to113. Configure actual object-store and platform identities and rebuild clients for that API. Personal TG notices require two-way binding, explicit owner opt-in, recent MFA and the dedicated notification worker; only a fixed sign-in prompt leaves the system. Unknown outcomes require human reconciliation. Real providers and formal production acceptance remain pending the material the user will provide later.

## 构建与证据 / Build and evidence

六个独立脚本负责四端构建、服务器构建、产物检查、封包、准备Git引用及最终核验；不会覆写152及更早脚本或证据。服务器输入逐文件与153的Git对象比对；两小程序检查实际打包内容；Android检查实际APK及Metro映射中的当前业务源码，包含153新增的两个TG通知模块。

153的原生HTTP／PG十四个子场景加一条汇总、传输适配器十八条通过记录、Web与共享移动各四项、三项实际Web交互及PNG／MP4／GIF按原字节保留，154不重新执行业务回归。运行器58项来自151，完整后端来自121，完整客户端来自136。Android的JVM任务缓存／新执行状态及XML原时间单列，不将缓存结果表述成新测试。

All fresh output construction and archive inspections are recorded separately from retained business proof. Source completeness, expanded archive contents and private-material checks are required for this handover; broad business regression and additional UI journeys are deferred for speed. No device, emulator, virtualization, network-performance special, historical migration/DR or AI fee policy is introduced.

## 发布关系与未完成项 / Publication and remaining work

源码注释标签`v0.1.0-rc.8`指向153；五个固定产物在独立`codex/release-v0.1.0-rc.8`分支，主分支保存154封包实现和说明。通过已有授权SSH推送并读取远端引用核对；不强推。既有GitHub Releases API账号403／suspended，未用相同凭据重复尝试，不声称已建立Releases网页。

全部23个整体工作包仍开放（14 P0／9 P1），八项Skill仍draft，5项移出范围不算通过。服务器和外部配置按用户答复后续提供。[本轮与全部剩余编号表](../delivery/05-functional-closure-154.md)区分当前可下载候选包与实际投产验收。正式发布所需签名、目标环境和公司验收没有因减少测试而取消。

The result is a complete matching internal candidate and full application source delivery, with explicit production boundaries. Real credentials, signing, deployment and company acceptance remain open. [Verification](../quality/154-frozen-release-verification.md) records exactly what was executed and retained.
