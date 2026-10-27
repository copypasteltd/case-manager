# 增量86验证记录 / Increment86 verification

本轮为实际功能增量：秘书Token四维额度、原子预留、可信用量上报、终态保守结算、范围账本与人工凭据核对。全部本地检查使用原生工具；不调度Docker、WSL、虚拟机或模拟器。完整四端生产目标仍未完成。

This is a functional increment for scoped token allowances, atomic reservations, trusted metering, conservative terminal accounting and evidenced reconciliation. Verification uses native tools. The full four-client production goal remains open.

## 1. 实际检查 / Actual checks

| 层 / Layer | 实际结果 / Actual result | 来源摘要 / Source SHA-256 |
| --- | --- | --- |
| Go / PostgreSQL | 158 top-level / 1029 results / 934 leaves; 8 batches; 72 migrations; 14 new native scenarios | `f43f8c254a38e71af34b86f212044836a67e6f11fd853af1a1b0a6b5f1735acd` |
| Web | 98 tests and build | `447a812444b8a17396fb4699c70e776cdd3fd435ae63e3d6be0427f45d818ca0` |
| Codex adapter | 47 tests, Skill validation and build; no provider | `3b966f4741d4da3fd35234f6e5843373055398bb76374e5f5d6f8b6f66a38c96` |
| Taro | 187 tests; types; WeChat/Douyin compilation with 17 pages each | `dcf8d3d2e860ffb835cd2fe640b755414573b62989d63c84d95e1ecf88d41452` |
| Android | 42,892,320 byte development APK; 17 package checks; 25项Gradle缓存结果 / 25 cached Gradle results | `bd2821290b674cade7c9c68fa78e66c12da76bce2cd596ad6d26d1352d465918` |

APK SHA-256：`d05f423ff0d867350ac544c96e494d57e16e3cbf7c236824bb7a1a45dd1d566c`。开发包仅在忽略目录内保留，未提交到Git；不是生产签名／分发包。四份JVM XML、原生任务日志与执行／缓存判定已冻结，不能把缓存结果描述为新执行。Linux记录只证明当前Go源码交叉编译，不证明服务器部署。

The APK stays outside Git and is a development artifact. Frozen JVM XML and the actual Gradle task marker establish the stated cache/execution scope. Linux evidence establishes compilation, not deployment.

原生场景验证：无策略拒绝不留下运行／消息／队列；管理员权限与当前策略版本；命令和消息重放仅预留一次；组织额度并发；员工／项目／Skill独立上限；零额度与既有预留；已知失败／缓存计数；越限用量；取消后仅记账而不恢复业务权限；未知用量凭证核对；不可变记录与范围游标；预留写入失败整笔回滚。用量提交通过真实本机Go控制面HTTP检查，不调用模型供应商。

The native scenarios exercise admission rollback, live authority/revisions, replay, concurrency and all dimensions; cancellation/failure/overshoot accounting; unknown usage reconciliation; immutable history, pagination and forced rollback. The metering event traverses the authenticated native Go HTTP controller.

## 2. 页面、帧与资源释放 / Pages, frames and cleanup

| Client | Actual flows | Captured pixels and timing | Client capture source SHA-256 |
| --- | --- | --- | --- |
| web | 6 flows / 3 layouts | 15 actual frames / 3057 ms | `e57dcb6310faa564639e02c93c631a45eab5b7ed99aeed2096837cf13bab5841` |
| mobile | 6 flows / 3 layouts | 38 actual frames / 7735 ms | `98da852bbb8afc1fa60a8e3a6e9e124b3b3189c754eaf703c3b709db714dd09d` |

每端完成真实MFA登录、组织账本、策略发布、凭据核对、范围查询与本人账本。敏感操作完成后才录制范围查询；不在视频中展示密码、MFA输入或能力令牌。失败截图也遮盖身份输入。四张新PNG、两对MP4／GIF来自实际浏览器像素，帧摘要与编码摘要可核对，不用静态拼图冒充操作录制。

Each client uses real login, native API and synthetic business records. Recording starts after sensitive input is closed, and shows actual filtered account queries. Four new screenshots and two MP4/GIF pairs are captured pixels with frozen timing and hashes. Mobile is Taro H5, not a device.

两个检查串行执行，各自关闭自建浏览器及全部上下文、测试API、隔离数据库和5189页面服务。原始JPEG帧留在忽略目录；公开记录冻结帧摘要、持续时间、合成数据／无音频属性和实际产物摘要。完整媒体目录为44 PNG、15 MP4、15 GIF。

