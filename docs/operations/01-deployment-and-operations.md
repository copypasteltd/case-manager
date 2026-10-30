# 原生部署、配置、发布与日常运维手册

增量130已实现主机CPU／内存／完整配置磁盘的原生采集桥接、签名接收、不可变记录、四端显示与配置／历史管理。真实目标安装、资源断流主动通知和独立告警送达继续归GAP-23；不以页面告警替代送达回执。[设计与使用](../development/130-host-resources.md) · [实际检查](../quality/130-host-resources-verification.md) · [全部23项](../delivery/05-functional-closure-130.md)。23 active（14 P0／9 P1）、5 excluded保持。

增量119增加可执行的完整原生打包、固定服务目录、版本化配置、发布协调器及手动CI。安装、切换和故障处置以[完整运行手册](03-native-release-runbook.md)及[设计](../development/119-native-production-release.md)为准；真实服务器、系统服务、HTTPS、CI和持久PG尚未验收。下方历史章节的迁移／容灾及AI费用门槛继续按[当前范围](../delivery/04-current-scope-and-acceptance.md)移出，不恢复这些旧门槛。

增量105原生执行控制器：Linux root控制器与公司任务Worker分离，专用发布目录、私有模型密钥、实际内核取证和cgroup回收的安装顺序见[详细手册](../development/105-native-agent-runtime.md)。本机仅完成可移植检查和交叉编译，未启用目标服务；范围继续遵循[修订99](../delivery/04-current-scope-and-acceptance.md)。

修订99适用边界：[当前范围](../delivery/04-current-scope-and-acceptance.md)优先。保留API／Runtime／Worker／HTTPS、生产持久PG、秘密隔离、正常数据库结构升级、应用发布／回退与故障告警；不要求真机、网络性能、历史数据导入、PITR／容灾或AI费用策略。旧部署／恢复章节中对应专项不再作为当前发布门槛。

版本：v1.0-draft；日期：2026-10-23；服务器信息待 Q-01。本文提供待执行手册，不表示已经部署、开放端口或创建云资源。

## 1. 部署拓扑与账号

首期应用主机运行 HTTPS 入口、Go API、Go Worker、Node Runtime 的独立原生服务。独立 PostgreSQL 保存全部业务状态，OSS/R2 保存文件/备份，独立外部探测负责主机不可用告警。操作系统候选 Linux；若实际为 Windows，应另补原生服务管理与等效权限方案，不能擅自引入 WSL/Docker。

| 身份 | 必需权限 | 必须隔离的权限 |
| --- | --- | --- |
| API | 业务 DB、签发指定 draft 凭证/下载授权 | DDL、云管理、任意正式对象覆盖 |
| Worker | 分任务 DB/存储读写、复制/解析/通知 | 宿主机管理员、无限制外发 |
| Agent Runtime | 模型身份、限时业务工具、任务临时目录 | PG管理、主/副桶管理密钥、部署/签名密钥 |
| Migrator | 受控 schema 迁移 | 日常 Web/模型入口不可获取其凭据 |
| Backup | 备份读权限、备份目标写 | 普通业务内容批改、删除现有所有恢复集 |
| Cleanup | 已获批准范围的对象删除 | 与普通上传身份分开，生产默认停用直至保留规则确认 |

首次系统管理员通过受控一次性 bootstrap 创建，不设默认 admin/admin。bootstrap 完成禁用，开启 MFA，恢复资料独立保管并演练。

## 2. 配置清单与秘密

配置键是设计占位，精确参数名随工程冻结；值从密钥服务/受限文件读取，不在源码、Web 环境变量、日志或文档中填真实内容。

| 配置域 | 必须具备 |
| --- | --- |
| 基础 | 环境名、对外域名、可信代理、TLS、允许 Origin/平台域名、业务时区 |
| PG | TLS DSN/服务身份、连接池上限/超时、schema兼容版本、备份身份 |
| OSS/R2 | endpoint/region/bucket、用途身份引用、签名TTL、能力探测、主存储epoch |
| 安全 | 会话/MFA/内容加密密钥版本、CSRF、速率限制、最小授权策略 |
| AI | 核准端点/认证引用、Runtime/Skill版本、资格放行、预算、工具网络策略 |
| TG | 已实现入站 `CP_TELEGRAM_INGRESS`（默认disabled）；独立secret、Bot及组织；白名单chat/topic由受限门户配置。Bot token／外发和披露仍待接通 |
| 任务 | 队列/并发/租约/重试、临时盘配额、解析格式与超时 |
| 可观测性 | 脱敏日志目标、指标、告警接收责任链、外部探测 |

