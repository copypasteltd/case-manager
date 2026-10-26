# 秘书订阅逐笔审阅与关联原记录

本增量推进 GAP-03／12／13，在[月度摘要](63-agent-finance-summary.md)上补充实际订阅发现、账期逐笔核对和关联费用／资金原记录。Go 控制器负责范围、当前权限和不可变证据；SDK 与 MCP 使用同一白名单。代码在 app/，文档在 docs/。真实模型、Skill 正式发布和生产财务数据尚未启用。

## 1. 功能与验收约束

| 编号 | 员工行为 | 实现和限制 |
| --- | --- | --- |
| SR-01 | 找到实际订阅 | 当前会话内真实 ID、修订、状态、标题，20条一页；游标绑定员工、组织授权版本、会话、受众、项目及筛选 |
| SR-02 | 逐笔核对账期 | 明确 subscription_id、from、to；to 为不包含的截止日期。返回当前配置、价格期间、全部所选已存账期及其实际费用与分配 |
| SR-03 | 深入读取原记录 | subscription_id、period_id、kind、id 同时校验实际关系；不能用任意财务 ID 逃出订阅范围 |
| SR-04 | 区分三类金额 | 保留十进制字符串及币种，区分预测、费用原额、项目分摊、资金原额和实际分配，不据整笔付款推断本订阅现金 |
| SR-05 | 识别缺口与候选异常 | 未生成账期、覆盖未知、缺发票、超额／不足保留原业务标记；内部停订不证明供应商停费，候选异常不证明重复扣款 |
| SR-06 | 多端查阅固定证据 | Web／Taro 复用原快照阅读器，按准确来源摘要分页读取全部正文、回到首段、收起，不以当前报表替换历史 |
| SR-07 | 保护共同阅读范围 | 每名成员必须能读取相同业务事实；附件、往来名称、业务关联或结算事实不同则整份拒绝。不同编辑权限不妨碍阅读 |
| SR-08 | 有实际读取才完成 | 列表发现、通用资源读取或 inspect_operation 不构成完成。Go／SDK 要求本次成功的财务摘要、逐笔审阅或原记录回执及准确引用 |

## 2. 工具参数与上限

| 工具 | 严格参数 | 固定快照 view | 上限 |
| --- | --- | --- | --- |
| list_subscriptions | 可选 query、state、cursor | subscription_list | query 最多200字符；状态白名单；20条／页，真实签名游标 |
| read_subscription_review | subscription_id、from、to | subscription_review | 规范小写 UUID、真实日期、最多366天；100个账期、200条费用、200条分配、500项私有依赖 |
| read_finance_record | subscription_id、period_id、kind、id | finance_record | kind 仅 cost／payment；实际关联、原始修订和全体受众的当前权限 |

正文最多90,000 UTF-8字节。超限直接失败，要求缩小范围，不持久化看似完整的前缀。page_exhausted 只表示本页到末尾；仅无输入游标且无后页时 list_complete_on_this_page 为真。调用次数仍受 Skill 与实际运行配置共同约束。

模型不能传入 project_id、audience、SQL、任意 URL 或文件路径。会话有项目时只发现该项目名下订阅；无项目时只发现公司名下订阅，不等于全部项目合计。费用存在项目分摊不会改变订阅所属范围。

## 3. 金额、日期与完整性

账期按订阅本地 start_date 进入窗口，不按付款日重写账期。配置与价格为读取时的当前修订；已生成账期保留自己的配置和价格修订，新价格不会自动重算旧账期。日历结合原 subscription_charge_eligible 计算应有账期，列出未生成项；草稿／不完整配置保留 coverage_known=false。all_stored_periods_in_window 只表示已存账期没有截断，不代替 coverage_complete。

费用原额、费用项目分摊、已结算／未结清和资金原额分别返回。付款可能服务多项费用；整笔退款和未分配余额属于原资金记录，只有实际 allocations 流水能归到订阅。未分配的全局退款不猜测为本账期退款。深入原记录可读取合法关联的冲销／退款族，但不自动增加本次分配。

