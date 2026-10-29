# 业务工作台验证 / Business workbench verification

增量114的核验对象是当前工作台源码及本轮构建产物。整体目标尚未完成，23个工作包仍active，5项移出。[设计与使用](../development/114-business-workbench.md) · [全部未完成项](../delivery/03-goal-gap-assessment.md)。

| 编号 | 检查 | 实际结果与边界 |
|---|---|---|
| BW-V01 | 完整后端回归 | 最终源码全部235个顶层测试／1435条结果，12个原生批次覆盖全部测试；无跳过、替换或先前源码批次复用。第9批在Go测试前的环境查询按原30秒时限超时，失败原样保留；续跑校验同一源码与前8个完整批次的日志摘要后，重试第9批并执行余批。结果记录包含嵌套子用例，不等于1435个不同业务场景。 |
| BW-V02 | 新业务接口 | 10组真实API／PG检查：严格查询、空可读范围、有效审批计数及队列一致性、香港日期和30天边界、已生成订阅预测与待核对口径、只读无副作用、已完成步骤消失、真实角色撤销、运行会话／TG／陈旧授权拒绝、真实撤销会话／取消读取及四个人类渠道。Web负责事项另由实际MFA浏览器覆盖；部分原生跨渠道断言使用无负责事项的真实普通成员会话。 |
| BW-V03 | 共享客户端边界 | Web257、移动351、运行时55。Web和移动分别执行同一19个新共享用例，不算38个不同场景。检查日期、完整3分组、完整计数与5条样本、固定引用、状态、排序、文字及路由边界。 |
| BW-V04 | 八项客户端命令 | 类型检查、三组测试、Web／运行时构建、微信／抖音构建均重新执行，前后完整输入摘要一致。双小程序各20页；构建成功不代表真实平台提交／发布。 |
| BW-V05 | 实际页面 | Web及Taro H5各6组：真实登录与MFA、准确数量与有限样本、认证后录制、三类实际详情链接、刷新与窄屏、其他员工隔离、实际角色撤销后旧数量／标题清除。无API响应替换、组件状态注入或用数据库业务状态伪造代替业务页面操作。撤权通过仅测试编译的受控fixture入口实施实际角色变更。 |
| BW-V06 | Android | 当前源码生成开发bundle与APK，18项常规包检查＋3项业务内容静态检查。26条JVM来自Gradle缓存，四份XML保留实际时间和任务状态。没有设备或模拟器执行，正式签名／发布仍GAP-16。 |
| BW-V07 | 媒体 | 6 PNG、2静音MP4、2截图轮播GIF来自实际认证后合成页面；视频帧来源和实际时长、GIF截图来源与每张2秒间隔、目视采样范围单独记录。266份旧媒体按已发布字节保留。 |
| BW-V08 | 原生持久化环境 | 独立临时PostgreSQL保持fsync、同步提交及完整页写入开启；检查结束0临时测试库并关闭自有集群。开发PG与用户无关进程保留；未改变用例时限或耐久配置。 |
| BW-V09 | 开发服务 | 仅替换核对归属的本地API及两项Worker。schema仍87，没有迁移新增或重跑；既有历史AI费用／用量四表的行数和内容摘要保留。真实云、模型、TG及平台凭据继续关闭。 |
| BW-V10 | API与Linux | 327路径／403操作／721模型；原326路径／717模型语义保持。两个当前Linux amd64 ELF交叉构建通过；没有目标Linux执行、内核隔离或生产部署声明。 |
| BW-V11 | 范围与验收适用性 | 保持23 active（14 P0／9 P1）、5 excluded、8项draft Skill。原30 AC／72 TC适用性校验不执行业务案例，正式业务案例通过数仍为0；GAP-28保持开放。 |
| BW-V12 | 发布 | 当前功能源码绑定、范围、适用性、文档链接、媒体、候选及实际暂存区检查按各自结果记录；提交和普通推送完成后以Git提交为准。 |

The final source passes the complete native backend run, all eight client commands and actual Web/H5 journeys. These are local source, artifact and controlled business checks. They do not establish production readiness, real cloud/model/TG integration, platform approval or company rule adoption. Cached JVM outputs and retained dependency/media evidence preserve their original execution scope.

早期尝试单独保留，均不改记为最终通过：首次初始化在没有MFA注册挑战时调用注册辅助器而停止；Web复合类型项目漏列共享文件，随后补齐；浏览器初次准备发现合成Alice没有录制所需MFA凭据，修正为临时授予注册角色、完成真实MFA注册后撤销该临时角色。该测试源码修正使先前回归结果失效，最终源码从第1批重新覆盖全量。移动浏览器首轮把普通刷新按钮误写为带行尾箭头的名称，等待响应超时；已保留失败及此前Web通过记录，改为实际按钮名并让点击与响应等待共同处理异常，随后两端均重新检查。该修正只改变检查脚本，不沿用此前Web通过记录作为最终源码通过。最终回归第9批的测试前原生PG环境查询曾超时，未运行的Go案例未计为通过；保留失败后按原时限续跑，核对同源码已完成批次的精确日志摘要。应用业务状态没有通过直接SQL伪造。

