# Agent 适配器、工具网关与 Skill 发布契约

版本：v1.0-draft；日期：2026-10-23；实现 AI-01—20。本文定义公司协议，不假定 Codex SDK 原生拥有同名工具/事件。

当前八项草稿包、加载校验、结果核验和SDK事件适配库已落地，实际能力与证据见[技能包与运行时实施](27-agent-skill-packages-and-runtime-adapter.md)。本文后续描述仍是完整目标；工具网关、真实运行与发布不能从说明性示例推导为已接通。

目录整理的实际工具、MFA批准、批量原子命令、不可变目录观察及当前Web／Taro入口见[增量55](55-secretary-directory-organization.md)。`information-organize`已接通有限盘点、同范围新建目录、重命名与移动；跨范围、标签、生命周期归档等完整目标未由该增量覆盖，八项Skill仍为draft。

指定目录与名称的文本／Markdown新建见[增量56](56-secretary-file-creation.md)：`stage_file_create`、`propose_file_create`、`commit_file_create`复用原扫描／首版本业务提交与凭证链路；本人确认发送准确请求构成单文件L1授权，不引入另一轮MFA批准。`file-update`源码包1.1.0，运行配置准确工具集合增至8项；历史更新、目录整理的批准语义及八项draft状态保持各自约束。

合同／采购／报销的已保存业务修订申请与送审见[增量57](57-secretary-oa-requests.md)。`oa-assist`源码包1.1.0，四项准确工具为read_resource、inspect_operation、draft_request、submit_request；明确发送构成一个准确L1操作，送审同时绑定真实预览和原始登录会话。模型不接收会话ID，不代审批。正文自动起草和更多业务工具仍属完整目标，八项manifest仍draft。

知识写入实际能力见[增量58](58-secretary-knowledge-writes.md)：project-briefing源码1.1.0／L1，五项工具含draft_post／publish_post。草稿生成内容受准确目标约束，发布需员工另行核对完整修订；所有读取来源需与目标同项目且不高于目标分类。普通无知识写入意图的简报仍按只读完成规则处理。八个manifest仍draft，真实模型和发布尚未验收。

项目里程碑与阶段的基础能力见[增量59](59-secretary-project-stages.md)：当时project-update源码1.1.0含read_resource、inspect_operation及六项准确写操作。后续[增量72](72-secretary-milestone-creation-and-task-association.md)补创建与任务关联，[增量73](73-secretary-deliverable-updates.md)补完整交付资料与关联，[增量74](74-secretary-deliverable-creation.md)补创建交付草稿。当前project-update源码1.4.0含九项准确写操作，仍逐次重查原始交互登录，完成须有当前运行的真实业务回执；批量、跨项目及其余业务工具继续开放，archive由原人工结项复盘处理，八项manifest继续draft。

增量60补上[Telegram本人绑定与入站接收](60-telegram-binding-and-ingress.md)：准确群／话题、允许Skill、本人数字身份及入口版本随请求固定；当前不创建agent_run、不调用工具、不外发结果。未来渠道桥接必须再次检查当前绑定／用户授权／入口版本和Skill，群请求不能继承Web个人会话的业务写入授权，结果披露及不确定送达须另行实现。

## 1. Runtime adapter

统一内部接口 `start_run / send_input / stream_events / request_cancel / inspect_run / rebuild_context`；原生续接能力作为可选 capability。公司 run_id 与 native_thread_id 分开保存，业务结果与幂等不依赖原生线程存在。

Runtime 首选候选 Codex SDK 有界任务，备选 codex exec；app-server 在所核实版本有实验性限制，只作为可替换适配器。固定包版本/事件映射/认证方式，内网 stdio 或受控通道，不暴露原生服务公网端口。产品/地区资格未确认不启用真实生产调用。

运行在独立 OS 身份，业务工具通过 Go 网关调用。凭证与模型可执行子进程环境隔离；网络/目录/资源配额由原生系统控制。若 Runtime 的默认 shell/网络/任意文件工具无法有效限制，不能拿 Skill 文本约束替代技术边界，不放行正式写入。

## 2. 工具请求 envelope

内部工具路由候选 `POST /internal/v1/tools/{tool_name}`，仅 Runtime 服务可达。

```json
{
  "run_id":"31c104ff-f29f-4080-9e16-ae02529b04a1",
  "invocation_key":"file-update:step-03",
  "arguments":{
    "file_id":"ad6e8563-9086-4db6-8a4e-6be474935d07",
    "base_version_id":"7b50d68b-76d7-4080-b088-eb99f18583f6",
    "draft_id":"80b96326-d6c3-442c-82e7-12184a8027ae"
  }
}
```