启动区分 liveness（进程活着）、readiness（能提供当前业务能力）、模块 capability。模型不可用不应让手工项目/OA readiness 失败；PG 不可用不能伪装业务写入正常。公网健康接口只返回必要状态，详细依赖在受限监控面。

Telegram入站的实际键、上限、HTTPS路径和身份流程见[增量60](../development/60-telegram-binding-and-ingress.md)。`app/.env.example`仅含disabled占位；不得把Webhook secret放进Vite／Taro变量或URL。实际setWebhook、允许更新类型、Bot权限与成员变化覆盖由运维在已获授权的真实环境验证；当前开发配置保持关闭，没有执行这些外部操作。增量60先接通持久化；增量61已增加受控TG任务调度。当前未启用真实模型与发送链路，不能将HTTP或运行成功解释为群回复成功。

## 3. 首次部署步骤

1. 核对 Q-01/03/09：资源、区域、网络、存储、PG与密钥保管；记录真实香港访问链路。配置原生服务账号、目录、文件权限、网络 allowlist 和时间同步。
2. 创建隔离数据库/存储前缀，初始化最小权限，验证两云能力与备份写路径。应用只绑定可信入口；DB/Runtime/指标端口不直接公网开放。
3. 部署与 manifest 匹配的冻结二进制/静态包/Runtime，校验摘要；迁移前备份，执行受控迁移并核验。
4. 填必需配置，创建一次性管理员/MFA，邀请测试用户；配置流程、分类与预算。未确认模块保持禁用，页面可见原因。
5. 运行原生集成、授权/审批/小文件/大文件、通知及恢复演练；不使用生产真实付款或对外群发来做 smoke test。
6. 配置外部探测、备份周期与失败告警；验证主应用停机仍告警。记录配置/账号所有者与操作手册访问方式。
7. 按迁移手册分批导入，四端提审/签名/安装验证，完成质量门后正式切流。

## 4. 发布与回滚

每次发布先出 release manifest、数据库差异、配置差异、平台兼容、Skill变化、测试证据、回滚触发。单节点发布采用短维护/受控流量切换，已有上传直传会话仍需可恢复；暂停新增高影响任务，保留已提交结果。

数据库只做向后兼容 expand 时可回滚旧二进制；存在不可逆数据变换时必须说明前向修复路径，不承诺“换回包就还原数据”。旧 App/小程序尚在用户端时后端继续兼容。Runtime/Skill 独立发布可只关写入，不停基础 OA。

当前单主机代码采用精确迁移清单：API和上传Worker启动时通过只读函数核对已应用迁移名称及摘要，readiness也核对；应用没有迁移台账直接读取/写入权限。发布时先停止旧进程再迁移/启动。即使只有新增迁移，旧二进制也会因清单不符拒绝启动，回滚包需包含经过验证的匹配清单或执行前向修复，不能直接跨迁移换回旧包。readiness不能物理停止已经运行的旧Worker，发布顺序仍需执行。

Android 签名密钥独立加密保管与恢复授权；旧版本升级测试不换签名。微信/抖音保留平台审核/发布记录、可用回退包与账号负责人，平台审批时长不纳入固定开发保证。

## 5. 日常指标与告警

| 频率/责任 | 检查内容 | 触发处置 |
| --- | --- | --- |
| 实时/运维 | 外部可用性、PG错误/连接、磁盘、租约/队列、worker死信 | 关键接口不可用立即事件；积压按年龄与大小分级 |
| 实时/资料负责人 | 副本失败/缺失/长时间未验证 | 确认风险窗口，修复链路/重跑；不显示已备份 |
| 每日/运维 | 最新可恢复点、备份校验、密钥可得、临时目录配额 | 超RPO/备份失败为高优先，不能只看job绿色 |
| 每日/业务 | 待审批、订阅到期、合同节点、付款异常 | 交给实际负责人；不默认全员看到金额 |
| 每周/负责人 | 费用/流量/模型用量、版本增长、权限与TG范围变化 | 预算偏差、失效权限、群披露复核 |
| 每月/运维 | 恢复抽演、依赖补丁、账号恢复、清理dry-run | 无恢复证据不继续扩大自动清理 |

日志记录原因码/关联ID，不存秘密/合同原文；清理临时目录只针对已解析在 task 根目录内的非活动资源，不对拼接路径递归删。缓存、派生物、multipart 与正式历史采用不同清理规则。

## 6. 安全与故障常用动作

