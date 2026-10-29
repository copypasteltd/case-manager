# 当前范围与验收口径 / Current scope and acceptance · 修订99

增量109：GAP-07的审计查询与固定导出已接通四端页面、真实API和原生PG约束；模型／Skill、TG运行和存储配置管理仍未完成。工作包按完整范围计数，保持23 active（14 P0／9 P1）／5 excluded，不把子功能或移出项记为整个工作包通过。见[设计](../development/109-audit-console.md)与[验证](../quality/109-audit-console-verification.md)。

增量108：npm完整审计63→0，7个直接公告包已修补或替换，正式生产依赖冻结与产物安全验收继续保留在GAP-26。当前范围仍为23 active（14 P0／9 P1）／5 excluded；AI仅移出费用控制，授权、审计、执行隔离及技术限制继续保留。见[开发](../development/108-dependency-integrity.md)与[验证](../quality/108-dependency-integrity-verification.md)。

增量105：新增仓库内的 Linux 原生 Codex 执行控制器，覆盖单任务 systemd 隔离、实际内核状态取证签名、私有模型／公司工具网关、取消与 cgroup 清理，以及独立运行时发布目录。模型密钥与证明私钥留在 root 控制器中。真实服务器隔离、Codex／模型互通和八项 Skill 正式评测仍待完成；23个工作包（14 P0／9 P1）保持开放，5项移出不变。

[实现与部署](../development/105-native-agent-runtime.md) · [验证](../quality/105-native-agent-runtime-verification.md)

增量104进度说明：已知未完成分片清理子流程已实现，GAP-05继续保留正式历史版本、已完成草稿及未知孤儿清理；真实云验收仍在GAP-20。范围计数仍为23 active（14 P0／9 P1）、5 excluded。见[开发](../development/104-multipart-cleanup.md)及[验证](../quality/104-multipart-cleanup-verification.md)。

日期：2026-10-28（Asia/Taipei）。本修订记录用户明确的四项范围调整，以及随后将“AI控制”更正为“AI费用控制”的回复。本文与[机器清单](04-current-scope.json)是当前范围依据；与历史PRD、AC／TC、质量门及规划冲突时，以本修订为准。历史实现、失败记录和验证证据保持原状态。

The user excludes real-device verification, network performance tests, data migration/disaster recovery and **AI cost control only**. This revision takes precedence over conflicting historical requirements and release gates. Secretary behavior, Skills, Telegram, execution isolation and authorization remain in scope.

## 1. 四项调整

| 用户要求 | 本轮移出的工作 | 继续保留的工作 |
| --- | --- | --- |
| 不做真机验证 | Android、微信、抖音实体设备安装、冷启动、操作、后台／杀进程、升级和兼容性实测 | Web、微信、抖音、Android四端；类型／单元／API／集成检查，Web／Taro H5流程，双小程序构建，Android原生构建、JVM／JS与APK检查；正式签名和发布配置 |
| 不考虑网络性能测试 | 香港／境内吞吐、延迟、并发余量、跨境基准与实网120GB长任务验收 | 正常接口连通、授权、超时／取消／重试等功能正确性；120GB设计上限、分片／续传及内容校验；不将小样本写成实网120GB实测 |
| 不考虑数据迁移与容灾 | 2TB存量盘点／导入／ACL映射／切换；跨云数据库备份、PITR、一致恢复集、整机／密钥恢复、RPO／RTO及容灾演练 | 生产持久PostgreSQL、建表／结构升级、OSS＋R2双端存储／复制／校验、普通回收站／版本恢复、日常服务故障通知、应用版本发布与回退 |
| 不考虑AI费用控制 | AI价格／币种计费、Token／金额配额、费用预留、供应商账单、差额核对／补偿及费用控制验收 | Codex秘书、8项Skill、股东TG入口、员工身份与业务权限、执行隔离、固定计划／预授权、披露控制、审计、超时和技术资源约束；公司的合同／采购／报销／订阅费用及项目预算 |

不启动本地Docker、WSL、虚拟机或模拟器；浏览器检查结束后释放自建页面、上下文和临时服务。移出真机验证也不授权启用模拟器。

## 2. 重新统计

| 口径 | 数量 | 解释 |
| --- | ---: | --- |
| 稳定编号 | 28 | 保留GAP-01～28，便于追踪历史 |
| 当前待完成 | 23 | P0：14；P1：9；均属当前范围 |
| 移出本轮 | 5 | GAP-14、17、21、22、27 |
| 因本次修订新增完成 | 0 | 范围取消不会记成实现完成或测试通过 |

23项具体剩余工作见[当前编号清单](03-goal-gap-assessment.md)。GAP-05／06／07／08／16／18／19／20／24／25／28等保留项同步收窄适用验收，不继续附带真机、性能、迁移、容灾或AI费用门槛。

GAP-17和GAP-21移出的是原真机／实网大文件专项；大文件功能继续由GAP-04／18／20核验。GAP-25继续处理生产持久化，容灾恢复任务移出。数据库版本结构升级继续属于正常发布工程，不属于本次取消的历史数据迁移。

## 3. 必须处理的代码差异

增量100已解除接单与执行对月度Token额度策略／费用预留的必配依赖。技术用量直接绑定租户与运行记录；旧费用写入接口对有权管理员返回410，历史账本只读。技术资源、并发、身份／权限、取消、超时与隔离要求继续生效。GAP-09仍保留真实Codex接入验收，见[实现说明](../development/100-secretary-execution-without-cost-quota.md)。

Increment100 implements cost-independent admission, execution and technical usage persistence. Legacy cost writes are retired and their archive is read-only. Real model integration remains open; technical and authorization safeguards remain required.

## 4. 当前验收规则

| 类别 | 当前适用证据 |
| --- | --- |
| 业务功能 | 本地原生PG、API／状态／权限／金额测试，Web及Taro H5真实业务流程，源码与构建产物核对 |
| 四端 | Web、微信、抖音、Android分别具备对应代码与构建／包检查；不要求实体设备或模拟器 |
| 文件 | 设计上限及小样本分片／续传／校验／失败恢复的功能证据；实际OSS＋R2配置和样本对象一致性 |
| AI／TG | 实际模型、执行器、已发布Skill、Bot／群的授权业务闭环；不要求费用配额／计费验收 |
| 部署 | 目标服务器、持久PG、API／Runtime／Worker、HTTPS、秘密隔离、就绪及应用发布记录；不要求容灾或历史迁移 |
| 既有AC／TC | 逐项标记适用、部分适用或移出；适用部分仍需实际证据，移出部分不记为pass |

功能连通性、安全访问限制和故障处理仍需正确；网络性能专项取消不代表接口可以不可用。公司财务与项目预算继续属于核心业务，AI费用控制取消不影响这些模块。

## 5. 证据与后续维护

修订99当时的功能源码和实际检查基线为增量98：Web135、移动248、运行时51；Go189顶层／1183结果；双小程序各20页；Android及JVM缓存／执行范围保留原记录。修订99只更新规划、门禁和文档约束，不把这些结果称为本次重跑。

修订99当时原97 PNG／35 MP4／35 GIF保持原字节、无新采集。增量100另行记录当前源码回归及新增演示，不改写原证据。见[修订验证](../quality/99-scope-revision-verification.md)。后续提交继续同步中英文README、台账、差异表、受影响文档及更新说明，并正常推送授权仓库。
