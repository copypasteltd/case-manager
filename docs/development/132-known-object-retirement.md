# 已知原件保留与清理 / Known-original retirement · 132

增量132将已知历史正式原件、已提交人工上传的草稿原件接入完整清理流程：分页盘点 → 明确保留制度 → 固定单份对象清单 → 至少24小时观察 → 最近MFA人工批准 → 永久使用屏障 → 独立Worker逐云核对执行 → 不可变逐次回执。Web、微信、抖音及Android共享同一业务合同。

## 使用流程

1. 由有权人员明确发布 `formal_history`、`draft` 保留制度；回收站文件还须满足回收站制度。没有制度、停用制度或仍在保留期内均不能进入清理。
2. 进入文件详情“原件保留与清理”→“盘点历史与草稿原件”。查看每个固定内容版本、保留期限及阻断原因。当前正式版本、法务／业务保留、固定业务引用、恢复／上传依赖、未过期下载许可、派生对象及来源不完整继续受保护。
3. 明确选择一个历史版本或一个已提交上传草稿并填写原因，建立不可修改的观察清单。历史版本固定OSS和R2两份原件；草稿只固定已提交上传的那份草稿，不把正式版本一起删除。清单不接受任意桶、对象键或地址。
4. 至少观察24小时，至观察结束后七天内，再核对文件修订、对象摘要、当前制度／权限／引用，完成最近MFA及不可逆影响确认后批准。观察期没有后台跳过入口；演示中时间推进仅存在于隔离测试夹具。
5. 批准历史原件立即将该版本置为 `retiring`，停用下载、正文读取、比较、恢复来源、新业务引用和重新复制。当前版本及版本元数据保持；已有数据库提取正文和历史证据不属于本次云原件对象删除，查阅仍受停用状态约束。独立Worker仍重新核对原批准人的有效会话、授权版本和当前保护；重新登录不自动延续旧授权。
6. 每个云对象保留开始、失败、重试及准确不存在回执。只有两边全部准确确认不存在，历史版本才成为 `purged`，释放相应逻辑容量；单边完成或未知结果继续占用容量。
7. 失败／受保护项可在原批准仍有效、原清单仍完全匹配时人工重试；新一轮代号保留旧结果。停止只阻止剩余工作，永久屏障和已执行结果不撤回，不能恢复已删除的原件。原会话失效或制度改变后的重新授权／续作尚未提供，保留待处理状态，不自动放行。

## 范围与保护

| 编号 | 可以处理 | 必要条件 |
|---|---|---|
| R132-01 | 已知历史正式版本的两份原件 | 当前版本之外，双云完整SHA及准确原生来源，所有制度／引用／保留／作业保护通过 |
| R132-02 | 已提交人工上传的草稿原件 | 不可变提交时间已满保留期、签名过期＋宽限、无作业、准确草稿来源、正式双副本已验证，或对应历史原件已完整清理 |
| R132-03 | 云成功、数据库最终回执回滚后的重试 | 重新核对原对象准确不存在，再原子保存回执、审计、outbox、队列及最终状态；不能凭前次删除请求成功猜测 |
| R132-04 | 部分清理后明确停止 | 保留成功项、永久对象键屏障和停用状态；其余项阻断，不制造全部完成或恢复 |

未完成上传分片沿用增量104。本轮没有处理已完成但未正式提交的草稿、秘书跨运行恢复来源、未知孤儿对象扫描／清理、复杂派生对象。空原件额外阻断待专项核对。上述余项继续GAP-05；真实OSS／R2身份、版本化、网络及提供方语义验收继续GAP-20，不把控制传输测试当作真实云删除。

## 数据、权限与事务

正常结构升级98→99新增 `object_retirement_plans/items/barriers/receipts` 四表及引用／对象键／可用性触发器；`upload_sessions.committed_at` 在提交时固定，对旧已提交记录以原更新时间补齐。此为正常结构升级；不启用历史数据迁移或容灾项目。保留原上传其他字段、原件元数据、历史、财务和服务事实。

