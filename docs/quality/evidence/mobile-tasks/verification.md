# 增量78验证记录 / Increment78 verification

实现与使用约束见[移动任务、子任务与前置依赖](../../../development/78-mobile-task-editing-and-dependencies.md)。本轮使用原生Windows、隔离PostgreSQL、真实登录／MFA及实际Taro页面和HTTP命令；没有调度Docker、WSL、虚拟机、模拟器或子Agent。

Manual mobile task creation/editing, subtasks and prerequisites use the original backend commands. Checks ran with native Windows tools, isolated PostgreSQL, actual authentication/MFA and Taro/API interactions. No virtualization, emulator or subagent was used.

| 范围 / Scope | 实际结果 / Result | 证据 / Evidence |
| --- | --- | --- |
| 移动测试 / Mobile tests | 最终124项通过，0失败／取消／跳过；新增9项任务输入／权限／回执／关系证明及1项共享草稿测试 / 124 passed, including nine task-boundary tests and one shared-draft test | [移动构建](mobile-build.json) |
| 类型与小程序 / Types and mini-programs | 类型检查、微信65个文件／958,247字节、抖音62个文件／897,952字节；各16个注册页面 / Types and both 16-page builds passed | [四阶段结果](mobile-build.json) |
| 实际页面 / Actual pages | 9组实际API／H5流程、6项320／390／768px任务创建与关系布局通过 / Nine actual API/H5 flows and six responsive checks passed | [结果](../mobile-tasks-browser/result.json)、[完整来源](../mobile-tasks-browser/source-manifest.json) |
| Android开发包 / Development APK | 新冻结APK 42,832,852字节；17项包检查通过，离线图包含新的任务表单、输入及命令模块 / New APK, 17 inspections, current task modules included | [构建](build.json)、[包检查](apk-inspection.json) |
| 原生测试 / Native tests | 实际任务为UP-TO-DATE，保留25项通过结果；当次Gradle日志及4份XML已冻结，本轮未重新执行 / 25 cached passing results, not newly executed | [执行范围](android-test-scope.json)、[日志](native-build.log)、[XML](android-unit-results/) |
| 独立纯Java / Pure Java | 来源未变，保留增量66的25项／4份XML准确证明，本轮未执行 / Exact unchanged increment66 proof retained | [原证明](../android-upload/jvm-source-proof.json) |
| 后端／运行时／Web / Backend/runtime/Web | 完整相关来源未变，复用增量74的146顶层／919条结果、39项运行时、81项Web及构建，本轮未全量重跑 / Reused only at matching complete source digests | [后端](../deliverable-creation/result.json)、[运行时](../deliverable-creation/runtime-build.json)、[Web](../deliverable-creation/web-build.json) |
| Linux | 完整后端来源与原产物摘要相同，未重建、执行或部署 / Same original artifact; not rebuilt, executed or deployed | [原编译](../deliverable-creation/linux-build.json) |
| 日常开发 / Daily development | 新10条只读探针、69项迁移逐项摘要、原API／Worker身份、0测试库；日常进程未重启 / Fresh read-only checks, no daily-service restart | [通用](development-runtime.json)、[当前约束](development-project-resources.json) |
| 媒体 / Media | 新2张PNG、32个实际源帧编码为6.7秒MP4及40帧GIF；实际查看两图及GIF首／中／末帧 / Two actual screenshots and a recording/GIF pair, with explicit visual sampling | [编码](media.json)、[查看](visual-review.json) |
| 最终来源 / Final source checks | 20项当前源码、产物、清理与原证据复算通过 / Twenty checks passed | [来源检查](final-source-check.json) |
| 仓库 / Repository | 当前候选／暂存文件的双语覆盖、链接、忽略规则、敏感信息、媒体及来源检查 / Current candidate/index publication preflight | [候选记录](repository-check.json)、[工具](../../../../app/scripts/verify_repository_publication.py) |

## 1. 来源与准确执行范围 / Sources and execution scope

