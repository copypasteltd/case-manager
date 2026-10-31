# 153 · 员工 TG 私聊业务提醒 / Personal Telegram business notices

日期：2026-10-31。当前源码结构113；冻结 `v0.1.0-rc.7` 仍为业务151／结构112。本增量属于冻结后的源码更新，不修改七个既有冻结版本。

## 目的与使用方式

此前站内已经支持任务、审批、申请进展、合同、订阅、文件副本、服务事件及秘书结果；TG 出站工作器只处理秘书请求阶段。本增量把已有站内业务事件接到员工主动选择的 TG 私聊，保留站内通知作为处理入口。

1. 员工在账户设置完成既有 TG 双向绑定：本人登录建立挑战、在 Bot 私聊执行 `/start`、回到原登录确认数字身份。绑定完成不会自动开启提醒。
2. 打开四端共用的通知中心，选择“修改本人 TG 提醒”，明确开启状态与通知类型，填写原因，核对确认项并完成最近十分钟内的两步验证。
3. 系统只接收该设置发布后、24小时窗口内的新站内投递。每次修改设置都建立新版本，不补发旧版本之前的历史通知。类型可选讨论提及、任务、待审批、申请进展、合同到期、订阅账期、文件副本、服务事件、秘书结果和主机资源。
4. 工作器只向当前已确认的本人数字身份私聊发送“您有新的公司内部通知，请登录查看。”及固定 HTTPS 通知中心地址；金额、标题、附件、审批正文及资源链接不出站。收到提示后登录系统处理业务。
5. 在通知中心查看本人回执、尝试次数和不可变历史。结果未知时先核对私聊，可填写确切消息号记录“本人核对已送达”、明确重试，或终止投递。人工核对与服务商确认分开显示；均不代表已读、审批通过或业务完成。
6. 关闭提醒、改变绑定、撤销业务权限、已读或失效的事项，会在下一轮准备检查中停止后续外发。进行中的网络请求和已发消息无法召回。关闭此补充渠道不会关闭站内通知或必要责任链。

所有页面使用现有品牌组件，不引入新的风格规范。Web 在通知中心直接展示，共享 Taro 页面覆盖微信、抖音与 Android 源码。真实 Bot、正式平台身份与签名配置由用户后续提供；本增量未发送真实 TG 消息。

## 数据与授权约束

- 迁移113新增个人订阅版本、跨渠道投递和不可变投递日志三张表。1–112不变；保留正常升级流程，未操作原开发库100。
- 设置按组织／本人／Bot递增，开启时固定绑定ID与版本。数据库验证同组织同本人，并禁止改写或删除历史。员工只能查看、修改自己的偏好；组织管理员不因此取得替他人开启提醒的权限。
- 读写都要求当前四端真实人工登录。修改和人工对账要求最近MFA；秘书 Run 与 TG 运行会话不可使用这些入口。Web变更继续要求Cookie、Origin及CSRF；命令继续使用固定幂等键与预期版本。
- 每个站内投递／Bot最多一条跨渠道回执。只选择当前最新、未读、当前类型命中的站内投递；发送前调用站内通知既有的当前资源、责任人、分类及四端渠道规则。取消、失效或撤权不会通过旧通知重新取得内容访问权。
- 准备外发时保存精确提示字节的SHA256、递增尝试和30秒租约，再提交事务进行网络调用。并发工作器共用既有Bot预算，最短间隔3.2秒；五次尝试封顶。只有服务商明确限流才自动延后，网络超时／不确定响应或失联租约进入 `delivery_unknown`，不自动重发。
- `sent` 必须有接口确认的精确数字消息号；`confirmed_sent` 是本人承担的人工核对事实。未知结果重试必须再次通过当前订阅、绑定、用户及业务权限。手工核对、终止及重试保留操作者与原因，不改站内已读或业务状态。
- 站外目的地来自已确认的正数个人身份，不能填入群号或任意收件人。个人提示省略无效的零值 `reply_parameters`；秘书群内回复继续保留既有回复参数。统一Bot API适配器只访问固定Telegram HTTPS端点，禁重定向及超限响应。
- 凭据只在出站工作器环境中，数据库、API返回、模型、审计与媒体不保存Token。新表只保存固定引用、状态、摘要与安全原因码，不持久化业务标题或站外正文。