初版Web／H5功能检查已经通过，但目视采样发现滚动快于采样间隔，演示几乎静止。该轮页面结果和原始帧均保留为被替代记录；仅在检查脚本增加实际页面阅读停留、固定窄屏录制与真实刷新，再重新执行两端检查与录制。应用源码、业务流程及后端回归输入不因此变化。

补录一轮在登录、录制和三类详情链接通过后，返回首页等待标题超时，失败截图为空白；当时没有记录到准确导航位置，原因未确认，不归因为已修复的业务错误。保留该轮后增加失败栈和不含业务正文的页面结构诊断。曾尝试整页重载来重置首页，但这会清除H5有意只保存在内存中的会话，诊断确认正确跳回登录；该测试策略已改为实际浏览器“返回”，保留原会话并等待首页显示。普通详情仍通过实际点击访问，应用会话机制没有改动。

一次重试在H5准备构建的原720秒时限内未完成，浏览器和原生fixture均未启动，自有构建及代理已关闭；该结果不计业务失败或通过，单独保存并重试。没有修改业务用例时限或数据库耐久配置。

Web复核曾超过原150秒环境准备等待，fixture在退出等待期间才写出，旧检查器未重新读取其释放凭据，停止子进程后留下一个断开的合成库。已保留私有fixture、精确日志和完整合成库dump，核对独立PG归属、唯一库名、项目ID及零连接后，只清理该库；另有明确清理记录。检查器现于结束阶段重新识别自有子进程的迟到fixture并请求释放。仅将准备等待调整为300秒；既有150秒结束等待、12分钟fixture持有、业务检查时限和数据库耐久配置均保持。此前移动端通过记录保留为旧检查器来源，两端随后重新检查。

| 记录 | 链接 |
|---|---|
| 最终源码绑定 | [final-source-check.json](evidence/workbench/final-source-check.json) |
| 后端全量与原始日志 | [结果](evidence/workbench-backend/result.json)、[批次](evidence/workbench-backend/batch-progress.json)、[私有日志保全](evidence/workbench-backend/raw-log-archive.json) |
| 客户端命令 | [client-checks.json](evidence/workbench/client-checks.json) |
| 实际页面与资源释放 | [Web](evidence/workbench-web/result.json)、[Taro H5](evidence/workbench-mobile/result.json) |
| Android范围 | [构建](evidence/workbench/build.json)、[包检查](evidence/workbench/apk-inspection.json)、[业务内容](evidence/workbench/android-feature-content.json)、[JVM缓存范围](evidence/workbench/android-test-scope.json) |
| 媒体与目视核对 | [编码及来源](evidence/workbench/media.json)、[目视采样](evidence/workbench/visual-review.json) |
| 原生PG | [启动与耐久](evidence/workbench/native-cluster.json)、[关闭与零测试库](evidence/workbench/native-cluster-closed.json) |
| 开发服务 | [更新](evidence/workbench/development-update.json)、[只读核对](evidence/workbench/development-project-resources.json) |
| Linux | [交叉构建](evidence/workbench/linux-build.json) |
| 复用证据 | [266份媒体及原依赖结果](evidence/workbench/reused-evidence.json) |
| 早期尝试 | [MFA挑战](evidence/workbench/attempts/01-existing-mfa-fixture/attempt.json)、[浏览器准备及失效源码](evidence/workbench/attempts/02-browser-mfa-fixture/source.json)、[类型包含配置](evidence/workbench/attempts/03-web-composite-includes/attempt.json)、[移动刷新定位](evidence/workbench/attempts/04-mobile-refresh-selector/attempt.json)、[测试前环境超时](evidence/workbench/attempts/05-native-attestation-timeout/attempt.json) |
| 初版录屏替代记录 | [目视发现、历史通过及精确来源核对](evidence/workbench/attempts/06-recording-readable-holds/attempt.json) |
| 补录导航等待失败 | [失败、清理与辅助脚本来源核对](evidence/workbench/attempts/07-mobile-home-navigation/attempt.json) |
| H5构建准备超时 | [未启动浏览器／fixture的准备失败](evidence/workbench/attempts/08-h5-build-timeout/attempt.json) |
| 整页重载与内存会话 | [已确认的检查策略问题](evidence/workbench/attempts/09-h5-memory-session-reload/attempt.json) |
| Web迟到环境与受控清理 | [准备失败与私有快照](evidence/workbench/attempts/10-web-late-fixture/attempt.json)、[唯一合成库清理](evidence/workbench/attempts/10-web-late-fixture/controlled-cleanup.json) |

重复检查入口见 `app/scripts/` 下的 `workbench_native_cluster.py`、`verify_workbench_backend.py`、`verify_workbench_clients.py`、`run_workbench_browser.py`、`build_android.py`、`build_workbench_linux.py` 及对应摘要／包／最终来源检查器。测试数据库由本轮独立原生所有权记录管理；不要对已关闭的历史记录直接复用活动所有权。准确私有日志、数据库、凭据、原始帧、工具、APK及源码试跑归档不入Git。

Only AI fee controls are excluded. Secretary execution, all eight Skills, TG, isolation, authorization, audit and technical resource limits remain required. Device validation, network performance/actual-network 120 GB tests and historical migration/disaster recovery remain excluded without being counted as passed.