run_id 仍必须与连接上的短时 task credential 绑定，不能信任 JSON 声称的 run。凭据包含 aud、jti、org/run/actor 引用、scope_ref、auth_version、expires_at；不把其完整明文回传模型。业务参数 Schema 禁止 actor_id/roles/任意 object_key/SQL/URL。

返回统一 safe result：`status(succeeded|pending|conflict|denied|failed), resource_refs, operation_id, committed, safe_summary, retryable, next_action`。模型只在 committed=true 或相应已确认读取结果时报告正式完成；pending 返回 job 引用，不持续无意义轮询消耗模型。

## 3. 首发工具目录

| 工具 | 参数要点 | 确定性前置与结果 |
| --- | --- | --- |
| search_resources | query、resource_types、project_ids、cursor、limit | ACL/AI许可先过滤；来源版本/安全片段 |
| list_directory | folder_id、cursor | 仅授权子项；不返回其他人的总数 |
| read_resource | type、id、version_id?、bounded_range? | AI许可/渠道；字节/字符上限；不接受任意URL |
| propose_project | 项目字段/模板 | 校验缺项/授权/重名；返回固定plan/预览 |
| create_project | plan_id/hash、expected_version | 既有授权或有效grant；同业务用例；返回初始化证据 |
| update_project / transition_task | target_id、expected_version、允许字段/动作 | 不绕过依赖/验收/归档 |
| create_file_draft | file/folder、base_version、format、有限内容 | 控制格式/大小，生成draft，不接收任意本地路径 |
| propose_file_update | draft_id、base_version、摘要 | 计算真实diff/hash，不能信模型自报摘要 |
| commit_file_update | plan_id/hash、draft_id、base_version | 重新授权、草稿检验、版本/纪元；返回正式版本 |
| propose_organization | scope、规则、limit | 返回逐项原/目标/权限影响；不降低分类 |
| commit_organization | plan_id/hash、expected_version | L2授权，逐项幂等；结果列表/补偿引用 |
| draft_post / publish_post | project、正文/refs、base_revision | 内部发布权限；引用仍校验 |
| draft_request / submit_request | 当前：准确已保存业务／申请版本、摘要及送审规则／预览；完整目标另含正文及附件候选整理 | 正式本人申请／真实待审批实例；逐项匹配本次意图，不提供 decide_approval 给模型 |
| read_finance_summary | period、scope、kind、currency | 口径明确、未确认单列；没有记账权限不写费用 |
| read_service_summary | service_ids、time_window | 只读摘要，明确观测时间/unknown |
| propose_notification | 明确接收范围、正文、refs | 固定正文/披露级别/收件人，生成计划 |
| send_authorized_notification | plan_id/hash | 既有明确发送授权或已批准grant；outbox，不能任意群聊 |
| inspect_operation / cancel_run | operation/run_id | 同用户/范围；取消不谎称回滚已提交 |

每个工具发布前补齐 JSON Schema、权限动作、最大输入/输出、超时、幂等键、错误、审计字段及 TC。工具集合本身与业务角色分别授权，不能因为允许工具名就给访问所有资源的权限。

## 4. Skill manifest

```yaml
skill_id: file-update
version: 1.0.0
state: draft
owner_role: business_admin
runtime_compatibility: poc-pinned-version
allowed_tools:
  - read_resource
  - create_file_draft
  - propose_file_update
  - commit_file_update
allowed_formats: [text/plain, text/markdown]
authorization_level: L1
input_schema_ref: company.file-update.input.v1
output_schema_ref: company.file-update.output.v1
max_tool_calls: 20
timeout_seconds: 900
test_case_refs: [TC-AI-01, TC-AI-02, TC-AI-03]
```

这是说明用 manifest，非已安装 Skill。真实包包括 SKILL.md、manifest、输入输出 Schema、模板/脚本（若有）、测试集和产物摘要；owner_role 不是执行身份。费用上限由组织策略与每次任务的较小值决定，不能由上传包自行提高。

## 5. 各 Skill 执行步骤

