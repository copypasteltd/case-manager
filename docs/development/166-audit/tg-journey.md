# PRD-04／J-08 — 股东群请求、授权和披露源码审查

审查基线：`ddf6a70cc2de5fba5d01f64d581c4a228052c1d7`（增量165）；只读应用源码，不执行测试、构建、数据库、服务、浏览器或真实Bot操作。本文是有界规范对照，不能替代整个GAP-11／15或真实模型／群／平台验收。正式工作包仍开放。

## 需求和判定

> 指定股东群可通过已绑定身份与秘书沟通，群披露范围受控。
>
> 提及 Bot → 身份/群校验 → 受理 → 查询/执行 → 摘要+链接。
>
> — [产品需求](../../requirements/01-product-and-journeys.md):20、43（PRD-04／J-08）

**Verdict: partial，源码判断置信度高；待独立反证。** 已有完整受理、两个只读任务直接调度、八类任务本人门户衔接、16项固定项目变更预授权、原群通用通知和单独人工审核正文披露。发现一处明确停止保证未覆盖新增授权类型：已触发TG固定项目预授权的通用通知，没有复核原组织授权版本。没有把该问题扩大成业务正文泄漏或任意写入。

| 子项 | 源码判定 | 依据与边界 |
|---|---|---|
| 数字身份绑定和单用证明 | implemented（所读链路） | 私聊证明、原登录确认、数字身份永久归属和当前会话均有检查；未运行真实TG绑定。 |
| 准确群／话题、明确触发和成员变化 | implemented（所读链路） | 白名单、当前员工、匿名发送者拒绝、成员／迁群暂停均可定位；真实群配置未提供。 |
| 八类任务进入已授权执行 | implemented（有界路径） | 两类直接只读；其他任务通过本人门户；16项项目操作另有一次性固定授权。不存在“其余七类完全没有执行链”的缺口。 |
| 业务正文原群披露 | implemented（本人门户转接结果路径） | 原群、准确结果、独立近期MFA、当前权限及有限Unicode片段；不推断任意群、任意结果或固定grant正文披露已开放。 |
| 组织范围变化后停止通用通知 | partial，F-TG-166-01 | 旧授权函数只识别原只读链接，漏掉`telegram_write_activations`。 |
| 实际Bot／模型／Linux／四端正式运行 | undecidable | 无外部配置及现场证据，本次不启动集成。 |

---

## 实际走读的入口和守卫

### 身份和入站

- `app/backend/internal/httpapi/server.go:193–229,245–289`：所有本人读／命令路由经真实认证；Web Cookie写入检查Origin与CSRF，拒绝混合凭据。`telegram.go:21–54`的本人请求／绑定路由复用这些包装器。Webhook是独立入口，`telegram.go:148–191`检查唯一secret头、固定JSON、无压缩、64KiB大小，成功持久化后才返回200。
- `telegram/bindings.go:18–33,35–61,64–93,97–154`：挑战绑定组织／员工／原session／Bot／失效时间，数字证明只在原登录重建；创建和确认要求近期MFA，确认准确版本和TG数字ID；`telegram_identity_owners`已有归属不被覆写。
- `telegram/ingress.go:25–56`：私聊证明校验MAC、单用／未过期挑战与当前原登录；`138–160`拒绝Bot、匿名sender_chat及不相符私聊身份。
- `telegram/update.go:153–180,203–237`：只接受指向本Bot的命令／提及／回复；八类Skill必须在群允许列表内，多Skill无默认值时不按数组顺序猜选。`telegram/channels.go:15–26`枚举八个准确Skill。
- `telegram/ingress.go:95–135,165–220`：成员变动／迁群先暂停入口；准确chat/topic查询，检查当前channel版本对应消息时刻、有效绑定和active员工，保存准确Skill、原消息和身份戳。
- `telegram/ingress.go:223–263`：`(bot,update)`事务锁和持久摘要去重；相同update不同正文报冲突，分类与inbox插入在同一事务，不把重放重新入队。

### 从请求到执行

