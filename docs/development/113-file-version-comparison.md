# 文件版本正文比较 / File version text comparison

增量113推进 GAP-04／18：员工在文件详情中明确选择两个正式内容版本，查看完整提取正文的差异，并按固定依据分页查阅。Web、微信小程序、抖音小程序和 Android 共用同一接口与校验规则。项目、原件、正式内容版本和秘书草稿均有各自的历史；本功能读取正式内容版本，不把草稿或当前文件头替换为用户选定的历史版本。

Increment 113 adds explicit comparison of two exact formal file versions across Web, WeChat, Douyin and Android. The backend compares complete, already indexed text; clients verify version identities, original/text hashes, row continuity and pagination. This is a subfeature of GAP-04/18, not completion of either package. AI fee controls remain excluded; secretary, eight Skills, TG, authorization, isolation, audit and technical resource limits remain required.

## 操作与结果解释 / Usage and interpretation

1. 打开有权限访问的文件详情，找到“比较两个内容版本”。必要时加载更多历史版本。
2. 分别选择“比较基础版本”和“比较目标版本”。选择本身不读取差异正文；点击“比较所选版本”才请求比较。允许同一版本自比较。
3. 默认显示所有差异及前后各3行。中间省略的相同行显示起点与准确数量，不能解释为正文缺失。
4. 选择“全部正文”后重新点击比较，以每页60项读取全部行；可以前后翻页。每页使用首次比较摘要固定依据，不把历史版本改为当前头。
5. 阅读新增、删除、相同行以及末尾换行标记。可交换比较方向；改变版本或显示范围时清除旧结果，并等待下一次明确比较。
6. 文件元数据、权限、会话、版本可用性或扫描状态发生变化后，旧依据可能失效。界面隐藏旧比较结果；重新打开或刷新文件，核对当前正式资料再比较。

Usage: open an accessible file, select exact base and target versions, explicitly compare, then choose changes/context or all-text paging. Selecting or swapping inputs clears the previous result. Refresh current file metadata after a conflict. Admission, extraction readiness, original availability and a successful text comparison are separate states.

| 编号 | 结果 | 含义与限制 |
|---|---|---|
| VC-01 | 原件字节相同、提取正文相同 | 两个准确原件 SHA-256 相同，且所用完整正文相同。 |
| VC-02 | 提取正文相同、原件字节不同 | BOM、CRLF／LF、排版或其他非正文信息可能不同，不能称原件相同。 |
| VC-03 | 提取正文不同 | 返回全文推导的行差异；包括空行、Unicode 和末尾换行差异。 |
| VC-04 | 正文尚未提取 | 返回明确等待状态，不返回空差异作为“相同”的结论。 |
| VC-05 | 不支持、失败或不完整 | 不展示部分提取内容冒充全文；核对提取状态或查阅原件。 |
| VC-06 | 依据已变化 | 返回冲突，不带旧正文；重新核对现行资料。 |

支持已有 `text-v1` 提取器允许的 UTF-8 文本、Markdown、CSV、JSON 和 DOCX 主文档正文。提取会去除开头 UTF-8 BOM，将 CRLF／CR 规范为 LF；JSON 先验证格式，DOCX 主文档按既有提取规则组成正文。表格语义、样式、分页、图片、批注、页眉页脚及修订痕迹不构成完整原件比较保证。PDF、OCR、其他 Office 格式与复杂 DOCX 仍属 GAP-04。

## 接口契约 / API contract

`GET /v1/files/{id}/text-comparison`

| 参数 | 约束 |
|---|---|
| `base_version_id` | 必填，基础正式版本 UUID，必须属于当前文件。 |
| `target_version_id` | 必填，目标准确正式版本 UUID，必须属于当前文件。 |
| `view` | `changes`（默认）或 `all`。 |
| `offset` | 默认0，规范非负整数，单位为显示项；省略标记也算一项。 |
| `limit` | 默认60，1～100；客户端固定60。 |
| `comparison_sha256` | 首次可省略；后续偏移必须提供首次返回的准确摘要。回到第一页也可固定原摘要。 |

拒绝重复参数、未知参数、非规范整数、错误 UUID／摘要、超限和不合法显示范围。接口不接受客户端正文、替代原件、任意对象地址或指向其他文件的版本。

```text
GET /v1/files/<file-id>/text-comparison
  ?base_version_id=<exact-base-id>
  &target_version_id=<exact-target-id>
  &view=all&offset=0&limit=60

GET /v1/files/<file-id>/text-comparison
  ?base_version_id=<same-base-id>
  &target_version_id=<same-target-id>
  &view=all&offset=60&limit=60
  &comparison_sha256=<first-response-hash>
```

响应包含文件 ID／元数据版本、两个准确版本的序号／类型／大小／原件摘要／正文摘要／字符数／提取时间，`source_equal`、`text_equal`、`extractor`、`algorithm`、`complete`、完整行统计、分页坐标及比较摘要。`rows` 只有以下四类：

