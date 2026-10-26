# 固定源完整性、恢复检查点与扫描

日期：2026-10-23。适用于迁移0018及 app/backend/internal/files/upload_integrity.go、internal/objectstore/checkpoint.go、internal/filescan/。前置流程见[上传控制](11-upload-control-and-freezing.md)，运行模型见[上传执行器](12-upload-worker-and-cloud-identities.md)。

## 1. 安全边界与实际范围

本阶段提供固定源流式摘要、可恢复检查点、扫描适配器、不可变扫描报告、重扫接口与两个校验车道。默认扫描器关闭时，文件进入 blocked/unscannable，不产生正式文件版本。扫描 clean 也只表示本次扫描策略下未发现问题；后续正式候选复制、完整内容核对和 commit 仍是独立步骤。

ClamAV 官方配置样例说明，目前存在超过2GB文件的扫描设计限制；调整 StreamMaxLength 不能消除此限制。本适配器把策略上限保守限制为2,000,000,000字节，实际值必须更小或相等，并与已部署引擎配置一致。120GB文件仍需完整计算 SHA-256；本引擎将其标为无法扫描，后续需合格的其他扫描方式或受控豁免。不能把切片逐段扫描等同完整容器分析。来源：[官方配置样例](https://raw.githubusercontent.com/Cisco-Talos/clamav/main/etc/clamd.conf.sample)。

当前没有安装或运行真实ClamAV，也没有真实云字节传输证据。已有独立的[扫描例外制度](15-scan-waivers-and-review.md)，默认没有生产策略或人员授权；关闭扫描不自动放行。

## 2. 来源和权威摘要

上传封存持久化唯一 draft Key、准确大小、ETag条件及可用原生版本。source_digest 是上述固定来源规范JSON的 SHA-256，用于绑定任务与检查点，不是文件内容摘要。文件内容 SHA-256 只能来自服务端完整读取该固定来源。

处理顺序：

1. 核对任务类型、payload版本、车道、会话生命周期版本、固定来源摘要和存储纪元。
2. 在短事务中核对当前租约和 fencing_token、原始上传人当前状态/权限、原始渠道、项目可写性、分类和提供商配置。
3. 在事务外按准确 Range 和来源条件读取，流式计算摘要；不把大对象加载到内存，不写长期本地文件。
4. 每完成一个准确范围，在新事务中重新核对上述条件并保存检查点。未完成的范围不保存。
5. 全部字节读取完成才写 final_digest；该摘要一经写入，数据库禁止更换或清空。
6. 扫描独立读取同一固定源；结果绑定 source_digest、最终SHA-256、大小和策略证据。

verified_bytes 仅是摘要检查点已确认字节，不是上传或副本进度。中断、撤权、纪元变化、项目归档、终止或租约接管均不能把已失效执行的结果写回。

## 3. 检查点格式与恢复

upload_hash_checkpoints 按组织/会话唯一，保存来源摘要、Go摘要状态格式/运行时版本、已完成偏移、有界二进制状态、校验和、可选最终SHA-256、任务ID与 fencing_token。

每个范围默认32MiB；实际读缓冲256KiB，尾范围按剩余长度。每次响应核对准确范围长度、ETag和可用原生版本，读取不足、多出字节或关闭失败都不能推进。重试从最后持久化范围后开始，最多重读一个未提交范围。最终摘要已持久化且证据一致时，重扫不重算整份摘要。

检查点校验和覆盖来源、格式、偏移、状态与最终摘要，用于发现存储损坏；它不是抵御数据库管理者伪造的签名。当前运行时还校验可反序列化的摘要状态及最终摘要。未完成检查点遇到运行时格式变化，明确记录从0重启；不会用不兼容状态继续计算。已完成权威摘要保留，并核对完整检查点的一致性。

摘要内部状态可能含最后不足64字节的源内容，必须按源文件分类保护。禁止把 hash_state、桶、Key、云上传ID、签名URL或临时凭据输出到公开API、普通日志或模型上下文。

## 4. 扫描协议与证据

Scanner 接收可关闭的固定源流，并负责关闭，包括禁用、超限、错误和取消。域服务再次关闭防止泄漏；服务独立记录实际读取字节和EOF，clean 回执须与真实完整读取一致。

ClamAV 适配器仅连接本机回环IP或本机绝对UNIX socket，不将无鉴权明文协议开放给远端。每次命令独立连接，使用NUL结束的 VERSIONCOMMANDS 和 INSTREAM；后者按64KiB块以4字节网络序长度封装，最后发送0长度。完整响应上限4096字节；取消会关闭连接及源流。协议依据：[ClamD官方协议](https://docs.clamav.net/manual/Usage/ClamdProtocol.html)。

| 引擎结果 | 域结果 | 后续 |
| --- | --- | --- |
| 准确 stream: OK，完整流、前后引擎/签名一致且策略已核验 | clean | 发出 scan.passed，等待正式候选流程 |
| 病毒/恶意检测 | quarantined，对外显示 blocked | 保持阻断，不可直接下载或正式提交 |
| 超策略大小、解密不可用、解包/嵌套限制、关闭扫描器 | unscannable | 保持阻断；可修正环境后重扫 |
| 传输失败、取消、引擎定义在扫描中变化 | 受控错误 | 按错误是否可重试记录任务，不能保存 clean |
| 多条/不完整响应、未知命令或缺少来源证据 | 拒绝/无法扫描 | 不伪称完整扫描 |

upload_scan_reports 保存不可变报告：来源摘要、权威摘要、大小、状态、受控代码、引擎/版本、签名库修订、策略摘要、实际发送字节、任务与执行凭证。报告、会话阶段推进、审计、Outbox、任务完成在同事务提交；审计失败会一起回滚，前面已经确认的摘要检查点可用于恢复。

streamed_bytes 表示发送给扫描器的字节，不能证明扫描器解析了每一层内部结构。部署必须验证 StreamMaxLength、MaxFileSize/MaxScanSize、嵌套/解包/超时上限、AlertExceedsMax、加密档案/文档告警和安全相关分析开关。人工声明不是自动探测，必须另有实际配置摘要与测试证据；原始 OK 不足以证明这些设置。上限/加密配置行为参照前述官方配置样例，生产按固定引擎版本核实。

## 5. 接口与状态

| 接口/字段 | 契约 |
| --- | --- |
| GET /v1/uploads/{id} | 增加 verified_bytes、scan_state、scan_result_id；隐藏来源内部字段 |
| GET /v1/uploads/{id}/scan-report | 本人及当前目标ACL可见最新报告；不存在返回404 |
| POST /v1/uploads/{id}/rescan | expected_version、幂等键；仅 blocked/failed 且存在固定源；仍需有效权限/纪元/期限 |
| blocked | 禁止签片、完成及正式提交；可以终止或申请重扫 |
| scan_state=pending | 尚未取得扫描结论 |
| scan_state=clean | 已取得该固定源与当前策略的有效回执；不等于 ready/committed |
| scan_state=blocked/unscannable | 公开受控阻断状态，不暴露病毒原始正文 |

重扫递增生命周期版本、清除最新报告引用及临时错误，并发出新任务事件；旧报告保留。若已有权威摘要则复用；扫描会重新读取来源。没有无限延长已过期会话，也不允许通过重扫修改源文件、目标目录或分类。

## 6. Worker与配置

事件 upload.source.frozen / upload.rescan.requested 通过独立 Outbox 消费者生成 upload.verify.v1。大小≤100,000,000字节进入 storage.verify.small，其余进入 storage.verify.large；不是按扩展名决定车道。

| CLI | 任务时限 | 共享规则 |
| --- | --- | --- |
| worker-verify-small | 30分钟 | 2分钟租约、30秒心跳、有界重试、当前租约确认 |
| worker-verify-large | 12小时 | 同上；与小文件独立领取，避免大文件阻塞票据 |

每个进程一次处理一个任务；带宽配额与生产并发上限需另行实测。只有 READ 固定来源用途的 CP_{OSS或R2}_UPLOAD_VERIFY_* 身份；不能回退生命周期/API身份。原生运行使用 app/scripts/run_backend.py 对应命令。服务候选模板见 [copypaste-verify@.service](../../app/deploy/native/copypaste-verify@.service)，未安装或启动。

| 配置 | 缺失/边界 |
| --- | --- |
| CP_SCAN_DRIVER | disabled默认阻断；仅 disabled/clamav |
| CP_CLAMD_NETWORK / CP_CLAMD_ADDRESS | tcp回环IP或unix本地绝对socket |
| CP_SCAN_MAX_BYTES | 1—2,000,000,000；必须显式匹配引擎有效能力 |
| CP_SCAN_TIMEOUT_SECONDS | 1—900秒，显式设置 |
| CP_SCAN_POLICY_SHA256 | 64位小写十六进制，绑定已核验扫描策略 |
| CP_SCAN_LIMITS_ATTESTED | 必须准确为true才接受clamav配置；不是自动核验结果 |

临时云凭据到期自动换新、扫描策略管理页面和真实配置探测仍待实现；当前运行时读取启动时明确配置。不同用途的生产进程使用独立系统账号、环境文件及最小SQL权限；本地数据库测试共用非特权应用角色不构成生产隔离证明。

## 7. 验证与尚未完成项

受控协议测试覆盖准确帧/EOF、引擎证据、恶意检测、超限/加密、未知/多记录、长短源、定义变化、超时/取消与连接释放。只使用自建本机测试监听器，结束后全部关闭。原生PG场景覆盖默认阻断与重扫、检查点重试、无证据clean拒绝、租约接管、流中撤权、报告/审计/任务原子性；没有真实扫描守护进程。

扫描例外及正式候选/commit已在[例外制度](15-scan-waivers-and-review.md)和[候选提交](14-file-promotion-and-commit.md)中独立实现。[双云回读/耗尽修复](16-cross-cloud-replication.md)与[受控下载](17-download-authorization-and-streaming.md)已有后续隔离实现。本阶段的扫描验证不覆盖真实云、上传会话续期、自动回收、2TB导入或真实120GB试验。对应功能继续按[文件需求](../requirements/06-files-and-search.md)和[实施台账](../delivery/02-implementation-ledger.md)推进，不能把受控小样本验证表述为全链路验收。
