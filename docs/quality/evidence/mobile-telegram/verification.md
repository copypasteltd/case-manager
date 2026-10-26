# 增量82准确验证 / Increment82 source-bound verification

[实现与使用](../../../development/82-mobile-telegram-access.md) · [差异表](../../../delivery/03-goal-gap-assessment.md) · [媒体](../../../media/README.md)

## 本轮结果 / Current results

| 层 / Layer | 结果 / Result | 准确范围 / Exact scope |
| --- | --- | --- |
| 移动 / Mobile | 新167/167，0失败/取消/跳过 | 原154项加13项TG输入、身份及完整回执边界；类型与微信/抖音编译通过，各16注册页。Fresh tests/types/builds; no device execution. |
| 实际页面 / Actual pages | 9组流程、9项布局通过 | 原生PG、隔离Go API、Taro H5和Edge；320/390/768宽度。Actual local UI/API; no device claim. |
| Android | 新42,867,115字节APK；17/17包检查 | 当前完整源码重新打包，TG页面与CPClipboard宿主入包。Fresh debug artifact; no emulator/device/release. |
| 原生单测 / Native tests | 新执行25/25，0失败/跳过 | 本次Gradle任务为 `> Task :app:testDebugUnitTest`，没有缓存标记；4份原XML和Gradle日志原字节冻结。Fresh JVM execution; no device clipboard test. |
| Go/PG完整回归 / Full backend | 准确复用154顶层/984结果/892末级、8批 | 完整Go/SQL/契约与81相同；本轮只新执行TestTelegramIngress夹具。Full batches were not rerun. |
| Web / Runtime | 准确复用81 / 39项 | Web沿用81，运行时沿用74完整相同来源；8项Skill仍draft。Unchanged complete-source proofs, not fresh executions. |
| Linux | 准确复用81平台产物 | 后端来源及原产物摘要相同，未新编译、执行或部署。Retained unchanged artifact only. |
| 日常只读 / Daily inspection | 69迁移、10探针通过 | 原API/Worker身份保持，0测试库/活动模型/启用TG；未重启日常服务。Read-only, no integration activation. |
| 最终复算 / Final verification | 23/23通过 | 当前源码、构建、媒体、历史Git及资源释放；见[准确清单](final-source-check.json)。 |

[移动构建](mobile-build.json) · [APK构建](build.json) · [包检查](apk-inspection.json) · [实际原生单测](android-test-scope.json) · [完整后端原证据](../status-bridge/result.json) · [原运行时](../deliverable-creation/runtime-build.json)

## 实际流程 / Actual workflows

1. 工作台和原TG请求页进入接入页面；普通员工没有管理配置按钮。Actual navigation; non-administrators lack channel controls.
2. 近期MFA后生成挑战，显式取消；刷新清除私聊能力。Real challenge/cancel; refresh clears the capability.
3. 本地webhook证明数字身份，原登录明确确认，再撤销并核对本人状态。Loopback proof, original-session confirmation and explicit revocation.
4. 明确业务授权后用真实项目选择器创建暂停入口，保存全部允许任务、默认任务及阶段链接开关。Explicit business visibility and actual project selection; new channel remains paused.
5. MFA编辑启用准确位置，写入版本递增并核对最新完整历史。Exact-location activation and immutable-history corroboration.
6. 另一写入者造成真实409；保留文字、停止重提，关闭后读取新版本。Real concurrency conflict without overwrite/retry.
7. 真实PATCH已提交后，受控代理删除一个回执字段；页面保留输入并拒绝重复写。Controlled incomplete response after a real committed mutation.
8. 真实API准备合成历史，页面游标读取最早完整修订；录制历史/列表操作。Actual history cursor and safe capture.
9. 实际列表继续分页；单独模拟的403清空条目并禁用新增。Actual list cursor, separately labelled synthetic permission fault.

缺字段和列表403是明确的受控代理故障，不当成真实权限撤回。真实业务角色授予则实际使管理员旧登录失效，检查显式重新认证后继续；系统管理权限不等于业务资料读取权。完整逐项说明见[浏览器结果](../mobile-telegram-browser/result.json)。

The missing receipt field and list403 are controlled response faults. A real role grant separately invalidated the target login and required reauthentication. Policy administration alone does not grant business visibility.

## 完整来源 / Complete sources

- 移动141项 / Mobile141: `5ee3742c44fa56ee5af8fa47965bc83b914695923c8b1f3cf6f6bc4e41a42eb1`。
- Android: `6467b63a2d934c6640c7313c0ea21a0b97aa6e0ae98cf39f0ccd9ad4c8714019`。
- 夹具、完整客户端及捕获793项 / Fixture/client/capture793: `4e7ac20b73a3b932694d728b0edcbada98e96e1d4a98146adfc9774d523a98bd`。
- APK: `0215866269f70b3c92e0cd51cb1b50a06586503cf7236b9f8c9ee4392629e96a`。
- 未变后端 / Unchanged backend: `9422bb5963afb514669192f63bb151c69eab4a51f97f0add717eb3ccdfdef34e`。

