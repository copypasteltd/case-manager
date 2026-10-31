# 增量152：冻结产物核验 / Frozen artifact verification

[最终源检查](evidence/release-candidate-152/final-source-check.json)核对当前服务器输入、结构112、原生构建结果、两小程序、Android开发包、准确源码映射、五包展开内容、完整应用Git blob重建、清单和六旧冻结引用。

[保留151证明](evidence/release-candidate-152/retained-functional-proof.json)逐记录哈希与固定提交核对；当前151原生36条记录、共享表单各4项、运行器58项及真实Web3项／新媒体按原来源保留，152不再重跑。较早完整后端121／完整客户端136同样不重新执行。JVM缓存状态及XML原时间以android-build-scope.json为准，不把缓存算新测试。Linux为交叉构建，不代表已在目标服务器执行。

按用户要求跳过扩大回归、额外移动UI、真机、网络性能、实网120GB、历史迁移与容灾、AI费用控制。后五项是移出范围，前三者中扩大回归／额外移动UI是本次延期；不计通过。正式签名、真实云／模型／TG／平台、原生Linux／生产PG／HTTPS及完整公司验收仍开放。

English: Expanded artifact and source checks are fresh; primary business tests/media are retained exactly from151. The development APK is rebuilt but formal signing remains open. No production, device or target Linux execution is established. Remaining23 packages stay active.
