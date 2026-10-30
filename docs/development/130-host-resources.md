# Increment 130 — 主机资源监控 / Host resources

## 交付行为 / Delivered behavior

服务详情新增独立的主机资源监控：CPU 近五分钟平均使用率、内存可用／总量、全部已配置文件系统的可用／总量与只读状态。Web、微信、抖音、Android 共用准确的配置、证据和时效规则。端点可用性继续按原探测证据判断；资源告警不会悄悄改变服务可用性或自动创建业务事件。

The four clients expose a separate host-resource card, explicit versioned configuration, immutable observation/configuration history, and original signature verification. An availability probe cannot establish host-resource health. All reported capacities remain exact decimal byte strings.

## 数据与授权

正常升级 `0097_host_resources.sql` 从 schema 96 到 97，新增 `service_resource_sources`、`service_resource_revisions`、`service_resource_observations`。一项服务对应一个资源来源；多个服务可分别绑定同一物理主机。暂未实现跨服务主机资产去重。

配置发布使用现有 `service.read`＋`service.manage`、有效人工会话及最近 MFA；绑定服务版本、准确来源版本和基础修订。项目只读阶段、归档服务、旧版本及撤权均不能发布。修改阈值、公钥、挂载点或启停状态都追加修订。新修订不继承旧指标；旧修订、签名原文和观测均不可更新或删除。应用角色只获来源头 SELECT/INSERT/UPDATE、修订与观测 SELECT/INSERT。

人工查阅持有组织、用户和会话授权锁，按当前服务分类和项目权限重新检查。历史分页游标绑定当前用户、授权与服务范围。AI/TG 不能借资源接收接口伪装采集端；更广的秘书监控工具仍属于 GAP-07/09/12。

## 采集与签名协议

复用原生 `cmd/status-bridge` 的受控 Prometheus HTTP 连接、固定 IP、重定向限制、私密凭据文件、每周期并发／超时、先持久化后发送、准确回执匹配和可恢复序号日志。`kind` 缺省保持旧 Blackbox 行为；`kind: host` 使用新域分隔签名和完整磁盘清单。旧 Blackbox 日志绑定字节保持兼容；主机日志额外固定种类、设备、文件系统类型和挂载点。

参考配置位于 `app/deploy/native/status-bridge-host.example.json`。占位地址、UUID、设备名必须替换为经核对的部署值；私钥和 Prometheus bearer 不上传仓库。Linux 原生部署复用 `copypaste-status-bridge.service`，不引入 Docker、WSL、虚拟机或模拟器。

资源接收端：`POST /v1/resource-sources/{source}/observations`。请求只接受 `application/json` 和单个 `X-CP-Status-Signature`，拒绝 Cookie、Origin、Authorization。签名消息为 UTF-8 `CP-HOST-V1\n`＋小写来源 UUID＋`\n`＋完整原始 JSON 字节。原端点探测使用不同签名域，不能互换。

载荷上限 16 KiB；所有字段必填且不可为 null，不接受大小写别名、重复／未知字段或尾随 JSON。载荷包括：`source_revision_id`、安全整数字符串 `sequence`、`observed_at`、0–10000 的 `cpu_usage_bp`、`memory` 和完整 `filesystems`。总量须为正、可用量不能大于总量；字节值为 0 到有符号 64 位上限的规范十进制字符串。文件系统必须与当前配置挂载点集合完全相等，1–16 项。

相同修订／序号／原文精确重放返回原回执；同序号不同原文返回 409。迟到序号、迟到时间和已过期样本仅保存未采纳事实，不刷新当前指标。未来偏差超过 15 秒拒绝；允许偏差内但仍在未来的当前观测显示未知。停用、换钥、换修订或归档先鉴权，再判断重放，旧签名不能借幂等绕过撤销。

## 固定指标与评估

