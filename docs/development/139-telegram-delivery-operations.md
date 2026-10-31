# 增量139 · 组织TG投递运维 / Organization TG delivery operations

## 问题与结果 / Problem and behavior

原入口只允许请求人核对本人投递；管理员无法统一排查其他请求的群通知。本轮新增组织级路由／状态查询与明确人工对账。它覆盖已经生成的通知，不把“没有投递记录”解释为Bot正常；未生成通知的入站调度、模型／Skill、群披露、实际Bot配置和真实发送仍在GAP-07／09／13／15内。

Administrators can now inspect generated notifications across their organization and reconcile an unknown outcome. The console exposes routing/status metadata only. It does not grant conversation content, Telegram membership, model execution or permission to disclose company documents.

## 权限、入口与流程 / Authority and usage

| 角色 | 权限 | 能力 |
|---|---|---|
| TG投递运维查阅人 | telegram.operations.read | 当前组织列表与准确状态历史 |
| TG投递对账经办人 | read＋telegram.operations.reconcile | 上述能力及未知通知人工对账；需近期MFA |
| 系统管理员 | 明确增加上述两项 | 管理同类通知事实；不取得请求正文／模型回复 |

新组织由Bootstrap建立；正常结构升级105为既有组织增加角色和系统管理员动作，并使权限缓存版本失效。通过既有角色授权页面分配到组织范围；本人／项目范围不能满足组织权限。所有读写都要求Web／微信／抖音／Android当前真实人工会话；秘书和TG actor不能调用此运维入口。

四端均从Telegram入口进入“组织TG投递运维”。列表可选状态、群入口UUID，按不可变创建时间／UUID倒序分页；游标绑定当前人、组织权限版本及筛选。更改筛选清空选择，详情读取最新投递头和不可变历史。路由显示原接收群与话题；入口旧／新版本及当前通知开关用于判断范围变化。

只对delivery_unknown操作：先在原群核对实际事实，再选①人工确认已发送（准确正整数消息编号）、②确认未发后重试（另行接受可能重复）、③放弃重发。处理原因1–200字，旧版本拒绝；当前会话／权限在幂等回放之前检查。人工确认是人工事实，不改写原供应商未知回执。

重试仅排队，不在API内发送：原请求仍需有效绑定、同一群入口版本、启用的portal_only通知、有效请求人及组织执行范围；接收不超过24小时，原通知发送次数少于5。原群／绑定／权限后续再变更时，独立worker仍重新核对并停止发送。重试保持原request／delivery／phase／run／channel／bot，不允许重新指向其他消息、生成正文或重置次数。关闭失效通知不会抹去未知历史。

## 服务接口与数据 / API and data

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /v1/telegram/operations | state、channel_id可选；cursor、limit1–100；未知／重复参数拒绝 |
| GET | /v1/telegram/operations/{id} | 同组织投递头＋按版本排序的状态历史 |
| POST | /v1/telegram/operations/{id}/reconcile | expected_version、action、message_id、reason、reviewed_original_channel=true、duplicate_risk_accepted；幂等与CSRF沿用普通命令 |

查询白名单包括内部引用、原群／话题、入口标签、请求人UUID、通知阶段／状态／次数／时间／消息编号及经办UUID。排除request_text、conversation正文、模型回复、Bot密钥、完整发送载荷、自由文本对账原因。原备注仍在不可变记录和原有本人回执中保存；组织级查询不会因此扩大正文权限。

API在同一数据库事务内完成投递版本变更、实际人工经办历史、审计与outbox；任一失败整单回滚。详情锁定投递头后读取历史，避免不一致头／尾。结构105只增加角色／权限及组织、状态、入口分页索引；沿用63的不可变投递事件和发送状态机，1–104迁移不变。

## 执行与限制 / Delivery boundary

应用实现位于app/backend/internal/telegram/operations.go、httpapi/telegram.go、identity/bootstrap.go和migration105；共享规则为packages/contracts/src/telegram-operations.ts，Web为TelegramOperations.tsx，移动为pages/telegram/operations.tsx。

实际认证HTTP与耐久原生PG覆盖四渠道元数据、人工确认、本人正文隔离、权限、筛选、版本、重试、有效期／次数、审计回滚和撤权后幂等；供应商回执、身份及过期时钟为隔离合成数据。实际Web完成查询→人工核对→持久回读；未调用真实Bot，不证明群内已经存在该消息。

Normal installations and upgrades must use schema105 before starting this backend. The persistent development database remains100 and is not silently upgraded. Both frozen tags remain immutable; rc.2 and the retained development APK137 contain business136/schema103, so rebuild/repackage to distribute139. Full regression, mobile browser/native rebuild and real integrations remain deferred under the user's accelerated-freeze instruction. This is complete source for this increment, not full-goal or production acceptance.

[验证与出处](../quality/139-telegram-operations-verification.md) · [全部余项](../delivery/05-functional-closure-139.md)
