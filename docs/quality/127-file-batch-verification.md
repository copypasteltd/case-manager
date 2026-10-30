# 固定清单文件批量整理验证 · Increment127

本轮使用原生Windows、隔离PG及实际HTTP接口，Web和共享Taro H5分别完成两文件的移动、回收、恢复，并从原文件重新打开回执。测试资料为合成的正式版本元数据；没有真实对象上传、设备、云或模型验收声明。

| 范围 | 证据 | 当前检查内容 |
|---|---|---|
| 后端 | [backend.json](evidence/file-batch/backend.json) | 五个选定HTTP／PG套件共51条通过记录，0失败／跳过；固定清单、普通名称空间、正式版本、范围移动及保留。当前不是全后端回归。 |
| Web | [web-tests.json](evidence/file-batch/web-tests.json) | 全部379项通过。 |
| 共享移动 | [mobile-tests.json](evidence/file-batch/mobile-tests.json) | 全部465项通过；含与Web共用的27个准确清单／回执案例。 |
| 类型与构建 | [类型](evidence/file-batch/typecheck.json)、[Web](evidence/file-batch/web-build.json)、[微信](evidence/file-batch/weapp-build.json)、[抖音](evidence/file-batch/douyin-build.json) | 当前源码与明确本地API配置；不替代真实平台身份／发布。 |
| Android JS | [构建](evidence/file-batch/native-bundle.json)、[准确模块](evidence/file-batch/android-js-content.json) | 既有公开四工作线程Metro／Windows原生文件适配，当前1,110模块、4个本轮准确页面／契约来源逐字核对；实际492.676秒，未新构建APK。 |
| Web／H5旅程 | [Web](evidence/file-batch-web/result.json)、[H5](evidence/file-batch-mobile/result.json) | 实际勾选、目录分析、本人确认／MFA、三次整批执行、原内容头／历史保持和持久结果重新打开。 |
| 本地正常升级 | [更新](evidence/file-batch/development-update.json)、[只读检查](evidence/file-batch/development-project-resources.json) | 94→95，原PG保持，旧94份迁移及财务／OA／文件事实保持；仅替换已验证属于本仓库的开发服务。 |
| 素材与目检 | [素材](evidence/file-batch/media.json)、[目检](evidence/file-batch/visual-review.json) | 6张实际PNG、2条本人验证后的MP4、2个每张2秒的三截图导览GIF。 |
| 最终源码／范围 | [源码](evidence/file-batch/final-source-check.json)、[范围](evidence/file-batch/scope-check.json)、[适用性](evidence/file-batch/acceptance-applicability-check.json) | 当前源码绑定、23 active／5 excluded、30AC／72TC适用性分类。分类本身不执行业务案例。 |
| 仓库 | [发布检查](evidence/file-batch/repository-check.json) | 当前可发布文件、相对链接、敏感资料与保留证据扫描；暂存检查另执行且不改报告。 |

## 实际断言

后端覆盖固定排序与完整成员、计划只读、三条普通操作、正式内容头和历史保持、源文件历史分页、创建／执行／撤销幂等、SQL不可改写、名称与目录同名冲突、保留锁、文件／头／目标／祖先变动、第二个实际子命令注入失败后全业务事实回滚、相同来源并发一成一败、创建人／agent／伪造会话边界、严格输入与查询、确认摘要、实际会话MFA过期、重放前再核对MFA、计划过期和组织授权版本变化。

失败回滚检查分别核对每个文件的原位置／修订，以及成功审计、Outbox、幂等成功记录和仍待处理的固定计划。独立失败拒绝审计允许保留。来源文件采用合成元数据，两个正式版本的ID／序号／摘要保持，未声称字节云校验。

首次Web录屏准备在浏览器启动前遇到新增文件路由正则解析错误，已修复；原失败记录保留在[attempt01](evidence/file-batch/attempts/01-before-recording-route-fix/README.md)。夹具退出、数据库清理和临时服务停止均有记录，不计为UI通过。

首次共享移动捕获点击复选框外围空白，选择仍为0且按钮正确保持禁用，捕获失败。随后仅修正捕获脚本为准确可见文字点击及新TOTP等待；应用字节、断言和时限保持。先前通过的Web与检查按原摘要保留在[attempt02](evidence/file-batch/attempts/02-before-mobile-capture-click-fix/README.md)，在途自建Metro子进程已停止，最终九项检查与两端捕获全部重新执行，不改写旧记录为通过。

移动选择恢复后，第二次捕获的目标行选择器错误包含页面根，导致行内按钮无法匹配；仅调整为行内按钮定位。之前通过Web及检查保留[attempt03](evidence/file-batch/attempts/03-before-mobile-target-locator-fix/README.md)。最终捕获调通后再固定来源并完整重跑；未改变应用逻辑、业务断言或时限。

共享移动第三次捕获已完成移动及回收；恢复筛选因向自带页面定位的选择器传入已定位节点而失败，仅修正该调用。部分结果保留[attempt04](evidence/file-batch/attempts/04-before-mobile-state-picker-fix/README.md)，不算完整三流程通过。

最终两端捕获通过后，检查发现新增素材编码分支仍沿用了旧存储工作任务的说明，仅修正本功能的说明文字。刚启动的自建原生编译及其四个子进程在修改前停止，记录与原输入保留[attempt05](evidence/file-batch/attempts/05-before-media-description-fix/README.md)；它不算最终构建通过。浏览器捕获使用的应用及捕获助手字节没有因此改变。最终九项检查绑定修正后的输入。

原生编译前[只读读取本机已安装依赖](evidence/file-batch/dependency-cache-preparation.json)61,359个文件、398,788,189字节以准备系统缓存；未修改依赖、来源、解析结果或产物，也未联网。此操作只属于本机构建准备，不用于网络性能或生产验收。

首轮仓库候选检查拒绝两处文档包装问题：新指南尚未写入当前移动测试数465，原始中止构建日志含原有尾部空白。已补齐三份入口／中英文指南的实际检查数据，原日志按准确字节无损gzip归档并记录解压前后摘要。原失败报告保留[attempt06](evidence/file-batch/attempts/06-before-publication-doc-fixes/README.md)，范围与发布检查重新执行；应用来源及九项已执行结果没有变化。

## 保留证据和范围

[增量126证据](evidence/cost-payment/final-source-check.json)及全部旧素材保持原字节。[复用清单](evidence/file-batch/reused-evidence.json)区分历史证据；旧121全回归／runtime／开发APK不改称当前127结果。本轮不新跑未修改runtime全套、不构建正式签名Android包，不把共享H5或JS bundle当作正式平台验收。

真实云、正式小程序与Android、Linux模型、生产数据库、依赖冻结及完整业务／发布验收继续留在[全部23项清单](../delivery/05-functional-closure-127.md)。AI费用、真机、网络性能、历史迁移／容灾等移出项不回到发布门槛。
