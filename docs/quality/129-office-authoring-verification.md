# Office实际检查 · 增量129

以[执行计划](evidence/office-authoring/verification-plan.json)及[最终源码复核](evidence/office-authoring/final-source-check.json)为准。创建／编辑产生真实OOXML字节，正式版本通过实际HTTP／PostgreSQL、完整摘要、扫描、提升与人工提交；对象传输和扫描器使用隔离合成夹具。没有真实云、模型、TG、容器、WSL、虚拟机、模拟器或真机调用。

| 编号 | 实际检查 | 覆盖与边界 |
|---|---|---|
| V129-01 | 六组选定HTTP／PG，46条通过、零失败／跳过 | 三格式新建与新版本提交、原历史不改、生成不发布、过期依据、错误文件／匿名、损坏来源、处理中撤权、小程序只读及生成正文哈希不一致；原上传控制／权限／摘要／扫描／提升回归。未执行全量后端。 |
| V129-02 | Go Office处理与三种独立原生消费库 | 实际六份新建／编辑产物由python-docx1.2.0、openpyxl3.1.5、python-pptx1.0.2读取；检查真实段落／表格／工作表／公式／幻灯片及清除缓存。未改压缩部件保持；外部指令／保护／嵌入与不支持公式拒绝。没有Office视觉排版验收。 |
| V129-03 | Web396项、共享移动481项与全项目类型 | 全现有客户端单元套件；新增结构化内容、字面公式／明确类型、候选元数据与完整有界摘要检查。 |
| V129-04 | Web、微信、抖音及Android构建 | 小程序20注册页；Android为当前local配置开发APK，含当前JS与原生私有来源，实际产物19项检查。Android28项在本轮Java先行检查中实际执行，随后完整打包复用其UP-TO-DATE结果；独立JVM28项在成功完整构建中实际编译执行。[原始XML和缓存说明](evidence/office-authoring/native-test-scope.json)。正式签名／升级／发布继续GAP-16。 |
| V129-05 | 实际Web、共享Taro H5与当前API | Web逐一编辑／生成Word、Excel、PPT并核对摘要及原文件头不变，另核对目录新建DOCX；共享H5只读读取三种准确正式版本。页面录制不声称完成真实云上传，完整提交另由V129-01覆盖。 |
| V129-06 | 开发服务更新、96项结构与旧资料 | 只替换已核实归属的API／通知／报表服务，不重启PG，不启用外部集成；既有文件／OA／费用／现金／历史提取数据保持，无新迁移。 |
| V129-07 | 6PNG／2MP4／2GIF可视检查及释放 | 本人登录后的实际页面，GIF明确为三张场景截图各2秒导览；无密码、MFA秘密或真实业务资料。所有自建浏览器／上下文、临时服务和隔离库完成后释放。 |

[后端](evidence/office-authoring/backend.json)、[原生生成消费](evidence/office-authoring/native-parser.json)、[Web](evidence/office-authoring-web/result.json)、[共享H5](evidence/office-authoring-mobile/result.json)、[当前开发APK](evidence/office-authoring/build.json)、[实际APK内容检查](evidence/office-authoring/apk-inspection.json)、[原生套件](evidence/office-authoring/native-suites.json)、[四个当前JS模块](evidence/office-authoring/android-js-content.json)、[素材](evidence/office-authoring/media.json)、[可视复核](evidence/office-authoring/visual-review.json)。APK、安装依赖、签名私钥、数据库和临时原件留在忽略目录。

为录制已验证身份后的业务，浏览器专用分支补一条无业务动作的MFA角色／绑定。此前八项源码检查的实际输入摘要不改写：固定逆补丁重建原测试文件及原计划，完整组合摘要必须等于已执行记录；生产字节、选定测试正文、命令和验收要求不变。修正只使合成账号走真实MFA，不增加业务权限。见[准确来源说明](evidence/office-authoring/browser-fixture-adjustment.json)与[MFA夹具尝试](evidence/office-authoring/attempts/04-browser-mfa-fixture/README.md)。

原生生成消费和Android构建绑定MFA夹具修正后的源码。随后仅为移动素材将已断言正文滚动到视口，并补准确的Office素材说明；两端浏览器和全部受影响素材重新执行。固定逆替换同时重建此前两种完整输入摘要，原生记录保留其真实执行摘要；当前应用字节、测试、命令和编码机制均不变，不为取景调整重复构建。此前页面检查通过但取景不足的素材及原输入，保留在[正文取景尝试](evidence/office-authoring/attempts/06-mobile-content-framing/README.md)。

早期命令参数和Web显式类型引用修正前的检查保留原状态；一次重复启动的原生构建被中断，不计通过。完整构建曾在Android测试输出文件被独立Java预检查占用时失败，保留[文件锁原记录](evidence/office-authoring/attempts/05-native-test-file-lock/README.md)，占用结束后单独执行相同冻结构建命令。其他探索性与录制尝试按本轮attempts保存，不改称最终通过。[执行计划](evidence/office-authoring/verification-plan.json)逐项绑定最终使用的命令、源码和边界。

全部128及更早证据／媒体原字节、96项历史迁移保留；旧121全量后端与runtime56项没有重新执行，不冒充本轮结果。当前开发APK独立于旧121开发包与正式Android验收。当前[范围](../delivery/04-current-scope-and-acceptance.md)、30AC／72TC适用性及[23项余项](../delivery/05-functional-closure-129.md)保持；有界Office编辑不等于完整Office兼容或生产就绪。
