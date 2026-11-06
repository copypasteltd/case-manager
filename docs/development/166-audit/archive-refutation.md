# J-10 订阅日期边界独立反证 / Independent archive-date refutation

审查基线：增量165提交 `ddf6a70cc2de5fba5d01f64d581c4a228052c1d7`。本文件由未提出原候选的审查代理独立核对。只读规范与源码；没有执行测试、数据库、构建、服务、浏览器或真实集成，没有修改应用、迁移和旧证据。

**结论：保留 F-166-J10-01，`contradicted`，源码置信度高；运行反例尚未执行。** 项目结项中固定香港日期与普通订阅自己的 IANA 日期有实际差异，后置守卫全部复用同一错误谓词，无法抵消。后果为提前允许结项／归档，或在已经生效后继续阻塞；不是越权，也不证明供应方实际继续扣费。

## 规范依据与问题定位

| 编号 | 准确依据 | 独立判定 |
| --- | --- | --- |
| R-01 | `docs/requirements/03-projects-and-knowledge.md:27` PM-05：未实际停费或取消尚未生效的订阅不得默认为完成；`:29` 要求准确版本、完整依据、近期 MFA、同事务归档 | 当前适用；不能只凭内部 `cancelled` 状态判定已经生效 |
| R-02 | `docs/development/37-subscriptions-billing-and-forecast.md:15`：明确 IANA 时区；`:23`：未来取消显示待生效，生效日起停正常预测；`:27`：订阅自身归档要求已生效及供应方证据 | 用户位于香港不能推出所有订阅必须香港时区 |
| C-01 | `app/backend/migrations/0122_closeout_refund_units.sql:83`，最新 `project_closeout_items` 的 subscriptions 分支：`s.cancel_effective_on > (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date` | 日期口径冲突。准确函数名为 `project_closeout_items`；不是 `project_closeout_records` |
| C-02 | `subscription_reads.go:48–56` 查询数据库实际时间，再用订阅时区；`subscription_calendar.go:187–192` 的 `SubscriptionToday` 用 `now.In(zone)` 取当地日期 | 普通订阅采用自身时区，只有空时区草稿才回退香港 |
| C-03 | `subscription_mutations.go:344–391`：Transition 用 `v.Timezone`，cancel 接受当天或未来生效日；`:398–400`：取消未在当地生效即拒绝订阅自身 archive | 同一订阅可同时被普通模块标为待生效、被项目核查标为已完成 |
| C-04 | `subscription_reads.go:68–73` 的 `cancellation_pending` 同样按自身当地日期；`0038_subscriptions_and_forecasts.sql:137` 的预测资格按固定取消民用日期边界 | 不是只存在于 UI 文案里的时区差异 |

以上短文件名均位于 `app/backend/internal/finance/`，SQL 均位于 `app/backend/migrations/`。

## 后置守卫反证

| 层 | 已读调用／行号 | 能否抵消 |
| --- | --- | --- |
| 预览入口 | `httpapi/project_closeout.go:11–12` → `project/closeout_reads.go:96–126` → `closeoutSnapshot`（`:40–54`）→ `project_closeout_snapshot` | 不能；`CanClose/CanArchive` 直接使用返回的 `blocking` |
| 普通进入 closing | `project/service.go:397–410` 读取同一 snapshot，`closeoutBlocked` 拒绝所有已知阻塞 | 不能；不会重新按每个订阅时区算日期 |
| 人工归档 | `project/closeout_mutations.go:59–73` 权限／真人 MFA／完整可见性；`:75–105` 锁定来源、项目版本、状态、当前 fingerprint、当前 blocking；`:109–134` 归档与状态更新同事务 | 不能；权限、锁、近期 MFA 与当前依据存在，但业务布尔值来自同一函数 |
| SQL snapshot | `0046_project_closeout_and_archive.sql:67–69` → 当前 `project_closeout_items` | 不能；读取最新0122定义 |
| 归档插入守卫 | `0046...sql:96–107` `prepare_project_archive`：状态、真实 MFA、权限、完整来源、当前 snapshot、所有 blocking | 不能；没有独立时区守卫 |
| 项目状态守卫 | `0046...sql:108–111` `guard_project_archive_state`：closing 检查同一 items；archived 要求同一 snapshot 的不可变归档 | 不能；重复核对的是同一错误日期 |
| 延迟提交守卫 | `0046...sql:112–115` `validate_project_archive`：最终项目状态、同一 items、固定证据计数 | 不能；同样复用该谓词 |
| 订阅自身归档守卫 | `finance/subscription_mutations.go:398–441` | 不能；项目核查接受 `cancelled` 或 `archived`，没有要求所有订阅先执行自身 archive |

