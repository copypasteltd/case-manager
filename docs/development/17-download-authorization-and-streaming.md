# 下载授权、固定版本与受控流式读取

日期：2026-10-23。适用[文件需求](../requirements/06-files-and-search.md)、[身份与分类](../requirements/02-identity-and-permissions.md)、[存储协议](03-storage-protocol.md)。实现位于app/backend/internal/files/downloads.go、internal/httpapi/downloads.go、internal/objectstore/download.go及app/web/src/download.ts、DownloadDialog.tsx；进度以[实施台账](../delivery/02-implementation-ledger.md)为准。

## 1. 授权对象与不变量

元数据详情、历史列表、文件ID、版本ID、上传会话ID、下载许可ID都不构成读取凭证。创建许可时必须重新核查file.read、当前组织与用户权限版本、有效登录、渠道、文件及所有祖先目录状态。管理员没有内容授权时仍然拒绝。归档项目可按现有阅读权限访问保留文件，不以项目不可写为由自动拒绝阅读。

许可固定逻辑文件、确切历史版本、已验证副本、provider/bucket、唯一对象Key、ETag及可用的OSS原生版本号、大小、原文件名、分类和处理方式。来源必须有完整SHA-256匹配、verified_at和可解析的冻结来源记录，且目标桶显式启用并配置下载用途。state='verified'本身不够；草稿、候选、另一个文件/版本的Key不能转为下载地址。

只允许available且clean，或持有正式放行引用的waived版本。pending、blocked/quarantined、unscannable及missing拒绝；回收站文件先恢复业务状态。waived始终保留风险提示，不允许原位inline预览，不转换成clean。

Web/微信/抖音/Android人工登录可按权限申请；TG、worker、携带run_id的Agent不得取得原件bearer URL。后续秘书使用受限内容工具与独立披露流程，不能复用这个人工原件下载入口。

## 2. 两种访问方式

下表说明 Web／Android 的常规选择。2026-10-24起微信、抖音人工会话为普通及机密资料统一使用 authenticated_proxy；有界附件内容的 JSON 分段扩展见[移动固定内容协议](46-mobile-fixed-content-reading.md)。渠道仍来自服务端当前会话；不能由请求自行指定更宽松方式。

| 内容 | 普通资料 | confidential |
| --- | --- | --- |
| 方式 | signed_url，直接向对象存储GET | authenticated_proxy，通过API按范围读取 |
| 云地址 | 临时响应返回，不持久化 | 客户端不会取得对象存储签名地址 |
| 原始许可 | 候选实现上限12小时；到期重新申请 | 同左，仍需每次请求重新鉴权 |
| 云签名 | 最长5分钟，不超过许可剩余时间 | 无 |
| 读取约束 | 必须带签入的If-Match，OSS同时固定原生版本 | 身份+设备会话家族+渠道+权限版本+固定副本 |
| 撤权边界 | 新许可/续签拒绝；已有云短签在到期前可能仍可开始读取 | 新请求拒绝；在途每2秒检查，失败取消源请求并关闭响应体 |
| 大文件流量 | 不默认经过应用机 | 每次最多32MiB，明确限速与并发限制 |

短签到期限制是提供商受理请求的条件，不能承诺正在进行的云读取恰在第5分钟停止。confidential检查周期也不代表已经发出的网络缓冲字节能够被收回；进程暂停、网络栈与代理配置仍需部署演练验证。

正常access/refresh轮换保留同一设备的session_family_id。许可记录初始session_id供溯源，授权比较当前有效会话与固定家族；另一设备、新登录家族、另一用户或另一渠道不能接续。旧access立即无效；退出、刷新重放撤族和停用后，家族标识本身不能代替有效登录。

读取可选择已配置、有完整证据的次端；不会修改active_primary或自动触发写入故障切换。一次许可只绑定一个物理副本，不在同一响应内悄悄切换来源。固定来源被修复/停用后，旧许可失效，重新申请。

## 3. API与幂等

`POST /v1/file-versions/{id}/download`，请求`{"disposition":"attachment"}`或受支持的inline。响应包含许可ID、模式、临时URL、GET方法、required_headers、地址与许可各自到期时间、固定版本ID、文件名、十进制大小、SHA-256、扫描状态、实际disposition、范围上限和警示码。

