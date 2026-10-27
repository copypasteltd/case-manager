# 增量92：统一搜索与固定版本正文 / Unified search and fixed-version text

本轮将原先只在 OpenAPI 声明的 `/search` 接通为实际业务接口，并新增固定版本正文提取、索引、查询与预览。应用位于 `app/`，全部说明、验证与媒体位于 `docs/`。开发预览继续保留 Web、微信、抖音和 Android 四端；本轮没有增加 PDF、OCR、Excel 或任意格式解析承诺。

This increment connects the previously declared search endpoint to actual business records and adds bounded extraction, persisted text, live authorization, snippets and exact-version previews. The four-client scope remains unchanged. PDF, OCR and spreadsheet extraction are not implemented by this increment.

## 1. 使用逻辑 / User flow

1. 登录后从 Web 主导航或移动工作台进入“搜索工作资料”。输入名称或正文中的词句，可选择项目、任务、知识记录、文件或目录类型。
2. 结果显示资源名称、命中位置、更新时间；文件另显示正文状态和提取时间。正文检索仅使用**当前正式版本**。不支持正文提取的文件仍可按名称找到。
3. 点击“查阅此版本正文”固定结果中的版本 ID。按每次最多4000个 Unicode 字符显示，可向前／向后分段；读取期间出现更新不会把旧引用替换为新版本。API 允许每段至8000字符。
4. 原件下载、版本历史及文件管理继续进入原业务详情。预览是派生文本，不能据此改写原件；读不到正文时显示待提取、不支持、被阻止或失败状态。
5. 知识记录按本人实际可打开的修订检索：作者／编辑者看当前修订，普通读者看已发布修订。草稿正文不能通过搜索向普通读者泄露。

Open **Search work material** from Web navigation or the mobile dashboard, enter a phrase and optionally select a resource type. Results identify name/body/tag matches and file extraction state. File body search uses the current formal version; preview pins the version returned by that result. The original detail page remains the entry point for originals and history. Non-editing knowledge readers search the published revision, while authors/editors search their current revision.

## 2. 提取能力与硬限制 / Extraction limits

| 输入 / Input | 范围 / Scope | 限制 / Limits |
| --- | --- | --- |
| `text/plain`, `text/markdown`, `text/csv`, `application/json` | UTF-8 文本；JSON 先校验语法；允许 UTF-8 BOM；统一 CR/LF 换行 / UTF-8, validated JSON, optional BOM, normalized newlines | 原始字节1B—1MiB；拒绝非UTF-8及NUL / 1B–1MiB, no invalid UTF-8 or NUL |
| DOCX MIME | `word/document.xml` 中段落、表格正文、制表与换行 / Main document paragraphs, tables, tabs and line breaks | 原件≤8MiB；XML≤8MiB；产出≤2MiB；不提取页眉／页脚／脚注／嵌入对象 / No headers, footers, footnotes or embedded objects |
| ZIP/XML 防护 / Bounds | 最多512项、64MiB声明解压总量、64层XML、50万token；大于1MiB项的压缩比≤100 / Bounded entries, expansion, depth and tokens | 拒绝重复／越界路径、加密ZIP、宏、DTD、格式损坏；不执行文件、脚本、关系链接或网络请求 / Reject ambiguous paths, encryption, VBA, DTD and malformed input; no execution or external fetch |
| 其他格式或超限 / Other/oversized | 保留原件、名称搜索与明确状态 / Keep original and name search | 不截取120GB文件前缀来冒充完整索引 / No partial-prefix indexing of huge originals |

`extractor=text-v1` 是固定解析版本。成功文本有独立 SHA-256、字符总数、来源 SHA-256 和提取时间。已完成记录不可更新或删除；格式适配升级应使用独立的新提取器版本和迁移。本轮没有人工覆写派生正文、手工伪造索引成功或清空终态后重新执行的接口。

`text-v1` fixes extractor semantics. Successful output records its own SHA-256, Unicode character count, exact source hash and extraction timestamp. Terminal records are immutable. A future parser upgrade requires an explicit new extractor version rather than overwriting past evidence. There is no arbitrary-text upload or terminal-reset endpoint.

## 3. 后台处理 / Worker protocol

```mermaid
flowchart LR
  V[正式版本 available + clean] --> S[每10秒扫描，批量50]
  S --> Q[同事务：派生 queued + 去重任务]
  Q --> L[独立读取身份、准确副本来源]
  L --> H[释放DB锁后有界读取及完整SHA校验]
  H --> X[Go ZIP/XML或UTF-8提取]
  X --> R[重取目录/文件/版本/租约锁并复核来源]
  R --> D[原子记录终态并完成任务]
```

任务 `file.text.extract.v1` 使用独立 `file-text` lane，固定文件／版本／提取器，版本去重，最多8次尝试、2分钟租约、90秒总处理预算、单次云读取10秒。网络读取不占用数据库事务。副本必须具有正式版本键、完整摘要校验、准确ETag／可用的来源版本ID、大小和校验时间；成功后再次核对同一来源指纹。改名／当前版本更新不改变固定原件，回收／隔离／来源变化阻止发布。

