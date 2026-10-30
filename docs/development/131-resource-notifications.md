# 资源告警、通知与待办 / Resource alerts, inbox and workbench · 131

增量131接通“原始资源观测 → 状态变更 → 站内通知 → 本人待办 → 真实恢复”的业务流程。Web、微信、抖音、Android共用同一后端及共享移动业务实现；本轮不接入外部告警服务，不把站内事件当作TG或其他通道已送达。

## 员工如何使用

1. 在服务详情明确发布资源监控配置。管理权限和最近MFA沿用增量130；阈值、全部挂载点和45至180秒采集断流阈值均绑定该次不可变修订。
2. CPU、内存或磁盘达到警告／严重阈值，磁盘变只读，或启用来源超时无有效采样时，当前有权负责人会在通知中心看到“主机资源状态有新进展”。工作台“我负责的主机资源待办”提供准确服务入口。
3. “标记已读”只记录阅读进度。故障仍存在时，待办保留；打开相关资料会重新检查当前权限。
4. 真实完整指标恢复后，待办依据原始观测消除，并在同一配置的通知线程产生新的未读投递。明确停用、归档或转交后重新计算责任，不显示虚假的“恢复正常”。
5. 在服务详情选择“资源状态记录”，分页核对当时的状态、原因、负责人、配置和观测。带观测依据的记录可核对原始SHA-256与Ed25519签名。历史记录不是当前健康结论；通知事件创建也不是投递回执。

## 业务状态约束

| 编号 | 原始依据 | 当前待办 | 提醒规则 |
|---|---|---|---|
| R131-01 | 未配置、停用或已归档 | 无 | 不发送资源提醒；保留历史，不制造恢复 |
| R131-02 | 新修订尚无观测，未到修订创建时间＋断流阈值 | 无，状态为unknown | 给首次完整采样明确宽限期 |
| R131-03 | 到期仍无任何已采纳观测 | 需要核对采集 | 创建NO_OBSERVATION提醒，不能无限静默 |
| R131-04 | 已采纳观测的采集或接收时间到期 | 需要核对采集 | 创建OBSERVATION_STALE提醒，不把旧数值当作当前值 |
| R131-05 | 观测时间暂在未来 | 需要核对时钟 | CLOCK_SKEW；原接收协议仍拒绝超过15秒未来偏差 |
| R131-06 | 任一资源达到警告／严重阈值，或磁盘只读 | 负责人待处理 | 阈值与精确字节比例一致；只读直接严重 |
| R131-07 | 同修订、同负责人真实恢复operational | 无 | 先前确有待办才发送恢复进展；新修订不继承旧指标 |
| R131-08 | 重复采样，状态／原因／责任／配置均未变化 | 保持原结果 | 不增加状态事件或重复投递；所有原始观测继续保留 |
| R131-09 | 负责人变更 | 转交当前有权负责人 | 新事件固定新负责人；排队旧事件不改收件人 |
| R131-10 | 通知已读、任务重试、Worker重启 | 不据此消除业务待办 | 读取原始当前证据和权限，不能依靠通知缓存判断恢复 |

CPU为既有五分钟指标；内存与磁盘使用率向上取整至基点。PostgreSQL使用numeric精确运算，避免64位容量乘法溢出或JavaScript浮点丢字节。Go资源卡片和数据库待办计算保持相同阈值、未来时间及双时间过期规则，并以实际HTTP／PG测试核对。

资源健康独立于HTTP可用性、维护窗口和人工服务事件。资源异常不自动创建或解决incident，也不因接口仍在线而隐藏资源问题。

## 数据、事务和调度

正常迁移97→98新增两张表和三个计算函数，不改写旧97配置、原始签名、服务历史、文件、OA及财务事实。

| 对象 | 责任与约束 |
|---|---|
| `service_resource_current` | 从当前配置、服务生命周期和最新已采纳原始观测计算状态与首次采样期限 |
| `resource_capacity_bp` | 精确容量使用率；仅处理既有严格验证后的不可变观测 |
| `resource_needs_attention` | 统一警告／严重、时钟及断流待办条件 |
| `service_resource_health` | 串行化的最新状态头；不是工作台业务事实来源 |
| `service_resource_health_events` | 原始负责人、配置、前驱、顺序、评估时间、原始观测和提醒意图；不可修改或删除 |
| `outbox_events` | 同一事务记录固定来源、状态顺序及健康事件；不能注入任意收件人、地址或正文 |
| `notification_deliveries` | 原租约Worker实际完成站内投递后才形成不可变回执 |

配置发布、签名接收和服务负责人／生命周期修改均在原事务内调用同一投影器。锁顺序为组织、服务、来源和状态头；状态历史、outbox、审计与业务变更一起提交或一起回滚。数据库检查精确前驱、连续顺序、最新状态头、原始已采纳观测、当前配置／负责人／评估事实及匹配outbox，禁止脱离依据的头更新。

