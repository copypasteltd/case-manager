# 增量104：未完成上传的受控分片清理 / Controlled multipart cleanup

本轮实现 GAP-05 中“已知未完成上传”的清理执行闭环。Web、微信、抖音及 Android 可在目录内盘点、选择固定候选、等待观察期、人工批准、撤销剩余工作、重试失败项并查看逐次回执。Go API 与独立 Worker 使用原生 PostgreSQL 持久化清单、任务和审计。正式历史版本、已完成草稿对象及未知孤儿对象的物理清理仍待实现；GAP-05 继续开放。

Increment104 implements folder-scoped administration of known abandoned multipart uploads across all four clients. Human approval queues exact multipart work; a separate worker records immutable start and outcome receipts. Formal-version deletion, completed draft-object deletion and unknown orphan inventory remain open. This increment does not close the full lifecycle work package or establish live-cloud acceptance.

## 使用流程 / Workflow

| 步骤 | 员工操作 | 系统结果 |
|---|---|---|
| 1 | 打开有权访问的具体目录，选择“盘点与清理” | 显示可见上传及逐项阻断；无权限对象不会作为候选泄露 |
| 2 | 明确选择1至50项，填写清理原因 | 固定上传修订、来源摘要、目录修订、保留制度及存储位置摘要 |
| 3 | 等待清单的观察期结束 | 至少24小时；实际采用所选制度中最长观察期；没有跳过接口 |
| 4 | 查阅固定清单，核对摘要、日期及影响；完成近期 MFA，明确批准 | 重新检查所有候选，一项变化即整体拒绝；封存上传并原子入队 |
| 5 | 刷新清单及逐次回执 | 已排队／执行中／等待重试／失败／阻断与确认不存在分别呈现 |
| 6 | 必要时撤销，或明确重试失败项 | 撤销停止尚未执行工作；失败重试新建执行轮次，保留过往回执 |

In a specific folder, choose **盘点与清理**, select exact eligible uploads and provide a reason. Review the resulting digest and observation window. After the window opens, verify MFA, acknowledge the fixed list and approve it. Refresh to inspect per-item outcomes. Cancellation stops remaining work; explicit retry creates a new generation while preserving previous receipts. An uncertain command result requires rereading existing plans before another submission.

清单状态 `observing / approved / cancelled` 表示人工生命周期；执行结果位于每个 item。`approved` 只表示批准排队，不能作为清理完成依据。页面仅将带确认时间的 `succeeded` 项计入完成。没有估算“释放容量”，原上传声明大小并非云端已上传分片占用量。

## 允许执行的边界 / Eligibility

- 调用人具有当前有效的人类会话、目录读取和 `file.retention.manage`；支持四种人类渠道，拒绝 Agent 运行上下文。若上传更新已有文件，还要求准确文件的读取与保留管理权限。
- 原上传来自 Web 或 Android，处于允许的未完成状态。会话过期后再等待最长签名许可时长15分钟，创建时间已超过明确发布的 multipart 保留期。
- 当前 multipart 制度必须存在且启用。默认配置不推定公司保留策略；观察期至少86400秒，保留期至少86400秒。
- 准确的原生 `upload_attempts` 必须与上传会话、草稿键、提供商 UploadId 和 active 尝试一致；草稿键还需符合该尝试的组织命名空间。
- 没有最终化 manifest、冻结来源、正式版本、活动上传任务、其他已批准清理占用或文件保留锁。普通下载和正式双云副本均不作为删除目标。
- 存储提供商、桶及非秘密 endpoint/region/bucket 摘要必须与盘点时一致。凭据轮换不改变位置摘要，账号或桶变化会阻断旧清单。

批准后把上传封存为 `expired` 并增加修订。Worker 保持核对封存修订、原批准人的会话及授权版本、目录修订、当前制度、存储绑定和保留锁。退出登录、撤权或来源改变都会阻断执行；失败项重试仍要求原批准继续有效，必要时重新建立清单。

## 数据、事务与恢复 / Data and transactions

迁移82新增三类记录：`upload_cleanup_plans` 固定清单、`upload_cleanup_items` 逐项执行、`upload_cleanup_receipts` 不可改写回执。清单的来源、摘要、观察期和有效期不可原地改写；已确认项不可恢复成待执行，应用角色不能删除清单或改写回执。

| 操作 | 同一事务内提交的内容 |
|---|---|
| 建立清单 | 固定 JSON 及 SHA-256、观察期和批准有效期、候选执行项、审计及 Outbox |
| 批准 | 当前权限与近期 MFA、完整固定候选复核、上传封存修订、全部执行项、持久任务、批准与审计 |
| 撤销／重试 | 清单或执行项修订、相应队列变化及不可变历史、审计及 Outbox |
| Worker 开始 | 持有有效租约，开始回执与审计先提交，再执行云调用 |
| Worker 结果 | 逐项状态、确认时间或安全错误码、不可变回执、审计／Outbox、任务完成或重试 |
| 最终尝试耗尽 | 对准确已 dead 的任务重新加锁，补记失败、审计及 Outbox；不再调用云服务 |

任务类型 `upload.multipart.cleanup.v1`，独立 lane `storage-cleanup`，每项每轮最多8次尝试，租约2分钟，单次处理45秒，失败回执恢复最多10秒。每轮任务使用准确 item/generation 去重。过期 Worker 不能通过 fencing 检查执行云操作。

开始回执单独落库。如果云端已终止分片但结果审计失败，结果事务回滚，重试会先检查准确 UploadId 的当前状态。不会因缺失本地成功回执而盲目重复终止。云请求期间持有相关数据库锁，限制在处理超时内；撤销和权限更新会等待已开始的这次有界操作。

