# 秘书准确业务申请起草、修订与送审

本增量推进 GAP-11／12／13／18。功能代码、共享契约、运行时与验证脚本均在 `app/`，本文与验收记录在 `docs/`。范围为合同、采购、报销的准确申请操作，完整平台 Goal 仍按 [28项差异表](../delivery/03-goal-gap-assessment.md)推进。本次不新增公司审批政策；测试规则和资料仅用于隔离验证。

## 1. 已实现流程

| 编号 | 员工操作 | 秘书实际行为 | 结果边界 |
| --- | --- | --- | --- |
| OA-A01 | 在本人公司／项目私密会话选择已保存业务及“起草或修订申请”，填写说明后明确发送 | `draft_request` 复用原业务命令，按准确业务修订创建本人申请，或修订关联的本人草稿／退回／撤回申请 | 保存申请草稿，尚未送审；不改写原业务正文 |
| OA-A02 | 选择“核对预览后送审”，阅读完整固定内容、已发布审批规则和审批人，勾选确认后发送 | `submit_request` 使用准确申请版本、修订、摘要、规则ID与预览摘要调用真实送审流程 | 创建真实审批实例和待办，状态为 `in_review`；审批、签署、生效、下单、验收、记账、付款分别处理 |
| OA-A03 | 查阅秘书任务结果、消息或事件进度 | 返回已持久化的授权和实际业务操作回执；可进入申请详情核对 | “秘书本次事项完成”不等于“业务获批” |
| OA-A04 | 重试原请求或取消任务 | 原键／原参数返回同一结果；取消前没有写入则不产生申请，取消后保留已确认的真实提交 | 不重复创建申请或审批实例，不声称撤销已提交事实 |

每个运行只允许一个准确写入意图。`oa_request` 与项目／任务更新、文件更新、新建意图互斥。仅四种真实交互登录渠道 `web / android / wechat / douyin`；没有 TG 代送审入口。

## 2. 版本与参数约束

`POST /v1/conversations/{id}/runs` 的 `oa_request` 为工具及参数的分支结构，公共API继续采用 [OpenAPI](openapi.json)，没有添加允许模型任意审批的接口。

| 工具 | 必填准确参数 | 可选参数及含义 |
| --- | --- | --- |
| `draft_request` | `kind`、`business_id`、`expected_business_version`、`business_revision_id`、`business_content_hash`、`reason` | `request_id` 与 `expected_version` 必须同时存在或同时省略；存在时仅修订该申请 |
| `submit_request` | `request_id`、`expected_version`、`revision_id`、`content_hash`、`workflow_definition_id`、`preview_hash`、`reason` | 不提供选择其他审批人、改变规则或跳过校验的参数 |

UUID 使用准确小写标识；摘要为64位小写十六进制；说明1至200字；参数上限64KiB；冻结授权上限512KiB。工具参数必须逐项等于本次员工发送的规范化参数。版本、内容、规则或预览变化后停止，要求员工重新核对并发送，不能自动换用最新修订。

## 3. 授权与读取

实际交互会话在受理时固定到授权意图，模型不能传入或看到登录会话标识。所有后续工具、重复调用、执行签发及完成回执重新核对原始会话仍有效，组织／用户授权版本、运行租约、取消、截止时间和会话安全版本仍有效。撤销登录会话或权限后不能继续本次执行；当前仍有权的历史业务事实可重新登录查阅。

业务执行保留 AI 身份：原合同／采购／报销、项目可写状态、申请创建／更新／提交权限与 AI 分级策略仍然生效。只对原始登录证明做窄范围的人工身份核对，不能通过清空运行标识绕过实际业务权限。送审仅接受绑定到本次运行、本人及准确申请的授权。

业务与会话项目必须一致，公司会话只能处理公司范围业务。业务分类不得超过会话上限。底层交易对手与固定文件版本从原始内容逐项校验，包括普通页面可能隐藏的失效引用；附件的ID、摘要和字节数必须与冻结依据一致。当前成员和AI没有读取权时，不能把底层引用交给模型。

