# Increment123 · Word 引用内容与文字修订 / DOCX stories and revisions

本轮实现 `docx-v2`，接通正式文件版本 → 已验证原件 → 独立正文任务 → 派生正文 → 四端检索、预览、完整文字比较与秘书固定资料读取。它保存可读文字和必要标记，原件及其正式版本不变。GAP-04／12／18继续开放，本轮没有新增编辑器或宣称完整 Office 格式支持。

## 使用流程 / Usage

1. 在 Web／Android 上传 `.docx`，完成既有上传流程。正文处理要求准确版本可用、扫描为 `clean`，并且有匹配 SHA-256 的完整已验证副本。
2. 独立 `worker-file-text` 自动登记新派生版本任务。未提取完成显示等待；不支持、损坏或来源不可用不影响原件版本。
3. 在搜索工作资料中输入正文、脚注或批注中的文字。结果固定当前正式版本；旧版本正文不会因旧派生行重复出现。
4. 选择“查阅此版本正文”。查看插入／删除／移动标记、单元格边界、引用部件及字段缓存提示；超过一页则使用原有按字符分页。
5. 在文件详情人工选择两个正式版本，点击“比较所选版本”。比较两侧各自的当前提取依据，保留原件摘要、文字摘要与完整分页；选择本身不触发读取。
6. 在秘书会话中明确选择准确文件版本。资料接口及既有 MCP 读取复用同一派生正文，并重新检查参与者、分类与版本权限；模型服务不获得对象存储凭据。

Web/Android upload remains the existing formal file flow. WeChat, Douyin and Android share the Taro read/compare pages. Search uses only the current formal head; explicit history reads and comparisons retain exact selected versions. The independent worker produces immutable text before a secretary reads it. This increment does not accept revisions, execute fields, or change business records from document prose.

## 支持内容及边界 / Supported content and limits

| 内容 | 本轮行为 | 明确边界 |
|---|---|---|
| 主文档 | 由包根 `officeDocument` 关系和主内容类型定位，支持 Transitional／Strict Word 命名空间 | 不按 ZIP 文件名猜测；缺关系、重复主体或错误根拒绝 |
| 正文／表格 | 按 XML 顺序保留段落、换行、制表符，标出表格与单元格 | 不还原表宽、合并、编号、页码或最终视觉布局 |
| 内容控件／超链接 | 读取 `sdtContent` 与链接显示文字 | 不执行数据绑定，不读取别名／作者元数据，不跟随外部链接 |
| 页眉／页脚 | 按当前节属性中的显式关系定位，标注默认／首页／偶数页；相同部件去重 | 不计算继承后的页面分配、启用条件及分页；原件核对 |
| 脚注／尾注／批注 | 只读取实际引用的 ID；首次引用顺序、正文去重；标出批注范围起止 | 不收录未引用部件／未引用注释／分隔符注释；不重建批注线程、回复、人员信息或 Word UI |
| 文字修订 | `[插入]`、`[删除]`、`[移入]`、`[移出]` 成对保留；删除文字需属于删除／移出范围 | 不自动接受／拒绝；格式、行列、样式修订和历史属性快照不重建 |
| 文本框／图像 | 读取可达 `txbxContent`，图像／绘图／嵌入对象提示核对原件 | 无图片 OCR、图表／公式数学识别或对象执行；条件 `AlternateContent` 拒绝而非猜测分支 |
| 隐藏文字 | 保留文字，标记直接 `vanish`／`webHidden` | 不解析继承样式、条件格式或排版可见性；派生文字可能包含原件界面默认隐藏的内容 |
| 字段 | 简单／复杂字段的指令作为不执行的文字，独立标记缓存可能过期；校验嵌套及闭合 | 不求值、不刷新，不访问 `INCLUDETEXT` 等地址；不能据缓存认定当前日期、金额或外部事实 |
| 外部子文档／导入块 | `subDoc`／`altChunk` 明确不支持 | 不发起网络、文件系统或 Office 应用调用 |

文字属于整份已授权文件的派生内容；没有单独的批注／隐藏段落访问授权。如果某些读者不应看到这些内容，应拆分文件并使用现有文件权限；不能依赖 Word 的显示隐藏来隔离信息。模型收到的资料仍属于不可信业务输入，既有动作授权、隔离与审计继续适用。

## 数据与任务 / Data and jobs

