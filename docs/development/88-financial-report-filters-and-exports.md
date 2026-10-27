# 88 · 财务筛选、来源与限时导出 / Financial filters, sources and expiring exports

本增量推进 FIN-12、GAP-03、GAP-18：把月度发生支出、现金事实和订阅预测接入统一的报表查询、规范明细和异步 CSV 导出。Web、微信／抖音小程序及 Android 使用相同 Go 接口、查询约束和客户端校验。应用在 `app/`；文档、验证、截图、录屏和 GIF 在 `docs/`。完整四端上线目标仍须完成真实设备、云资源及生产验收。

This increment adds shared financial filters, canonical source pagination and expiring asynchronous CSV exports. The four clients use the same Go API and portable response guards. Application code stays in `app/`; documentation and evidence stay in `docs/`. The full four-client production goal remains open.

## 1. 用户流程 / User workflow

1. Web 从原有“月度发生支出”“现金报表”“订阅预测”入口进入；Taro 从工作台“财务核对 → 财务报表”进入。三个标签切换统计口径。
2. 选择月份，或明确填写起始日与结束日。起始日包含、结束日不包含；单次范围为 1 至 366 个自然日。发生支出与现金使用香港业务日期，订阅预测使用固定期间修订的本地日期／时区。
3. 选择全部可见记录、公司分摊或项目；再按原币、类别、供应方、经办／负责人、标题／来源编号筛选。点击“查询报表”后才应用条件。费用／现金的“经办”指原记录创建者，预测的“负责人”指固定订阅期间的负责人，不能解释成当前项目负责人。
4. 核对每个原币的独立合计、读取时间、未计入草稿和预测覆盖。系统保留 `base_currency=null`、`fx_coverage=unconfigured`，不猜测汇率，不把不同币种相加。
5. 查看原始明细，前后翻页，打开规范费用、收付款或订阅期间。每次响应中的合计与该页来自同一数据库快照；翻页是重新读取当前事实，不保证跨页历史冻结。
6. 有独立 `report.export` 权限的人填写导出用途，按已应用条件创建任务。`202 queued` 仅代表排队；后台实际执行时才固定 `as_of`、汇总、来源和权限依据。任务列表显示生成、重试、失败、取消或过期状态。
7. 生成成功后下载／打开 CSV。Web 与 Taro 共用分段、身份、字节数及 SHA-256 校验；在完整核验前不交付文件。可取消仍在排队／运行／待重试的任务。生成与取消保留幂等请求，网络结果不明时先核对任务，重试使用同一请求编号。

Open a report, apply an explicit interval and business filters, inspect separate original-currency totals, and open the canonical sources. Export is a separate authorized action: enqueue, wait for the worker snapshot, then verify and retrieve the complete CSV. A queued receipt is not a generated file. Pagination rereads current facts; only a generated export is fixed.

## 2. 四端差异 / Client behavior

| 客户端 / Client | 查询与核对 / Read and reconcile | CSV 交付 / Retrieval |
| --- | --- | --- |
| React / Vite Web | 三种口径、完整筛选、来源分页、原记录导航；个人任务按类型／状态分页 | 校验 JSON 分段后保存 CSV；同时提供完整鉴权 GET 下载接口 |
| 微信小程序 / WeChat | 同一报表、筛选、分页、任务创建／查看／取消 | 客户端支持 `shareFileMessage` 时分享校验后的临时 CSV，完成后删除临时文件；不支持时提示 Web 下载 |
| 抖音小程序 / Douyin | 同一报表与任务流程，原始明细可继续查看 | 当前没有启用 CSV 文件分享适配，明确引导 Web 下载；不伪装为已下载 |
| Android / Taro RN | 同一查询、来源与任务逻辑 | 私有有限缓存写入 CSV，核验长度后用本机文档应用打开；结束时释放临时文件与 URI 授权；没有查看器时显示真实失败 |

小程序财务阅读和审批仍属于首发范围。表内能力描述是实现范围；H5 录屏、编译及 JVM 测试不替代微信、抖音与 Android 真机验收。现有 Web／Android 120 GB 原文件上传流程独立，CSV 上限不改变原文件上传能力。

