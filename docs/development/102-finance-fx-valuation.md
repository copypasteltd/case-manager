# 增量102：基础币种、人工汇率版本与财务折算

本轮推进 GAP-03。公司管理员可明确发布基础币种与人工汇率版本，员工为发生、现金或订阅预测报表选择准确版本，核对覆盖情况、原始记录和折算依据，再生成固定版本的 CSV。金额计算不经过浮点数。Web 与微信／抖音／Android 共用的 Taro 报表页均接入。

这是一套报表折算能力。跨币种付款核销、汇兑损益、手续费归集、摊销计划仍是 GAP-03 的后续工作；实际公司的基础币种和报价内容由有权限的员工在系统内选择。本次没有为公司预设 HKD，也没有接入或假称核验第三方实时报价。

## 1. 功能与操作

| 编号 | 功能 | 使用方式与约束 |
|---|---|---|
| FX-01 | 发布基础币种 | 财务报表 → 选择汇率版本 → 发布人工汇率版本。需组织配置范围的 `finance.fx.manage`、当前交互会话及 MFA；内置角色为“财务汇率管理人”。 |
| FX-02 | 填写报价 | 填写标题、基础币种、人工报价表和发布依据，并明确确认来源及舍入口径。报价表每行六列，Tab 分隔，最多2000行。 |
| FX-03 | 查阅历史 | 历史版本分页显示，详情保留发布人、依据、报价日、区间、精确汇率和内容摘要。修正时发布新版本。 |
| FX-04 | 明确选用 | 选择“用于当前报表”；没有选择时继续按原币分列。发布新版本不会自动替换已有筛选或已生成导出。 |
| FX-05 | 核对覆盖 | 显示已覆盖金额／笔数、缺汇率笔数及原币净额、未定价、未计入草稿；存在缺口时完整合计为空。 |
| FX-06 | 核对来源 | 发生、现金、预测仍独立列示。每条明细说明折算状态、基础币种金额和汇率日期／来源，可进入原始业务记录。 |
| FX-07 | 固定导出 | 按所选版本排队生成 CSV，保留版本ID、修订、SHA-256、筛选、原币及每条折算依据。生成、下载和分段读取继续执行既有当前权限检查。 |

首次使用时，由有角色授权权限的管理员进入“成员与授权 → 授予角色”，选择“财务汇率管理人”，授权范围选择“本组织”并记录原因。员工完成两步验证后重新登录；发布仍检查当前会话和近期MFA。个人或项目范围的该角色不能修改公司基础币种。

报价表格式：`币种 → 起始日 → 结束日 → 报价日 → 汇率 → 来源`，其中箭头表示一个 Tab。可从电子表格复制六列后粘贴。金额和汇率都保留十进制字符串。

下面仅为合成示例，无真实财务或报价含义：

```text
USD	2025-01-01	2025-02-01	2024-12-31	7.8123456789	Synthetic treasury quotation
```

该行表示2025-01-01至2025-02-01之前，1 USD 按7.8123456789基础币种单位折算；报价日独立记录，不会因为用于以后期间而改写。基础币种无需人工填写1倍报价；空报价表仅覆盖基础币种本身。

## 2. 计算口径

| 编号 | 规则 | 结果 |
|---|---|---|
| FX-R01 | 人工区间报价 | `manual_interval`；区间为 `[valid_from, valid_to)`，最长366天。同币种区间可相邻，不可重叠。 |
| FX-R02 | 明确报价方向 | `rate = 基础币种单位 / 1原币单位`。支持最多10位小数，必须为正数，不接受科学计数法。 |
| FX-R03 | 逐条范围金额 | 使用当前筛选范围内的贡献额，乘该条汇率，按基础币种配置精度四舍五入后相加，口径名 `scope_line_half_up`。负数的中点向远离零方向舍入。 |
| FX-R04 | 分摊尾差 | 公司、项目等分别筛选时分别舍入，分项之和可能与全量范围相差最小币种单位。该视图不产生新的核销分录，也不把尾差自动写回业务。 |
| FX-R05 | 日期 | 发生使用业务发生日；现金使用支付业务日；订阅预测使用已保存期间起始日。 |
| FX-R06 | 冲销 | 冲销在其实际冲销月份体现负贡献额，但匹配报价时继承原记录业务日期。真实退款使用退款自己的日期。 |
| FX-R07 | 缺少报价 | 不补1、不视为0。保留已覆盖小计，完整合计为 `null`，按原币显示缺口金额和笔数；净额抵消为0也不掩盖缺失笔数。 |
| FX-R08 | 草稿与未定价 | 草稿单独计数，不计入确认金额。非草稿缺少币种或贡献额时为未定价，阻止显示完整合计。 |
| FX-R09 | 预测源覆盖 | 订阅尚有缺失期间、配置或其他覆盖缺口时，`source_coverage_complete=false`；即使已保存期间都有报价，仍不显示完整预测折算合计。 |
| FX-R10 | 原币事实 | 原费用、付款、分摊、订阅价格、期间和原币汇总不改写。原币汇总始终独立，不能直接跨币种相加。 |

数据库使用 PostgreSQL `numeric`；Go 对分页明细使用 `big.Rat`／`big.Int` 进行同口径计算。汇总在与明细相同的可重复读事务中，对相同授权谓词筛出的全部记录计算；不会把当前一页冒充全量，也不会把全量记录无限搬到服务内存。分页游标绑定完整筛选与所选报价版本。

