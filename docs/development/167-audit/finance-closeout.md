# 增量167：财务单位、项目归属与结项源代码核对

审查基线：`e36ec01a7b78eed742a7c1f6c831a4114b96bcde`。仅审查本条财务不变量，采用 `spec-to-code-compliance` 的规范→入口→服务→SQL约束→读口径追踪。没有运行测试、构建、服务、浏览器、云接口或真实业务数据；没有修改应用代码。本报告不是全23工作包验收，也不把已有测试记录当作本轮执行。

## 1. 被核对的要求与结论

本条要求：订阅、批准OA对应的实际费用、付款、退款、手续费、汇兑与摊销保留准确单位及项目归属；已经录入的真实未结义务不能通过项目结项；准确结清的异币本金不能仅因混用实收币种而留下阻塞。

直接规范依据：

- `docs/requirements/05-finance-and-subscriptions.md:23–27`：FIN-01要求预测、实际费用、付款独立；FIN-02要求同一实际事项只有一个费用ID；FIN-04要求分配不得超过付款及明确反向关系；FIN-05要求“全部分摊之和等于费用金额”。
- 同文件`:36–40`：FIN-07要求十进制字符串、币种精度、保留原币和准确汇率快照，币种之间不得直接相加。
- `docs/development/44-project-closeout-and-archive.md:7`：“审批通过、采购已完成、合同已终止、上传成功、供应方取消登记均不能替代其他事实。”同文件`:35–38`分别规定报销、实际费用、资金退款及订阅的阻塞口径。
- `docs/development/134-foreign-expense-refunds.md:37–41`明确 `amount` 为原付款本金、`cost_amount` 为费用原币、`refund_cash_amount` 为实收现金，并规定准确尾量和项目现金份额。
- `docs/development/160-batch-functional-closure.md:7`规定项目／采购结项按退款原本金核对，三种币种独立。
- `docs/development/166-batch-functional-closure.md:9,19`规定订阅当地取消日；日期满足仍须核查供应方及完整账期／费用。

**裁定：implemented（已追踪的当前普通业务与结项执行路径）；置信度：中高。确认的新代码背离：0项。** 当前实现已经修复旧版本的退款本金单位和取消日问题。没有找到一个可由当前普通命令到达、同时绕过当前SQL约束的反例；这不是穷尽性正确性证明。实际公司制度、真实财务样本和目标部署验收属于 `undecidable`，不可据本次静态阅读填写通过。

## 2. 完整执行链与单位依据