- `agent/telegram.go:78–148`：只选择`project-briefing`、`incident-summary`以及准确`project-update authorize:`触发；组织排他锁串行处理，重查当前绑定、员工、群／话题／Skill及30分钟时限，运行配置／容量暂不可用保留等待，不吞掉非预期500。
- `agent/telegram.go:151–243`：只读会话固定员工、组织／群版本、配置和来源，原消息、run、job和link同事务保存；机密分类进入另行明确授权流程，不能由群文本自动扩大范围。
- `agent/telegram_handoff.go:34–87`、`agent/runs.go:748`：本人门户选准确请求摘要／版本，必须本人private会话和近期MFA，保存不可变handoff后将原请求记为人工转接。`migrations/0116_telegram_human_handoffs.sql:16–45`是不能忽略的数据库最终守卫：精确匹配请求版本／原文摘要、当前channel和binding版本、相同项目、Skill、运行actor／会话及当前组织授权。
- `agent/telegram_handoff.go:90–107`：后续执行继续重查原登录、当前组织授权、原channel／binding及有效转接状态。不能因只看到dispatcher的两类名单就断言其余Skill缺失。
- `agent/telegram_write_grants.go:78–154,168–214,231–294`：固定预授权仅消费一项完整现有项目变更，绑定本人原登录、原群／话题、绑定、部署配置和有效期；建立时不入队；原群本人准确触发后重查原依据、来源、业务变更并一次性激活和入队。`migrations/0128_telegram_project_grants.sql`把允许的准确项目变更扩展为16项，不能从群正文补参数。
- 读过的门户路由为`httpapi/agent_runs.go:43–78`及本人TG请求路由；Web／共享移动组件对应入口存在。未完整审阅所有压缩JSX分支，不据此签字四端UI全部通过。

### 原群输出和未知回执

- `telegram/delivery.go:72–100,133–178,228–269`：合并当前阶段，只发固定阶段文案和需要登录的请求链接，使用原chat/topic/message；生成候选及实际准备均调用当前授权SQL。已开始发送的失租约尝试改为`delivery_unknown`，待发候选仅`pending/retry_wait`，不自动重发unknown。
- `telegram/delivery_transport.go:49–75`：固定Telegram HTTPS目标、拒绝重定向，传输结果不确定保留unknown；未完整核验所有供应商错误分类和限流分支，本次不是transport完整审计。
- `agent/telegram_disclosure.go:43–85,97–135`：正文披露只从本人已完成handoff结果生成；复用`NotificationRun`、完整来源可读性、准确结果版本／摘要和独立近期MFA。数据库`0117_telegram_reviewed_results.sql:12–46`固定原群、原绑定、组织／员工授权版本、结果摘要和会话；最多2000字符。
- `telegram/reviewed_delivery.go:10–35`在准备网络前重新读取上述准确结果，截取固定Unicode范围，带原消息回复和关闭链接预览。未发现把模型输出直接无审阅群发的可达路径。

---

## F-TG-166-01 — 已触发固定项目授权的通知漏查原组织授权版本

> 准备发送时核对当前组织、用户认证版本、有效绑定、群入口版本／状态、Bot归属及固定话题；已有任务还核对原TG会话授权版本。
>
> 撤销绑定、群暂停、范围或认证版本改变阻止后续准备。
>
> — [TG-OUT-02／07](../64-telegram-delivery.md):21、26

**Verdict: partial，置信度高；源码可达，尚未运行复现。**

### 缺口调用链

1. 固定项目grant由`telegram_write_grants.go:123–128`保存`organization_auth_version`及真实run；`:283–289`激活只追加`telegram_write_activations`，不插入原只读`telegram_run_links`。
2. `migrations/0125_telegram_write_grants.sql:485–489`新增`telegram_effective_run_links`，用UNION覆盖原只读links和grant activations。
3. `telegram/delivery.go:134–137,179–183,228–246`已经用effective view关联运行与当前阶段，但发送资格仍调用`telegram_delivery_authorized`。
4. 全部迁移中该函数只有`migrations/0063_telegram_delivery.sql:4–18`一处定义。它在`:10–11`仅LEFT JOIN原`telegram_run_links/telegram_conversation_scopes`；`:17–18`条件为`l.run_id IS NULL OR (... s.organization_auth_version=o.authorization_version ...)`。
5. 对activated grant，原link不存在，`l.run_id IS NULL`为真。只要员工认证版本、绑定、群／topic配置仍匹配，之后组织授权版本变化不能使该发送谓词失败。普通准备路径只生成`portalMessage`，不会经过`NotificationRun`或`telegramGrantState`补查原grant授权版本。