[完整清单](../mobile-telegram-browser/source-manifest.json)在夹具/构建前冻结，构建后和浏览器结束摘要相同。原证据保持原字节，不改写历史摘要制造通过。

The complete manifest freezes before fixture/build preparation and matches after build and capture. Historical bytes are preserved.

## 媒体与抽查 / Media and review

新增3张截图、4.9秒无声MP4及29帧GIF，来自23张实际JPEG、4730毫秒原时间跨度；GIF不加速。原始帧留在忽略目录，公开[帧摘要/时间](recording-frames.json)、[来源](recording-provenance.json)、[编码摘要](media.json)及[画面抽查](visual-review.json)。媒体总计29 PNG、8 MP4、8 GIF。

Three screenshots, a 4.9-second silent MP4 and 29-frame GIF come from 23 actual JPEG captures spanning 4730ms, without GIF acceleration. Raw frames remain ignored; metadata binds their timing/hashes. Repository media totals are 29 PNGs, 8 MP4s and8 GIFs.

每帧检查路径与可见密码/MFA/私聊能力节点；人工看过3张截图和GIF第0/14/28帧，不宣称全部像素OCR审计。录屏只包含历史滚动、返回及重新打开，不含绑定/MFA输入。

Every capture checks the permitted route and credential nodes. Manual inspection covers all three screenshots and GIF frames0/14/28, without claiming exhaustive pixel OCR. The clip contains history/list interactions only.

81旧状态画面按提交 `9c710a0f4c785a02539441ec1611bc6678f0050d` 的787份Git原blob复算，匹配原清单，见[历史证明](historical-status-capture.json)。77—80旧来源和失败记录也保留原字节。

Increment81 is retained at its published commit, reconstructed from 787 exact Git blobs. Earlier media and failures preserve their original historical scope.

## 失败记录 / Preserved failures

- [typecheck01](attempts/typecheck-01/result.json)：目标不支持Object.hasOwn，改用hasOwnProperty.call，未升级目标。Unsupported target API corrected without changing targets.
- [Android预检01](attempts/android-preflight-01/result.json)：缺少明确API地址/profile，构建阶段未启动。Missing local build configuration; no build phase started.
- [browser01](attempts/browser-01/result.json)：入口行包含副标题，修正脚本定位。Workspace row locator missed its subtitle.
- [browser02](attempts/browser-02/result.json)：夹具没有项目；通过原API创建合成项目并明确业务授权。Missing synthetic project/visibility prepared through real APIs.
- [browser03](attempts/browser-03/result.json)：角色授予实际撤销旧会话，新增明确重新登录。Actual session invalidation required explicit reauthentication.

每次失败均保留当时结果、清单、脚本及安全截图/日志；清理所属上下文、临时服务和测试库后重跑。最终通过记录独立保留。

Each failed attempt retains its exact checker, manifest and safe diagnostics. Owned resources were cleaned before retrying; the passing record does not overwrite failures.

## 重现与放行 / Reproduction and release gates

按[开发文档](../../../development/82-mobile-telegram-access.md)使用已配置的原生环境，保持源码固定，依次运行 `verify_mobile.py mobile-telegram`、`build_android.py --google-mirror --evidence-group mobile-telegram`、`inspect_android_apk.py --evidence-group mobile-telegram`、`run_mobile_telegram_browser.py`、`freeze_mobile_telegram_evidence.py`、`encode_mobile_telegram_media.py`、`verify_mobile_telegram_history.py`、`verify_mobile_telegram_development.py` 和 `verify_mobile_telegram_sources.py`，最后运行仓库发布检查。

Android先明确设置 `CP_MOBILE_BUILD_PROFILE=local` 及 `CP_MOBILE_API_ORIGIN=http://127.0.0.1:18080`。所有Python命令设置 `PYTHONUTF8=1`。冻结拒绝覆盖不一致原文，重跑先归档完成尝试；新媒体必须重新查看并更新人工检查记录。

Use the configured native environment, explicit local Android origin/profile and UTF-8 Python. Run the listed build/browser/freeze/encode/history/development/final-source checks, then the publication guard. Archive a completed attempt before overwriting evidence and manually inspect newly encoded output again.

真实四端、系统剪贴板及跨App返回、真实Bot/群/话题、境内出站、模型/通知、可靠成员核验、群正文披露、120GB/2TB和生产恢复仍开放。原114项依赖审计（11 critical/32 high）也不代表当前安全放行。完整平台Goal未完成。

Real platforms/devices, clipboard/cross-app return, Bot/group/topic/mainland egress, model/delivery, reliable membership, business disclosure, capacity and recovery remain open. The retained 114-finding audit also leaves its security gate open. The full goal is not complete.
