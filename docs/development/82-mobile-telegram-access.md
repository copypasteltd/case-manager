# 增量82：移动端 Telegram 身份与群入口 / Mobile Telegram access

[中文指南](../README.zh-CN.md) · [English guide](../README.en.md) · [验证与边界](../quality/evidence/mobile-telegram/verification.md) · [演示](../media/README.md)

## 目的与实现范围 / Purpose and implementation

员工从移动工作台进入「Telegram 接入」，完成本人数字身份的私聊证明、原登录确认、取消或撤销。具有 `policy.manage` 权限的管理员在同页配置准确的 Bot、群、话题、项目范围、允许任务、默认任务与阶段通知；新入口必须先暂停。微信、抖音与 Android 使用同一业务页面和既有 Go API。

Employees enter **Telegram access** from the mobile workspace to prove their own numeric identity in a Bot private chat, confirm it through the original portal login, or cancel/revoke it. Administrators with `policy.manage` configure the exact Bot/chat/topic, project scope, allowed/default task and stage-link notifications. A new channel starts paused. WeChat, Douyin and Android share these business components and the existing Go API.

本增量不修改后端、数据库迁移或 OpenAPI，不启用真实 Bot、模型或执行进程。八项 Skill 仍是草稿；允许任务配置不能授予项目读写权限或群正文披露权。原 TG 请求、阶段投递回执和 MFA 未知投递对账入口保留。

This increment keeps the existing backend, migrations and OpenAPI. It does not activate a real Bot, model or executor. All eight Skills remain draft. Allowing a task does not grant project access or group disclosure. Existing request, delivery-receipt and MFA reconciliation flows remain accessible.

## 本人身份使用流程 / Own-identity workflow

1. 登录工作空间，进入「Telegram 接入」，核对接收配置与通知进程状态。接收配置、通知可用性和实际送达是不同事实。
2. 选择明确 Bot，填写原因，确认仅在该 Bot 私聊证明，并完成近期 MFA，再生成十分钟有效的链接或 `/start` 命令。
3. 在所选 Bot 的私聊完成证明。返回原门户登录后重新读取，核对数字身份；用户名与显示名不能代替数字 ID。
4. 明确确认身份并完成近期 MFA。页面同时验证写入回执和 `/me/telegram` 中的真实绑定，才关闭操作表单。
5. 未完成的流程可以取消；既有绑定可以撤销。需确认准确对象、版本、原因及影响。撤销后停止新请求，服务器撤回尚未执行的请求，历史保留。

1. Sign in, open Telegram access and inspect ingress and delivery availability separately.
2. Choose a specific Bot, enter a reason, acknowledge private-chat proof and pass recent MFA to obtain a ten-minute link or `/start` command.
3. Prove ownership in that Bot private chat. Return to the **original portal login**, refresh and inspect the exact numeric identity.
4. Explicitly confirm with recent MFA. The client validates both the mutation receipt and the authoritative own-status binding.
5. Cancel an unfinished challenge or revoke a binding by confirming the exact object, revision, reason and impact. The server blocks new requests and withdraws unexecuted ones while retaining history.

绑定能力只保存在当前页面内存，刷新、离开或退出即清除；不写草稿、持久存储、审计正文或演示。明确点击复制后，内容进入系统剪贴板，寿命由操作系统管理，页面不会宣称自动清除系统剪贴板。链接过期后失效；重新发起也会使同 Bot 的旧流程失效。

A challenge capability lives only in current-page memory and clears on refresh, hide or sign-out. It is excluded from drafts, persistent client storage and recordings. Explicit copying puts it on the **OS clipboard**, whose lifetime is independent of page memory. Expiry or a new challenge invalidates the old capability; the app does not promise to erase the user's clipboard.

Android 使用独立的 `CPClipboard` 宿主接口，只接受有界的私聊证明链接或命令，不提供读剪贴板接口。前台 Activity 才可复制，并设置敏感内容标记；实际系统提示与剪贴板行为仍需真机验收。Web／两小程序沿用 Taro 复制 API。

Android uses a bounded `CPClipboard` host interface that only copies a proof link or command from a foreground Activity, marks it sensitive and exposes no clipboard-read API. Actual OS behavior remains a device-acceptance item. H5 and mini-programs use Taro's clipboard API.

## 管理员配置逻辑 / Administrator configuration