### 已尝试的反证

- **“执行器已经检查grant版本，所以通知也安全”**：`agent/telegram.go:48–53`确实在执行调用`telegramGrantState`；但独立`telegram.Service.PrepareDelivery`不经过此函数，任务终态通用通知也由独立worker准备。
- **“effective view已补上原链接”**：view只用于delivery的运行状态连接；被调用的旧SQL仍查原表，不读取view或grant。简单把旧SQL的原link改为view也不正确：grant是本人private会话，没有旧`telegram_conversation_scopes`行，会把合法grant全部拒绝，需要单独固定grant依据分支。
- **“消息没有敏感正文，所以没有问题”**：确实没有项目名、请求正文或业务结果；这是影响边界，不能据此声称正文泄漏。但TG-OUT-07仍明确承诺范围变化后停止准备，源码未对新增类型满足此承诺。
- **“群暂停或解绑会阻止”**：会阻止；本例保持二者不变，只改变已有grant固定的组织授权版本。
- **“有后置数据库守卫覆盖”**：`0117`重写delivery origin/state守卫仍调用相同`telegram_delivery_authorized`；搜索后续迁移未发现替换。`0125/0128`的grant／mutation守卫保护执行，不在通用发送准备中生效。

### 可一次性实现的修正建议

追加正常迁移，保留所有旧行／旧事件：让通用投递授权显式区分尚未调度请求、原只读link和已激活固定grant。grant分支按准确activation、run、grant、actor、原群／绑定、组织／员工授权版本复核；保留所有原有Bot、topic、窗口、开关和当前身份检查。不得扩展为自动正文披露，也不把原只读会话要求直接套到private grant。

统一代码完成后再加入原生业务用例：合法grant先能准备通用通知；改变组织授权版本而不改变员工认证版本／channel／binding后，新的准备被抑制；超过100条清理窗口的候选也不能被实际发送选择器选中；准确旧通知／发送中unknown事实不改写。普通只读请求和未调度受理提示保持原有语义。未在本审查运行这些用例。

---

## 搜索记录、未确认项和未检查范围

- `rg --files ... | rg 'telegram|Telegram|...|preauth'`发现入站、dispatcher、grant、handoff、disclosure、delivery、本人门户及相关迁移；未把文件名当执行证据。
- `rg -n 'CREATE( OR REPLACE)? FUNCTION (telegram_delivery_authorized|...reviewed...)' app/backend/migrations -g '*.sql'`：通用授权只有0063一个定义；reviewed授权另在0117，未发现后续覆盖通用谓词。
- `rg -n 'telegram_human_handoffs|...effective_run_links|...write_grants' ...`：确认0116／0117、0125／0128的不同收据类型和最终守卫。
- `rg -n 'recordTelegramHandoff|telegramHandoffAuthority|telegramGrantState|NotificationRun' ...`：找到真实执行和结果读取调用；通用`portalMessage`准备没有调用结果读取，不虚构其补查。
- 一次猜测`httpapi/identity.go`的读取不存在，改为实际`server.go`认证包装器；该不存在文件不作为缺失鉴权的证据。一次Windows路径通配符搜索未成功，之后使用目录＋`-g`准确重查。
- J-08异常栏的“未绑定、群无权限、严格受限只给登录入口”可能被解读为所有拒绝都必须主动回复链接。当前入站对无绑定／无群白名单直接ignored；64方案只对已接受请求生成群通知。未明确是否必须给拒绝消息回复，故不将其列为代码缺陷，也不擅自新增Bot回话／群刷屏策略。
- `outbound-preview`只支持已人工handoff的结果，activated project grant没有该handoff行。这是源码能力边界；当前161方案明确“仅本人既有TG人工转接结果”，故不把任意grant正文群披露列作已承诺缺陷。
- 未读取生产配置、密钥、私有日志、真实业务资料；未核验真实Bot、模型、Skill安装、网络、正式Android和完整四端UI。八项Skill仍draft；正式GAP-11／15及其余当前编号不因本报告完成。
