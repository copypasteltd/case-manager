# 增量152：冻结交付 / Frozen delivery

业务151／结构112源码已固定，152只执行原生构建、准确源绑定、归档、敏感材料扫描、清单与发布，不继续新增业务功能。原生Linux／Web／秘书运行器、微信／抖音、Android开发包以及完整应用源码与媒体使用同一业务源码。

使用独立152构建、检查、封装和发布脚本。源码标签固定业务提交；产物分支只加入经哈希核对的五包与交付清单，不覆盖原主线或六个旧标签。源码ZIP含全部应用实现；较大的历史证据留在固定源码标签，所有媒体另包。

[下载与操作](../releases/v0.1.0-rc.7/README.md)、[验证与延期](../quality/152-frozen-release-verification.md)、[所有剩余工作](../delivery/05-functional-closure-152.md)。包内数据库结构112须正常升级；原开发PG100及既有API保持原状。正式服务账号、模型、TG、云存储和平台、正式签名／生产验收保持独立开放。

English:152 freezes the complete151 application source and packages matching native server/Web/runtime, miniapps and development APK outputs. New artifact/source references never move an earlier freeze. Expanded archives are checked for exact Git source bytes and known private material. Selected151 business evidence is retained without replay. This is internal delivery, without a production deployment or formal signing claim.
