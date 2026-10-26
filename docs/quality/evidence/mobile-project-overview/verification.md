# 增量70验证记录

日期：2026-10-26。已接通移动项目业务概览、费用／收付款／订阅／变更原记录查阅及预算修订分页。当前源码的89项移动测试、微信／抖音各16页编译、6条真实API浏览器链及18项布局检查通过；Android重新生成离线业务包及开发APK，17项包检查通过。[最终来源与资源核对](final-source-check.json)24项通过。实现与约束见[增量70](../../../development/70-mobile-project-overview-and-business-reading.md)。

## 1. 实际执行与复用范围

| 范围 | 证据 | 结果及边界 |
| --- | --- | --- |
| 移动业务 | [测试与双目标构建](mobile-build.json) | 89项通过，0失败／取消／跳过；类型检查和两目标编译通过，实际输出各16个注册页面；新增10项准确金额、时间、引用和概览完整性测试 |
| 实际业务读取 | [浏览器结果](../mobile-project-overview-browser/result.json)／[来源](../mobile-project-overview-browser/source-manifest.json) | 原生隔离PG、真实HTTP、Taro H5、真实登录／MFA及业务接口；6条完整链通过；只有明确标记的传输故障注入，没有替换业务响应 |
| 布局与截图 | [布局结果](../mobile-project-overview-browser/result.json)／[人工复核](../mobile-project-overview-browser/visual-review.json) | 概览、原费用与受限读者3页，浅／深两主题，320／390／768共18项；查看3张当前390像素截图，保留准确图像摘要；不是原生UI设备测试 |
| Android构建 | [构建记录](build.json)／[包检查](apk-inspection.json) | 离线业务包和原生Gradle编译均通过，25项Android工程边界用例、17项包检查通过；包含16个注册页面、实际原生请求／缓存／上传适配层；开发签名、arm64-v8a |
| 既有独立JVM | [增量66最终来源证明](../android-upload/jvm-source-proof.json) | 同一批25项纯Java用例的来源及4份XML摘要仍逐项匹配；本轮复用独立JVM证据，Android工程执行相同25例，不合计为50项独立场景 |
| 后端与Web | [增量69后端](../project-overview/result.json)／[Web](../project-overview/web-build.json) | 来源未变，复用后端142顶层、872条结果／789末级场景及Web75项／生产编译；本轮不计作重新全量执行，浏览器另行实际使用概览与原业务API |
| 秘书运行时 | [既有34项](../android-native/runtime-build.json) | 来源未变，沿用34项测试及8个draft Skill证据；没有新增工具或开启真实模型 |
| 所属开发环境 | [64项迁移与模板保护](development-project-templates.json)／[运行核对](development-runtime.json)／[概览路由](development-overview.json) | 只读核验，迁移摘要逐项一致，0个隔离测试库；概览路由未登录401；原API／PG／通知Worker／Vite保持运行，未重启或写入真实业务资料 |

移动来源 SHA-256：`fd1ad596eff77cb9004e9bd536b860f747da7dc97b4c3513422a225ff68da487`。浏览器完整来源：`c16c0c590b64e0ac8c0a1c935201928d98593cebe4d7a7ebb852ee3b9b4d8b42`，夹具启动前、H5编译后及测试结束保持一致。

Android完整构建来源：`a8e65e02d146cbc5fff0d34197c3ce11825a50264497e8efde2d94e5cd8fe730`。APK为42,809,150字节，SHA-256：`5e8c6cbfcd272dd15672df260d18df2c79eb37a7b59487ef2d5e092bda4bf2cc`；按内容摘要保存的APK（仅本机保留，未纳入Git：`app/.local/android/artifacts/mobile-project-overview/5e8c6cbfcd272dd15672df260d18df2c79eb37a7b59487ef2d5e092bda4bf2cc/app-debug.apk`）独立于Gradle可变输出及增量66冻结APK。离线业务包摘要：`8acb8c9366faf930b9af53fde51a79021279209c3891c6412b6fd4e399884a4e`。

## 2. 功能验收映射

