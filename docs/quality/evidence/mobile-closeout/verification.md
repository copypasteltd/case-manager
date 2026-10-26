# 增量79验证记录 / Increment79 verification

实现与使用逻辑见[移动项目结项与固定归档](../../../development/79-mobile-project-closeout-and-archive.md)。本轮使用原生Windows、隔离PostgreSQL、真实账号／MFA、原始HTTP业务命令与实际Taro H5；未调度Docker、WSL、虚拟机、模拟器或子Agent。

Mobile closeout and archive connect to the existing original service. Native Windows checks used isolated PostgreSQL, actual authentication/MFA and actual API/H5 interactions. No virtualization, emulator, subagent, real model, cloud or Telegram delivery was used.

| 范围 / Scope | 实际结果 / Result | 证据 / Evidence |
| --- | --- | --- |
| 移动逻辑 / Mobile logic | 137项通过，0失败／取消／跳过；新增12项完整依据、权限、阶段、附件、回执／回读及1项有界草稿测试 / 137 passed, including 13 new boundary/draft tests | [移动构建](mobile-build.json) |
| 类型与两小程序 / Types and mini-programs | 类型通过；微信65文件／988,703字节、抖音62文件／928,408字节，各16注册页面 / Types and both 16-page builds passed | [四阶段结果](mobile-build.json) |
| 实际页面 / Actual UI | 9组原API／H5流程、3页×320／390／768共9项布局通过 / Nine actual flows and nine responsive checks | [页面结果](../mobile-closeout-browser/result.json)、[完整来源](../mobile-closeout-browser/source-manifest.json) |
| Android开发包 / Development APK | 新APK 42,842,779字节；17项检查，离线图包含结项页面、核查及异步命令模块 / New APK and 17 inspections | [构建](build.json)、[包检查](apk-inspection.json) |
| 原生测试 / Native tests | 实际Gradle任务UP-TO-DATE，25项通过结果被保留，本轮未新执行 / 25 retained results, not newly executed | [执行范围](android-test-scope.json)、[当次日志](native-build.log)、[4份XML](android-unit-results/) |
| 纯Java / Pure Java | 完整来源未变，准确复用增量66的25项／4份XML，不计本轮执行 / Exact unchanged source proof reused | [原始证明](../android-upload/jvm-source-proof.json) |
| 后端／运行时／Web / Backend/runtime/Web | 完整来源未变，复用74的146顶层／919条结果／832末级、39运行时、81Web与构建，本轮未全量重跑 / Reused only at exact complete source digests | [后端](../deliverable-creation/result.json)、[运行时](../deliverable-creation/runtime-build.json)、[Web](../deliverable-creation/web-build.json) |
| Linux | 原后端完整来源及产物摘要一致，未重建、执行或部署 / Same artifact, not rebuilt, executed or deployed | [原编译](../deliverable-creation/linux-build.json) |
| 日常原生环境 / Daily environment | 新69迁移摘要、10只读探针、原API／Worker身份、0测试库；日常服务未重启 / Fresh read-only checks, no restart | [通用检查](development-runtime.json)、[当前约束](development-project-resources.json) |
| 媒体 / Media | 新3张PNG、17真实JPEG帧／3459ms原录制，编码3.6秒MP4和22帧GIF；实际查看三图及GIF0／11／21帧 / Actual pixels, recorded timing and explicit sampling | [媒体](media.json)、[原帧清单](recording-frames.json)、[来源](recording-provenance.json)、[查看](visual-review.json) |
| 最终复算 / Final checks | 21项源码、产物、原证明、媒体与所属资源核对通过 / Twenty-one passed | [来源检查](final-source-check.json) |
| 仓库 / Repository | 候选及实际暂存区的敏感信息、双语覆盖、链接、媒体、来源和原字节检查 / Candidate/index publication preflight | [候选报告](repository-check.json)、[工具](../../../../app/scripts/verify_repository_publication.py) |

## 1. 来源及准确执行范围 / Source identity