| 类型 | 基础坐标 | 目标坐标 | 内容 |
|---|---|---|---|
| `equal` | 原行号 | 原行号 | 相同正文，`count=1`。 |
| `delete` | 原行号 | `null` | 从基础版本移除的行，`count=1`。 |
| `insert` | `null` | 原行号 | 目标版本新增的行，`count=1`。 |
| `skip` | 起始行号 | 起始行号 | 仅 `changes`，省略的相同行，空 `text`，准确 `count`。 |

普通行的 `text` 不含行尾 LF；`newline` 单独表示这一行是否有末尾换行。空正文为0行，空行仍是实际行。`base_offset`／`target_offset` 记录本页开始前消耗的两侧行数，客户端逐项验证坐标及终页覆盖；`offset` 记录显示项数，与两侧正文行号不同。`all` 不允许 `skip`，遍历全部页可恢复两侧完整提取正文。

`comparison_sha256` 固定提取器版本、组织／员工／会话及其授权版本、文件 ID／元数据版本、准确版本与正文证据、显示范围和算法；不固定请求时间或每页长度。每次读取仍重新查当前授权、生命周期和扫描状态，摘要不是绕过权限的下载凭证。

主要错误：无权查看／跨文件版本404，会话撤销401，版本不可用、提取等待或比较依据变化409，扫描未通过、格式／提取不可用、行数或请求超限422，提取正文完整性核对失败502。失败不返回部分差异。具体安全提示以生成的 OpenAPI 和实际错误回执为准。

## 算法与资源约束 / Algorithm and technical limits

两侧先读取不可变的完整 `file_text_derivations`，验证 UTF-8、字节摘要与字符总数。现有提取器继续限制源文本为1 MiB、DOCX 原件为8 MiB、提取输出为2 MiB；额外限制每侧最多40,000行。120 GB 原件上传能力不表示可将120 GB 任意文件送入正文比较。

算法先固定共同前缀和后缀，在有界中段运行 Myers 行差异，最大编辑距离256、工作预算1,000,000，并检查取消。达到算法预算时使用 `bounded-replacement-v1`：完整删除基础中段、完整插入目标中段，仍保留全部正文，但可能把中段相同行计入删除／新增。界面必须说明该限制，不称最小编辑结果。普通结果为 `myers-v1`。

单次读取使用5秒技术期限，超限、取消或正文行数超限时不返回不完整“成功”。每页最多100项；行本身不截断。前端按页替换显示，不持续追加最多80,000个节点；长行换行显示。技术期限、行数和算法预算属于资源边界，与已移出的 AI 费用／token 计费或预算控制无关。

## 权限、事务和前端约束 / Authorization and client constraints

- 仅当前人工交互会话可用。沿用组织、项目、文件分类及会话撤销检查；秘书模型、MCP 和 TG 不因本功能获得正文读取能力。
- 在同一事务中读取并锁定当前文件／命名空间和两个准确版本，不写业务资料；版本锁按 ID 排序，避免正反方向比较互锁。
- 两个版本必须可用且扫描 `clean`。人工扫描例外 `waived` 不代替正文安全提取证明。
- 不触发云下载、扫描、重新提取、正式版本写入、上传计划、任务队列或秘书接单。提取器已有的后台链路负责原件读取、摘要验证与扫描前提。
- Web 和共享移动代码使用同一 `checkedComparison`：核对当前选择、版本证据、比较摘要、完整标记、行统计、行坐标、分页连续性、终页覆盖及相等结论。
- React／Taro 将差异作为文字展示，不执行 Markdown HTML 或文件中的脚本。删除／新增同时提供符号、坐标和文字说明。
- Web 查询在退出页面／换输入时取消，定时前台读取重新验证；移动请求由页面 Scope 关闭和取消。错误、权限或文件不可用时隐藏已有正文。

## 源码位置 / Source map

| 编号 | 实现 |
|---|---|
| VC-S01 | `app/backend/internal/files/text_comparison.go`：当前授权、完整正文证据、固定比较身份及分页。 |
| VC-S02 | `app/backend/internal/files/text_comparison_diff.go`：有界行比较、完整替换降级和上下文。 |
| VC-S03 | `app/backend/internal/httpapi/search.go`：严格 GET 参数；`app/scripts/build_openapi.py` 生成契约。 |
| VC-S04 | `app/packages/contracts/src/file-comparison.ts`：四端共享输入、完整响应和分页校验。 |
| VC-S05 | `app/web/src/FileComparison.tsx`、`app/mobile/src/pages/files/comparison.tsx`：明确选择和分页展示。 |
| VC-S06 | `text_comparison_diff_test.go`、`file_comparison_integration_test.go`、共享 `file-comparison-cases.ts`：算法、实际 API／PG 与边界用例。 |

数据库结构仍为87，本轮没有新增迁移，也不重新执行86→87升级。既有历史费用资料保持只读，公司 OA／财务功能继续保留。验证命令、构建、实际浏览器、截图／录屏／GIF、缓存范围和未完成项见[本轮验证](../quality/113-file-version-comparison-verification.md)及[全部23项](../delivery/03-goal-gap-assessment.md)。