发现越权/泄露：暂停相关工具/渠道写入与披露→撤会话/凭证→保存脱敏证据→核定影响资源与收件范围→修复并回归→负责人放行。不能为取证继续发敏感正文到TG。

应用盘满：停止新增解析/报表等可暂停任务→按活动租约核查临时文件→清理有清单的过期缓存→扩容/限流→恢复。不得删业务 DB、签署件或唯一恢复资料“腾空间”。

模型/TG链路不可用：任务显式排队/失败，保持人工工作；修复后查询已发送/已提交状态再重试，避免重复业务或群消息。

## Telegram dispatcher 部署边界（增量61）

迁移至0060后可单独部署 `platform worker-telegram`，使用 `app/deploy/native/copypaste-telegram-worker.service` 和 `/etc/copypaste/telegram-dispatch-worker.env`。环境只提供应用数据库、运行环境和 `CP_TELEGRAM_DISPATCH_BOTS` 的组织／Bot身份；不得复用API的Webhook secret、对象存储或模型凭据。进程只调度持久请求，不监听公网、不调用Bot或模型。

真实项目简报执行另需现有agent executor、正式发布的Skill、不可变配置中的普通／TG独立SDK哈希及有效原生隔离证明。当前开发配置关闭TG，8个Skill仍draft、活动运行配置0；没有在服务器启用新service。上线前须实测香港到境内API、TG到HTTPS入口及执行环境访问，并补齐Bot披露／送达流程。未满足配置时请求30秒后重查，接收30分钟后撤回；积压与到期应由运维在实际环境监控。详见[实现与状态约束](../development/61-telegram-agent-dispatch.md)。


## 服务事件摘要部署增量

[增量62](../development/62-agent-service-summary.md)新增迁移0061和不可改写服务摘要。原生 API 应用角色仅获得快照 SELECT／INSERT；部署后运行 `python app/scripts/verify_service_summary_development.py` 核对所属开发环境。不要将其用于未匹配的生产连接。事件摘要源码仍 draft；实际模型配置安装、隔离证明、真实监控采集及 TG 收发仍需分别验收。沿用既有原生 TG 调度服务，无新增外发 worker。

## 增量63运行约束

原生开发环境应用第62项迁移，新增财务快照三重数据库保护；仅更新本任务拥有的 API 与通知 worker。订阅审阅运行配置仍须按正式发布流程安装，不因只读工具齐备自动启用模型、付款或 TG。保留旧快照及授权目录，不作数据库人工覆写；参考[月度财务实现](../development/63-agent-finance-summary.md)。


## 增量64：独立 TG 发送进程

新增 `worker-telegram-delivery` 与[原生服务模板](../../app/deploy/native/copypaste-telegram-delivery-worker.service)。使用独立受限环境文件配置 `CP_TELEGRAM_DELIVERY_BOTS`；接收、入站调度和模型进程不共用发送 Token。发送依赖全部 63 项迁移、固定 HTTPS 门户及显式启用的原群／话题。默认关闭；模板未部署，当前开发环境未启动真实发送。

值守时按逐条回执区分 sent、delivery_unknown 与人工 confirmed_sent；未知不自动重发。恢复后先等过期发送准备进入未知，再由请求本人对账，不清空队列或改写历史。429 冷却存于数据库，扩进程不能绕过。群迁移需要复核新入口，不自动换群。详细[运行与恢复契约](../development/64-telegram-delivery.md)。

## 原生监控桥接增量81 / Native monitor bridge

监控机独立运行成熟 Prometheus／Blackbox；平台 API 保存来源修订、公钥及签名事实。先升级包含完整观测回执的 API，再按[完整桥接部署协议](../development/81-native-prometheus-status-bridge.md)部署独立二进制，填写[配置模板](../../app/deploy/native/status-bridge.example.json)，审阅并按实际环境安装[原生服务模板](../../app/deploy/native/copypaste-status-bridge.service)。此桥接不取得业务数据库或云存储主密钥，也不执行门户展示地址。journal 是专用监控进程的持久协议状态，不能当成可清理缓存；更换标签、端点或签名身份须发布新来源修订。

Run mature Prometheus/Blackbox independently. Upgrade the platform API before enabling the bridge, then follow the linked protocol and review the private configuration/service templates for the actual monitor host. Preserve the exclusive durable journal across restarts and uncertain deliveries. A changed identity, endpoint or label binding requires a new source revision. Native Linux builds are compile checks; no remote installation or actual independent alert delivery is claimed by this repository evidence.
