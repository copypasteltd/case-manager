# 双云副本执行、恢复与维护

日期：2026-10-23。补充[存储协议](03-storage-protocol.md)；实现位于 app/backend/internal/files/replica_jobs.go、replica_retry.go、objectstore/copy.go、uploadworker/runner.go 与迁移0021。本文描述代码行为；真实 OSS/R2、120GB、跨境链路与费用仍需独立验收。

## 1. 复制单位与权威证据

复制单位为已提交的 file_version，固定 file_id/version_id/size_bytes/sha256。主端提交时已有固定对象和完整回读证据，次端只记 queued；不得把“存在两条副本记录”显示为完成两地备份。

Outbox 的 upload.committed 包含 replica_id、replica_version、来源/目标提供商、版本、大小、SHA-256 与存储纪元。独立消费者 file.replication.v1 只解析固定输入，不访问云；按副本ID及发起版本去重。replica.retry.requested 采用同一契约。

复制前后每个检查点均核对：当前存储纪元/主端、正式版本摘要和大小、来源副本 verified 且实际摘要/时间/固定源齐备、双方 storage_targets 显式启用、执行器提供商和桶与数据库一致。不存在可用固定源时拒绝推进。

来源员工停用、项目归档、文件进入回收站不撤销已提交版本的备份维护。后台任务依据保留的正式版本运行；不能用上传者当前是否还能编辑项目来决定历史版本是否需要备份。来源/目标故障也不自动切换写主端或修改正式版本内容。

## 2. 状态与通道

`queued → copying → verifying → verified`。可恢复错误进入 retry_wait；不可恢复错误或耗尽尝试进入 failed。主端已验证副本和正式版本的 availability 不因次端失败被改写。

| 项 | 当前实现 |
| --- | --- |
| 小任务 | ≤100,000,000字节；storage.replica.small |
| 大任务 | 大于100,000,000字节；storage.replica.large |
| 进程 | worker-replica-small / worker-replica-large |
| 每进程 | 同时一个任务；顺序分片，默认32MiB |
| 内存 | 固定源流式传输及256KiB摘要缓冲；不全量读取文件 |
| 租约 | 120秒，30秒心跳；接管递增 fencing_token |
| 超时 | 小任务30分钟，大任务12小时；超时后保留可恢复事实 |
| 重试 | 上限8次，受控退避/抖动；错误不保留原始云响应 |

大小车道隔离避免大文件长期占用小任务；当前没有实现每任务两片并行、带宽限额或剩余时长预测，不能据此承诺吞吐。120GB在实际链路上的完成时间必须计复制和目标完整回读。

## 3. 云端恢复与物理Key

复用已确认 multipart upload_id 前先 ListParts，对照固定分片布局校验片号/长度/ETag，已确认片不重复传输。分片从固定源 Range 读取，应用不落整份原件。源范围响应与 ETag/原生版本不符时拒绝记录成功。

初始化前先持久化 object_transfers 日志。若 Begin 成功与否未知、数据库没有确认 upload_id，重试会把旧日志记 uncertain，使用新的物理Key初始化。次端Key为：

`org/{org}/files/{file}/versions/{version}/replicas/{attempt}/original`

逻辑版本保持不变。[历史内容恢复](19-history-restoration.md)使用同类尝试Key，但会生成新的逻辑版本；它与维护既有版本的跨云副本是两个独立任务。保留不确定尝试供后续清理，不能在同一个Key上重新 Begin 造成旧执行者覆盖新对象。

已知 multipart 的完成响应丢失时，先 Head 唯一Key对账；存在后冻结对象证据，再按固定对象完整回读计算SHA-256。长度相同、ETag相同或元数据宣称摘要相同都不足以通过验证。完整摘要检查点可复用，崩溃恢复不用再次读取已持久化确认的前缀。

## 4. 事务与租约

每个短事务依次获取组织共享锁、项目/公司文件名称空间锁、副本行锁、任务租约锁、传输日志锁。云请求在事务外执行。所有正向检查点重新核对固定输入、来源与纪元；旧租约无法写片回执、摘要进度、失败状态或完成状态。

最终 verified、实际SHA-256、固定目标源、verified_at、审计、Outbox及任务 succeeded 同事务提交。审计失败时不能留下未经审计的 verified。任务可据已保存目标及摘要检查点恢复。

运行失败的副本状态与任务 retry_wait/dead 同事务更新。进程可能在最后一次租约中崩溃，执行器会有界扫描已耗尽任务，把相应副本标 failed 并记 ATTEMPTS_EXHAUSTED；也涵盖尚未来得及写入 active_job_id 的首次业务事务。不会将永远 copying 作为正常终态。

## 5. 手动重试

POST /v1/file-replicas/{id}/retry，输入 expected_version、reason。四个人工登录渠道可调用，当前不接受Agent运行身份。需要当前文件 file.update 权限，并在重放幂等结果前重新鉴权；系统管理员不因管理角色获得内容权限。

允许 failed/missing，或原任务已 dead/cancelled 的恢复。现有任务仍 running/queued/retry_wait 时返回 REPLICA_JOB_ACTIVE，防止平行建立第二个复制任务。手动重试递增副本版本、清除旧验证结果，重新排队并记录原因；同幂等键不重复创建任务。

存储写主端已切到目标提供商时返回 STORAGE_REPAIR_PLAN_REQUIRED；该命令不代替切主隔离、全域恢复或迁移计划。没有已验证来源则返回 REPLICA_SOURCE_UNAVAILABLE。文件祖先已不可见时拒绝人工命令，但不影响已提交版本的后台备份继续执行。

FileReplica DTO 新增 version，用于展示当前事实和提交准确的乐观锁；版本列表聚合双云状态时仍检查双方摘要及验证时间。

## 6. 配置与原生部署

CP_STORAGE_DRIVER 选择来源主端。来源使用该提供商 REPLICA_SOURCE，目标使用另一提供商 REPLICA_TARGET；双方端点/桶显式配置。不能继承上传、promotion、共享SDK或系统云身份。未配置时不启动执行器。

来源最小云权限为正式固定对象的读取/Head；目标为指定副本前缀的 multipart 初始化、上传、列片、完成和固定对象读取/Head。无清理删除/全桶管理权限。实际权限必须在云账号上配置并用真实拒绝测试验证；代码角色名称本身不能约束一个过宽的云密钥。

[原生服务候选](../../app/deploy/native/copypaste-replica@.service)按 small/large 实例提供；尚未安装或启用。每实例独立环境文件、独立系统用户，移除MFA/模型/迁移密钥。本地脚本过滤无关云用途变量；生产数据库角色的进一步细分仍按[执行器与身份](12-upload-worker-and-cloud-identities.md)验收。

## 7. 验证边界

原生PG与受控对象源场景覆盖保留版本维护、分片恢复、不确定初始化/完成、内容损坏与重试、旧租约、纪元切换、审计回滚，以及执行中/首次业务事务前耗尽。最新整体结果、源码摘要和执行时间以[后端验证记录](../quality/evidence/backend/result.json)为准；测试名字与父测试不重复计数。

这些测试没有连接真实OSS/R2，也不是120GB传输证据。首次2TB迁移、云端凭证撤销/旧写者隔离、双向灾备恢复、周期巡检、主端对象缺失探测、历史保留/孤儿清理、带宽与费用测量仍需继续实现或外部验收。当前不会自动删除来源或次端对象。
