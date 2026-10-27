# 增量85验证 / Increment85 verification

本轮实现秘书完整授权创建任务／子任务、原任务命令与回执、Web／Taro完整表单、Schema、数据库约束及演示。代码对应当前源码清单；先前失败和重录保留在 `attempts/`。正式四端首发仍未完成。

This increment implements exact complete secretary task/subtask creation, original task commands/receipts, Web/Taro forms, schemas, database guards and actual demos. Current evidence binds source manifests; failed and superseded attempts remain under `attempts/`. Full four-client launch is not complete.

## 1. 当前检查 / Current checks

| 范围 / Scope | 结果 / Result | 记录 / Record |
| --- | --- | --- |
| 原生后端 / Native backend | 157顶层、1,014条结果／920末级、8批全部通过；71份迁移。157 top-level tests,1,014 results/920 leaves, eight passed batches,71 migrations. | [结果](result.json)、[批次](batch-progress.json) |
| 本轮业务 / New business integration | 12组完整任务创建场景通过，使用真实原生PostgreSQL、HTTP及MCP；无真实模型。12 actual native PostgreSQL/HTTP/MCP scenarios; no real model. | `TestAgentTaskCreation/*` in [result.json](result.json) |
| Web | 95测试与生产构建通过，135来源文件，前后摘要一致。95 tests/build passed,135 source files, unchanged during checks. | [Web](web-build.json) |
| 运行时 / Runtime | 45测试、Skill校验与构建通过，52来源文件。45 tests, Skill validation/build,52 sources. | [Runtime](runtime-build.json) |
| Taro | 187测试全部通过，两份实际TAP汇总为178+9；类型检查、微信／抖音各16页编译通过，160来源。187 tests, two real TAP suites178+9, types and16 pages per mini-program,160 sources. | [Mobile](mobile-build.json)、[原日志](mobile-verification.log) |
| Android | 新开发APK42,888,501字节；17项包检查。Fresh development APK42,888,501 bytes;17 package inspections. | [Build](build.json)、[Inspection](apk-inspection.json) |
| JVM | 25项保留结果；Gradle实际任务为`UP-TO-DATE`，来自缓存，非本轮重新执行。25 retained results; actual Gradle task UP-TO-DATE, cached rather than newly executed. | [范围](android-test-scope.json)、[日志](native-build.log)、[XML](android-unit-results/TEST-hk.copypaste.workspace.ApiHTTPTest.xml) |
| Linux | 当前后端交叉编译通过，未执行、未部署。Current backend cross-compiled; neither executed nor deployed. | [Linux](linux-build.json) |
| Web实际页面 / Actual Web | 7流程、3视口、626来源，MFA／API／完整表单／真实发送／重试／取消及自建资源清理通过。Seven flows, three layouts,626 sources and owned cleanup. | [结果](../task-creation-web/result.json)、[清单](../task-creation-web/source-manifest.json)、[采集](../task-creation-web/capture.json) |
| Taro实际页面 / Actual Taro H5 | 7流程、3视口、649来源；使用实际Taro H5和原生API，非小程序／Android实机。Seven flows, three layouts,649 sources; actual H5/native API, not device acceptance. | [结果](../task-creation-mobile/result.json)、[清单](../task-creation-mobile/source-manifest.json)、[采集](../task-creation-mobile/capture.json) |
| 日常本地环境 / Owned daily environment | 仅更新已核对身份的API至71份迁移；10只读探针、0测试库／活动模型／启用TG；PG与通知Worker未重启。Only owned API updated;71 migrations,10 read-only probes, zero test DBs/active model profiles/enabled TG; no PostgreSQL/worker restart. | [更新](development-update.json)、[探针](development-runtime.json)、[资源](development-project-resources.json) |
| 最终来源 / Final source consistency | 25项完整来源、证据、媒体及清理检查，结果以机器记录为准。25 complete source/evidence/media/cleanup checks; machine report is authoritative. | [复核](final-source-check.json) |

十二项场景覆盖：完整字段及非法额外授权；真实子任务、review=true、微秒日期和两种重试；null／false；模型改动任一关键字段；普通会话／外项目；项目变化／后续人工改动；非成员及跨项目关联；分配权限而自己／未分配遵守原规则；提交时关闭父任务或里程碑；证据失败完整回滚；取消及追加历史；原登录撤回。

