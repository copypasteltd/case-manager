# J-09 — 独立外部告警、过期与恢复的源码审查

审查基线：`ddf6a70cc2de5fba5d01f64d581c4a228052c1d7`。本记录只读源代码及公开模板，不执行测试、构建、服务、浏览器或真实消息；不读取实际配置、凭据或私有日志。PRD-06仅检查本条运维切片，不对整个PRD-06或GAP-23宣称完成。

> “主应用宕机外部告警仍可达；采集过期为 unknown”
> — [J-09](../../requirements/01-product-and-journeys.md)，44行。

> “外部探测与告警链路必须独立于主应用机，主机宕机仍能触达收件人。”
> — [OPS-01—04](../../requirements/08-status-notifications-and-nfr.md)，15行。

**总裁定：partial（源代码／部署工程），置信度高；实际跨主机、真实服务商送达及恢复均未验证。** 当前过期计算、签名采样及站内资源提醒不是缺失实现；独立外部告警的可部署配置／规则／收件人接线在已检索仓库中缺失，不能全部归作“已有工程、只待真实安装”。这不等于必须重造告警服务：规范明确复用成熟工具，具体供应商尚未由配置确定。

---

## AL-01 — 主应用、PG、Worker故障边界

**Verdict：partial。**

已读调用链：

- [原生CLI](../../../app/backend/cmd/status-bridge/main.go) 55–98行打开桥接器、周期采集并只记录安全结果；[Bridge.Open](../../../app/backend/internal/statusbridge/bridge.go) 42–87行建立Prometheus及平台两个客户端。[cycleTarget](../../../app/backend/internal/statusbridge/bridge.go) 153–196行保存采样后调用`deliver`；199–222行只向平台的签名观测API发送。平台不可达或5xx时返回`DELIVERY_PENDING`，保留准确pending；此路径没有外部收件人投递。
- [通知Worker](../../../app/backend/cmd/platform/notification_worker.go) 45–76行打开平台PG、检查结构并调度资源提醒；[个人TG通知Worker](../../../app/backend/cmd/platform/notification_telegram_worker.go) 15–39行同样依赖平台PG及普通通知服务。把任一进程放到别的主机仍保留PG依赖，不能据此证明主应用／PG故障时仍告警。
- [通知消费者](../../../app/backend/internal/notification/jobs.go) 21–23、37–46行只订阅固定业务事件，包括人工incident和资源健康事件；不订阅可用性`service.observation_received`。[接收器](../../../app/backend/internal/servicestatus/ingest.go) 189–196行记录可用性观测outbox，不等于外部告警已经发出。
- [状态桥接service](../../../app/deploy/native/copypaste-status-bridge.service) 7–17行是独立身份、持久journal及重启模板，未包含Prometheus、Blackbox或外部告警服务的规则和receiver；[发布目录](../../../app/deploy/release/catalog.py) 2–19行只列应用Worker及status-bridge，没有独立告警组件。
- [API健康入口](../../../app/backend/internal/httpapi/server.go) 55–64行：`/health/live`恒200；`/health/ready`在2秒上下文内调用`migrations.CheckCompiled`。[结构检查](../../../app/backend/migrations/compatibility.go) 16–40行实际查询PG结构清单，但不检查所有Worker是否活跃。
- [原生发布Nginx](../../../app/deploy/release/catalog.py) 102–116行代理`/v1/`，明确对`/health/`返回404，根路径可以只返回静态Web；[发布控制器](../../../app/deploy/release/controller.py) 169–181行只在本机访问`127.0.0.1:8080/health/ready`并逐个核对systemd单位。不能给外部Blackbox填公开`/health/ready`后就声称已经验证API／PG；也不能用静态首页200证明API／PG或任务Worker正常。

**反证检查：** 已查普通个人TG、资源通知及独立桥接进程。它们确有各自的恢复／重试机制，但均不是绕过主应用／主PG的独立告警发送链路。`provider=prometheus`只登记采集来源；成熟监控可以由运维另装，故不报告“必须编写新告警服务”的缺陷，而报告仓库缺少可审阅的独立接线工程。

---

## AL-02 — 采集过期、缺样与unknown

**Verdict：implemented（已读源代码）；实际采集与时钟未验。**

