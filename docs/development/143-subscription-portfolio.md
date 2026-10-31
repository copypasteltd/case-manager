# 增量143 · 跨订阅完整审阅 / Complete subscription portfolio review

## 使用与业务口径

秘书会话中的“跨订阅完整审阅”允许人工选择明确开始日与不含结束日，再读取准确会话范围内全部可读订阅。列表显示每个订阅的状态、已存／应有／缺失账期，以及去重后的关联费用、付款和共享原记录数。展开“查看完整逐笔依据”可以在同一次完整内容中翻页；刷新页面清除预览，需明确重新读取。

日期按订阅本地自然日的账期开始日选择，最多366日。日期限制账期，不把没有生成账期的订阅过滤掉；所有可读状态包括draft、active、paused、cancelled、archived。公司会话仅含公司订阅，项目会话仅含该项目；不外推全部项目或供应商实际扣费。没有可读订阅时集合枚举可完成，但日程覆盖仍未知，不宣称零支出或没有义务。

`subscription_set_complete`只表示读取时刻、发起人有权且允许AI处理、在会话分类范围内的订阅集合已完整枚举。`schedule_coverage_known`和`…complete`单独描述应有账期是否确定、是否已生成。覆盖完整不等于供应商停费、付款完成或金额正确。不同币种不相加；共享费用／付款关联可能是正常分摊，不据此认定重复收费。

人工预览是只读临时结果，不写财务事实，也不冒充不可变秘书报告。需要秘书分析时，在原请求中明确选择订阅审阅并说明日期，要求使用`read_subscription_portfolio`；工具实际调用参数和成功回执是准确来源，普通消息文字不构成新的强制结构化意图。真实模型选工具、理解日期和判断异常的质量仍待验收。

## 接口、权限与实现

| 接口／工具 | 参数 | 结果和约束 |
|---|---|---|
| GET `/v1/conversations/{id}/subscription-review` | 必需from、to；不接受重复或额外参数 | 当前真人会话、conversation.read、允许AI／渠道、全部当前成员；返回会话修订、安全上下文、读取时间、摘要、完整正文和SHA-256 |
| `read_subscription_portfolio` | 仅from、to，真实自然日、有序、不超过366日 | subscription-review专属只读工具；准确执行租约、运行授权与隔离证明；TG不开放 |
| 原agent_finance_report读取 | 准确会话及固定报告ID | 不可变原值，继续核对当前全体成员和全部私有依赖；新增成员不能自动取得旧报告 |

后台先枚举发起人的全部可读、允许AI处理及具有report.read权限的订阅，再逐个复用现有完整单订阅审阅。每名当前成员必须能读取每笔费用、现金分配、固定凭证、供应方及相关链路；任何一笔不满足即整次拒绝，不静默删掉难读项。不可读订阅既不列名也不提供隐藏数量。人工读取用repeatable-read事务，模型工具复用组织串行的执行事务及普通业务写入锁序，无网络或模型请求置于数据库事务内。

上限为100个订阅、每个100个账期、200个费用／分配、合并500个准确依赖及完整正文89000字节。超过任一上限整笔失败，不返回“完成”的前缀；可缩小日期或使用准确项目会话。日期无法缩小订阅数量。原有月度摘要、发现分页、单订阅及单原记录审阅保留各自范围，发现分页仍不能证明审阅完成。

组合内容保留全部单订阅逐笔报告，包括配置、价格、缺失账期、原费用／付款、项目份额、真实现金分配和退款口径。费用、付款按实际ID去重；跨订阅共享原记录单独列出关联ID，不输出混合币种总额。依赖按完整来源及关联字段去重，不能只按文件ID丢失链路。

工具沿用`agent_finance_reports`不可变台账：完整正文、依赖、原受众、固定项目／分类、文档摘要与一条准确typed receipt同事务写入；审计或回执失败完整回滚。重放和完成结果必须引用本次成功报告。历史内容保留读取时刻事实，不承诺以后新增订阅自动并入旧报告；继续执行现有历史版本、关系、当前权限与保护检查。

四端共用日期和准确回包检查，比较会话修订、安全上下文、日期、项目、完整摘要和正文SHA；正文以惰性文本分页显示，不作为HTML或指令运行。移动端离开页面关闭请求并清除预览；Web日期、会话或授权变化切换查询并隐藏旧结果。没有把预览放入持久浏览器缓存。

## 升级、来源与发布边界

正常迁移109扩展既有工具／报告视图白名单及准确回执触发器，保留旧报告约束、不可变性与迁移1–108字节。个人开发PG仍结构100，不自动更新用户服务；运行当前后端前按原生部署手册正常升级至109。OpenAPI为380路径／460操作／851 schema。

subscription-review从draft1.3.0升为draft1.4.0，来源摘要改变，旧安装证明不能被视为新包验收。其他七项包没有源码变动，八项仍draft。MCP、Go执行／完成核对、JS工具允许名单和结果验证均接通新工具；AI费用控制仍移出，授权、审计、隔离和技术限制继续保留。

两个冻结候选不变，rc.2与开发APK137仍业务136／结构103，不包含138–143。143只交付当前主线源码和选定检查／媒体，没有新APK或新的冻结包。全量回归、完整移动UI及真实模型／Linux／云／平台／生产验收继续延期或开放。整个GAP-12／18及23个工作包没有因此关闭。

## English usage and design

Open an active secretary conversation, enter explicit local start and exclusive end dates, then read the complete portfolio. The reply enumerates every subscription the actor may read and process with AI in the exact conversation project/classification. All states are included, even when no period was generated. Company scope excludes project subscriptions. Every current participant must read every detailed dependency; one failure rejects the entire review.

Set enumeration and schedule coverage are distinct. Empty sets, draft schedules and missing periods never establish zero obligations, payment completion or vendor cancellation. Original currencies remain separate. Shared financial records are deduplicated by identity and listed without alleging duplicate charges. The human preview is ephemeral and does not alter financial facts. The actual subscription-review tool stores one immutable, receipted full snapshot, checked again for current authority when cited or read later.

Bounded complete results reject instead of truncating:100 subscriptions,500 combined dependencies and89000 content bytes, with existing per-subscription limits. Schema109 is a normal upgrade; subscription-review1.4.0 remains draft. Current source and focused evidence are separate from frozen rc.2/APK137 business136/schema103. Full regression, new native artifacts and external acceptance remain deferred.

[关键验证与延期](../quality/143-subscription-portfolio-verification.md) · [本轮及全部余项](../delivery/05-functional-closure-143.md) · [当前范围](../delivery/04-current-scope-and-acceptance.md)
