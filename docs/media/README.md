# 实际页面与操作演示 / Actual pages and interactions

[中文使用指南](../README.zh-CN.md) · [English guide](../README.en.md) · [仓库首页 / Home](../../README.md)

本目录包含 **26张PNG、7段MP4、7个GIF**：增量77初始15张图及三组录制，78／79／80分别补任务、结项和变更媒体；增量81新增三张真实状态截图及一对原生监控工作流MP4／GIF。全部使用隔离合成业务数据；不含真实合同、员工资料、密码、MFA、私钥或令牌。

This catalog contains **26 PNGs, seven MP4s and seven GIFs**: initial captures from77, task/closeout/change additions in78–80, and three new actual status images plus a native-monitor recording/GIF pair in81. All media uses isolated synthetic records and excludes real business records and credentials.

## 1. Web 项目创建与任务闭环 / Project and task workflow

实际操作：创建项目 → 创建带负责人及日期的任务 → 开始 → 完成 → 打开看板。录制在登录与 MFA 完成后才开始，不录制身份输入。操作由自动化执行，MP4 可使用播放器慢速查看。

Actual actions: create a project, create an assigned/dated task, start and complete it, then open the board. Recording starts after authentication/MFA. Automation performs the actions quickly; use a slower playback rate to inspect individual steps.

![Web项目与任务 / Project and task](gifs/web-project-flow.gif)

[完整 MP4 / Full MP4](videos/web-project-flow.mp4) · 1440 × 900 · 4.2 s · 无声 / silent · GIF 1×

## 2. 业务页面浏览 / Operations tour

依次查看十六类项目概览、合同、实际费用、文件、知识、秘书权限边界和服务状态。文件与服务使用明确标记的元数据／观测夹具；费用的大额与小数用于精度验证。当前身份没有秘书读取动作，页面正确拒绝进入会话；录制不临时扩权。

The tour visits the sixteen-domain project overview, contract, actual costs, files, knowledge, secretary access boundary and service status. File metadata/service observations are controlled fixtures, and unusual monetary values exercise precision. This identity lacks secretary-read permission, so the screen correctly denies access; the recording does not add privileges.

![业务页面浏览 / Operations tour](gifs/web-operations-tour.gif)

[完整 MP4 / Full MP4](videos/web-operations-tour.mp4) · 1440 × 900 · 16.6 s · 无声 / silent · GIF 约 / approximately 1.09×

## 3. 移动风险转问题与处理 / Mobile risk-to-issue workflow

实际操作：从同一个新项目进入风险 → 新建风险及应对措施 → 明确发生 → 沿真实关联问题链接 → 调查 → 填写解决结果 → 解决并关闭。后台确认实际风险、问题及最终状态。

Actual actions: open risks for the newly created project, create a risk and mitigation, explicitly realize it, follow the actual linked issue, investigate, record the resolution, resolve and close. The backend verifies the real records and final state.

![移动风险与问题 / Mobile risk and issue](gifs/mobile-risk-issue.gif)

[完整 MP4 / Full MP4](videos/mobile-risk-issue.mp4) · 390 × 900 · 6.9 s · 无声 / silent · GIF 1× · H5

## 4. 移动任务、子任务与前置依赖 / Mobile task, subtask and prerequisite

实际操作：保存带负责人及六位微秒香港排期的任务 → 创建子任务 → 选择当前项目里程碑 → 添加前置任务 → 通过原接口读取实际关系与任务版本。录制由自动化操作，开始于表单保存区域，部分确认过程很快，可慢速查看 MP4；不包含登录或 MFA 输入。

Actual actions: save an assigned task with a six-digit Hong Kong schedule, create a child, select a real project milestone, add a prerequisite and read back the actual edge/task revision. Automated actions are fast; capture begins in the form save area, so use slower MP4 playback for the confirmations. Authentication/MFA inputs are excluded.

![移动任务、子任务与前置依赖 / Mobile task, subtask and prerequisite](gifs/mobile-task-workflow.gif)