## 接口

所有地址以 `/v1` 为前缀，响应沿用 `data/meta`；列表使用当前本人权限绑定的游标，每页1–50条。结构详见 [OpenAPI](openapi.json)。

| 方法与地址 | 用途与固定输入 |
| --- | --- |
| GET `/notification-telegram-settings` | 本人Bot设置、当前有效绑定及是否实际生效；默认关闭 |
| POST `/notification-telegram-policies/{bot}` | `expected_version`（首次0）、`enabled`、显式 `categories` 数组及 `reason`；返回新不可变版本 |
| GET `/notification-telegram-policies/{bot}` | 本人指定Bot的设置历史 |
| GET `/notification-telegram-deliveries` | 本人跨渠道回执列表 |
| GET `/notification-telegram-deliveries/{id}` | 本人单条回执及完整版本日志 |
| POST `/notification-telegram-deliveries/{id}/reconcile` | `expected_version`、`action`（`confirm_sent/retry/abandon`）、显式可空 `message_id` 及 `reason` |

`confirm_sent` 仅处理结果未知；`retry` 仅处理未知或失败、且未满五次的当前有效通知。`abandon` 可处理待发、等待重试、未知或失败。发送中及终态不能被覆盖。另一员工的回执按不存在处理；游标不能跨员工使用。

## 原生部署

继续运行既有站内通知工作器：它消费业务outbox并建立站内投递。另增加以下独立进程：

```text
platform worker-notification-telegram
```

使用 [原生服务模板](../../app/deploy/native/copypaste-notification-telegram-worker.service)。它沿用既有 `telegram-delivery-worker.env` 中的数据库配置和 `CP_TELEGRAM_DELIVERY_BOTS`，与秘书群回执共用限速表，但各自保有独立回执与日志。示意字段只说明结构，不提供可用凭据：

```text
CP_TELEGRAM_DELIVERY_BOTS=[{"bot_id":"<numeric_bot_id>","organization_id":"<organization_uuid>","token":"<secret_from_secure_environment>","portal_origin":"https://<company_portal_host>"}]
```

实际环境文件应由服务器管理员安全创建，禁止提交Git、录入聊天或展示在页面。API的既有Bot接收配置与个人双向绑定仍须先完成；仅增加此出站工作器不会自动建立绑定、员工偏好或真实平台域名。

## English usage and implementation

Employees explicitly opt into personal Telegram reminders after completing the existing two-way identity binding. In Notifications, select an enabled state and categories, enter a reason, review the confirmation and provide recent MFA. Binding alone never opts anyone in. Each immutable preference revision applies only to newer in-app deliveries within a 24-hour window.

The independent `worker-notification-telegram` process rechecks the current recipient, consent, binding and ordinary business access before preparing each send. It sends a fixed sign-in prompt and the configured HTTPS Notifications URL to the employee's confirmed private identity. Business titles, amounts, attachment bodies and approval content stay inside the portal. In-app notifications remain available.

The worker shares the existing conservative Bot budget, prepares exact payload hashes before I/O and stops after five attempts. Definite provider rate limits may retry; uncertain outcomes and expired sending leases require explicit human reconciliation. Provider-confirmed delivery and an employee's manual confirmation remain distinct. Neither marks a notice read nor completes its business task. Employees can disable future reminders, inspect immutable policy/delivery history, confirm an exact personal-chat message ID, explicitly retry with duplicate risk, or abandon a pending outcome.

Source schema113 and the four-client source include this feature. Frozen rc.7 remains151/schema112 with development APK152; it does not contain this post-freeze change. Current evidence, retained artifacts and deferred acceptance are separated in the [verification guide](../quality/153-personal-telegram-notifications.md). Actual server, Bot, OSS/R2, model and platform material will be supplied later. Group business broadcast policies and real provider acceptance remain open.