既有`worker-notification`每10秒扫描一页最多100个来源，包括停用来源，每个来源独立有界事务，按来源UUID继续并循环。即时命令处理状态变化；定时扫描负责时间自然到期和首次采样截止。重复或并发扫描不重复生成事件。多页完整巡检的延迟随来源数增加，不宣称所有规模都具有固定10秒全量时限。

资源事件进入既有`notification-consumer.v1`和`notification.deliver.v1`。输入仍只有业务事件ID，保留租约、栅栏、幂等、不可变结果和受控人工重试。每个“组织＋本人＋服务＋配置修订”独立合并通知线程。消费时重新检查原始事件、当前状态头、配置、负责人、用户状态、频道和普通`service.read`权限；过时、已转交或已撤权的排队事件抑制投递。恢复也会重置未读进度，旧已读序号不能吞掉新投递。

## 页面、API与权限

工作台现在包含六类本人工作：审批、合同到期、订阅账期、服务事件、文件存储、主机资源。完整授权计数与最多五条样本在同一快照读取。资源项直接读取当前配置及原始观测；即使定时Worker尚未运行，已经过期的来源也会成为待办。

新增`GET /v1/services/{id}/resource-monitor/health-events?limit=10&cursor=...`，采用当前人工会话、普通服务读取权限和绑定本人／授权版本／服务的游标。历史分页不授予原件或其他服务权限；签名核对仍走既有准确观测证据API。Web查询按人员及授权版本隔离，关闭后不保留历史查询缓存；共享移动页面沿用原会话作用域。

通知增加`service_resource`分类和固定`service`目标。Web只允许`/services/{uuid}`；移动只允许`/pages/status/index?kind=service&id={uuid}`。服务器返回的其他跳转文本、外链或讨论上下文不能成为任意跳转。通知显示“资源待核对”或“资源进展记录”，不把30秒前读取的通知快照显示成当前主机正常证明。

实现入口：`app/backend/internal/servicestatus/resource_health.go`、`internal/notification/resource_scheduler.go`、`internal/workbench/service.go`、`cmd/platform/notification_worker.go`；Web `ResourceHealthHistory.tsx`；共享移动`pages/status/resource-health-history.tsx`；共享工作台与通知合同由原API生成流程维护。

## 部署与交付边界

沿用原生API、PostgreSQL和`worker-notification`，没有新增外部服务或密钥。先运行正常结构升级，再部署匹配98项迁移的API和Worker；原生Worker启动时校验迁移匹配。保留原130采集器及目标配置。持续运行的通知Worker是断流主动提醒的必要条件；Worker停止期间，工作台仍按读取时刻计算待办。

真实Prometheus／node_exporter／Blackbox目标部署、实际目标故障与恢复，以及独立于本站故障域的外部告警送达继续为GAP-23。真实云存储故障／跨通道回执和完整公司业务验收继续为GAP-08／18；八项Skill仍draft。当前23个活跃工作包、5项移出不变，不把本子流程完成记为整个工作包验收通过。

不运行Docker、WSL、虚拟机、模拟器、真机、网络性能或实网120GB检查，不增加历史迁移／容灾或AI费用控制。源码验证、复用的旧证据、实际构建缓存和目标环境验收分别记录于[本轮核验](../quality/131-resource-notifications-verification.md)及[全部余项](../delivery/05-functional-closure-131.md)。

## English guide

Increment131 connects original signed resource observations to immutable state changes, owner-bound in-app notifications, current personal work and real recovery. Publishing an enabled source explicitly selects its thresholds and45–180-second deadline. Before the first-sample deadline, no observation means unknown with a grace period; after it, the source requires attention. Stale or future-dated evidence also requires attention. Readonly filesystems are critical.

Reading a notification acknowledges only its observed delivery sequence. Work remains until current evidence recovers, the source is explicitly disabled, the service is archived or responsibility changes. A genuine recovery under the same revision/owner creates a new unread delivery. Repeated same-state samples do not spam the inbox. Reassignment never redirects an old queued event; delivery rechecks its original recipient, current source/head, active user and ordinary service authority.

Open the personal resource-work card or a resource notification to reach the ordinary service detail. “资源状态记录” lists immutable historical evaluations with their original owner, revision and optional observation. Verify the exact original signature when an observation exists. An event-creation record is distinct from a completed notification-delivery receipt, and historical normal status is distinct from current health.

Schema97→98 adds state heads/history and exact SQL evaluation, preserving all original business and signed-evidence records. Existing notification workers project commands immediately and scan bounded100-source pages every10 seconds for expiry; large inventories require multiple pages. Workbench reads calculate attention directly from original evidence even before the worker catches up. No additional external service is required.

All four clients retain the full scope. Actual independent external alerts, target installations, real cloud/model/TG/platform/production acceptance and complete business journeys remain open in their numbered work packages. Local synthetic UI recordings, a debug APK and cached native test XML do not close those acceptance items.
