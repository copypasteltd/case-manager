# Office 创建、编辑与正式版本 / Increment129

## 业务使用

Web 和 Android 可以在有上传权限的目录新建 Word、Excel、PPT，在有更新权限的文件详情修改当前版本的受支持内容。微信、抖音提供准确版本的内容片段查阅，保持既定查询／审批职责。没有新增外部 Office 服务、编辑器账号、数据库表或运行 Worker。

操作顺序：选择目录“新建 Office 文件”或文件详情“编辑 Office 内容” → 填写内容／读取准确版本依据并修改 → 生成独立候选 → 核对名称、内容片段、警告和 SHA-256 → 明确确认加入上传队列 → 原有传输、完整摘要、扫描与副本提升 → 本人提交正式版本。生成或预览候选不会改变文件头、合同、费用或项目状态。取消页面中的候选即放弃；结果只驻页面内存，Android 加入队列后存于应用私有临时来源。

| 格式 | 新建输入与编辑能力 | 保留边界 |
|---|---|---|
| Word DOCX | 标题、逐行段落、Tab 分列的矩形表格；编辑主文档及实际引用的页眉／页脚／注释／注脚等文字片段。 | 保留未改 XML 和压缩部件；域、内容控件、修订文字等保持只读。单个文字片段可包含换行／Tab；不提供分页、样式重排或所见即所得。 |
| Excel XLSX | 服务端支持命名多工作表；首发页面输入一张工作表的 TSV，默认全部为文字；明确 `n:` 数字、`b:` 布尔、`f:` 安全公式、`t:` 强制文字。编辑已有单元格。 | 不把 `=...` 默认当公式；拒绝外部链接及不支持的表达式。共享／数组公式保持只读。修改后清除公式结果缓存和计算链，要求原编辑器完整重算；图表缓存需人工核对。 |
| PowerPoint PPTX | 每页首行为标题，其余为要点，单独一行 `---` 分页；编辑实际幻灯片中的已有文字片段。 | 保留主题、母版、布局与未改部件；已有文字片段只接受单行修改，域保持只读。没有版式编辑、重新分页或图片编辑承诺。 |

正文例：Excel 的 `月费\tn:1200` 中 `\t` 位置实际使用 Tab，`f:SUM(B1:B5)` 表示明确公式，`t:=literal` 表示原样文字。PPT 的第二页以 `---` 后的新标题开始。金额输入不自动形成财务事实；这些文件仍是资料附件。

## 接口与处理约束

| 接口 | 固定依据及结果 |
|---|---|
| `GET /v1/files/{id}/versions/{version}/office` | 当前人类读取权限、准确版本、扫描通过、准确存储副本、ETag／原生版本／大小／完整 SHA；返回 `office-author-v1` 工作区与依据摘要。 |
| `POST /v1/folders/{id}/office-render` | 当前人类 Web／Android 会话、目录上传权限、目录预期版本、可写项目、结构化文档；返回真实 OOXML 文件字节及服务器固定的 UploadCreate。 |
| `POST /v1/files/{id}/office-render` | 另要求更新权限、当前内容头、文件预期版本、原件 SHA、工作区 SHA、唯一已知可写片段及明确类型；返回独立候选，原件和旧版本保留。 |

接口使用现有人类会话与 Web 防跨站规则；不向 Agent、TG 或小程序增加写入权。原件 I/O 和 ZIP／XML 处理期间不持有授权锁，结束后重新核对当前身份、权限、目录、文件头、版本和准确来源。读取期间撤权、移动、替换或内容损坏，不能返回正文／候选。拒绝行为保留既有安全审计；正式文件变更仍由原有上传提交与不可改写证据记录。

原件最多8 MiB，候选最多12 MiB；每个 XML 部件最多8 MiB、解压总量32 MiB，XML 深度64、最多50万标记；最多2000内容片段、正文总量512 KiB、单值8192字节。Word最多500段、200行／32列表格；Excel最多16张表、每表500行／64列；PPT最多64页、每页128行。HTTP 写入最多1 MiB。这些是执行资源上限，不是 AI 费用控制；普通120GB上传目标保持独立。

