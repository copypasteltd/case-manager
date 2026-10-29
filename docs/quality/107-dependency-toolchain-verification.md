# 增量107：工具链验证 / Toolchain verification

[实现与使用](../development/107-dependency-toolchain.md) · [完整待办](../delivery/03-goal-gap-assessment.md) · [验收适用性](04-current-acceptance-applicability.md)

当前源码、实际依赖调用、客户端和Android本地检查通过。完整审计70→63个包级记录，high6→0；剩余0 high／56 moderate／7 low全部开放。当前23项完整工作包（14 P0／9 P1）、5项移出不变，完整GAP-26、GAP-28及Goal未完成。

| 编号 | 范围 | 已核对结果与证据 |
|---|---|---|
| V-01 | 原生Node | 项目专用22.23.4／npm10.9.9，官方ZIP校验和一致；要求Node >=22.15.0 <23。[工具记录](evidence/dependency-toolchain/native-node.json)。 |
| V-02 | 标准安装 | 最终锁a30b6e88…的完整npm ci通过，2137个包正常安装；生命周期脚本开启，未用force／legacy-peer／omit。安装后重新核对有效配置。[安装](evidence/dependency-toolchain/clean-install.json)／[配置](evidence/dependency-toolchain/install-config.json)。 |
| V-03 | 实际依赖调用 | 40项依赖调用、2项原生JS及实际npm ls全部通过，无跳过／取消；覆盖MiniPlugin同步模板、WXSS、真实Webpack服务、socket转发、边界与关闭。[结果](evidence/dependency-toolchain/security-checks.json)。 |
| V-04 | 客户端 | 8项命令全部通过：类型检查、Web172／共享移动273／运行时55，Web／运行时构建及微信／抖音各20页；完整输入未变化。格式转换未另行重跑测试。[命令](evidence/dependency-toolchain/client-checks.json)。 |
| V-05 | Android | 当前源码下JS与原生构建通过，APK包含当前冻结JS包；18项静态检查通过。4份XML共26条JVM结果对应实际UP-TO-DATE标记，gradle_cached=true，不记为本轮重新执行。[构建](evidence/dependency-toolchain/build.json)／[APK检查](evidence/dependency-toolchain/apk-inspection.json)／[JVM](evidence/dependency-toolchain/android-test-scope.json)。 |
| V-06 | Taro H5 watch | 5项服务检查及3项原生Edge检查通过；实际watch编译无错误，空登录页读取／重载、Host／跨域限制通过；浏览器、上下文、进程及端口全部释放。[服务](evidence/dependency-toolchain/h5-server-check.json)／[浏览器](evidence/dependency-toolchain/h5-client-check.json)。 |
| V-07 | 完整审计 | 同次旧锁70→当前锁63，移出7个包级记录，无新增告警包；7个包含直接公告对象、56个包仅传播依赖告警，不等于独立CVE数。安全门继续开放。[审计](evidence/dependency-toolchain/dependency-audit.json)／[全部剩余项](evidence/dependency-toolchain/remaining-findings.json)。 |
| V-08 | 保留证据 | 121 PNG／45 MP4／45 GIF共211份媒体保持106原始字节，7份原始记录保留。未改后端224个顶层／1355条结果、2个Linux交叉构建沿用原执行范围。[复用清单](evidence/dependency-toolchain/reused-evidence.json)。 |
| V-09 | 本地持久环境 | 只读检查通过：schema83、10项基础探测、0临时数据库、0激活运行配置、外部集成关闭。[环境](evidence/dependency-toolchain/development-project-resources.json)。 |
| V-10 | 验收适用性 | 原30项AC：18保留／8部分保留／4移出；原72项TC：56保留／14部分保留／2移出。12项一致性检查通过；分类执行业务用例数与通过数均为0。[分类检查](evidence/dependency-toolchain/acceptance-applicability-check.json)。 |
| V-11 | 最终源码 | 28组来源检查通过，包含最终锁、完整输入、产物、旧证据与失败归档；full_goal_complete=false、production_ready=false。[来源检查](evidence/dependency-toolchain/final-source-check.json)。 |
| V-12 | 当前范围与发布 | 当前范围12项检查通过，保留原28个GAP编号，23项开放／5项移出。[范围检查](evidence/dependency-toolchain/scope-check.json)。发布文件结果另存同目录repository-check.json；提交前执行verify_repository_publication.py --staged核对实际暂存字节。 |