## 云端操作及身份 / Storage operations and identity

执行器只接收 `Abort` 和 `MultipartAbsent` 两个方法。先 `ListParts` 检查准确 UploadId，仍存在才执行 `AbortMultipartUpload`，再检查一次。**只有明确的 `NoSuchUpload` 说明这一 multipart 会话不存在**；成功返回空分片列表、普通404、`NoSuchKey`、桶不存在、权限错误均不能作为成功依据。确认只覆盖该 UploadId，不能据此宣称完成对象已删除或释放特定字节。

阿里云说明终止分片会取消对应 multipart；已完成上传也可能返回 `NoSuchUpload`。本实现因此同时检查应用中的最终化／正式来源并封存会话；回执语义严格限定为分片会话确认不存在。[OSS AbortMultipartUpload](https://www.alibabacloud.com/help/en/oss/developer-reference/abortmultipartupload)、[OSS ListParts](https://www.alibabacloud.com/help/en/oss/developer-reference/listparts)。R2 通过 S3 兼容接口提供相应操作，具体支持以其官方兼容表为准。[R2 S3 API](https://developers.cloudflare.com/r2/api/s3/api/)。

API 进程只加载非秘密存储位置元数据，不能取得清理身份。独立 Worker 显式加载 `MULTIPART_CLEANER` 专用凭据，不回退到上传身份、默认环境凭据或管理员身份。OSS 应将身份限制为所需组织草稿前缀及 ListParts／AbortMultipartUpload；代码没有 DeleteObject 调用。

R2 的令牌授权粒度须按真实账号核对，官方对象读写权限可能包含更广能力，不能把接口只暴露两种操作描述成云端已经授予“仅两操作”权限。应使用独立、限定桶的令牌及单独进程配置；更细范围与真实身份验收仍属于 GAP-20。[R2 token permissions](https://developers.cloudflare.com/r2/api/tokens/)。

## 配置与原生运行 / Native configuration

```dotenv
# API 与 Worker 都需要相同的非秘密位置描述；空列表默认停用。
CP_MULTIPART_CLEANUP_PROVIDERS=oss,r2
CP_OSS_ENDPOINT=https://oss-cn-example.aliyuncs.com
CP_OSS_REGION=cn-example
CP_OSS_BUCKET=your-private-bucket
CP_R2_ENDPOINT=https://your-account.r2.cloudflarestorage.com
CP_R2_REGION=auto
CP_R2_BUCKET=your-private-bucket

# 仅写入独立 Worker 的受限环境文件；不要提交真实值。
CP_OSS_MULTIPART_CLEANER_ACCESS_KEY_ID=replace-outside-git
CP_OSS_MULTIPART_CLEANER_ACCESS_KEY_SECRET=replace-outside-git
CP_R2_MULTIPART_CLEANER_ACCESS_KEY_ID=replace-outside-git
CP_R2_MULTIPART_CLEANER_ACCESS_KEY_SECRET=replace-outside-git
```

实际字段以 [配置加载器](../../app/backend/internal/storageconfig/multipart_cleanup.go) 和 [角色存储配置](../../app/backend/internal/storageconfig/config.go) 为准。完成正常数据库升级后运行 `platform worker-multipart-cleanup`；Worker 不自动迁移。Linux 原生服务模板见 [systemd unit](../../app/deploy/native/copypaste-multipart-cleanup-worker.service)，环境文件为 `/etc/copypaste/multipart-cleanup-worker.env`，权限0700目录／0600文件；API 环境文件不放清理凭据。

## API 与客户端约束 / API and client constraints

| 方法 | 路径（省略 `/v1`） | 用途 |
|---|---|---|
| GET | `/folders/{id}/upload-cleanup-candidates` | 权限过滤后的游标分页盘点 |
| GET / POST | `/folders/{id}/upload-cleanup-plans` | 分页历史／固定候选与原因建立清单 |
| GET | `/upload-cleanup-plans/{id}` | 当前清单及所有执行项 |
| POST | `/upload-cleanup-plans/{id}/approve` | 修订、摘要、原因及近期 MFA 批准 |
| POST | `/upload-cleanup-plans/{id}/cancel` | 修订、摘要、原因及近期 MFA 撤销 |
| POST | `/upload-cleanup-items/{id}/retry` | 失败项准确修订、近期 MFA 明确重试 |
| GET | `/upload-cleanup-items/{id}/receipts` | 绑定调用人权限及执行项的游标回执 |

写入统一要求幂等键；Web 使用 Cookie／CSRF，其他三端使用各自渠道会话。客户端共享选择、固定回执及观察期校验；读取结果不符合原目录、计划或准确候选时停止显示成功。切换目录、身份、授权版本或手动刷新会重新建立页面状态；文件选择、确认及 MFA 不存为业务草稿。

## 验证边界 / Verification limits

本轮原生 PG／真实 HTTP／官方 SDK 传输夹具与 UI 操作记录见 [验证文档](../quality/104-multipart-cleanup-verification.md)。云 SDK 测试使用合成 HTTP 响应；业务 Worker 测试使用受控云接口，演示数据库中的观察期由隔离夹具推进。它们不能证明真实云 IAM、真实云清理或实际等待24小时。Web／Taro H5录制不能替代四端正式发布验收。

保持用户修订99：不进行真机、网络性能、历史迁移、容灾或 AI 费用控制。保留正常 schema 升级、OSS＋R2复制、普通恢复、秘书权限／隔离／审计及技术资源限制。当前23个工作包仍开放（14 P0／9 P1），5项移出；完整源码仍需按当前缺口清单继续推进。