全迁移搜索 `project_closeout_items` 的 CREATE 定义得到0046、0089、0122；0122为最新。搜索 `prepare_project_archive / guard_project_archive_state / validate_project_archive / project_closeout_snapshot` 的 CREATE 定义只得到0046，未发现后续替换抵消此差异。

## 可由正常命令到达的跨日情形

下列均为**未执行的静态输入推演**。前提为：具有普通权限与真人近期 MFA，项目仍 active；供应方取消核对使用不在未来的真实记录时间及可读正式文件版本，当前供应方状态已确认 `cancelled`；没有其他十三类阻塞项。不能用虚构供应方证据完成真实验收，今后隔离测试仅使用合成证据。

| 情形 | 同一实际时刻 | 普通订阅日期与操作 | 当前项目谓词 | 应有结果 |
| --- | --- | --- | --- | --- |
| A：提前放行 | `2026-11-06T04:00:00Z`；香港11月6日12:00；`Pacific/Honolulu` 11月5日18:00 | 明确 timezone=`Pacific/Honolulu`；cancel effective_on=`2026-11-06`，普通取消允许未来当地日；普通读取仍 `cancellation_pending`，自身 archive 拒绝 | `11-06 > 11-06` 为假；若其余条件满足，订阅分支不阻塞 closing 和后续人工 archive | 当地仍11月5日，应阻塞取消尚未生效的订阅 |
| B：生效后误拦截 | `2026-11-06T14:00:00Z`；香港11月6日22:00；`Pacific/Kiritimati` 11月7日04:00 | timezone=`Pacific/Kiritimati`；cancel effective_on=`2026-11-07`，属于当地当天，可正常取消；普通读取已无 pending，自身 archive 日期守卫允许 | `11-07 > 11-06` 为真，仍阻塞项目结项 | 在其他依据均满足时，不应仅因香港尚未跨日而阻塞 |

普通可达性不依赖越权 SQL、篡改配置或修改系统时钟：

1. `httpapi/subscriptions.go:42–49` 是正常 Create 与 Transition 入口。`finance/subscription_mutations.go:158–205` 创建 draft；`subscription_content.go:52–68` 接受合法民用日期和 IANA 时区；`subscription_calendar.go:44–65` 不强制香港，也不把首次收费日限制为今天。
2. **最短路径**为 draft 直接 cancel：`subscription_mutations.go:376–391` 明确允许 draft→cancelled。未启用没有生成账期，故不会必然被 forecast 条件挡住。供应方取消证据与内部取消生效日分开记录，源码没有要求二者必须是同一当地日。
3. **不依赖草稿语义的路径**也成立：完整创建订阅，开始日取当前当地日、first_charge_date=`2027-12-01`、合法月锚点1及覆盖首次收费的价格；正常 activate 后再 cancel。完整性校验 `subscription_content.go:160`、SQL `0038...:269–272` 允许这种未来首次收费。`subscription_mutations.go:354–359,264–273` 的启用／提交本身不生成账期；`subscription_jobs.go:73–78` 只调度 first_charge < 当地今天+366 的候选，此处首次收费更远，未形成 forecast。即使后续请求正常生成，`subscription_period_mutations.go:93–100` 和 SQL `0038...:137–139` 会跳过不早于取消日的收费锚点。
4. 供应方证据校验 `subscription_content.go:105–140` 及 SQL `0038...:260–261` 限定状态枚举、实际核对时间不在未来、固定文件证据；它没有把未来取消生效日期强制改为已生效日期。不能把正确存在的供应方核对守卫当成独立取消日期守卫。
5. 配置在启用后固定：`subscription_mutations.go:284–288` 与 SQL `0038...:262–266`。这里的跨日时区在首次合法创建时即确定，不是非法启用后改时区。

