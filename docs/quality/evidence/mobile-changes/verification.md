# 增量80验证记录 / Increment80 verification

实现、完整使用逻辑、接口及约束见[移动项目变更与预算修订](../../../development/80-mobile-project-changes-and-budgets.md)。本轮使用原生Windows、隔离PostgreSQL、实际身份／MFA及原HTTP业务命令与Taro H5。不调度Docker、WSL、虚拟机、模拟器或子Agent。

This increment connects the existing change/budget protocol to the shared mobile implementation. Checks use actual original APIs, authentication/MFA and browser pixels with synthetic business records. No real model, cloud, Telegram delivery or physical device is invoked.

| 范围 / Scope | 实际结果 / Result | 证据 / Evidence |
| --- | --- | --- |
| 移动逻辑 / Mobile logic | 154项通过，0失败／取消／跳过，新增16项输入／来源证明及1项有界草稿测试 / 154 passed, including 17 new boundary/draft tests | [结果](mobile-build.json)、[原日志](mobile-verification.log) |
| 类型与小程序 / Types and mini-programs | 类型及双目标通过，各16页；微信65文件／1,038,055字节，抖音62文件／977,760字节 / Fresh type check and both builds | [四阶段结果](mobile-build.json) |
| 实际页面 / Actual UI | 10组原API／H5流程，3页×320／390／768共9项布局 / Ten actual flows and nine layouts | [页面结果](../mobile-changes-browser/result.json)、[完整来源](../mobile-changes-browser/source-manifest.json) |
| Android开发APK / Development APK | 42,858,139字节；17项检查，离线图含变更表单、详情、输入与异步证明模块 / New APK and seventeen inspections | [构建](build.json)、[包检查](apk-inspection.json) |
| 原生测试 / Native tests | 25项结果由Gradle UP-TO-DATE保留，本轮未新执行 / Cached results, not new execution | [范围](android-test-scope.json)、[当次日志](native-build.log)、[4份XML](android-unit-results/) |
| 纯Java / Pure Java | 未变完整来源，准确复用66的25项／4份XML / Exact unchanged source proof reused | [原始证明](../android-upload/jvm-source-proof.json) |
| 后端／运行时／Web / Backend/runtime/Web | 完整来源未变，复用74的146顶层／919条结果／832末级、39运行时、81Web及构建 / Reused at matching complete source digests | [后端](../deliverable-creation/result.json)、[运行时](../deliverable-creation/runtime-build.json)、[Web](../deliverable-creation/web-build.json) |
| Linux | 同一完整后端来源及原产物摘要；未重建、执行或部署 / Existing artifact, not newly built or deployed | [原编译](../deliverable-creation/linux-build.json) |
| 日常环境 / Daily environment | 新69项迁移摘要、10只读探针、原API／Worker身份、0测试库，日常服务未重启 / Fresh read-only checks | [通用检查](development-runtime.json)、[当前约束](development-project-resources.json) |
| 媒体 / Media | 新3张PNG、35个真实源JPEG／7178ms，编码7.3秒MP4和44帧GIF；实际查看PNG及GIF0／22／43帧 / Actual pixels and explicitly sampled review | [媒体](media.json)、[原帧清单](recording-frames.json)、[来源](recording-provenance.json)、[查看](visual-review.json) |
| 最终复算 / Final checks | 22项当前源码、原证明、产物、媒体及所属资源核对通过 / Twenty-two passed | [来源检查](final-source-check.json) |
| 仓库 / Repository | 候选及实际暂存区检查双语覆盖、文件组织、链接、敏感信息、来源、媒体与原字节 / Candidate and actual-index preflight | [候选报告](repository-check.json)；[工具](../../../../app/scripts/verify_repository_publication.py) |

## 1. 来源及实际执行范围 / Source identity

| 范围 / Scope | SHA-256 |
| --- | --- |
| 135项移动来源 / Mobile sources | `879c48e31ad23d8cb9f565d17c750e6018100b53ffc2cc295958c3a15e586113` |
| 604项夹具／H5／浏览器来源 / Fixture, H5 and browser sources | `75c46bcfc7965ccfce612b324e05190cab66092a6f09ed6e3afa6d5dd59034d6` |
| Android完整来源 / Complete Android sources | `8f428980ea6bab1cd4e944227c36bd27b57f015c3236d0998a643682c700fe70` |
| 冻结APK / Frozen APK | `4263eb232c2605f2e5f079ab8bab76030b2a2de9fd8dfc5946cd13fe555fda6e` |
| 完整Go／SQL／契约 / Complete backend | `61754e7e60bdac1ad937c5890330de0fa872deaa72d116f428348ef46bdfb10d` |

以上清单范围不同，摘要不能互比。最终移动检查从 2026-10-26T14:33:51.616109+00:00 至 2026-10-26T14:36:59.750113+00:00 完成，期间来源未变；浏览器从原生夹具前、H5编译后至页面结束的604项来源一致。Android完整来源在JS打包、原生编译及冻结期间一致。APK留在忽略的 app/.local/android/artifacts/mobile-changes/，不提交SDK、签名私钥、缓存或APK。

The manifests cover different scopes. Each run establishes source stability within its own scope. The actual native task is `> Task :app:testDebugUnitTest UP-TO-DATE`. The freezer preserves the exact native log, four original XML files and the completed mobile log; cached native results, pure-Java historical results and reused backend/runtime/Web checks are not reported as newly executed.

## 2. 十组原业务流程 / Ten original-service flows