| 编号 | 入口与实际调用链 | 当前约束／事实单位 | 核对结果 |
|---|---|---|---|
| FIN167-01 | `internal/httpapi/costs.go:60` → `finance.RecordBusinessCost`，`internal/finance/business_cost.go:144–214` | 固定批准来源类型、ID、头版本、修订及摘要；`:179–180`要求原币相同；`:194–208`使用来源项目／分类，普通创建→确认→准确关联在同一父事务执行 | 不将批准自动视为实际费用，不由组合入口另写金额／分摊规则 |
| FIN167-02 | 普通费用确认 → `internal/finance/content.go:80–94,105–127,144–152`；正常升级后的 `migrations/0041_purchase_variance.sql:433–486` | `core/money.go:13–39,54–67`使用整数定点；确认必须原币精度正确、项目／公司份额合计等于总额。最新 `validate_cost` 另检查准确来源币种、批准／执行上限、项目覆盖；订阅可表达真实超额扣款 | 金额与项目归属在服务和当前SQL约束两层校验；未把旧0036函数误作最新实现 |
| FIN167-03 | 订阅账期关联 → `finance.LinkSubscriptionCost`，`internal/finance/subscription_period_mutations.go:354–405` | `:363–377`固定费用版本／修订／摘要、已确认费用、账期币种／供应方、订阅项目覆盖；`:391`新增准确账期关联；确认分支`:439–453`依据真实费用集合及指纹，不以预测充实际 | 当前账期引用已钉住资料。账期指纹不包含付款变化，但项目费用分组另行核查完整核销，不能据此跳过费用待结 |
| FIN167-04 | `internal/httpapi/costs.go:48` → `finance.RecordCostPayment`，`internal/finance/cost_payment.go:164–249` | 核对实际已付与准确费用；`:200`重查来源；`:212–231`沿原费用管理项目创建现金、普通登记再普通分配，同一事务提交 | 现金总额和本次费用核销额独立，组合入口没有绕过普通分配 |
| FIN167-05 | `internal/httpapi/payments.go:57` → `finance.AllocatePayment`，`internal/finance/payment_allocations.go:54–137` | `:94–105`付款原币输入及当前可用现金、独立费用币报价；`:116–134`费用份额、付款份额分别计算并按相同项目位置保存；`:14–47`持久化 `amount / cost_amount / refund_cash_amount` 及各自份额 | 不能以付款币金额充当费用原币，项目份额在保存时逐位置对齐 |
| FIN167-06 | 当前普通分配及费用结算SQL → `migrations/0101_foreign_refund_expense_attribution.sql:14–84`；`0089_cross_currency_settlement.sql:125–135` | 最新FX分配函数核对本金／费用币及精度、准确反向尾量、根级和每项目费用余额；费用结算按 `sum(effect*cost_amount)`，每项目不超过原费用份额 | 双币金额与双币项目份额守恒；已冲销费用不能留下有效分配 |
| FIN167-07 | 退款登记／后续归属 → `internal/finance/payment_foreign_allocations.go:14–68` → 普通 `inverseAllocation`；当前SQL为 `0101_foreign_refund_expense_attribution.sql:198–277` | 原付款本金、费用回转、实收现金分别处理；实际现金按准确剩余本金比例和实收币精度计算；SQL再次复核公式、最小单位、项目最大余数、整笔上限及冲销原份额 | 正确保留三套单位和项目归属；最后本金可取得准确剩余现金，不额外推定汇率 |
| FIN167-08 | 合同节点现金／退款／解除 → 当前 `migrations/0102_receivable_foreign_returns.sql:28–115,117–144` | 节点金额、原收入本金与异币向外退回的现金分别保存；`:13–25`返回独立现金详情；`:126–132`项目现金份额按 `contract_return_remaining_cash`；`:138`资金容量按适用现金／本金额校验 | 不把合同本金作为异币现金报表金额。`refund_original_amount` 的非异币回退在 `0093_actual_foreign_refunds.sql:2–4`为普通原金额，不是零额度 |
| FIN167-09 | 手续费关联 → `internal/finance/payment_fees.go:116–159` | 绑定已有原付款、已有手续费现金、已有费用和准确正向分配；需有效双币余额、同管理项目／分类；保留每个事实各自币种金额。数据库证据约束位于 `migrations/0091_payment_fee_evidence.sql:43–97` | 手续费通过普通费用／现金计入；关联本身不是额外金额或第二次发生费用 |
| FIN167-10 | 汇兑预览→人工登记→独立历史 → `internal/finance/fx_journals.go:89–160,224–314`；`migrations/0118_payment_fx_journals.sql:39–101` | 用原费用发生日和实际付款日分别估值准确剩余双币；绑定原分配、同管理项目／分类、准确汇率书和两日报价。历史依据变化退出有效汇总；按基础币分组，原始金额不被改写 | 汇兑台账是独立口径。没有发现把损益重新计为原费用／付款或用其放行结项的调用 |
| FIN167-11 | 人工摊销→独立报表 → `internal/finance/amortization_schedule.go:49–87`；`migrations/0083_cost_amortization.sql:102–138`；`internal/finance/amortization_report.go:13–33` | 整数最小单位计算；每月合计、全期总额以及每项目／公司份额分别守恒；报表按原费用币种和准确份额带符号查询，不改费用／现金 | 不将年费现金、实际费用与摊销相加；已有确认费用仍由原费用结算核查 |
| FIN167-12 | 发生／现金明细和现金合计 → `internal/finance/report_sources.go:14,26–39,127–128`；`internal/finance/cash_report.go:25–28,68–80` | 发生按原费用份额；全局现金按实收总额，指定项目按 `refund_cash_amount` 优先份额及合同实际退回现金；现金总计 `GROUP BY currency` | 原本金、费用与实际现金未互相替换；未分配资金保留独立金额，不猜测项目归属 |

