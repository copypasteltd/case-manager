# 95 · 任务讨论、固定附件与明确提及 / Task discussions and explicit mentions

[文档中心](../README.md) · [中文使用指南](../README.zh-CN.md) · [English guide](../README.en.md) · [编号差异表](../delivery/03-goal-gap-assessment.md)

本增量补齐任务协作中的讨论链路。Web 和共享 Taro 页面均可新建任务评论、修改自己的评论、核对固定附件与明确提及成员、查阅不可变修订，并从站内通知打开指定讨论。四端目标不变：React Web、微信小程序、抖音小程序、Android；H5 仅用于本地审查共享移动代码。

This increment adds a task discussion workflow to React Web and the shared Taro task page: comments, immutable edits, fixed file-version attachments, explicit member selection, revision history, and exact discussion links from the in-app inbox. The four shipping clients remain Web, WeChat, Douyin and Android. H5 is a local review harness.

## 1. 使用流程 / User workflow

| 编号 | 操作 | 系统行为 |
| --- | --- | --- |
| TC-01 | 从项目打开任务，选择“新增任务评论” | 重新读取任务、项目权限和当前讨论；只读项目与查看者不提供写入入口。 |
| TC-02 | 填写评论文字 | 接受 1–20000 个 Unicode 字符；文字中的 `@姓名` 不会自动产生通知。 |
| TC-03 | 从当前有权文件选择正式版本 | 每个附件固定为准确 `file_version_id`，最多20个；当前文件后续更新不会替换原依据。附件查阅时独立核对当前权限。 |
| TC-04 | 从有权项目成员列表明确选择提及对象 | 列表独立分页、支持姓名查询、最多20人；排除作者本人、停用用户、非成员和不能读取任务的成员。选择不是权限授予。 |
| TC-05 | 点击“核对任务评论”，检查预览，再勾选确认并保存 | 客户端绑定准确任务、原评论修订及所选关联；服务器提交后，客户端再次读取准确评论和不可变历史，完整一致才显示完成。 |
| TC-06 | 被提及者打开通知中心，点击“打开相关资料” | 重新校验权限，定位准确任务和当前评论修订，即使该讨论已不在最新20条中；加载后自动滚动，使目标正文进入可视区域。通知已读不代表任务完成。 |
| TC-07 | 作者修改自己的评论 | 默认只修改文字，保留服务器上的全部原附件和原提及；创建后继修订，保留旧记录。 |
| TC-08 | 作者勾选“明确替换全部原附件／原提及” | 以本次重新选择的完整列表替换对应关联；空列表表示清空。部分原资料不可核对时拒绝替换，避免客户端过滤结果造成静默删除。 |
| TC-09 | 查看任务评论历史 | 独立分页查看准确修订、原文字与仍有权访问的固定附件。历史记录不会被新正文覆盖。 |
| TC-10 | 遇到冲突、失去权限、回执丢失或不完整 | 保留会话内文字，停止重复提交；先关闭表单并重新读取讨论核对。不会自动把不确定操作转换成另一次创建。 |

English: open a project task, write a comment, optionally select exact file versions and eligible members, review the preview, explicitly confirm, then save. The client verifies the committed record and history. Recipients open the exact current discussion from their inbox. Editing creates a successor and preserves associations unless a complete replacement is explicitly requested. Conflicts and uncertain responses retain text and require reconciliation before another operation.

## 2. 接口与共享约束 / API and shared contracts

| 接口 | 用途与边界 |
| --- | --- |
| `GET /v1/tasks/{id}/comments?limit=20&cursor=…` | 当前评论叶节点列表；签名游标绑定组织、用户授权、渠道和任务。 |
| `POST /v1/tasks/{id}/comments` | 创建评论；使用 `Idempotency-Key`，Cookie 写入同时要求 CSRF 与来源校验。 |
| `PATCH /v1/comments/{id}` | 只能修改自己的准确前序修订；必须提供 `expected_revision`，已有后继时返回冲突。 |
| `GET /v1/comments/{id}` | 独立读取准确评论，用于通知定位和保存后核对；不会因为列表首屏不包含该记录而误判不存在。 |
| `GET /v1/comments/{id}/history` | 从选中修订向前读取不可变历史；独立游标，不混入其他评论。 |
| `GET /v1/tasks/{id}/mention-candidates?q=…&limit=20&cursor=…` | 当前可选提及成员；最多每页100人，查询最多200字符，游标绑定任务与查询。要求当前任务读取和评论写入权限。 |
| `GET /v1/notifications…` / `GET /v1/notifications/{id}` | 新增 `mention` 类别；每次投递和读取都重新检查接收人当前权限及提及修订链。 |

`mentioned_user_ids` 是显式 UUID 数组。新建时省略表示没有提及；修改时省略表示保留完整原关联；显式空数组表示清空。知识正文评论继续使用原上下文约束，不支持任务提及。`mentions_filtered` 表示部分原成员因当前资格被过滤，客户端不得把可见数组当作完整原始数组覆盖。

`app/packages/contracts/src/task-comment.ts` 供两套界面共用。回执必须匹配任务、作者、完整文字、修订号、准确前序、固定附件、明确提及以及时间；任务评论的知识正文上下文必须为 `null`。缺失提及字段属于不完整回执。新增回执字段在通用 OpenAPI 评论模型中保持可选以兼容旧知识评论调用，但新的任务评论交互严格要求完整字段。

