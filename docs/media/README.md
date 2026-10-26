# 实际页面与操作演示 / Actual pages and interactions

[中文使用指南](../README.zh-CN.md) · [English guide](../README.en.md) · [仓库首页 / Home](../../README.md)

本目录包含 **20 张 PNG、5 段 MP4、5 个 GIF**：增量77的14张实际截图、1张历史秘书计划来源裁剪，以及增量78新增的2张任务截图和1对任务录屏／GIF；增量79新增3张结项截图和1对结项录屏／GIF。画面来自真实应用与实际 API，数据是隔离合成测试资料；不含真实合同、员工资料、登录密码、MFA 秘钥或访问令牌。无真实云传输、模型调用或 TG 群外发。移动画面是 390 × 900 的 Taro H5，共享界面不能作为 Android／小程序真机验收。

This catalog contains **20 PNGs, five MP4s and five GIFs**: increment77 supplied 14 actual captures and one attributed earlier secretary-plan crop; increment78 adds two task screenshots and one task recording/GIF pair; increment79 adds three closeout images and one closeout recording/GIF pair. These are real application/API interactions with isolated synthetic data. They exclude real company records, passwords, MFA secrets and access tokens. No real cloud transfer, model call or TG group delivery occurred. Mobile captures show the shared Taro H5 interface at 390 × 900, not physical-device acceptance.

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

本轮完整源码前后摘要、真实并发／断网／历史分页／限制角色与所属浏览器清理见[页面结果](../quality/evidence/mobile-closeout-browser/result.json)及[源码清单](../quality/evidence/mobile-closeout-browser/source-manifest.json)。编码逐帧核对真实 JPEG 清单，[公开原始帧清单](../quality/evidence/mobile-closeout/recording-frames.json)仅保留时间与摘要，原JPEG留在忽略目录；见[媒体摘要](../quality/evidence/mobile-closeout/media.json)及[实际视觉检查](../quality/evidence/mobile-closeout/visual-review.json)。

Source stability, real concurrency, lost responses, pagination, restricted roles and owned-resource cleanup are recorded in the [browser result](../quality/evidence/mobile-closeout-browser/result.json) and [source manifest](../quality/evidence/mobile-closeout-browser/source-manifest.json). [Media evidence](../quality/evidence/mobile-closeout/media.json) binds every actual JPEG frame to the outputs; [visual review](../quality/evidence/mobile-closeout/visual-review.json) records inspected samples.

## 6. 页面截图 / Screenshots

| 页面 / Page | 图片 / Image | 范围 / Scope |
| --- | --- | --- |
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

![项目业务概览 / Project overview](screenshots/web-overview.png)

![秘书准确计划，历史来源 / Exact secretary plan, historical source](screenshots/web-secretary-plan.png)

历史计划图的原图为[既有实际秘书流程证据](../quality/evidence/secretary-discovery-web/plan-reference.png)，只裁取顶部 1440 × 900，未生成或改写界面。来源摘要与裁剪坐标见[来源记录](../quality/evidence/repository-showcase/historical-media.json)。

The plan image is a 1440 × 900 top crop of the [earlier actual secretary-flow evidence](../quality/evidence/secretary-discovery-web/plan-reference.png). No UI was synthesized or rewritten. [Provenance](../quality/evidence/repository-showcase/historical-media.json) records source hashes and crop coordinates.

## 7. 来源、编码与复现 / Provenance, encoding and reproduction

| 记录 / Record | 内容 / Contents |
| --- | --- |
| [result.json](../quality/evidence/repository-showcase/result.json) | 完整源码前后摘要、两个阶段、原生夹具与临时服务清理 / Source stability, phases and native fixture/server cleanup |
| [web.json](../quality/evidence/repository-showcase/web.json)、[mobile.json](../quality/evidence/repository-showcase/mobile.json) | 实际 HTTP 方法／路径／状态、截图摘要、浏览器关闭 / Actual HTTP method/path/status, image digests and browser closure |
| [media.json](../quality/evidence/repository-showcase/media.json) | 逐帧清单摘要、输出摘要／体积、编码器版本、ffprobe结果 / Frame-manifest digests, output hashes/sizes, encoder version and probes |
| [source-manifest.json](../quality/evidence/repository-showcase/source-manifest.json) | 完整来源文件与摘要 / Complete capture source manifest |
| [visual-review.json](../quality/evidence/repository-showcase/visual-review.json) | 初始画面的实际查看、抽样范围与限制 / Initial image inspection, sampling scope and limits |
| [任务来源清单](../quality/evidence/mobile-tasks-browser/source-manifest.json)／[页面结果](../quality/evidence/mobile-tasks-browser/result.json) | 新任务采集的完整源码及真实流程／清理 / Complete new task sources, actual flows and cleanup |
| [任务媒体](../quality/evidence/mobile-tasks/media.json)／[视觉查看](../quality/evidence/mobile-tasks/visual-review.json) | 32个真实源帧、输出摘要、ffprobe及GIF首中尾抽样 / 32 actual source frames, output digests, probe and first/middle/last GIF samples |
| [当前功能来源](../quality/current-functional-baseline.json)／[最终复算](../quality/evidence/mobile-tasks/final-source-check.json) | 当前增量与历史媒体提交的明确区分 / Current functional increment and explicit historical-media commit |
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

任务流程复现须检出增量78的历史提交；本轮结项使用当前增量源码，执行下列命令。两组页面采集共用端口与原生夹具，须先后运行，不能并发：

Check out increment78 to reproduce its historical task capture; use the current sources for closeout. Both captures share a port/fixture and must run sequentially:

```powershell
$env:PYTHONUTF8='1'
python scripts/run_mobile_closeout_browser.py
python scripts/encode_mobile_closeout_media.py
python scripts/verify_mobile_closeout_development.py
```

采集会创建自己的隔离数据库，顺序使用本机 5189 端口，并在 finally 中关闭自己的浏览器、服务与夹具。若已有同名私有夹具，脚本会拒绝接管。原始 JPEG 帧与登录资料留在忽略的 `app/.local/`，编码器只接受摘要核对通过的真实帧，输出无声 H.264 MP4 与调色板 GIF。MP4 保留实际时间间隔，以 10 fps 编码；GIF 6 fps，桌面宽 960、移动宽 390，长片段适度加速。没有插帧生成或重新绘制页面。

Capture owns an isolated database and uses local port 5189 sequentially. Its cleanup closes owned browsers, servers and fixtures; it refuses an existing private fixture. Raw JPEG frames and login material remain under ignored `app/.local/`. The encoder verifies frame hashes before producing silent H.264 MP4 and palette GIF output. MP4 preserves recorded intervals at 10 fps; GIF uses 6 fps, desktop width 960/mobile width 390, with modest acceleration for the long clip. No generated UI or synthesized in-between frames are used.

手动视觉检查是另一个步骤，脚本重建后须实际查看新图再更新记录；不能仅沿用上次“看过”的结论。完整凭据检查基于采集前置条件、数据隔离、提交扫描和实际画面检查；并不宣称对所有像素完成自动 OCR 审计。后续受影响的功能截图、录屏、GIF 与中英文说明应同步更新，见[维护约定](../contributing.md)。

Manual visual inspection is a separate step: inspect rebuilt media before updating its review record. Credential exclusion relies on capture preconditions, isolation, commit scanning and actual visual inspection; this is not a claim of exhaustive pixel OCR. Update affected media and both language guides together with future changes; see [maintenance rules](../contributing.md).
