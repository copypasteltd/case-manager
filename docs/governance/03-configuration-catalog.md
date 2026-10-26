# 配置目录与候选默认值

版本：v1.0-draft；日期：2026-10-23。候选值用于实现和验证，不是已确认公司制度。安全边界不得被普通配置管理员随意关闭。

## 1. 配置机制

部署参数由受限环境配置/密钥服务提供；业务参数保存在 PG 的版本化配置中。生效记录包含旧值摘要、新值、操作者、理由、时间和版本。变更须校验上下界；失败不覆盖旧配置。每个任务固定其使用的配置版本，撤权/禁用等即时安全策略仍在每次执行时重查。

敏感值只显示“已设置/密钥版本”，不返回明文。环境分 dev/test/staging/prod；不同存储前缀、PG、TG Bot、模型费用项目和平台账号不得串用。

## 2. 参数清单

| 配置名 | 候选值或来源 | 约束与生效 |
| --- | --- | --- |
| `app.timezone_default` | Asia/Hong_Kong，Q-07 | DB 时间存 UTC；账期按业务时区；改默认不重写历史 |
| `finance.base_currency` | 未设，Q-07 | 跨币种报表生效前必填；已入账记录不追改 |
| `workflow.*.definition` | 未发布，Q-06 | 无有效流程禁止正式提交；不回退成自动批准 |
| `workflow.self_approval` | 默认拒绝，Q-06 | 允许例外须明确人员、金额、场景；保留审计 |
| `auth.access_ttl` | 15 min | 候选；每次请求查会话/授权版本以支持立即撤销 |
| `auth.refresh_idle/max` | 7d / 30d | 旋转刷新、重放撤销；管理员敏感动作需二次验证 |
| `auth.mfa_admin_required` | true | 管理员/高影响权限必须；恢复码单独保管 |
| `file.max_bytes` | 120,000,000,000 | 用户 120 GB 基线；精确字节显示；120 GiB 验证另作扩展，不混算 |
| `file.mini_attachment_max_bytes` | 20,000,000 | A 候选；微信/抖音分别实测，不能当官方统一上限 |
| `upload.part_bytes` | 33,554,432（32 MiB） | 会话创建后固定；分片数须满足两云能力；120 GB 约 3577 片 |
| `upload.parallel_parts` | 2 / 会话 | 候选；账号并发/带宽另限，避免手机与服务器争用 |
| `upload.sign_ttl` | 15 min | 只签 draft、单 upload/part；过期可刷新，不重建会话 |
| `upload.idle_expiry` | 72h | 以服务端活动记录；须短于云端未完成上传回收且保留安全余量 |
| `download.url_ttl` | 60s | 严格受限走鉴权代理；已签 URL 到期前无法保证即时撤回 |
| `scan.*.max_bytes/timeout` | 按格式 PoC 冻结，Q-14 | 超限标记无法扫描；高敏不自动放行；不编造全格式“安全”结果 |
| `replica.small_lane_max_bytes` | 100,000,000 | 此阈值下正常网络候选 p95 15 min；非大文件承诺 |
| `replica.large_jobs/parts` | 1 / 2 | 每条大任务固定源版本；小文件单独队列 |
| `replica.retry_cap` | 8 次自动尝试 | 指数退避+抖动；耗尽告警/人工恢复，不无限刷流量 |
| `retention.trash_days` | 90，Q-11 | 未确认不启用永久清除；法务保留优先 |
| `retention.history_days` | 180，Q-11 | 当前版/签署引用/有效恢复点引用不能按天数盲删 |
| `backup.logical_interval` | 6h | 全局目录关联同快照引用；成功完成及复制时间参与 RPO |
| `backup.pitr_days` | 14，Q-03/12 | 需真实备份产品能力；不以 pg_dump 充当物理 base backup |
| `backup.catalog_interval` | 24h | 补充全量目录，不代替每次 DB 备份的引用清单 |
| `retention.backup_daily/weekly/monthly` | 14 / 8 / 12，Q-11 | 恢复集包含 DB、对象引用、校验和密钥可得性 |
| `retention.audit_days` | 365，Q-11 | 候选；归档也受权限和备份保护 |
| `retention.chat_days` | 180，Q-11 | 业务提交/审批证据独立保留，不随聊天清理消失 |
| `retention.probe_raw_days` | 30 | 聚合 365d 候选；清理不得影响未关闭事件证据 |
| `agent.concurrent_runs` | 1（可评估至 2） | 队列外置；不可用时 OA 不受牵连 |
| `agent.run_timeout/tool_limit` | 15 min / 50 | 候选；上传复制作为业务 job，不撑住推理线程 |
| `agent.daily/monthly_budget` | 未设，Q-17 | 生产 AI 启用前必填；估计预留+实际结算，供应商异步账单有误差 |
| `agent.bulk_threshold` | ≥20 个对象或跨项目 | 进入固定计划 L2；逐项授权；阈值降低不扩大权限 |
| `agent.plan_ttl` | 30 min | 权限、内容、基础版本或对象列表变化即失效 |
| `agent.skill_write_enabled` | 各 Skill 独立开关 | 可仅关闭写入；不能关闭 Go 鉴权 |
| `telegram.binding_ttl` | 10 min | 单次使用随机凭证；不在群里发送绑定秘密 |
| `telegram.allowed_chats/topics` | 空 | 未配置无群资料访问；numeric ID，迁移后复核 |
| `status.probe_interval` | 60s | 各探测不同错峰；外部独立探测 |
| `status.failure/success_threshold` | 3 / 2 | 迟到结果不覆盖新状态；维护窗口保留原始失败 |
| `status.stale_after` | 180s | stale→unknown；不可保持绿色 |
| `jobs.lease/heartbeat` | 120s / 30s | 长任务续租；fencing token 阻止旧 worker 提交 |
| `api.idempotency_retention` | 最少 7d | 金融/审批永久业务唯一键另存，不随缓存过期消失 |
| `api.page_default/max` | 20 / 100 | 稳定 cursor；搜索片段/计数同 ACL |
| `local.temp_quota` | 按实测冻结，Q-01 | 分用户/任务配额；建议至少 20% 余量，禁止整份 120 GB 落盘 |

