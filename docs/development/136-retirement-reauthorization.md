# 增量136 · 已知原件清理重新授权

## 业务问题与结果

增量132的原件清理绑定初次批准人的会话和权限版本。批准人退出、会话到期或组织权限变化后，后台会阻止删除；原计划仍保留停用屏障，因此简单重试无法恢复。现在有权限的员工可以明确采用**原固定清单中的全部剩余原件**，生成一条新的执行授权并重新排队。原批准、观察期、来源身份、永久屏障和已有结果不变。

本轮仅扩展132已有的历史正式原件、已提交人工上传草稿；未正式提交的已完成草稿、秘书恢复来源、未知孤儿和复杂派生对象继续列在GAP-05。实际OSS／R2身份和提供方语义仍须GAP-20验收。

## 使用顺序

1. 进入文件详情 → **原件保留与清理** → **盘点历史与草稿原件**，查阅原计划。
2. 等待所有剩余任务显示失败或受保护／停止；已排队、执行中或等待自动重试的任务不能提前重新授权。
3. 选择**重新授权剩余原件**，核对来源摘要、初次批准、已确认数量和不可逆影响，填写依据并明确勾选确认。
4. 最近十分钟内完成本人MFA后，点击**确认重新授权并继续**。服务端再次核对当前会话、授权、制度、文件修订和永久屏障。
5. 成功后只有剩余原件进入下一执行轮次。查阅**重新授权记录**和各原件**逐次回执**；刷新读取持久结果。

初次批准期限只约束初次批准。重新授权不修改原期限，不补做或跳过最少24小时观察。已停止或全部完成的计划不能续作。若制度、来源、文件修订或保护条件变化，本操作拒绝；不会通过重新授权接受不同原件或降低保留要求。

Web和共享移动页面均提供上述入口；微信、抖音、Android使用同一移动实现。当前Android开发包仍沿用133，未重新打包；136功能需要当前源码构建。

## 数据与原子性

正常升级迁移：`app/backend/migrations/0103_retirement_reauthorizations.sql`。1–102保持原字节。已有计划的执行授权指针为空，继续沿用初次批准；旧回执的新增授权关联为空。

| 数据 | 约束 |
|---|---|
| `object_retirement_authorizations` | 只追加；记录顺序、计划修订、前次授权、固定摘要、剩余项旧修订／新轮次、理由、时间、员工及私有会话／权限版本 |
| `object_retirement_plans.execution_authorization_id` | 指向本组织、本计划的新授权；数据库触发器要求连续修订、连续授权顺序、正确前序和原摘要 |
| 初次批准字段／清单／对象 | 原不可变约束保留；不以新授权覆盖初次批准人或期限 |
| 原件执行项 | 仅失败／阻塞项进入新轮次，修订递增；成功项完全保留 |
| `object_retirement_receipts.authorization_id` | 新执行结果关联实际采用的授权；原回执继续只追加 |

一次命令在组织独占锁、计划锁和文件锁下完成：当前人身份与MFA → 精确比对全部剩余项及修订 → 重新评估来源／保留规则／屏障 → 追加授权 → 切换指针 → 排队下一轮 → 审计和事件提交。任何一步失败均回滚，包括晚到的审计失败。重复命令沿用幂等回执，不增加授权或任务。

工作进程按计划最新执行授权重新读取真实会话和权限。后续再次退出或撤权仍会阻止执行。旧轮次任务在执行前核对轮次和队列租约；已确认的原件不会再次调用云清理。取消只停止剩余工作，保留永久屏障及删除结果。

## 接口

| 方法／路径（省略`/v1`） | 用途 |
|---|---|
| `POST /object-retirement-plans/{id}/reauthorize` | 当前计划修订、原摘要、理由和完整剩余项`id/expected_version`；成功202并返回计划 |
| `GET /object-retirement-plans/{id}/authorizations` | 当前授权下分页读历史；游标绑定组织／员工权限及计划 |
| 既有计划／清单读取 | 新增可空`authorization`公开元信息；私有会话、权限版本、桶／对象来源不返回 |

输入及响应在Go、生成OpenAPI、共享TypeScript和两个页面中保持一致。共享规则检查授权来源、项目成员、顺序、修订和新轮次，确认返回结果保留全部成功项及初次批准。

## English usage and boundaries

When an original cleanup approval loses its session or authority, the worker stops safely. An authorized human can now adopt the exact stopped remainder, acknowledge irreversible effects, supply a reason and complete recent MFA. Only failed or blocked remaining items move into the next generation; confirmed items and the initial observed approval remain unchanged.

Open file details → original retention and cleanup → the fixed plan → **重新授权剩余原件** → **确认重新授权并继续**. Read the new append-only authorization history and per-original receipts. Cancelled/completed plans and work still active cannot renew. Current source, policy, file revision, authority and permanent barriers are rechecked atomically. A later logout or revocation also stops the renewed grant.

Apply normal schema103 before running this backend; the persistent development database remains100 and needs101–103. The immutable first freeze uses100 and contains none of134–136. This increment adds source functionality, focused native checks and a synthetic authenticated Web demonstration; it does not establish production, model, platform or live-cloud acceptance. See [verification](../quality/136-retirement-reauthorization-verification.md) and [all remaining work](../delivery/05-functional-closure-136.md).
