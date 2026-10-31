# 增量144 · 冻结候选的必要检查与延期

业务源码固定 `e1c7a671cc4c7f52888a1e6ed72d7ce1a9c042d3`，结构109。本轮只封装现有功能。按用户加速首冻及功能收口的要求，只执行下列主流程和产物必要检查；没有将未执行内容改成通过。

## 当前执行

| 编号 | 检查 | 范围与结果 |
|---|---|---|
| Q144-01 | Linux／Web／秘书运行器 | 干净143源码实际交叉构建，Node22.23.4；792项包文件、109项结构清单；未执行目标Linux／systemd |
| Q144-02 | 八组原生HTTP／持久PG主流程 | 原生PG18.6，实际执行8组、87个子场景，共95条通过记录；无失败／跳过；不是95个独立业务场景 |
| Q144-03 | 微信／抖音 | 两平台当前源码分别实际构建，保存输入与输出摘要；本地API配置，无平台发布 |
| Q144-04 | Android | 当前源码生成新的开发APK，逐项检查签名／SDK／权限／JS／DEX／实际源码图；不运行真机或模拟器 |
| Q144-05 | 服务器包及五项交付 | 服务器内层清单、归档、3216项源码／文档及媒体Git blob核对；5153项展开成员的已知私密值／高置信密钥模式检查 |
| Q144-06 | 冻结与推送 | 新源码标签、独立产物分支及下载对象一致；原两个冻结候选不移动，不强推或覆盖无关远端工作 |

八组实际执行名称：

| 编号 | 原有测试组 | 主要覆盖 |
|---|---|---|
| B144-01 | TestProjectCollaborationIntegration | 当前成员／角色、撤销及项目协作授权 |
| B144-02 | TestSequentialApprovals | 固定审批规则、逐级人工审批、退回／撤回／冲突、MFA和原子回滚 |
| B144-03 | TestContractSignatureAndActivation | 准确合同修订、签署与生效 |
| B144-04 | TestBusinessCostRecording | 采购／报销／合同来源、实际费用、限额、版本及永久引用 |
| B144-05 | TestCostPaymentRecording | 实际费用到付款／核销，保留原费用与现金事实 |
| B144-06 | TestSubscriptionBilling | 账期、预测与实际费用分别记账、修订及终止核对 |
| B144-07 | TestUploadPromotionAndCommit | 准确字节回读、提升、提交、并发／取消及副本排队 |
| B144-08 | TestServiceStatus | 服务配置、事件及当前授权状态 |

执行记录：[native-smoke.json](evidence/release-candidate-144/native-smoke.json)。测试使用隔离合成业务资料及原生耐久PG；对象存储传输为测试替身，不代表真实OSS／R2。没有浏览器钩子、真实模型／TG／平台调用。自建数据库和PG在测试结束后关闭，开发PG14660及API18080保留。具体源码、执行命令、起止摘要和准确结果保留，不使用旧测试结果冒充本轮执行。

第一次范围文档检查发现：结构化GAP-18已更新为当前开发包，显示表仍沿用历史“新APK延期”文字。已只同步该表格单元，原失败记录及旧单元保留在[尝试记录](evidence/release-candidate-144/attempts/01-scope-table/reason.json)。业务源码、测试选择、冻结产物和旧证据均未改变，不重跑业务测试。

封装分支基于业务143，144交付指南位于main。首次推送前以普通快进提交仅修正产物README的指南链接；源码标签和五个产物字节不动，初次通过的发布记录保留在[说明链接记录](evidence/release-candidate-144/attempts/02-artifact-guide-links/reason.json)。最终引用与发布元数据重新核对，不重跑业务测试。

## 复用与未执行

143完整跨订阅审阅的三组选定HTTP／PG／MCP边界和运行时56项、136的完整Web452及共享移动537项保留原身份。143的28条结果包含3个组汇总、10个组合审阅子场景、14个单订阅审阅子场景及1个未启用浏览器钩子，不描述为28个新业务场景。121完整后端／运行器回归仍为历史出处。既有各轮实际PNG／MP4／GIF按原始字节入媒体ZIP，本轮不打开浏览器或重录。

Android的实际Gradle任务和缓存状态、原XML时间及字节见[原生范围](evidence/release-candidate-144/android-build-scope.json)，包检查见[apk-inspection.json](evidence/release-candidate-144/apk-inspection.json)。单独JVM套件未重跑；缓存结果不计为新执行。开发签名和包检查不等于正式签名发布或运行验收。

延期完整后端／运行器／Web／移动回归、额外四端UI旅程和媒体、开发库100→109更新、真实Linux／CI／HTTPS／云／模型／TG／生产PG／平台接入、正式Android签名发布和当前范围完整验收。移出真机、网络性能／实网120GB专项、历史迁移／容灾、AI费用控制，不重新作为门槛，也不计为通过。

已知私密值及高置信文本模式检查不等于完整秘密检测或画面OCR。便利源码ZIP完整包含应用源码，历史证据通过完整冻结Git标签查阅。新封装工具／记录在main144，业务标签和源码ZIP准确固定143；没有修改业务源码、既有迁移或早期证据。

[最终来源检查](evidence/release-candidate-144/final-source-check.json) · [封装证明](evidence/release-candidate-144/seal.json) · [原证据复用](evidence/release-candidate-144/reused-evidence.json) · [下载与操作](../releases/v0.1.0-rc.3/README.md) · [全部23项](../delivery/05-functional-closure-144.md)

## 远端发布结果

主线db14b740ec718542a1c19187693b788786420ee1、源码标签v0.1.0-rc.3及下载分支17f0fd77a7639eeedefdbe835ce2e77a55ed97f5已通过SSH成功推送，10项远端引用逐项相符，原两个冻结标签／产物不变。随后GitHub Releases API请求因现有凭据账号停用返回403，未创建页面；五个固定下载包不受影响。[准确回执](evidence/release-candidate-144/github-release-api.json)。本说明补充提交只刷新文档／发布元数据检查，没有重跑业务测试、构建或媒体，也不修改封存的源码标签、清单和五个产物。