XML／ZIP拒绝加密、重复／穿越路径、宏、嵌入对象、外链内容、数字签名、活动保护、DTD／处理指令及不支持的可选内容。外部普通超链接只允许 http／https／mailto，处理器从不访问链接。Word 指令跨 run 合并后检查；公式仅接受有界语法、已知工作表与纯函数白名单，拒绝 DDE、网络函数、INDIRECT、外部工作簿、命名表达式及损坏语法。不支持的原件整体拒绝，不能静默删掉复杂部分再发布。

修改只替换固定原 XML 字节区间；其余 ZIP 部件直接复制原压缩数据。新建使用随源码固定的三个空白 OOXML 模板，无宏、图片、业务内容或外部引用。模板来源、可重复生成方式、摘要和 MIT 许可证见[依赖记录](../dependencies/office-authoring-129/README.md)。部署运行不调用 Python 或下载模板。

## 上传及 Android

候选固定 `office-artifact-v1:<完整SHA256>`。Web 解码后完整读取校验，Android 在私有源创建与重新选择时读取全部有界字节；服务端完整流式摘要再次核对，来源被替换时阻止扫描／提升／正式提交。普通大文件仍走原有 SAF／分片与抽样重选依据，服务端完整 SHA 机制保持。

Android 小候选使用专用私有来源：严格 owner UUID／随机 UUID 定位、目录边界检查、固定三种 MIME、规范 base64、大小与 SHA 校验、落盘同步。JS 不能提供任意本地路径；原大文件桥接不传递正文。切换账号／退出仍沿原生 generation 取消与来源释放；有队列引用的临时文件保留以支持重试，释放最后引用时删除。

原生 API 仅对两类准确的 Office 生成 POST 路由允许18 MiB响应，其他路由保持2 MiB；请求、TLS、重定向与取消规则不变。APK仅为当前 local 配置开发包，正式签名、版本升级与发行仍属于 GAP-16，不要求真机或模拟器。

## 可追溯验证与余项

[执行记录](../quality/129-office-authoring-verification.md)分别记录实际 Go OOXML／独立原生库消费、HTTP／PostgreSQL完整提交、Web／共享H5、四端构建和 Android 产物。UI 合成演示与接口完整提交是两种证据，不声称录屏完成实际云上传。无容器、WSL、虚拟机、模拟器、真实模型、TG或云账号。

GAP-04继续保留完整复杂 Office 编辑、版式预览与真实业务样本验收；不把有界内容编辑等同完整 Office 套件。全23项仍见[收口表](../delivery/05-functional-closure-129.md)。当前数据库保持96项迁移，历史数据、业务事实和全部旧证据保留。

一手格式依据：[WordprocessingML](https://learn.microsoft.com/en-us/office/open-xml/word/how-to-open-and-add-text-to-a-word-processing-document)、[Spreadsheet Cell](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.spreadsheet.cell)、[PresentationML](https://learn.microsoft.com/en-us/office/open-xml/presentation/structure-of-a-presentationml-document)。格式说明不替代实际 Office 视觉验收。

## English guide

Web and Android create DOCX/XLSX/PPTX and edit supported content in an exact current formal version. WeChat/Douyin read version-bound content and retain their lookup/approval responsibilities. Choose a folder or file, enter/read content, generate a separate real candidate, review its name/text/warnings/hash, explicitly join the upload queue, then complete the existing upload/full-hash/scan/promotion and human formal-commit workflow. Rendering never publishes a version or modifies OA/finance facts.

Word supports paragraphs/tables and bounded existing text, Excel supports typed cells and an explicitly safe formula subset, and PPT supports slide titles/body text and existing single-line text runs. Unchanged compressed parts remain intact. Complex fields/protection/active content are read-only or rejected. Excel caches are cleared and require full recalculation. This is bounded content authoring; layout editing, general Office compatibility and real business samples remain open under GAP-04.

The three new typed endpoints bind current human authority, exact source/hash and optimistic version/workspace evidence. I/O runs outside authorization locks, with final rechecks. Source8 MiB/output12 MiB, ZIP/XML/content limits and exact generated-file full hashes apply. Android uses owner-bound private temporary sources; ordinary large files keep SAF. No Python runtime, external Office server, new database migration or worker is needed for authoring. Template sources/licenses and actual scoped checks are linked above; development APKs and synthetic browser evidence do not establish cloud, platform, model or production acceptance.