- [fetchSample](../../../app/backend/internal/statusbridge/prometheus.go) 56–87行使用准确selector及`timestamp(selector)`，要求原始采样时间一致、非未来且年龄低于限值；`up != 1`返回`SCRAPE_UNAVAILABLE`，不伪造目标失败或新鲜正常值。
- [Advance](../../../app/backend/internal/servicestatus/projection.go) 56–98行拒绝旧序号／旧采样、忽略过期事实，并在序号断层或过期后重置连续成功／失败状态。[VisibleSource](../../../app/backend/internal/servicestatus/projection.go) 102–118行按采集及接收双时间判定`OBSERVATION_STALE`；不存在、停用、未形成有效状态均不能变绿。阈值校验在30–35行限制`stale_after`不超过180秒且不少于三个间隔。
- [readSources](../../../app/backend/internal/servicestatus/sources.go) 14–47行只取当前来源修订并调用`VisibleSource`；[present](../../../app/backend/internal/servicestatus/service.go) 79–127行使用PG当前时刻、准确截止点及维护窗口汇总，维护是覆盖层，底层观测状态保留。
- [资源读取](../../../app/backend/internal/servicestatus/resource_store.go) 110–167行没有新有效采样时不返回旧值为当前健康；[SQL资源依据](../../../app/backend/migrations/0098_resource_notifications.sql) 6–34行独立计算无观测、时钟异常和双时间过期，以及首次采样宽限期结束后的责任。
- [资源调度](../../../app/backend/internal/notification/resource_scheduler.go) 14–64行遍历准确资源来源并经PG投影；[资源事件](../../../app/backend/internal/servicestatus/resource_health.go) 35–87行形成不可变状态／outbox。它依赖运行中的应用Worker及PG；主应用故障时的unknown告警仍需独立链路覆盖。
- [Web状态时效](../../../app/web/src/statusFreshness.ts) 5–16行以单调时钟扣除完整请求耗时；[Web徽标](../../../app/web/src/StatusShared.tsx) 13–16行过期显示待重新确认。[移动live](../../../app/mobile/src/pages/status/live.ts) 58–59行轮询及清理，移动状态页在到期后也不继续宣称正常。这里只读相应时效分支，没有运行UI检查。

**反证检查：** “没有外部告警模板”不意味着平台unknown实现不存在；对unknown重复新增数据库状态机或把`up=0`伪造成业务`healthy=false`会与已有协议冲突。独立规则应明确区分目标失败、采集器失败、时间过期与系列完全消失。

---

## AL-03 — 故障确认与真实恢复

**Verdict：partial（源代码已覆盖平台投影，外部链路未提供）。**

[Advance](../../../app/backend/internal/servicestatus/projection.go) 74–98行按明确阈值计算连续失败／连续快速成功；慢成功不会直接恢复down。过期后重新开始积累，旧样本不构成恢复。[Summarize](../../../app/backend/internal/servicestatus/projection.go) 133–161行分别保留维护展示及原始状态。[资源事件](../../../app/backend/internal/servicestatus/resource_health.go) 63–87行只有同修订／同负责人此前确有待办且真实`operational`才创建恢复进展，重复状态不重复投递；换配置、停用或责任变化不冒充健康恢复。

这证明平台侧已有可复用语义，不能证明外部规则、成熟告警器及实际接收人执行了同样语义。外部告警消失可能因为序列缺失、配置删除或采集器停止，不能笼统称“已恢复”。简单`for: N*interval`也不能不加说明地当成“准确连续N次有效新采样”。具体成熟工具版本／查询及配置需在整批实现时固定并按其官方协议核对；本审查未假设任何未提供的真实监控安装。

---

## 搜索记录与边界

- `rg --files app/deploy docs/requirements docs/operations docs/development app/backend/cmd app/backend/internal`用于定位实际目录，随后读取上述具体调用链。
- `rg -n 'Alertmanager|alertmanager|alerting:|rule_files:|receivers:|telegram_configs:|webhook_configs:' app/deploy app/backend -g '!**/*_test.go' -g '!**/openapi.json'`：0命中。
- 扩展到`app/backend/cmd`、`app/backend/internal`、`app/agent-runtime`、`app/packages`、`.github`，搜索`alertmanager|pagerduty|opsgenie|smtp|alerting:|rule_files:|receivers:|webhook_configs:|telegram_configs:`（忽略测试／生成合同）：0命中。
- 文件名搜索Prometheus／Blackbox／Alertmanager／告警规则配置：只有`internal/statusbridge/prometheus.go`，未找到部署配置。全文相关命中指向桥接实现、现有资源通知及文档中“独立另部署”的描述；[增量81图](../81-native-prometheus-status-bridge.md) 10–19行明确把Prometheus→外部告警画成另一条边，[当前运维文档](../../operations/01-deployment-and-operations.md) 112–114行也未声明已经安装／送达。
- 初始对不存在的`internal/status`路径及Windows通配符路径的读取失败，不作为缺失证据；后续已使用真实`servicestatus`目录及`-g '0098*.sql'`更正。