1. 页面创建完整提案及100.01替换预算，选择准确历史正式文件；读回固定草稿，项目及当前批准预算不变。
2. 编辑产生120.01不可变修订，明确更新原申请草稿；原100.01提案和申请修订仍保留。
3. 原预览与明确送审，由另一真实身份独立审批准确修订；批准本身不改变项目／预算。
4. 两项人工核实和本次本人MFA后实施，读回项目版本加一、准确实施记录与120.010000目标预算。
5. 单独批准并实施保持模式 null，核对原预算ID和总额完全保留。
6. 单独批准并实施清空模式 []，核对新零行批准预算和已取代的真实前序；币种／状态筛选读取原历史。
7. 控制性移除一次真实成功响应的rid字段；原写入只发生一次，页面保留输入、不显示成功且禁止重放，原API读回唯一提案。
8. 原API推进项目版本产生真实409；保留文字，显式重读等待准确新名称／版本后重新采用依据，再保存新修订。额外原命令产生13份修订，十份游标分页并读回原第1份内容。
9. 控制性403刷新清除缓存变更、历史和写入入口；该故障注入本身不宣称真实权限撤回。
10. 原接口实际移交负责人并降为viewer；重新真实登录及MFA后，直达表单、详情与列表均不提供本项目新建／编辑／准备入口。

Original `POST /changes/{id}/submit` returns 201 and creates a request draft; explicit original request revision returns 200. Neither command silently submits an approval. Independent review and implementation remain separate. Controlled incomplete-response/403 injection is identified separately from real version conflicts and actual membership changes.

原实现成功判断不仅核对三字段回执，还核对原不可变提案、固定文件、本人、原因、完整基线、准确批准申请、实际项目、当前预算及已取代前序。证明窗口最多读取各状态100份；找不到准确预算时保持结果未核对，不取另一份预算替代。完整历史入口继续按原cursor分页。

Success reconciles actual original facts rather than accepting a receipt alone. The bounded proof window never substitutes a different budget. All five owned contexts, the temporary H5 server, private fixture and isolated database were cleaned up; browser/server/cleanup error arrays are empty. Existing daily services were preserved.

## 3. 媒体与失败历史 / Media and retained attempts

录像是初次100.01提案的真实保存过程；后续截图分别展示修订后的120.01获批提案和独立清空后零行预算，不剪接成一次审批／实施。35帧原始JPEG仍在忽略目录，公开清单保留每帧时间、SHA及原采集条件。输出无声、不生成界面，7.3秒是编码后的时长。实际查看三个PNG与44帧GIF的0／22／43采样，未见密码、验证码或令牌；公开合成业务ID和摘要保留。该抽样不等同于全像素审计或设备验收。

The recording covers initial drafting, with later revisions shown in separate screenshots. Media are actual shared-H5 pixels at automation speed. Capture preconditions, isolation, credential scanning and manual sampling establish the stated exclusion scope; they are not exhaustive pixel OCR.

| 原始尝试 / Attempt | 结果及处理 / Result and correction |
| --- | --- |
| [mobile-01](attempts/mobile-01/README.md) | 初始类型／测试夹具及行ID收窄问题，补正式测试入口并修正；保留原报告／日志 / Initial type, fixture and test-registration corrections |
| [mobile-02](attempts/mobile-02/README.md) | 当时154项及双目标通过，但在最终验证消息／历史分类修正前；属被取代来源，不计最终证明 / Passing but superseded source, not final proof |
| [browser-01](attempts/browser-01/README.md) | 检查器将原201误设为200；依原服务修正断言 / Incorrect expected status corrected |
| [browser-02](attempts/browser-02/README.md) | 对已缓存Taro页面直接改hash保留旧路由上下文；改用实际页面按钮 / Reused page navigation corrected to actual UI actions |
| [browser-03](attempts/browser-03/README.md) | 六条主流程已过，检查器在预算列表查找详情返回按钮；改用该页真实提案入口 / Incorrect page/button assumption corrected |
| [browser-04](attempts/browser-04/README.md) | 七条流程已过，真实409后的重读未完成就断言；等待准确新名称／版本与稳定页面 / Reread synchronization corrected |

失败／被取代报告、原日志及截图均保留原始字节，不统一换行或改写旧结果。最终通过记录只使用最终摘要；不会把历史失败转记为通过。

Increment77 media retain commit `9e87869397112e0807dd28abdbc691c7b5abb15b`, increment78 task media retain `7a31bc84c4470862659d22733b1434d318b9a8df`, and increment79 closeout media retain `8f77924a06cfd3141c9d1b9cd1c574bfa2191d89`. Original sources/reports are preserved; only the new change media bind this increment's 604-source capture.

## 4. 仍未验收 / Remaining acceptance

八项Skill仍draft，真实集成关闭。114项历史依赖问题（11 critical、32 high）仍未解除；本轮未升级依赖，不把原审计当作新在线安全检查。真实模型／隔离／Skill发布、云／TG／平台／设备、120GB／2TB、完整业务与四端矩阵、安全／性能、生产部署／恢复继续按[28项稳定差异](../../../delivery/03-goal-gap-assessment.md)推进。

All eight Skills remain draft. This is source-bound local development evidence, with exact reuse where code is unchanged. It does not establish full-goal completion or production readiness. Reproduction commands are in the [implementation protocol](../../../development/80-mobile-project-changes-and-budgets.md); inspect newly generated media before updating its visual-review receipt.