Mini-program financial reading remains in the launch scope. WeChat CSV sharing is capability-gated; Douyin directs file retrieval to Web. Android uses private temporary storage and a local document viewer. H5 recordings and compilation do not prove real-device behavior. The CSV limit is independent of the existing 120 GB original-file upload path.

## 3. 统计与权限约束 / Accounting and authorization

| 口径 / Basis | 计入规则 / Contribution | 不得混用 / Distinction |
| --- | --- | --- |
| `incurred` | 已确认费用按规范发生日与实际分摊计入；冲销在自己的生效日按负份额计入 | 草稿不计入；支出冲销不自动表示现金退款 |
| `cash` | 已记账支出、退款和更正分别保留；项目／公司使用已确认资金分摊 | 未分配现金单独列示，不能据此虚构项目现金支出 |
| `forecast` | 汇总已保存、仍适用的订阅预测期间与固定价格；排除作废／豁免期间 | 草稿、缺期间、缺价格、配置不完整及覆盖上限均明确显示，缺失不当作零 |

`report.read` 还需与每条来源的实际阅读权限相交。所有合计、明细、候选与导出保持 `authorized_records_only`。类别、供应方和负责人候选从当前可读且匹配的来源提取，分页后再选择；不借此枚举员工目录。供应方名称还需 `party.read`，否则只显示可读来源标题构成的相关方标签。负责人姓名只用于归属已可读的业务事实。

`finance_exporter` 是新增但不默认绑定的角色，包含 `report.read` 和 `report.export`；它不会授予费用、现金或订阅的阅读权限。任务仅对申请人本人开放。秘书运行身份不能创建这些人工导出任务；现有秘书财务摘要仍使用其独立工具边界。

后台生成、任务查看及每次下载重新核对当前用户／组织授权版本和来源权限。生成后的任何授权版本变化要求重新生成；来源变得不可读时不返回旧快照。任务列表在分页前过滤不可读任务，避免用数量或下一页信号泄露隐藏记录。

All summaries, choices and exports intersect report permission with canonical source permission. Export is independently granted and owner-only. The worker and every retrieval reauthorize; changed authority invalidates the old export. Hidden entries are filtered before pagination. No employee directory, guessed FX conversion or payment execution is introduced.

## 4. API 与不可变记录 / API and immutable records

| 接口 / Endpoint | 行为 / Behavior |
| --- | --- |
| `GET /v1/finance/reports` | `kind,from,to,currency,category,q,project_id,company_only,supplier_id,owner_id,cursor,limit`；每页 1–100 条 |
| `GET /v1/finance/report-options` | 同一查询约束，增加 `field=category\|supplier\|owner`、`option_search` 与分页 |
| `POST /v1/finance/report-exports` | `{filters,reason}` 与幂等头；返回 `202` 的固定任务回执 |
| `GET /v1/finance/report-exports` | 本人且当前仍有权的任务，支持类型／状态／游标／条数 |
| `GET /v1/finance/report-exports/{id}` | 重新鉴权的任务状态和有限文件元数据 |
| `POST /v1/finance/report-exports/{id}/cancel` | 原因与幂等头；取消任务并递增执行围栏，阻止旧 worker 写回 |
| `GET /v1/finance/report-exports/{id}/chunks` | 规范整数 `offset,length`；每段最多 512 KiB，返回完整及分段摘要 |
| `GET /v1/finance/report-exports/{id}/download` | 完整鉴权 CSV；不接受查询参数、Range 或 HEAD |

查询拒绝未知字段、重复字段、畸形编码、不兼容公司／项目范围和失配游标。来源、候选、任务的签名游标分别绑定用户与全部相应筛选。JSON 分段和完整下载共用全局三路、每用户一路的并发门，完整下载期间定期再次鉴权。

迁移 `0073_financial_report_exports.sql` 新增不可变导出请求和不可变派生快照。请求固定申请人、渠道、授权版本、筛选、用途、任务和到期时间；数据库约束要求关联精确的版本化队列任务。快照写入要求仍在有效租约内、围栏一致、条件相同、协议正确且来源条数完整。运行角色不能更新请求或快照，也不能删除未过期快照。数据库以自身 JSONB 文本编码重算快照摘要，下载时再校验。

