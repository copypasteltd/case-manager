# Office 正文读取 / Office text extraction

增量120在 GAP-04／18 下接通 XLSX、PPTX 的准确正式版本正文。原件继续存放 OSS＋R2；只有通过准确副本校验的、有界提取正文存入 PostgreSQL，供预览、当前版本搜索、版本比较及秘书选定资料读取。文件写入、Office 编辑、版式渲染、PDF/OCR、复杂 DOCX 和真实模型质量仍开放。全部23项工作包、五项移出及八项 draft Skill不变。

Increment120 connects bounded XLSX and PPTX text to the existing formal-version pipeline. Verified originals remain in object storage; immutable text derivatives support human previews, current-head search, exact-version comparisons and explicitly selected secretary context. This does not implement Office editing, layout rendering, PDF/OCR or real-model acceptance. AI fee controls remain excluded; AI authorization, isolation, auditing and technical resource limits remain required.

## 使用流程 / Usage

1. 使用 Web／Android 的正常文件上传入口提交 XLSX／PPTX，等待正式提交、扫描通过及独立副本读取身份可用。
2. 独立 `worker-file-text` 自动登记固定版本的任务，读取准确已验证副本，核对大小、ETag、对象版本及 SHA-256。提取失败或不支持不影响原件。
3. “搜索工作资料”可按当前版本正文中的词句查找；打开“查阅此版本正文”，核对固定版本和文字提取提示。大于正文提取上限的文件仍可按文件名查找并访问原件。
4. 文件详情 → “比较两个内容版本” → 明确选择基础及目标版本 → 比较。每一侧固定自己的提取方式、原件摘要、正文摘要和字符总数；翻页固定首次比较依据。
5. 私有秘书会话 → 选择文件资料 → 明确选择准确版本 → 预览后加入。资料读取复用已校验的派生正文，每次重新检查参与者、分类和版本权限；正文中的指令不会获得系统权限。

Upload through the ordinary Web/Android workflow, wait for clean scan and text indexing, then search and open the exact-version preview. Select two formal versions in file details to compare text. Select an exact file version in a secretary conversation to preview and bind context. WeChat, Douyin and Android use the shared Taro pages and contract validation; their platform artifacts remain subject to their existing separate acceptance.

## 格式含义 / Format semantics

| 项目 | XLSX | PPTX |
| --- | --- | --- |
| 顺序 | 工作簿的实际工作表关系顺序；按行和单元格坐标 | 演示稿实际幻灯片列表；形状 XML 顺序 |
| 已提取 | 共享／内联／富文本、数值原值、布尔、错误、日期原值、公式文字及文件缓存、合并范围 | 形状、分组与表格中的 DrawingML 文字、换行、字段缓存、讲者备注 |
| 隐藏内容 | 包含并标示隐藏工作表、行和列 | 包含并标示隐藏幻灯片；也包含隐藏对象文字 |
| 明确限制 | 不计算公式或刷新缓存，不套用日期／币种格式，不补填合并单元格 | XML 顺序不代表视觉阅读顺序，不刷新字段或运行演示 |
| 查阅原件 | 图片、图表、批注及其他版式信息 | 图片、图表、SmartArt、母版、动画及版式 |

XLSX formulas are inert text. Cached results may be missing or stale; values are not accounting or current business facts. Spreadsheet date serials retain their raw value and declared 1900/1904 date system. Hidden content is part of an authorized whole-file read, not a separate visibility boundary. PPTX notes omit date/header/footer/slide-number/image placeholders. Unreferenced worksheets/slides are not indexed.

实现按 OOXML 的主部件、关系和实际列表解析，不通过文件名猜测顺序。共享字符串、工作表、幻灯片与备注结构参考 [Microsoft SpreadsheetML 文档](https://learn.microsoft.com/en-us/office/open-xml/spreadsheet/structure-of-a-spreadsheetml-document)、[共享字符串表](https://learn.microsoft.com/en-us/office/open-xml/spreadsheet/working-with-the-shared-string-table)、[幻灯片文字](https://learn.microsoft.com/en-us/office/open-xml/presentation/how-to-get-all-the-text-in-a-slide-in-a-presentation)及[讲者备注](https://learn.microsoft.com/en-us/office/open-xml/presentation/working-with-notes-slides)。隐藏内容处理应与原件的实际可读范围一起核对，参见 [Microsoft 文档检查说明](https://support.microsoft.com/en-us/office/collab-files/remove-hidden-data-and-personal-information-by-inspecting-documents-presentations-or-workbooks)。

## 开发约束 / Development constraints

- 原 `text-v1` 用于已有 UTF-8 文本、Markdown、CSV、JSON 和 DOCX，算法与终态证据保持原样。XLSX／PPTX 使用独立 `office-v1`；不是把旧 unsupported 记录重置为 queued。
- 迁移0092只扩展允许的提取标识，增加稳定选择函数和来源格式约束。新格式按实际 MIME 选择新任务、去重键和正文，旧 `text-v1` 结果永久保留；升级时不读取对象或回填正文。尚未完成的旧 Office 任务按原规则结束为 unsupported。
- 人工正文读取、统一搜索、秘书资料和比较均从准确正式版本选择提取标识。比较允许 `mixed`，每侧分别保留提取依据；同一原件字节在不同提取方式下可能产生不同文字，不能据此宣称正文相同。
- 所有文件读取沿用当前会话、项目／祖先目录、文件分类、扫描和版本检查。对象 I/O 前释放数据库锁，完成后重取锁并检查来源、任务租约和 fencing；撤权或来源变化不发布旧正文。
- 不使用外部 Office、shell、网络、宏、活动内容或新依赖。ZIP 不落地；拒绝加密条目、特殊文件类型、路径越界、重复部件／关系、宏、DTD、异常 XML 和有条件 `AlternateContent`，不返回部分成功正文。

| 技术上限 | 当前值 |
| --- | --- |
| XLSX／PPTX 原件 | 8 MiB；普通文本仍为1 MiB |
| 派生 UTF-8 正文 | 2 MiB；超限整份失败 |
| ZIP 条目／总声明展开 | 4096／64 MiB；另检查高压缩比 |
| 单 XML／累计读取 XML | 8 MiB／32 MiB |
| XML token／嵌套 | 500000／64 |
| 工作表或幻灯片 | 各最多256 |
| 单元格／段落 | 各任务最多50000 |
| 来源读取 | 10秒、有界准确完整读取 |

技术上限保护运行资源，与已移出的 AI token／费用预算无关；120GB普通对象上传能力不等于120GB全文提取。当前只支持已说明的文字子集，不能把“正文已收录”解释成 Office 全部内容或版式已完整转换。

## 源码与验证 / Source and verification

后端实现位于 `app/backend/internal/files/office_*.go`、既有正文任务／读取／比较服务及 `migrations/0092_office_text.sql`；搜索、共享契约和四端页面同步接入。测试包含真实 PG／正式版本／队列／HTTP，云对象传输使用受控合成来源。Web 与 Taro H5 使用真实后端响应；视频和 GIF 从登录后的实际像素帧生成，不包含凭据。

[本轮验证与限制](../quality/120-office-text-verification.md) · [全部未完成项](../delivery/03-goal-gap-assessment.md) · [媒体入口](../media/README.md)