命令事务只保存非秘密permit_id、固定许可、审计和outbox。相同幂等标识重放前重新鉴权；事务结束后再次核查许可、生成新的短签，并在返回前复核一次。响应中的URL与签名请求头不会写入幂等结果、审计、outbox或聊天。

幂等许可到期后返回DOWNLOAD_EXPIRED。客户端申请新的许可必须使用新幂等键，仍固定原版本与摘要；不能因为文件产生了新版本而把未完成下载切到current_version。

`GET /v1/downloads/{id}/content`只支持原件鉴权代理模式。ID不含访问能力，必须携带人工登录。HEAD不启动读取。错误保持标准JSON错误信封；成功返回二进制，发送文件前后不能混入JSON。

## 4. 范围、资源和安全响应

| 项目 | 当前候选实现 |
| --- | --- |
| Range | 单个明确闭区间`bytes=start-end`；不接受多范围、开放尾部、后缀、越界或非规范十进制 |
| 无Range | 仅允许大小不超过32MiB的完整对象；更大返回416及明确错误码 |
| 单请求上限 | 32MiB；成功范围返回206、Content-Range与准确Content-Length |
| If-Range | 可携带逻辑版本SHA-256强ETag；不匹配返回412，禁止降级为整个120GB文件 |
| 并发 | 每进程总计3个代理读取、每用户1个；超出429，不无限排队 |
| 带宽 | 每个代理流候选上限2MiB/s，64KiB分块平滑发送 |
| 超时 | 代理请求2分钟；每次写入刷新30秒写期限；云客户端自身限制连接/握手/响应头等待 |
| 显式缓冲 | 代理64KiB，不写服务器本地对象文件；不据此声称SDK/整个进程只有64KiB内存 |

固定范围采用源If-Match、明确原生版本及精确长度，拒绝来源回执不符。最后一块发送前检查EOF，拒绝短流和多余字节。失败若已发送部分内容，结束不完整响应并记录partial，不能追加错误JSON或谎称完成。

默认attachment+application/octet-stream；文件名用规范化Content-Disposition编码，拒绝控制字符、路径分隔符与非法名称。inline仅开放clean的PNG/JPEG/WebP/GIF/PDF/text/plain，排除HTML/SVG等活动内容；这不代替独立预览沙箱或PDF生成器验收。代理设置no-store、nosniff、no-referrer及限制性CSP，不让受限内容变成应用脚本。

并发/速率门限是首期单API进程方案。部署多进程前必须引入统一的容量控制或按实例重新核算，不能把单进程限制写成全站保证。反向代理应关闭该路径缓冲和内容转换，保留Range/ETag/Content-Length及必要超时；不得缓存机密响应。

## 5. Web保存过程

页面先说明所选内容版本和扫描状态，获取授权后，用户再次点击选择本机保存位置。原生保存选择器必须在用户点击中调用，不能在等待网络后失去用户激活。签名地址仅保留在当前下载内存，未提供复制URL、日志、localStorage或聊天入口。

Web以最多32MiB范围顺序fetch，响应体逐块写入FileSystemWritableFileStream。逐段核对206、总大小/起止位置、Content-Length与固定ETag；整文件不使用blob()/arrayBuffer()。只有完整接收所选版本才close提交；取消、范围不符或写盘失败调用abort放弃未完成保存。当前实现不计算浏览器侧整文件SHA-256，完成提示只说明接收长度一致；云端完整校验和条件读取证据单独存在。

每段前读取/me的session_expires_at；不足3分钟时在段间续期。共享续期任务避免同时下载导致重复旋转，迟到的旧401读取在其他请求已续期时只重发安全读取，不再次旋转。业务写命令仍不自动重放。短签接近到期时重新申请同一固定版本的授权，12小时许可接近到期时使用新幂等键。

当前支持相应文件保存接口的桌面浏览器，并要求安全上下文；其他浏览器显示能力限制，不偷偷把120GB改走应用代理或整文件Blob。当前下载对话框离开页面会取消，缺少跨页面/刷新恢复的下载队列；上传的全局可恢复队列与Android原生传输仍按原需求继续实现，不能用这里替代。