受控算例：预测 HKD 100.000000，确认费用 HKD 100.120000，实际分配 HKD 40.000000，未结清 HKD 60.120000，账期保留 overcharge 与 invoice_missing 候选标记。另一 USD 订阅及未关联费用不混入 HKD 明细；2月缺少已生成账期，返回缺口且覆盖不完整。

## 4. 共同权限、历史依赖与事务

逐成员使用原业务投影，核对相同的附件、往来名称、关联标题和结算事实；只读快照的编辑能力标记统一为 false。不能将发起人的丰富投影直接共享给权限较少的成员。

私有来源目录不进入模型正文，保存订阅／账期／费用／资金修订、实际分配保存的原费用修订、可见固定文件版本、往来修订、业务关联和完整结算权限。回看再次核对当前 AI／渠道／分类／目录链、固定版本、原受众和实际关系。文件进入回收站或付款权限撤回时，旧的丰富快照隐藏，正文不改写。

沿用 agent_finance_report 与 append-only 表。迁移0065扩展正文版本、工具回执及观察白名单，将 view 绑定准确的成功工具、运行、会话、受众、正文及摘要。提交时延迟核对回执；回执写入失败则快照一同回滚。同一 invocation_id 重放保留原结果。

Go 控制器保留本次真实观察来源，所以完成消息同时包含主审阅、缺失账期观察和两项原记录。本轮浏览器按实际引用顺序验证四份来源及全部正文分页，不假定引用顺序或只看首段。

## 5. 运行和交付边界

subscription-review 源码1.2.0 保持 draft；六个工具是 read_resource、inspect_operation、read_finance_summary 及新增三项。Schema 新增明确订阅／日期输入，旧月度／自然语言输入兼容；其余七项包仍为 draft。TG 财务工具继续拒绝。

本增量不提供价格修改、账期生成、费用关联、异常确认、付款登记或银行支付的模型工具。受控读取具备逐笔证据，真实模型异常判断质量、正式发布／停用／回滚仍须验收。

只向精确所属开发 API 应用0065并替换原进程树；PostgreSQL、通知 Worker 和日常 Vite 不重启，日常库不写入浏览器合成业务。模型、OSS／R2、TG 关闭；Linux 仅交叉编译，不执行或部署。未使用 Docker、WSL、虚拟机、模拟器或实体设备。

## 6. 代码和复现

| 部分 | 入口 |
| --- | --- |
| 业务投影／依赖 | app/backend/internal/finance/agent_subscription_review.go、agent_finance_dependencies.go、agent_summary.go |
| 控制器与证据 | app/backend/internal/agent/finance_summary.go、execution_tools.go、execution_results.go、execution_observations.go |
| MCP／SDK | app/backend/internal/agentworker/mcp.go、bridge.go；app/agent-runtime/src/skills.ts、evidence.ts、http-boundary.ts、describe-profile.ts |
| 实际业务测试 | app/backend/internal/httpapi/agent_subscription_review_integration_test.go；app/agent-runtime/src/skills.test.ts |
| 迁移／Skill | app/backend/migrations/0065_agent_subscription_review.sql；app/skills/subscription-review/；docs/skills/subscription-review/ |
| 全量与运行时 | python app/scripts/verify_backend_batches.py --evidence-group subscription-review --batch-size 20；python app/scripts/verify_agent_runtime.py subscription-review |
| 浏览器 | python app/scripts/run_agent_browser.py --subscription-review；python app/scripts/run_mobile_browser.py --subscription-review |
| 开发与最终来源 | app/scripts/update_subscription_review_development.py、verify_subscription_review_development.py、verify_subscription_review_sources.py |

执行结果、复用证据、失败尝试及剩余范围见[验证记录](../quality/evidence/subscription-review/verification.md)。完整首发按[28项差异](../delivery/03-goal-gap-assessment.md)继续推进。
