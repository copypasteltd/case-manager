# 增量91：秘书任务前置依赖 / Verification

日期：2026-10-27。应用位于 `app/`，文档与证据位于 `docs/`。这是源码绑定的开发验证，完整四端生产目标继续开放。原始受控MCP进程用于核对真实业务和隔离边界，没有调用真实模型。

## 实际检查 / Actual checks

| 范围 / Area | 本轮结果 / Result | 证据 / Record | 边界 / Limit |
| --- | --- | --- | --- |
| Go / PostgreSQL | 162顶层、1,080结果、982末级、9批；全发现执行 | [result.json](result.json) | Native Windows Go/PostgreSQL，非生产数据库 |
| 新依赖场景 | 22子场景及1根结果 | [完整测试](../../../../app/backend/internal/httpapi/agent_task_dependencies_integration_test.go) | 实际HTTP／MCP／原业务事务，受控原生执行器 |
| 数据库 | 74迁移，原73迁移原字节；实际关系／回执约束 | [开发只读核对](development-project-resources.json) | 归属API与两数据库worker；云／TG／模型关闭 |
| Web | 120测试、TypeScript及生产bundle | [web-build.json](web-build.json) | 编译产物不等于部署 |
| Runtime | 49测试、构建、Skill校验 | [runtime-build.json](runtime-build.json) | 八项Skill全draft；project-update1.7.0，17允许／15完成工具 |
| Taro | 212测试、类型、微信／抖音各18页编译 | [mobile-build.json](mobile-build.json) | 未调用小程序开发工具、模拟器或实际设备 |
| 两端实际页面 | Web／Taro H5各5组流程、各3宽度 | [Web](../task-dependencies-web/result.json) · [Mobile](../task-dependencies-mobile/result.json) | 原生真实API、隔离合成数据；非真机 |
| Android | 当前源码开发APK、18包检查 | [build.json](build.json) · [inspection](apk-inspection.json) | APK保存在忽略目录，不提交签名密钥／APK，不代表正式签名或分发 |
| JVM | 26结果、4 XML，保留实际Gradle任务日志 | [scope](android-test-scope.json) · [native log](native-build.log) | `UP-TO-DATE`为缓存复用，不称本轮新执行；无设备 |
| Linux | 当前完整后端linux/amd64编译 | [linux-build.json](linux-build.json) | 未执行产物、未部署服务器 |
| 媒体 | 新增8 PNG、2 MP4、2 GIF | [media.json](media.json) · [目录](../../../media/README.md) | 实际像素；原始JPEG与凭据保持忽略 |

Native database scenarios cover exact mandatory fields, null/extra fields, self/cross-project/stale/no-op decisions, add/remove, actual predecessor version and relationship, original start blocker, cycles, in-progress constraints, changed sources, read/write/session revocation, forged receipt fields, rollback after receipt failure, exact retry/lost acknowledgment, cancellation and immutable history. A successful unit test or compilation is not production acceptance.

当前开发APK：42,923,515字节，SHA-256 `94962044c3525db005677d07ea99e2e2ef9bfec645ac2de62d32c2869c8f52d1`；准确产物路径与完整来源以build记录为准。两录像实际时长Web 17.406秒、Taro H5 15.818秒；不是设备录屏。

## 员工路径与媒体 / Employee flow and media

两端检查登录与MFA后进入实际会话，选择准确更新方式和两个同项目任务，检查缺项／无变化，预览前不发送运行，发送完整六字段后以同幂等键重放，再在执行业务前取消。两个任务版本及真实关系均保持未改。随后查看夹具通过真实原Dependency命令及受控MCP提交的新增／解除历史，核对实际关系、版本与业务操作回执。不能把取消的UI运行说成已经执行，也不能把已提交夹具说成真实模型响应。

Both clients use their actual authenticated interface and API. Selection and preview do not write. The UI send is accepted, exactly replayed and cancelled before execution; committed add/remove views are separately established genuine business operations through controlled MCP. The current edge is independently read from the public dependency endpoint. Actual viewport widths are Web768/1280/1440 and H5 360/390/768. Visible wheel selections must also survive a2.5-second wait spanning live conversation polling.

MP4保留实际捕获间隔、H.264、10fps、无声；GIF为6fps／128色，Web960宽、移动390宽，速度由原始时长决定。关键步骤停留便于阅读，真实路由加载保留。截图与GIF首／中／末帧抽样复核，不宣称逐像素OCR或完整录像逐帧人工验收。布局检查只核验实际视口无横向溢出，不替代平台设备适配。

Capture manifests, per-frame digests, encoder identity, artifact hashes, ffprobe dimensions and exact durations are retained in the media record. Login/MFA occur before capture. Recordings contain synthetic task IDs and factual operation receipts, without passwords, MFA secrets or real business data. Owned browser contexts, review server, API fixture and disposable PostgreSQL databases are closed after each attempt.

## 来源、失败与复用 / Sources and retained attempts

后端、Web、运行时、移动、Android、Linux及页面捕获分别固定完整来源，不用代码片段摘要代替完整来源。新增迁移74保留原73项已发布字节。页面启动摘要覆盖完整应用、共享协议和采集辅助程序；捕获摘要另含实际客户端来源。所有当前证据以最终相同源码为准。

已发布增量90按提交 `e45c347a31cc7496b77585dc5328bcfdec92ec6d` 的原始Git对象重建完整启动／捕获来源；[历史核对](historical-90.json)保留84份旧证据／媒体，其中8份旧媒体，原字节均不改。10份发布前文档／基线快照见[publication-before.json](publication-before.json)。更早证据仍保留自身范围及限制。