新增 `agent_oa_request` 来源固定本次授权摘要。`read_resource` 分段读取公开授权及真实结果，`inspect_operation` 核对本运行的提交情况；正文仍作为不可信数据。读取历史授权不能变成新一次写入授权。

## 4. 数据与事务

迁移 [0056](../../app/backend/migrations/0056_agent_oa_requests.sql) 增加两张仅追加表：

| 表 | 内容 | 强制证据 |
| --- | --- | --- |
| `agent_oa_intents` | 运行、原始用户／会话、原始登录会话、工具、业务／申请、准确参数、操作前依据及意图摘要 | 真实员工消息与来源摘要、当前业务版本、本人／项目／分类与原始会话 |
| `agent_oa_commits` | 同一运行的真实操作ID、申请ID及结果 | 真实幂等命令结果、实际申请状态／版本、审批实例／步骤及同事务成功工具回执 |

六项触发器限制不可改写、准确绑定与延迟证据检查；应用角色只有 SELECT／INSERT，不能 UPDATE／DELETE。既有工具回执与观察枚举加入两种工具，不扩展任意命令执行权限。

业务申请、关联业务状态、申请修订、审批实例／步骤、审计、Outbox、幂等结果、OA提交与工具回执在同一数据库事务中保存。回执或任意证据失败必须整体回滚。内部路由为 `/internal/agent-oa/{tool}`，固定键 `agent-oa:{run_id}`。重复调用不能改说明、对象、版本或政策。

## 5. 前端与运行时

Web 和 Taro 公共页面均提供三类业务选择、准确版本摘要、已关联申请状态、送审完整内容／规则／审批人预览、显式确认、移除授权和实际执行记录。选择与预览不会发送任务；确认发送才构成一次授权。单独显示申请草稿和等待员工审批的送审结果，复核后可进入申请详情。执行记录保留提交时状态，当前审批进度以申请详情为准。

`oa-assist` 源码包为1.1.0，仍为 draft。MCP、原生约束证明工具白名单和 HTTP 运行边界均加入 `draft_request / submit_request`。完成判断必须引用本运行实际持久化的成功业务操作；模型生成的申请号、自然语言或 inspect 输出均不能代替提交证据。详见 [技能SOP](../skills/oa-assist/SKILL.md)。

## 6. 验证与复核

[验证记录](../quality/evidence/secretary-oa-requests/verification.md)列明完整回归、两端构建、运行时、浏览器、来源、开发服务和清理证据。后端使用原生 PostgreSQL 与真实认证HTTP／MCP，只控制原生约束证明和隔离文件依赖，不调用真实模型或云存储。

可复核命令：

```powershell
$env:GOFLAGS='-p=1'
$env:GOMAXPROCS='2'
$env:PYTHONUTF8='1'
python app/scripts/test_backend.py --package ./internal/httpapi -run '^TestAgentOARequests$' -timeout 8m
python app/scripts/verify_backend_batches.py --evidence-group secretary-oa-requests --batch-size 20
python app/scripts/verify_web.py secretary-oa-requests
python app/scripts/verify_mobile.py secretary-oa-requests
python app/scripts/verify_agent_runtime.py secretary-oa-requests
python app/scripts/run_agent_browser.py --oa-requests
python app/scripts/run_mobile_browser.py --oa-requests
python app/scripts/verify_secretary_oa_requests_development.py
python app/scripts/verify_secretary_oa_requests_sources.py
```

浏览器验证拥有独立隔离库、浏览器上下文和临时服务，完成及失败均关闭自身资源，不接管用户浏览器。Web 与 Taro H5 测试按顺序运行，避免共享临时夹具冲突；文档中的并行排期不授权子Agent。遵守不调用 Docker、WSL、虚拟机或模拟器的限制。

## 7. 继续开放的边界

本版使用人工已保存的业务资料，未实现秘书自动编写合同／采购／报销正文、自动整理未知凭证、知识写入、采购差异／项目变更、订阅／状态等其余业务工具。公司实际审批人、金额阈值、币种口径和制度仍须确认。八项 Skill 的真实发布、实际模型与原生约束验收、TG、双云、120GB／2TB、微信／抖音真机、Android与生产部署恢复均继续列为未完成，不能由本地受控通过代替。
