# 增量95验证 / Increment95 verification

[方案与使用流程 / Workflow](../development/95-task-discussion-and-mentions.md) · [媒体 / Media](../media/README.md) · [编号差异 / Gaps](../delivery/03-goal-gap-assessment.md) · [当前功能基线 / Current baseline](current-functional-baseline.json)

日期：2026-10-28，Asia/Taipei。范围为真实本机 Go API、原生 PostgreSQL、React Web、共享 Taro H5、小程序编译和 Android 开发构建。合成数据来自隔离数据库，未接通真实云、模型或 Telegram。本轮是功能推进，不构成完整四端生产放行。

This record distinguishes fresh local execution, exact-source historical records, Gradle cache reuse and external acceptance. It covers actual local task comments, fixed-version evidence, explicit mentions and inbox navigation. H5 recordings are browser evidence of shared mobile source; they do not establish Android or mini-program device readiness.

## 1. 实际执行 / Executed checks

| 编号 / ID | 检查 / Check | 结果与来源 / Result and evidence |
| --- | --- | --- |
| V95-01 | 全量原生后端 / Full native backend |171顶层、1133结果、9批，全部发现项执行；[原始结构化结果](evidence/task-mentions-backend/result.json)。后端摘要 `a296d5bad22b83e42f3aac30e2a27b3d9d2ca6608ac6120c59a99783d76cb52e`。 |
| V95-02 | 客户端 / Clients | 新执行类型检查、Web135／移动238／runtime51项、Web／runtime构建及微信／抖音构建，共8条命令；[执行记录](evidence/task-mentions/client-checks.json)。 |
| V95-03 | 小程序 / Mini-programs | 微信与抖音各20页，包含既有文件中心及任务详情；讨论在原详情页中，不增加注册页。[移动摘要](evidence/task-mentions/mobile-build.json)。 |
| V95-04 | Android构建 / Android build | 生成当前源码开发APK；18项包检查通过。[构建](evidence/task-mentions/build.json)、[检查](evidence/task-mentions/apk-inspection.json)。不提交APK或密钥。 |
| V95-05 | JVM缓存范围 / JVM cache scope |4份JUnit XML共26项无失败；Gradle任务为 `:app:testDebugUnitTest UP-TO-DATE`，本轮**未重新执行**26项。[实际范围与摘要](evidence/task-mentions/android-test-scope.json)。 |
| V95-06 | Linux交叉编译 / Cross-compile | 当前Go源码编译 `linux/amd64` 成功，未执行、未部署；[记录](evidence/task-mentions/linux-build.json)。 |
| V95-07 | Web真实流程 / Actual Web flows |8组，两名用户、两个浏览器上下文；[结果](evidence/task-mentions-web/result.json)、[完整启动来源](evidence/task-mentions-web/source-manifest.json)。 |
| V95-08 | Taro H5真实流程 / Actual H5 flows | 同8组，390×900；[结果](evidence/task-mentions-mobile/result.json)、[完整启动来源](evidence/task-mentions-mobile/source-manifest.json)。 |
| V95-09 | 契约与迁移 / Contracts and migrations | OpenAPI286路径／355操作／625模型；文档与嵌入契约一致、TS类型重新生成。新增迁移76，原75项保持已发布94字节。 |
| V95-10 | 原生开发环境 / Native development | 加76迁移、按进程归属更新API／通知及报表worker；[更新](evidence/task-mentions/development-update.json)。只读核对76迁移／当前来源／零隔离库／外部接入关闭：[资源](evidence/task-mentions/development-project-resources.json)、[运行时](evidence/task-mentions/development-runtime.json)。不是服务器生产部署。 |
| V95-11 | 来源与媒体 / Sources and media |8张实际PNG、两套MP4／GIF、当前完整955项启动来源；[媒体摘要](evidence/task-mentions/media.json)、[直接像素复核](evidence/task-mentions/visual-review.json)、[最终来源检查](evidence/task-mentions/final-source-check.json)。 |
| V95-12 | 仓库发布 / Publication | 检查双语说明、链接、源码／证据、已知本机秘密、禁入路径及待提交字节；[候选检查](evidence/task-mentions/repository-check.json)。另执行staged检查，再提交推送。 |

Web、移动与runtime摘要由同一份真实8命令执行记录整理，不代表额外跑了三轮。[整理器](../../app/scripts/freeze_task_mentions_clients.py)核对来源和原始日志摘要，标记 `adapter_reran_tests: false`。所有测试执行均使用原生工具，没有Docker、WSL、虚拟机或模拟器。

