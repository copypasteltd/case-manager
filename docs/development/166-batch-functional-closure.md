# 增量166：完整代码轮次 / Complete code wave

遵循用户本轮要求：先核对23个活动编号的全部实现位置与未定事项，完成所有本轮可明确实现的应用、四端消费者、schema、配置、用例、执行/媒体工具和文档，再统一测试；汇总失败后集中修复，复测受影响部分。外部资料和公司制度决定仍保留，不以虚构配置或固定演示结果补足。

## 整轮开发范围

| 编号 | 完整代码交付 |
|---|---|
| C01 | schema131项目结项按订阅固定IANA时区计算取消日；安全投影准确时区、核查日、取消日及日期条件；Web与共享移动端显示，历史缺失不补算 |
| C02 | schema132通知来源与有效预授权run统一；三个普通阶段和排队组织版本复核；保留独立人工披露、不可变历史和共享发送预算 |
| C03 | 原生独立Prometheus/Blackbox/Alertmanager/SSH/node工程、官方版本摘要锁、固定公网目标、unknown与watchdog规则及完整安装说明 |
| C04 | 受控发布个人TG worker、三类8192字节私密配置与PID1凭据接入；严格普通环境文件不放宽 |
| C05 | 原生HTTP/PG日期与TG三阶段/组织失效用例、私密文件负例、发布状态机用例、监控输入及真实工具规则用例 |
| C06 | 整轮源码ZIP、统一批量执行/分项结果、实际Web/H5演示、截图/MP4/GIF及所有自建资源清理 |
| C07 | 23编号完整处置、设计/操作说明、双语README与更新记录、源绑定证据与普通推送 |

新增131/132为正常结构升级，历史schema1–130、九次冻结、全部旧证据/素材保持。开发PG100/API不升级；APK164/business163/schema129保留。当前源码变更不冒充新的Android JS构建或APK。

结项当前预览按业务日固定依据，跨当地午夜后需重新读取/确认；不要将准确业务日替换为每次变化的精确时间戳，否则无法完成同一预览复核。历史归档保存原始日历元数据，不读取今日日期补造历史。日期条件满足只是一项结项条件，供应方凭据、完整账期/费用等仍按原规则处理。

## 全部23编号处置