The twelve scenarios cover complete/invalid input, actual reviewed subtasks and two replay modes, explicit null/false, altered model intent, ordinary chat/foreign scope, changed project/human edits, invalid members/links, assignment authority, closed links at commit, evidence rollback, cancellation/append-only history and revoked login.

## 2. 源码摘要 / Source digests

| 来源 / Source | SHA-256 |
| --- | --- |
| 后端完整Go／SQL／契约 / Complete backend | `43736f3ab245d384de58d48b32d0fc1957f5e9d8a6c6b75f91cf0086c7190e4c` |
| Web构建 / Web build | `73978b7032a02c1b7a89b6f3fd2755cfb224928627baea98b6ad960d92ff83d2` |
| Runtime／Skill／SOP | `54300a4be6980a27fbae7bb0c0e9f1f46dc9fccdb1e1ff83fcc4f3c34a0d56f6` |
| 移动构建 / Mobile build | `3f38490b7bc973b217a1a1ca526255ba41969f20be0947085b9d31274dcdc524` |
| Android完整原生／共享来源 / Android complete scope | `afe7a49dd575d0ecf827f184724cba9bd022ecb5378c84340057939c53942ab8` |
| APK实际产物 / Actual APK | `26ffcaf989f978be9daf54ca783a845a9ae764053de1e5319f7885cda4cfd1c0` |
| Web页面采集 / Web capture | `407243b6d15352358fd598ea281ebaf7b7d09cbb9e4a7a4c3d4d8274c81b3583` |
| Taro页面采集 / Taro capture | `8d69eecb3a88b1f81e601003fc107c708adedc554351c3a67553e8e98f6de50f` |
| 两端完整启动／构建来源 / Complete capture startup/build | `f09ef0b641e1060c07a20ac01128d122f89b5f8e5685310b72daffc27ed3fff1` |

前70份迁移与原提交24ad850保持字节一致，第71份追加完整任务创建工具和真实结果约束。OpenAPI两副本一致，271路径／338操作／589 Schema。`project-update`源码1.6.0开放16工具／14准确写工具，八包全部draft。APK留在忽略的本机产物路径，未将二进制或签名材料入库。

Previous70 migrations retain exact bytes from24ad850; migration71 adds complete creation and actual-result guards. Both OpenAPI copies match at271 paths/338 operations/589 schemas. `project-update` source1.6.0 has16 tools/14 exact writes; all eight packages remain draft. APK binaries/signing material stay outside Git.

## 3. 实际页面素材 / Actual media

新增四张PNG和两端各一对MP4／GIF；整个媒体目录为40PNG／13MP4／13GIF。两段录屏均9张真实帧：Web1846ms，移动1838ms；MP4均2.0s，GIF均12帧／2000ms、1×、无声。FFmpeg／FFprobe与产物摘要见[媒体](media.json)，冻结原始帧清单见[Web](web-capture-manifest.json)、[移动](mobile-capture-manifest.json)。原始帧被忽略，不包含密码／MFA／令牌。

Four new PNGs and one MP4/GIF pair per client bring the catalog to40/13/13. Each recording has nine actual frames:1846ms Web,1838ms mobile. Both MP4s are2.0s, both GIFs12 frames/2000ms at1×, silent. Encoder/probe/output digests and frozen frame manifests bind the actual artifacts. Raw frames remain ignored and credential fields are excluded.

人工已查看四张原尺寸视口截图，以及两段GIF的首／中／末帧，见[像素核对](visual-review.json)。保留实际视口裁切，移动回执下部需滚动查看，完整授权另见预览；没有把截图裁切宣称为完整长页。已提交历史来自受控原生业务夹具，真实浏览器本轮新发送固定为authorized后取消，不调用真实模型创建任务。

Manual review inspected the four original viewport screenshots and first/middle/last GIF frames. Images retain actual viewport crops; the mobile receipt continues below its crop while the preview shows all authorization fields. Committed history is produced by the controlled native business fixture. Browser sends create authorized runs and cancel them; they do not execute a real model.

每次采集均关闭1个自建上下文及所属浏览器，停止临时Web／H5服务，释放5189端口，删除私有夹具并结束／删除隔离数据库；未操作用户已有浏览器或S:／R:别名。

Each capture closes its one owned context/browser, stops its temporary server, releases port5189 and removes its fixture/database. User browsers and existing S:/R: aliases are preserved.

## 4. 失败与历史边界 / Failures and historical limits