## 3. API 与数据库约束

| API | 作用 |
|---|---|
| `GET /v1/finance/fx-policy` | 当前最新版本或明确的 `version=0,current=null`。 |
| `GET /v1/finance/fx-books` | 版本元数据及签名游标分页。 |
| `GET /v1/finance/fx-books/{id}` | 精确历史版本内容。 |
| `POST /v1/finance/fx-books` | 提交 `expected_version/content/acknowledged/reason` 与幂等键，发布新版本。 |
| `GET /v1/finance/reports?...&fx_book_id={id}` | 在原币报表上附加所选版本的 `valuation` 与每条来源依据。 |
| `POST /v1/finance/report-exports` | 在 `filters.fx_book_id` 固定所选版本，复用现有队列、快照、下载和失效协议。 |

读版本接口支持可选 `project_id`，用于校验项目范围的 `report.read`；它不会授予公司财务记录的读取权限。未知、重复查询条件及换范围游标均拒绝。发布限定员工操作，检查组织资源的 `finance.fx.manage`，个人所有权、项目或其他单项资源授权无法发布公司口径；不接受 Agent run。其他组织的版本ID按不可见处理。

迁移81追加 `finance_fx_books`，保留迁移1—80原字节。每个组织的修订连续递增；内容为规范JSON并在数据库校验SHA-256，行不可更新、不可删除。数据库同时检查币种、区间、报价数量、精度、来源和元数据绑定；公司基础币种变化必须与最新发布版本相符。

发布命令在读取当前版本前取得组织独占锁，避免从授权共享锁升级导致的并发死锁。比较预期版本、插入版本、更新基础币种、写入幂等回执／审计／事件在同一事务中提交。两个基于同一旧版本的发布只有一个成功，另一请求收到409；完全相同幂等请求可以在后续版本发布后读取原成功回执。

新字段均为兼容追加：不选择汇率版本时，原筛选序列化、原币接口和23列 CSV 保持旧协议字节。选择版本的 CSV 附加基础币种、状态、匹配日期、折算额、精确汇率、报价索引、报价日、来源及有效区间，并在元数据保留版本及覆盖汇总。旧快照不会跟随最新汇率改变。

## 4. 客户端与验证范围

Web 使用现有 React＋Vite 报表布局、品牌色和对话框。Taro 在同一报表页提供选择、详情、人工发布、覆盖提示和排队导出，编译到微信、抖音和 Android；没有新建替代入口或缩减四端范围。

验证与演示使用原生 PostgreSQL 隔离合成库、真实HTTP API和实际浏览器像素，原始凭据与临时文件只保留在忽略目录。截图／录屏／GIF从登录后业务页面开始，不录密码或MFA。浏览器上下文、页面、临时服务和夹具在检查后关闭。

运行结果、失败／中断尝试、来源摘要、Android JVM缓存范围及受影响媒体见[验证记录](../quality/102-finance-fx-verification.md)。真机、网络性能、历史数据迁移／容灾和AI费用控制仍按[修订99](../delivery/04-current-scope-and-acceptance.md)移出；真实云、Codex、TG和生产发布等保留目标继续开放。

## 5. GAP-03 后续

| 编号 | 待完成内容 | 验收方向 |
|---|---|---|
| FX-N01 | 实际公司财务口径选用 | 由有权限员工选择基础币种、报价依据和使用期间；示例报价不充当公司正式配置。 |
| FX-N02 | 跨币种付款与核销 | 固定原币、结算币、人工确认汇率、手续费和汇差，保留核销分摊及冲销链；不得把报表折算当作核销。 |
| FX-N03 | 摊销计划 | 明确启用条件、起止日、最小单位尾差和更正规则，创建独立版本化计划；不重复计入发生与现金。 |
| FX-N04 | 四端完整财务旅程 | 合同／采购／报销／订阅至实际付款、核销、冲销、报表、过期与撤权的完整功能验证；真机仍不作为验收门槛。 |

## English usage and boundary

An authorized administrator first grants the Finance FX Manager role in Members and Authorization with the organization scope; the employee completes MFA and signs in again. Open Financial Reports, expand quote versions and explicitly select an immutable book. A finance FX manager with an interactive MFA session and permission on the organization can publish the base currency and manually reviewed interval quotes. Own, project or unrelated-resource bindings cannot publish company policy. Each TSV row contains source currency, inclusive start, exclusive end, quote date, decimal rate and source. The rate is base units per source unit. Publish corrections as another version; selection and completed exports remain pinned.

Incurred, cash and forecast views retain their original facts. Each authorized scope contribution is converted and rounded half-up to the base currency precision before summation. Missing quotes or prices produce a coverage gap and a null full total. Drafts are excluded. Reversals reuse the original fact date; actual refunds use their own date. Incomplete forecast source coverage also prevents a full total. CSV snapshots preserve the exact book, digest, source rows and quote evidence, while existing exports without a selected book retain their previous bytes.

This increment does not complete GAP-03: cross-currency settlement, exchange differences/fees, amortization and complete four-client finance acceptance remain open. Synthetic quotes do not configure the real company. See the verification record for executed checks, retained evidence and remaining production limitations.