The client summaries derive from one fresh eight-command run; adaptation does not rerun tests. The Android artifact is newly built, while the26 JVM results are explicitly cached. Linux is compilation only. Existing daily native development services remain available; owned review browsers, contexts, fixtures and temporary servers are closed after checks.

## 2. 八组真实浏览器流程 / Eight actual browser flows per client

| 编号 | 行为与实际核验 / Behavior and readback |
| --- | --- |
| B95-01 | 在真实UI选择历史正式文件版本1（当前文件已到版本2），明确选择非负责人bob，预览／确认创建。准确评论及不可变历史核对正文、附件、成员、身份、修订与时间。 |
| B95-02 | 作者仅改正文，省略关联字段保留原附件与提及；读取新叶修订，通知不重复投递。 |
| B95-03 | 通过实际API准备22条标记分页样本，再用被提及者UI打开通知：准确加载不在首20条的当前评论，并由应用自动定位；检查正文实际进入可视区域，测试脚本不代滚动。 |
| B95-04 | 使用真实评论历史入口，核对原文字及原固定文件版本。 |
| B95-05 | 明确标记的传输故障：真实修订已提交，浏览器从201响应移除两个提及字段。客户端拒绝判定成功，保留文字并阻止重复写入。 |
| B95-06 | 明确标记的传输故障：真实创建已提交，随后丢失响应。客户端保留原意图、停止重复提交，实际列表只有一条新评论。 |
| B95-07 | 用户主动重读列表后重新打开表单，仅恢复文字，重新选择／核对并确认相同意图。复用原幂等键对账，读取唯一已提交记录，没有第二条创建。 |
| B95-08 | 实际移除bob项目成员资格，原会话失效；打开中的讨论清空并回到登录。浏览器上下文与隔离本机服务正常清理。 |

Both clients use the real API and database for all eight flows. Only the two labelled response-fault cases intercept transport after an actual commit. The notification recipient is not the task assignee. Pagination setup uses22 explicit synthetic API records; no client success is faked. Screenshots include review, committed discussion, recipient inbox and exact-comment navigation; the short video captures creation and its committed result, not every fault scenario.

## 3. 后端边界 / Backend boundaries

新增集成检查覆盖：字面 `@姓名` 不投递；明确成员选择与当前权限；自我、重复、跨组织、非成员及超过20个标识的拒绝；知识评论禁止任务提及；成员搜索／任务／查询绑定游标；当前评论读取；不可变数组、单后继与数据库直写约束；新提及投递去重；正文修改不重复提醒；移除／重新加入不复活旧上下文；已排队事件在投递时再核对；撤权过滤与原幂等回执重新授权；仅改正文保留不可见原提及，明确替换则拒绝；标记已读不改变任务。

Integration tests exercise delivery from committed comments through the existing leased notification consumer, current access at dispatch/read time, continuous recipient inclusion across successor revisions, exact current links and hidden-association preservation. Shared contract tests validate request bounds, predecessor identity, canonical routes and complete receipts. No external Telegram/email/push sender is enabled by this change.

## 4. 媒体与准确来源 / Media and provenance

当前启动／构建／浏览器来源摘要为 `55d3b07c38e183e9b9a7cde936230e8a49ea3f29880c0fe8564a944f50448a89`，955项。来源记录同时覆盖Go／SQL、Web、移动、共享包、runtime、Skill、部署配置、锁文件以及实际启动／记录／检查脚本，前后摘要一致。

| 客户端 | 实际捕获 | 导出与范围 |
| --- | --- | --- |
| Web |1365×900，28捕获帧，原时长5669ms | H.264因偶数宽度导出1364×900／5.8秒；GIF960宽、1.0× |
| Taro H5 |390×900，27捕获帧，原时长5777ms | MP4为390×900／5.9秒；GIF390宽、1.0× |

无声MP4按原始帧间隔编码，GIF未加速。PNG、视频与GIF使用合成业务信息，登录／MFA完成后开始，不含密码、MFA秘密、会话材料或私密证明能力。8张PNG和两GIF首／中／末帧均直接查看；准确通知评论正文在可视区内。原始JPEG帧仍忽略；公开摘要保存帧哈希、时间和编码器信息。媒体目录合计90 PNG／33 MP4／33 GIF。

Current development APK:42,940,937 bytes, SHA-256 `30eb371f3f1c7dc0cfd47b712cd879325bce96de04f2984e4394d14e8bc8a97d`; source digest `85b2eec0d19a1525dbd16f1c78859179bfeafbcff1ea8c86e457ef2c81f27dce`. The content-addressed artifact remains ignored locally. A development build does not establish installation, background-transfer, signing or distribution acceptance.

## 5. 原诊断与历史 / Retained diagnostics and history