[完整 MP4 / Full MP4](videos/mobile-task-workflow.mp4) · 390 × 900 · 6.7 s · 无声 / silent · GIF 1× · H5

增量78历史画面（提交7a31bc84c4470862659d22733b1434d318b9a8df）的完整596项来源清单、9组实际流程、6项布局及5个上下文清理见[任务历史浏览器结果](../quality/evidence/mobile-tasks-browser/result.json)。旧三组录屏及14张初始截图属于提交 `9e87869397112e0807dd28abdbc691c7b5abb15b` 的历史采集；后续检查从该提交读取711项已提交来源，并保留另1项被忽略编译缓存的准确摘要记录，不将旧图标为最新功能重录。

The increment78 capture, retained at commit 7a31bc84c4470862659d22733b1434d318b9a8df, binds 596 complete source entries, nine actual flows, six layouts and cleanup of five contexts in the [historical task browser result](../quality/evidence/mobile-tasks-browser/result.json). The three earlier recordings and 14 initial screenshots belong to historical commit `9e87869397112e0807dd28abdbc691c7b5abb15b`; verification reads 711 original committed sources and retains the recorded digest of one ignored generated cache instead of presenting old captures as newly recorded features.

## 5. 移动结项与固定复盘 / Mobile closeout and immutable retrospective

实际操作：提交已经人工填写、分别确认并完成本次 MFA 的复盘 → 原事务归档 → 读取实际项目及历史 → 展开当时完整复盘 → 明确重开项目 → 原复盘仍保留且标记依据变化。录制在验证码输入已清除后开始；三项复盘文字和十三类清单的可见区域另有截图，声明／MFA位于表单更下方。自动操作较快，可慢速查看MP4。操作为隔离合成数据的真实 API／Taro H5，不是模型执行或真机证明。

Actual actions: submit a completed retrospective after separate attestations and current MFA, commit the original archive transaction, reconcile actual project/history reads, expand the fixed review, explicitly reopen and verify the preserved review is marked changed. Recording starts after MFA input is cleared. Separate screenshots cover the visible checklist and three-note form regions; attestations/MFA are farther down the form. Automated actions are fast; use slower MP4 playback. These are actual API/Taro H5 actions with synthetic data, without model or physical-device execution.

![移动结项与复盘 / Mobile closeout and retrospective](gifs/mobile-project-closeout-workflow.gif)

[完整 MP4 / Full MP4](videos/mobile-project-closeout-workflow.mp4) · 390 × 900 · 3.6 s · 无声 / silent · GIF 1× · H5

增量79历史采集属于提交 `8f77924a06cfd3141c9d1b9cd1c574bfa2191d89`，完整源码前后摘要、真实并发／断网／历史分页／限制角色与所属浏览器清理见[页面结果](../quality/evidence/mobile-closeout-browser/result.json)及[源码清单](../quality/evidence/mobile-closeout-browser/source-manifest.json)。编码逐帧核对真实 JPEG 清单，[公开原始帧清单](../quality/evidence/mobile-closeout/recording-frames.json)仅保留时间与摘要，原JPEG留在忽略目录；见[媒体摘要](../quality/evidence/mobile-closeout/media.json)及[实际视觉检查](../quality/evidence/mobile-closeout/visual-review.json)。

The increment79 capture belongs to historical commit `8f77924a06cfd3141c9d1b9cd1c574bfa2191d89`. Source stability, real concurrency, lost responses, pagination, restricted roles and owned-resource cleanup are recorded in the [browser result](../quality/evidence/mobile-closeout-browser/result.json) and [source manifest](../quality/evidence/mobile-closeout-browser/source-manifest.json). [Media evidence](../quality/evidence/mobile-closeout/media.json) binds every actual JPEG frame to the outputs; [visual review](../quality/evidence/mobile-closeout/visual-review.json) records inspected samples.

## 6. 移动完整变更提案 / Mobile complete change proposal

