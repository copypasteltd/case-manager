# 大文件上传、正式提交、复制与清理协议

版本：v1.0-draft；日期：2026-10-23；实现 FILE-01—14。提供商能力来源见原 [存储方案](../04-storage-backup-and-recovery.md)和 [核实记录](../07-research-sources.md)。

## 1. 存储命名与身份

建议 Key：`org/{org_id}/drafts/{session_id}/source`、`org/{org_id}/files/{file_id}/versions/{version_id}/original`；副本使用同一逻辑版本下的独立尝试Key，详见[副本实现](16-cross-cloud-replication.md)。派生物含原 version_id 与生成器版本。客户端名称只存元数据，不拼对象路径。版本 Key 不复用；是否启用 OSS 原生版本另作保护，不作为跨云语义前提。

API 签名身份只可给当前会话 draft 的特定分片，不能 Delete/List 全桶/覆盖正式 Key。promotion/replica/backup/cleanup 使用各自最小权限身份；管理账号不作为日常 Worker 凭据。R2 使用独立 S3 兼容适配器，不能依赖其未支持的 versioning/replication API。

## 2. 上传阶段协议

1. **create**：鉴权目录/目标文件与分类，校验大小≤120,000,000,000、平台能力与 quota；固定 base_version、part_size、part_count、storage_epoch；创建 PG 会话和云 multipart，云创建失败可重试对账。
2. **sign_parts**：提交有限 part_no 列表，后端重查会话 owner、状态、过期与纪元，只发对应签名/必要请求头。CORS/域名允许平台 origin，暴露必要 ETag，不允许任意来源使用公司凭据。
3. **put_part**：客户端按偏移读取有限 buffer，默认并发 2；同片重试用同 part_no。客户端可 ack ETag 方便恢复，但不能据此相信云端必然存在。
4. **resume**：服务器 ListParts 与 PG 状态合并；云端结果为准，返回已确认片段、缺失片段、过期信息；不同源文件需拒绝恢复。会话过期/云端被回收则明确新建，不假显示继续。
5. **finalize**：使用 Idempotency-Key 和 expected_version；禁止新签名；服务器分页核实全部片号/大小/非末片等长/ETag，完成 multipart 并取得固定对象 source_id，写入 verifying job。延迟到来的旧分片不能覆盖已经冻结源。
6. **verify/promote**：流式计算固定源 SHA-256/size，安全扫描或记录受控 waiver；必要适配器检查后，服务端生成客户端不可写的正式候选 Key。若云提供源版本/条件复制，校验条件；否则只有冻结唯一对象的服务器受控流式读取。120 GB promotion 不能走单次小对象 Copy 假设。
7. **ready**：正式候选对象已存在/检验，upload_session ready；尚未修改 files.current_version_id。
8. **commit**：再查权限/expected_version/base_version/epoch；新文件分配 file_id，事务插 file_version、更新 current、audit/outbox/idempotency result。副本 job 由 outbox 唯一建立；返回 file_id/version_id、主可用和 replica queued。

finalize 与 commit 为不同命令；可由客户端自动串接，但 UI 必须分阶段。服务端 job 可负责固定授权下自动 commit，仍走同用例与版本检查；冲突则 ready 草稿等待处理。

upload_session 的 version 保护生命周期、目标和授权边界；签分片、写入分片确认不递增该生命周期版本，避免两片并发互相制造冲突。分片自身按 `(session_id,part_no)` 幂等更新；finalize/abort/commit/过期会推进生命周期版本并阻止旧状态继续签名。最终一致片清单以云端对账为准。

## 3. 内存、磁盘与会话恢复

上传端/复制端只使用有上限的分片或更小 stream buffer；总内存模型为并发流数×每流 buffer+解析余量，不与文件全长成比例。32 MiB 分片不意味着每个请求必须额外复制多份 buffer。禁止 base64 编码整个文件或读取到 JS ArrayBuffer 全量再上传。

Android：SAF/平台文件选择取得可持续 URI 权限，原生 worker 流式读，队列元数据本机安全持久化，服务器会话外置。应用升级/杀进程/网络变化后按云端已完成片恢复；进度事件桥接 Taro，不经 JS 传整片字节。Web 不能假设刷新后仍可拿旧 File 对象，恢复需用户重选并确认源指纹。

上传进度至少区分 selected/uploading/paused/finalizing/verifying/ready/committed/replicating/verified/failed。百分比上传只由已确认字节算，校验与复制显示独立阶段，不能“100%”同时声称双云完成。

## 4. 双云复制协议

`queued → copying → verifying → verified`；错误进入 retry_wait，耗尽 failed，源缺失 missing。复制固定源 file_version/provider_source_id/sha256/storage_epoch，目标 Key 属于同一逻辑版本；结果未知的初始化使用新的物理尝试Key，不读取不断变化的“文件最新名”。

Worker 领取带 fencing_token 的 job，建立目标 multipart 后保存 upload_id；每片按 source Range 有界读流写目标，云确认后保存 offset/size/ETag。中断后先 ListParts 对账，不从 0 重传；源版本不符或 epoch 变化停止。只有有效租约可提交控制状态。

大任务 1 个、每任务 2 片为候选，小任务单独车道；带宽限额、队列年龄、estimated remaining 与网络费用可观测。合并/多片 ETag 不是全文件摘要。目标必须读取或通过已验证的等效完整内容校验途径得到与权威 SHA-256 匹配证据，才 verified；单纯比较 Content-Length/传递元数据不够。

小文件复制 p95 15min 仅正常链路、≤100MB 车道目标；120GB 在 100Mbps 纯传输约 2.67h，校验和重试另计。初次2TB在100Mbps单遍约44.4h；必须在计划/成本中计首迁与回读校验。

## 5. 并发、失败与回收

| 故障点 | 可恢复事实 | 处理 |
| --- | --- | --- |
| 云创建成功、PG 未记录 | 可发现的 draft/upload 标签或受控前缀 | 对账/回收孤儿，不创建正式文件 |
| 客户端断线 | PG 会话+云 ListParts | 补缺片；不以客户端缓存认定成功 |
| 完成 multipart 超时 | 同 draft Key/source_id 状态 | 查询云对象/上传状态后决定重试，禁止直接另覆盖 |
| 校验/扫描失败 | 固定源、错误与候选对象 | quarantined/failed；不把附件挂入正式申请 |
| 正式对象存在、DB commit 冲突 | ready 候选+旧 base | 保留草稿，返回 409，人工/秘书基于新版本重做计划 |
| DB 已提交、响应丢失 | 幂等结果+version ID | 返回同正式版本，不再追加版本 |
| 主端失联 | 已验证副本与 epoch | 只读切换可先做；写切换必须隔离旧主、审批纪元 |
| 旧 Worker 恢复 | 旧 fencing_token/epoch | 无权提交状态/签发新上传；限制已有旧云凭据并对账 |

cleanup 分为过期未完成 multipart、未引用 draft、已标记过期正式历史。每种独立策略和 dry-run 报告。法务保留、签署/交付引用、有效恢复集引用优先；删除主端不触发副本即时清除。R2 bucket lock 仅作为辅助保留控制，不能声称普通管理员也无法改变。

## 6. 完成证据

每个真 120 GB 测试保存：实际字节/源与目标 SHA-256、云 upload IDs（脱敏）、每次中断点/恢复缺片、时间/吞吐/并发、峰值 RSS/临时盘、费用样本和清理结果。三个独立测试为 Web 上传、Android 上传、跨云复制；互相不能替代。