- [初次专项／类型检查](attempts/initial-focused-checks/diagnosis.json)：首次读取边界断言发现创建运行可读其他项目任务，已收窄到真实新任务；未结束首例造成后续活动运行冲突，未将其当成多个独立业务缺陷。共享参数priority类型修正后两端重新通过；新测试误用依赖列名也已修正。失败Web／移动摘要保留。
- [后端清理超时](attempts/backend-cleanup-01/diagnosis.json)：跨云副本用例的临时PostgreSQL删除超出两分钟，失败批次原状态保留。同一源码摘要续跑第3批及其余批次通过。通过创建时间、原服务器取消DROP日志和零连接证明准确归属后，仅删除该残留合成测试库，见[清理回执](orphan-cleanup.json)。最终只读核对为0测试库，不重启PG或取消持久化。
- [采集缓存重录](attempts/capture-cache-01/diagnosis.json)：首轮两端业务流程通过，但通用采集来源误含忽略的Web编译缓存。原媒体／清单保留为superseded，已排除缓存并将两端全部重录，不重绑旧图为新来源。
- [历史84核对](historical-84.json)：完整启动来源摘要`13e210075540a94db51bd72aca2a1f8fcb215a0ad28b61063d845e9d95b6f785`及移动642项来源可按24ad850原Git重建。旧Web620项中619项Git文件可重建，但原`web/tsconfig.tsbuildinfo`忽略缓存未冻结，完整旧Web采集摘要无法重建；保留旧报告与原摘要，不用当前缓存补造历史证据。

Initial focused/type failures, the native cleanup timeout and superseded cache-bound captures remain explicit. Same-source backend resume passed all discovered tests. Exactly the owned leftover synthetic DB was identified and cleaned. Both clients were recaptured after excluding compiler caches. Increment84 complete startup/mobile sources reconstruct from its original Git commit; its Web capture has one unfrozen ignored compiler cache, so only619 of620 entries reconstruct. The missing artifact and old full-capture digest are not fabricated.

## 5. 命令与尚未放行 / Commands and open gates

```powershell
$env:PYTHONUTF8='1'
python app/scripts/test_backend.py --package ./internal/httpapi -run '^TestAgentTaskCreation$' -v
python app/scripts/verify_backend_batches.py --batch-size 20 --evidence-group task-creation
# A resume is valid only for identical sources; retain the failed batch state first.
python app/scripts/verify_backend_batches.py --batch-size 20 --evidence-group task-creation --resume
python app/scripts/verify_web.py task-creation
python app/scripts/verify_agent_runtime.py task-creation
python app/scripts/verify_mobile.py task-creation
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python app/scripts/build_android.py --google-mirror --evidence-group task-creation
python app/scripts/inspect_android_apk.py --evidence-group task-creation
python app/scripts/freeze_task_creation_evidence.py
python app/scripts/verify_linux.py task-creation
python app/scripts/run_task_creation_browser.py
python app/scripts/run_task_creation_browser.py --mobile
python app/scripts/encode_task_creation_media.py
python app/scripts/record_task_creation_history.py
python app/scripts/verify_task_creation_development.py
python app/scripts/verify_task_creation_sources.py
python app/scripts/verify_repository_publication.py
```

已有证据／采集先检查归档，不能覆盖当前记录。原生API更新脚本仅用于已验证归属且真实集成关闭的本地开发实例，不是生产部署命令；PG与Worker保持运行。没有调度Docker、WSL、虚拟机或模拟器，也未启用真实云／模型／TG。

Archive existing evidence first. The API updater targets only an ownership-verified native development instance with integrations disabled; it is not a production deployment command. PostgreSQL and worker stay running. No Docker, WSL, VM or emulator was invoked and no real cloud/model/TG integration was activated.

完整首发仍待真实设备、平台准入、模型／隔离／Skill发布、TG群、120GB／2TB与OSS／R2、独立PG备份恢复、生产容量及安全放行。锁文件未改，保留依赖审计114项（11 critical／32 high）仍开放。GAP-01／11／12／13／18推进，不因本轮通过而关闭全部目标；见[稳定28项差异](../../../delivery/03-goal-gap-assessment.md)。

Device/platform/model/confinement/Skill/TG,120GB/2TB dual-cloud, independent PostgreSQL recovery and production capacity/security gates remain open. The unchanged lockfile retains the prior114 dependency findings (11 critical/32 high), not a fresh security assessment. This increment advances GAP-01/11/12/13/18 without declaring the whole goal complete.