| 保留记录 / Attempt | 原状态 / Original status | 原因与处理 / Reason |
| --- | --- | --- |
| [web-01](attempts/web-01/attempt.json) | 失败 | 检查脚本消息字段标签错误，真实表单检查先通过；准确旧来源保留后修正 |
| [typed-fixtures-01](attempts/typed-fixtures-01/attempt.json) | 失败 | 测试夹具字面量被推宽，TypeScript拒绝；保存完整来源和编译日志；首个自有Android构建中断，不宣称通过 |
| [backend-catalog-count-01](attempts/backend-catalog-count-01/attempt.json) | 失败 | 原测试仍要求16个工具；真实新场景已通过，全量在第二批失败；改为17后全量9批重跑 |
| [mobile-01](attempts/mobile-01/attempt.json) | 失败 | 可见滚轮拖拽未到目标项；改用可见选项点击，未直接修改组件状态 |
| [mobile-03](attempts/mobile-03/attempt.json) | 失败 | 曾观察到滚轮目标行与实际业务标签不一致，保留最初时序诊断；后续轮询复核定位到选项数组重建导致重置 |
| [mobile-04](attempts/mobile-04/attempt.json) | 失败 | 实际轮询重置滚轮，未绕过界面或写组件状态；稳定SelectField选项数据后重新采集 |
| [polling-range-superseded](attempts/polling-range-superseded/attempt.json) | 编译／包检查通过，明确被替代 | 保留完整来源与原移动／Android检查；实际交互发现轮询问题后修正共享选择器并重新构建 |
| [web-02](attempts/web-02-superseded/attempt.json) · [web-03](attempts/web-03-superseded/attempt.json) | 通过，明确被替代 | 夹具类型／共享检查脚本变化，原通过状态不改写，最终重新采集 |
| [mobile-02](attempts/mobile-02-superseded/attempt.json) · [web-04](attempts/web-04-superseded/attempt.json) | 通过，明确被替代 | 像素复核发现纠正输入后旧错误提示未清除，录像停留偏短；修正两端提示并重新构建采集 |
| [visual-review-superseded](attempts/visual-review-superseded/attempt.json) | 构建／媒体通过，明确被替代 | 完整旧来源、通过的构建／APK检查、日志及媒体保留；用于追溯，不用于当前源码结论 |

首轮临时原生夹具测试还修正过接口路径、状态码及测试准备状态；各次完整旧来源未在改动前冻结，未把这些过程日志包装成可重现的通过证据。最终完整当前来源与全量结果已独立记录。所有归档仅含源码、合成诊断及必要媒体，不提交数据库、真实业务或秘密。

来源校验器首次把发布前快照数量写成11；实际清单为10份，全部与旧Git字节相同。已保留[诊断](attempts/source-verifier-01/diagnosis.json)、原校验器及原失败结果，仅修正数量断言后复核。

Retained attempts keep exact source ZIPs and their original result: a superseded pass remains a pass, while a failed attempt never becomes current passing evidence. Initial ad-hoc native fixture iterations lack complete pre-edit archives; that provenance limitation is disclosed. The final current-source full regression is separate.

锁文件与增量90完全相同，本轮未重新请求审计。沿用[原始完整审计](../dependency-hardening/dependency-audit.json)：86项，critical0／high20／moderate64／low2，生产安全放行继续开放。原90的12项有界安全回归和2项原生JS检查保留原证据范围，不宣称本轮重新执行。JVM缓存复用与真实新执行分别记录。

## 复查命令 / Recheck commands

先按项目工具配置准备原生开发数据库。以下是复查入口列表，按依赖顺序逐项执行并检查退出码，不是允许遇到失败后继续覆盖证据的批处理。

在仓库根目录使用原生Python及已配置Go／Node／PostgreSQL执行；已有同名证据须先按准确来源归档，不覆盖旧结果。运行npm时进入 `app/`。Android需实际已安装的JDK17和SDK34，不启动设备或虚拟化。构建配置为loopback开发环境，不可当生产配置复制。

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_backend_batches.py --batch-size 20 --evidence-group task-dependencies
python app/scripts/verify_web.py task-dependencies
python app/scripts/verify_agent_runtime.py task-dependencies
python app/scripts/verify_mobile.py task-dependencies
python app/scripts/run_task_dependencies_browser.py --mobile
python app/scripts/run_task_dependencies_browser.py
python app/scripts/encode_task_dependencies_media.py
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python app/scripts/build_android.py --google-mirror --evidence-group task-dependencies
python app/scripts/inspect_android_apk.py --evidence-group task-dependencies
python app/scripts/freeze_task_dependencies_evidence.py
python app/scripts/verify_task_dependencies_development.py
python app/scripts/verify_task_dependencies_sources.py
python app/scripts/verify_repository_publication.py
python app/scripts/verify_repository_publication.py --staged
```

Linux编译使用原生Go跨编译，记录完整后端摘要，不启动Linux／WSL。开发环境更新器只替换确认归属且准确来源变化的API及报表worker，保留原生数据库和通知worker；迁移用独立配置。来源复核以 [final-source-check.json](final-source-check.json) 为准。

## 未验收部分 / Remaining acceptance

继续保留[全部28项差异](../../../delivery/03-goal-gap-assessment.md)：更多业务工具与八项Skill真实发布／模型隔离；实际TG及境内网络；OSS／R2身份、双云完整性和恢复；Web／Android120GB真实网络、中断／过期／重启与2TB迁移；微信／抖音真机平台、Android实际设备／正式签名／分发；审计剩余安全项、容量、备份PITR及生产部署／切换。没有把本地开发包、合成录像或通过测试当作四端生产完成。

The full goal remains active. Code, both language guides, affected documentation, media, evidence and changelog are committed and pushed together to the authorized repository, without force-pushing or exposing local secrets. Publication completion does not close the production gaps.
