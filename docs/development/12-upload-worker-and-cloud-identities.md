# 上传控制执行器与云身份配置

日期：2026-10-23。关联[上传控制](11-upload-control-and-freezing.md)、[队列协议](05-events-jobs-and-concurrency.md)和[原生部署](../operations/01-deployment-and-operations.md)。实现入口为 platform worker-upload；该执行器仅负责 multipart 初始化及固定源封存；独立校验进程及ClamAV大小边界见[完整性与扫描](13-upload-integrity-and-scanning.md)。

## 1. 进程边界

API 负责身份、上传会话命令、限定分片签名及按会话对账。worker-upload 是独立原生 Go 进程，从 PostgreSQL Outbox 与任务队列领取受控工作；不开放 HTTP、执行任意 shell、调用模型或读取浏览器会话/MFA密钥。

每轮最多投递20条上传事件，再领取1个 storage.control 车道任务；不会因持续事件流而只转发不执行。单进程并发1，初始化/封存为控制操作，不能把这个车道作为120GB扫描/复制车道。空闲轮询500ms；数据库/投递异常退避5秒，仅记录受控原因码。

每个任务租约120秒，每30秒续租；心跳失败取消当前调用。控制任务最长10分钟。任务结束前停止并等待自己的心跳协程；主进程中断时停止外部调用，不单独确认任务，重启后由租约机制接管。多个实例仍以数据库租约为准。

## 2. 失败、退避和审计

暂时网络/429/未知完成结果可重试；明确权限拒绝、源条件变化及不合法业务输入进入终态。重试间隔从5秒指数增加至5分钟，附向下最多20%的抖动；执行输入固定 max_attempts=8。

受控失败码、会话 retry/failed 状态、Worker审计、Outbox 与任务 retry_wait/dead 同事务。审计失败时全部回滚，保留运行租约供恢复；不通过另一个数据库调用把任务单独标记 dead。无法定位有效会话的畸形任务才只记录任务拒绝，不修改业务。

状态成功推进会清除此前的临时错误码。日志只记录 job_id/kind/code/是否终态，不输出请求URL、云正文、SQL参数、密钥或源文件名。

已耗尽最后一次且崩溃的租约可由队列 sweep 转 dead；对应上传会话的专用状态修复、人工重试页面和死信告警仍需后续实现。当前不能只看上传会话状态判断所有后台故障，任务表与租约也是恢复依据。

## 3. 显式配置及角色

CP_STORAGE_DRIVER 支持 disabled/oss/r2，缺省 disabled。API 禁用时保留人工项目/知识功能；显式启用但缺少用途凭证时启动失败，不回退个人SDK配置、AWS全局凭据或其他角色密钥。worker-upload 在 disabled 下拒绝启动。

| 设置 | API | worker-upload |
| --- | --- | --- |
| CP_DATABASE_URL、CP_ENV | 必需 | 必需；生产PG要求verify-full |
| CP_PUBLIC_ORIGIN、CP_CURSOR_KEY、CP_MFA_KEY等 | 按API既有规则 | 不需要、不读取 |
| CP_STORAGE_DRIVER | 上传模块开关 | 必须oss/r2 |
| CP_OSS_ENDPOINT / REGION / BUCKET | OSS启用时 | OSS启用时 |
| CP_R2_ENDPOINT / BUCKET | R2启用时 | R2启用时；区域由SDK固定auto |
| CP_{OSS或R2}_UPLOAD_SIGNER_ACCESS_KEY_ID / ACCESS_KEY_SECRET / SESSION_TOKEN | 分片签名用途 | 不传 |
| CP_{OSS或R2}_UPLOAD_INSPECTOR_ACCESS_KEY_ID / ACCESS_KEY_SECRET / SESSION_TOKEN | ListParts用途 | 不传 |
| CP_{OSS或R2}_UPLOAD_LIFECYCLE_ACCESS_KEY_ID / ACCESS_KEY_SECRET / SESSION_TOKEN | 不传 | multipart控制及固定对象Head用途 |
| CP_MIGRATION_DATABASE_URL | 生产API禁止 | 所有环境禁止 |

上表大括号是参数族说明，实际变量如 CP_OSS_UPLOAD_SIGNER_ACCESS_KEY_ID。SESSION_TOKEN 仅在临时身份下设置；其他两个凭证字段必需。代码不签发新的父身份、不代管云管理Token；临时身份轮换与到期提前刷新尚未自动化。

开发启动脚本在 worker-upload 模式仅传数据库、环境、存储公共参数与生命周期用途变量，过滤本地文件和父环境中的其他 CP_ 设置。生产各服务使用独立环境文件及系统账号，不加载包含全部角色秘密的公共环境文件。

## 4. 云端能力边界

本地测试仍共用非特权应用数据库角色；生产Worker专用SQL权限集合与独立数据库账号尚未部署验收。用途云身份和系统进程隔离，不能替代数据库权限隔离。

客户端只获得限定 UploadPart URL，不获得SDK三元组。服务端签名角色限定 draft 路径；提供商可支持的动作限制与应用层唯一分片接口共同约束，不能凭变量名称认定实际云权限已经安装。Inspector与生命周期角色也只给所需路径和操作；正式版本、候选、复制、删除使用其他身份。

截至2026-10-23核实，R2支持带会话令牌的临时身份，以及路径限制；精确 S3 actions 限制通过本地签发方式提供，API签发的预设权限类别较宽。可由受控签发环境提供按用途收窄的身份，应用进程不接收父秘密。来源：[R2临时身份官方文档](https://developers.cloudflare.com/r2/api/s3/temporary-credentials/)。当前尚未实现自动签发器，也未用真实角色验证跨路径/跨动作拒绝。

默认主端、提供商/桶配置必须与数据库最新 storage_epochs 及会话固定值相符；已有2TB资料量不自动生成配额/纪元。配置启用前需受控设置 quota 和初始存储纪元，确认主体区域、CORS和真实云权限。相关配置管理用例仍待完成。

## 5. 原生运行与部署产物

开发配置完成后可使用：

```powershell
python app/scripts/run_backend.py worker-upload
```

本任务当前没有云凭证，没有在该机器启动真实云 Worker。CLI接线、配置隔离与受控提供商的数据库执行闭环分别验证；不把隔离测试说成真实服务联网。

Linux 候选部署模板见 [copypaste-upload-worker.service](../../app/deploy/native/copypaste-upload-worker.service)，仅为后续发布准备，没有安装或启动。模板要求专用非特权账号、受限环境文件和固定发布二进制；只允许独立缓存目录写入。最终服务器若为Windows，另用原生服务方式，不引入WSL/Docker/模拟器。

## 6. 验证与后续

配置测试验证两个提供商的用途凭证不回退、独立Inspector、默认禁用及不安全端点拒绝。隔离数据库测试通过真实 Runner.RunOne 串接转发、领取、执行、失败与重试；覆盖未知完成结果恢复、明确云权限拒绝、自动执行审计归属，以及审计拒绝后域状态/任务一起回滚。

30秒心跳周期尚无真实云长任务证据，受控数据库租约接管已单独验证。真实云角色、凭证轮换、断网/进程崩溃演练、死信监控、带宽公平性、自动回收，真实双云等仍按独立验收推进。verifying之后的[正式候选/提交](14-file-promotion-and-commit.md)已有独立执行器与隔离验证。
