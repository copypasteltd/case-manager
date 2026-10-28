# 增量97验证：小程序身份与解绑失效

本轮使用原生 Go／PostgreSQL、Node 和浏览器，不启动 Docker、WSL、虚拟机或模拟器。方案与操作约束见[开发说明](../development/97-mini-program-identity.md)。当前只验证本地实现；真实微信／抖音平台交换、公司 AppID、香港真机与生产首发仍须独立验收。

## 来源与执行

| 检查 | 证据与准确范围 |
| --- | --- |
| 完整后端 | [分批结果](evidence/miniapp-identity-backend/result.json)：自动发现全部顶层测试并分批执行；含两平台受控 HTTP 协议、证明、绑定、MFA 设置／验证／恢复、会话来源与刷新解绑竞争 |
| 客户端 | [八项命令](evidence/miniapp-identity/client-checks.json)：新执行类型检查、Web／移动／运行时测试及构建、微信和抖音目标构建；完整输入摘要与私有原始日志摘要绑定 |
| React Web | [实际页面结果](evidence/miniapp-identity-web/result.json)：真实本地 API、隔离合成绑定；取消、解绑、响应丢失后的原幂等意图核对与窄屏布局 |
| Taro H5 | [实际页面结果](evidence/miniapp-identity-mobile/result.json)：同一后端，查看／解绑／刷新对账；H5 不提供平台身份获取入口，不伪造 wx.login 或 tt.login |
| Android | [构建](evidence/miniapp-identity/build.json)、[包检查](evidence/miniapp-identity/apk-inspection.json)、[JVM 范围](evidence/miniapp-identity/android-test-scope.json)：开发 APK 与当前业务包绑定；是否新执行 JVM 测试以实际 Gradle 任务状态为准 |
| Linux | [交叉编译](evidence/miniapp-identity/linux-build.json)：当前 Go API 编译；无 Linux 执行或部署 |
| 开发环境 | [更新记录](evidence/miniapp-identity/development-update.json)、[只读核对](evidence/miniapp-identity/development-project-resources.json)：仅操作原有且身份可核对的原生 API 与站内／报表进程；平台与其他外部集成默认关闭 |
| 截图／MP4／GIF | [编码及探测结果](evidence/miniapp-identity/media.json)：实际认证后页面像素、合成业务记录，凭据字段保持空白；原始帧仅存被忽略的本地目录 |
| 页面复核 | [视觉复核](evidence/miniapp-identity/visual-review.json)：两张最终截图人工核对，七张由实际布局与交互断言覆盖 |
| 最终一致性 | [独立来源核对](evidence/miniapp-identity/final-source-check.json)：当前源码、迁移、契约、客户端、媒体与准确验收边界 |

最终交付结果如下；源码摘要、逐项结果和运行时间见上方机器可读记录。

| 项目 | 实际结果 |
| --- | --- |
| 完整后端 | 189个顶层测试、1,183条结果，10批全部通过，最多同时两批原生进程 |
| Web／移动／运行时 | 135／248／51项新执行测试全部通过；类型检查与相应构建通过 |
| 微信／抖音 | 各20页构建通过；真实平台与真机尚待验收 |
| 迁移／契约 | 78项迁移保留原77项；OpenAPI 298路径／369操作／647模型 |
| Android | 42,945,032字节开发APK，18项包检查通过；26项JVM测试新执行通过 |
| 新增媒体 | 7张PNG、2段MP4、2个GIF；原90张PNG／33段MP4／33个GIF保留原始字节 |

开发APK SHA-256：`7dfd5fb4ddcb3c4c4c479ea1f7e87836d9a689ff8f86a936dc17cb0eb3cb8544`。完整Goal与生产发布仍未完成。

未完成或失败的早期尝试保留在 `evidence/miniapp-identity/attempts/`；最终通过的证据不覆盖旧失败为成功。最初原生回归发现新身份表缺少应用读取授权，已在尚未发布的迁移78中补齐；其他联调修正（包括 Android Expo 保护检查、抖音实际 AppID 接口及未完整执行的构建）见[本轮诊断记录](evidence/miniapp-identity/early-check-diagnostics.json)。

## DNS 测试夹具与完整重验

既有状态桥 DNS 拒绝测试在首次全量回归失败，隔离重复20次出现3次失败。拒绝分支改为独立客户端与不可变解析结果，命中计数改用原子变量，避免先前连接复用和共享计数干扰。运行时传输实现保留原发布字节；[最终重复验证](evidence/miniapp-identity/dns-fixture-check.json)记录20次新执行全部通过，[首次失败与诊断](evidence/miniapp-identity/attempts/10-before-dns-fixture-isolation/dns-fixture-diagnostic.json)保留原事实。

最终完整后端记录器为 `python app/scripts/verify_miniapp_identity_backend.py`：重新发现全部测试，每批最多20个顶层名称，同时最多两批原生进程。每个集成夹具创建随机独立 PostgreSQL 数据库并在结束时清理；不复用修正前已通过的批次。最终记录绑定脚本、每批原始日志和完整源码摘要。Linux 编译与完整来源页面媒体一并重新生成；未改动的客户端和 Android 各自使用其准确源码证据。

## 新执行与历史复用

新增页面媒体只展示本人绑定管理及恢复操作；实际 WeChat／Douyin 登录握手由受控原生 HTTP 测试验证请求与错误边界，不将受控服务响应称为真实平台身份。客户端可构建、APK 可检查不等于设备可用或商店可发布。

原96及更早的媒体、测试结果和文档保留其原始来源与时间。既有依赖锁文件不变，原90审计及开放的生产安全门槛不被重标为本轮审计。八项 Skill 继续 draft，不能把小程序登录能力等同于秘书已完成真实模型执行。

## 待真实环境验收

| 编号 | 仍需完成 |
| --- | --- |
| REAL-97-01 | 两个平台公司主体、AppID、Secret、HTTPS 合法域名、隐私声明、发布资格与审核 |
| REAL-97-02 | 香港微信／抖音真机本人绑定、MFA、平台登录、解绑、换号／重绑及账号停用 |
| REAL-97-03 | 同一真实员工查询、审批、资料阅读、秘书会话的完整平台能力验收 |
| REAL-97-04 | 境内服务器访问平台出口、超时／限流恢复、密钥托管与恢复演练 |
| REAL-97-05 | 四端正式构建、签名、原生安全持久化和发布闭环；其余28项 Goal 差异继续按原编号推进 |

完整 Goal 继续 active，未宣称生产就绪、真实云、120GB／2TB、真实模型或真实 TG 网络已完成。
