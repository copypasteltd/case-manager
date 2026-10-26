# 增量71验证记录

日期：2026-10-26。已实现秘书订阅分页发现、完整日期窗口逐笔核对及实际关联费用／资金原记录；Web／Taro 沿用固定来源阅读器。实现和 SR-01—08 约束见[功能契约](../../../development/71-secretary-subscription-detail-review.md)。[最终来源核对](final-source-check.json)24项通过，完整平台 Goal 仍在执行。

## 1. 实际执行与复用

| 范围 | 证据 | 结果与边界 |
| --- | --- | --- |
| 完整后端 | [结果](result.json)／[分批状态](batch-progress.json) | 8批，143个顶层测试、887条结果／803个末级场景，0失败／跳过；固定源码完成全部发现的测试，新增14项实际业务集成场景 |
| SDK／Skill | [测试与构建](runtime-build.json) | 36项受控测试、TypeScript 构建与8项来源包校验通过；明确日期输入和当前运行明细回执校验；无真实模型 |
| Web 实际来源 | [浏览器](../subscription-review-web/result.json)／[来源](../subscription-review-web/source-manifest.json) | 4条检查通过，真实内部账号／MFA／HTTP，准确财务事实、三项原始来源摘要及四份实际引用；正文共5段全部核对 |
| Taro 实际来源 | [浏览器](../subscription-review-mobile/result.json)／[来源](../subscription-review-mobile/source-manifest.json) | 3条检查通过，同一冻结来源生成 H5 后读取四份快照的全部5段，回到首段并收起；不是小程序平台或 Android 真机证明 |
| 截图 | [Web复核](../subscription-review-web/visual-review.json)／[Taro复核](../subscription-review-mobile/visual-review.json) | 已查看本次实际展开阅读器，记录来源与图像摘要；固定证据和任务进度分开，范围提示、换行及后续段控制可见；未重做风格文档 |
| Web／移动原测试 | [Web75项](../project-overview/web-build.json)／[移动89项](../mobile-project-overview/mobile-build.json) | 产品源码与原证据摘要匹配，复用既有测试、生产编译与双小程序各16页编译；不计作本轮新执行 |
| Android／JVM | [增量70 APK](../mobile-project-overview/build.json)／[17项包检查](../mobile-project-overview/apk-inspection.json)／[最终纯Java来源](../android-upload/jvm-source-proof.json) | Android 源码及冻结 APK 未变，无重建／真机声明；纯Java的25例及4份 XML 摘要仍匹配，不重复累计为新执行 |
| Linux | [交叉编译](linux-build.json) | 当前源码 linux/amd64 编译通过，产物摘要固定，未执行／部署 |
| 日常开发 | [更新](development-update.json)／[基础核对](development-runtime.json)／[业务保护](development-subscription-review.json) | 65项迁移逐项摘要匹配；只替换精确所属 API，PG／通知 Worker／Vite 未重启；无活动模型、启用TG、日常财务快照和测试库残留 |

本轮后端来源 SHA-256：`0e9ac5dcf5ea7377e8319d8a6fcbceb4987c244fd1dd3652c06c69b9161539c8`。SDK／Schema／链接 Skill 指令的准确摘要见 runtime-build.json；两浏览器各自绑定全部实际产品与验证脚本，不拼接不同来源的通过结果。

## 2. 关键断言

| 约束 | 实际证据 |
| --- | --- |
| 不能以发现冒充完成 | 真实列表返回实际 ID；仅 list_subscriptions 回执的 completed 被 Go 和 SDK 拒绝 |
| 不共享更丰富的私有结算 | Bob 保留费用阅读但没有付款阅读时共享明细失败；获得相同阅读权后，即使编辑权限不同也成功 |
| 不混算金额 | 预测100、费用100.12、实际分配40、余额60.12逐项对照原 API，全部为十进制文本，保留原币种和分配份额 |
| 保留覆盖缺口 | 2月未生成账期时 missing_scheduled_periods 为1，coverage_complete=false；不补零或推断没有义务 |
| 真实关系与严格范围 | 错误订阅／账期／记录、无效日期、超过366天及自带 project_id 失败；公司会话不读取项目名下订阅 |
| 真实分页与界限 | 21项订阅分20＋1页，不重复；游标拒绝跨筛选／会话；120个真实生成日账期触发100项上限，未落不完整快照 |
| 固定证据与事务 | UPDATE／DELETE 与无对应准确回执的 INSERT 被拒绝；相同 invocation 重放不变；受控回执写入失败时快照回滚 |
| 当前权限撤回历史 | 不能跨会话打开；付款阅读撤回后旧丰富快照隐藏，费用仍可读但结算受限；凭证文件进入回收站后旧原记录快照隐藏 |

## 3. 中间尝试与清理

首次 Web 检查错误假定来源顺序；第二次错误假定只有三份引用。Go 控制器会保留本次真实读到的2月缺口，所以实际有四份来源。修正为核对真实引用及全部正文后通过，再补拍展开阅读器并完整重跑。前两次失败和第三次通过均保存在 [web-01](../subscription-review-web/attempts/web-01/result.json)、[web-02](../subscription-review-web/attempts/web-02/result.json)、[web-03](../subscription-review-web/attempts/web-03/result.json)，最终结果为同一产品源码的第四次完整执行。

完整后端首次第5批在既有项目归档场景的夹具 DROP DATABASE 清理阶段超过2分钟，整批记录为失败，不算通过。保存[原始状态](attempts/backend-batch-05-timeout/batch-progress.json)和[原始日志](attempts/backend-batch-05-timeout/backend-batch-5.log)，在来源不变且浏览器构建结束后从第5批重新执行全部20项，其后完成剩余批次。第1—4批准确来源未变，可按原分批协议保留。

第一次开发检查发现该失败 DROP 留下一个无法连接的临时库，记录在[清理前结果](attempts/backend-batch-05-timeout/development-before-cleanup.json)。根据本轮 PG 错误日志中的准确库名、所属端口、owner、connection_limit=-2、0连接和已结束的全部测试核对后，只删除该确切临时库，不使用 FORCE，不重启 PG，不修改日常业务库。见[清理证明](fixture-cleanup.json)；重新核验后为0残留。

最终来源脚本首次误读 manifest 的 id 字段而退出，修正为实际的 skill_id 后24项全部通过。该修正只涉及证据脚本，没有改变已验证的业务、运行时或客户端源码。

所有浏览器所属标签页、上下文、浏览器、5189临时服务和私有夹具已释放。S: 为仍在使用的所属开发路径别名，保留；其他任务的服务与 R: 不处理。

## 4. 尚未验收

八项 Skill 均 draft，subscription-review 源码1.2.0 有六项只读工具。真实模型异常判断质量、发布／停用／回滚、全公司跨项目审阅计划、财务写入与 TG 财务披露仍开放。没有真实云、TG 外发、120GB、2TB迁移、平台、实体设备或生产部署／恢复声明。依赖锁未变，既有生产安全问题继续保留，完整差异见[28项清单](../../../delivery/03-goal-gap-assessment.md)。
