# Increment125 · 批准业务登记实际费用 / Recording actual business expenses

合同、采购、报销详情现在可以直接进入实际费用登记。员工明确核对实际日期、金额、供应方、固定凭证和分摊，重新验证本人后，一次完成费用草稿、费用确认和准确业务关联；任一步失败，整笔操作回滚。审批金额仅作为关联上限，不能据此自动制造已发生费用或银行付款。

## 操作顺序

1. 打开已批准采购、已批准报销，或已生效／完成／终止且仍以准确批准修订为依据的应付合同，选择“登记实际费用并关联”。需同时具备业务读取、费用读取、登记及确认权限。
2. 页面重新取得当前业务版本、准确批准修订和摘要。费用归属与资料分类固定为来源范围，币种固定为来源币种；只预填描述性标题。日期、实际金额、供应方、凭证和分摊由本人填写。
3. 选择供应方的准确修订和至少一份正式文件版本。分摊可含多个可写项目与公司份额，同一范围仅一行，精确合计须等于实际费用原额。
4. 填写原因和永久来源标识，明确确认“实际已发生”与“已核对准确批准来源”，完成近期两步验证，再确认提交。修改事实字段后，两个核对选项重新确认。
5. 成功后进入已确认费用详情，保留普通费用的三条操作历史：草稿创建、确认、新增业务关联。报表立即读取实际费用和份额；付款、核销及履行结项继续使用各自明确操作。

示例：三个独立批准业务各登记 HKD25.01，其中项目20.00、公司5.01。发生额合计75.03，项目份额60.00、公司份额15.03。批准上限100.00仍属于业务依据，不能计作另一个100.00的费用。

## 当前接口

| 接口 | 输入与结果 |
|---|---|
| `GET /v1/cost-business-sources/{kind}/{id}` | `kind`仅限`contract`、`purchase`、`claim`；不接受查询参数。返回当前版本、准确批准修订／摘要、标题、范围、分类、币种、批准上限和快照时间；不建立费用、余额预留或将来写入授权。 |
| `POST /v1/costs/from-business` | 普通`CostContent`、固定`source`、永久`source_key`、原因及两个明确核对布尔值；不允许覆盖`project_id`或`classification`。成功返回201、版本3的准确已确认费用，含一条正常来源关联。 |

`source`包括`type`、`id`、`version`、`revision_id`、`content_hash`。付款不作为此接口来源，订阅账期沿用原有独立费用关联与人工核对流程。公开契约与生成类型保持一致，见[OpenAPI](openapi.json)。

## 原子事务与金额规则

服务协调器使用现有`Commands.Tx`组合普通费用`Create`、`Confirm`和`Link`。每个子命令继续执行原权限、身份、近期MFA、当前项目、正式凭证、永久来源、金额、幂等、审计和Outbox检查；子命令不自行提交。三条普通费用历史、一份费用修订、固定文件引用及四条审计／Outbox事件在最外层事务同时提交，或者全部撤销。

外层幂等标识与固定请求体确定三个独立内部幂等标识。相同请求重试返回原费用；同一标识换请求被拒绝。当前权限与真实会话的MFA在幂等重放前重查，不能用客户端时间、伪造内存MFA或Agent身份进入人工确认。

锁顺序沿用普通财务操作：组织与用户授权快照、排序后的项目、业务来源。重新锁定来源后比较当前业务头版本及准确修订摘要。采购下单等执行版本变化也要求重新打开并核对，不可继续使用旧依据。现有来源金额约束同时覆盖普通多步操作与新组合操作；两个独立操作争用同一100.00上限时，两笔60.00不能同时成功。

预览显示批准上限，不披露其他不可读费用的汇总或承诺“可用余额”。最终来源额度检查可能在费用创建与确认之后失败，事务必须撤销这些中间写入和幂等记录。预览不预留任何金额。

## 四端行为与恢复

Web详情打开固定来源费用表单，共享Taro业务详情进入同一来源的财务操作页面，覆盖微信、抖音和Android源码。服务端始终重新授权，入口显示只提供操作提示。未知业务、其他组织、只读用户、Agent执行身份、无效范围覆盖、过期依据、撤权和MFA不足均不能通过确认。

共享移动端仅保存白名单文字草稿，按用户／授权、来源类型／ID、版本、修订及摘要隔离。凭证、供应方引用、MFA和两个核对选项不从本地草稿恢复。来源发生变化后必须重新核对，不能恢复旧请求体直接执行。

网络结果未知时使用原操作查询与幂等恢复，不换永久来源标识重复记账。Web额外核对返回费用的状态、准确版本、原金额、发生日、全部分摊、凭证、范围和唯一来源关联；不能核对时停止连续提交，引导按永久来源查询。移动端使用现有命令状态机与同一准确回执校验。

## 检查与未完成边界

[本轮核验](../quality/125-business-cost-verification.md)记录实际检查、原生构建、页面和媒体；[当前范围](../delivery/04-current-scope-and-acceptance.md)与[完整差异表](../delivery/03-goal-gap-assessment.md)继续保留23个开放工作包和5项移出。本轮没有新增结构升级，继续使用schema94，旧迁移字节保持。

原生Windows的当前JS打包帮助器为自有Metro子进程显式传递同一文件读取适配，修正其空`execArgv`不继承父适配的问题；仅读取真实本地文件及链接，保持四Worker和no-Expo源映射检查，不创建APK。外部Node选项不继承，原始失败与主动停止结果单独保留，最终命令记录重新绑定当前输入。

仍需完整的公司审批／财务制度、跨模块最终付款／核销／结项正常及异常旅程、各移动平台正式发布及真实云／模型／TG／生产验收。没有开展真机、模拟器、网络性能、实网120GB专项、历史迁移／容灾或AI费用控制。两地对象存储、日常文件恢复及AI授权／隔离／审计要求继续保留。

## English implementation and usage

Open an approved purchase or claim, or an effective payable contract, then choose the explicit actual-expense action. The page pins the current source version, approved revision/hash, owning project, classification and currency. Only a descriptive title is seeded; the human supplies actual date/amount, a fixed supplier revision, formal evidence versions, exact project/company shares and a reason, then explicitly acknowledges the facts and approved basis with recent MFA.

The new coordinator composes existing create/confirm/link commands in one PostgreSQL transaction. Existing authorization, source ceilings, project locks, permanent source identity, fixed evidence, journals, audit, Outbox and idempotency rules continue to apply. Any late failure rolls everything back. Exact replay does not create a second expense; changed input, stale source, revoked authority and stale real-session MFA are rejected. The preview never reserves a balance or turns approval into incurred expense or bank cash.

Web and the shared WeChat/Douyin/Android pages use the same typed basis and exact receipt validation. Mobile drafts preserve only permitted text and are isolated by actor authority and source version/revision/hash; evidence, supplier references, MFA and acknowledgements are not restored. A successful expense appears in ordinary history and actual reports. Bank payment, settlement and business completion remain explicit separate actions. See the linked verification record for executed local scope, retained evidence and remaining production/platform work.
