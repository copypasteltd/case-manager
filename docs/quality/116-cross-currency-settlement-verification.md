# 增量116核验 / Increment116 verification

[业务约束与接口](../development/116-cross-currency-settlement.md) · [全部未完成项](../delivery/03-goal-gap-assessment.md)。

| 编号 | 检查 | 实际结果与边界 |
|---|---|---|
| FX-V01 | 完整后端 | 238顶层／1477条结果，12串行批次，全部发现用例执行，无跳过；嵌套结果不等于不同业务场景数。 |
| FX-V02 | 新API与PG | 15组：明确报价／旧协议、双币项目守恒、回转复核、原报价退款、全额冲销、整数币尾差、费用零但现金剩余、零现金项目份额、跨付款费用上限、并发、SQL伪造拒绝、独立退款后的重检、现金及关闭单位、真人MFA幂等、已批准员工垫付。 |
| FX-V03 | 纯金额与客户端 | 精确Go金额11组；Web294、移动390、运行时55。13个新共享边界用例在Web和移动复用，不加倍计业务场景；另有2个移动表单／旧草稿用例。 |
| FX-V04 | 四端构建 | 八项类型／测试／构建命令通过；微信／抖音各20页。Android当前开发APK18＋3静态检查；26条JVM来自Gradle缓存，保留4份XML，未运行设备或模拟器。 |
| FX-V05 | 真实页面 | Web、Taro H5各6组，实际API／PG，不替换响应：显式合成授权与事实；错误报价阻止写入；正确报价及MFA；部分双币释放；原事实保留与双币流水；无财务权限用户拒绝。基础运行夹具复用已存在文件范围测试，财务授权和数据经实际API创建，不称新增财务Go夹具。 |
| FX-V06 | 升级 | 已发布115的schema88完整真实现金用例先执行，再以当前正常migrator升至89；14张原事实表不变，旧核销／项目份额按原额补齐。此独立旧源码升级检查不计当前后端数量。 |
| FX-V07 | 媒体 | 6 PNG、2静音MP4及2截图导览GIF；认证信息不入素材，MFA输入不录制。286份旧媒体保持原字节，不包装成新验收。 |
| FX-V08 | 服务与资源 | 原生开发PG88→89，准确源码API／两项worker重载，旧AI费用资料及原现金／费用表保持；独立耐久测试PG关闭，0临时库，浏览器／临时端口关闭。外部集成保持关闭。 |
| FX-V09 | 合约与Linux | 原333路径／409操作不变，5模型显式扩展＋2新模型，731模型；两个Linux目标交叉构建，不是目标机执行。 |
| FX-V10 | 当前范围 | 23 active／5 excluded，八项Skill仍draft；公司财务制度、费用／汇差及完整旅程保留GAP-03／18。适用性分类不执行业务案例，完整目标未完成。 |

首轮新Go测试有两处测试代码编译问题，随后员工报销夹具缺少独立读取授权；修正后另行运行最终定向和完整回归。Web首次类型检查遗漏共享文件编译清单；一次客户端检查在完善回执币种绑定前主动中止。失败／中止日志保留私有文件摘要，不计本轮最终通过；浏览器若有早期失败，另列attempt记录，不能替代最终源码验证。

首次页面准备因给自己授权后原会话正确失效而停止；改用独立管理员授权并重新登录。共享移动页面随后暴露本地金额校验被通用提示隐藏的问题，已修正同步财务输入边界，并重新执行受影响客户端、Android及两端页面检查。修正前通过的客户端／APK／Web证据、原源码、原素材与日志单独保留，不计当前源码通过。独立升级首次实际迁移通过，但收集器未拼接Go长日志分段，修正后只重跑该升级检查。

进一步诊断确认Taro把BigInt幂改写为`Math.pow`，导致实际移动页面在报价校验前发生类型错误；共享计算改用有界BigInt乘积，并检查真实编译H5业务流程与新APK的已知改写模式。此发现说明源码单元测试不能代替实际构建消费者检查，原失败与诊断不计最终通过。

修正前的交互源码归档没有覆盖客户端清单中的另外26个兼容包／验证脚本。补充归档使用未变的已发布Git字节及可由原聚合摘要确认的原脚本字节，要求完整原客户端和Android源码摘要同时一致；保留最初归档原字节，不计新测试执行。