实际录制：选择明确币种 → 读取并采用完整项目／预算基线 → 填写范围、日期和100.01预算替换行 → 选定正式文件版本 → 保存并读回固定草稿。录像仅覆盖初次提案，不把随后审批或实施剪入同一过程。另两张截图来自后续120.01修订获批及独立实施清空预算后的真实页面；完整十组流程另含保持预算、原申请修订、独立审批、本人MFA实施、409、403、限制角色及历史分页。

Actual capture: choose the currency, read and accept the full project/budget basis, enter scope/dates and a 100.01 replacement line, select a formal file version, then save and reconcile the immutable draft. The recording covers initial drafting. Separate screenshots show the later 120.01 approved revision and the zero-line budget after a separately approved/implemented clearing proposal. Ten verified flows also cover keep mode, original request revision, independent approval, MFA implementation, conflicts, access loss, restricted roles and history pagination.

![移动完整变更提案 / Mobile full change proposal](gifs/mobile-project-change-workflow.gif)

[完整 MP4 / Full MP4](videos/mobile-project-change-workflow.mp4) · 390 × 900 · 7.3 s · 无声 / silent · GIF 1× · H5

35个真实JPEG源帧、7178毫秒时间记录及其摘要绑定编码结果，GIF含44帧。采集器允许公开合成业务ID／摘要，拒绝登录密码、验证码和令牌；未通过遮盖重绘业务界面。3张PNG及GIF首／中／末帧已实际查看，表单截图仅覆盖当时可见区域。完整依据见[浏览器结果](../quality/evidence/mobile-changes-browser/result.json)、[源码清单](../quality/evidence/mobile-changes-browser/source-manifest.json)、[逐帧清单](../quality/evidence/mobile-changes/recording-frames.json)、[采集证明](../quality/evidence/mobile-changes/recording-provenance.json)、[编码记录](../quality/evidence/mobile-changes/media.json)及[视觉检查](../quality/evidence/mobile-changes/visual-review.json)。

The output binds 35 actual JPEGs and their original 7178 ms timestamps/hashes; the GIF has 44 frames. Public synthetic business IDs/hashes remain visible. Capture rejects credential inputs and does not redraw the UI. All three PNGs and first/middle/last GIF samples were inspected; form images cover their actual visible regions. [Verification](../quality/evidence/mobile-changes/verification.md) distinguishes these H5/API checks from device and external acceptance.

## 7. 原生监控与状态切换 / Native monitor and status transitions

独立Windows桥接程序实际读取受控Prometheus协议端，经过持久journal／签名投递，让同一Web服务连续三次失败后显示异常、连续两次成功后恢复，并在页面验证原始签名。录制只包含Web服务页、观测历史和签名核对；原生CLI在后台执行，无登录／MFA输入。真实Prometheus／Blackbox尚未安装，协议端不代表实际探测目标验收。

The actual native Windows bridge reads a controlled Prometheus protocol endpoint, journals/signs the observations and drives the original API. The Web page shows three-failure outage, two-success recovery and original-signature verification. The CLI runs in the background; no authentication entry is recorded. Actual Prometheus/Blackbox/targets remain deployment gates.

![原生监控状态流程 / Native monitor status workflow](gifs/native-service-health-workflow.gif)

[完整MP4 / Full MP4](videos/native-service-health-workflow.mp4) · 1440 × 900 · 6.0 s · 无声 / silent · GIF720px、6fps、36帧、1×

28个实际JPEG及5781ms原始记录绑定[编码结果](../quality/evidence/status-bridge/media.json)和[公开帧清单](../quality/evidence/status-bridge/recording-frames.json)。[页面结果](../quality/evidence/status-bridge-browser/result.json)绑定787份完整来源、六组实际流程、12次CLI、九项布局与两上下文清理。三张完整视口图及GIF0/12/24/35帧已[实际查看](../quality/evidence/status-bridge/visual-review.json)；MP4适合核对较小签名文字，移动图仅显示当时可见的服务状态／资料，来源及历史位于下方。