- project-create：收集必填→读取模板/可分配成员→固定计划→匹配授权→事务创建→检查异步初始化→返回完成与待补项。
- project-update：读指定版本→提出明确字段/状态动作→依赖/验收校验→提交→返回原/新事实，不把推测改状态。
- file-update：读授权版本→本地受限草稿/工具内容→格式校验/真实差异→固定hash→提交→展示版本及复制中，不等待大文件进模型。
- information-organize：限定盘点→分类/重复候选→预演权限变化→计划授权→逐项操作→部分失败和可恢复依据；不做硬删。
- oa-assist：提取原件候选→必填/金额校验→展示待核实→保存草稿→明确提交授权后提交→查询真实审批状态。
- subscription-review：取账期/价格/实际→区分预测/已确认→重复/缺票/取消后扣费候选→草稿报告→按明确授权通知。
- project-briefing、incident-summary：按窗口取有效来源→标注缺失/过时→生成引用摘要→校验渠道→保存/获授权投递。

缺字段只询问影响当前动作的最少信息；读取、整理草稿和独立工作可继续。真实歧义涉及金额、审批、对外披露、跨项目移动不能自造答案。

## 6. 测试、发布与回滚

发布链 draft→tested→pilot→published→disabled，已发布包按 hash 不可变。评审输入输出/旁路权限/注入/格式与金额/取消/重复调用/预算；试点只用获授权数据。普通文档上传不进入此发布链。

发布清单保存 Runtime/模型端点配置版本、Skill版本、工具版本、评测集版本及结果。支持独立 disable-writes、回滚至已验版本、撤销未提交计划；不能把回滚 Skill 当作回滚已提交业务。失败先给业务可追踪结果，再决定是否重启模型。


## 常规更新实现边界（增量50）

[常规项目与任务更新](50-secretary-routine-updates.md)实现本次发送绑定的 L1 准确范围，提供 `update_project`、`update_task`、`transition_task`。模型必须逐项匹配服务端 `authorized_mutation`，复用业务权限与状态校验；正常更新不追加批准。更新页面按明确字段／动作选择，接口可接受同一对象的明确字段集合。里程碑资料／状态及项目阶段已按[增量59](59-secretary-project-stages.md)扩展；[增量72](72-secretary-milestone-creation-and-task-association.md)补齐完整里程碑创建表单和任务关联／解除入口。交付关联编辑、批量及跨项目仍属未完成范围。实际源包仍为 draft，本地受控证明不代替原生／模型或生产验收。

## 指定文件正文实现边界（增量51）

[指定文件版本正文读取](51-secretary-file-text.md)扩展现有 `read_resource` 和会话资料预览，支持 1 字节至 1 MiB 的 UTF-8 文本／Markdown／CSV／JSON。要求准确版本、扫描通过、已验证正式副本、显式专用身份及共同 AI 许可，完整长度／摘要核验后才按字符输出；网络前后分别检查授权与来源，不持数据库锁等待对象读取。Web／Taro 支持同一文件两个版本分别引用。旧原文件名缺失明确拒绝，旧创建人未知按 NULL 授权处理。当前仍无文件草稿、diff 或新版本提交工具，`file-update` 包继续为 draft。

## TG 入站运行约束（增量61）

TG项目简报由 `platform worker-telegram` 消费已绑定身份和准确入口的持久请求，在一个事务内创建来源关联、独立会话、员工消息、queued运行和现有工作队列。普通门户不能创建TG类型或向其发送运行。上下文按入口版本、绑定及员工／组织授权版本隔离，执行时再次检查实时权限、撤销、租约和来源。

同一不可变项目简报配置新增 `telegram_configuration_sha256`；TG原生SDK配置、证明与MCP列表仅有 `read_resource`／`search_resources`／`inspect_operation`，Go另行拒绝写入和迟到结果。普通Web配置不能代替TG专用校验。`describe-profile`可输出两套待检查配置，但不发布或安装任何Skill。`incident-summary`目前没有完整TG工具链，保留请求而不创建运行。详见[完整实现契约](61-telegram-agent-dispatch.md)；真实模型及外发仍开放。


## 增量62：事件摘要工具

`incident-summary` 源码升至 1.1.0，保持 draft。Go／MCP／SDK 对 Web 与 TG 提供 `read_resource`、`search_resources`、`inspect_operation`、`read_service_summary` 四项只读工具，补上增量61时未接通的事件任务。快照绑定原会话受众，逐项核对跨服务事件，保留 captured_at／时间窗／截断信息；不会把历史证据当作实时状态。数据库由迁移0061保障不可改写和真实回执绑定。详见[协议及上限](62-agent-service-summary.md)。

