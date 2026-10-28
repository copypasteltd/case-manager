# 增量96：存量文件盘点、原生迁移与逐项核验

[中文指南](../README.zh-CN.md) · [English guide](../README.en.md) · [恢复与切换](../operations/02-backup-migration-and-recovery.md) · [验收边界](../quality/96-native-file-import-verification.md)

## 1. 交付范围

将 GAP-22 中的人工迁移方案落实为 `app/backend/cmd/file-import` 原生 Go 命令及普通业务 API。工具只读取明确的源目录，通过员工会话和短时分片签名上传；不读取数据库口令或 OSS／R2 密钥。盘点、映射、计划、运行和核验输出都是 JSONL，保存在忽略目录或仓库外。原件从不由工具删除。

首发四端范围保持 Web、微信、抖音和 Android。此工具供迁移操作人使用；小程序继续承担查阅、审批及有界内容访问。当前代码支持单文件 1—120,000,000,000 字节、单批最多 1,000,000 个条目；实际 2 TB 规模、120 GB 传输及香港到境内网络仍需外部验收。CLI 不等于完成这些验收。

## 2. 状态与业务边界

```mermaid
flowchart LR
 A[源目录只读窗口] --> B[流式盘点与逐文件SHA256]
 B --> C[逐项人工映射]
 C --> D[固定计划链摘要]
 D --> E[完整清单入库]
 E --> F[近期MFA批准]
 F --> G[原上传与断点对账]
 G --> H[原扫描和正式候选流程]
 H --> I[普通提交和来源绑定]
 I --> J[OSS与R2独立复制核验]
 J --> K[逐项核验报告]
 K --> L[另行恢复与切换验收]
```

| 环节 | 代码约束 | 操作人责任 |
| --- | --- | --- |
| 盘点 | `os.OpenRoot` 限定读取范围；跳过软链接；分块遍历和 64 KiB 摘要；读前后核对文件身份、大小和时间 | 在整个迁移窗口冻结来源；检查错误行及被拒绝的空文件／特殊文件 |
| 映射 | 每一来源路径必须按原顺序提供映射；缺失、未知字段或 `UNREVIEWED` 所有者不成计划 | 核对原所有者、目标账号、目录、目录版本、项目、分类、名称、类型；更新必须提供逻辑文件与当前基础版本 |
| 固定计划 | 保存原盘点字节摘要、映射字节摘要、逐项规范化链摘要、准确数量和总字节 | 保留原文件；不得在同一计划内编辑源或映射来“续传” |
| 入库与批准 | 完整顺序、数量、总字节和链摘要一致后才可批准；`file.import`、当前目录权限、本人近期 MFA；批准 48 小时并绑定用户／组织授权版本 | 明确确认映射已审阅、来源只读；权限变化或到期后重新核对批准 |
| 传输 | 复用普通上传配额、outbox、worker、扫描、正式候选；分片直传，显式上传域名白名单；不自动跟随重定向 | 提供准确云上传 origin；保持足够的本地读取和网络时间；会话失效后重新登录续跑 |
| 续跑 | 来源命名空间＋准确相对路径固定来源键；同内容复用原会话／正式版本；云端分片重新对账 | 使用原命名空间、原计划和原员工；不靠改名或新建计划绕开冲突 |
| 失败重试 | 仅准确失败／取消／过期且尚未正式提交的旧上传可明确重建；旧版本、原因、近期 MFA 必须匹配 | 先查旧回执与源状态；不能把“结果不确定”直接当失败重建 |
| 增量 | 已存在的来源逻辑文件不可隐式改为另一个文件；新内容需新盘点、新计划及准确 `file_id/base_version_id` | 先核对当前正式版本，再批准新计划；原历史保留 |
| 核验 | 按完整清单分页，检查原件摘要／大小、当前目录映射、目标人可读性及 OSS／R2 已验证摘要 | 复核真实 ACL、对象版本、恢复结果和切换窗口；报告不是源 ACL 等价或切换批准 |

POSIX 模式和修改时间仅作为盘点线索，不能代表 Windows ACL、NAS ACL 或来源身份已被自动迁移。`mapped_owner_id`记录归属对照，并检查目标账号至少在一个现有人工作业渠道可读；不会改写文件的实际创建人，也不会自动授予编辑／管理权限。实际创建人与审计仍是迁移操作人，完整源所有权／ACL迁移不能据此判定完成。源 ACL 清点与目标业务权限配置仍需操作人独立核验。读取检测也不能替代来源只读锁／快照。