The migration appends immutable export requests and derived report snapshots. Database guards bind the exact versioned job, live lease, fencing token and complete fixed scope. Earlier migrations retain their published bytes. Strict query parsing and independently bound cursors prevent accidental filter changes or hidden paging signals.

## 5. Worker、存储与运行 / Worker, storage and operations

| 项目 / Item | 固定约束 / Constraint |
| --- | --- |
| 后台命令 / Command | `platform worker-report-export`；本地通过 `python app/scripts/run_backend.py worker-report-export` |
| 队列 / Queue | `finance.report_export.v1`，`reports` lane，最多 4 次尝试；同一原生 worker 串行处理 |
| 执行 / Execution | 两分钟租约，单次工作上下文 90 秒；最终写入再次核对围栏与时钟 |
| 完整性 / Completeness | 最多 50,000 来源、CSV 16 MiB、应用 JSON 快照 30 MiB；超限明确失败，不生成截断文件 |
| 预测覆盖 / Coverage | 超过现有 1,000 订阅覆盖检查范围时导出失败，不能宣称覆盖完整 |
| 时限 / Retention | 请求起 24 小时；过期立刻停止文件交付并隐藏派生元数据，worker 每分钟分批清理最多 100 个过期快照 |
| 持久化 / Persistence | PostgreSQL 保存可限期清理的业务派生 JSON 和摘要；CSV 下载时重建，服务器不保留持久 CSV 文件 |
| 原文件 / Originals | 原文件继续走既有 OSS／R2；本功能不改变正式文件存储或两地副本策略 |
| 原生部署 / Native service | `app/deploy/native/copypaste-report-export-worker.service`；只需要 `CP_ENV`、`CP_DATABASE_URL`，不读取模型／OSS／TG 密钥 |

新增 worker 需单独部署、监控并保持数据库迁移版本匹配。没有 worker 时任务保持排队，前端不能自行宣告生成完成。生产应配置该服务开机启动；不部署此服务也意味着不会执行后台到期清理。

CSV 使用 UTF-8 BOM、CRLF 与 23 列，包含协议、过滤条件、原币汇总、来源数量及逐条规范 ID／修订／日期／时区／金额。金额始终是精确十进制字符串；用户导入电子表格时应把金额列设为文本。文本字段防止 `= + - @` 以及隐藏在空白后的表格公式解释；已验证的金额负数保留会计符号。

The database stores expiring derived business JSON; CSV is regenerated and hash-checked on authorized retrieval. Original documents remain in the existing OSS/R2 path. The worker needs only its database environment. Limits fail explicitly instead of silently truncating. CSV preserves exact decimal strings, source revisions and source timezones; spreadsheet text cells are protected against formula interpretation.

## 6. 验证与交付 / Validation and delivery

- [本增量验证记录 / Verification](../quality/evidence/finance-reports/verification.md)：当前源码、原生数据库、客户端构建、Android/JVM 范围和清理记录。
- [证据目录 / Evidence](../quality/evidence/finance-reports/)：实际结果、源码摘要、历史重建及保留的失败诊断。早期未冻结源码的尝试仅标记诊断，不作为可重建的通过证据。
- [媒体目录 / Media](../media/README.md)：真实合成数据界面截图、实际浏览器帧转码的 MP4／GIF；不录制账号口令或 MFA。
- [差异台账 / Gap register](../delivery/03-goal-gap-assessment.md)：保留完整四端与云／模型／TG／真实备份恢复／生产上线验收。

原生数据库场景分别核对人工算例、筛选与游标、只有报表权限的空结果、完整异步导出、旧围栏、原币／分摊／冲销、后续事实不改旧快照、取消、不同口径导出、本人任务分页、不可变约束、过期清理及授权失效。过期测试用隔离数据库的特权 fixture 模拟经过 24 小时，生产角色没有这项改时权限。客户端测试验证完整分段、错位／串任务／坏摘要、取消与失败时清空缓冲。所有本地检查使用原生工具，禁止 Docker、WSL、虚拟机或模拟器；结束即关闭自建浏览器上下文和临时服务。