最终仓库发布检查另发现Web付款页面末尾有一个额外空行。修正前通过的客户端、两端页面、媒体及失败发布记录连同准确源码和私有日志分别归档；移除该2字节空行后，重新执行八项客户端命令及Web／Taro H5两端页面检查和媒体生成。后端及Android输入的准确摘要未改变，保留其原执行时间与缓存状态，不称重新执行。

该EOF归档另漏列了浏览器清单特有的`build_openapi.py`和`project_concern_picker.mjs`，使首次最终来源收集器在生成报告前停止。已从独立绑定的导航修正前源码归档补齐，要求原客户端及两个原浏览器完整摘要同时一致；原归档和收集器保持原字节，此补全不计新业务执行。

这次移动页面复查已通过前4组，但脚本在应用核对回执并自动返回详情期间再次强制跳转，导致流水读取超时，失败截图显示加载中的详情与两个活动页面。修正仅让脚本等待真实详情导航和流水区就绪；未修改应用、断言、超时或替换响应。原失败及同一脚本版本下已通过的Web证据分别归档，随后以修正后的准确脚本重新检查两端；不重复计作客户端、后端或Android测试。

| 证据 | 链接 |
|---|---|
| 准确源码 | [最终检查](evidence/payment-fx/final-source-check.json) |
| 后端 | [全量](evidence/payment_fx-backend/result.json)、[批次](evidence/payment_fx-backend/batch-progress.json)、[原日志摘要](evidence/payment_fx-backend/raw-log-archive.json) |
| 客户端与页面 | [八项命令](evidence/payment-fx/client-checks.json)、[Web](evidence/payment-fx-web/result.json)、[Taro H5](evidence/payment-fx-mobile/result.json) |
| Android | [构建](evidence/payment-fx/build.json)、[18项静态检查](evidence/payment-fx/apk-inspection.json)、[双币内容](evidence/payment-fx/android-feature-content.json)、[JVM范围](evidence/payment-fx/android-test-scope.json) |
| 升级／开发 | [旧版实际升级](evidence/payment-fx/schema-upgrade.json)、[开发更新](evidence/payment-fx/development-update.json)、[只读检查](evidence/payment-fx/development-project-resources.json) |
| 媒体 | [来源与编码](evidence/payment-fx/media.json)、[目视检查](evidence/payment-fx/visual-review.json)、[旧媒体](evidence/payment-fx/reused-evidence.json) |
| Linux／关闭 | [交叉构建](evidence/payment-fx/linux-build.json)、[原生PG关闭](evidence/payment-fx/native-cluster-closed.json) |
| 初期尝试 | [日志摘要](evidence/payment-fx/attempts/early-checks.json) |
| 追加尝试与修复 | [会话失效](evidence/payment-fx/attempts/01-browser-session-header/diagnosis.json)、[移动提示失败](evidence/payment-fx/attempts/02-mobile-validation-message/mobile/result.json)、[修正前通过范围](evidence/payment-fx/attempts/03-before-mobile-validation-fix/preservation.json)、[升级收集](evidence/payment-fx/attempts/upgrade-output-chunks.json) |
| 移动构建诊断 | [实际通用提示](evidence/payment-fx/attempts/04-mobile-notice-diagnostics/mobile/result.json)、[BigInt改写根因](evidence/payment-fx/attempts/05-taro-bigint-lowering/diagnosis.json) |
| 原源码归档补全 | [26个未变输入与完整原摘要](evidence/payment-fx/attempts/03-before-mobile-validation-fix/source-archive-supplement.json) |
| 发布格式修正 | [原通过结果、失败发布记录、源码与媒体](evidence/payment-fx/attempts/06-web-eof-whitespace/preservation.json) |
| EOF归档补全 | [两个原浏览器输入及原摘要](evidence/payment-fx/attempts/06-web-eof-whitespace/source-archive-supplement.json) |
| 移动导航检查 | [原失败与已通过Web的准确来源](evidence/payment-fx/attempts/07-mobile-navigation-race/navigation-preservation.json) |

These checks establish the bounded, explicitly reviewed settlement workflow. Live providers, formal releases, real Codex/TG and production acceptance remain open. Prior and cached evidence retains its original execution scope.