- 新增普通结构升级 `0094_docx_stories.sql`，将派生身份允许值加入 `docx-v2`，调整 MIME → 提取器的共享 SQL 选择函数，并加入准确 DOCX 来源触发器。
- 新派生主键仍为组织、准确内容版本、提取器。原件 SHA-256、文字 SHA-256、字符数、终态和索引时间继续受到既有不可变与一致性约束。
- 旧 `text-v1` 的 ready、unsupported、failed 等终态均保留原字节，不清空重做。仍在队列的 `text-v1` DOCX 任务继续运行旧算法；新调度采用 `docx-v2`，以版本与提取器去重。
- 不重写旧秘书选择依据。如果原选择记录仍固定旧派生身份，新读取会拒绝失效依据，要求重新选择；不会静默替换原来引用的文字。
- Worker 对象读取期间释放事务锁；回写前重新获取当前文件、完整目录、扫描／可用性、来源指纹、任务租约与 fencing 依据。旧／变更来源不发布新正文。
- 读取只使用独立服务身份，核对准确对象的大小、ETag、对象版本及完整 SHA-256；不接受用户给出的 URL 或本地路径。

`pending → ready / unsupported / failed / blocked` 延续原状态约定。失败可追溯但不会把截断正文发布为 ready。这里是派生任务版本升级及正常 schema 升级，不属于移出范围的历史数据迁移／容灾。

## 原生部署 / Native operation

先按已有安装文档配置 PostgreSQL 与独立对象读取身份，使用原生运行入口：

```text
python app/scripts/run_backend.py migrate
python app/scripts/run_backend.py worker-file-text
```

服务端生产运行仍使用对应 Linux 原生产物与运行配置；本轮 Windows 原生开发检查不替代 Linux、真实存储或生产验收。不使用 Docker、WSL、虚拟机或模拟器。

| 限额 | 固定值 |
|---|---|
| DOCX 提取源 / 输出 | 8 MiB / 2 MiB；不等于正式文件上传上限 |
| ZIP 条目 / 声明解压总量 | 4096 / 64 MiB；异常压缩比拒绝 |
| 单 XML / 累计已解析 XML | 8 MiB / 32 MiB |
| XML token / 深度 | 500000 / 64 |
| 实际遍历节点 / 引用 ID / 节 / 字段嵌套 | 50000 / 4096 / 256 / 32 |
| 准确对象读取 | 10秒有界；完整字节、摘要、关闭读取体 |

不新增第三方解析依赖；标准库 ZIP/XML、现有有界 OOXML 包读取器与进程内受限遍历完成处理。注释 ID 索引按部件缓存，避免对每个引用重复扫描整个注释文件。没有 shell、动态代码、Office 自动化或外部抓取。

## 验证、原件与待完成 / Evidence and remaining work

实际结果与复用边界见[检查记录](../quality/123-docx-text-verification.md)。新媒体是隔离合成数据的实际 Web／Taro H5 画面；MP4 为登录后视口录制，GIF 明确为三张实际截图导览。旧发布证据和媒体保留原字节；增量121完整回归、runtime和Android开发APK不改标为当前123执行。

发布核对允许计划中准确列出的正常新增结构，逐字节保留整个既有迁移目录（含加载器源文件）。独立旧发布文件使用最多8个并行读取核对同一Git字节身份，保留稳定结果顺序、1 MiB读取块及准确基准提交；这项本地核对优化不改变业务或平台验收范围。

PDF／OCR、完整 Office 创建／编辑、继承样式与布局、复杂修订／扩展及真实公司样本逐格式核对仍属 GAP-04。更广秘书工具与真实模型质量仍属 GAP-12；完整四端业务旅程仍属 GAP-18。23个工作包和5项移出没有变化，八项 Skill 仍为 draft。

Microsoft 的公开格式说明用于核对实现语义：[Word 文档结构](https://learn.microsoft.com/en-us/office/open-xml/word/structure-of-a-wordprocessingml-document)、[插入修订](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.insertedrun)、[删除文字](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.deletedtext)、[脚注引用](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.footnotereference)、[字段边界](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.fieldchar)、[批注读取](https://learn.microsoft.com/en-us/office/open-xml/word/how-to-retrieve-comments-from-a-word-processing-document)。采用有界文字提取的支持子集，不宣称实现完整 OOXML 规范。
