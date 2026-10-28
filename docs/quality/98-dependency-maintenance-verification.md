# 增量98：依赖维护验证 / Dependency maintenance verification

[方案与复现](../development/98-dependency-maintenance.md) · [当前基线](current-functional-baseline.json) · [完整证据目录](evidence/dependency-maintenance) · [媒体目录](../media/README.md)

本轮调整依赖图并重新验证受影响的客户端；界面与 Go 实现保留原发布字节。下表区分新执行、Gradle缓存、历史证据及尚未具备条件的外部验收，不以旧采集替代当前构建。

| 范围 / Scope | 准确状态 / Exact status | 证据 / Evidence |
| --- | --- | --- |
| 当前完整依赖扫描 / Complete audit | 原97锁本次新扫描91项→98锁80项；high20→13；仍未通过生产安全门槛 | [扫描摘要](evidence/dependency-maintenance/dependency-audit.json)、[原始后扫描](evidence/dependency-maintenance/audit-after.json) |
| 真实调用的安全与兼容 / Actual consumer checks | 新执行14项依赖测试、2项原生JS测试；不做无界资源耗尽载荷 | [检查](evidence/dependency-maintenance/security-checks.json)、[完整依赖测试日志](evidence/dependency-maintenance/security-tests.log) |
| 客户端 / Clients | 新执行类型检查、Web135／移动248／运行时51项测试；Web／运行时构建及两小程序各20页 | [8条命令及输入摘要](evidence/dependency-maintenance/client-checks.json)、[移动构建](evidence/dependency-maintenance/mobile-build.json) |
| Android | 新执行Metro打包；Gradle assemble为UP-TO-DATE，APK与97字节相同；18项包检查新通过 | [构建](evidence/dependency-maintenance/build.json)、[包检查](evidence/dependency-maintenance/apk-inspection.json) |
| JVM26 | 以实际Gradle任务及保留XML说明新执行或缓存；不算设备测试 | [任务状态](evidence/dependency-maintenance/android-test-scope.json) |
| 后端 / Backend | 复用原97的189顶层／1183结果、10批；完整Go摘要不变，本次不称新执行 | [原始结果](evidence/miniapp-identity-backend/result.json)、[复用清单](evidence/dependency-maintenance/reused-evidence.json) |
| Linux | 复用原97相同Go源码的交叉编译；从未作为Linux执行或部署验收 | [原交叉编译](evidence/miniapp-identity/linux-build.json) |
| 页面／媒体 / UI and media | 复用原97 PNG／35 MP4／35 GIF，原发布对象与文件字节保持；本次没有新浏览器采集 | [逐文件来源](evidence/dependency-maintenance/reused-evidence.json) |
| 日常开发环境 / Daily development | 新只读核查：原生API及两个worker归属有效、迁移78、零隔离库、所有外部接入关闭 | [环境核查](evidence/dependency-maintenance/development-project-resources.json) |
| 正式验收 / External acceptance | 真实平台／设备／云／模型／TG、120GB／2TB、生产安全与恢复均开放；8项Skill仍draft | [GAP-01—28](../delivery/03-goal-gap-assessment.md) |

This increment freshly executes dependency, client and Android checks. Backend, Linux and UI evidence is explicitly retained from97 on unchanged sources; retained media is not a new browser or device run. The source gate verifies both categories and keeps `full_goal_complete=false` and `production_ready=false`.

更新前的9份入口／台账／基线快照见[原发布来源](evidence/dependency-maintenance/publication-before.json)。锁图、当前完整审计及全部80项开放记录保留原始数据；旧86项与当前91项来自不同公告时点，不将公告变化计作修复。

首次新增CLI检查因读取上游字符串形式的`bin`配置时使用对象形式而失败，原日志和检查源码保留在[该次尝试](evidence/dependency-maintenance/attempts/02-security-test-cli-path)。修正测试定位后14项通过；该失败不被覆盖，也不被称为业务漏洞。

源检查入口为 `python app/scripts/verify_dependency_maintenance_sources.py`；仓库发布检查继续使用 `python app/scripts/verify_repository_publication.py`，提交前分别核对候选和暂存字节。所有原有截图、录像及GIF保留，浏览器未在本次启动。

## 当前 Android 产物 / Current Android artifact

APK: 42945032 bytes; SHA-256 `7dfd5fb4ddcb3c4c4c479ea1f7e87836d9a689ff8f86a936dc17cb0eb3cb8544`. Development profile; APK remains ignored.18 package checks pass. JVM26: Gradle cache/UP-TO-DATE reused, not freshly executed. Gradle assemble is UP-TO-DATE; the APK is byte-identical to published97 after the fresh Metro/current-lock build. See [artifact scope](evidence/dependency-maintenance/artifact-summary.json). No device or production release.

首次仓库检查25项中仅原生Gradle原始日志的尾空白不符合代码／指南格式检查；原失败摘要见[该次检查](evidence/dependency-maintenance/attempts/03-publication-whitespace/repository-check.json)。原始日志不作清洗，保持与Gradle记录相同SHA-256。`publication_files`中的代码／指南格式范围排除本轮机器原始`.log`；所有日志仍提交Git，继续接受全仓库隐私／路径扫描和来源摘要验证，不减去任何功能或安全检查。

The initial publication preflight rejected trailing whitespace in the original Gradle log. Its failure is retained. Raw logs keep their exact bytes and remain selected Git content, privacy-scanned and source-bound; the code/guide formatting list excludes machine logs.
