# GAP-26 — 依赖安全与正式产物准备

> “继续完成目标正式发布版本的依赖冻结与产物安全核对，覆盖目标Linux部署及正式Android等产物，不以本地开发构建替代生产安全验收。”
> — `docs/delivery/04-current-scope.json:456`；验收要求在`:457`：“安全问题按实际风险和既定规则关闭／签核，不压低框架版本或隐藏公告。”

**判定：代码准备已有实现；完整工作包仍 partial／正式签核 undecidable。新增独立确认的代码缺项：0。** 这是增量168只读源码检查，不是重新扫描公告库、执行构建、检查本机安装树或批准正式产物。

---

## 已追踪的执行链

- 依赖身份及实际调用：`app/package.json:6`限制Node22.15至22.x，`:28`连接五组依赖专项，`:31`起固定工作空间依赖及覆盖；`app/package-lock.json:1`固定解析图，`app/backend/go.mod:3`固定Go工具链及模块，`app/mobile/android/gradle/wrapper/gradle-wrapper.properties:4`固定Gradle分发摘要。未以降级React/Taro或省略dev依赖消除公告。`packages/deepmerge-safe/UPSTREAM.json:1`保存来源／原入口摘要／公告／MIT身份，`index.cjs:6`的自身数据属性定义用于三个合并分支；`packages/uri-decoder-compat/index.cjs:4`真实调用明确别名的上游0.5.1，非空适配壳。
- 审查与留证工具：`app/scripts/audit_dependency_integrity.py:39`的完整npm扫描参数包括dev／optional／peer，`:49`后的结果区分命令完成、零公告与生产安全门；不隐藏尚存发现。`verify_dependency_integrity_security.py:54`为新尝试分配独立目录并先保存源码，`:69`调用实际安全／native-JS消费端用例，`:83`检查真实结果，`:89`检查完整安装树。`dependency_integrity.test.mjs:42`、`:110`、`:155`、`:176`、`:208`、`:272`分别沿实际包解析、RN配置、source-map、XML、UUID、静态中间件调用核对；这里只读了源码，未将这些历史用例视作168通过。
- 正常原生CI：`.github/workflows/native-release.yml:4`手动触发且固定官方Action提交；`native_ci_release.py:96`要求专用非root Linux、干净提交与Node22，`:107`正常`npm ci`，`:109`后端真实结果及`:114`客户端步骤，`:123`进入服务器构建且再核对干净来源。`:133`明确不声称生产部署／隔离／模型验收；日志、测试身份与上传白名单分开。该CI未自动调用全部依赖专项，现有手动入口也不能代替正式风险签核；需求没有强制指定一个新的扫描器或每次CI执行策略，未据此虚构代码缺陷。
- 服务器产物：`build_production_release.py:21`收集准确输入，`:75`保存输入清单和源码，`:91`真实编译三类Linux ELF、Web和运行器；`:120`固定迁移及全文件清单，`:124`拒绝打包时来源变化。`build_native_runtime_release.py:93`沿实际安装的运行依赖递归取包，检查包身份／路径／上限，`:129`保存依赖及锁摘要。`deploy/release/bundle.py:92`严格校验清单，`:123`对照实际树，`:146`拒绝链接、重复／越界成员并按外部给定完整摘要解包；`controller.py:289`的stage实际调用该校验，随后冻结权限，安装后再次检查。可信外部摘要锚点是当前主机信任边界，不能把随不可信下载取得的摘要当作正式签核（运行手册`:37`）。
- Android正式产物：`build_android.py:59`仅在显式私有签名配置下选择正式APK+AAB，`:101`固定完整源集合，`:132`核对APK内真实业务bundle，`:145`调用独立签名检查。`android_release.py:40`拒绝仓库内私钥／本地HTTP／非递增版本，`:84`核对APK和AAB内bundle及独立工具的证书、签名、包名和release标志。`mobile/android/app/build.gradle:90`后的正式任务也拒绝缺配置／local profile。源码具备准备路径，不证明已提供公司签名资料或实际构建当前正式包。

## 搜索与反证

用`rg --files`定位上述入口；在`app/scripts`、`app/deploy`、`.github`检索`dependency|security|audit|govuln|sbom|signoff`，并追踪`manifest|release_id|sha256|source|release|sign`的生产者与消费端。发现真实专项、构建、清单及签名检查链，不能从“尚无正式产物验收”推导这些代码不存在。反向核对119说明`:59`后的人工审阅边界及当前167余项表`:28`，均未把正式安全签核取消或宣称已过。

没有执行npm审计／包安装／Go或Gradle／构建器／CI／浏览器／服务器。本报告没有查询最新公告，也未逐字读取整份锁图、所有第三方源码、机器缓存或任何私有资料；因此不判定当前依赖无漏洞、安装树可信或正式签核通过。九个冻结版本与APK164保持原来源，当前Android JS／新APK延期及实际目标Linux、公司风险规则、证书、正式发布接受仍开放。不得用本项“无新增代码发现”关闭GAP-26或全部23项。
