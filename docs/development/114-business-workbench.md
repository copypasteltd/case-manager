# 业务工作台 / Business workbench

增量114把审批、合同到期和订阅核对接入 Web 与共享微信、抖音、Android 首页。工作台展示当前员工有权查阅、由本人负责的业务事实。员工打开原业务详情核对后，继续使用原有审批、合同及账期流程。

Increment 114 adds assigned approvals, owned contract expiry and subscription-period review to Web and the shared WeChat, Douyin and Android home pages. It reads the current employee's authorized work. Actions remain in the ordinary business detail pages, where current authority and current revisions are checked again.

## 业务口径 / Business rules

| 编号 | 分组 | 纳入条件 | 展示与入口 |
|---|---|---|---|
| BW-01 | 待我审批 | 当前请求处于审核中；步骤为 active、分配给本人；步骤对应请求的当前修订与当前审批轮次；同时具有请求读取、审批及底层业务读取权限。项目变更额外核对预算权限，采购差异额外核对原采购权限。 | 当前请求标题、固定修订与步骤引用；打开原请求详情。 |
| BW-02 | 我负责的合同到期 | 未删除、已生效合同；使用 `active_revision_id` 的完整修订确定标题、负责人和到期日；负责人为空时归创建人；到期日不晚于香港业务日加30天。包括已到期合同，不设过期下界。 | 已到期／即将到期、正式到期日；打开原合同详情。 |
| BW-03 | 我负责的订阅待核对 | 已生成账期、订阅未归档；账期当前修订负责人，为空时归订阅创建人；当前 `can_read_subscription` 通过；账期原状态待核对、已到期或已有费用关联的预测账期、已确认但费用依据已变的账期。 | 账期开始日、待核对状态；打开原账期详情。 |

Contract dates use the immutable active revision and a fixed `Asia/Hong_Kong` calendar window, inclusive through today + 30 days. A later draft revision does not change the effective obligation used here. Subscription due dates use each period's stored timezone. The subscription projection follows the ordinary period list: a due forecast or stale confirmation requires human review. It does not establish an unpaid debt or an actual charge.

数量先按完整授权集合统计，再选前5条。按日期升序、更新时间降序、UUID降序排序；审批没有到期日，按更新时间和UUID降序。超过5条明确提示到对应业务列表继续查阅。业务列表具有自己的筛选规则；汇总不发放分页游标，也不声称列表默认筛选与汇总一致。

Counts cover the entire matching authorized set before a five-item sample is selected. Dates sort ascending, then updated time and UUID descending. Approval items have no due date. `has_more` discloses an incomplete sample; the existing business lists retain their own filters. There is no dashboard paging cursor or claim that a default business list uses this exact personal filter.

零表示“本人当前可读范围没有符合条件的事项”，不表示组织没有相关业务。权限不足的记录不参与数量、标题、日期或引用。三个分组不包含金额，不跨币种加总，不推测尚未生成的账期，也不自动审批、签署、续费或核销。

Zero means no matching work in this caller's current readable scope. It says nothing about the organization's total workload. Inaccessible records contribute neither counts nor metadata. These groups contain no amounts, currency totals, ungenerated periods or automatic business decisions.

## 使用 / Usage

1. 登录工作空间。首页“业务待办”展示三类负责事项、准确数量和最多5条记录。
2. 核对日期口径，点击具体记录进入请求、合同或订阅账期原详情。
3. 阅读当前依据，再使用原业务流程处理。汇总读取不等于审批、合同终止或费用已支付。
4. Web 使用“刷新业务待办”；移动端使用“刷新工作台”。刷新期间隐藏旧汇总，失败展示错误。Web 前台每30秒及重新聚焦时重新读取；移动端回到页面和明确刷新时重新读取。

Open the home page after login, review the counts and sample, then open a specific resource to inspect its current facts before acting. Web has a dedicated refresh button and foreground/focus refresh. Shared mobile refreshes when the page is shown or the user requests it. Old summary content is hidden while refreshing and after an error. Authority changes between reads are resolved on the next successful refresh and on every ordinary detail request; already received pixels cannot be retroactively recalled.

Web 的汇总查询与原任务／项目查询独立，缓存键包含用户和授权版本，退出组件即回收。移动端业务汇总的错误独立展示，原任务与通知查询仍保留既有错误处理；页面隐藏时关闭请求作用域并清除旧结果。不把读取失败显示成零条。