以下失败和未完成尝试均保留其原始结果，未改写为通过；完整输入或来源差异归档由最终源码检查核对。

| 尝试 | 原始结果及后续处理 |
|---|---|
| [01](evidence/dependency-toolchain/attempts/01-initial-consumers/result.json)／[02](evidence/dependency-toolchain/attempts/02-install-deadline/result.json) | 首次9项中7通过／2失败：旧嵌套PostCSS7、关闭后读取已被清空的监听器字段。正常解析生成新锁，关闭检查保留原监听器句柄。第二次安装触及900秒期限，不能视为安装通过；最终完整ci另有记录。 |
| [03](evidence/dependency-toolchain/attempts/03-security-deadline/result.json)／[04](evidence/dependency-toolchain/attempts/04-security-cold-start/result.json)／[05](evidence/dependency-toolchain/attempts/05-server-and-tree-deadlines/result.json)／[06](evidence/dependency-toolchain/attempts/06-optional-peer-tree/result.json) | 保留中断套件、39通过／1取消、树检查超时及40＋2通过但peer树无效的记录。验证文件顺序执行，完整套件期限900秒，服务／树检查技术期限300秒；压缩器自身15秒限制保留。同版本Taro React插件在根明确固定，真实peer树另行通过。 |
| [07](evidence/dependency-toolchain/attempts/07-native-log-encoding/result.json) | 原生构建已通过，但证据归档程序把CP936日志当UTF-8读取而失败。保留旧程序与原始日志字节，现只匹配ASCII任务标记，不改写日志。 |
| [08](evidence/dependency-toolchain/attempts/08-watch-development-peer/result.json)／[09](evidence/dependency-toolchain/attempts/09-browser-executable/result.json)／[10](evidence/dependency-toolchain/attempts/10-watch-socket-and-input/result.json) | 真实watch发现缺少React Refresh peer；首次浏览器尝试缺少捆绑可执行文件；随后发现socket入口缺失与重复placeholder定位。根固定Refresh0.6.3，沿用原生Edge，socket仅转发维护版上游live client，精确定位native input，并要求真实编译无错误。此前候选的通过记录和失败来源全部保留。 |
| [11](evidence/dependency-toolchain/attempts/11-post-ci-timeouts/result.json) | 最终干净安装后首次40项中38通过／2失败：glob子进程和合成HTTP请求触及原10秒期限；2项原生JS及依赖树通过。完整24份输入、日志与结果归档；随后相同输入、相同锁、相同断言和期限下40＋2全部通过。 |

H5原始日志保留1项上游视频模块webpackExports magic-comment警告，以及重载／关闭期间的提前关闭流日志。实际编译无错误、页面读取／重载通过；该检查范围不包含完整HMR编辑或业务旅程。检查使用空登录页与隔离合成资料，未录入账号口令或真实业务内容。本轮没有新的业务页面或媒体采集。

本轮未启动Docker、WSL、虚拟机、模拟器或真机。真实云、模型、TG、平台发布和生产验收继续开放；AI仅移出费用控制，秘书功能、授权、审计、隔离与技术资源限制保留。正常PG结构升级、OSS＋R2复制及普通文件恢复仍保留。

Current local checks pass: a standard clean install, 40 checks of dependency calls, 2 native JS checks, 8 client commands, an Android development build with 18 APK checks, and 5 server plus 3 native Edge checks. The 26 JVM results came from Gradle's cache. All 28 source checks and 12 scope checks pass. All 30 AC and 72 TC IDs retain explicit boundaries, verified by 12 consistency checks; classification executes no business acceptance cases.

All 63 remaining audit package findings stay open. Failed attempts preserve original inputs and outcomes. The unchanged 106 backend, UI and 211 media files retain their historical scope. No Linux execution, live integration, device or production acceptance is inferred from these local checks. The full 23 work packages remain active; only AI cost controls are excluded from the AI requirements.
