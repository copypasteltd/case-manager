# PDF、扫描件与图片正文 / Document text and OCR · Increment128

## 业务结果与使用

Web、微信、抖音与 Android 共用的正文查询链路支持 PDF、PNG、JPEG。PDF 每一页分别保存内嵌文字与该页渲染后的 OCR 结果，混合页面的图片文字也参与识别；图片直接 OCR。正文供搜索、准确版本分页预览、版本比较和秘书资料读取使用。四端展示 OCR 误读、遗漏、简繁体变化、金额／日期／合同核对原件的说明和完整页数。

使用顺序：Web／Android 上传原件 → 原件扫描通过、准确副本完整校验 → 独立正文 Worker 调度 → 在搜索中检索文件内的词句 → 打开“查阅此版本正文”。文件详情明确选择两个正式版本可比较派生文字；秘书会话明确选择该准确版本可读取同一份正文。微信／抖音负责查询与审批，上传职责不扩张。

OCR 产生可追溯的派生正文，不修改原件、合同、付款、费用、来源关联或项目状态。它不自动形成财务事实，不保证原版式、逐字准确、手写／复杂表格／复杂排版识别。PDF 内嵌文字和 OCR 可能重复；二者分别标注，不擅自合并或消除差异。

## 明确处理边界

| 对象 | 当前边界与结果 |
|---|---|
| 原件大小 | 单个待提取原件最多8 MiB；原有120GB上传目标与对象存储能力不因此缩小。 |
| PDF页数 | 1至20页，所有页面均处理内嵌文字及OCR；超限、加密、损坏或任一页失败不发布部分正文。 |
| 图片 | PNG／JPEG，最长边16384像素，总像素最多2000万；PDF渲染最长边2200像素、150dpi。 |
| 输出 | UTF-8、无NUL，总正文最多2 MiB；输出与诊断流分别有界，溢出取消进程。 |
| 时间／并行 | 单个原生调用15秒，整体提取75秒，Worker工作90秒、租约2分钟。OCR线程固定1；这是技术资源上限，AI费用控制仍移出。 |
| 语言 | 固定 `chi_tra+chi_sim+eng`，不接受文件或用户传入的命令、URL、模型路径、语言参数或shell。 |
| 临时资料 | 仅在已配置的本机临时根内建立私有操作目录；成功、失败和取消均释放。 |

空白页面返回明确“未识别到文字，请查原件”标记。已完整处理所有页面不等于识别到了所有可见文字，也不代表 OCR 正确率承诺。操作系统级内存、进程及网络隔离需要目标 Linux 服务身份与环境验收，当前 Windows 检查不代替该验收。

## 原生工具与部署

运行的是本机原生 Poppler 与 Tesseract；没有容器、WSL、虚拟机或模拟器。应用不捆绑或在线下载这些工具。管理员在独立只读工具目录安装后，生成本机固定清单；工具、DLL／库、Poppler映射、字体配置和三份模型的SHA256均列入清单。系统回退字体及目标Linux库解析尚未全部冻结，不声明跨主机逐字一致。

本地实际检查使用 Poppler26.10.0、Tesseract5.5.3、官方 `tessdata_fast` 三语言模型；版本、包构建、许可证、下载来源及模型摘要见[原生依赖记录](../quality/evidence/document-text/native-dependencies.json)。原生安装、模型、凭据和清单实际路径留在忽略目录，未提交 Git。许可证分发与生产依赖冻结继续属于 GAP-26。

```powershell
python app/scripts/build_document_text_profile.py --root C:\native\document-text --temp C:\runtime\document-work --tessdata share/tessdata-copypaste-v1 --output app/.local/document-tools/profile.json
$env:CP_DOCUMENT_TEXT_PROFILE = (Resolve-Path app/.local/document-tools/profile.json).Path
python app/scripts/run_backend.py worker-file-text
```

命令名称应与 `platform` 的 `worker-file-text` 子命令一致。生产使用服务管理器注入 `CP_DOCUMENT_TEXT_PROFILE` 的绝对路径，并按已有正文 Worker 配置独立只读存储身份。原生子进程仅继承固定 PATH／临时目录／语言等白名单，不继承数据库、OSS、R2、模型或 Telegram 凭据；数据库与存储访问由原有独立 Worker 完成。

配置错误阻止 Worker 启动；未配置原生清单时，普通文本／Office保持原功能，PDF／图片待处理且不调度原生任务。所有处理同一队列的原生 Worker 必须使用完全相同的清单／绝对工具和临时路径。清单摘要在任务入队固定，在每次处理时重新核对；工具及资产启动、执行前后复核。更新工具需要受控算法版本与队列部署方案，不能重写已有完成证据或悄悄换用不同清单。

## 接口、持久化与权限

复用现有 `/v1/search`、`/files/{id}/versions/{version}/text`、`/files/{id}/text-comparison` 和会话 `context-sources/file`。新提取标识为 `document-ocr-v1`。正文、比较的每个版本和秘书索引的完成证据增加可选 `native_evidence={engine_sha256,page_count,ocr_page_count}`；新格式 ready 必须有完整一致证据，旧格式不附加此字段。搜索文件结果增加可选 `text_extractor`，页面对 OCR 命中作出说明。

正常迁移0096只新增提取算法、三列及约束／守卫；旧迁移和旧终态结果保留。新任务固定组织、文件、准确内容版本、源摘要和引擎清单摘要；PDF旧 `text-v1` 结果与新结果分别保存。旧排队算法不被替换，旧不支持结果不会冒充新结果。

原件I/O及原生处理期间不持有数据库锁。发布时重新取得命名空间、版本与租约锁，核对当前文件／目录、扫描状态、来源副本、ETag／VersionID／完整源摘要。迟发撤回、替换、来源变化或租约过期不能发布正文。ready记录要求所有页面和OCR页数相等，且源与引擎依据固定；数据库阻止半页完成、改引擎和终态改写。

读取继续使用当前人类会话、文件／目录／版本、分类与范围授权；扫描隔离和当前撤权立即阻止派生正文。秘书额外保留会话全部当前参与者、AI策略、固定资料和工具执行边界。读取已完成正文不向模型提供对象凭据，也不再次解析原件。本轮只验证资料读取与证据，不宣称真实Codex／模型或八项draft Skill验收。

## 验证与保留范围

见[实际检查](../quality/128-document-text-verification.md)、[本轮及全部23项剩余工作](../delivery/05-functional-closure-128.md)。原生OCR使用公开合成夹具，实际 HTTP／PG、真实 Web 与共享Taro H5处理流程独立记录。小程序及Android JS构建属于源码检查，不等于真实平台／正式签名发布。

上游一手依据：[Poppler](https://poppler.freedesktop.org/)、[Tesseract命令说明](https://tesseract-ocr.github.io/tessdoc/Command-Line-Usage.html)、[安装说明](https://tesseract-ocr.github.io/tessdoc/Installation.html)、[官方模型](https://github.com/tesseract-ocr/tessdata_fast)、[原生micromamba安装](https://mamba.readthedocs.io/en/latest/installation/micromamba-installation.html)。这些说明不替代本仓库实际执行证据。