## 3. 生产缺失参数行为

PG、主/副存储或加密密钥缺失：相关服务 readiness 失败。审批规则缺失：仅该类申请禁止提交，草稿允许。费用币种/汇率缺失：原币展示，禁止输出伪完整折算总额。模型/预算/资格未放行：秘书显示未启用。TG 范围为空：不处理群资料。保留策略未确认：不执行不可逆清理。

同一参数不能在前端、Worker、Skill 各写一套默认。服务端提供非敏感 capability/config 视图，各端据此展示限制；服务端最终校验。


## Telegram 入站实现配置（增量60）

当前实际键为 `CP_TELEGRAM_INGRESS`，空值或 `disabled` 关闭接收；JSON数组最多8项／8192字节，每项为Bot ID、组织、用户名和独立Webhook secret，严格校验后随API启动生效。secret不返回门户，白名单准确chat/topic、项目范围及允许Skill由`policy.manage`用户经近期MFA和版本检查调整，并保存实际历史。新入口暂停，群成员或标识变化再暂停。`execution_available`和`delivery_available`当前均false；没有Bot token配置或自动注册Webhook。字段与边界见[实现契约](../development/60-telegram-binding-and-ingress.md)，部署资料仍需后续提供。

## Telegram 调度实现配置（增量61）

`CP_TELEGRAM_DISPATCH_BOTS` 为独立dispatcher的身份JSON数组，仅允许 `bot_id`、`organization_id`，最多8项／8192字节；不接受Webhook secret、Bot token或其他未知字段。缺失／空值／disabled时worker拒绝启动。API接收配置、入口MFA配置及已核验的TG运行配置分别受控，任一不足不创建虚假运行。

`telegram_channels.default_skill_id` 可空且必须属于允许任务；明确命令优先，多个允许任务而无默认不猜选。`agent_runtime_profiles.telegram_configuration_sha256` 只用于项目简报TG读取配置，安装后不可改写；普通Web哈希不足以启用TG。新原生service使用独立env文件，开发配置保持关闭。完整字段、等待／撤回和实际工具边界见[增量61](../development/61-telegram-agent-dispatch.md)。


服务事件摘要（增量62）沿用现有服务目录、签名采集策略、AI 分类政策及执行配置；未新增秘密环境变量。`telegram_configuration_sha256` 可绑定 project-briefing 或 incident-summary 的准确原生配置，仍不可事后修改。事件摘要的窗口、条数、正文上限见[工具协议](../development/62-agent-service-summary.md)，不是任意命令入口。

## 增量63财务摘要边界

subscription-review 源码 1.1.0 draft；工具固定当前公司分摊／单一项目分摊、2000—2100年月、三位大写可选币种，单次最多500项授权输入及既有20个任务来源。缺失配置继续不可执行；未增加自动付款、供应商操作或 TG 财务授权开关。详见[契约](../development/63-agent-finance-summary.md)。


## 增量64：TG 发送配置与固定边界

`CP_TELEGRAM_DELIVERY_BOTS` 仅供独立发送进程；空值／disabled 关闭。严格 JSON 数组 1—8 项，字段为 bot_id、organization_id、token、portal_origin。Token 必须归属同一数字 Bot；门户为不含路径、查询和片段的 HTTPS origin，不能来自模型或员工消息。每个入口另有默认 false 的 delivery_enabled，保存需要 policy.manage 和近期两步验证。

固定边界：全 Bot 最小准备间隔 3.2 秒，网络超时 8 秒，准备租约 30 秒，回执上限 64 KiB，单阶段最多准备 5 次，待发窗口 24 小时，进程活动状态 90 秒。供应商 retry_after 以 1—86400 秒接收，未知结果停止自动重试。没有提高额度、改群、重定向或业务正文广播开关。见[实现与配置](../development/64-telegram-delivery.md)。
