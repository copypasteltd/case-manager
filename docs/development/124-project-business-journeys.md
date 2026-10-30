# 增量124：项目业务连续操作与移动订阅 / Project business journeys and mobile subscriptions

本轮直接推进应用代码：从项目进入合同、采购、报销、实际费用、实际资金和订阅，查询、新建、保存和返回都保留当前项目。移动共享页面补上订阅管理与账期处理，覆盖微信、抖音、Android的共享实现。正式平台发布和完整业务验收仍按当前范围登记，不因局部通过而结清工作包。

## 业务入口与使用

1. 打开项目 → 业务概览 → 项目业务办理。六类入口分别提供“查看项目…”及当前可用的“起草项目…”。关闭、归档、取消或缺少相应权限时保留查阅入口，停止提供新建入口。
2. 项目列表显式显示当前项目。搜索、翻页、刷新沿用同一项目；“退出项目范围”明确回到全部可读记录，不静默切换。
3. 项目新建表单继承项目及分类；项目归属固定。需要换项目时关闭表单，从相应项目重新起草。保存只生成该业务自己的草稿与修订，不自动送审、批准、确认费用、登记银行资金或启用供应方订阅。
4. 合同、采购、报销继续使用已有的准备申请、固定版本审批及履行业务。费用与资金继续使用独立确认、分配、结算、更正和冲销流程。
5. Web详情提供返回项目概览及同项目同类业务入口；共享移动页面保留项目返回与过滤列表。当前对象项目归属由服务端资料决定，不能用URL替换已有记录的归属。

## 订阅与月开支的移动操作

| 编号 | 功能 | 使用与结果 |
|---|---|---|
| SUB-01 | 新建／修订 | 产品、类别、供应方固定修订、合资格负责人、时区、起始／试用、周期／原始日历、续费方式、提醒、价格期间、备注和原因；允许保存未知草稿。 |
| SUB-02 | 启用 | 服务端核对完整日历、供应方、负责人和首期价格；不在外部平台购买服务。 |
| SUB-03 | 暂停／恢复／取消 | 明确人工依据与新近MFA；暂停、取消、历史恢复需额外人工确认。内部状态与供应方实际停扣分开记录。 |
| SUB-04 | 核实供应方 | 保存供应方实际状态、核实时间及最多20个固定文件版本；非未知状态要求凭证。 |
| SUB-05 | 调价 | 新价格保留有效期间、原币精度和套餐说明；已有账期不被静默重算。 |
| SUB-06 | 生成账期 | 按订阅时区自然日、最多366日窗口生成；新增数、已存在数、缺价格日期和覆盖完整性作为回执保留。 |
| SUB-07 | 异常账期 | 对已到达的停订区间收费日明确登记；不自动登记扣款或费用。 |
| SUB-08 | 关联费用 | 从已确认原费用中选择同供应方、币种且未归属其他账期的记录；固定费用版本、修订和内容摘要，服务端再次核对范围。 |
| SUB-09 | 人工核对 | 固定当前费用集合，选择核对凭证；预计与实际不同需明确确认差异，可保留发票待补异常。费用冲销使旧核对失效，不自动登记银行付款。 |
| SUB-10 | 重估／免付／作废／重开 | 重估保留旧预测；免付和作废只处理符合条件、从未关联费用的预测。重开保留旧核对历史。 |
| SUB-11 | 归档／历史恢复 | 服务端核对取消生效、供应方停费及已存账期核对情况；恢复只开放历史核对，不恢复供应方未来收费。 |
| SUB-12 | 修订／操作记录 | 移动订阅展示修订索引；账期展示预测修订与操作事件、当时费用快照及可读固定凭证。Web保留既有完整详情及历史入口。 |

修改已启用的订阅保持原始日历和价格，另以调价操作记录变化。现有条件和最新权限由原有业务命令决定；前端入口不授予权限，也不预先锁定未来操作资格。

## 查询与权限约束

`GET /v1/projects/{id}/business-context`提供当前人类交互会话可读项目快照、事务时间和六类新建能力。只支持Web、WeChat、Douyin、Android；不作为AI或TG授权入口。读取核对组织／用户权限版本及会话，严格资料的普通新建入口不虚构对象授权。事务快照不构成写入预留；后续写命令仍独立鉴权、校验引用并锁定项目状态。