| 指标 | 固定来源／计算 | 失败规则 |
| --- | --- | --- |
| CPU | `100*(1-avg by(精确目标标签)(rate(node_cpu_seconds_total{mode="idle",精确标签}[5m])))` | 缺少预热样本、空／多结果、NaN／Inf、超出 0–100 均不生成观测 |
| 内存 | `node_memory_MemTotal_bytes`、`node_memory_MemAvailable_bytes` | 缺失、非整数、溢出或可用量大于总量均拒绝 |
| 文件系统 | 每项精确 mountpoint/device/fstype 的 `node_filesystem_size_bytes`、`node_filesystem_avail_bytes`、`node_filesystem_readonly` | 缺任一已配置磁盘或只读值不是 0/1 均拒绝；只读事实本身判严重告警 |
| 抓取新鲜度 | `up=1`，原始指标 `timestamp(selector)`；CPU 原始计数器时间取固定标签分组最小值 | 必须同一实际抓取时间、新鲜且不在未来；查询评估时间不能刷新陈旧数据 |

不接受任意 PromQL、目标 URL、模式标签或磁盘设备标签覆盖。仅查询指定 Prometheus，桥接器不直接探测目标。固定查询语义参考 [Prometheus functions](https://prometheus.io/docs/prometheus/latest/querying/functions/)；原生主机采集器参考 [node_exporter](https://github.com/prometheus/node_exporter)。

每类资源分别配置警告／严重阈值，单位基点，1% = 100 bp。内存／磁盘使用率为向上取整的 `(total-available)/total`，使用整数运算避免溢出和舍入隐藏阈值；CPU 百分比也向上取整至基点。任一严重阈值或只读磁盘判 `critical`；否则任一警告判 `degraded`；全部有效且低于阈值才 `operational`。默认表单 80%/95%、90 秒仅为未发布建议，不代表公司政策。

服务器从采集与接收时间同时计算新鲜度；45–180 秒可配置。断流超时、无观测、停用、归档或时钟异常返回 `unknown` 且 `observation: null`。UI 每五秒重读，单次结果有效期最多十秒，并从请求开始扣除耗时；无单调时钟或到期后隐藏当前数值。历史仍可单独查阅，但显式标记不代表现时状态。

## API 与开发入口

| 方法 | 路径（均为 `/v1` 下） | 行为 |
| --- | --- | --- |
| GET | `/services/{id}/resource-monitor` | 当前来源、权限、资源状态与有效指标 |
| POST | `/services/{id}/resource-monitor/revisions` | 人工 MFA 发布版本固定的完整配置 |
| GET | `/services/{id}/resource-monitor/revisions` | 不可变配置历史，游标分页 |
| GET | `/services/{id}/resource-monitor/observations` | 原始事实／未采纳原因，游标分页 |
| GET | `/services/{id}/resource-monitor/observations/{observation}/evidence` | 当前权限下校验摘要和 Ed25519 签名并返回原文 |
| POST | `/resource-sources/{source}/observations` | 独立签名采集接收，不使用人工登录 |

实现：`servicestatus/resources.go`、`resource_store.go`；HTTP `host_resources.go`；原生桥接 `statusbridge/resources.go`、`resource_journal.go`；共享校验 `packages/contracts/src/host-resources.ts`；Web `HostResources.tsx`；Taro `pages/status/resources.tsx`。API 合同由 `scripts/build_openapi.py` 统一生成。

## 当前边界

本轮完成资源功能链路，不代表目标服务器已安装 node_exporter／Prometheus。真实目标注册、目标 Linux 运行、独立告警通道、资源断流主动通知及跨通道送达回执仍为 GAP-23；本轮不虚构外部告警已交付。资源事件写入审计与 outbox，但未接入站内通知投影。数据保留清理仍受 GAP-05 的受控生命周期约束。

不运行真机、模拟器、实网大文件、网络性能、迁移或灾备演练，不增加 AI 费用控制。原始 23 活跃／5 移出登记保留；本轮只扣除 GAP-23 中已实现的资源采集、持久化、四端显示和历史子项。