## 3. API 与数据库

新增迁移 `0077_file_imports.sql`，原 76 个迁移保持原字节。清单／批准／尝试记录不可改写；稳定来源对象保存同一逻辑文件和当前有效上传指针。正式提交仍走原接口，额外检查批准摘要和当前来源指针；同幂等键重放也重新检查迁移权限。

| 接口 | 用途 |
| --- | --- |
| `POST /v1/file-imports` | 注册准确批次；同来源／计划对账已有批次 |
| `GET /v1/file-imports/{id}` | 当前本人权限下读取批次及批准有效性 |
| `POST/GET /v1/file-imports/{id}/entries` | 顺序加入不可变条目／有签名游标的完整分页 |
| `POST /v1/file-imports/{id}/approve` | 完整计划、近期 MFA 和明确声明 |
| `GET /v1/file-import-entries/{id}` | 当前上传、正式版本和实际副本证据 |
| `POST /v1/file-import-entries/{id}/uploads` | 原配额／上传流程内原子预留或对账 |
| `POST /v1/file-import-entries/{id}/retry` | 明确重建准确失败／取消／过期会话 |

现有 `sign-parts/ack-parts/resume/finalize/commit` 路由负责原上传流程。元数据入库与子命令共用事务；网络 PUT 和 worker 均不在数据库事务内。`file_importer` 角色要求 MFA，并包含迁移及普通文件读取／上传／更新动作，仍受项目成员、目录分类和客户端策略限制；角色不会自动绑定员工。批次只由其原操作人读取和操作，跨人员接管尚未实现，需要原操作人恢复或另行设计受审计接管。

## 4. 构建与身份

使用已有原生工具，不启动虚拟化环境。从仓库根目录执行：

```powershell
$env:PYTHONUTF8='1'
Push-Location app/backend
../.tools/go/bin/go.exe build -o ../.local/file-import.exe ./cmd/file-import
Pop-Location
New-Item -ItemType Directory -Force app/.local/import-private | Out-Null
```

服务器原生 Go 环境可在 `app/backend` 用 `go build -o /受控目录/file-import ./cmd/file-import` 编译。API 必须先应用迁移 77，并具备正式存储、容量和扫描配置。迁移角色、项目／目录授权和 MFA 由管理员先配置。云对象密钥只在原服务／worker 环境；CLI 不接受这些密钥。

CLI 的会话通过普通 `android` Bearer 登录流程获取，执行与 Android 相同的现有分类策略，不能据此声称已完成 Android 设备验收。已完成 MFA 绑定的操作人可使用如下当前进程示例；不要启用 HTTP 调试、PowerShell transcript 或输出登录响应：

```powershell
$apiBase = 'https://已配置的内部域名/v1'
$credential = Get-Credential
$loginBody = @{login=$credential.UserName; password=$credential.GetNetworkCredential().Password; device_type='android'} | ConvertTo-Json -Compress
$loginReply = Invoke-RestMethod -Method Post -Uri "$apiBase/auth/login" -ContentType 'application/json' -Body $loginBody
Remove-Variable loginBody,credential
if ($loginReply.data.step -ne 'mfa') { throw '先在现有账户流程完成 MFA 绑定，或核对角色设置' }
$mfaSecure = Read-Host 'MFA code' -AsSecureString
$mfaCredential = [System.Management.Automation.PSCredential]::new('mfa',$mfaSecure)
$mfaBody = @{challenge=$loginReply.data.challenge; code=$mfaCredential.GetNetworkCredential().Password} | ConvertTo-Json -Compress
$sessionReply = Invoke-RestMethod -Method Post -Uri "$apiBase/auth/mfa/verify" -ContentType 'application/json' -Body $mfaBody
$env:CP_FILE_IMPORT_TOKEN = $sessionReply.data.access_token
Remove-Variable mfaSecure,mfaCredential,mfaBody,loginReply,sessionReply
```

`CP_FILE_IMPORT_TOKEN` 仅保存在当前进程环境，子进程会继承；按凭据管理，不写入 `.env`、参数、计划或 Git。CLI 不保存 refresh token 或自动绕过 MFA。批准需要近 10 分钟内交互 MFA；长批次会话过期时停止，重新登录后用同计划继续。完成后 `Remove-Item Env:CP_FILE_IMPORT_TOKEN`。示例异常时也应在当前进程清理临时凭据变量。