| 来源范围 / Source scope | SHA-256 |
| --- | --- |
| 131项移动来源 / Mobile sources | `bfeb85bd0cb07ab936ecb71b3f620f9d66b6dc617cf1b538c5c80d2e91a23583` |
| 599项原生夹具／H5构建／浏览器来源 / Fixture, H5 and browser sources | `e4792f2a402030067555f840be7a670a1b6ca983989823b0bf6c684ae0a4af59` |
| Android完整来源 / Complete Android sources | `1ee595dbc453b58b69b27cc6dafec7aba6729b8adc7eca32e90ca43333457f3c` |
| APK | `a8bdb19b7500198d2618c79b080e09abaabec58723c31954480bb64c86ed5219` |
| 完整Go／SQL／契约 / Complete backend source | `61754e7e60bdac1ad937c5890330de0fa872deaa72d116f428348ef46bdfb10d` |

各清单范围不同，SHA不能互比；浏览器从原生夹具前、H5构建后到实际页面结束，来源始终一致。新APK冻结到忽略目录app/.local/android/artifacts/mobile-closeout/，未提交APK、SDK、密钥或缓存。原生任务准确行为为 `> Task :app:testDebugUnitTest UP-TO-DATE`，当次日志及4份XML保留原字节和摘要。原生结果、纯Java旧证明、后端／运行时／Web旧测试不记作本次新执行。

The manifests have different scopes. The browser fixture, H5 build and actual UI used unchanged sources. The frozen APK stays in the ignored artifact store. Cached Gradle results and unchanged historical proofs are not new test execution. All eight Skills remain draft; real integrations remain disabled.

增量77媒体继续使用9e87869历史提交的711项Git来源及1项正确忽略的生成缓存摘要；原始采集报告和[不可变来源补充](../mobile-tasks/historical-capture-sources.json)不改写。增量78任务截图／录屏及其596项来源属于7a31bc84c4470862659d22733b1434d318b9a8df，保留原报告。此次3张结项图和新MP4／GIF绑定本次599项准确来源，不将旧图冒充重录。

Initial media retain their historical Git/cache provenance. Task media retain increment78 commit 7a31bc8 and their original reports. Only the new closeout images/recording bind this increment's 599-source manifest.

## 2. 实际业务与失败场景 / Actual flows

1. 项目详情进入结项，原接口启动、暂停、恢复；准确版本和目标阶段回执逐项核对。
2. 创建实际未完成任务，十三类完整清单显示真实阻塞；进入结项被阻止。原记录链接可读，明确处置任务后独立进入closing。
3. 当前表单填复盘、两项声明及MFA后，另一个真实命令推进项目版本，旧表单归档409且未产生历史。文字保留，显式重读恢复输入并重置声明／MFA。
4. 真实201归档回执须经过实际项目／历史异步证明；展开固定复盘后实际重开，旧archive ID、复盘及创建时间仍保留，依据标记changed。
5. 使用原命令完成11次独立归档，以实际游标从10份载入11份历史，没有伪造分页、截断或重复记录。
6. 原事务确实提交后，仅对浏览器历史回读注入缺失条目。页面不宣告成功、不清空输入、不重放；独立原API确认恰好一份历史，显式重读回到实际archived状态。
7. 原事务提交后中断浏览器归档响应，恰好一次写入和一份历史；页面保留文字并阻止重复，显式核对真实状态。
8. 缺少完整依据权限的身份只看到受限null计数；历史无内容、数量或游标。故障注入错误项目ID后不打开工作区。
9. 真实移交负责人，再把原负责人降为viewer；重新MFA登录后无阶段／归档写入口。所有角色变化使用原授权接口。

These are actual original-service mutations and reads. Fault injections remove only a browser response/proof or replace a returned scope to verify rejection; they never fabricate a passing business transaction. The API independently confirms committed records. Optional attachment ordering, full metadata and large decimal byte text are covered by the new portable tests and the exact unchanged backend file-pinning proof; this browser fixture archived without attachments.

## 3. 媒体与清理 / Media and cleanup

