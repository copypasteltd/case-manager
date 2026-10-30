# API 规范、目录与错误契约

增量130已实现主机CPU／内存／完整配置磁盘的原生采集桥接、签名接收、不可变记录、四端显示与配置／历史管理。真实目标安装、资源断流主动通知和独立告警送达继续归GAP-23；不以页面告警替代送达回执。[设计与使用](../development/130-host-resources.md) · [实际检查](../quality/130-host-resources-verification.md) · [全部23项](../delivery/05-functional-closure-130.md)。23 active（14 P0／9 P1）、5 excluded保持。

版本：v1.0-draft；日期：2026-10-23。机器契约见 [OpenAPI](openapi.json)，人类语义以需求/状态机为补充。所有路径在 `/v1` 下；已实现的本地接口见[实施台账](../delivery/02-implementation-ledger.md)，尚未生产部署。

## 1. 统一规则

- HTTPS JSON；除文件签名请求、受控流式下载、SSE 外使用 UTF-8 application/json。日期 `YYYY-MM-DD`；时刻 UTC ISO8601；金额与字节数为十进制字符串；UUID 不泄露顺序编号。
- 认证使用有效会话 Cookie（Web）或 Bearer（移动端）；工具网关另用限定任务凭据。actor/org/roles/执行权限由服务器注入，body 不接受替代身份。Web Cookie 写操作必须 CSRF token+Origin 校验。
- 成功：`{data: ..., meta: {request_id, server_time}}`；列表 data 为 `{items,next_cursor}`，cursor 不透明且绑定筛选/稳定排序。默认20最大100，未授权资源不参与统计。
- 错误：`{error:{code,message,details,retryable},meta:{request_id,server_time}}`；details 仅安全字段/阻塞原因。错误不暴露 SQL/签名/堆栈。前端按 code 处理，不解析中文 message。
- 所有业务写请求带 Idempotency-Key，更新/动作带 expected_version；纯认证 bootstrap、webhook 使用各自重放机制。key 作用域包含组织、actor/设备或服务、route；请求 hash 包含规范化 body 与路径/方法。
- 同键同参数返回原结果；同键异参数 409 IDEMPOTENCY_MISMATCH；仍执行中 409 OPERATION_IN_PROGRESS+安全查询引用。短期幂等保留≥7d，审批/费用/账期永久唯一键另存。
- 服务端拒绝未知写字段，禁止 mass assignment。read-only 字段如 status/current_version/paid_amount 不能通过通用 PATCH 绕过动作接口。PATCH 明确 allowlist；缺字段不变，null 仅对声明可空字段有效。
- 异步受理202+job/run/session ID，查询200；创建201；动作完成200；不把 HTTP200 当作底层所有异步步骤成功。

短时签名使用持久化的无秘密许可，绑定同一个upload_session/version/part，不新建业务文件。当前实现对同键同参数重放先重新鉴权及核对阶段，再生成新的短期URL；URL及签名头不进入幂等结果、审计或事件，因此临时URL不保证逐字节相同。过期、终止或进入完成阶段不返回新许可。该凭证签发例外见[上传控制](11-upload-control-and-freezing.md)，正式commit仍重放同一个业务结果。

## 2. HTTP 与业务错误

| HTTP | code 示例 | 客户端动作 |
| --- | --- | --- |
| 400 | INVALID_ARGUMENT, UNKNOWN_FIELD, INVALID_CURSOR | 标示字段，修正后再提交 |
| 401 | UNAUTHENTICATED, SESSION_REVOKED | 有效刷新只尝试一次；仍失败重新登录 |
| 403/404 | FORBIDDEN / RESOURCE_NOT_FOUND | 不泄露目标；无权对象按策略返回404 |
| 409 | VERSION_CONFLICT, INVALID_TRANSITION, BASE_VERSION_CHANGED, DUPLICATE_SOURCE, IDEMPOTENCY_MISMATCH | 拉取最新安全信息；禁止盲重放新版本 |
| 412 | PLAN_EXPIRED, AUTHORIZATION_CHANGED, STORAGE_EPOCH_CHANGED | 重新授权/计划/会话，旧批准失效 |
| 413 | SIZE_LIMIT_EXCEEDED | 展示该端上限；不把120GB导向普通表单上传 |
| 422 | REQUIRED_BUSINESS_FIELD, WORKFLOW_NOT_CONFIGURED, FX_RATE_MISSING, CLOSE_BLOCKED | 展示可处理阻塞；不填默认假值 |
| 429 | RATE_LIMITED, BUDGET_EXCEEDED | Retry-After/人工预算处理；禁止循环重试 |
| 503 | DEPENDENCY_UNAVAILABLE, STORAGE_UNAVAILABLE | 查询已提交结果再重试，基础模块尽量可用 |