- 移动128项来源SHA-256：`68d664da8c45649baa3ef17574c8e2e6722fc5d83f7993ff30a6021dbbcd84fc`。
- 实际原生夹具／H5构建／浏览器596项来源SHA-256：`b917a2696f4c81c1d22c3c2d65b2fd7ef6b3025c7400bedddce01f9b389d6465`；三个阶段均一致，完整前后未变。
- Android完整来源SHA-256：`9825dbc4d2dc02a1839054d976c156e1fa27b69d330158213d0987e694f98a1b`。
- 新APK SHA-256：`031cacf4072ad5ecd3117c9e72d03789d2314d5ad80083d1be77119edcfe2c10`；准确产物保存在忽略的app/.local/android/artifacts/mobile-tasks/下，不提交APK、签名密钥或SDK配置。
- 后端完整来源SHA-256：`61754e7e60bdac1ad937c5890330de0fa872deaa72d116f428348ef46bdfb10d`，与增量74相同。

各清单覆盖范围不同，不能直接互比SHA。原生25项测试的实际任务行为为 `> Task :app:testDebugUnitTest UP-TO-DATE`。没有将缓存XML、独立纯Java旧证明或原后端测试记作本次新执行。未启用真实模型、云存储或TG；八项Skill仍draft。

The source manifests cover different scopes, so their digests are not interchangeable. The exact Gradle task was UP-TO-DATE. Cached XML and unchanged historical backend/runtime/Web/Java proofs are not counted as new executions. All eight Skills remain draft and real integrations remain disabled.

初始仓库媒体的712项清单中，711项可从9e87869准确读取，另1项web/tsconfig.tsbuildinfo是被正确忽略的生成缓存。本次[历史来源补充](historical-capture-sources.json)在缓存原字节仍可读取时完整重建原始来源字节流，SHA与旧采集一致；随后发布逐文件摘要和重建回执，保留原报告。后续检出逐项核对历史Git字节及保留缓存摘要，不要求提交或恢复生成缓存，也不将旧媒体标为当前重新采集。

The initial 712-entry media manifest includes 711 committed sources and one correctly ignored generated TypeScript cache. The supplement reconstructed the exact original byte-stream while that cache was available and matched the old digest. Future checks verify historical Git blobs plus the retained cache-hash receipt; generated cache bytes and old reports remain untouched.

## 2. 实际流程 / Actual flows

实际操作验证新建任务、内容／负责人／排期、分页候选、子任务、父任务及里程碑关联；香港时间与UTC表示按同一微秒时刻核对。不可实现日期不发送创建请求；编辑清空日期或关联明确传null，保持负责人时省略assignee_id。

前置未完成时原业务返回422，反向循环返回409；完成实际前置后才能开始。添加／移除后核对实际任务下一版本及关系。负向检查只隐藏浏览器的一次实际关系列表，验证原命令已提交但证明不完整时不能显示成功；独立原API确认真实关系。另检查响应字段不完整、真实并发409及提交后断开响应：保留输入、禁止重复提交，实际创建只有一条记录。

错误项目深链接不能编辑另一真实可读项目的任务。实际绑定角色、移交负责人并限制原用户为viewer后，以新MFA身份核对新建／编辑／子任务／依赖入口关闭；项目取消后有业务动作的身份也不能通过深链接创建。

Checks cover actual creation/editing/association, microsecond dates, paginated candidates, original prerequisite/cycle rejection and subsequent allowed transitions. Controlled transport/receipt faults are negative tests for blocking false success, with actual committed state checked separately through the original API. Cross-project links, fresh-MFA viewer restrictions and cancelled-project creation are also exercised.

5个所属浏览器上下文全部关闭；浏览器、临时H5服务、原生夹具、一次性数据库与私有夹具均清理，5189端口释放。未向组件注入业务状态，未把负向响应修改当作正常成功证明。

All five owned contexts, the browser, temporary server, native fixture and isolated database were closed. No component business state was injected; modified negative-test responses do not serve as positive success evidence.

## 3. 保留尝试 / Retained attempts