三张公开PNG均为390×900真实视口，表单截图展示三项复盘文字，声明／MFA位于更下方；历史截图展示实际固定复盘可见区域。MP4以10fps保留时间间隔，GIF为6fps／1倍，自动动作很快，可慢速查看MP4。录制开始时验证码已清除；实际查看PNG与GIF首、中、末帧，无可见凭据。该视觉抽样不是所有像素OCR或真机验收。公开真实帧清单保留每帧时间与SHA，原JPEG和私有登录夹具保持忽略。

最终5个所属上下文全部关闭，浏览器关闭，原生夹具退出0，临时端口5189释放，测试库0，私有夹具已删除。日常API／通知Worker仍为原所属进程；未接管无关浏览器、进程、盘符或服务。原生构建没有启动模拟器／虚拟化。

## 4. 保留尝试 / Retained attempts

| 尝试 | 结果与修正 |
| --- | --- |
| [移动01](attempts/01-membership-test-shape/) | 首次新测试未提供成员authorization_version；按实际契约修正测试数据。保留失败报告、原日志及当时测试源码。 |
| [移动02](attempts/02-refill-effect-syntax/) | 首次页面类型检查发现ref泛型闭合错误；修正源码。137项逻辑已通过的旧报告不冒充页面编译通过。 |
| [浏览器01](../mobile-closeout-browser/attempts/01-original-scope-guard/) | 原后端正确拒绝已启动项目直接改scope；改为允许的名称更新制造并发版本，保留原规则。 |
| [浏览器02](../mobile-closeout-browser/attempts/02-explicit-refresh-after-external-cycles/) | 原命令外部创建11次历史后，同一路由不自动刷新；检查器改为显式点击重读。保留该次实际图、帧清单和源码，之后重新完整采集。 |
| [发布03](attempts/03-first-publication-report/) | 首次候选枚举时自身报告尚未生成，报告链接未包含在候选中；保留该报告后再次执行完整检查，不跳过链接检查。 |

两次浏览器失败均在finally关闭所属浏览器／服务和原生夹具，数据库清理通过。旧日志、失败源码和截图保持原始字节；仓库只对原始证据路径豁免空白检查，未豁免链接、敏感信息、媒体或来源校验。最终portable报告替换早期成功来源，原始失败仍保留；本次页面退出时的异步证明请求也纳入所属Scope取消。

## 5. 复现与同步 / Reproduction and publication

从仓库根目录，在原生工具和私有开发环境准备好后按顺序执行。浏览器脚本拥有同名夹具与端口，不并发运行其他页面夹具；不编辑正在构建或采集的来源。

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_mobile.py mobile-closeout
python app/scripts/run_mobile_closeout_browser.py
# Native ffmpeg/ffprobe must be on PATH.
python app/scripts/encode_mobile_closeout_media.py
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python app/scripts/build_android.py --google-mirror --evidence-group mobile-closeout
python app/scripts/inspect_android_apk.py --evidence-group mobile-closeout
python app/scripts/verify_mobile_closeout_development.py
```

冻结本次Gradle日志／XML及准确执行范围，实际查看新媒体并更新查看记录后，运行最终来源检查和发布检查：

```powershell
python app/scripts/verify_mobile_closeout_sources.py
python app/scripts/verify_repository_publication.py
# Stage the corresponding code, documentation, media and updated report together.
python app/scripts/verify_repository_publication.py --staged
git diff --cached --check
```

现有用户授权要求本轮代码、双语README、媒体、文档及更新说明一起提交推送到git@github.com:copypasteltd/case-manager.git，后续完成更新同样同步，不强推。候选检查记录实际待发布内容，暂存检查读取真实Git索引并逐字节比对工作区；最终推送后核对远端main与本地提交一致，收据保存在忽略目录。

移动变更实施／预算、更多完整模块、实际模型／隔离／Skill安装、真实云／TG／平台／设备、120GB／2TB、安全／性能、生产部署／恢复及完整验收继续开放，见[28项稳定GAP](../../../delivery/03-goal-gap-assessment.md)。此处检查不表示完整Goal或生产放行。
