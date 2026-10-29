# 跨范围文件整理核验 / File scope transfer verification

增量115核验当前源码、原生PG、四端构建及实际Web／Taro H5操作。[设计与使用](../development/115-file-scope-transfers.md) · [完整未完成清单](../delivery/03-goal-gap-assessment.md)。

| 编号 | 检查 | 结果与边界 |
|---|---|---|
| FT-V01 | 全量后端 | 236个顶层测试／1449条结果，12个原生串行批次；全部发现用例执行，无跳过。结果包含嵌套子用例，不代表1449个不同业务场景。 |
| FT-V02 | 新API／PG | 13组：完整内容历史与权限增减、真实人类身份／伪造渠道、严格输入／本人历史、近期MFA及准确摘要、分页、陈旧目标与撤销、引用／保留／上传阻塞、授权变更、伪造最终回执拒绝、真实权限时间到期、分类降级与跨类型同名冲突、原子执行及幂等、完整目录树与后代目标拒绝。 |
| FT-V03 | 共享边界 | Web281、移动375、运行时55；Web／移动分别执行同一24个新共享边界用例，不算48个不同场景。 |
| FT-V04 | 客户端 | 八项类型、测试与构建命令针对同一源码运行；微信／抖音各20页，正式平台审批另验收。 |
| FT-V05 | 真实页面 | 两端各6组：真实登录与MFA、实际目标选择及不移动的分析、完整计划及响应布局录制、明确声明及MFA后实际执行、真实新位置导航、旧项目成员刷新失去文件。没有API响应替换或组件状态注入。 |
| FT-V06 | Android | 当前开发APK通过18＋3静态检查；26条JVM来自Gradle缓存，保留4份XML和准确任务状态。未启动设备或模拟器，正式签名／升级发布仍GAP-16。 |
| FT-V07 | 媒体 | 6 PNG、2静音MP4、2截图导览GIF。录屏只覆盖固定计划阅读与刷新，不包括MFA输入；实际执行另有API检查与回执截图。GIF每张2秒共约6秒，不称连续操作录屏。276份旧媒体按已发布字节保留。 |
| FT-V08 | 数据库与服务 | 全耐久独立临时PG在检查后关闭，0临时测试库；开发PG87→88正常结构升级，原AI费用／用量四表内容保持，仅替换归属核对的API与两项Worker。外部集成保持关闭。 |
| FT-V09 | 合约与Linux | 333路径／409操作／729模型，原327路径／721模型语义保留；两个Linux amd64 ELF交叉构建，不等于目标Linux执行或隔离验收。 |
| FT-V10 | 范围／发布 | 23 active（14 P0／9 P1）、5 excluded、8项draft Skill保持；30 AC／72 TC适用性分类不执行业务案例。当前源码、范围、适用性、链接、媒体及候选／实际暂存区检查分别记录；普通提交推送以Git提交为准。 |

Early fixture attempts consumed an aborted upload's pending initiation before later content helpers. Processing that single real lifecycle job resolved the fixture; assuming an extra provider-abort job was incorrect because no provider multipart upload existed. Both failed logs remain private with published hashes. The earlier successful targeted run predates the additional permission-expiry case and is not final-source full regression evidence.

早期类型检查分别发现记录类型收窄、故意错误枚举的测试类型，以及移动目标不支持Object.hasOwn；已修正，最终八项命令独立记录。前两次浏览器目标按钮等待超时，未提交分析／执行，实际目标目录已显示；检查脚本误用按钮可见文字，忽略了用于区分目标的实际可访问名称。失败截图、资源关闭、私有日志摘要及校验过的原源码归档均保留。检查脚本改用实际可访问按钮名称，并以服务器回执的目标ID及版本核对实际选择。旧失败或被替代记录均不记成当前通过。

首次归档与检查脚本编辑发生竞争；未通过源码绑定的初始压缩包只在私有目录保留。该次发布引用的归档按原清单恢复两行检查脚本并逐文件核对，最终整体摘要与原失败检查的清单一致；归档记录明确标注重建过程。此后归档完成后再编辑。

The checks establish this bounded human workflow using controlled local data. They do not establish production readiness, real cloud/model/TG integration, platform submission or company rule adoption. Cached JVM results and retained evidence preserve their original scope.

| 证据 | 链接 |
|---|---|
| 当前源码绑定 | [结果](evidence/scope-transfer/final-source-check.json) |
| 全量后端 | [结果](evidence/scope_transfer-backend/result.json)、[批次](evidence/scope_transfer-backend/batch-progress.json)、[原始日志摘要](evidence/scope_transfer-backend/raw-log-archive.json) |
| 客户端命令 | [八项命令](evidence/scope-transfer/client-checks.json) |
| 实际页面 | [Web](evidence/scope-transfer-web/result.json)、[Taro H5](evidence/scope-transfer-mobile/result.json) |
| Android | [构建](evidence/scope-transfer/build.json)、[静态检查](evidence/scope-transfer/apk-inspection.json)、[内容检查](evidence/scope-transfer/android-feature-content.json)、[JVM范围](evidence/scope-transfer/android-test-scope.json) |
| 媒体 | [编码与来源](evidence/scope-transfer/media.json)、[目视核对](evidence/scope-transfer/visual-review.json) |
| PG与服务 | [原生PG](evidence/scope-transfer/native-cluster.json)、[关闭](evidence/scope-transfer/native-cluster-closed.json)、[升级](evidence/scope-transfer/development-update.json)、[只读核对](evidence/scope-transfer/development-project-resources.json) |
| Linux及复用 | [交叉构建](evidence/scope-transfer/linux-build.json)、[保留证据](evidence/scope-transfer/reused-evidence.json) |
| 早期尝试 | [定向检查](evidence/scope-transfer/attempts/targeted-checks.json)、[首次浏览器](evidence/scope-transfer/attempts/01-target-selector/attempt.json)、[第二次浏览器](evidence/scope-transfer/attempts/02-target-button-name/attempt.json) |