读取和命令均要求当前人工会话、`file.read`、`file.retention.manage` 及文件当前可见范围。Web沿用Cookie＋CSRF；其余三端沿用绑定频道的人工会话。模型运行、Skill及TG不能调用此流程。批准／停止／重试要求最近MFA；审批人、会话、频道及组织／人员授权修订固定，Worker执行前重新核对。游标绑定本人、授权版本、文件和集合用途。

批准在组织独占锁下锁定清单、文件及历史版本，重新评估后，在同一事务创建永久对象键屏障、排队任务、记录批准、停用版本及审计／outbox。数据库触发器对当前版本指向、业务版本引用、下载许可、恢复来源、上传基版本和派生对象加行锁并拒绝使用已停用版本，覆盖非HTTP写入。对象传输、复制、正式版本和上传尝试不能复用已退休对象键。

Worker专用任务 `file.original.retirement.v1`／队列 `original-cleanup`，两分钟租约、45秒单次执行、最多八次自动尝试，租约栅栏及独立失败回收。持久开始回执先提交；准确云操作和最终状态仍在持锁、有限时间的事务内执行，数据库空闲事务上限本地设为60秒。云结果无法与PG分布式回滚：若云删除完成但最终事务失败，保留未知／重试状态，重新确认原对象不存在后收口。

不可变对象身份、云位置摘要、对象键、ETag和原生版本保留在内部证据中；业务API仅返回固定内容与摘要及结果，不暴露桶、原生对象版本或路径。历史原件最多两项，草稿一项，没有批量模糊前缀删除入口。

## 提供方语义与部署

API只读取配置位置摘要，不装入清理身份。`CP_ORIGINAL_CLEANUP_PROVIDERS=disabled` 为默认状态；正式启用时显式列出 `oss,r2`，各自配置原位置 `CP_OSS_ENDPOINT/REGION/BUCKET` 和 `CP_R2_ENDPOINT/REGION/BUCKET`。独立Worker私有环境使用 `CP_OSS_ORIGINAL_CLEANER_ACCESS_KEY_ID/ACCESS_KEY_SECRET`、`CP_R2_ORIGINAL_CLEANER_ACCESS_KEY_ID/ACCESS_KEY_SECRET` 及原用途安全令牌（如使用）；不回退借用上传、下载、复制或API身份。按提供方实际能力配置最小权限：OSS限制受管目录与准确版本读取／删除；R2独立令牌限定桶，其对象读写权限可能更宽，不宣称提供方已授予“仅两操作／仅目录”权限。实际授权粒度继续GAP-20。Worker与API共享同一位置摘要。凭据文件不入库。

