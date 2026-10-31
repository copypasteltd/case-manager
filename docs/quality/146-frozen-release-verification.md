# 增量146 · 冻结发布核验 / Frozen release verification

准确业务源码145为 `294bf0509a7c87f63d9c829d5317fe33fb76a6f3`，结构110。146只封装交付，无业务源码／UI变更；原三个冻结与全部历史证明、媒体和结构文件保持原字节。

## 本轮重新执行

| 编号 | 核对 | 证据 |
| --- | --- | --- |
| V146-01 | 准确145应用输入构建Linux amd64三个Go程序、Web、秘书JS运行器及八项draft包；110项结构清单；归档解包检查 | [服务器回执](../releases/v0.1.0-rc.4/server-build.json) |
| V146-02 | 微信／抖音实际本地配置构建，源码执行期间不变 | [两小程序](evidence/release-candidate-146/miniapps-build.json) |
| V146-03 | 当前145共享源码重新进入开发APK；实际包、开发签名、JS与原生桥接、20个注册页面及最新OA明细模块检查 | [Android构建](evidence/release-candidate-146/build.json) · [实际包检查](evidence/release-candidate-146/apk-inspection.json) |
| V146-04 | 三组精选原生HTTP／耐久PG：OA起草及人工采用、批准业务记录实际费用、费用记录现金及核销；无模型／云提供方调用 | [原生记录](evidence/release-candidate-146/native-smoke.json) |
| V146-05 | 五项归档完整性、逐文件Git字节对应、展开私密材料扫描、全部旧标签与产物引用、SSH远端12项引用核对 | [封存](evidence/release-candidate-146/seal.json) · [固定清单](../releases/v0.1.0-rc.4/release-manifest.json) · [远端回读](evidence/release-candidate-146/remote-publication.json) |
| V146-06 | 当前范围、AC／TC适用分类、源码／证据／发版一致性及仓库发布检查 | [最终来源](evidence/release-candidate-146/final-source-check.json) · [范围](evidence/release-candidate-146/scope-check.json) · [发布](evidence/release-candidate-146/repository-check.json) |

本轮精选原生共45个子场景＋3个组汇总，48条通过记录，无失败或跳过；子场景与组汇总分别计数，不当作所有业务验收。数据库为独立合成夹具，已关闭自建PG；不修改原开发库100或既有API。原生构建不启动真机、浏览器、模拟器、Docker或WSL。

Android原生28项单元XML及实际Gradle任务状态见[构建范围](evidence/release-candidate-146/android-build-scope.json)；缓存结果不是新测试执行，单独JVM未重跑。实际APK内容核对是本轮重新执行。

## 复用与延期

[复用索引](evidence/release-candidate-146/reused-evidence.json)及[原样145证明](evidence/release-candidate-146/retained-functional-proof.json)绑定原始文件摘要：145原生两组32条、选定Web11／共享移动11、运行器57、类型与Web／两小程序、真实Web三项交互及三PNG／MP4／截图GIF；136完整Web452／共享移动537、121完整后端／运行器回归保留历史身份。未重录页面，旧失败／修复尝试不被覆盖。

Deferred by the user's accelerated-release instruction: full regression, additional UI journeys/recordings, development database upgrades, real Linux/CI/HTTPS/production PG/cloud/model/TG/platform integration, formal Android signing/publication and complete current-scope acceptance. The four-client target remains; development APK/local builds do not establish production readiness.

GitHub Releases API403为[144原始拒绝记录](evidence/release-candidate-144/github-release-api.json)，本轮未重试，也未创建Releases网页。SSH源码／产物交付与网页发布分别记录。仍23 active（14 P0／9 P1）、5 excluded、八项draft，完整goal及生产就绪均未标记完成。

[交付设计／使用](../development/146-frozen-release-delivery.md) · [固定下载](../releases/v0.1.0-rc.4/README.md) · [本轮及全部余项](../delivery/05-functional-closure-146.md)

服务器封包首次发现原构建器的裸Node版本读取用了Windows父进程路径（22.14.0），封存前置检查拒绝该次产物；[失败记录](evidence/release-candidate-146/attempts/01-server-node-resolution/reason.json)。随后在进程级明确Node22.23.4并重新构建；每项实际构建输入与145 Git字节对应，待提交146发版脚本／文档使整个工作区为dirty，原始标志如实保留，不伪装为干净工作区。

首次仓库发布检查误把尚未生成的当前输出回执列入文本输入，读取时失败；移除自输出项后重跑，生成回执继续接受完整候选／暂存扫描。[原失败与修正](evidence/release-candidate-146/attempts/02-publication-output-name/reason.json)。未绕过检查或改变任何产物。

第二次发布检查报告原生Gradle日志第2行的原始尾空格，以及当时尚未存在的自身回执链接；保留[完整失败报告](evidence/release-candidate-146/attempts/03-publication-log-envelope/repository-check.failed.json)。当前日志采用[准确UTF-8字符串封装](evidence/release-candidate-146/native-build-log.json)，原始空格／换行、字节长度和SHA-256可完整还原核对，不改变测试结果或产物。生成的回执已存在并参加后续扫描。
