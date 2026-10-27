# 增量94：移动文件与目录整理 / Mobile file and folder workspace

本轮实现位于 `app/mobile/src/pages/files/` 与 `app/mobile/src/platform/file-input.ts`，复用现有Go业务接口、权限、准确版本、幂等及审计。新增公共Taro页面用于微信、抖音和Android；H5仅用于本机界面验证。既有Web文件工作区、Android原生传输与固定版本阅读继续保留。

This increment adds a shared Taro file workspace using existing Go commands, authorization, exact revisions, idempotency and audit. WeChat, Douyin and Android share the source; H5 provides local interface evidence. Existing Web workflows, native Android transfer and fixed-version readers remain available.

## 1. 员工使用 / Employee workflow

| 入口与操作 / Entry and action | 实际行为 / Actual behavior |
| --- | --- |
| 首页或项目的文件／目录入口 / Home or project file entry | 打开当前身份可访问的资料；进入实际目录后只显示该目录内容。Browse permitted records, then a specific folder's direct children. |
| 查找、子目录、更多 / Filter, children and more | 名称筛选、目录和文件分别按20条游标分页；回收站是单独状态。Separate folder/file cursors and an explicit trash filter. |
| 新建目录 / Create folder | 输入原样名称；具体父目录继承项目和分类；公司根目录明确选择分类。Enter a literal name; inherit the actual parent's scope or select a company-root classification. |
| 改名或移动 / Rename or move | 输入名称、原因，浏览并选中实际目标，核对源修订、对象和位置后确认。Review the exact source, revision, destination and reason before submitting. |
| 移入回收站 / Trash | 核对影响并确认；保留全部正式内容，当前文件不可阅读。Retain formal versions while disabling reading of the trashed file. |
| 恢复至原目录 / Restore | 恢复原目录、原名称与当前正式内容；名称冲突由服务器拒绝。Restore the existing record without overwriting a conflicting name. |
| 全部正式内容版本 / Formal version history | 分页列出版本号、原名称、长度、日期、扫描、可用状态、原件SHA-256及副本状态；明确选择准确版本。Review version metadata and select an exact immutable version. |
| 上传 / Upload | Android沿用原生选择与传输；小程序查阅和审批，超大文件由Web／Android承担。Existing native Android transfer; Web/Android handle large files. |

名字允许1至200个Unicode字符，保留输入写法。拒绝首尾空白、路径字符、隐藏控制字符、平台保留名等；最终NFKC／大小写重名与业务约束以服务器结果为准。移动表单允许400个UTF-16码元，避免截断200个非BMP字符。操作原因是1至200个Unicode字符。

Names preserve their entered spelling and accept1–200 Unicode characters. Unsafe paths, controls, reserved names and surrounding whitespace are rejected; server normalization/collision rules remain authoritative. The field permits400 UTF-16 code units so200 non-BMP characters remain possible. Reasons allow1–200 Unicode characters.

## 2. 范围与权限 / Scope and authorization

移动目标目录必须与源相同项目、相同资料分类，处于active且当前可新建；不接受自由填写UUID。项目资料根目录只能改名。目录自身不出现在候选中，不能从该入口进入其后代；服务器仍检查环、后代、状态、名称和当前授权。浏览深度限制32层，目录页可逐层返回。

Destinations must be active and creatable in the same project and classification. Users choose real records rather than entering UUIDs. Project roots can be renamed. The picker excludes the source folder and limits navigation to32 levels; server cycle, descendant, lifecycle, name and authorization checks still apply.

当前范围不提供跨项目／分类迁移、不提供改变权限的人工覆盖、不永久清除云原件、不把历史版本恢复为新的当前内容。跨范围权限影响方案、完整保留／清理执行器和真实双云验收仍属原差异。保留锁、只读项目与撤权等由原业务命令决定，界面使用返回的具体能力标记。

This UI implements same-scope metadata organization and untrash. Cross-scope permission-impact plans, permanent cleanup, promotion of old content and complete retention execution remain separate work. Existing domain commands enforce legal hold, project state and current authority.

