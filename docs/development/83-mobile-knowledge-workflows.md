# 83. 移动知识与项目博客 / Mobile knowledge and project blog

## 1. 范围 / Scope

沿用现有 Go/PostgreSQL 知识后端，将 Taro 原来的知识查阅页补为完整操作入口。微信、抖音和 Android 共享列表、正文编辑、发布、归档、退回草稿、不可变历史、恢复、附件和评论逻辑。四类资料为项目日志、会议纪要、SOP、内部文章；既可属于项目，也可属于公司公共知识。没有新增注册页面，仍为16页，通过原列表和详情路由分派。

The existing Go/PostgreSQL knowledge service now has a complete shared Taro interface for project logs, meeting notes, SOPs and internal articles. Company/project scope, editing, publication, archive/draft transitions, immutable history, restoration, attachments and comments use the original business authority. The two mini-program targets and Android share these controls through the existing sixteen registered pages.

这属于手工知识工作流；SOP 不会因此自动成为已发布 Agent Skill。真实平台运行、设备和正式环境验收仍按编号差异表推进。

These are manual knowledge workflows. Saving an SOP does not publish an Agent Skill. Real platform/device and production acceptance remain governed by the numbered gap register.

## 2. 员工使用逻辑 / Employee workflow

1. 从工作台打开“知识与记录”，或从项目进入其知识列表。按标题、摘要、正文、标签检索，选择可见状态，使用游标加载后续记录。
2. “新建知识记录”中选择公司或当前可见的可写项目、资料类型和分类，填写标题、摘要、标签与 Markdown。可先预览。项目、类型和分类在保存后的正文编辑中锁定；范围调整沿用既有影响评估。
3. 附件逐个明确选“跟随当前版本”或某个“固定版本”。保存草稿创建首个不可变正文修订；再次编辑创建后继修订。
4. 发布时重新核对正文、附件、准确修订，填写原因并人工确认。修改已发布资料仅创建未发布内容，普通读者继续读取上一次已发布修订。
5. 从历史列表查看完整历史正文和附件。恢复操作复制所选历史内容，创建新的当前修订，保留旧历史和原已发布指针；需要另行发布。
6. 评论绑定正在阅读的正文修订。可附固定版本、修改自己的文字、查看不可变评论历史。正文变化时旧上下文的未提交评论会冲突，先保留文字并重新核对。
7. 归档后正文和评论写入入口关闭；有权限者可明确退回草稿，再继续编辑。

Open the global or project knowledge list, search and page through visible records, then choose company/project scope and one of the four types. Save an initial draft with Markdown, tags and explicit attachment modes. Later saves append immutable body revisions. Publication needs its own reason and acknowledgement and preserves the previous published body until confirmed. History restoration creates a new body revision; it never rewrites history or implicitly publishes. Discussions belong to the displayed body revision and keep immutable edit history. Archived records are read-only until an authorized draft transition.

## 3. 输入和呈现约束 / Input and presentation

| 项目 / Item | 约束 / Contract |
| --- | --- |
| 标题 / Title | 1—200 Unicode 字符 / Unicode code points |
| 正文、摘要 / Body, summary | 各最多20,000 Unicode字符；发布正文不能为空 / Up to20,000 each; publication needs nonempty body |
| 标签 / Tags | 中英文逗号分隔；最多20项，各1—50字符，大小写不敏感去重 /20 unique, case-insensitive tags of1—50 characters |
| 知识附件 / Body attachments | 最多100项，保留原顺序和已有重复；可跟随当前版或固定版本 /100 references; preserve ordering and existing duplicates; follow-current or fixed |
| 评论 / Comment | 1—20,000字符；最多20个不同的固定版本 /1—20,000 characters and20 unique immutable pins |
| 请求 / Request | 客户端沿用1MiB请求上限；附件仅传引用 / Existing1MiB command limit; references rather than file bytes |
| 分页 / Paging | 默认20条，不推算游标，不用页内位置冒充版本 /20 records and opaque cursors; no page-position version resolution |

Markdown 由受限解析器生成文字节点，支持标题、段落、列表、引用、代码、加粗、斜体和显式 http(s) 链接。HTML 与外部图片保留为文字，不执行脚本、不自动请求图片。链接拒绝脚本、data、file、相对地址和内嵌用户名密码。Web 预览显式开新页并隔离 opener；Android 由系统处理；小程序在用户操作后复制链接供浏览器打开。

The Markdown renderer emits text nodes, with headings, paragraphs, lists, quotes, fenced/inline code, bold, emphasis and explicit HTTP(S) links. Raw HTML and remote images remain text. It never executes scripts or automatically fetches images. Link handling rejects non-HTTP(S) schemes, relative addresses and embedded credentials. H5 uses an isolated new page, Android uses system linking, and mini-program users explicitly copy a link for their browser.

## 4. 附件、权限和完整依据 / Attachments and complete evidence

知识可引用本人当前可见的本项目、公司或其他可见项目文件；不擅自把既有跨项目引用收窄。候选版本需可用、扫描通过或有已批准扫描例外。固定引用按准确版本读取，即使不在最近20条历史中也不替换成当前版本。跟随引用每次查阅重新解析当前版，界面明确标注方式、序号、大小和 SHA-256；内容读取沿用固定下载授权和摘要核验。

Knowledge may reference currently authorized files from its project, company scope or another visible project. Candidates must be available and clean or explicitly scan-waived. A pin resolves its exact version even beyond the first history page. Follow-current references are resolved again on each read. Mode, version sequence, size and SHA-256 are visible; content access continues through the scoped verified reader.

