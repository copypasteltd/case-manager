# 审计查询与固定导出 / Audit query and sealed exports

增量109实现GAP-07中的审计查询和导出。员工在同一管理入口按操作线索定位事件；获明确授权的人员可固定查询结果，再下载可核对的CSV。费用控制继续排除，秘书、八项Skill、TG、授权、审计、隔离和技术资源限制继续保留。GAP-07中的模型／Skill、TG运行与存储配置管理仍开放。

Increment 109 implements the audit-query/export portion of GAP-07. An authorized human can locate organization security metadata, seal the matching event references and download a verified CSV. This does not close the entire administration work package. Only AI cost control is excluded; secretary, Skills, TG, authorization, audit and isolation remain required.

## 操作顺序 / Usage

1. 使用已有成员账号和MFA登录。Web侧栏进入“审计记录”；微信、抖音、Android从“我的工作空间 → 管理与审计 → 查询审计记录与固定导出”进入。
2. 选择香港时间的起止日期，结束时间不包含在内。默认本月首日至明日零时，最大366天。动作、结果、来源渠道准确匹配；更多线索支持人员、执行者、资源、请求、秘书任务及后台任务标识。
3. 点击“应用查询”。修改输入时，旧列表继续标记为上次已应用结果；每次应用重置分页。可查看事件详情及前后内容摘要。
4. 有独立导出权限时填写用途，按已应用条件生成固定CSV；最多10,000条、8 MiB，有效24小时。超限整单拒绝，不静默截断。生成和下载均要求最近10分钟内完成MFA。验证过期时先在账号安全入口重新验证，再重试。
5. 在“我的导出”核对原始条件、条数、SHA-256和到期时间，再点击校验下载。固定导出只向创建者开放；权限依据变化或过期后重新生成。网络结果不确定时保留原请求编号核对，不自动重新创建。

Apply filters explicitly, inspect the technical event and supply a human export purpose. Creation freezes all matching references rather than just the visible page. Refreshing or adding later events does not change that export. Download checks identity, authority, expiration, row count, size and hashes before saving. A server read audit means the bytes were made available; it does not assert successful delivery to a device.

| 客户端 / Client | 查询与生成 / Query and create | 文件出口 / File delivery |
|---|---|---|
| Web | `/audit`；查询、详情、分页、本人导出历史 | 分段摘要及完整摘要验证后浏览器保存 |
| 微信小程序 / WeChat | 现有settings页面的`section=audit`；保留20页配置 | 平台支持`shareFileMessage`时校验后分享；临时文件结束即清理，否则提示同账号Web下载 |
| 抖音小程序 / Douyin | 同一共享业务页面，支持查询、生成和历史 | 当前不接原生CSV文件分享；明确提示同账号Web下载，不把H5下载等同于抖音文件能力 |
| Android | 同一共享页面和约束 | 校验后使用现有原生文件保存桥；正式签名和发布仍按GAP-16验收 |

本地Taro H5是共享页面的浏览器验证入口，不替代微信／抖音平台发布验收或Android设备验证。真机验证、网络性能／实网120GB专项、历史迁移与容灾均不作为当前门槛。

## 权限与披露 / Authority and disclosure

`audit.read`是明确授予的**组织级安全元数据权限**，`audit.export`另行授予；导出同时需要两者。系统管理员的技术审计访问不会获得项目、合同等业务正文读取权。请仅向需要查看全组织操作线索的人员授予这些权限。现有管理员角色已包含相应权限，本轮不自动向普通成员添加授权。

查询及导出仅允许有效的人类交互会话（Web／微信／抖音／Android），拒绝AgentRun和TG执行会话。查询游标绑定账号、组织、渠道、授权版本和完整筛选条件。下载每段重查当前会话和权限；客户端在保存前再次核对当前身份、权限及固定回执。

The disclosure identifier is `organization_metadata_v1`. This is an explicitly granted organization security plane, not a new business-content bypass. Only the following allowlist is returned: event ID/time, actor/executor IDs, channel, action, result, resource type/ID/version, request/run/job IDs and before/after digests. Free-form reasons, session IDs, payloads, business titles and document bodies are excluded from this generic API and CSV. Export purposes remain in the protected database/audit trail and are not echoed in public receipts.