English: omitted arrays preserve stored associations on edits; explicit empty arrays clear them. `mentions_filtered` identifies a filtered view, not a complete replacement basis. Both clients share payload and exact-receipt validation. Task discussions have no knowledge-publication context. Existing post comments retain their original revision rules.

## 3. 数据模型与权限 / Data and access

迁移 `0076_task_comment_mentions.sql` 为 `comments` 增加 `mentioned_user_ids uuid[] NOT NULL DEFAULT '{}'`。原评论表仍禁止更新与删除；单后继约束保留。数据库约束拒绝重复或空标识、自我提及、跨组织成员、非任务关联及超过20人的数组；新增提及必须满足当前项目成员资格和至少一个人类客户端渠道的任务读取权限。

应用层在幂等查询前和实际修改锁内检查显式替换及当前资格。只修改文字时可保留已经失去资格的原关联，但读取与回执均过滤这些标识，投递不会因此获得访问权限。组织授权屏障、当前会话、项目关闭状态和既有任务 ACL 继续生效。

Migration76 adds immutable mention arrays to existing comment revisions. Database and application checks enforce recipient identity, organization, membership, task access and bounds. Read-time filtering and pre-idempotency authorization prevent an earlier stored receipt from revealing currently inaccessible associations. Existing authorization barriers and project read-only rules remain in force.

## 4. 通知生命周期 / Notification lifecycle

| 触发情形 | 投递规则 |
| --- | --- |
| 新建评论，明确提及新成员 | 从实际已提交评论读取接收人，通过现有 outbox → 已租用通知任务 → 站内通知管线投递。 |
| 修改正文，提及列表不变 | 不新增提醒；读取通知时定位当前叶修订。 |
| 修订中新增提及某成员 | 只为相对直接前序新增的成员生成提醒，排除作者本人。 |
| 移除某成员的提及 | 原提醒隐藏；排队但尚未投递的旧事件也不能绕过移除。 |
| 移除后再次提及同一成员 | 新修订产生新的提醒上下文；旧提醒不会因后续重新加入而复活。 |
| 撤销项目权限或停用接收人 | 投递时不创建可访问提醒，读取时不返回原业务标题与链接；客户端按当前会话状态退出或重新授权。 |
| 标记通知已读 | 只推进通知已读序号，不修改任务状态或审批结果。 |

去重键为 `mention:{task_id}:{origin_comment_id}`，仍受组织、接收人和站内渠道约束。通知上下文不可变，业务标题与正文不保存在通知头中。递归读取当前后继时，要求接收人贯穿起点至叶节点的**每一条修订**；仅检查最新数组会导致“移除后再加入”错误恢复旧提醒，因此不采用该规则。

投递沿用真实业务事件、任务租约、fence 与幂等账本；不新增 Telegram、邮件或推送发送器。现有 Telegram 秘书群沟通链路属于另一条权限与投递体系，本增量的提及提醒仅进入各员工自己的站内通知。

English: mention recipients come from committed immutable comments, never an event payload or text parser. Deduplication includes the originating comment revision. Resolving a later leaf requires continuous inclusion through every successor, so removing and later re-adding a recipient cannot resurrect the earlier notification. Delivery and read operations recheck live access. Reading a notification never completes a task. No external message sender is added.

## 5. 入口与客户端状态 / Client state

Web 通知地址为 `/projects/{project_id}?task={task_id}&comment={comment_id}`；Taro 地址为 `/pages/detail/index?kind=task&id={task_id}&comment_id={comment_id}`。共享通知解析器只接受规范 UUID 与允许的业务路由；`comment_id` 只能用于任务，服务器返回的 Web 路径必须与重新计算的安全路径完全一致。

移动端沿用既有任务详情页，四端页面范围继续完整保留。附件沿用准确版本的查阅／下载流程；本增量不会把120GB原件加载进评论表单，也不让小程序承担超大原件上传。

表单中的文字可以暂存在会话内存中；固定文件选择、成员选择、核对声明与鉴权材料不恢复。草稿有容量与寿命限制，并绑定当前身份、授权与业务目标。结果不确定时保留原意图的幂等标识，禁止悄悄换成另一条写入。用户先主动核对列表，再重新打开、选择关联并确认同一意图时，复用原幂等键读取真实结果，不自动重发。Web与Taro在通知准确评论载入后自动定位；浏览器验证直接核对目标正文的可视边界，不由测试脚本代替滚动。凭据、数据库与原始录屏帧继续留在 Git 外。

## 6. 验证与验收边界 / Verification boundaries

本轮具体结果、源码摘要、浏览器操作、媒体来源与失败记录见 [本轮验证记录](../quality/95-task-discussion-verification.md)。历史增量94的发布入口、源码归档与后端证据保存在 `quality/evidence/task-mentions/history/published-94/`，不会以新结果覆盖原结论。

Native PostgreSQL integration checks and actual local Web/Taro H5 interactions establish the locally exercised workflows. Mini-program compilation and Android development builds establish build scope only. WeChat/Douyin real-device and platform review, Android installation/background behavior, real OSS/R2 operations, model availability, Telegram network delivery, disaster recovery, migration of2TB,120GB transfer and mainland-to-Hong-Kong performance remain separate acceptance work.
