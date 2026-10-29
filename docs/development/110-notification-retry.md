# 通知投递异常恢复 / Notification delivery recovery

增量110补齐GAP-08的站内通知人工重试：经办人核对最终失败任务、填写原因、完成近期MFA并明确确认后，系统追加独立任务，按原始事件重新检查当前接收人。原失败、原输入、已有通知和业务事实继续保留。工作台聚合、合同提醒配置、实际复制故障来源及跨渠道接入验收仍在GAP-08中开放。

Increment110 adds controlled human recovery for failed in-app notification jobs. A new leased child preserves the original terminal failure and immutable event input. The delivery service checks current business responsibility and recipient access again. This does not close the entire notification work package or establish live TG/cloud/production acceptance. AI cost controls remain excluded; secretary authorization, audit, isolation and technical limits remain required.

## 使用流程 / Usage

1. 管理员在“成员与授权”明确授予“通知重试经办”角色，范围选择本组织。现有“通知投递运维”角色继续只有查询权限，不会自动获得重试权；新角色也不会自动绑定任何员工。
2. 员工以自己的账号和MFA登录，从“通知中心 → 投递异常与重试”查看任务。这里展示任务标识、状态、安全错误码和尝试次数，不展示原业务正文、事件payload、凭据或完整诊断。
3. 点击“核对任务与重试”，检查准确任务及固定业务事件。自动退避中的任务继续等待原计划；不支持的事件版本、拒绝事件和无效输入须先处理来源，不能在本页修改输入绕过协议。
4. 对可重试的最终失败任务，填写1至200字原因，勾选核对声明并完成最近10分钟内的MFA，再点击“确认创建重试”。打开详情、完成验证或填写原因均不会自动提交。
5. 页面显示新任务并刷新等待中状态；Web及共享移动页面每3秒读取等待中任务，终态后停止该轮询。“刷新投递记录”可手动重新核对。Web异常和历史列表按30秒刷新。
6. 在“重试历史”查看原失败及新任务。创建回执表示已提交队列；只有实际Worker结果才能说明写入了多少条站内信或略过多少项。站内信写入不代表员工已读，也不代表TG已经外发。

An administrator explicitly grants the organization-scoped Notification Retry Operator role. Open Notifications, inspect a terminal failure, supply a reason, confirm the unchanged-event behavior and provide recent MFA. Review the new job and its worker outcome. A failed child may itself be reviewed for another retry, but each failed parent can have only one child. Keep the original request identity when a network result is uncertain and inspect history before proceeding.

四端沿用已有通知入口：Web为`/notifications`，微信、抖音、Android为现有`pages/notifications/index`。本轮不新增移动路由，保留20页配置。Taro H5是共享业务页面的本地浏览器验证入口，平台真实身份和正式发布继续按GAP-16／19处理；不启用真机、模拟器或虚拟环境。

## 权限和信息范围 / Authority and disclosure

| 操作 | 必须条件 |
|---|---|
| 查询任务及重试历史 | 有效的人类交互会话；本组织`notification.delivery.read` |
| 提交人工重试 | 同时拥有本组织`notification.delivery.read`和`notification.delivery.retry`；最近10分钟MFA；准确任务revision及原因 |
| 实际通知投递 | 当前业务接收人、当前资料权限、有效任务租约和fencing token；不继承经办人的业务访问权 |

运行会话和TG执行会话不能使用人工运维接口。运维权限是明确授予的组织技术元数据权限，不获得项目、合同等业务内容权限。详情及历史仅返回固定字段，重试原因保存在不可变记录及审计中，不通过通用技术回执回显。经办人、组织、会话、授权版本、渠道和当前权限均重新核对；幂等重放也先鉴权。

The read plane contains organization security metadata only. It never grants access to the underlying task or other business content. Generic receipts omit free-form reasons, payloads and session credentials. Recipient selection and final inbox visibility remain governed by the original domain rules, independently of the operator's authority.

## 状态和不可变依据 / State and fixed input

原自动任务保留其既有尝试上限及原退避逻辑。人工重试仅接受无活动租约的`dead`通知任务，创建另一个`notification.deliver.v1`任务，固定原payload及版本，新任务从0次开始、最多自动尝试4次。人工命令不能传入接收人、事件内容、尝试次数、渠道或新凭据。

原任务不会重置为queued，不会清空失败码、改写尝试次数或更新时间。不可变`notification_retries`关联准确父任务、子任务、业务事件、经办人、原因和创建时间；父、子各自唯一，避免并发分叉。同一幂等请求返回同一创建回执；相同父任务的另一条请求不能再创建子任务。

