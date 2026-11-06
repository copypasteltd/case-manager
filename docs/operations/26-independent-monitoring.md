# 独立原生监控 / Independent native monitoring

状态：增量166代码工程；真实目标、独立主机、接收人、watchdog与送达验收待配置。生成、配置检查、规则模拟不代表实际部署。

主应用仍使用现有status资源采集、连续故障/恢复与站内待办；本工程提供独立Prometheus、Blackbox、Alertmanager、SSH隧道及应用主机node_exporter。平台API故障时，部分既有PG后台通知仍能运行；整台应用主机故障时，由另一台主机上的本工程告警。监控主机故障时由第三方watchdog检测心跳停止，不能由监控机自己证明存活。

## 配置与边界

1. 在仓库外复制 `app/deploy/monitoring/example.json`，替换真实部署标识、不同的主机标识、HTTPS目标及人工确认的公网IP。示例声明不证明物理独立；示例域名和地址不得作为真实可用性验证。
2. 每个目标只连接固定公网IP，保留原HTTPS Host/SNI，拒绝跳转、私网地址、认证信息、query和fragment。地址变化需要重新人工审阅配置；DNS解析不能自动改写获准目标。
3. 应用API仍只监听127.0.0.1:8080；公网nginx的 `/health/` 继续404。监控侧SSH转发127.0.0.1:18080到API，19100到node_exporter。`/health/ready`检查数据库和普通schema版本；仅探测首页不能替代它。
4. 监控组件监听监控机127.0.0.1的9090/9093/9115。应用node_exporter监听应用机127.0.0.1:9100。保持防火墙禁止公网访问这些端口。
5. SSH专用账号仅允许本机API/node两个端口转发，禁用PTY/agent/X11，使用固定主机公钥。私钥与known_hosts由PID1 LoadCredential读取，不在命令参数写入令牌。
6. 运营通知可选Telegram或HTTPS webhook；watchdog必须单独配置。私密文件只允许 `/etc/copypaste-monitoring/secrets/` 固定文件名；bot token、chat id与URL各自一文件，不进入生成包、Git、截图或日志。Telegram消息仅部署标识、告警类型与状态，不包含业务、服务器指标正文或任务内容。第三方webhook接收基础设施标签，收件服务与权限须由运维审阅。

## 生成与安装（仅目标Linux操作指南）

使用原生Python3，不使用Docker/WSL/虚拟机。运行命令前用真实配置替换路径；以下不会由本地验收自动执行。

```sh
python3 app/deploy/monitoring/monitoring.py --config /secure/monitoring.json --output /secure/reviewed-monitoring
python3 app/deploy/monitoring/fetch_components.py --platform linux-amd64 --cache /secure/native-archives --output /secure/monitoring-bin
```

版本与官方摘要固定在 `app/deploy/monitoring/components.lock.json`：Prometheus3.16.0、Alertmanager0.34.2、Blackbox0.29.1、node_exporter1.12.1。下载器核对完整压缩包摘要，只取固定目录中的指定普通二进制及许可证文件，不自动执行或启动。保留components.json与生成manifest.json，复核每个文件摘要；安装时携带许可证与NOTICE。升级要更新锁文件、复核官方摘要并重新验证，不能替换同版本文件。

应用主机：创建无登录的 `cpmon-node` 系统用户/同名组；将node_exporter以root所有、0755安装到 `/opt/copypaste-monitoring/bin/`，安装 `application-host/copypaste-monitor-node.service` 到 `/etc/systemd/system/`。监控主机：分别创建 `cpmon-prometheus`、`cpmon-blackbox`、`cpmon-alertmanager`、`cpmon-tunnel` 用户/同名组；安装对应二进制到相同bin路径；仅安装 `monitor-host/` 下四个unit。所有安装目录root所有且不可被服务账号写入。

把生成config四文件安装到一个新的root所有0755目录，例如 `/etc/copypaste-monitoring/configurations/monitor-001`，非秘密配置0644，再将 `/etc/copypaste-monitoring/current` 指向该目录。生成暂存包本身0700/0600，不能原样复制成服务不可读的配置。`/etc/copypaste-monitoring/secrets` root所有0700，凭据文件root所有0600，由systemd分别交付给指定用户；不要把所有凭据改成可被服务组共同读取。

使用安装后的promtool检查Prometheus配置和规则，amtool检查Alertmanager，Blackbox `--config.check`检查探测配置。必须先在目标Linux审阅 `systemd-analyze verify`、cgroup/BPF地址限制、DBus/systemd采集权限、隧道固定端口与SSH服务端限制；Windows配置检查没有执行这些能力。经运维部署审阅后才daemon-reload并启动本机对应units。不要在同一主机误启用两组角色，不要覆盖用户已有服务。

## 规则、恢复与运营

60秒采样/求值；连续失败表达式维持2分钟触发EndpointDown。缺少3分钟观察触发ObservationUnknown，不把缺数据记为健康。必需应用unit非active触发RequiredUnitDown，单位指标缺失触发RequiredUnitUnknown。组件抓取失败触发MonitorComponentDown。Watchdog规则常驻，每分钟向独立接收器发送，接收器须另设失联期限（建议至少5分钟并由运营确认）。配置真实阈值、通知静默、值班人员前不得标为正式接管。

Alertmanager发送resolved仅说明规则表达式恢复，不等于平台连续两次成功恢复、业务责任完成、人工工单关闭或实际通知确认。平台原有连续恢复规则不改变。外部服务默认重复提醒30分钟；网络失败由Alertmanager维护重试，不等于已送达。Prometheus本地TSDB保留15天/2GB，只是监控数据，正式公司文件仍存OSS＋R2。

真实验收应包括应用HTTP停止、PG不可用、应用主机断开、SSH/采集断流、监控主机停止、恢复及接收回执。用户已排除网络性能测试、真机验证和容灾迁移，不将这些重新作为门槛；以上是正常服务可用性/送达核对，待用户提供配置。当前不会发送真实TG消息或探测真实公司服务。

## English operation summary

Generate and inspect a new private bundle, fetch only pinned official native components, and install the node role on the application host and four monitoring roles on a different host. Keep configuration readable but immutable to service accounts; keep credentials root-private and deliver them through systemd LoadCredential. API readiness and node metrics use pinned SSH loopback forwards. Public HTTPS probes use reviewed fixed public IPs with strict TLS/SNI and no redirects. A separate watchdog receiver observes monitor failure. Local native parser/rule checks establish neither Linux service operation nor actual host independence, target health or delivery. Rule resolution is not business incident closure.

Primary references: [Prometheus configuration](https://prometheus.io/docs/prometheus/latest/configuration/configuration/), [Alertmanager configuration](https://prometheus.io/docs/alerting/latest/configuration/), [Blackbox configuration](https://github.com/prometheus/blackbox_exporter/blob/v0.29.1/CONFIGURATION.md), [systemd credentials](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html).