合同、采购、报销新增有效的`company_only=true`查询，拒绝同时指定项目；分页绑定公司范围，不能将公司游标用于全局或项目列表。

费用、资金列表的`project_records=true`仅在指定项目时可用，表示“本项目资料或实际项目份额”，让尚未分摊的项目草稿可被找到。原有不带此参数的`project_id`仍表示实际分摊／资金归属查询。该开关不适用于金额报表，不能将草稿原额当成发生支出。项目概览记录卡片同步包含项目资料；金额卡片仍调用原有实际份额汇总。

客户端校验UUID、冲突范围和六类能力回执；Web重复范围参数会停止加载，不退回全部记录。项目读取失败或撤权时不显示先前项目事实作为新结果。项目类型共用契约，避免四端自行猜测新建资格。

## 草稿、冲突与重试

移动会话草稿限15分钟、最多32项及共享10万字符，只保留白名单文本；订阅价格文本深复制。供应方、文件凭证、MFA、人类确认、API回执与命令载荷不入缓存。草稿按用户、会话、授权版本、对象版本／修订隔离，项目新建草稿包含项目身份。

新订阅使用独立永久来源标识；每条修改命令使用固定版本、修订、摘要及原因，沿用已有幂等提交机制。403／404／409／412及结果不确定时停止继续提交，保留文字，要求重新读取依据。生成账期明确展示不完整覆盖，不把HTTP成功等同窗口完整。

## 源码与验证范围

- 后端：`project/business_context.go`、HTTP路由、合同／采购／报销公司过滤、费用／资金项目记录过滤、项目概览。
- Web：`ProjectBusinessHub`、`ProjectBusinessScope`及六类列表／编辑／返回入口。
- 共享移动：项目概览、通用列表、业务／财务编辑器、订阅操作、账期核对；原生控件沿用现有平台封装。
- 共用契约：`project-business.ts`、OpenAPI与生成类型；订阅文本输入与会话草稿白名单。

无需新增数据库结构，本轮使用schema94。新代码不执行数据迁移或容灾，也不引入AI费用控制、真机或网络性能验收。验证结果与媒体见[本轮记录](../quality/124-project-business-verification.md)；全部未完成项见[编号清单](../delivery/03-goal-gap-assessment.md)。

## English implementation and usage

Open a project → business overview → project business actions. All six collections and new drafts preserve the selected project; each command independently checks current authority and project state. Explicitly leave the project scope to view all readable records. Saved drafts do not imply submission, approval, incurred expense, bank payment or vendor activation.

Shared mobile pages now expose subscription creation/editing, calendar and price configuration, qualified owners, fixed supplier evidence, lifecycle/vendor checks, bounded period generation, price changes, unexpected stopped-period registration, cost linking and human review. Sensitive operations require a fresh MFA step. A period review binds the exact actual-cost fingerprint; later reversals invalidate old review. Price forecasts and bank cash remain separate facts.

The new read-only business-context endpoint reports six ordinary creation capabilities for the currently authorized human session. It grants no authority and reserves no future write. Project-owned cost/payment draft navigation uses an explicit list-only option, while financial reports retain actual allocation valuation. Company-only filtering now works for contracts, purchases and claims, with pagination isolation.

This increment changes application source and adds focused native HTTP/PostgreSQL and client checks plus genuine isolated Web/H5 interactions. Refer to the verification record for executed counts and retained evidence. Shared source/build checks do not replace official Android signing, mini-program review, live cloud/model/production or full business acceptance.


## 本机Android JS编译边界 / Native JS review

`build_native_js_review.py`只调用已发布增量121的bundle适配函数：四Worker Metro、生成目录排除、当前工作区原生文件系统兼容及同一no-Expo源映射检查。应用入口、resolver、实际源字节、minification和资源处理保持既有编译器语义。它不调用旧证据写入，不执行Gradle或生成APK；环境采用原生构建白名单，仅带明确API／profile与必要系统变量。完整正式Android发布仍按GAP-16处理。

The current JS-only review reuses the published121 native Windows bundle adapter, without its main/APK runner or historical evidence writer. It keeps actual filesystem results and current source/resolution semantics, four-worker Metro, generated-output exclusions, minification/assets/maps and the native no-Expo guard. This yields current JS compilation evidence, not a signed Android artifact.