The [verification record](../quality/evidence/status-bridge/verification.md) separates fresh execution, native cached tests, exact runtime reuse and failed checker attempts. Original JPEGs remain ignored. Earlier change media is historical at commit `ee04df957ea4b590db8bad1510e308f393e876aa`; its bytes and source proof remain unchanged.

## 8. 页面截图 / Screenshots

| 页面 / Page | 图片 / Image | 范围 / Scope |
| --- | --- | --- |
| 原生桥接正常 / Bridge operational | [native-status-bridge-operational.png](screenshots/native-status-bridge-operational.png) | 实际CLI及API形成的正常状态 / Healthy projection produced by the actual CLI/API |
| 原生桥接故障 / Bridge outage | [native-status-bridge-down.png](screenshots/native-status-bridge-down.png) | 三次实际失败、当前来源状态 / Three actual failures and current source |
| 移动状态 / Mobile status | [native-status-bridge-mobile.png](screenshots/native-status-bridge-mobile.png) | 只读Taro H5员工查看同一故障；可见视口 / Reader sees the same outage in the captured H5 viewport |
| 工作台 / Dashboard | [web-dashboard.png](screenshots/web-dashboard.png) | 当前身份、可见项目 / Current identity and visible projects |
| 项目资料 / Project | [web-project.png](screenshots/web-project.png) | 实际新项目、尚无任务时 / Newly created project before task creation |
| 任务 / Task | [web-task.png](screenshots/web-task.png) | 实际完成任务及操作原因 / Actual completed task and reason |
| 看板 / Board | [web-project-board.png](screenshots/web-project-board.png) | 六状态横向看板，按实际窗口裁取 / Horizontal board in the actual viewport |
| 项目概览 / Overview | [web-overview.png](screenshots/web-overview.png) | 合成的完整业务聚合 / Synthetic business aggregation |
| 合同 / Contract | [web-contract.png](screenshots/web-contract.png) | 原合同与固定依据 / Original contract and fixed evidence |
| 实际费用 / Actual costs | [web-finance.png](screenshots/web-finance.png) | 原币精度与原记录 / Original-currency precision and records |
| 文件 / Files | [web-files.png](screenshots/web-files.png) | 元数据、目录；非云传输证明 / Metadata and folders, not transfer proof |
| 知识 / Knowledge | [web-knowledge.png](screenshots/web-knowledge.png) | 已发布与草稿入口 / Published/draft entry points |
| 秘书权限 / Secretary access | [web-secretary.png](screenshots/web-secretary.png) | 当前账号正确拒绝 / Correct current-identity denial |
| 服务 / Services | [web-status.png](screenshots/web-status.png) | 服务与无观测提示 / Services and missing-observation state |
| 移动项目 / Mobile project | [mobile-project.png](screenshots/mobile-project.png) | Taro共享界面 / Shared Taro UI |
| 移动风险 / Mobile risk | [mobile-risk.png](screenshots/mobile-risk.png) | 实际创建风险 / Actual created risk |
| 移动问题 / Mobile issue | [mobile-issue.png](screenshots/mobile-issue.png) | 真实问题已关闭 / Actual closed issue |
| 秘书准确计划 / Secretary exact plan | [web-secretary-plan.png](screenshots/web-secretary-plan.png) | 历史受控流程的顶部裁剪，非本次模型运行 / Top crop of an earlier controlled flow, not a new model run |
| 移动任务表单 / Mobile task form | [mobile-task-form.png](screenshots/mobile-task-form.png) | 分配、香港排期、关联与保存区域 / Assignment, Hong Kong schedule, association and save region |
| 移动任务关系 / Mobile task relations | [mobile-task-dependencies.png](screenshots/mobile-task-dependencies.png) | 子任务、当前可读前置和里程碑链接的可见区域 / Visible subtask, readable prerequisite and milestone links |
| 移动结项清单 / Mobile closeout checklist | [mobile-project-closeout.png](screenshots/mobile-project-closeout.png) | 实际未完成任务及十三类完整核查 / Actual unfinished task and complete thirteen-category check |
| 移动复盘表单 / Mobile retrospective form | [mobile-project-archive-form.png](screenshots/mobile-project-archive-form.png) | 成果、经验与跟进，验证码输入已清除 / Outcomes, lessons and follow-up after MFA input is cleared |
| 移动固定历史 / Mobile immutable history | [mobile-project-archive-history.png](screenshots/mobile-project-archive-history.png) | 实际归档后复盘与原清单 / Retrospective and fixed checklist after an actual commit |
| 移动变更表单 / Mobile change editor | [mobile-change-editor.png](screenshots/mobile-change-editor.png) | 修订后完整范围与日期，可见区域 / Revised full scope and dates in the actual viewport |
| 移动已批提案 / Mobile approved proposal | [mobile-change-approved.png](screenshots/mobile-change-approved.png) | 固定修订、原申请入口，批准仍需明确实施 / Fixed revision and original request; implementation remains separate |
| 移动预算修订 / Mobile budget revisions | [mobile-change-budget-history.png](screenshots/mobile-change-budget-history.png) | 明确币种／状态筛选和已实施清空后的当前预算 / Currency/state filter and current zero-line budget after implementation |