上述 `internal/...` 与 `migrations/...` 均相对 `app/backend/`。路径与行号绑定审查基线；没有因为符号名字相似就推断调用存在。

## 3. 结项不会只依赖前端状态

实际入口为 `app/backend/internal/httpapi/project_closeout.go:10–23`。

1. `project.Closeout` 和归档复用 `closeoutSnapshot`，直接读取 PostgreSQL 的 `project_closeout_snapshot`；`internal/project/closeout_reads.go:40–54`在超过5000项时明确拒绝，未截断后返回通过。该快照函数在 `migrations/0046_project_closeout_and_archive.sql:67–68`调用当前同名 `project_closeout_items`，因此采用131的最新定义。
2. `project.Transition` 在 `internal/project/service.go:360–362`拒绝旧普通archive捷径，`:397–407`进入closing前复核实际阻塞。`ArchiveProject` 在 `closeout_mutations.go:17–24,63–104`按顺序锁项目、相关费用、相关资金，核对真人近期MFA、完整权限、当前版本、完整依据指纹及阻塞。明确人工业务／财务核实位于`:46–47`。
3. `migrations/0046_project_closeout_and_archive.sql:96–115`仍有归档插入、阶段变更及事务末尾约束：准确当前快照、无阻塞、完整授权、准确归档记录与阶段一致；应用检查之后不能仅靠改项目状态放行。
4. 项目费用集合由 `0046...sql:2–8`覆盖管理项目、当前分摊项目、合同／采购／报销／订阅来源；资金集合由`:10–18`覆盖关联费用、合同来源及递归原付款／退款／冲销家族。共享费用检查整笔义务，是 `44-project-closeout-and-archive.md:36`的明确设计。

当前131定义的财务阻塞：

| 类别 | 精确当前实现 | 保留的实际条件 |
|---|---|---|
| 合同 | `migrations/0131_closeout_subscription_calendar.sql:13–16` → `0089_cross_currency_settlement.sql:248–266` → `0040_contract_closure.sql:40–45` | 生命周期／审批／有效修订、实际节点本金、未归属退款、当前履约核对、费用原币净核销及子合同 |
| 采购 | `0131...sql:17–20` → 当前 `0122_closeout_refund_units.sql:16–44` → `0042_purchase_completion.sql:74–83` | 履约、差异、费用确认／结清、准确原本金未归属退款；取消仅豁免必须有验收交付，不豁免财务 |
| 报销 | `0131...sql:21–28` | 员工垫付以 `cost_amount` 对照批准原币；确认费用须准确批准修订且币种一致、金额完整 |
| 实际费用 | `0131...sql:29–34` | draft或费用原币净分配不等于当前有效应结金额；冲销后有效分配不等于零也阻塞 |
| 资金／退款 | `0131...sql:35–38` → `0122_closeout_refund_units.sql:3–13` | draft阻塞；退款以 `refund_original_amount + sum(effect*amount) - contract_finance_remaining`计算未归属原本金，独立提供本金币种。已逆转资金不再次算有效退款 |
| 订阅 | `0131...sql:39–44` | 非取消／归档、缺少取消日、按当前固定订阅IANA时区尚未生效、缺供应方取消／核查时间／凭据、forecast账期或当前实际集合指纹失效；安全日历事实固定业务日，不用每次变化的精确时刻污染指纹 |

具体单位反证：若退款实际收到100 USD、退回原付款本金780 HKD，已归属原本金780 HKD，则当前助手函数使用 `780 - 780 = 0 HKD`；131直接调用该函数，不再用100 USD与780 HKD相减。若只归属400 HKD，仍有380 HKD阻塞。此为源代码代数追踪，**没有执行算例或宣称测试通过**。

## 4. 已排除的误报与待定边界

