# 增量93：秘书正文检索与固定DOCX资料 / Secretary body search and fixed DOCX context

本轮把增量92的文件正文索引接入公司秘书。员工可在原Web／Taro资料入口选择准确DOCX正式版本，核对派生正文后加入请求；模型侧原有 `search_resources` 与 `read_resource` 使用同一会话共同权限和持久化回执。应用仍在 `app/`，文档、验证和媒体仍在 `docs/`。

The secretary can discover permitted body matches and read fixed-version extracted DOCX text through the existing MCP tools. This is an implementation increment. All eight Skill packages remain draft; a controlled MCP fixture is separate from a live model, Telegram disclosure or production acceptance.

## 1. 员工使用与边界 / User flow

1. 打开当前有权进入的公司／项目会话，选择“准确文件版本正文”，再明确选择文件及正式版本。不会用最新版本替代历史选择。
2. 核对正文片段、原文件版本和SHA-256、提取器、正文SHA-256、索引时间及字符数。DOCX只含主文档已提取文字；版式、图片、关系、宏和嵌入对象须查原件。
3. 按服务端给出的Unicode字符偏移继续阅读。资料或提取证据改变时重新选择；未提取、失败或不支持时显示真实错误，不自动改读其他对象。
4. 将准确资料加入本次请求，填写要求后沿用原发送、运行、取消与结果流程。预览、翻页和加入资料不会自行发送消息、启动模型或批准写操作。

Web and Taro use the same optional evidence validation. The original file reference must match the selected file/version and original digest, have a clean scan, and agree with the ready derivation's character count and continuation. Original plaintext formats retain their existing reader and validation path.

## 2. 格式与执行路径 / Formats and read paths

| 资料 / Source | 检索 / Discovery | 阅读 / Read |
| --- | --- | --- |
| 项目、任务、知识 | 在实际共同可读投影的标题和正文匹配 | 原有准确版本与正文投影；知识采用所有成员共同可见的修订 |
| UTF-8文本／Markdown／CSV／JSON，1MiB内 | 当前正式版本的已就绪派生正文，以及正式文件名称 | 原独立AI只读身份下载完整原件并校验长度／SHA-256，再分段；不改变原读写语义 |
| DOCX，原件8MiB内 | 当前正式版本的已就绪主文档文字与名称 | 读取PostgreSQL中不可变派生正文；运行时不再下载或解析原件 |
| PDF、其他Office、超限或失败版本 | 可按有权读取的正式名称发现，返回明确提取状态 | 未实现正文时拒绝，不把名称检索当成全文已读 |

派生正文上限2MiB，每次最多10000个Unicode字符，客户端默认4000。解析器、压缩／XML限制及原始云来源核验继续遵守[增量92](92-file-text-search.md)。本轮未新增PDF、OCR、图片理解、DOCX写入或版式保真能力。

The persisted derivation records the cloud verification performed by the indexing worker. Reading that record does not prove a fresh remote HEAD/download or current real-cloud availability. Current formal file availability, clean scan, namespace, audience and AI permissions are checked again before disclosure.

## 3. MCP查找 / MCP discovery

`search_resources` 保留原参数：`query`、`resource_types`、`cursor`、`limit`；普通范围支持 `project/task/post/file`，服务摘要Skill另可查 `service`。项目计划仍使用原独立 `agent_plan` 分支及状态规则，不允许混合越界查询。

查询限定为当前会话的同一项目或公司范围。候选只在所有当前成员均可进行AI处理、分类不高于会话、准确文件依赖可用时参与匹配。对标题及实际正文作大小写不敏感的字面查找；`%`、`_`、反斜线不成为通配符。文件正文只匹配当前正式版本；明确历史版本仍可通过原读取工具核对。

每页最多20条，每项返回准确来源、领域摘要、命中位置类型、最多240字符片段，以及适用的 `text_index`。游标绑定运行、当前授权、查询、类型、每页数量和本轮查询语义；不能换请求、换筛选或换运行继续使用。候选数量仍受既有10000项保护上限约束；超过上限明确失败。本轮没有完成2TB规模与大目录性能验收。

The tool never treats a search snippet as an instruction or a write grant. Body results are an observed fragment, not proof that the entire file was read. Unsupported formats expose metadata and extraction state only. Audience and source checks occur before a result is returned or counted toward the visible page.

## 4. 固定来源、正文与回执 / Fixed evidence and receipts

| 字段 / Field | 含义 / Meaning |
| --- | --- |
| `version_id` / `evidence[].version_id` | 明确正式文件版本，不跟随当前头版本 |
| `evidence[].sha256` / `text_index.source_sha256` | 完整原件SHA-256；两者必须一致 |
| `text_index.extractor` / `text_sha256` | 派生算法版本与完整派生正文SHA-256 |
| `text_index.indexed_at` / `total_characters` | 正文终态证据的时间与总字符数，不是本次云下载时间 |
| `source_sha256` | 领域来源投影摘要；包含正式版本、分类、可见资料及适用的派生证据，不能当原文件SHA-256 |
| `offset` / `next_offset` | Unicode字符位置；到准确末尾返回null |

`read_resource` 的DOCX输出另返回 `read_mode=indexed-text`。会话预览的 `ConversationContextView` 增加可选 `text_index`；原始文本回复可继续不含此字段。OpenAPI保持285路径／353操作，新增一个证据Schema至622项，无新增数据库迁移，原75项迁移保留原字节。

派生正文通过原应用角色只读访问；运行时没有新的对象写入身份。每次读取重新核对执行租约、取消、预算、调用键、会话范围、所有成员权限与正式文件可用性。终态派生记录沿用数据库不可变约束。文件被回收、隔离、撤权或会话改变后，缓存文本不能绕过当前检查。重复相同调用复用持久化回执；复用调用键却改变版本或字符位置会拒绝。

Successful read receipts are committed through the existing execution ledger. Model-proposed IDs and operation claims cannot replace them. Completion references must match the exact observed file/version and current run authority; a read does not provide a committed write operation. The existing final-result dependency and withdrawal checks remain in force.

## 5. 配置与验证 / Configuration and verification

应用既有75项迁移后，索引worker继续按[原生索引运维](../operations/file-text-search.md)配置独立凭据。DOCX读取不需要向模型提供云凭据；UTF-8原件读取仍需要原 `CP_*_AGENT_FILE_READER_*` 独立角色。云索引凭据与AI原件读取凭据不得互相回退。现有API及worker的原生更新仍需先核对归属，不关闭用户其他服务。

八项draft源码包按本次共同阅读协议递增版本：`project-create`1.1.0、`project-update`1.8.0、`file-update`1.2.0、`information-organize`1.2.0、`oa-assist`1.2.0、`project-briefing`1.2.0、`subscription-review`1.3.0、`incident-summary`1.2.0。工具允许集合与准确写工具数量保持原规则。未安装这些新来源摘要的运行配置不能声称已升级；正式发布仍需真实模型评测。

当前源码、原生检查、UI像素、历史原字节与缓存复用范围见[准确验证](../quality/evidence/agent-indexed-search/verification.md)及[媒体目录](../media/README.md)。没有使用Docker、WSL、虚拟机或模拟器；浏览器完成后须关闭自建context、标签页和临时服务。

Real models and Telegram, devices, actual IAM/cloud objects, 120GB transfers, 2TB capacity, recovery, dependency security and the complete four-client release remain separate requirements in the [28-item gap register](../delivery/03-goal-gap-assessment.md). This increment does not close those gates.