未验：真实主机／网络／DNS／证书／防火墙、Prometheus／Blackbox／node_exporter／告警器版本与安装、收件人及凭据、主应用停机后的真实投递、监控机自身失效、全量客户端与正式验收。没有测试执行证据，不把搜索或静态分支裁定写成测试通过。未检查其他23包；未要求网络性能、真机、历史迁移／容灾或AI费用门槛。

## 可整批实施的工程建议

可以一次性补成熟监控工具的原生配置生成器、严格输入校验、模板、离线场景用例及安装核对指南；无需接触真实主机或重新实现探测／告警服务。以下是建议设计输入，不是已经确认的真实配置，也不把选用某一家供应商当作原始需求：

| 类别 | 必须明确的输入／输出 | 重点约束及攻击面 |
|---|---|---|
| 版本与故障域 | schema版本、固定工具版本／摘要、监控主机及应用主机声明、专用运行身份和配置／状态目录 | 不依赖平台API／PG生成告警；不自动下载latest或启动服务。两个主机标识不同只证明配置声明不同，正式验收仍需确认实际故障域。 |
| 探测库存 | 有界唯一target ID、用途（HTTPS／API就绪／采集器／获准Worker）、准确job／instance／额外标签、可选service/source/revision对照 | 门户标签不是可执行地址；目标由运维明确批准。防PromQL／YAML／shell插入、重复／歧义标签、任意命令与配置路径穿越。 |
| 网络 | 明确URL、协议、端口、固定许可IP／可信私网边界、TLS server name与验证、重定向策略、抓取超时 | 防SSRF、元数据／回环／链路本地、DNS重绑定、代理或重定向绕过；选择成熟工具实现限制并明确其真实能力，不以离线字符串检查声称已固定运行时解析。 |
| 就绪覆盖 | 明确API＋PG readiness的可到达受限路径；必要Worker的准确固定单位／已批准导出器指标 | 现有公网`/health/`被拒绝，首页与live不能替代ready；不放开任意公共探测接口。APIready不证明所有Worker能处理任务，离线可验证独立规则与固定单位清单，实际指标仍需目标安装核对。 |
| 告警及恢复 | 明确评估／采集间隔、连续确认依据、无系列／过期策略、故障／恢复／未知分组、聚合／抑制及有界重试 | 不能因无系列或移除配置发送虚假恢复；不能将时间窗等同有效连续采样。通知写入最少身份和安全状态，不含业务名称／请求内容／凭据。 |
| 接收人 | 单独批准的固定receiver类型／ID、真实凭据的私密文件路径、固定门户URL、发恢复与限速规则 | 独立监控凭据不取得平台PG／员工会话／存储权限；不自动复用个人TG或股东群授权。禁止任意外发地址，默认不开启、不向任何人发实际消息。 |
| 输出与安装 | 可审阅的Prometheus／Blackbox／成熟告警器配置、专用systemd模板、组件／配置摘要、严格生成及只读核对命令 | 原子生成到明确新目录、不覆盖配置／密钥；配置文件与服务身份最小权限，状态目录和资源限额受控；禁止把Docker/WSL当安装依赖。 |
| 全波用例 | 先完成生成器／模板／脚本及离线场景，再统一检查正常／目标失败／采集失败／无样本／陈旧／真实新样本恢复／危险输入拒绝 | 使用隔离合成输入，必要时调用固定成熟工具的官方配置校验器；不启动真实服务或发消息，不把本地配置通过写成跨主机及真实送达验收。 |

English: stale/unknown computation and platform-side recovery already exist. No deployable independent receiver/rule wiring was found in the inspected source/configuration paths. A bounded native configuration and validation package for mature tools would be concrete code progress; real targets, separate infrastructure, recipient approval and actual delivery remain open.
