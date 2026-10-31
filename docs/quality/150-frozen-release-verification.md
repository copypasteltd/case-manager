# 增量150：冻结产物核对与准确延期

当前业务149／结构111；冻结`v0.1.0-rc.6`。150只做包装与交付，没有新业务执行、浏览器、模型或目标Linux运行。

| 编号 | 本轮核对 | 原始出处与边界 |
| --- | --- | --- |
| V150-01 | Linux/Web/runtime实际构建与归档inspect | [准确输入](evidence/release-candidate-150/server-build-scope.json)；逐冻结Git blob，dirty如实记录；交叉构建 |
| V150-02 | 两小程序重建及实际JS入口 | [构建](evidence/release-candidate-150/miniapps-build.json)／[产物内容](evidence/release-candidate-150/miniapps-inspection.json)；本地API，无平台上线 |
| V150-03 | Android开发包重建、签名／JS／DEX／映射 | [APK](evidence/release-candidate-150/apk-inspection.json)／[任务](evidence/release-candidate-150/android-build-scope.json)；10检查，185模块，20页 |
| V150-04 | 原生Gradle日志及28项XML | [准确原日志](evidence/release-candidate-150/native-build-log.json)；`> Task :app:testDebugUnitTest UP-TO-DATE`；缓存=true，无单独JVM重跑 |
| V150-05 | 五包、源码及媒体逐blob、展开字节检查 | [封存](evidence/release-candidate-150/seal.json)；已知私密值／高置信度模式，无像素OCR，不等于正式安全签核 |
| V150-06 | 新标签及固定产物、旧五冻不变 | [Git引用](../releases/v0.1.0-rc.6/git-publication.json)／[远端](evidence/release-candidate-150/remote-publication.json)；18精确引用，无强推 |
| V150-07 | 既有业务检查与媒体准确复用 | [复用记录](evidence/release-candidate-150/reused-evidence.json)／[149原证明](evidence/replica-notifications/final-source-check.json)；本轮未新执行 |
| V150-08 | 范围、AC／TC及发布检查 | [范围](evidence/release-candidate-150/scope-check.json)／[适用性](evidence/release-candidate-150/acceptance-applicability-check.json)／[仓库](evidence/release-candidate-150/repository-check.json)；分类不计业务通过 |

149原生15条通过记录不是15个独立业务场景：12子场景、1既有回归、1未启用钩子及1组汇总；Web5／移动5、Web3、三截图／2.8秒实际无声操作MP4／6秒截图导览GIF原字节保留。首次MFA测试准备失败及修正仍在149，不改作生产授权修正或抹除失败。完整后端121、完整客户端136、运行器145均为原出处复用。

额外业务旅程、完整回归延期；开发PG100保留，未部署111。真实Linux／CI／生产PG／OSS＋R2／Codex／TG／平台身份与正式Android签名待对应验收。八项Skill仍draft，23工作包active，production_ready=false。已移出真机、网络性能／实网120GB、历史迁移／容灾、AI费用控制不回加门槛。

GitHub账号403原证据保持，不重试同一凭据，不宣称创建Releases网页；获授权SSH标签与固定包可用。全部旧证明／媒体、结构1–111及旧五冻原字节保留；没有虚拟化、模拟器或新浏览器。

首次仓库候选检查在生成报告自身之前检查链接，因此报告的文档链接暂缺而失败；其余检查通过。保留[首轮失败原记录](evidence/release-candidate-150/attempts/01-publication-self-report/repository-check.json)与对应文档原字节；报告产生后重新检查，不改动业务、安装包或既有证据。该失败不计为通过。