幂等命中原结果的新请求仍产生新 request_id 及关联原操作 ID。传输超时不能推定未提交；提供 `/operations/{operation_id}` 或资源查询核实。

## 3. 路径目录

下列 `{id}` 为资源 UUID，`{step_id}` 等按对应资源命名；资源 DTO 字段由数据字典和 OpenAPI 共同定义。GET 列表支持 cursor/limit，模块特定过滤仅白名单。

| 组 | 路径与方法 | 写入语义/权限 |
| --- | --- | --- |
| 会话 | POST `/auth/login`、`/auth/refresh`、`/auth/logout`；GET `/me`、`/capabilities` | login 可返回需 MFA challenge；不接受 org 角色；停用立即失效 |
| 账号 | GET/POST `/users`；GET/PATCH `/users/{id}`；POST `.../disable`、`.../role-bindings` | POST users 是邀请；角色/停用专门动作，管理员二次认证 |
| 模板 | GET/POST `/workflow-definitions`；POST `/{id}/publish`；GET `/project-templates` | 发布不可覆盖；无规则不提交 |
| 项目 | GET/POST `/projects`；GET/PATCH `/projects/{id}`；POST `.../transition`；GET/POST `.../members` | transition校验关闭阻塞；成员单独权限 |
| 任务 | GET/POST `/projects/{id}/tasks`；GET/PATCH `/tasks/{id}`；POST `.../transition`、`.../dependencies` | 防环/父子约束；删除依赖为受版本保护命令 |
| 里程碑/交付 | GET/POST `/projects/{id}/milestones`、`.../deliverables`；GET/PATCH `/deliverables/{id}`；GET `.../revisions`、`.../revisions/{revision}`；POST `.../submit`、`.../decisions` | 准确版本／修订／指纹、完整固定文件、真实交互及独立近期MFA验收；写命令返回纯收据，详情另查 |
| 风险/问题/变更 | GET/POST `/projects/{id}/risks`、`.../issues`、`.../changes`；PATCH `/risks/{id}`、`/issues/{id}`；POST `/changes/{id}/submit`、`.../implement` | 变更送审走通用流程，实施需 base_project_version |
| 知识 | GET/POST `/posts`；GET/PATCH `/posts/{id}`；GET `.../revisions`；POST `.../publish` | 正文修订/乐观锁；内部 publish |
| 目录/文件 | GET/POST `/folders`；POST `/folders/{id}/move`；GET `/files/{id}`、`.../versions`；POST `.../move`、`.../trash`、`.../restore` | move 校验目标范围与分类；restore恢复回收站；历史内容使用独立history-restores任务及显式commit生成新版本 |
| 上传 | POST `/uploads`；GET `/uploads/{id}`；POST `.../sign-parts`、`.../ack-parts`、`.../finalize`、`.../commit`、`.../abort` | draft短签；finalize202；commit201；无直接正式对象 PUT |
| 下载/检索 | POST `/file-versions/{id}/download`；GET `/downloads/{id}/content`、`/downloads/{id}/chunks`、`/search` | 固定源短签／鉴权范围；移动JSON段最多1MiB且必填摘要条件头，见[固定内容协议](46-mobile-fixed-content-reading.md)；搜索含ACL，禁止任意 object_key |
| 申请/审批 | GET/POST `/requests`；GET/PATCH `/requests/{id}`；POST `.../submit`、`.../withdraw`；GET `/approval-tasks`；POST `/approval-steps/{id}/decisions` | 修订与内容摘要；分配人当前步骤；不让模型 approve |
| 合同 | GET/POST `/contracts`；GET/PATCH `/contracts/{id}`；POST `.../transition`、`.../signature`、`.../milestones` | 关键变更新修订/审批；签署登记固定批准修订和文件版本 |
| 采购履行 | GET/POST `/purchases`；GET/PATCH `/purchases/{id}`；GET `.../fulfillment`、`.../orders`、`.../receipts`、`.../execution-events`；POST `.../orders`、`.../receipts`、`.../orders/{order}/cancel`、`.../cancel` | 部分验收／准确来源退货；订单价格与当时执行依据固定，批准与执行分离 |
| 采购差异 | GET `.../execution-plan`；GET/POST `/purchases/{id}/variances`；GET/PATCH `/purchase-variances/{id}`；GET `.../revisions`及单修订；POST `.../activate` | 完整独立审批后仍须明确人工启用；原商业批准及旧订单保留 |
| 采购完成 | GET `/purchases/{id}/completion-preview`、`.../completion`；POST `.../completion` | 固定完整交付／差异／实际结算集合及指纹，人工声明／近期MFA；历史与当下变化分别返回 |
| 报销 | GET/POST `/claims`；GET/PATCH `/claims/{id}` | 字段/明细修订；提交关联 requests；不直接 PATCH 已付款；GET `/claims/{id}/settlement`推导准确垫付结算 |
| 订阅 | GET/POST `/subscriptions`；GET/PATCH `/subscriptions/{id}`；POST `.../prices`、`.../transition`；GET `.../revisions`、`.../revisions/{revision}`、`.../events`、`.../reminders`；GET `/subscriptions/owners` | 固定配置/价格；内部停订与供应方核实分开；提醒只证明排队 |
| 账期/预测 | GET `/subscription-periods`、`/{id}`、`.../revisions`、`.../events`；GET `/subscriptions/{id}/periods`；POST `.../periods/generate`、`.../periods/unexpected`；POST `/subscription-periods/{id}/reforecast`、`.../costs`、`.../review`；GET `/reports/forecast` | 账期自然唯一；人工固定版本及实际费用集合指纹；预测钻取 `forecast_only=true`；后台只用真实任务租约 |
| 费用 | GET/POST `/costs`；GET `/costs/{id}`；POST `.../confirm`、`.../reverse`、`.../links` | 永久 source_key；confirmed 不直接改金额 |
| 付款 | GET/POST `/payments`；GET/PATCH `/payments/{id}`；POST `.../record`、`.../reverse`、`.../allocations`、`.../allocations/{allocation}/release`、`.../refund-allocations`；GET明细/修订/事件 | 人工资金事实；准确费用/垫付明细；并发锁余额，不发起银行动作 |
| 报表 | GET `/reports/expenses?kind=incurred`、`/reports/cash`；POST `/reports/exports`仍为规划 | 费用发生与现金独立原币统计；预测/摊销/导出继续接入 |
| 会话/任务 | GET/POST `/conversations`；GET `.../{id}/messages`；POST `.../{id}/runs`；GET `/runs/{id}`、`.../events`；POST `.../cancel` | runs包含消息client_message_id去重；SSE带事件序号 |
| 固定计划 | GET `/plans/{id}`；POST `.../approve`、`.../commit` | 绑定 hash+expected_version+授权；工具commit二次检查 |
| Skill | GET `/skills`；POST `/skills/{id}/releases`、`.../disable-writes` | 不在普通文件管理修改发布包 |
| TG | POST `/integrations/telegram/bindings`；POST `/webhooks/telegram/{bot_ref}` | 绑定需登录；webhook无用户Bearer，按secret+update_id验证 |
| 状态 | GET/POST `/services`；GET/PATCH `/services/{id}`；GET `.../observations`；GET/POST `/incidents`；PATCH `/incidents/{id}`；POST `/maintenance-windows` | 目标allowlist；readonly探测不得变SSH |
| 通知/管理 | GET `/notifications`；POST `/notifications/{id}/read`；GET `/audit-events`、`/jobs/{id}`、`/operations/{id}`、`/storage/replicas`、`/backups` | 每个对象权限；审计与健康接口不泄露配置 |