附件 ACL 独立核查；不可用或无权访问时不展示文件名称。后端正文摘要在过滤附件前计算，移动端按相同 Go JSON 顺序、字段、HTML 转义和省略规则复算完整内容摘要。如果正文可见但附件集合已被过滤，禁止用这个不完整集合覆盖正文或发布。用户须先核对完整依据；不以“成功读到正文”推断所有附件都有权访问。

File authority remains independent. Unavailable attachments expose no file names. The backend hashes complete body evidence before filtering references. The client reproduces the same Go JSON field order, escaping and omitted fields. A filtered reference set cannot be used to overwrite or publish the body: incomplete evidence requires a fresh, complete authorized read.

## 5. 评论附件保留修复 / Comment attachment preservation

旧接口的评论修改会直接采用客户端的 `attachment_versions`，而读接口可能已经按当前权限过滤该列表。文字修改因此可能误删隐藏附件。本轮修复适用于 Web、任务评论和移动知识评论：

| 修改意图 / Intent | 后端行为 / Server behavior |
| --- | --- |
| PATCH省略附件字段 / Omitted pins | 保留原服务器附件；文字修改不丢隐藏原件 / Preserve original server-side pins, including currently hidden ones |
| PATCH明确传数组 / Explicit array | 明确替换；空数组即清空；必须能核对全部原附件和所有新附件 / Replace intentionally, including clearing with an empty array; require access to every original and replacement |
| 新建评论 / Create | 省略表示没有附件 / Omission means no attachments |
| 新回执或幂等重放 / Fresh or replayed receipt | 按当前权限再次过滤附件，不外泄内部保存的隐藏版本ID / Reapply current visibility to the returned pins |

A text-only edit now preserves the original server-side attachments. Explicit replacement requires access to every original attachment as well as the replacements. The check runs before idempotency lookup and again under the mutation lock. Both new and replayed receipts reapply current visibility. The original immutable comment rows stay unchanged. The regression test withdraws file availability, edits and replays without leaking or losing the pin, rejects an inaccessible explicit clear, restores visibility, and then verifies an intentional clear and immutable history.

## 6. 接口与代码入口 / API and implementation

| 接口 / Route | 用途 / Purpose |
| --- | --- |
| `GET/POST /v1/posts` | 可见列表、检索、初始草稿 / Visible list/search and initial draft |
| `GET/PATCH /v1/posts/{id}` | 读取指定呈现修订、准确基础上的新正文修订 / Read the presented body and append an exact-base revision |
| `POST /v1/posts/{id}/publish` | 显式publish、archive、draft / Explicit publication/state transitions |
| `GET /v1/posts/{id}/revisions` | 不可变正文历史 / Immutable body history |
| `POST /v1/posts/{id}/restore` | 复制历史内容建立新修订 / Restore into a new body revision |
| `GET/POST /v1/posts/{id}/comments` | 当前呈现正文的评论 / Discussions of the presented body |
| `PATCH /v1/comments/{id}` | 新建评论修订 / Append a comment edit |
| `GET /v1/comments/{id}/history` | 不可变评论历史 / Immutable comment history |
| `GET /v1/file-versions/{id}` | 新增：从评论固定版本ID解析正式元数据，复用当前文件ACL / New: resolve a comment pin through the existing file ACL |

正文命令绑定 `expected_version` 和 `base_revision_id`；评论绑定 `expected_context_revision_id`，修改另带 `expected_revision`。每个业务意图沿用幂等键。客户端核对完整回执，再读取正式记录与不可变历史；冲突、撤权、缺字段或结果不明时保留输入并停止重提，不伪报完成。

Body commands bind an expected version and base revision. Comments bind the displayed body context and, for edits, the exact prior comment revision. Existing intent-based idempotency applies. Complete receipts are checked against authoritative rereads and immutable history. Conflicts, withdrawn permissions, incomplete receipts and uncertain results retain input and stop repeat submission.

主要实现 / Main implementation: `app/mobile/src/pages/knowledge/`、`app/mobile/src/platform/post-input.ts`、`post-markdown.ts`；后端 `knowledge/comments.go`、`files/versions.go`；Web文字评论沿用保留原附件语义。OpenAPI文档、内嵌契约和生成TypeScript同步更新，没有新增数据库迁移。

## 7. 复现、验证与发布 / Reproduction and publication

使用仓库已有原生工具链和隔离测试配置，不使用 Docker、WSL、虚拟机或模拟器。测试夹具和录制仅使用合成数据，原始帧、账号和密钥留在忽略目录。完整命令、来源摘要、实际执行范围和任何失败重跑见[本轮验证记录](../quality/evidence/mobile-knowledge/verification.md)。

Use the repository's native toolchain and isolated test configuration. No local virtualization or emulator is used. Synthetic fixtures and capture frames remain in ignored local storage; published media excludes authentication secrets. See the [verification record](../quality/evidence/mobile-knowledge/verification.md) for exact sources, commands, execution scope and retained failed attempts. The [media catalog](../media/README.md) links the actual screenshots, MP4 and GIF.

代码、双语 README、功能文档、实际素材、差异表及更新记录一起提交并同步到指定远端；后续完成更新继续遵守同一规则。当前开发包和共享H5流程不代表四端正式验收完成。

Code, both language guides, functional documentation, affected real media, gap register and changelog are published together to the authorized remote. Subsequent completed updates follow the same rule. Development builds and shared H5 checks do not establish full four-client production acceptance.