## 5. 盘点、映射、计划

先给同一来源分配并保留一个 UUID；未来增量仍用它。下面的源和云域名由操作人按实际部署替换。`--out` 不覆盖已有文件；父目录必须已存在、支持硬链接发布，且输出不得位于源根目录（包括解析后的别名路径）。默认由操作系统保护运行目录；Windows 需再设置目录 ACL，0600 不能替代 Windows ACL。

```powershell
$sourceNamespace = [guid]::NewGuid().ToString()
$sourceRoot = 'E:/readonly-source'
$private = 'app/.local/import-private'
app/.local/file-import.exe inventory --root $sourceRoot --namespace $sourceNamespace --out "$private/inventory-001.jsonl"
app/.local/file-import.exe template --inventory "$private/inventory-001.jsonl" --out "$private/mapping-template-001.jsonl"
```

盘点错误写为 `type=error`，汇总包含 errors；任何错误阻止生成计划。修复来源后重新盘点并保留旧失败记录。不要手工删除错误行伪造完整性。模板默认 `source_owner=UNREVIEWED`，必须另存为审阅映射。逐行填入：

```json
{"source_path":"project-a/original.bin","source_owner":"经核对的原始归属与ACL记录编号","mapped_owner_id":"实际员工UUID","folder_id":"实际目录UUID","folder_version":3,"project_id":"实际项目UUID或null","classification":"project_restricted","name":"original.bin","media_type":"application/octet-stream"}
```

上例文字占位不能直接执行；公司级目录用 JSON `null` 的 `project_id`。从授权 API／现有页面取得准确目录和当前版本，不能猜测 UUID、降分类或默认为公共目录。映射与盘点保持完全相同顺序。源扩展名推导的媒体类型只是模板建议，必须审阅。对于已纳管文件的新内容，映射另填真实 `file_id` 和当前 `base_version_id`，不能只凭同名覆盖。

```powershell
app/.local/file-import.exe plan --inventory "$private/inventory-001.jsonl" --mapping "$private/mapping-reviewed-001.jsonl" --reason '首批只读原件与归属逐项审阅' --out "$private/plan-001.jsonl"
$planSHA = (Get-Content "$private/plan-001.jsonl" -First 1 | ConvertFrom-Json).plan_sha256
```

同一输入字节、映射与原因得到同一固定计划；盘点的记录顺序采用保存下来的原生遍历顺序。跨扫描顺序不保证相同，相同命名空间与准确相对路径的来源键仍保持相同。未变化的完整条目链已经注册时，继续使用原批次和原计划；不能只改盘点时间或原因来替换原批次证据。源路径改名／移动会产生新来源键；自动识别源重命名、收编先前手工上传文件与跨操作人接管尚需单独流程。单条 JSON 行上限 32 KiB；路径有明确 Unicode／长度／相对路径限制，拒绝越界、软链接、ADS、控制字符及危险规范化形式。源数据、计划、报告及临时 spool 属敏感资料，必须留在忽略目录或仓库外，按业务保留策略清理。

## 6. 执行与续跑

```powershell
app/.local/file-import.exe run --root $sourceRoot --plan "$private/plan-001.jsonl" --api $apiBase --upload-origin 'https://实际OSS上传域名' --approve-plan $planSHA --reviewed-mappings --sources-read-only --out "$private/run-001.jsonl"
```

可重复 `--upload-origin` 指定准确 origin；禁止给泛域名、API token 或路径。仅在受控原生隔离检查中可显式 `--allow-loopback-http`，且只允许数字 loopback，生产使用 HTTPS。默认逐文件、逐分片上传，内存不会一次装载整个原件；默认单文件等待 24 小时，`--wait` 可明确调整，API 会话、上传过期和批准有效期仍独立约束。不存在单机 2 TB 暂存要求。

退出码0只表示该命令执行结束；`run`不等待全部副本，`report`完整生成也可能包含pending／invalid，必须检查汇总字段，不能把退出码当迁移完成。退出非零时先检查私有运行记录及当前 API。已取得批次 ID 的运行会追加可取得的当前核验；中止行和不完整报告不得当完成。使用**同一计划和新输出文件**重新执行会逐项对账已入库清单和原上传，再读取云端已接收分片；不确定提交先回读，不自动创建另一个原件。可加 `--only-source-key 实际64位来源键` 单独恢复一项，但仍验证整份原计划。

