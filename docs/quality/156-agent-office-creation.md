# 156 · Office 新建核验 / Office creation verification

本轮按加速要求执行与新增功能直接相关的检查。使用原生 Go／PostgreSQL、Node22 和独立浏览器；不使用 Docker、WSL、虚拟机或模拟器。外部资料后续提供，真实 Codex、Linux 隔离、OSS／R2、TG 与平台接入不改称本地验收。

| 编号 | 核验内容 | 本轮证据及边界 |
| --- | --- | --- |
| 1 | 三种 Office 正式文件 | DOCX／XLSX／PPTX 真实二进制、隔离对象传输、清洁扫描、正式第一版和普通准确版本读取 |
| 2 | 范围与安全 | 员工固定目录／修订／文件名／格式；严格输入、危险公式拒绝、已有 Office 更新拒绝、处理中撤权停止 |
| 3 | 准确恢复 | 取消后以固定配方恢复相同字节，使用新扫描及准确第一版；模板／配方变更拒绝 |
| 4 | 原生选定结果 | `native-focused3.json` 共38条通过记录：新 HTTP 九子场景＋套件、Office／文本单位检查、MCP 边界及原有创建／恢复场景；三格式均核对人工完成回执，非受众不能读取；不是38条独立业务旅程 |
| 5 | 客户端与运行器 | Web7、共享移动4、当前运行器60项通过且无跳过；目录实际列出八个 draft 包 |
| 6 | 构建 | 工作区类型检查、Web／微信／抖音本地构建；移动共享源码适用 Android，本轮未重建 APK |
| 7 | 实际 Web | 五项操作核对：选择不执行、实际请求固定目标、消息固定意图摘要、受控执行后正式回执、普通准确版本入口读取 |
| 8 | 媒体 | 三张实际像素截图、无声 MP4 和明确标注的六秒截图导览 GIF；登录／MFA／私密夹具能力不进入画面 |

另执行四个现有上下文相关套件，共23条通过结果，记录为 `native-context-current.json`。这些是针对本轮公共引用路径修复的关联回归，不是完整后端回归。

原生检查使用合成对象存储适配器和受控模型／隔离证明驱动，业务 API、PG、文件生成、扫描状态、工具和提交路径真实执行；没有调用真实模型／云存储，Windows 检查不代替 Linux 实部署。浏览器先经过真实登录和 MFA，录制只覆盖合成业务画面。截图导览 GIF 由三张截图组成，不声称是连续操作录像。

## 失败与修正 / Failed attempts

1. `native-focused.json`：DOCX 已正式提交后，普通 Office 读取返回 `DOWNLOAD_SOURCE_UNAVAILABLE`。隔离测试下载适配器仅接入 Reader，遗漏 Signer；补齐测试适配器后重新执行38条记录。未绕过生产下载／权限检查。
2. `typecheck-1.json`：共享移动格式选择遗漏组件必填 `disabled` 属性；恢复实际禁用状态传入，随后对当前精确客户端／运行器输入重新执行选定检查及构建。早期通过记录仅代表当时输入。
3. 首轮浏览器检查的待提交提示完全匹配遗漏了页面实际包含的文件名；修正检查定位。第二、三轮分别保留未定位和已定位的任务读取404结果，不改称通过。
4. 浏览器发现真实人工任务完成读取的404：创建意图的历史可见性复用了包含AI专用文字索引的内容读取。将准确文件引用的受众／项目／分类／权限／扫描检查提取为共同路径，创建回执只调用该引用检查；AI正文读取仍执行原索引／授权逻辑。三种格式增加人工回执及非受众拒绝断言，并重新检查现有上下文受众、索引搜索、文件文字及文件更新流程。早期 `native-focused2.json` 未覆盖这一人工完成读取，不作为最终源码证明。

失败记录、原始通过记录和最终源绑定均保留。最终核验独立检查当前源摘要、OpenAPI 差异、113项既有迁移、八个冻结引用、媒体字节和资源清理；证据记录中的计数不能替代原始业务用例验收。

## 保留与延期 / Retained and deferred

完整后端回归121、完整客户端136及 rc.8／开发 APK154 的业务153证明按原字节保留。本轮运行器60项为新执行，不冒充旧151／58项；没有本轮完整后端／客户端回归、额外移动 UI、新 APK 或正式平台验收。原开发 PG100／API不升级、不重启；自建浏览器、临时服务和原生数据库检查结束即关闭。

English:38 selected native result records, Web7/shared mobile4 and the current runtime60 are fresh. The five Web checks use real API/PG/file commits with a controlled model driver and synthetic object transport. Retained broad regressions and APK154 remain historical. Failed fixture/typecheck attempts are preserved. No real cloud/model/native Linux or production acceptance is claimed.

[最终核验](evidence/agent-office-create/final-source-check.json) · [构建及运行器计数](evidence/agent-office-create/runtime-and-client-summary.json) · [实际Web证据](evidence/agent-office-create-web/result.json) · [全部剩余项](../delivery/05-functional-closure-156.md)