## 是否必须把确切当地日期固定进依据

**应在新依据中明确显示并固定本次用于取消生效判断的时区和当地日期，但不把“每过午夜必须全部重做人工确认”虚构为现有独立验收要求。**

当前 `0122...sql:86–88` 已固定整个 subscription head／当前 revision（其 payload 包含 timezone）及所有账期，归档行另有绝对 created_at；因此“完全没有日期证据、历史无法重建”不是准确说法。现有依据主要缺少**实际用哪个当地日期得出当次 blocking 的显式事实**。`project/closeout_reads.go:97` 的响应 AsOf 使用 Go 当前 UTC 时间，并非参与 SQL 计算的确切单一时刻。

建议新增迁移，在当前 subscription item facts 中加入本次 evaluation 的明确 timezone、local_date、cancel_effective_on／pending 判定；至少让最终布尔值与 snapshot 采用一致日期来源。依据完整性与 fingerprint 的现有重读逻辑应继续消费这些事实；若加入每日变化字段导致过午夜 fingerprint 变化，应对用户说明重新核查。也可只固定影响结论的日期边界事实，避免无意义的每日变化，具体取舍需由本批实现统一规定。若采用统一 as_of，应避免每次调用变动的精细时间戳让同一事务中多个 snapshot 永不相等。

迁移应追加在130以后；不得重写0122、旧 `project_archives.basis`、旧 fingerprint 或旧证据。重开／再归档新增记录保持原约束。

## 一次性实现建议与后续批量核对输入

1. 在追加迁移中修正最新 `project_closeout_items` subscriptions 分支，按**当前固定配置的合法 timezone** 计算当地日期；明确空时区草稿回退规则与普通 `subscriptionDate` 一致。保持供应方证据、账期复核、共享费用等其余十三类规则完整。
2. 同步新 basis 的解释字段、Web／共享移动用户可见原因与说明（若当前页面仅显示通用状态，需要至少让待生效的日期／时区可辨认），保留旧历史读取兼容。该 UI 建议用于解释实际阻塞，不是本独立审查另行证明的缺失功能。
3. 全部本批代码保存后，在隔离数据里统一核对 A／B、同日香港基线、无时区 draft、正常合法时区、当前 vendor／账期阻塞仍生效、旧依据重提被拒绝、历史归档不可变。边界应以单一准确 as_of 投影来表达；不依赖机器运行时刚好跨午夜。

## 搜索、局限与未读范围

- 搜索了全部迁移的上述 archive／snapshot／items 定义，及普通 subscription date、cancel、archive、vendor、calendar、generator 路径；阅读的函数和行号如上。早期两次推测文件名与一次 PowerShell 通配路径搜索不存在，随后改为真实文件清单与明确文件路径补读，不据失败搜索断言 absent。
- 读取了 `archive-journey.md` 的候选说明以确定反证命题，随后独立阅读实际规范、创建／取消／归档和最终 SQL 守卫；未改该文件。
- 本报告没有运行反例、未独立穷举全部四端页面、所有项目业务模块或云端供应方流程；未读取凭据、真实公司文件、数据库和秘密配置。真实供应方取消确认、正式四端验收及其余23包缺口仍保持原状态。
- 不把更广泛 FX／账期完整性、一般审计或实际供应方停费重新包装为本条已经验证的缺陷；本条仅确认取消生效日期口径冲突与所有已读最终守卫未抵消。