The new Web query is independent of task/project queries and has no retained cache after unmount. Its key includes user and authorization version. Shared mobile keeps a separate business-summary error within the owned page request scope. Page hide closes that scope and clears old results. A failed read is not rendered as a zero count.

## 接口与开发约束 / API and implementation

`GET /v1/dashboard/business` 使用现有认证、JSON信封及 `Cache-Control: no-store`。不接受查询参数、用户ID、组织ID、负责人、日期窗口或动态SQL。返回 `WorkbenchSummaryResponse`；三个固定分组必须完整返回。响应不携带业务正文、文件地址、金额、会话或凭据。

| 字段 | 含义 |
|---|---|
| `as_of` | PostgreSQL 事务时间；三个新分组共享同一可重复读快照。 |
| `scope` | 固定 `my_readable_work`。 |
| `business_date`、`contract_timezone`、`contract_through` | 香港业务日、固定时区、包含边界的30天截止日。 |
| `groups[].kind` | `approvals`、`contracts`、`subscription_periods` 各一次。 |
| `count`、`items`、`has_more` | 完整可读匹配数量、前5条准确引用、是否还有匹配记录。 |
| `items[].id`、`version`、`revision_id` | 原请求／合同／账期及其当前引用依据。合同修订明确指向生效版本。 |
| `related_id` | 审批步骤ID、订阅ID；合同为 null。 |
| `title`、`state`、`due_on`、`updated_at` | 通过当前授权后取得的必要业务元数据；审批日期为 null。 |

The new groups share one repeatable-read transaction. The existing task/project dashboard and notification summary remain separate reads; their timestamps are not a cross-module closeout attestation. Item links are fixed client mappings, never server-supplied URLs.

后端实现位于 `app/backend/internal/workbench/`。审批队列与工作台复用 `workflow.PendingApprovalsSQL`，防止权限、当前轮次和特殊业务检查分叉。合同和账期使用参数化固定查询及现有 `can_access`／`can_read_subscription`。不新增数据库迁移，schema保持87。

Shared response validation and route mapping are in `app/packages/contracts/src/workbench.ts`. Both clients execute the same boundary-case suite. Validation rejects missing/duplicate groups, wrong date windows, malformed references, impossible sample/count claims, invalid states and incorrect ordering. React/Taro render titles as ordinary text, including angle brackets.

## 授权与只读边界 / Authority and read boundaries

| 编号 | 约束 |
|---|---|
| BW-S01 | 仅 Web、微信、抖音、Android 人类交互会话；Agent运行、TG执行及伪造渠道／会话组合拒绝。 |
| BW-S02 | 事务依次锁定组织、用户和会话；核对当前账号状态、用户及组织授权版本、会话归属、渠道、撤销及到期。返回前再次核对时钟到期。 |
| BW-S03 | 每类记录在计数和采样前通过当前底层权限；本人负责也不绕过项目、分类、渠道或其他费用依据可见性。 |
| BW-S04 | 5秒技术时限、固定3分组及每组最多5条；任一读取失败时整体新汇总失败，不能返回部分成功或猜测零。 |
| BW-S05 | 不写业务表、审计、发件箱、通知事实、费用、付款或账期；不调用云存储、模型、TG或Worker。普通数据库读取和锁定仍需连接资源。 |
| BW-S06 | 角色撤销后旧标题和数量在下一次刷新被清除；原详情访问始终重新授权。保留现有会话、CSRF及业务命令协议。 |

The transaction locks serialize ordinary role/user/session revocation with this read. Human responsibility never grants missing domain authority. Technical deadlines and response bounds remain in scope; AI fee, token quota and billing controls remain excluded.

## 当前交付范围 / Delivery boundary

本轮推进 GAP-08 与 GAP-18 的子功能。服务运行事件、文件／复制异常等其他聚合、实际存储故障来源、跨渠道回执及公司规则选用继续开放。23个工作包仍待完成，5项移出不变。

This is a functional increment within GAP-08/18. Wider aggregation, actual storage failures, cross-channel receipts, company policy adoption and full client journeys remain open. Local native PG, browser/H5, build and package checks do not establish live cloud/model/TG, platform release or production acceptance. Real devices, network performance, actual-network 120 GB tests, historical migration/disaster recovery and AI fee controls stay excluded.

[验证与来源 / Verification](../quality/114-business-workbench-verification.md) · [全部未完成项 / Remaining work](../delivery/03-goal-gap-assessment.md)