明确失败、取消或过期的旧上传需要重建时，先取得当前条目、旧上传 ID／version。重新完成近期 MFA，核对同一计划，然后：

```powershell
app/.local/file-import.exe retry --api $apiBase --entry '实际条目UUID' --upload '准确旧上传UUID' --version 4 --approve-plan $planSHA --reviewed-mappings --sources-read-only --reason '已回读原会话且确认过期，原件摘要不变' --out "$private/retry-001.jsonl"
```

随后运行同一 `run` 命令续传新会话。已正式提交的会话不能重建；活跃会话不能用 retry 绕过。来源发生变化时，停止旧计划的该项，制作新的盘点／映射／计划并明确当前基础版本。工具不提供自动全源删除、跨账号接管、自动 ACL 翻译或无界并行上传。

## 7. 核验与切换

```powershell
app/.local/file-import.exe report --api $apiBase --batch '运行记录中的准确批次UUID' --out "$private/verify-001.jsonl"
```

| 报告字段／状态 | 准确含义 |
| --- | --- |
| `all_expected_rows_observed` | 本次按顺序观察的条目数量、字节和链摘要与完整计划一致 |
| `primary_verified` | 正式原件摘要／大小与来源匹配，另一端证据尚未全部成立 |
| `replicas_verified` | 当前映射、目标人可读性、准确正式版本及 OSS／R2 已验证摘要全部匹配 |
| `mapping_changed` | 当前文件版本／位置／分类／目录版本或目标人可读性已偏离计划 |
| `observed_content_verified` | 本次所有预期条目观察均达到双端内容证据要求；并非同时刻快照 |
| `cutover_approved=false` | CLI 永远不替代独立恢复、权限与业务切换批准 |

条目证据分批实时回读，读取现有副本核验凭据及其时间，不会为生成报告重新下载全部云原件。云端对象当前可用性仍需独立核对和恢复演练。报告不是跨所有文件和所有时刻的一致快照。权限收回、页缺失、链不完整、缺少精确副本证据都会阻止完整核验。主端完成时不等待副本才能继续下一个文件，待后台复制后再次 report。真实交付仍按 [迁移与恢复方案](../operations/02-backup-migration-and-recovery.md)执行多类型抽检、120 GB 真件、两端对象版本／摘要、真实角色 ACL、2 TB 账实核对、恢复和回切演练，并由业务人员批准切换；保留源原件。

## 8. English operator summary

Build `./cmd/file-import` with native Go from `app/backend`. The CLI uses an ordinary MFA-authenticated Android-channel Bearer session in `CP_FILE_IMPORT_TOKEN`, with the existing channel and classification policies. It never needs database or cloud credentials. Store source inventories, reviewed mappings, plans and reports outside Git and outside the source tree, with operating-system directory permissions.

Use a stable source namespace UUID. Run `inventory`, generate a `template`, fill every ownership/folder/project/classification/current-base mapping, then run `plan`. Review the exact chain SHA and establish a read-only source window. Run `run` with `--approve-plan`, `--reviewed-mappings`, `--sources-read-only`, the HTTPS API base and explicit cloud upload origins. Reuse the same saved plan and employee session identity after interruption; write a new output path. Sources are hashed through bounded reads and held open during sequential section uploads.

An accepted part with a missing success receipt is reconciled from provider listings on the next run. Upload, scan, promotion, commit and replica verification reuse existing business services. A changed source requires a new reviewed plan and the existing logical file/current base version. `retry` is a separate reviewed MFA action for an exact failed, aborted or expired uncommitted upload. Run `report` again after replica workers finish. `mapped_owner_id` records reviewed ownership correspondence and checks permitted readability; it does not rewrite the file creator or automatically grant edit/manage access. The report reads stored replica verification receipts; it does not download all objects again or establish their present availability. Complete per-row observations and matching OSS/R2 content evidence do not establish source ACL equivalence, an atomic restore point or approval to cut over. The CLI never deletes source files. Real cloud, Hong Kong networking, 120 GB/2 TB, platform/device and production recovery acceptance remain open.