| 编号 | 对应要求 | 实际证据与限制 |
| --- | --- | --- |
| MVO-AC-01 | MVO-01：16类可读聚合与准确导航 | 真实概览逐项对照API，8项任务的完整数与5项预览分别核对；11类原记录实际GET校验准确ID，不依赖可重名标题 |
| MVO-AC-02 | MVO-02：原币精度与不同业务口径 | 便携逻辑测试大额／负额／零／6位精度；浏览器核对本项目分摊、冲销与净额，以及草稿、付款与未批准预算的独立性 |
| MVO-AC-03 | MVO-03：费用原记录与固定来源 | 真实原费用／冲销读取，核对准确修订、原币金额及实际分摊；固定凭证复用已有版本读取组件与授权协议，不冒充新增云传输验收 |
| MVO-AC-04 | MVO-04：资金与分配 | 实际付款及分配GET核对准确ID／金额，分配与发生费用分开；本轮只读页面，不发起银行支付或新增业务写入 |
| MVO-AC-05 | MVO-05：订阅配置与账期区别 | 实际读取订阅与空账期，明确没有已生成账期不等于没有义务；价格、已生成账期和供应商核对采用原字段，本轮浏览器未覆盖非空价格／账期的完整操作旅程 |
| MVO-AC-06 | MVO-06：提案与预算准确引用 | 实际建立23项草稿核对20项游标分页；再新增22项使原已选预览移到后续页，点击保留的原预览后继续分页，仍准确定位同一预算ID及来源提案，不替换为最近版本 |
| MVO-AC-07 | MVO-07：当前权限与失败清理 | 费用／概览传输失败先清除旧事实，重试真实API恢复；受限读者不能读取费用；真实移除成员使会话失效且旧标题／金额清除；取消项目保持授权历史只读 |
| MVO-AC-08 | MVO-08：多端构建与触控 | 微信／抖音各16页实际编译，Android离线业务包和APK实际生成并核验；320像素真实触控面积问题修复为至少44 CSS像素；不替代任何平台或实体设备验收 |

## 3. 失败尝试与修正

浏览器保留7次中间尝试及来源、失败截图和清理结果，分别为[不必要的管理员夹具](../mobile-project-overview-browser/attempts/01-unneeded-administrator-fixture/reason.json)、[服务标题断言](../mobile-project-overview-browser/attempts/02-service-heading/reason.json)、[同名费用选择](../mobile-project-overview-browser/attempts/03-duplicate-cost-titles/reason.json)、[窄屏点击面积](../mobile-project-overview-browser/attempts/04-narrow-row-touch-target/reason.json)、[直接改hash的页面切换](../mobile-project-overview-browser/attempts/05-hash-only-same-page-transition/reason.json)、[Taro保留页历史](../mobile-project-overview-browser/attempts/06-retained-taro-page-history/reason.json)及[成员命令路径](../mobile-project-overview-browser/attempts/07-member-command-path/reason.json)。

实际产品修正为共享可点击行使用44PX最小高度，避免Taro缩放后在320像素屏幕低于44像素。其余修正让检查器采用原夹具、真实页面标题、准确HTTP记录ID、Taro实际导航和有效返回历史，以及既有成员命令字段；没有放宽权限或伪造业务响应。最终执行完整6条链和全部18项布局，不拼接中间尝试的通过结果。

触控修正前的[首个Android构建](attempts/01-row-touch-source-correction/result.json)由准确PID及创建时刻确认后主动停止，仅终止本轮所属构建树，未将不完整产物计为成功；随后对修正后的固定源码完成新构建。来源核对首次[误引中间Java证据](attempts/02-source-evidence-reference/reason.json)，最终改为核对增量66的纯Java最终来源及实际XML摘要，同时保留本轮Android工程25例通过证据；产品Java代码没有为此修改。

## 4. 清理与未完成范围

所有本轮所属浏览器上下文、标签页、浏览器、临时5189监听和原生隔离数据库已释放，私有夹具文件已删除。日常18080 API及5178前端保持就绪；原开发API和通知Worker的准确进程身份仍匹配，PG数据目录和S:映射保留。未调用Docker、WSL、虚拟机、模拟器或实体设备，没有发送TG消息或开启模型／云存储。

本轮是新增查阅功能和构建验证。项目立项／成员及更多操作、完整财务筛选／导出及跨OA旅程、知识／附件余项、秘书剩余工具和真实运行、正式平台身份／真机／签名升级、120GB／2TB、生产部署／恢复／性能仍按[28项差异](../../../delivery/03-goal-gap-assessment.md)推进。依赖锁未变，既有审计114项告警（critical 11、high 32）仍未关闭；完整Goal、正式发布和真实资源验收均未完成。