| 输入 / Input | 约束 / Constraint |
| --- | --- |
| Bot、群、话题 / Bot, chat, topic | 明确 Bot；群 ID 是保留负号的十进制字符串；话题为0或有界正整数。保存后位置不可换，换位置须新建。Exact Bot, decimal chat string and bounded nonnegative topic; location is immutable. |
| 项目范围 / Project scope | 通过当前可见项目选择器选择，或明确使用公司范围；展示准确项目 ID。Select a currently visible project or explicitly choose company scope. |
| 允许任务 / Allowed tasks | 项目简报、服务事件摘要至少一项，不能重复；默认任务须属于允许集合。At least one supported unique task; the default must be allowed. |
| 启用状态 / State | 新建始终暂停；后续启用/暂停必须核对当前版本、完整范围并通过近期 MFA。New channels stay paused; edits require exact revision, full-scope confirmation and recent MFA. |
| 阶段通知 / Stage notification | 独立开关；只允许阶段与登录链接；固定 `portal_only`。Independent opt-in for stage and authenticated portal links only. |
| 原因 / Reason | 明确非空，最多200个 Unicode 字符；不能自动填入通用默认理由。Explicit nonempty reason, at most200 Unicode code points. |

页面把全部配置作为一个明确意图提交，写后逐字段核对当前回执，再读取最新不可变历史，检查准确频道、版本、来源、原因及完整快照。默认任务、允许列表或披露模式被替换、缺失时，页面保留输入并停止提交。

The client submits a complete intent, compares every returned field and reads the latest immutable revision to corroborate the exact channel, version, source, reason and full snapshot. Missing or substituted fields block further submission while preserving input.

列表和历史均用后端不透明游标分页；历史显示当时完整配置，不能用当前配置解释旧记录。项目选择只显示当前可见资料。

Channel lists and immutable history use opaque backend cursors. Each historical entry shows its own complete configuration. Project selection is limited to currently visible records.

## 并发、权限和失败 / Concurrency, permission and failure

- 使用既有 `useCommand` 的作用域、意图键和幂等请求；403、409、版本变化或回执不明确时停止重提，不自动覆盖、重复写或推定成功。Explicit scope and idempotency prevent automatic overwrite/retry after forbidden, conflicting or uncertain results.
- 409保留本次文字；关闭表单、读取最新配置后，用户重新核对完整范围与 MFA。A conflict preserves text and requires an explicit reread and renewed review.
- 页面重新读取先核对 `/me`；管理员配置只在当前 `policy.manage` 存在时显示。新权限版本会重新创建配置表单。Each reload checks current identity/authorization; management controls require current `policy.manage`.
- 刷新、离开、退出会取消页面作用域；列表失败会清空既有条目，迟到结果不能追加到新范围。Page scopes cancel on refresh/hide/sign-out; failed collections clear cached entries.
- 用户修改任一配置会清除完整范围确认；项目选择关闭表单子树时也会清除该次 MFA 状态。Any configuration change resets scope acknowledgement; leaving the editor subtree resets its MFA state.

## 代码与接口 / Code and interfaces

| 路径 / Path | 职责 / Responsibility |
| --- | --- |
| `app/mobile/src/pages/telegram/access.tsx` | 本人挑战、确认、撤销与写后状态核对 / Own challenge, confirmation, revocation and status corroboration |
| `app/mobile/src/pages/telegram/channels.tsx` | 配置表单、可见项目选择、分页列表与完整历史 / Configuration, visible project picker, paginated list and history |
| `app/mobile/src/platform/telegram-input.ts` | 精确输入、完整回执和身份校验 / Exact inputs, complete receipts and identity validation |
| `app/mobile/src/platform/telegram-clipboard.rn.ts` | 有界原生复制桥 / Bounded native proof copying |
| `app/mobile/android/app/src/main/java/hk/copypaste/workspace/CPClipboardModule.java` | 前台敏感剪贴板写入，不提供读取 / Foreground sensitive clipboard write without reads |

复用 `GET /me/telegram`、挑战创建/确认/取消、绑定撤销、频道列表/创建/修订与历史接口。访问路径仍为既有第16页 `pages/telegram/index?section=access`，两小程序不新增注册页面。

The existing own-status, challenge create/confirm/cancel, binding revoke and channel list/create/patch/history endpoints are reused. Access stays under the existing sixteenth registered page, `pages/telegram/index?section=access`.

## 验证与后续放行 / Verification and release gates

精确执行结果、来源摘要、失败尝试、截图与视频的对应关系以[本轮验证](../quality/evidence/mobile-telegram/verification.md)为准。运行命令见该报告；所有本地验证使用 Windows 原生工具，不启动 Docker、WSL 或模拟器。

See the [source-bound verification record](../quality/evidence/mobile-telegram/verification.md) for actual results, source hashes, failed attempts and media provenance. All checks use native Windows tools without virtualization.

真实微信／抖音／Android、系统剪贴板、原登录跨 App 返回、平台域名/上架、真实 Bot 配置、群与话题成员核验、境内出站网络、真实通知/模型及独立群披露仍按 GAP-15／18 验收；本地 webhook 和 H5 页面不能关闭这些放行项。

Real WeChat/Douyin/Android execution, OS clipboard and cross-app return, platform submission/domains, real Bot/group/topic membership, mainland egress, actual delivery/model execution and separate group-disclosure acceptance remain open under GAP-15/18. A local webhook and H5 review do not satisfy those gates.