## 接口与数据 / API and data

所有路径以`/v1`为前缀，使用既有JSON信封和错误约定；CSV二进制下载除外。完整定义见[OpenAPI](openapi.json)，Web和移动端共享生成类型与`contracts/audit`校验器。

| 方法 / Method | 路径 / Path | 行为 / Behavior |
|---|---|---|
| GET | `/audit/events` | 必填from/to；准确筛选、签名游标、limit 1–100，默认20 |
| GET | `/audit/events/{id}` | 当前组织的技术事件详情 |
| POST | `/audit/exports` | filters＋1–200字符用途；近期MFA、Idempotency-Key、固定事件引用 |
| GET | `/audit/exports` | 当前创建者的导出历史；默认10条，签名游标 |
| GET | `/audit/exports/{id}` | 本人固定回执、当前ready／expired／authorization_changed状态 |
| GET | `/audit/exports/{id}/chunks` | 单次最多512 KiB，返回完整与分段SHA-256、十进制字符串偏移和大小 |
| GET | `/audit/exports/{id}/download` | CSV流，64 KiB前重查权限；拒绝Range，限制同账号并发读取 |

迁移84新增`audit_exports`和`audit_export_events`，关联既有不可变`audit_events`。创建事务保存筛选、披露版本、创建者及授权版本、24小时期限、条数、大小、摘要和按位置排列的事件ID；导出回执、审计、Outbox和幂等记录同事务提交。延迟约束要求提交时引用完整连续，位置约束阻止后续追加；应用账号没有UPDATE／DELETE权限。新导出先读取最多10,001条，超限不产生部分文件或成功回执。

查询按`occurred_at DESC,id DESC`排序，以签名键集游标翻页。**查询是实时视图，不是跨请求数据库快照**；后续写入的回溯时间事件可能影响后续页面。固定导出则保存准确事件ID，不因之后的写入改变。下载从这些不可变事件重新生成CSV并核对条数、大小和SHA-256，失败即停止；服务器不持久保存CSV工作文件。

CSV采用UTF-8 BOM、CRLF及15个固定列。日期统一UTC，资源版本保留十进制字符串，避免超过JavaScript安全整数后失真。潜在电子表格公式单元格加前导单引号；使用者仍应按文本导入。客户端在每个分段和完整文件验证后才创建下载文件，关闭页面／会话时取消读取并清空临时字节。

## 开发与验证 / Development and verification

实现位置：后端`app/backend/internal/auditlog/`、HTTP`internal/httpapi/audit.go`、迁移`0084_audit_exports.sql`；Web`AuditConsole.tsx`；Taro`pages/settings/audit.tsx`；共享类型校验`packages/contracts/src/audit.ts`。

OpenAPI生成器现在会在基本契约后统一应用既有摊销扩展，防止完整生成时遗漏已发布接口。原314个路径和687个模型逐项核对保留，本轮新增6个路径、7个操作和11个模型，合计320／395／698。契约两份JSON保持一致，再运行`npm run contract`生成TypeScript。

从仓库根目录使用已有原生工具和依赖，检查入口如下；重复执行前须归档已有同名尝试，不能覆盖原证据。

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_audit_console_backend.py
python app/scripts/freeze_audit_console_backend.py
python app/scripts/build_audit_console_linux.py
python app/scripts/verify_audit_console_clients.py
python app/scripts/freeze_audit_console_clients.py
python app/scripts/run_audit_browser.py
python app/scripts/run_audit_browser.py --mobile
python app/scripts/encode_audit_media.py
```

Android使用既有`build_android.py --evidence-group audit-console --google-mirror`，显式设置本地或正式API配置；只构建和静态核对，不启动模拟器或真机。开发环境按既有原生数据库升级流程执行迁移84，保留历史记录；临时浏览器检查拥有独立PG数据库、服务及上下文，并在finally中释放。

[本轮验证与来源](../quality/109-audit-console-verification.md)区分真实本地执行、历史保留、Gradle缓存与尚未进行的外部验收。[全部工作包](../delivery/03-goal-gap-assessment.md)仍保留23项active和5项excluded，不把此子功能计为整个系统完成。