![项目业务概览 / Project overview](screenshots/web-overview.png)

![秘书准确计划，历史来源 / Exact secretary plan, historical source](screenshots/web-secretary-plan.png)

历史计划图的原图为[既有实际秘书流程证据](../quality/evidence/secretary-discovery-web/plan-reference.png)，只裁取顶部 1440 × 900，未生成或改写界面。来源摘要与裁剪坐标见[来源记录](../quality/evidence/repository-showcase/historical-media.json)。

The plan image is a 1440 × 900 top crop of the [earlier actual secretary-flow evidence](../quality/evidence/secretary-discovery-web/plan-reference.png). No UI was synthesized or rewritten. [Provenance](../quality/evidence/repository-showcase/historical-media.json) records source hashes and crop coordinates.

## 9. 来源、编码与复现 / Provenance, encoding and reproduction

| 记录 / Record | 内容 / Contents |
| --- | --- |
| [result.json](../quality/evidence/repository-showcase/result.json) | 完整源码前后摘要、两个阶段、原生夹具与临时服务清理 / Source stability, phases and native fixture/server cleanup |
| [web.json](../quality/evidence/repository-showcase/web.json)、[mobile.json](../quality/evidence/repository-showcase/mobile.json) | 实际 HTTP 方法／路径／状态、截图摘要、浏览器关闭 / Actual HTTP method/path/status, image digests and browser closure |
| [media.json](../quality/evidence/repository-showcase/media.json) | 逐帧清单摘要、输出摘要／体积、编码器版本、ffprobe结果 / Frame-manifest digests, output hashes/sizes, encoder version and probes |
| [source-manifest.json](../quality/evidence/repository-showcase/source-manifest.json) | 完整来源文件与摘要 / Complete capture source manifest |
| [visual-review.json](../quality/evidence/repository-showcase/visual-review.json) | 初始画面的实际查看、抽样范围与限制 / Initial image inspection, sampling scope and limits |
| [任务来源清单](../quality/evidence/mobile-tasks-browser/source-manifest.json)／[页面结果](../quality/evidence/mobile-tasks-browser/result.json) | 新任务采集的完整源码及真实流程／清理 / Complete new task sources, actual flows and cleanup |
| [任务媒体](../quality/evidence/mobile-tasks/media.json)／[视觉查看](../quality/evidence/mobile-tasks/visual-review.json) | 32个真实源帧、输出摘要、ffprobe及GIF首中尾抽样 / 32 actual source frames, output digests, probe and first/middle/last GIF samples |
| [当前功能来源](../quality/current-functional-baseline.json)／[最终复算](../quality/evidence/status-bridge/final-source-check.json) | 当前增量与历史媒体提交的明确区分 / Current functional increment and explicit historical-media commit |
| [历史来源补充](../quality/evidence/mobile-tasks/historical-capture-sources.json) | 原712项字节流完整重建通过：711项Git源码及1项忽略编译缓存；仅发布摘要，不提交缓存字节 / Exact original 712-entry reconstruction: 711 Git blobs plus one ignored generated cache; digests published, cache bytes excluded |