OSS必须已有真实且非 `null` 的对象原生版本。先按准确版本＋ETag进行单字节有界GET核对，再删除该准确版本，最后重新确认准确对象／版本不存在；不会用普通无版本Delete生成标记冒充物理回收。[OSS DeleteObject](https://www.alibabacloud.com/help/en/oss/developer-reference/deleteobject)。

R2不提供原生版本化，本实现不把DeleteObject的If-Match当作已证明的原子比较删除。必须由部署者显式设置 `CP_R2_CLEANUP_EXCLUSIVE_NAMESPACE=true`，核对受管不可变命名空间独占写入、无外部写入者／自动生命周期任务，并在平台作业屏障下执行；不满足则阻断。若无法保证该条件，请保持R2清理关闭。发送If-Match仅作附加检查；仍先核对准确来源、最后确认准确不存在。[R2 S3支持](https://developers.cloudflare.com/r2/api/s3/api/) · [一致性](https://developers.cloudflare.com/r2/reference/consistency/) · [扩展](https://developers.cloudflare.com/r2/api/s3/extensions/)。

两种云均不将204、普通404、代理NotFound或NoSuchBucket当作准确原件不存在。只接受实际SDK解析的 `NoSuchKey/NoSuchVersion`，超时／权限错误／来源变化均保留失败或未知。原件大小大于零才进入本流程。

目标原生Linux部署顺序：发布匹配源码的platform → 正常 `migrate` → 核对99项迁移及私有位置／清理身份 → 在专用系统账号安装 `app/deploy/native/copypaste-original-cleanup-worker.service` → 启动 `platform worker-original-cleanup` → 就绪及逐项回执核对。此服务没有在当前开发机启动；API、通知和报表开发进程仅作匹配升级，云／模型／TG继续关闭。正式Linux运行及云身份语义仍待目标环境验收。

## 接口与实现

| 接口 | 用途 |
|---|---|
| `GET /v1/files/{id}/object-retirement-candidates` | 当前文件准确候选与阻断原因，分页 |
| `GET/POST /v1/files/{id}/object-retirement-plans` | 分页历史／创建一份固定观察清单 |
| `GET /v1/object-retirement-plans/{id}` | 当前有权读取的原清单及执行项 |
| `POST /v1/object-retirement-plans/{id}/approve` | 最近MFA、准确修订／摘要及二次评估后批准 |
| `POST /v1/object-retirement-plans/{id}/cancel` | 人工停止剩余工作，保留永久屏障与原结果 |
| `POST /v1/object-retirement-items/{id}/retry` | 原批准及当前保护匹配时显式增加执行轮次 |
| `GET /v1/object-retirement-items/{id}/receipts` | 本对象不可变逐次结果，分页 |

实现位于 `app/backend/internal/files/object_retirement*.go`、`internal/objectstore/original_cleanup.go`、`internal/storageconfig/object_retirement.go`、`cmd/platform/object_retirement_worker.go`；Web `ObjectRetirement.tsx`，共享移动 `pages/files/retirement.tsx`，共享规则 `packages/contracts/src/object-retirement.ts`。OpenAPI与TypeScript合同沿用原生成流程。

## English guide

Increment132 implements fixed known-original retirement across Web, WeChat, Douyin and Android. In file details, inspect exact historical or committed-upload-draft candidates, select one eligible source, enter a reason and create an immutable observing plan. At least24 hours later and within its seven-day approval window, review the exact digest and current protections, refresh MFA and explicitly approve. No arbitrary object keys, buckets, prefixes or URLs are accepted.

Historical approval permanently retires that version before cloud operations, blocking reads, references, restoration and recreation. Two exact OSS/R2 originals must both be confirmed absent before the version becomes purged and logical capacity is released. Draft cleanup removes only its committed upload draft. Metadata and receipts remain; cancellation stops remaining work without restoring originals or removing key barriers. Explicit retries preserve earlier evidence and require the original approved session/authorization and current protections to remain valid. Renewed authorization after session loss is still open.

A dedicated leased/fenced native worker uses independently configured ORIGINAL_CLEANER credentials, never API/upload/replica credentials. API configuration contains location metadata only. OSS requires an exact non-null native version. R2 requires explicitly attested exclusive immutable managed storage with no external writers/lifecycle deletion; If-Match is additional protection, not a claim of documented atomic conditional deletion. Exact signed bounded GETs verify identity and absence; deletion acknowledgements, generic404 and missing buckets never count as success. Cloud side effects cannot roll back with PostgreSQL: a failed final transaction is reconciled against the original object on retry.

Normal schema98→99 adds immutable plans/items/barriers/receipts and dependency guards plus committed timestamps. Known multipart cleanup remains increment104. Unknown orphans, completed-but-uncommitted drafts, secretary recovery sources, complex derived objects and renewed approvals remain open under GAP-05. Actual cloud, Linux/runtime, model/TG/platform and production acceptance remain explicit. All23 packages, five exclusions and eight draft Skills remain; no device, emulator, virtualization, network-performance or live120GB test is introduced.

[本轮实际核验](../quality/132-known-object-retirement-verification.md) · [完整23项余项](../delivery/05-functional-closure-132.md)