参考依据：[MDN保存选择器与用户激活](https://developer.mozilla.org/en-US/docs/Web/API/Window/showSaveFilePicker)、[文件流写入和提交](https://developer.mozilla.org/en-US/docs/Web/API/FileSystemWritableFileStream/write)、[Response.body流](https://developer.mozilla.org/en-US/docs/Web/API/Response/body)。这些浏览器能力存在兼容边界，首发矩阵必须实测。

## 6. 云用途、签名和CORS

`CP_DOWNLOAD_PROVIDERS=disabled`为默认；显式设置oss、r2或oss,r2。它独立于CP_STORAGE_DRIVER，主写端停止不自动阻断已配置的次端只读。

每个列出的端必须明确提供DOWNLOAD_SIGNER及DOWNLOAD_READER身份，各自的ACCESS_KEY_ID/ACCESS_KEY_SECRET/可选SESSION_TOKEN；端点/region/bucket沿用提供商明确配置。不回退到UPLOAD、PROMOTE、REPLICA或SDK共享身份；启用但缺失用途身份则启动失败。

DOWNLOAD_SIGNER只需要获准正式版本区的读取签名能力，DOWNLOAD_READER只读相同正式区；应用机不会向客户端分发原始密钥。OSS V4显式将if-match加入additional signed headers，R2签名覆盖If-Match；普通超链接无法附带此请求头，必须使用受控客户端。

云CORS须按实际Web域名允许GET/OPTIONS及Range、If-Match，显式暴露ETag、Content-Range、Content-Length；不要为带应用凭证的跨域流开放泛域。对象存储请求不携带应用Cookie/Authorization，拒绝重定向，不转发Referrer。R2短签采用其S3端点，不能凭空切换到自定义域。

依据：[R2短签规则](https://developers.cloudflare.com/r2/api/s3/presigned-urls/)、[R2 GetObject兼容性](https://developers.cloudflare.com/r2/api/s3/api/)、[OSS GetObject条件与响应覆盖](https://help.aliyun.com/zh/oss/developer-reference/getobject)。SDK离线签名检查不等于真实提供商已经接受全部响应覆盖、CORS和IAM设置；这些属于真实云放行检查。

## 7. 审计与异常

迁移0022建立不可变download_permits和download_read_events，应用身份仅SELECT/INSERT。代理读取前在短事务保存started与审计，失败不打开云流；云网络操作始终位于事务外。结束以独立短事务记录completed、partial或failed及请求范围、bytes_written、受控原因码。

bytes_written表示应用成功写入HTTP响应的字节，不代表用户设备已经持久保存或实际阅读。进程崩溃可能只有started而无结束事件，不能补写completed。结尾审计失败会记录受控故障，但不能承诺已经发出的字节能够撤回。后续异常观测与告警需接状态模块。

常见可操作结果：DOWNLOAD_SOURCE_UNAVAILABLE表示没有配置完整且校验合格的副本；DOWNLOAD_SOURCE_CHANGED/下载范围不符须刷新固定来源；DOWNLOAD_EXPIRED须新申请；AUTHORIZATION_CHANGED/DOWNLOAD_SCOPE_CHANGED须重新核对权限；DOWNLOAD_BUSY稍后再试。

## 8. 验证与待补证据

原生PG隔离测试覆盖许可重放新签且秘密不落库、当前权限与分类变化、confidential家族隔离/正常续期/退出、过期、未验证副本/隔离版本/无内容权限管理员、次端只读回退、签名期间撤权、审计失败、短流/长流、中途撤权关闭连接及每用户并发限制。SDK单元检查两端签名条件/正式Key/TTL和活动内容拒绝；前端检查顺序范围、来源/长度不符、取消、权限凭证隔离、授权更新和120GB数值下的有界请求。

这些测试使用受控生成流和隔离数据库，不访问真实OSS/R2，不构成120GB实际下载、真实CORS、香港网络、机密代理长任务或本机保存选择器的端到端验收。实际命令结果、源码摘要与浏览器证据见[实施台账](../delivery/02-implementation-ledger.md)。[历史内容恢复](19-history-restoration.md)已有独立复制/提交与Web任务；真实云及Android、独立预览、下载恢复队列与保留清理继续按完整需求交付。