## 3. 命令与回执 / Commands and receipts

| 请求 / Request | 绑定 / Binding |
| --- | --- |
| `POST /folders` | `name`、`classification`、可选实际`parent_id`；父目录决定项目。Actual parent determines the project. |
| `POST /folders/{id}/move` | `expected_version`、`new_name`、`reason`、可选`target_folder_id`。 |
| `POST /files/{id}/move` | 同上；内容正式版本保持准确原值。Same fields; the formal content version is retained. |
| `POST /files/{id}/trash` | `expected_version`、`action: trash`、`reason`。 |
| `POST /files/{id}/restore` | `expected_version`、`mode: untrash`、`reason`。 |
| `GET /files/{id}/versions` | 20条独立游标，阅读链接仍固定正式版本。20-record cursor; each read pins a formal version. |

创建成功须验证实际新ID、version1、父目录、项目、分类、创建人和状态；整理／回收／恢复须验证同一对象、准确修订加一、预期名称／位置／状态、原创建资料，以及文件原正式版本与保留标记。缺少字段或不符时不显示成功。共享会话层提供原幂等键、权限变化和页面取消机制。

Create receipts validate a new identity, revision1, parent, scope, creator and lifecycle. Other receipts validate the same identity, exactly one revision advance, intended metadata/state and unchanged creation/content/hold fields. Incomplete or mismatched receipts cannot announce success. Existing command/session infrastructure supplies idempotency and authority/cancellation behavior.

现有OpenAPI的`Move`含仅required的anyOf分支，其生成类型包含unknown；本轮使用明确的本地受限`FileMove`类型，不弱化回执或扩大请求字段。不新增API、数据库迁移、依赖或Skill。

The generated `Move` union includes unknown from required-only anyOf alternatives. A strict local supported-subset type preserves request checking. This increment adds no API, migration, dependency or Skill.

## 4. 草稿、确认与不确定结果 / Drafts and uncertain results

文字草稿只驻留进程内存，绑定本人、授权版本、对象类型、准确ID、源修订和具体操作。目标目录、选择结果和已勾选确认不进入草稿；输入／目标变化清除预览和确认。离开表单保留白名单文字；实际完整回执后清除相应草稿。

Memory-only drafts bind identity, authorization version, resource type, exact ID, source revision and operation. They retain bounded text only. Destination selection and confirmation are never restored. Editing invalidates the preview; only a verified complete receipt clears the draft.

重名、并发冲突、撤权、网络响应丢失或回执不完整时，保留输入并阻止直接重复提交。先刷新当前目录／文件和业务列表，核对是否已提交、最新修订及权限，再按实际结果继续。响应丢失不等于操作失败，不能自动再次创建。

Conflicts, revocation, transport loss and incomplete receipts retain text and stop direct repeats. Refresh authoritative records and permissions before continuing. A lost response can follow a successful commit.

## 5. 验证与发布 / Verification and publication

本轮独立来源、实际本机API／PostgreSQL流程、截图与录制、构建及复用证据见[准确验证](../quality/evidence/mobile-files/verification.md)。既有93证据／媒体与入口文档原字节另存；不将旧检查标为本轮新执行。

See [exact verification](../quality/evidence/mobile-files/verification.md) for current source binding, native API/browser flows, media, builds and unchanged-source reuse. Published93 evidence/media and entry snapshots retain original bytes. Reused checks are identified as such.

H5暗色、375像素竖屏、横屏、平板、减少动效与浏览器200%根字号检查只说明受测浏览器。开发APK、JVM缓存与小程序构建不代表真机或生产放行。仍需微信／抖音平台、Android真机与签名、真实云／模型／TG、120GB／2TB、网络和恢复及依赖安全验收。完整四端Goal继续active。

H5 dark mode, portrait/landscape/tablet, reduced motion and200% browser root-font checks describe the tested browser. Development APKs, cached JVM tests and mini-program builds do not establish device or production readiness. Full four-client and external acceptance remain open.