在已有原生开发工具、依赖、测试数据库和 Edge 的环境，从 `app/` 执行。以下初始三段采集的精确复现以提交 `9e87869397112e0807dd28abdbc691c7b5abb15b` 为准；当前源码下运行会产生新的采集，必须新建相应证据，不能覆盖历史来源说明：

With native tools, dependencies, test database access and Edge prepared, run from `app/`. Exact reproduction of the original three captures uses commit `9e87869397112e0807dd28abdbc691c7b5abb15b`. Running against later source creates a new capture requiring new evidence, not replacement of the historical provenance:

```powershell
$env:PYTHONUTF8='1'
python scripts/run_repository_showcase.py
python scripts/encode_repository_showcase.py
python scripts/verify_repository_publication_development.py
python scripts/verify_repository_publication.py
```

任务、结项、变更复现须分别检出增量78、79、80的历史提交；以下为80的历史变更命令，当前81状态复现见后续步骤。所有采集共用端口与原生夹具，须先后运行：

Check out increments78/79/80 for historical task/closeout/change captures. The following change commands belong to80; current81 status reproduction follows. All captures share a port/fixture and must run sequentially:

```powershell
$env:PYTHONUTF8='1'
python scripts/run_mobile_changes_browser.py
python scripts/freeze_mobile_changes_evidence.py --capture-only
python scripts/encode_mobile_changes_media.py
python scripts/verify_mobile_changes_development.py
```

当前81在原生工具、测试库、Edge及桥接二进制准备后，从`app/`顺序运行；先归档此前证据，再重新生成独立来源和实际视觉查看。完整构建／复核命令见[验证记录](../quality/evidence/status-bridge/verification.md)。

For81, prepare the native toolchain/test database/Edge/bridge binary, archive previous evidence, then run sequentially from `app/`. A rebuild needs new source bindings and actual visual review.

```powershell
$env:PYTHONUTF8='1'
python scripts/run_status_bridge_browser.py
python scripts/freeze_status_bridge_evidence.py --capture-only
python scripts/encode_status_bridge_media.py
```

采集会创建自己的隔离数据库，顺序使用本机 5189 端口，并在 finally 中关闭自己的浏览器、服务与夹具。若已有同名私有夹具，脚本会拒绝接管。原始 JPEG 帧与登录资料留在忽略的 `app/.local/`，编码器只接受摘要核对通过的真实帧，输出无声 H.264 MP4 与调色板 GIF。MP4 保留实际时间间隔，以 10 fps 编码；GIF 6 fps，初始桌面宽960、移动宽390；81桥接GIF宽720，长片段适度加速。没有插帧生成或重新绘制页面。

Capture owns an isolated database and uses local port 5189 sequentially. Its cleanup closes owned browsers, servers and fixtures; it refuses an existing private fixture. Raw JPEG frames and login material remain under ignored `app/.local/`. The encoder verifies frame hashes before producing silent H.264 MP4 and palette GIF output. MP4 preserves recorded intervals at 10 fps; GIF uses 6 fps, initial desktop width960/mobile width390; bridge81 width720, with modest acceleration for the long clip. No generated UI or synthesized in-between frames are used.

手动视觉检查是另一个步骤，脚本重建后须实际查看新图再更新记录；不能仅沿用上次“看过”的结论。完整凭据检查基于采集前置条件、数据隔离、提交扫描和实际画面检查；并不宣称对所有像素完成自动 OCR 审计。后续受影响的功能截图、录屏、GIF 与中英文说明应同步更新，见[维护约定](../contributing.md)。

Manual visual inspection is a separate step: inspect rebuilt media before updating its review record. Credential exclusion relies on capture preconditions, isolation, commit scanning and actual visual inspection; this is not a claim of exhaustive pixel OCR. Update affected media and both language guides together with future changes; see [maintenance rules](../contributing.md).