`revision`是任务状态、尝试次数、fence、更新时间、输入摘要及已创建子任务的摘要，只用于并发前置核对，不作为权限或Worker租约。旧版页面、当前权限变化或另一个经办人先行处理时，服务器拒绝新的提交，页面保留失败提示，要求刷新核对。

异常列表仅展示尚未创建后续重试的失败叶节点。原失败仍可通过详情和历史查看；若子任务再次失败，则子任务进入异常列表。不能把“从异常列表移出”解释为投递成功。

Existing thread ordering and the unique business-event/recipient/channel/template delivery key prevent replay from issuing the same effective notification twice. Older events are not reinterpreted as new assignments. Lost access, changed responsibility or a stale business revision may legitimately produce a successful worker result with suppressed deliveries.

## 接口 / API

所有路径以`/v1`为前缀，沿用现有JSON信封、CSRF、幂等键和错误协议。

| 方法 | 路径 | 行为 |
|---|---|---|
| GET | `/notification-delivery-failures` | 当前失败叶节点，保留既有字段及签名分页 |
| GET | `/notification-delivery-jobs/{id}` | 当前组织通知任务、固定revision、重试限制、父子标识及已有Worker结果 |
| POST | `/notification-delivery-jobs/{id}/retry` | 仅`expected_revision`和`reason`；近期MFA；201创建回执 |
| GET | `/notification-delivery-retries` | 不可变重试历史，默认20条、最多50条；创建时间及ID倒序的签名键集游标 |

详情和命令不接受查询字段；列表拒绝未知、重复和错误编码参数。历史游标绑定当前组织、人员、渠道及授权版本。回执固定父任务、子任务、事件、经办人及创建时间；它不包含可变的实时结果。客户端另读任务详情，并核对回执与当前人员、原任务及原事件一致。

迁移85新增表、同组织外键、唯一父子、不可变触发器、固定输入检查及延迟审计／Outbox约束。子任务、重试依据、幂等记录、审计和Outbox在同一事务提交，任一失败全部回滚。应用账号只有SELECT／INSERT，不能修改或删除历史。升级已有组织和全新Bootstrap均创建新角色；不会扩大已有绑定。

## 实现与检查 / Implementation and checks

后端位于`app/backend/internal/notification/retries.go`，HTTP入口为`internal/httpapi/notifications.go`。Web新增`NotificationDelivery.tsx`，共享移动页面新增`pages/notifications/delivery.tsx`。两端共用`contracts/notifications`的固定字段、回执身份、来源和状态一致性校验。全部沿用现有视觉规范和原生文件布局。

```powershell
$env:PYTHONUTF8='1'
$env:PATH=(Join-Path (Get-Location) 'app/.tools/node-v22.23.4-win-x64')+';'+$env:PATH
python app/scripts/verify_notification_retry_backend.py
python app/scripts/freeze_notification_retry_backend.py
python app/scripts/verify_notification_retry_clients.py
python app/scripts/freeze_notification_retry_clients.py
python app/scripts/run_notification_retry_browser.py
python app/scripts/run_notification_retry_browser.py --mobile
python app/scripts/encode_notification_retry_media.py
```

重复检查前先归档同名尝试和准确源码／日志，不能覆盖原失败。H5先完成构建再启动业务fixture，通过仅限本机的临时5190端口转发到实际API；不替换响应，不延长Go fixture存活或数据库清理期限。检查结束释放5189页面和5190转发端口。浏览器使用隔离合成业务、原生PG、真实HTTP及租约投递服务；检查结束关闭所有自建页面、上下文、服务和数据库。全量后端按单个原生批次进程顺序执行，减少本机PG清理争用，不修改耐久设置或业务断言。Android仅原生构建和包检查，Linux仅交叉编译；实际外部接入继续单独验收。

本轮原开发PG出现测试库DROP清理超时，串行重跑亦出现同一问题。诊断记录了重叠窗口中132.018秒的耐久checkpoint；未改动120秒清理期限。浏览器辅助脚本使用另一套临时的原生Windows PG，保留全部耐久设置，并沿用现有ASCII路径适配器处理中文工作目录。`notification_retry_native_cluster.py prepare`创建或核对这套自有临时集群；全部原生测试和页面检查结束后，执行`python app/scripts/notification_retry_native_cluster.py stop`核对无遗留测试库并关闭它。所有连接秘密、临时数据及原始诊断留在忽略路径；不触动原开发PG，不使用Docker、WSL或模拟器。当前辅助脚本按一次完整采集阶段管理这套集群；重复执行须先核对旧归属记录并保留原证据，不能覆盖失败记录。

[全部剩余工作](../delivery/03-goal-gap-assessment.md)保留23个完整工作包及5项移出。网络性能、实网120GB专项、历史数据迁移与容灾、真机验证和AI费用控制不重新成为发布门槛。