临时来源／网络失败指数退避重试；格式、内容损坏等形成明确终态。最后一次失败和超限租约由任务扫尾转为失败，避免永久显示处理中。队列 fence 和时钟在最终提交时重新验证，旧执行者不能提交。扫描器只读取正式记录，不接受浏览器提交的对象路径或URL。

The worker uses a dedicated queue lane, version-specific deduplication, eight attempts, a two-minute lease and a 90-second processing budget. A cloud read has a ten-second timeout and releases SQL locks before transfer. The complete bytes must match the source hash and read metadata. Publication reacquires resource/lease locks and checks the same source fingerprint. Transient failures retry; immutable terminal states describe unsupported, blocked or failed content. Exhausted/expired attempts are sealed instead of remaining pending forever.

迁移75新增 `file_text_derivations`、GIN trigram 索引和只负责锁定的 `lock_file_text_version`。应用角色仍没有正式 `file_versions` 的 UPDATE 权限；锁函数采用固定 search_path，撤销PUBLIC执行权，只授权应用角色。原74项迁移字节保持不变。

Migration75 adds derivation storage and trigram indexes. A narrowly scoped definer function acquires only a version read lock; it does not grant UPDATE on immutable formal versions. All previous migrations retain their original bytes.

## 4. 查询与授权 / Query and authorization

| 接口 / Endpoint | 行为 / Behavior |
| --- | --- |
| `GET /v1/search?q=...&type=...&project_id=...&company_only=...&limit=...&cursor=...` | 1—200字符；字面、大小写不敏感子串；`%`、`_`、`\`转义；默认20、最多50；项目／公司范围互斥 / Literal case-insensitive substring, bounded page size, mutually exclusive project/company filters |
| `GET /v1/files/{file}/versions/{version}/text?offset=0&limit=4000` | 固定正式版本，Unicode字符分页，返回来源／文本摘要和状态；非ready绝不返回正文 / Exact formal version, Unicode offsets, hashes/state, no body for non-ready states |

每种资源在 SQL 的分页和limit**之前**调用实时 `can_access`。目录使用有效祖先链；文件正文只有当前版本available且clean、派生来源SHA匹配才进入命中集合。结果不提供未经授权的总数。组织、用户和交互会话按原权限命令顺序加共享锁；不接受AI运行身份调用人工搜索。跨类型游标同时固定类型、ID和更新时间，HMAC绑定用户、组织、渠道、授权版本、词句、筛选和页大小。

预览重新校验当前文件权限、目录状态、版本可用性和clean扫描条件；不依靠旧搜索结果提供的权限。HTTP保持`no-store`。索引存储不复制旧权限列表，因此撤权或重新分级无需等待后台重建即可生效。React／Taro按纯文本渲染结果和预览，不注入HTML。

Authorization is part of each SQL branch before pagination. Folder ancestry, current file classification, available/clean version state and matching derivation source hash are evaluated live. No inaccessible total count is returned. HMAC cursors bind the caller and complete query. Preview independently rechecks the current permission and exact version. Responses are not cacheable, and text is rendered literally. A prior search hit is not a reusable access grant.

PostgreSQL `pg_trgm` 支持GIN上的LIKE／ILIKE查询；短于三个字符的词句仍可检索，但可能扫描更多行。本轮仅有本地功能验证，未给2TB规模或香港跨境链路作性能保证。解析使用Go标准库的[ZIP](https://pkg.go.dev/archive/zip)和[XML](https://pkg.go.dev/encoding/xml)，检索索引能力参考[PostgreSQL pg_trgm文档](https://www.postgresql.org/docs/current/pgtrgm.html)。

## 5. 部署与凭据 / Deployment

先用独立迁移身份执行迁移75，重建API及worker二进制，再配置原生服务。系统服务模板为 `app/deploy/native/copypaste-file-text-worker.service`；示例变量见[正文索引运维](../operations/file-text-search.md)。索引进程必须使用**独立只读云身份**，无模型、下载签名、上传、复制或迁移凭据回退。API只读取派生文本，不需要此云身份。

Apply migration75 with the separate migration identity, rebuild the native API/worker, then configure the dedicated text-reader credentials and service. The API needs no extraction cloud credential. The worker refuses disabled readers instead of borrowing a human download, upload or model identity.

## 6. 验证边界 / Verification scope

[本轮验证与来源记录](../quality/evidence/file-search/verification.md)区分新执行检查、未改来源的复用证明、历史保留和已替代尝试。媒体仅使用隔离合成记录、实际本机PostgreSQL与认证HTTP；云字节由受控fixture提供。移动录屏是Taro H5；Android为原生编译／包检查，没有启动模拟器或宣称真机通过。

实际云权限／网络、2TB迁移／120GB传输、真实设备、小程序平台提交、真实Codex／Telegram、备份恢复和生产安全仍按28项差异验收。此增量没有改变这类门槛或宣称完整Goal完成。