Both checks close owned contexts, browsers, disposable APIs/databases and review servers. Raw JPEGs remain ignored; public manifests retain frame hashes, timing and artifact hashes. The complete catalog contains44 PNGs,15 MP4s and15 GIFs.

## 3. 原生开发状态 / Owned native development state

只更新已验证归属的本机API到第72项迁移；保留原PostgreSQL与通知Worker。只读探针确认四张账表、四个绑定与四个不可变触发器、应用角色禁止修改历史。日常库没有预设额度、运行预留、活动模型配置、启用TG或遗留测试数据库。八项Skill仍draft，project-update仍为1.6.0／16工具／14写工具。

Only the verified owned local API was updated. PostgreSQL and the notification worker were preserved. Read-only probes confirm migration72, append-only budget records, no production allowance/reservation, no active model/TG configuration and no leftover test database. No real cloud, provider, Telegram send or production deployment is claimed.

## 4. 历史与失败记录 / History and failed attempts

增量85的完整启动来源（853 files）、Web和移动的完整捕获清单均从已发布Git提交`83bf46819fca3e670f5d3101b0234fbe46a2ad23`重建。旧证据逐字节保留；不把旧媒体重新绑定到本轮源码。更早84的未冻结忽略编译缓存限制仍保留在其原历史记录中。

Increment85 complete startup and both client capture manifests reconstruct from its published Git objects. Old evidence bytes are preserved. Earlier increment84 cache limitations stay explicit.

`attempts/`保留：早期Go类型和测试请求修正、Vitest注册错误、首次Web读取旧状态、移动额外MFA、误用“确认本次操作”而实际按钮为“确认本人操作”（早期响应等待解释已被更正）、后续移动布局定位错误、失败截图的身份遮罩、APK／最终来源检查仍要求16页和原生探针误查旧任务函数文字等记录。失败或被新脚本替代的通过检查均不冒充当前通过证据。早期未冻结的失败源码只作诊断摘要，不宣称能够重建。

The attempts directory preserves compilation/input/runner issues, early capture synchronization, a mobile MFA selector using the wrong button label (superseding the earlier response-wait hypothesis), a later mobile layout locator error, identity-input redaction, the stale16-page package/source assertions and the incorrect retained task-function probe. Failed and superseded captures are excluded from current passing evidence. Unfrozen early failures are summaries, not reconstructed passing proofs.

## 5. 复现与开放项 / Reproduction and open gates

已有证据须先归档；不要覆盖旧报告、脚本、帧清单或素材后沿用旧摘要。按根目录指南准备原生依赖及忽略的本机测试配置，保持应用源码冻结后执行相应检查：

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_backend_batches.py --batch-size 20 --evidence-group token-budgets
python app/scripts/verify_web.py token-budgets
python app/scripts/verify_agent_runtime.py token-budgets
python app/scripts/verify_mobile.py token-budgets
python app/scripts/verify_linux.py token-budgets
python app/scripts/run_token_budget_browser.py
python app/scripts/run_token_budget_browser.py --mobile
python app/scripts/encode_token_budget_media.py
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python app/scripts/build_android.py --google-mirror --evidence-group token-budgets
python app/scripts/inspect_android_apk.py --evidence-group token-budgets
python app/scripts/freeze_token_budget_evidence.py
python app/scripts/verify_token_budget_sources.py
```

These commands require the native setup described in the repository guides and archived prior output before recapture. Final source/publication reports bind current source and artifact bytes.

GAP-14仍需价格版本、币种、金额预留与账单差额、供应商费用硬限制、分段成本控制、个别额度覆盖及核对更正补偿。SDK在turn结束后报告用量，不能保证逐Token提前中止计费。已知费用失败不会消失，未知用量也不自动视作免费。

所有28项原始差异与四端目标保留：真实云与模型／TG、真机与平台审核、120GB上传和2TB迁移、生产备份恢复及依赖安全继续验收。锁文件未变；保留审计114项（11 critical／32 high），不能据此宣布安全放行。

Versioned monetary pricing, invoices, provider spending limits, segmented control, individual overrides and compensating corrections remain open. Real cloud/model/TG operation, device/platform acceptance,120GB uploads,2TB migration, recovery and dependency security remain part of the full goal. The unchanged lockfile retains114 findings, including11 critical and32 high.

[操作与设计 / Workflow](../../../development/86-secretary-token-budgets.md) · [媒体 / Media](../../../media/README.md) · [全部差异 / Full gap register](../../../delivery/03-goal-gap-assessment.md)
