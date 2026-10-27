# 文件正文索引运维 / File text indexing operations

[功能与协议](../development/92-file-text-search.md) · [中文使用指南](../README.zh-CN.md) · [English guide](../README.en.md)

## 1. 原生配置 / Native configuration

执行迁移75后，以应用数据库角色运行 `platform worker-file-text`。不得使用迁移／数据库超级用户身份运行常驻worker。Linux模板：`app/deploy/native/copypaste-file-text-worker.service`。本地仅允许原生工具，`python app/scripts/run_backend.py worker-file-text`会将CP配置缩减至数据库、明确provider和独立文本读取角色。

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

原件和派生文本分别备份：对象存储保存正式字节，PostgreSQL保存授权、版本、索引与提取证据；仅备份OSS/R2不包含这些数据库事实。恢复后先验证数据库／对象的对应关系和正式版本可读性，再开放检索。终态不能直接UPDATE／DELETE；故障修复须保留证据，新格式适配通过版本化迁移与提取器升级处理。

Back up PostgreSQL as well as object storage: derivation evidence and authorization live in the database. After recovery, verify source/version correspondence before exposing search. Do not reset or mutate terminal evidence in place. Future adapters require a versioned extractor migration.

## 3. 验证与生产限制 / Acceptance

本轮验证覆盖受控对象来源、真实本地HTTP／数据库、Web与Taro H5、新19页双小程序编译和Android开发包。生产前仍需实际对象读取策略、香港链路、2TB索引负载／数据库容量、提取资源限制、监控、备份恢复及真实设备验证。PDF/OCR/Excel等格式保留未接通状态，不将名称命中标为正文已收录。

Current proof uses controlled object bytes with native PostgreSQL/HTTP and real browser UI, plus compiled client artifacts. Production acceptance still requires actual cloud credentials/network, representative data scale, resource measurements, backup recovery and devices. Unsupported formats retain explicit state and name-only search.