接口目录包含首发服务面；OpenAPI 第一版覆盖核心高风险链及主要资源 DTO，其 `x-contract-scope` 逐项列明机器契约边界。目录中未进入机器契约的配置/细分管理接口，在该工作包开始编码前必须补齐精确 DTO 与 OpenAPI，不能按自由 JSON 临时实现。工具网关不直接开放给浏览器，另见工具契约。

## 4. 关键请求范例

```json
{"title":"采购设计服务","kind":"purchase","business_id":"2f20766f-c0cc-4f52-b9da-92a214b2111c","project_id":"a499c6d8-f68a-493d-a3f6-0a0d508ccf49"}
```

提交申请请求：`{expected_version:3, revision_id:"UUID", content_hash:"64hex"}`；审批决定：`{expected_version:2, request_version:4, decision:"approve|return|reject", reason:"..."}`。身份不在 body；审批人从会话与 step 取得。

上传创建：`{folder_id,name,size_bytes:"120000000000",media_type,file_id?,base_version_id?,source_fingerprint}`；最终提交：`{expected_version,base_version_id?,sha256}`。新文件不得带无意义 base_version，旧文件更新必须带 base_version。返回正式 file/version、operation_id 与副本状态。

费用创建：`{source_key,title,project_id,classification,occurred_on,currency,amount:"12000.00",category,counterparty:{party_id,revision_id},allocations:[{project_id,amount:"12000.00"}],evidence_version_ids,notes,reason}`。日期/币种/金额/供应方允许null，草稿数组仍须明确提供。确认、冲销和关联固定准确费用版本/修订/摘要并检查当前人工MFA；采购/报销/生效应付合同共同关联同一费用，不另增金额。当前汇率未配置，base_amount为null，各币种分别统计。实际接口及分摊筛选语义见[唯一实际费用](35-canonical-actual-costs-and-incurred-reports.md)。

