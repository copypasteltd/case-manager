# 文件正文索引运维 / File text indexing operations

[Office 扩展与格式边界](../development/120-office-text-extraction.md) · [功能与协议](../development/92-file-text-search.md) · [中文使用指南](../README.zh-CN.md) · [English guide](../README.en.md)

## 1. 原生配置 / Native configuration

执行当前全部迁移（增量120为92）后，以应用数据库角色运行 `platform worker-file-text`。不得使用迁移／数据库超级用户身份运行常驻worker。Linux模板：`app/deploy/native/copypaste-file-text-worker.service`。本地仅允许原生工具，`python app/scripts/run_backend.py worker-file-text`会将CP配置缩减至数据库、明确provider和独立文本读取角色。

```dotenv
CP_ENV=production
CP_DATABASE_URL=postgresql://application-role:REPLACE_PRIVATELY@database.example/case_manager?sslmode=verify-full
CP_TEXT_INDEX_PROVIDERS=oss,r2
CP_OSS_ENDPOINT=https://YOUR-OSS-ENDPOINT
CP_OSS_REGION=YOUR-REGION
CP_OSS_BUCKET=YOUR-FORMAL-BUCKET
CP_OSS_TEXT_INDEX_READER_ACCESS_KEY_ID=REPLACE_PRIVATELY
CP_OSS_TEXT_INDEX_READER_ACCESS_KEY_SECRET=REPLACE_PRIVATELY
CP_OSS_TEXT_INDEX_READER_SESSION_TOKEN=
CP_R2_ENDPOINT=https://YOUR-ACCOUNT.r2.cloudflarestorage.com
CP_R2_REGION=auto
CP_R2_BUCKET=YOUR-FORMAL-BUCKET
CP_R2_TEXT_INDEX_READER_ACCESS_KEY_ID=REPLACE_PRIVATELY
CP_R2_TEXT_INDEX_READER_ACCESS_KEY_SECRET=REPLACE_PRIVATELY
CP_R2_TEXT_INDEX_READER_SESSION_TOKEN=
```

以上仅为占位示例，实际密钥放在Git外环境文件，限制文件读取者。只允许读取对应组织正式版本路径；不授予写对象、删除、签名、列举无关bucket或任意URL访问。IAM／R2策略与实际路径必须在目标云另行验证。本地fixture不证明云策略有效。`disabled`或空值不会启用读取器；显式启动worker但未配置独立读取器时启动失败。

Keep real credentials outside Git and restrict the environment file. Grant read access only to the required formal-version object prefix. Verify the provider policy and target network independently. Empty/disabled providers do not enable extraction; starting the worker without a dedicated reader fails. No other role or ambient SDK identity is reused.

## 2. 状态与排查 / State and diagnosis

| 页面状态 / State | 排查动作 / Action |
| --- | --- |
| 正文待提取 / Pending | 核对worker运行、数据库迁移、扫描clean、任务lane与available_at；不把人工扫描放行当clean / Check worker, schema, clean scan and queue availability |
| 不支持 / Unsupported | 核对MIME、编码、大小；保留原件，使用名称查询或提供受支持的文本版本 / Check MIME, encoding and bounds; use the original/name search |
| 被阻止 / Blocked | 核对回收状态、有效祖先、版本隔离和来源指纹变化；按业务流程处理原件 / Check lifecycle, ancestry, quarantine and changed source |
| 失败 / Failed | 核对安全错误码、完整字节／摘要、格式损坏和尝试上限；勿打印正文、对象密钥或云凭据 / Inspect safe codes and evidence without logging private content |
| 已收录 / Ready | 对比固定版本ID、来源SHA、文本SHA、提取器、时间；改名不重写原件，正式新版本独立提取 / Compare exact version/hash/extractor/timestamp evidence |

正式原件字节继续由 OSS＋R2 保存；PG保存授权、版本与提取证据。普通文件版本恢复继续使用既有业务流程。派生正文终态不能直接UPDATE／DELETE；格式升级采用新的提取标识，不重置旧结果。迁移92将XLSX／PPTX选为office-v1，旧text-v1 unsupported结果保留；升级本身不回填正文。

Original bytes remain in OSS/R2 while PG stores authorization, formal-version and derivation evidence. Ordinary file/version recovery remains available. Terminal derivatives cannot be reset. Schema92 selects office-v1 for XLSX/PPTX and preserves all prior text-v1 results, without object reads or body backfill during upgrade.

## 3. 当前验证边界 / Current verification boundary

增量120核对受控准确对象来源、真实本地PG／队列／HTTP、Web与Taro H5以及双小程序／Android本地产物。XLSX／PPTX仅支持已说明的文字子集，原件8MiB、正文2MiB；公式不计算、缓存可能过期、隐藏内容按完整文件权限读取。PDF/OCR、复杂DOCX与原件版式仍开放。

实际云端独立身份、对象可读性及生产运行验收仍属于GAP-20／24／25；不以本地合成来源代替。用户移出的真机、网络性能／实网120GB专项、历史迁移／容灾和AI费用控制不作为当前验收门槛。AI权限、隔离、审计和技术资源限制保留。

Local proof covers exact controlled objects and the native pipeline, real Web/Taro H5 pages and compiled client artifacts. It does not establish live cloud or production acceptance. The user-excluded device, network-performance/live120GB, historical migration/DR and AI-cost gates are not reintroduced. Authorization, isolation, audit and resource bounds remain required.