| 尝试 | 发现与修正 / Finding and correction | 原记录 / Record |
| --- | --- | --- |
| 01 | 检查器将实际+08:00时间与Z字面串比较；应用已经正确归一化，改正检查器，保留六位精度 / Corrected checker comparison of offset-equivalent instants | [结果](../mobile-tasks-browser/attempts/01-offset-equivalent-timestamp/result.json)、[原因](../mobile-tasks-browser/attempts/01-offset-equivalent-timestamp/reason.json) |
| 02 | 7组流程已过，最终viewer夹具PATCH为422；初步误判缺少name/type，原记录保留，诊断由03更正 / Initial setup diagnosis was incorrect and is explicitly corrected in attempt03 | [结果](../mobile-tasks-browser/attempts/02-owner-transfer-required-fields/result.json)、[原诊断](../mobile-tasks-browser/attempts/02-owner-transfer-required-fields/reason.json) |
| 03 | 8组流程已过；确认ProjectPatch多发reason，违反原Schema。改回原ownerPayload的expected_version和owner_id，最终全流程通过 / Removed the extra unsupported reason field; original contract preserved | [结果](../mobile-tasks-browser/attempts/03-owner-transfer-extra-field/result.json)、[更正](../mobile-tasks-browser/attempts/03-owner-transfer-extra-field/reason.json) |
| 页面通过快照 | 新检查器末尾多一空行被仓库格式检查发现；移除后重跑受影响页面采集并更新媒体，移动／原生业务来源不变 / Archived the original positive capture and media before a checker-only EOF cleanup and recapture | [原通过记录](../mobile-tasks-browser/attempts/04-before-checker-eof-normalization/result.json)、[范围](../mobile-tasks-browser/attempts/04-before-checker-eof-normalization/scope.json) |
| 移动旧快照 | 最终新关联等待读取完成后重新跑124项与双构建；之前通过的完整来源和日志另存，未将旧摘要当当前 / Rebuilt after final reference-read readiness change; prior source-bound success archived | [旧证明](attempts/01-before-reference-readiness/mobile-build.json)、[范围](attempts/01-before-reference-readiness/scope.json) |
| 仓库历史来源 | 初次仅从Git读取全部旧清单，因已忽略的tsbuildinfo不存在而停止；补711项Git及1项生成缓存的原字节重建／摘要记录，缓存不提交 / Added an explicit ignored-cache reconstruction receipt after the Git-only check stopped | [原失败](attempts/02-historical-ignored-cache/result.json)、[补充](historical-capture-sources.json) |

上述浏览器尝试都已退出原生夹具并关闭所属浏览器及临时服务；原失败结论没有覆盖成通过。最终完整当前来源的通过记录是本证据组与mobile-tasks-browser根目录结果。

暂存检查中的60项空白提示全部来自6份原始日志／历史快照，按[专用范围记录](raw-evidence-whitespace-scope.json)保留字节；Git属性只豁免证据日志及attempts快照，不改变当前应用／指南的检查，也不豁免敏感信息、链接、体积或来源核对。

## 4. 复现与余项 / Reproduction and remaining gates

在独立检出且已准备原生工具与依赖的app/中依次执行。以下为本次准确命令，写入固定证据路径；后续增量应先调整为新证据组以保留已有历史。页面与移动／Android编译不要同时更改或争用相同构建目录：

```powershell
$env:PYTHONUTF8='1'
python scripts/verify_mobile.py mobile-tasks
python scripts/run_mobile_tasks_browser.py
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python scripts/build_android.py --google-mirror --evidence-group mobile-tasks
python scripts/inspect_android_apk.py --evidence-group mobile-tasks
python scripts/encode_mobile_tasks_media.py
python scripts/verify_mobile_tasks_development.py
```

原始帧留在忽略路径，编码工具要求本机ffmpeg／ffprobe。重建后需重新实际查看媒体，并将该次Gradle日志／4份XML及准确任务状态另存到新的证据组；来源／仓库工具随后复算，不照抄本次“看过”和缓存状态。

Reproduction requires native tools and dependencies; raw frames remain ignored. Review newly rebuilt media and record that run's actual Gradle task/log/XML in a fresh evidence group before final verification. Do not copy the prior manual-review or cache claims.

完整Goal仍未完成。实体Android／微信／抖音、真实云／模型／TG、120GB／2TB、隔离评测、八项Skill发布、剩余移动变更／预算／结项／知识／财务及生产安全、部署恢复按[28项GAP](../../../delivery/03-goal-gap-assessment.md)继续，不以本地编译或H5媒体替代正式验收。