资金创建、正式登记、分配、释放、退款与冲销的准确DTO、状态和余额规则见[资金与结算协议](36-payments-refunds-and-settlement.md)。合同节点使用独立财务头版本，归属、解除、人工核对与只含标识的幂等回执见[合同财务协议](38-contract-financial-reconciliation.md)。应收退回的资金预留反映在 `contract_return_reserved_amount`，不能再次用于费用结算。

采购差异的完整前后依据与明确启用见[执行基线协议](40-purchase-variance-approval-and-execution-baselines.md)；采购完成的完整性、人工核实、准确集合指纹、来源退货和固定历史见[完成核对协议](41-purchase-completion-and-settlement-review.md)。采购完成回执仅含标识与版本，读取历史和重放仍先检查当前授权。

当前同源契约为221路径、282操作、456 Schema，覆盖项目交付的列表／创建、详情／编辑、准确提交／决定及修订列表／详情8项HTTP操作；仍包含尚未实现的规划路由，不表示全部平台接口已落地。机器可执行定义以同源`openapi.json`为准，目录缩写不能被实现为未定义的通用状态跳转接口。

## 5. SSE 与版本演进

SSE `id` 使用 run 内单调 seq，event 为 run.accepted/progress/tool.completed/approval.required/run.completed/run.failed；data 仅公共安全事件 envelope。客户端通过 Last-Event-ID 或 after_seq 恢复，重复 seq 丢弃；事件窗口过期返回需重新读取 run/messages，而不是重新执行 run。

兼容性变更新增可选字段/枚举需客户端容错；未知业务状态只读显示 unknown 并禁用不确定动作，不自动映射 approved。删除字段、改金额语义、身份逻辑、旧状态转移属于破坏性变更，需要版本/兼容期和平台包发布时间协调。OpenAPI diff 是评审材料，不能只改生成客户端不改服务端。
