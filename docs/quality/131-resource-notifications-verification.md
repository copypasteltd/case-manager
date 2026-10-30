# 资源提醒与处置流程核验 · 增量131

本轮实现原始签名观测→不可变资源状态事件→实际站内投递→本人待办→真实恢复。以[执行计划](evidence/resource-notifications/verification-plan.json)、各次原始执行记录和[最终源码复核](evidence/resource-notifications/final-source-check.json)为准。选定业务检查使用实际HTTP、原生PostgreSQL、签名桥接和原租约通知Worker；采集端与业务账号为隔离合成数据，不表示目标服务器已安装监控。

| 编号 | 检查 | 结果及适用边界 |
|---|---|---|
| V131-01 | 八组选定HTTP／PostgreSQL套件 | 76条通过记录，零失败／跳过。资源首次采样宽限、真实45秒期限／过期、精确大容量及Go／SQL阈值一致、只读告警、重复与并发调度、真实投递、已读仍待办、恢复后新未读、旧排队事件抑制、转交／归档／停用、新配置、不变历史／前驱／头、分页及撤权；回归原资源、服务状态、通知、重试及两类工作台。没有执行全量后端。 |
| V131-02 | 状态／桥接／通知／Worker核心及采集器编译 | 94条实际测试通过记录，零失败／跳过测试。工作台包另为编译通过且没有单元测试用例，Go的包级skip不冒充测试通过。Windows及Linux/amd64采集器实际编译；Linux产物没有在目标服务器运行。 |
| V131-03 | Web411／共享移动496及类型检查 | 全部通过，新增第六类资源待办、固定服务跳转与严格响应验证；保持所有原客户端单元测试。 |
| V131-04 | 四端当前源码构建 | Web、微信、抖音构建及完整Android开发APK通过；当前APK19项签名／DEX／JS内容检查，四个相关模块与实际Metro映射源码一致。原生JS两项通过。正式签名、升级、真实平台身份和发布继续GAP-16／19。 |
| V131-05 | 原生Android和独立JVM测试结果 | 两个28项任务均为UP-TO-DATE，八份原始XML精确匹配已发布130证据，保留原时间和摘要，不声称本轮重新执行56项测试。见[准确任务与缓存边界](evidence/resource-notifications/native-test-scope.json)。 |
| V131-06 | Web及共享Taro H5各四段实际流程 | 原始只读严重观测产生一项本人资源工作；实际筛选通知、标记已读仍需处理、打开服务；正常观测消除工作并产生新未读投递；核对当时责任／配置／状态历史及SHA-256／Ed25519，返回首页确认零项。业务响应未被替换。 |
| V131-07 | 正常97→98升级及原生开发服务 | 仅首次正常升级实际执行；26类旧业务／签名观测表的行数及内容摘要一致。只替换已核验归属的API、通知和报表进程；PG未重启，云／模型／TG未激活，检查后零隔离测试库。升级后只读检查不冒充再次执行升级。 |
| V131-08 | 六张截图、两段MP4及两份GIF | 合成账号登录／MFA后实际视口，未录入凭据；MP4为实际画面录制，GIF明确为三张实际截图各两秒的导览。可视检查覆盖全部PNG、两段MP4取样及GIF三场景。移动长页只展示当前视口，完整分页／签名由真实UI和API另行断言；不宣称所有历史和签名提示同时可见。 |

[选定后端](evidence/resource-notifications/backend.json) · [核心与跨构建](evidence/resource-notifications/host-core.json) · [Web实际流程](evidence/resource-notifications-web/result.json) · [共享H5实际流程](evidence/resource-notifications-mobile/result.json) · [当前APK](evidence/resource-notifications/build.json) · [APK检查](evidence/resource-notifications/apk-inspection.json) · [准确映射模块](evidence/resource-notifications/android-js-content.json) · [媒体](evidence/resource-notifications/media.json) · [可视复核](evidence/resource-notifications/visual-review.json) · [实际升级](evidence/resource-notifications/development-update.json) · [只读复核](evidence/resource-notifications/development-project-resources.json)。工具、APK、私钥、数据库和原始临时录制帧仍在忽略目录。

两次准备检查保留原失败和执行来源：[通知分页上限](evidence/resource-notifications/attempts/01-checker-pagination/README.md)、[移动筛选加载时机及无用例包分类](evidence/resource-notifications/attempts/02-browser-ready-and-empty-package/README.md)。第一次使用100条通知查询被实际50条上限正确拒绝；第二次移动检查在列表加载前点击不可用筛选器，核心脚本另将没有用例的包级skip误当成跳过测试。修正仅涉及检查器，未放宽业务API、校验或测试断言。修正后重新执行当前选定后端、客户端、完整开发APK和两端页面流程；不把准备记录改名成最终通过，也不复写130以前的任何证据。

[规划表初次检查](evidence/resource-notifications/attempts/03-scope-table-sync/README.md)另发现GAP-08可见行未同步结构化清单中已实现的131子流程；保留失败记录和原文，修正该单元格后12项范围检查全部通过。工作包数量、验收分类、应用和实际执行记录不变；规划检查不是功能测试。

[发布指南核对](evidence/resource-notifications/attempts/04-guide-count-sync/README.md)要求两份完整语言指南也写明本轮496项共享移动检查；补入四份指南的真实执行数量及原生缓存边界，保留原始失败结果。应用、功能检查和媒体保持，发布扫描重新执行。

所有130及更早的已发布证据／媒体、97项历史迁移保持原始字节。原121全量后端、runtime56项和历史正式产物证据保留原来源，没有冒充本轮全量重跑。无Docker、WSL、虚拟机、模拟器、真机、网络性能或实网120GB检查。30AC／72TC适用性分类未修改；八项Skill仍draft。

资源站内提醒和本人工作代码已经实现；实际目标监控安装／故障／恢复及独立外部告警仍为GAP-23。实际OSS＋R2、真实模型／TG／平台、正式Android、生产部署及完整业务验收继续按[全部23项](../delivery/05-functional-closure-131.md)编号收口。开发包、合成录屏和通过的单元测试不代表生产就绪。

The current checks cover eight real HTTP/PostgreSQL suites (76 passing records),94 core test records,411 Web and496 shared-mobile cases, all configured client builds and the current development APK. The workbench package is compiled without unit cases; it is not counted as an executed test. Both actual Web/H5 journeys verify original alerts, read acknowledgements, live recovery, owner-bound deliveries and immutable signed history. Native28+28 results retain their exact UP-TO-DATE XML boundary. All prior published evidence remains byte-identical. Actual target/cloud/model/TG/platform and production acceptance stay open in their numbered packages.