[已发布94来源及入口](evidence/task-mentions/history/published-94/provenance.json)以Git提交 `e0b715929418644c61dd1367608383f6d64d1137` 为来源，保存应用ZIP、原后端证明及README／台账／差异／媒体入口快照。先前正式媒体保持原字节。

本轮9次浏览器诊断／被替代采集保存在 `evidence/task-mentions/history/browser-attempts/`，每次保留当时报告、原状态、完整来源ZIP与已生成素材：

| 原尝试 | 保留的结论与修正原因 |
| --- | --- |
|01–03 Web | 原测试选择器及隔离测试身份MFA设置问题，未完成业务流程；保留原诊断。 |
|04–05 Web | 浏览器检查曾误用Bearer假设、后又缺少Cookie请求来源；更正检查器使用真实Cookie／CSRF／Origin，原结果不改。 |
|06 Web | 原7组流程已通过，直接像素复核后补原样式与表单草稿绑定；该通过结果被替代，不称失败。 |
|07 Mobile | 前3组通过后，检查器把Taro文本入口误作button查找；改为实际文本入口，保留原失败。 |
|08–09 Web／Mobile | 原8组均通过，但图片显示准确评论仍在首屏下方；保存原来源及PNG／MP4／GIF后，补应用自动定位及可视边界断言，再完整重新采集。 |

Earlier client checks, pre-focus Android build evidence and original media metadata are preserved under the same history directory. The first client build lacked an explicit local API origin; later complete runs were superseded by source changes. An interrupted backend run remains incomplete; the current nine-batch record is complete. Two readonly development diagnostics retain a missing wrapper field and two idle test databases, followed by an explicitly scoped, no-FORCE cleanup and a fresh zero-test-database check. None of these original conclusions are rewritten as successful current evidence.

最终汇总检查器另保留两次格式诊断：首次把Go结构化事件的 `Action` 字段当作 `passed`，随后错误地在OpenAPI相对路径中包含 `/v1`（该前缀属于服务器基址）。原脚本及失败报告分别保存在 `history/source-verifier-01/`、`history/source-verifier-02/`；修正读取格式，不修改原后端或契约证据。

首轮仓库候选检查把搬迁后的原94 Markdown入口快照当作当前导航，产生历史相对链接错误；原结果与检查器／基线保存在 `history/publication-01/`。快照改用 `.md.snapshot` 标记原始历史文本，并在原路径清单中记录SHA-256，内容字节不变；当前文档仍执行完整链接检查。首次生成的检查报告随后存在，消除其自身的初次链接缺项。

## 6. 复核与重建 / Reproduction

在仓库根目录使用原生Python／Node／Go／PostgreSQL，先按双语指南安装依赖与工具。浏览器脚本创建临时隔离数据库及自有服务，结束后清理；勿与源码修改并行。运行录制前应把现有证据按原状态归档，录制器拒绝覆盖已有结果。

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_backend_batches.py --batch-size 20 --evidence-group task-mentions-backend
python app/scripts/verify_task_mentions_clients.py
python app/scripts/freeze_task_mentions_clients.py
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python app/scripts/build_android.py --google-mirror --evidence-group task-mentions
python app/scripts/inspect_android_apk.py --evidence-group task-mentions
python app/scripts/freeze_task_mentions_android.py
python app/scripts/run_task_mentions_browser.py
python app/scripts/run_task_mentions_browser.py --mobile
python app/scripts/encode_task_mentions_media.py
python app/scripts/verify_task_mentions_development.py
python app/scripts/verify_task_mentions_sources.py
python app/scripts/verify_repository_publication.py
python app/scripts/verify_repository_publication.py --staged
```

`update_task_mentions_development.py` 只用于归属明确的本机开发环境升级，不是生产部署命令。它先核对进程身份、启动时间及原来源，再执行追加迁移并更新自有API与两个worker；不启动虚拟化，也不替换无关进程。媒体视觉复核须实际查看后记录，不能由构建结果代替。

## 7. 尚未完成的验收 / Outstanding acceptance

微信／抖音主体与域名、平台审核及真机；Android设备、后台上传、断点恢复、正式签名和分发；真实OSS／R2、2TB迁移、120GB传输与副本恢复；香港到境内网络；真实Codex模型及TG网络送达；外部通知与生产监控余项；生产部署、恢复演练和依赖安全放行均继续按原28项差异推进。八项Skill仍draft。相同锁文件仅引用[原90审计](evidence/dependency-hardening/dependency-audit.json)：86项，0 critical／20 high／64 moderate／2 low，未重新扫描且生产安全gate未通过。

The full four-client goal remains active. Local passing tests, a development APK and synthetic recordings do not replace these external and production acceptance records.