| 编号 | 业务包 | 本轮代码处置 | 正式验收 |
|---|---|---|---|
| GAP-01 | 项目完整管理 | C01 fixes subscription-local cancellation dates in complete project closeout; explicit calendar metadata shared by all clients. | 仍开放 |
| GAP-02 | 合同、采购、报销 | Current source retained. Company decisions, actual credentials/configuration, explicitly bounded wider support and formal acceptance stay separately open; no inferred completion. | 仍开放 |
| GAP-03 | 费用、订阅与报表 | C01 uses the same pinned subscription timezone and date semantics as ordinary subscription transitions. | 仍开放 |
| GAP-04 | 文件正文、预览与检索 | Current source retained. Company decisions, actual credentials/configuration, explicitly bounded wider support and formal acceptance stay separately open; no inferred completion. | 仍开放 |
| GAP-05 | 文件生命周期 | Current source retained. Company decisions, actual credentials/configuration, explicitly bounded wider support and formal acceptance stay separately open; no inferred completion. | 仍开放 |
| GAP-06 | 账号生命周期 | Current source retained. Company decisions, actual credentials/configuration, explicitly bounded wider support and formal acceptance stay separately open; no inferred completion. | 仍开放 |
| GAP-07 | 统一后台管理 | C02 fixes exact activated-grant notification projection without widening group prose disclosure. | 仍开放 |
| GAP-08 | 工作台与通知 | C02 stops grant notices after original organization authorization changes; C03 adds independent infrastructure alerts. | 仍开放 |
| GAP-09 | 真实Codex秘书接入 | Current source retained. Company decisions, actual credentials/configuration, explicitly bounded wider support and formal acceptance stay separately open; no inferred completion. | 仍开放 |
| GAP-10 | AI原生执行隔离 | Current source retained. Company decisions, actual credentials/configuration, explicitly bounded wider support and formal acceptance stay separately open; no inferred completion. | 仍开放 |
| GAP-11 | AI计划与授权 | C02 preserves exact human grant origin in all generic notice insert/authorization guards. | 仍开放 |
| GAP-12 | 秘书业务工具 | Current source retained. Company decisions, actual credentials/configuration, explicitly bounded wider support and formal acceptance stay separately open; no inferred completion. | 仍开放 |
| GAP-13 | 八项Skill正式发布 | Current source retained. Company decisions, actual credentials/configuration, explicitly bounded wider support and formal acceptance stay separately open; no inferred completion. | 仍开放 |
| GAP-15 | 股东TG入口 | C04 completes fixed personal Telegram release service and bounded per-service credentials; actual Bot/server configuration remains deferred. | 仍开放 |
| GAP-16 | Android构建与发布 | Current source retained. Company decisions, actual credentials/configuration, explicitly bounded wider support and formal acceptance stay separately open; no inferred completion. | 仍开放 |
| GAP-18 | 四端业务覆盖 | C01 safe calendar evidence is present on Web and shared mobile; no current APK is claimed. | 仍开放 |
| GAP-19 | 微信／抖音平台接入与发布准备 | Current source retained. Company decisions, actual credentials/configuration, explicitly bounded wider support and formal acceptance stay separately open; no inferred completion. | 仍开放 |
| GAP-20 | 实际OSS＋R2 | Current source retained. Company decisions, actual credentials/configuration, explicitly bounded wider support and formal acceptance stay separately open; no inferred completion. | 仍开放 |
| GAP-23 | 服务监控与告警 | C03 adds missing independent Prometheus/Blackbox/Alertmanager/native SSH/node exporter configuration, rules and delivery wiring; actual separate host and receiver acceptance open. | 仍开放 |
| GAP-24 | 生产部署与发布工程 | C04 completes fixed personal Telegram release service and bounded per-service credentials; actual Bot/server configuration remains deferred. | 仍开放 |
| GAP-25 | 生产数据持久化 | Current source retained. Company decisions, actual credentials/configuration, explicitly bounded wider support and formal acceptance stay separately open; no inferred completion. | 仍开放 |
| GAP-26 | 依赖安全与产物 | C03 pins four upstream native archives and checksum validation; formal target artifacts remain open. | 仍开放 |
| GAP-28 | 当前范围完整验收 | C05/C06/C07 complete the whole declared code/fixture/tool wave before selected validation; formal full23 acceptance remains open. | 仍开放 |

当前计数仍23项（14P0/9P1）开放、5项移出、八个Skill均draft。代码处置不是完整验收通过；真实云/平台/Bot/Codex/Linux/正式签名配置由用户后续提供。

## 验证执行约束

开发完成后先保留完整源码清单/摘要/ZIP和全部7项状态，再执行统一native、工程与客户端批次。初始失败记录不覆盖；先汇总再集中修复，只重跑受到修改影响的步骤。源字节变化必须重绑证据；既有121全回归、163 runtime66和164APK均如实保留。禁止本地Docker/WSL/虚拟机/模拟器，不做真机、网络性能/实网120GB或迁移容灾测试，不加入AI费用控制。所有浏览器/上下文/临时服务/原生临时PG均按所有权关闭。

验证状态：完整代码先封存，随后统一检查、集中修复。原生4族40条结果、Web527/共享移动553、7客户端步骤、发布42与监控原生工具及Web/H5通过；全部21项当前源核对通过。首次失败和证据工具修复见[验证记录](../quality/166-batch-functional-verification.md)，保留全部边界。

关联：[独立监控操作](../operations/26-independent-monitoring.md)、[TG受控发布](../operations/27-telegram-controlled-release.md)、[整轮计划](166-batch-code-plan.json)、[审查与反证](166-audit/archive-journey.md)。

English: implement all seven declared code/fixture/tool tasks before unified validation. Subscription-local closeout calendars, exact grant delivery origins and authorization checks, independent native monitoring and private Telegram release credentials are one coherent code wave. All23 formal packages, real integrations and eight draft Skills remain open. Native Windows parser/rule checks, synthetic browser media and portable process doubles do not establish real Linux deployment, physical independence, delivery, signing or production readiness.
