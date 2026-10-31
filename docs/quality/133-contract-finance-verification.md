# 增量133验证与首个冻结发布 / First-freeze verification

用户于2026-10-31明确要求在主功能闭环基础上适当跳过部分验证，快速构建首个冻结release。本轮按 `v0.1.0-rc.1` 发布候选交付；延期项目不记为通过，整体Goal与生产验收仍未完成。

The user explicitly authorized reduced validation for the first freeze. This is a frozen release candidate; skipped checks never pass and production readiness remains open.

| 编号 | 已执行 | 实际结果 |
|---|---|---|
| FV-01 | 五组原生HTTP／PostgreSQL业务套件 | 74条通过记录、0失败、0跳过；合同审批／签署／生效、实际资金、异币本金归属、部分解除、固定核对、并发限额、审计回滚与全额冲销 |
| FV-02 | 完整Web／共享移动单元 | 428／513通过，包含九项合同财务固定来源、币种确认和回执检查 |
| FV-03 | 类型与Web／两小程序构建 | 全部通过；两项原生JS基础检查通过；当前Android开发APK构建成功、19项包内容检查通过，独立320文件输入及真实源码映射与当前源码一致 |
| FV-04 | 正常开发结构升级 | 99→100，28类逻辑历史保留；准确归属API／通知／报表进程更新，未重启PG或开启外部集成 |
| FV-05 | 实际Web页面操作 | 真实登录／MFA后读取双币依据，追加195 HKD解除并重新查询持久回执；浏览器、临时服务与PG均关闭 |
| FV-06 | 发布必要检查 | 凭据／私有路径、历史字节、源码／产物摘要及归档完整性；不替代生产部署 |

## 明确延期 / Deferred

- 不重新运行完整后端／秘书运行时回归或扩大场景覆盖；保留原121全量证据。
- 共享移动浏览器首次在“相对定位器”处失败；页面实际已显示持久历史，没有脚本／服务错误。仅修正定位器，本轮延期重跑和移动视频／GIF；不把该失败写成通过。
- Android原生／JVM结果按真实EXECUTED／UP-TO-DATE记录；不追加真机、模拟器、正式签名或发布验收。
- 真实云／模型／TG／平台／Linux／生产集成保留原GAP，不因发布候选而关闭。

## 真实输入与失败保留 / Provenance

最初旧93合同拒绝触发器阻挡新路径；迁移100只放开合同本金路径，独立费用保护保持。随后修正零金额显示尺度断言、Web弹窗按钮、Web显式编译文件登记。失败尝试均保存在 `evidence/contract-refunds/attempts/`。

最终两个仅检查脚本修正为：开发巡检核对两个原退款保护、旧合同触发器不存在与新本金保护；移动历史改为准确文字筛选。已通过的业务、单元和构建记录保留原摘要。[冻结核验器](../../app/scripts/verify_first_freeze.py)严格重建原输入，证明业务源码、测试主体、选择、命令和期限相同；修正后开发巡检已重新执行。

The freeze checker preserves original execution hashes and reconstructs the exact two check-only corrections. It does not relabel old executions or the failed mobile browser case. Four API channel sessions use actual challenge verification; only synthetic TOTP replay cursors are reset between arithmetic fixture preparations, so they are not replay-security evidence.

Android构建期间仅检查脚本及冻结核验器输入变化，因此外层 `native-bundle.json` 保持失败（命令退出0、总输入摘要变化），不重写为通过。独立构建记录 `build.json`、`apk-inspection.json` 与 `android-final-source.json` 分别证明实际产物成功、APK内容及准确Android输入与当前源码一致。Android28条／JVM28条均为UP-TO-DATE，保留132原XML和原时间。开发APK与两小程序均为本地API配置；连接服务器须配置真实HTTPS地址并重新构建。

The outer plan record remains failed because inspection-only inputs changed during packaging. The independent Android-specific320-file source manifest, actual source map and APK inspection match the current source. Both28-case native suites are explicitly cached, with original timestamps. Client artifacts use a local API profile; configure HTTPS and rebuild for server access.

素材：Web三张真实PNG、登录／MFA后视口MP4及明确三截图各两秒GIF；移动仅保留实际概览PNG。没有密码、验证码、真实业务资料或银行付款。[媒体索引](../media/README.md) · [最终必要核验](evidence/contract-refunds/final-source-check.json) · [全部23项余项](../delivery/05-functional-closure-133.md)。
