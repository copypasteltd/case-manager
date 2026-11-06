# J-09 独立外部告警：独立反证记录

审查基线：`ddf6a70cc2de5fba5d01f64d581c4a228052c1d7`，增量165源码。独立于本轮第一份[J-09审查](external-alerts.md)，先搜索实际源码／发布工程，再核对该审查的主张。未执行测试、构建、服务、浏览器、真实网络探测或消息发送；未读取私有配置、凭据和生产日志，未改动应用、旧迁移或历史证据。本记录只裁定该需求切片，不关闭GAP-23。

**裁定：partial；外部整机故障告警的部署工程缺口成立，但“仓库没有任何脱离HTTP API的发送路径”不成立。** 已有资源过期调度与个人Telegram通知进程可以绕过HTTP API，仍依赖平台PG、业务授权及显式个人同意；当前源码没有证明它们已安装在另一台主机，也没有提供完整的外部端点探测、规则和固定收件人部署。独立于主应用机是需求；额外要求平台PG同时宕机时仍成功，不是本条文字已明确规定的验收边界。

## 1. 规范边界

- [J-09](../../requirements/01-product-and-journeys.md)，44行：主应用宕机外部告警仍可达，采集过期为unknown，恢复留存历史。
- [OPS-01—03](../../requirements/08-status-notifications-and-nfr.md)，7–15行：复用成熟监控工具；可用性、资源健康和无新数据必须区分；**“外部探测与告警链路必须独立于主应用机，主机宕机仍能触达收件人。”** 仅独立systemd进程或桥接器存在不足以证明此项。
- [原生桥接协议](../81-native-prometheus-status-bridge.md)，3–20行：桥接运行在受信监控主机，外部独立告警单独验收；图中Prometheus直接接外部告警链路。文档没有指定必须使用Alertmanager，因此缺少该产品的文件名本身不是不符合需求的充分证据。

## 2. 尝试推翻“无独立外部链路”的结果

| 反证尝试 | 已读代码与边界 | 结论 |
|---|---|---|
| 桥接器本身发送外部告警 | `statusbridge/config.go:33–70`只有Prometheus、平台API、journal及目标配置；`bridge.go:42–87`只建立两类客户端；`153–222`持久化采样，再POST平台的`/v1/status-sources/.../observations`或`/v1/resource-sources/.../observations`。不可达／非200返回`DELIVERY_PENDING`，没有收件人发送分支 | 未推翻：桥接是签名事实传输路径，不能代替外部告警 |
| 通知Worker不依赖HTTP API，能自行判断断流 | `cmd/platform/notification_worker.go:45–79`直接打开PG、检查schema、每10秒调用`ScheduleResources`；`notification/resource_scheduler.go:14–67`扫描来源并直接调用`ProjectResourceHealth`。`servicestatus/resource_health.go:35–81`读取PG时钟和当前资源事实，生成健康变化outbox；`0098_resource_notifications.sql:31–35`将`OBSERVATION_STALE`／`CLOCK_SKEW`的unknown列为需关注 | **部分推翻过强断言**：只停HTTP API，PG和该Worker仍可用，已有资源来源可产生过期提醒；无需新造同类调度器 |
| 个人Telegram进程能将上述资源提醒发给外部收件人 | `notification/jobs.go:21–23/96–104`消费`service.resource_health.changed`并生成`service_resource`站内线程；`models.go:76–81`包含该类别。`notification/telegram_worker.go:23–39/47–91`从当前站内投递读取，按个人启用政策、绑定、类别、未读与24小时范围准备发送；`cmd/platform/notification_telegram_worker.go:15–53`直接连PG并调用Bot传输，无HTTP API调用 | **已有条件路径**：有效个人同意／绑定、当前权限、PG及两个Worker存活时，资源提醒可以脱离HTTP API发送。没有据此证明整机故障独立性，或所有可用性故障都会进入该类别 |
| 普通可用性观测会自动进入上述发送路径 | `notification/jobs.go:21–23/37–46`固定事件清单没有`service.observation_received`，只含人工incident和资源健康事件；`jobs.go:96–104`该分支明确读取资源来源／当前owner | 未推翻：资源unknown不等价于所有端点down告警，人工incident也不是独立故障探测 |
| 已有独立systemd模板就是独立主机部署 | `copypaste-status-bridge.service:2–17`、`copypaste-notification-worker.service:2–16`、`copypaste-notification-telegram-worker.service:2–16`分别提供原生进程；没有主应用API的systemd依赖 | **只证明进程可分开**。模板不禁止管理员另机安装，但文件本身不指定故障域、独立监控目标、收件人或安装验收；不能把未知部署事实认定为已完成 |
| 当前正式打包流程另有外部告警配置 | `build_production_release.py:84–114`固定构建platform／agent-native／status-bridge，复制Web、runtime、发布工具及Nginx，按catalog渲染systemd；`release/catalog.py:2–19`没有成熟外部探测／告警组件，且个人Telegram通知模板也不在此固定目录；`release/controller.py:228–234`要求enabled units含api | 未发现隐藏的完整监控包。当前受控发布器不是“仅监控机”安装器；不意味着管理员不能另行手工部署，或未来必须把外部组件放进应用发布器 |

