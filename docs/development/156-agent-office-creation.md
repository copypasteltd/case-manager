# 156 · 秘书受控新建 Office / Controlled Office creation

## 结果与使用 / Outcome and use

员工可以在秘书会话选择自己有权新建文件的准确目录、目录修订、文件名及 Word／Excel／PowerPoint 格式，发送明确新建请求。秘书通过既有三阶段工具生成真实 DOCX／XLSX／PPTX 文件；正式回执提供准确文件版本入口。Web 与微信、抖音、Android 共享源码同时增加格式选择、Office 摘录说明和准确版本链接。TXT／Markdown 新建继续可用。

1. 打开会话并选择具备 `file-update` 的运行配置，点击“选择新建文件位置”。
2. 选择当前有权使用的目录、格式并填写匹配扩展名的文件名；加入请求时固定目录修订。仅选择不会发起执行。
3. 输入正文／工作表／演示文稿需求，核对目标后发送。此处使用既有 L1 准确新建预授权；本轮没有增加第二次逐草稿人工批准。
4. 等待草稿、扫描及正式提交结果；Office 摘录帮助阅读内容，完整二进制 SHA256、字节数与正式版本回执用来核对文件。
5. 点击正式版本入口，查看该次生成的准确版本；需要原始版式时下载文件。普通 Office 内容读取和人工工作区编辑仍使用原有流程。

English: Select an exact authorized directory/revision, name and Office format before sending. The existing L1 creation authorization covers only that target. The secretary produces a real binary through the ordinary staged/scan/promote/commit flow. The receipt links the exact formal file version; the excerpt is a content projection, while the binary hash identifies the complete document. No additional per-draft human approval is introduced.

## 支持范围 / Supported content

| 编号 | 格式 | 受控结构 | 明确边界 |
| --- | --- | --- | --- |
| 1 | DOCX | 段落、有限层级标题、列表、表格等既有工作区结构 | 固定模板；不接收任意模板路径、远程附件或宏 |
| 2 | XLSX | 工作表、类型化单元格、有限安全公式 | 类似公式的普通字符串仍为文本；危险公式在上传前拒绝 |
| 3 | PPTX | 页、标题和有限文本内容 | 固定模板；不声称任意复杂版式还原 |
| 4 | TXT／Markdown | 原有 `content` 正文 | 保持原有文本新建行为 |

只扩展“新建文件”。既有 Office 文件更新仍不进入秘书文本替换工具；复杂原件修改、真实样本版式准确性及更广模板留在 GAP-04／12。合同正文自动绑定／人工采用继续限文本，不因选择 DOCX 自动获得合同业务写入权。真实 Codex／Linux／云接入和八项 Skill 正式验收分别按 GAP-09／10／13／20 完成。

## 接口及执行 / Contract and execution

没有新增 HTTP 路径、数据库迁移或工具名。现有 `AgentFileCreateInput`、`AgentFileUpdateTarget`、`AgentFileDiff` 扩展；新增可选 `AgentOfficePreview`，OpenAPI 为390条路径／471项操作／880个结构。新增／修改仅影响相关类型，生成的服务端契约及共享 TypeScript 定义同时更新。

`stage_file_create` 的工具输入在 `content` 与 `document` 中二选一。格式由原始员工请求固定，模型不能改名、跨目录或改变格式。`document` 是关闭未知字段的类型化 JSON，最多48KiB；整体工具参数仍受既有64KiB限制，生成二进制最多1MiB，并保留原有 Office 工作区数量／长度限制。

服务端调用既有 Office 构造器及固定模板，生成真实 ZIP 格式文档，计算完整字节摘要。既有草稿存储、两阶段权限复核、隔离上传、清洁扫描、晋升及正式第一版提交继续生效。`propose_file_create`／`commit_file_create` 没有获得任意文件写入权；处理中撤权会停止且不生成正式文件。TG 入口没有增加写工具。

正式创建回执的人工可见性只核对准确文件引用：当前受众、文件／目录／项目／分类权限、准确版本和清洁扫描继续检查，不调用AI专用正文索引。秘书实际读取正文仍执行原有索引与AI权限检查；不可见资料不会因回执链接而公开。

`diff.office` 固定适配器、格式、模板摘要、工作区摘要、单位数量和警告。兼容字段中的文本／字符差异对应内容投影；客户端显示“Office内容摘录”，不将其宣称为完整文件字节，也不显示仅适用于文本的 BOM／字符范围语义。

## 准确恢复 / Exact recovery

保存的创建配方固定 `agent-office-create-v1`、规范化结构和模板 SHA256。恢复时要求同一适配器、格式、模板、规范 JSON 及既有完整二进制摘要／长度／差异一致；拒绝未知字段、尾随数据和模板变更。新恢复尝试沿用既有新扫描与准确提交流程，不复用旧扫描作为新正式文件的证明。

这是已知 Office 新建草稿的日常恢复子流程，不等于秘书全部恢复源清理、未知对象治理、历史迁移或容灾。GAP-05／20 仍开放。

## Skill 与交付 / Skill and delivery

`file-update` 更新为 **1.5.0 draft**，九个工具保持不变；其余七项包字节不变，全部八项仍 draft。当前本地包摘要见[实际目录记录](../quality/evidence/agent-office-create/runtime-and-client-summary.json)。真实安装必须重新固定新包摘要并完成模型／工具验收；本轮未向真实服务器安装。

源码156处于 rc.8 冻结之后，数据库目标仍113；原开发 PG100／API保持原样。八个冻结源码标签和产物分支不变。**rc.8 与开发 APK154 固定业务153／结构113，不含155／156功能**；本轮没有新 APK、新冻结版或生产部署。

English: The existing nine tools and ordinary file security checks remain in force. Only structured new-document creation expands. Exact recovery pins the adapter, template, normalized recipe and full binary hash. All eight Skills remain drafts, and the changed package needs new installation verification. Source156 is separate from frozen rc.8/APK154 (business153/schema113).

[核验和失败记录](../quality/156-agent-office-creation.md) · [本轮及全部23项](../delivery/05-functional-closure-156.md) · [实际Web录屏](../media/videos/agent-office-create-web.mp4) · [六秒截图导览](../media/gifs/agent-office-create-web.gif)