## 增量63：月度财务只读工具

`read_finance_summary(month,view,currency?)` 复用既有三类财务报表，范围由会话固定。新增 `agent_finance_report` 固定来源、私有授权目录、原受众及当前权限重查；Go／SDK 完成校验要求引用本次成功财务读取来源。subscription-review 1.1.0 保持 draft，允许 read_resource、read_finance_summary、inspect_operation，未开放 TG。完整约束见[增量63](63-agent-finance-summary.md)。


## 增量64：TG 通知不是模型工具

受理／进度／结果的群通知由独立 worker 从当前持久状态派生，模型不能指定 Bot Token、外部 URL、群号、话题、回复目标或通知正文。入口只允许 portal_only，回执与人工对账不改变秘书输出来源的当前权限。既有 TG Skill 范围不因通知实现扩大。详见[群通知链路](64-telegram-delivery.md)。

## 增量71：订阅逐笔只读工具

subscription-review 1.2.0 增加 list_subscriptions、read_subscription_review、read_finance_record，连同原有三项共六个只读工具，仍为 draft。发现页游标绑定会话与筛选；明细使用准确订阅／日期及实际关联，超限拒绝不返回前缀。逐成员普通投影与私有依赖共同保护当前事实及历史回看；迁移0065绑定 view 和准确成功回执。Go／SDK 排除仅发现列表的完成声明，TG 财务工具继续拒绝。见[协议、金额及上限](71-secretary-subscription-detail-review.md)。
## 工具实现补充：创建里程碑（增量72）

新增 `create_milestone`：`id`／`expected_version` 固定父项目，`title`／`owner_id`／`due_at`／`reason` 完整匹配人工授权。原创建命令决定新 ID，迁移0066将版本1新结果和原业务回执绑定，旧更新仍要求基础版本加一。Go、MCP、SDK及原生隔离证明共用新增工具白名单；`project-update` 1.2.0包含7项准确写工具、`read_resource`和`inspect_operation`，继续保持 draft。

后续读取仅开放本次实际创建的新里程碑；不能据创建权限枚举其他里程碑。任务关联使用既有 `update_task`，Web／Taro新增真实同项目分页候选和显式清空。创建与关联分别授权，不能通过一次创建自动批量修改任务。详细实现与验收见[增量72](72-secretary-milestone-creation-and-task-association.md)。

## 工具实现补充：完整交付资料（增量73）

`update_deliverable` 严格接收七项参数：准确 `id`、交付 `expected_version`、`name`、`acceptance_criteria`、`owner_id`、必须存在但可为null的 `milestone_id` 和 `reason`。资料更新使用原 `UpdateDeliverable`；只允许协议1草稿或退回，沿用原 `deliverable.create` 权限与成员、关联、项目阶段、共同AI受众及原登录约束。没有交付状态、正文修订、文件或审批参数。

工具结果为原 `DeliverableReceipt` 的 `id`／`version`／`revision_id`；Go及SDK完成校验使用真实操作，数据库另核对实际完整资料与原固定修订，不虚构完整对象回执。内部 `read_resource` 的交付读取仅限本次准确授权并使用不可变 `agent_mutation` 来源。迁移0067增加范围和回执约束，追加0068将名称／准则首尾Unicode空白处理与Go统一，未改写已应用迁移。详见[完整协议](73-secretary-deliverable-updates.md)。

## 工具实现补充：创建交付草稿（增量74）

`create_deliverable` 严格接收父项目 `id`／`expected_version`、`name`、`acceptance_criteria`、`owner_id`、明确 `milestone_id` 或 `null` 及 `reason`，完整匹配本次人工授权。原 `CreateDeliverable` 创建协议1草稿，回执仅为新 `id`、`version:1`、`revision_id:null`；父项目基础版本不递增，后续更新须另用新交付版本。执行仍检查原业务权限、项目阶段、实际成员、同项目里程碑、共同受众及原登录。

迁移0069绑定完整资料、真实新草稿和原成功操作，不接收正式修订、文件、状态或验收字段；保留同事务审计、outbox、不可变历史及回执。成功后只开放本次新交付的受控阅读。该追加迁移还让既有里程碑创建／更新标题的首尾Unicode空白比较与Go一致，保留已应用迁移摘要。project-update源码1.4.0含九项准确写工具、read_resource及inspect_operation，继续draft。参数、复现、人工提交验收及验证范围见[增量74](74-secretary-deliverable-creation.md)。