上述条件路径同时核对了outbox推进：`notification_worker.go:94–123`自身调用`DispatchOne`、`Claim`和`Process`；`jobqueue/outbox.go:33–76`直接从PG的`outbox_events`读取并入队，不需要HTTP API代为派发。`notification/telegram_worker.go:255–264`调用传输器后保存结果。以上是静态调用链推导，没有运行故障注入、模拟发送或重新解释历史证明。个人通知的当前授权、同意和绑定检查是现存业务边界，不能为补外部告警而删除。

## 3. 检索范围与缺失证据

已独立检索`app/backend`、`app/deploy`、`app/scripts`和`.github`的公开源码／模板，排除测试和依赖锁内容；关键词包括`Alertmanager`、`alerting:`、`rule_files:`、`receivers:`、`telegram_configs:`、`webhook_configs:`、`pagerduty`、`opsgenie`及`blackbox_exporter`。文件名搜索覆盖原生部署、发布工具及CI中的prometheus／alert／blackbox／monitor。未找到可部署的独立探测规则和接收人路由配置；此结果结合上面的完整发布写集才支持缺口判断，不以单次关键词零命中代替调用链。

另外已读：

- [生产打包工具](../../../app/scripts/build_production_release.py)，84–114行；[原生CI入口](../../../app/scripts/native_ci_release.py)，27及121–127行。后者调用前者，未看到另一个外部告警发布入口。
- [发布bundle清单](../../../app/deploy/release/bundle.py)，108–114行：要求应用二进制／runtime，不能由此推断格式绝对禁止以后加入外部配置；当前构建实际没有复制它们。
- [桥接示例](../../../app/deploy/native/status-bridge.example.json)及host示例：配置采样和平台签名投递，不配置外部收件人。
- [部署运维](../../operations/01-deployment-and-operations.md)，110–114行；[原生发布指南](../../operations/03-native-release-runbook.md)，23、49、86行：要求独立Prometheus／Blackbox和实际安装；指南中的监控JSON指桥接配置，未补独立告警规则。
- [双语入口](../../../README.md)，573／581行，及[英文指南](../../README.en.md)，506、518–522行：明确独立外部告警送达仍在GAP-23，未声称实际完成。
- [生产Nginx](../../../app/deploy/release/catalog.py)，102–116行；[健康入口](../../../app/backend/internal/httpapi/server.go)，55–64行：公开`/health/`为404，根路径可返回静态Web；内部`/health/ready`检查PG结构、`/health/live`恒200。不能直接给外部探测器填公开`/health/ready`，也不能用根页面200证明API或Worker可用。

未读取服务器、已有监控服务商或公司私有部署资料。因此“真实环境肯定没有外部监控”“必须新开发自有告警服务”“必须在平台PG故障时仍送达”“只差填Token就完成”均不能由本审查支持。

## 4. 可进入整批代码波的明确缺口

| 编号 | 应补工程／明确配置 | 与现存能力的关系 |
|---|---|---|
| RF-01 | 选定成熟工具后提供独立监控机原生安装与版本固定入口、目标／探测模块模板、告警及恢复规则、接收人路由、配置校验与部署说明；也可对接已有外部服务，但必须有可审阅的实际接线协议 | 属于部署工程；现有桥接不承担此链路。未提供公司配置时保留占位并禁用真实发送，不伪造部署 |
| RF-02 | 明确整机故障、API不可用、采样断流、监控自身故障的探测信号及接收责任；定义公开且受控的探测契约或可信网络内探测入口，不默认探测当前公开404路径 | 不应重写现有资源unknown投影，也不能只检查静态首页 |
| RF-03 | 明确现有个人TG资源提醒的辅助地位；保留其个人同意、权限与当前事实检查。若选择跨主机Worker辅助方案，另补独立主机安装、PG可达与故障域说明，不能使用强制含api的应用安装器假装已支持监控专机 | 可复用已有调度／发送实现；不需要另写相同资源调度器 |
| RF-04 | 公司待提供：独立监控主机或外部服务、已许可目标／网络、收件人／频道、故障阈值和恢复策略、密钥安全路径、监控自身失联的接收安排 | 属于配置／制度未知，不能静态源码完成，亦不能标作测试失败或通过 |

整批代码和前置用例保存后再统一检查；本次反证没有先行执行任何局部测试。真实独立主机安装、服务商收件及故障／恢复仍须与代码缺口分开列明，不把取消的网络性能、真机或灾备项目重新引入门槛。
