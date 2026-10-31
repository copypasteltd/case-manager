# 增量154：冻结产物核验 / Frozen artifact verification

[最终源检查](evidence/release-candidate-154/final-source-check.json)核对固定业务153、结构113、服务器输入、原生构建、两小程序、Android开发APK与实际源码映射、五包展开内容、应用Git字节重建、摘要、源码标签与产物分支。七个旧冻结引用、113项既有结构升级及全部历史证据和媒体保持原字节。

| 编号 | 本轮核验 | 证据 |
| --- | --- | --- |
| 1 | 服务器／Web／运行器原生构建与固定输入 | `server-build-scope.json`、发布目录`server-build.json` |
| 2 | 微信、抖音构建和153提醒标记 | `miniapps-build.json`、`miniapps-inspection.json` |
| 3 | Android开发包、签名结构及实际当前模块映射 | `build.json`、`apk-inspection.json` |
| 4 | JVM任务实际缓存状态，28条XML原时间与来源 | `android-build-scope.json`；不将缓存算新测试 |
| 5 | 完整应用源码、媒体字节及展开私密信息检查 | `seal.json`、固定清单与服务器内层清单 |
| 6 | 23包／5项移出与原30AC／72TC适用性分类 | 范围及适用性检查；业务用例通过数为0 |
| 7 | 发布文件检查、Git远端引用与所有旧冻结保留 | `repository-check.json`、`remote-publication.json` |

[保留153证明](evidence/release-candidate-154/retained-functional-proof.json)及[原生记录](evidence/release-candidate-154/retained-native-proof.json)逐字节来自原出处。十四个业务子场景加一条汇总、传输十八条通过记录、Web4／移动共享4、三项实际Web交互与媒体均未在154重跑。运行器58项保留151，完整后端保留121、完整客户端保留136，不用本轮构建替代全量业务验收。

此前153两次失败的原生集中检查、两次失败录制及修复记录仍保留在原目录，不重命名为成功。154单独保存新证据，任何失败须保留后另做新尝试。既有开发PG100和API不升级、不重启。无新浏览器页面或临时服务，因此不重录未改动的UI。

English: Fresh154 build and expanded archive/source inspections are distinct from retained153 business proof. The APK is rebuilt with current153 sources, but formal signing remains open. Cached JVM results remain labelled as cached. Linux is cross-built and not executed on the target host. No real TG, cloud, model, platform or production acceptance is established.

扩大业务回归、额外移动UI旅程按用户加速要求延期。真机、网络性能／实网120GB专项、历史迁移与容灾、AI费用控制属于移出范围，不能计通过。正常结构升级、生产PG、OSS＋R2、AI授权／隔离／审计／技术限制及正式签名等保留。全部[23项未完成](../delivery/05-functional-closure-154.md)继续开放；真实配置后续提供。

153最终证明中的旧功能基线元数据从固定153的Git对象验证；当前功能基线正常推进到154，其余被引用证据继续对当前文件核对。未修改或重新出具153证明。 / The superseded153 baseline is checked from frozen Git; current baseline metadata advances to154 while all other referenced proof stays byte-identical.

154第一次范围检查发现GAP-18正文已更新但结构化清单仍写旧150版本，失败原记录保留在attempts/scope-table-before-sync.json；同步两处交付描述后重新核对，未改变23项状态、优先级或排除范围。

首次待发布检查本体21项通过、退出码0；本机摘要脚本把数值checks当列表，报告写入后摘要输出异常。原检查数据和处理错误分别保留于attempts/repository-before-remote.json、publication-summary-error.json；修正本机摘要输出后，再检查包含远端回执的最终暂存内容，不将摘要异常表述成业务失败或掩盖记录。