| 编号 | 候选现象 | 裁定与依据 |
|---|---|---|
| B01 | 旧migration46/89仍含实收额／本金差式或香港固定日期 | 不是当前生效定义：122修退款单位，131在其基础上修订阅当地日期。历史SQL和旧快照应保持不变，不能修旧文件制造“所有历史曾经正确” |
| B02 | 一笔付款还有未分配现金但能结项 | 文档明确普通资金余额不自动等同费用欠款（`44...md:37`）；真实费用与退款义务仍独立阻塞。不能将未分配现金本身新增为公司欠款规则 |
| B03 | 订阅取消后未生成未来账期 | forecast是预测，供应方停费凭据及已生成账期／已录费用仍检查；`44...md:38–40`明确系统外义务须人工核实。本系统不是供应方自动对账接口 |
| B04 | 摊销或汇兑没有代替费用结清／新建自动会计费用 | 明确支持边界：`05-finance-and-subscriptions.md:17,68,95–104`；`158-batch-functional-closure.md:10`。两者是人工独立视图／台账，不能据此认定隐藏漏记会计总账 |
| B05 | 当前公司财务制度／税制／基础币／实际报价、真实凭据未采纳或未提供 | undecidable，继续GAP-03及实际配置清单；不发明默认公司事实，也不取消AI之外的财务费用能力 |
| B06 | 全量业务样本、真实四端、Linux／PG／OSS／R2等正式验收 | 本次未执行，继续开放；与已证明的代码调用链区分。没有用开发APK、旧单元结果或本报告代替验收 |

没有确认需要本轮修复的财务代码项，所以没有虚构修复提交或测试通过。若后续独立反证给出具体可到达反例，完整修复应沿原普通命令及追加迁移保护同一单位，并更新使用该单位的项目／采购／合同快照、现金及导出读口径；须保留既有账簿、不可变归档与冻结包，不通过改旧证据或按任意汇率回填解决。公司制度应由有权人员明确选择，不能用技术修复代填。

## 5. 实际检索及阅读范围

执行过的关键只读检索与结果（Windows中不依赖shell展开通配符；少数初次错误路径已经使用目录配合 `-g`修正，错误检索不作为无结果证据）：

| 检索 | 结果及后续 |
|---|---|
| `rg --files docs app/backend/migrations app/backend/internal/project`配合finance／closeout文件名筛选 | 定位05需求、44结项协议、134／135／158／160／166说明及0046／0089／0122／0131；大输出截断后逐文件限定行号重读 |
| `rg -n '^CREATE( OR REPLACE)? FUNCTION (project_closeout\|contract_closure\|purchase_completion\|subscription_actual)' app/backend/migrations -g '*.sql'`（实际使用正则分组及alternation） | 找到当前同名函数的所有定义；project items最新131、purchase snapshot最新122、contract snapshot最新89、closure predicate40、purchase blockers42，未把旧定义当当前 |
| `rg -n '^CREATE( OR REPLACE)? FUNCTION (validate_cost\|validate_cost_settlement\|validate_subscription\|guard_cost_amortization)' app/backend/migrations -g '*.sql'`（同上使用实际alternation） | 费用最新校验41、费用结算89、订阅38、摊销83；逐段重读当前定义 |
| `rg -n 'project_closeout_\|closeout\|archive'` 对project读写及0046（实际使用alternation） | 定位HTTP入口、当前预览、普通transition、人工归档、SQL插入／阶段／延迟约束；未仅停留GET按钮 |
| `rg -n 'cost_amount\|refund_cash_amount\|refund_original_amount\|project'` 对finance目录及89／93／100／101／102／118／122（实际使用alternation） | 分别跟踪费用单位、原本金单位和实际现金单位，包括正常组合命令与独立报告 |
| `rg -n 'reportCashContribution\|payment_allocation_shares\|refund_cash_amount' app/backend/internal/finance -g 'report*.go' -g 'payment_*shares.go'`（实际使用alternation） | 定位当前发生／现金明细、项目现金份额、现金合计及导出共用来源；逐段阅读 `report_sources.go` 与 `cash_report.go` |
| `rg -n 'type Decimal\|func ParseDecimal\|func CheckCurrencyAmount' app/backend/internal/core -g '*.go'`（实际使用alternation） | 实现位于money.go，确认整数定点，未根据文件名猜测不存在的decimal.go |

全部源文件通过UTF-8行号片段读取。没有把未读完的截断输出当已审查证据。本轮没有检查任意SQL超级用户直接篡改、未录入的外部合同／银行事实或实际财务样本准确性，也未重跑历史测试。范围内保留上述证据支持的实现结论，范围外继续明确开放。
