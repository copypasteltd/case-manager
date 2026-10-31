# 155 · 申请与交付跟进 / Application and delivery followups

首页新增三个真实业务入口：本人发起的草稿／退回申请、本人负责的待提交交付，以及当前可独立核对的他人已提交交付。原有任务和六组业务工作台继续使用各自接口；新增`GET /v1/dashboard/followups`单独读当前业务事实，支持Web及共享微信／抖音／Android首页。

The home page now follows the work after drafting or return, and after a formal delivery submission. Three groups use current authoritative records and current permissions. They lead to the ordinary request or project delivery pages, where revisions, files, MFA and human decisions remain enforced. This is a source155 update; frozen rc.8 and development APK154 stay fixed at business153/schema113. No new migration or APK is introduced.

## 需求与业务口径 / Requirements and business rules

| 编号 | 分组／约束 | 当前规则 |
| --- | --- | --- |
| FU-01 | 我发起的待推进申请 | `applicant_id`为本人，状态为`draft`或`returned`；当前申请读取和底层业务读取均通过 |
| FU-02 | 特殊OA范围 | 项目变更还须预算读取；采购差异还须原采购读取。沿用普通申请列表的权限口径 |
| FU-03 | 我负责的待提交交付 | 正式协议交付的当前`owner_id`为本人，状态为`draft`或`changes_requested`，当前具有项目读取、交付读取和提交权限 |
| FU-04 | 我可核对的已提交交付 | 当前状态`submitted`，固定修订属于该交付，本次提交人为其他人；当前具有项目读取、交付读取和验收权限。并不声称由本人专属负责 |
| FU-05 | 项目与里程碑 | 项目不处于归档／取消／结项处理中；关联里程碑存在、属于该项目且未达成／取消 |
| FU-06 | 真实版本 | 返回当前业务版本及真实固定修订编号。首次未提交交付可以没有修订，返回null，不构造占位修订 |
| FU-07 | 完整计数／有限窗口 | 先按完整授权集合计数，各组返回最近更新的5条；更新时间及UUID降序，`has_more`明确表示还有事项 |
| FU-08 | 独立人工办理 | 首页只读，不自动送审、提交、验收、标记已读或产生新的业务回执。打开详情重新核对当前操作资格与正式资料 |
| FU-09 | 数据最小化 | 仅ID、版本、修订、项目、标题、状态、更新时间；不返回正文、金额、附件明细、对象坐标或其他申请人的待推进申请 |
| FU-10 | 会话与范围 | 只允许四个人工渠道的有效交互会话；拒绝AI执行身份及TG渠道；不接收负责人、组织、分页或其他查询参数 |

Counts describe attention, not a guarantee that the next command will succeed. Current attachment access, fixed evidence, project versions and the exact business action are checked again in ordinary detail commands. A reviewer with broad permission may see several colleagues' submissions; the group is labelled readable review work, not an exclusive assignment.

## 状态闭环 / State closure

| 编号 | 业务操作 | 首页下一次读取 |
| --- | --- | --- |
| 1 | 草稿申请正式送审 | 从本人待推进组移除；审批继续进入原有审批组 |
| 2 | 审批退回申请 | 以当前版本重新出现于申请人待推进组 |
| 3 | 申请人重新送审 | 再次移除，保留原修订、审批轮次和决定 |
| 4 | 本人负责的交付正式提交 | 从待提交组移除；本次提交人之外、当前具有验收权限的人可以看到待核对入口 |
| 5 | 审核人要求修改 | 从已提交组移除，以当前状态／修订重新进入负责人待提交组 |
| 6 | 修改后再次提交并由另一人验收 | 使用新的固定修订；验收成功后移除待核对入口，保留全部原修订与验收记录 |
| 7 | 当前权限或分类范围变化 | 同时移除不再可见的数量和条目，不沿用旧授权快照 |

An inbox read marker or visiting a page does not resolve a followup. Only the underlying ordinary business transition changes this summary. An error or in-progress refresh hides the Web snapshot instead of presenting stale data as current. Shared mobile reloads the read on return to the home page and keeps a failed followup request separate from the other home data.

## 页面与使用 / Pages and usage

1. 登录后在首页“申请与交付跟进”核对三组数量。只显示当前授权事实；超过5条时去原申请或项目列表继续查阅。
2. 草稿／退回申请打开普通申请详情，按当前业务依据修订并明确重新送审。新读取确认事项已经移除。
3. 待提交交付打开正确项目的“交付”页签，并定位到该交付详情。核对正式文件版本、说明及验收准则后，使用原提交动作。
4. 已提交交付打开同一正式详情，核对准确修订及全部固定附件，再明确验收或退回。验收仍要求另一名有权人员、近期MFA和两项核对确认。
5. 操作完成返回首页，或点击“刷新申请与交付跟进”，确认当前结果。其他人的待推进申请和本人提交的自验收入口不会被混入该组。

Web links use the existing project route with validated `tab=deliverables` and delivery ID. The real project delivery component opens that record, checks its parent project and resets detail state when the selection or project changes. A mismatched authorized parent shows an error and exposes no delivery action. Shared mobile uses its existing request and delivery pages. The original six-group endpoint and its contract are unchanged, preserving older frozen clients.

## 实现与接口 / Implementation and API

后端新增工作流申请元数据查询及工作台跟进读取。读取沿用五秒期限、组织／用户授权版本共享锁、有效交互会话前后复查和单一一致事务。三组计数及条目在同一快照中计算，不由客户端拼接不同用户／时间的列表。

OpenAPI新增一个只读操作及四个结构，原接口／结构保持语义相同。共享协议校验分组、数量、窗口、状态、真实ID、null修订和排序，拒绝混入正文等额外字段；由固定路由生成器构建入口，不执行服务端返回的任意URL。标题作为普通文字渲染。

The implementation is under `app/backend/internal/workbench/followups.go`, `workflow/followups.go`, the shared `followups.ts`, Web home and shared mobile home components. Schema remains113. Application database upgrades and the original development PG100/API are unchanged.

## 交付与验收边界 / Delivery and acceptance

本轮集中检查真实原生HTTP／PG业务状态与权限边界、共享客户端协议、类型、Web及微信／抖音构建，以及真实Web入口与独立验收操作。首次失败的审批资料权限、测试角色重叠、非法结项夹具、缺失类型文件、浏览器夹具参数和未完成MFA登录均保留原记录，不能计为通过。详细结果见[核验说明](../quality/155-business-followups.md)。

The browser fixture deliberately branches after a short ordinary business setup; it does not stand in for all eleven focused native cases. The separate native run provides those cases. Media uses synthetic records and real Web/native API/database actions. Full regression, extra mobile UI journeys and a new APK are deferred for speed. Real provider/platform configuration and company acceptance remain open; user-excluded device/network/DR/AI-fee work stays excluded.

全部23个工作包、八项draft Skill及真实外部服务／正式签名／投产验收继续开放。[全部未完成编号表](../delivery/05-functional-closure-155.md) · [固定rc.8下载](../releases/v0.1.0-rc.8/README.md)。
