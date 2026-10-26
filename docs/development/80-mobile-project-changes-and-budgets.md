# 移动项目变更与预算修订 / Mobile project changes and budget revisions

本增量将[既有项目变更协议](43-project-changes-and-budget-revisions.md)接入 Taro 公共页面，供微信、抖音及 Android 使用。沿用 Go、PostgreSQL、原申请与独立审批，不增加另一套业务状态机。项目范围、日期、预算、实际费用、合同及付款各自保留原始业务事实。

This increment connects the existing project-change protocol to the shared WeChat, Douyin and Android implementation. Original Go/PostgreSQL commands remain authoritative. A proposal, an approval, budget implementation and actual spending are separate facts. Local H5 interactions exercise the actual API; they do not establish device or production acceptance.

## 1. 完整操作路径 / Complete workflow

1. 在项目的“项目变更”进入“新建完整变更提案”，或从“项目预算修订”提出预算调整。
2. 明确输入服务器已配置的三位大写币种，读取当前项目版本及该币种完整批准预算。系统不默认币种，也不把未查到的预算当成零。
3. 核对项目名称、范围、起止日期、版本、当前预算 ID、总额和全部明细，勾选明确采用本次基线。
4. 填写完整拟实施范围、范围影响、时间影响、拟定日期和原因，明确保持或完整替换本币种预算。替换模式零行表示明确清空。
5. 从当前有权文件中选择准确正式版本。草稿允许暂缺附件；准备申请前至少一个正式版本。保存新增不可变提案修订，不修改项目或当前批准预算。
6. 在变更详情准备申请草稿。随后在原申请中读取固定内容、附件、制度与指定审批顺序，明确送审，由指定人员独立决定。
7. 准备申请后若仍允许编辑，先保存新提案修订，再在原申请选择“更新申请草稿”。该动作另增申请修订，旧内容与决定保留，不自动重新送审或沿用旧批准。
8. 获批后返回变更，重新读取准确批准申请及当前基线，分别核实范围日期和预算影响，填写实施原因、完成本次本人 MFA，再明确实施。
9. 页面收到原回执后还要读回实际变更、实施记录、项目和本币种预算。替换预算时核对新批准预算；有原预算时再核对已取代前序。证明一致后才清理输入。
10. 从提案修订历史、原申请历史与预算修订分别查看当时内容及当前事实，不能以旧提案推断当前批准或实施状态。

Open a project's Changes or Budget revisions, select an explicit configured currency, read and accept the complete current basis, enter the full proposal and select exact formal file versions. Save an immutable proposal, prepare a request, explicitly revise it when necessary, inspect the original workflow and submit for independent approval. Approval still requires a separate authorized implementation with current basis, two human attestations and recent interactive MFA. Success requires actual source reads in addition to the receipt.

## 2. 页面、权限与阶段 / Routes, authorization and states

| 入口 / Entry | 实现 / Implementation | 原始权限与限制 / Original controls |
| --- | --- | --- |
| 项目变更列表 / Changes | 原项目页 section=changes；名称、状态筛选与游标分页 | project.read、change.read；列表不是全公司权限目录 |
| 新建／编辑 / Create/edit | 原项目页 section=change-form；项目、币种及准确 change_id | change.create／change.update、budget.read；实际成员非 viewer；项目 draft／active／on_hold |
| 变更详情 / Change detail | 原 finance 页 kind=change；原固定提案、申请链接、实施事实及历史 | change.read；服务器 can_edit／can_prepare_request／can_implement 再与实际项目／成员及动作核对 |
| 预算修订 / Budgets | 原项目页 section=budgets；币种、状态、准确 focus ID 与游标 | project.read、budget.read；草稿来源变更还须当前可读 |
| 准备申请 / Prepare | 原 change submit 命令，只生成申请草稿 | change.update、change.submit；准确版本／修订／摘要、当前基线、至少一个正式文件 |
| 原申请修订／送审／决定 / Request flow | 原 request、preview、revision、submit 和 decision 页面 | 原申请能力、完整固定快照、制度、独立审批顺序与准确版本 |
| 明确实施 / Implement | 重新读取批准申请与实际基线，原 implement 命令 | change.implement、project.update；当前 can_implement、批准准确修订、原 request revision、本人近期交互 MFA |

继续使用原有 16 个注册页面；不新建与原申请平行的移动审批服务。只读成员即使动作列表中还有其他范围的权限，也不会获得本项目写入入口。界面判断用于引导，后端仍在事务中按当前范围、成员、版本、分类、文件及业务状态重新授权。

The existing sixteen registered pages are retained. A viewer receives no project write affordance. UI checks guide the user; the original transaction rechecks current scope, membership, classification, versions, attachments and state.

## 3. 预算与金额约束 / Budget and decimal rules

| 用户选择 / Choice | 请求值 / Wire value | 实际含义 / Meaning |
| --- | --- | --- |
| 保持当前预算 / Keep | budget_lines=null | 保留本币种原预算 ID 与全部明细；仍核对项目范围／日期 |
| 完整替换 / Replace | budget_lines=[…] | 所列为本币种全部新明细；其他币种分别管理 |
| 明确清空 / Clear | budget_lines=[] | 建立零行预算提案，审批并实施后才生效；旧预算成为历史 |

- 预算金额始终是非负十进制字符串，最多 14 位整数及 6 位小数；币种是否配置、有效小数精度及总额由原服务再次验证。不使用 JavaScript 浮点数计算预算或审批金额。
- 最多 100 行。每行类别不为空且最多 200 字，真实 YYYY-MM-DD 日期、结束不早于开始；同类别区间首尾均包含，交叠拒绝。
- 原服务返回的“增加合计”和“预算净变化”分别展示。减少金额不抵消其他明细增加；客户端不从净额推算审批规则。
- 币种创建后固定。其他币种另建提案；预算不能与跨币种、跨期间费用直接相减。
- 日期留空明确序列化为 null，表示清除对应日期，不偷偷回填旧日期。
- 附件为最多 20 个不同正式版本，保留选择顺序，读取并核对原文件 ID、版本 ID、序号、原名、十进制字节文本和 SHA-256；不能用当前文件替代历史固定版本。

Keep, replacement and clearing are deliberately different wire values. Exact decimal strings, inclusive non-overlapping periods and configured currency precision remain mandatory. Server-provided positive increases and signed net changes are displayed independently. Budget approval/implementation does not create an expense, payment or contract.

## 4. 原接口与回执证明 / Original endpoints and result proof

| 接口 / Endpoint | 用途与准确依据 / Purpose and binding |
| --- | --- |
| GET /projects/{id}/change-basis?currency=… | 当前项目版本、完整范围日期分类与本币种原预算快照 |
| GET／POST /projects/{id}/changes | 名称／状态／cursor 列表；新建完整 content、base_project_version、nullable base_budget_id、reason |
| GET／PATCH /changes/{id} | 完整当前提案；编辑还带 expected_version、base_revision_id，原修订不改写 |
| GET /changes/{id}/revisions | 每页十份固定历史，继续按原 cursor 读取 |
| GET /changes/{id}/revisions/{revision_id} | 明确读取所选原修订及当时固定基线／附件，不用当前修订替代 |
| POST /changes/{id}/submit | expected_version、revision_id、content_hash、reason；返回 Request 草稿 |
| POST /requests/{id}/revisions | 原申请版本、当前业务版本／修订／摘要、reason；旧申请历史保留 |
| 原 preview／submit／approval decision | 沿用原固定内容与制度，不由移动端拼装另一套批准结果 |
| POST /changes/{id}/implement | 准确变更版本／修订／摘要、原项目版本、批准 request_revision_id、两项 true 声明、reason |
| GET /projects/{id}/budgets | 按明确币种与 draft／approved／superseded 状态独立筛选，列表每页二十项 |

保存的三字段回执本身不能证明完整业务：还要 GET 同一变更，核对新版本、准确修订、seq、base_revision_id、作者、原因、全部规范化内容、原基线及每个固定文件；再次读项目与预算基线，确认提案保存没有改变当前依据。

实施须核对真实 implemented／effective 头部及原不可变修订，实际实施 actor、原 request revision、原因、MFA 时间、前后项目版本及恰好加一的变更版本。实际项目的范围、日期、负责人、名称和阶段必须符合本次操作；实际本币种基线应指向准确目标预算或保持原预算。替换时还读取 approved 预算及 superseded 前序，核对来源提案、明细、币种、总额和 predecessor ID。操作证明读取各状态最新最多 100 份；若没有找到准确原记录，即视为结果尚未核对，不用其他预算替代。用户历史列表仍支持继续分页。

An HTTP 200/201 or a three-field receipt cannot clear the draft on its own. Async source proof reconciles exact revisions, actor, reason, files and frozen basis. Implementation additionally reads actual project/implementation facts and exact current/predecessor budgets. Missing, mismatched or unreadable proof leaves the outcome unconfirmed and blocks replay. The bounded proof window never substitutes another budget; the normal history view remains cursor-paginated.

## 5. 并发、恢复与访问收回 / Concurrency, recovery and access loss

- 使用既有业务命令幂等键。冲突、拒绝、响应未知或证明不完整时保留文字并阻止自动重放；先读真实变更和列表，不重复创建相同业务。
- “重新读取并核对基线”重新读取本人、授权版本、项目、成员、当前提案和预算。仅同一有效身份可回填当前页面文字；再次明确采用新依据，所有提案核实标志重新完成，不承接旧批准。
- 跨页面内存草稿只含明确白名单文字和稳定明细行 ID，按用户、会话纪元、授权版本、变更版本／源修订与基线 SHA-256 绑定。准备／实施原因另按准确业务修订及摘要绑定。
- 共享上限为 15 分钟、32 份及 100000 字符。文件选择、原基线对象、请求／回执、声明、MFA、验证码与令牌不进入恢复草稿；超预算会提示离页恢复限制。
- 重新读取提案失败时清除已展示事实及子历史工作区；不能以旧缓存继续显示受保护业务。写入拒绝保留文本供本人核对，不自动把拒绝当成功。
- 离页／卸载关闭所属请求；新身份或授权版本不接收旧响应。服务器要求最近 10 分钟本人交互 MFA，本地 9 分钟状态只是保守提示，验证后不会自动实施。

Conflicts and uncertain outcomes preserve text and block automatic replay. Explicit reread refreshes identity and original facts; acceptance and attestations must be completed again. Bounded drafts contain editable text only. Read rejection removes the displayed source workspace and history. Server-side authorization and recent interactive MFA remain authoritative.

## 6. 代码与验证 / Implementation and verification

| 位置 / File | 职责 / Responsibility |
| --- | --- |
| [change-form.tsx](../../app/mobile/src/pages/project/change-form.tsx) | 完整基线读取／采用、文字恢复、预算明细、固定文件、保存及异步证明 |
| [change.tsx](../../app/mobile/src/pages/project/change.tsx) | 固定详情、准备申请、独立实施、实际证明及不可变修订历史 |
| [change-input.ts](../../app/mobile/src/platform/change-input.ts) | 纯输入约束、完整快照、精确回执、预算及实施证明 |
| [request-actions.tsx](../../app/mobile/src/pages/business/request-actions.tsx) | 将 change 接入原申请修订；原送审／撤回／决定／历史继续使用 |
| [business.tsx](../../app/mobile/src/pages/project/business.tsx) | 变更查找、状态筛选、币种预算筛选及分页 |
| [边界测试](../../app/mobile/src/platform/change-input.test.ts) | 金额／日期／预算模式、精确基线、角色、固定文件、申请及真实实施证明 |
| [实际浏览器检查](../../app/scripts/check_mobile_changes.mjs) | 原 API 与共享页面、真实独立审批、MFA、并发、故障注入、分页、限制角色及清理 |
| [验证记录](../quality/evidence/mobile-changes/verification.md) | 准确执行范围、原证明复用、失败历史及新媒体来源 |

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_mobile.py mobile-changes
python app/scripts/run_mobile_changes_browser.py
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python app/scripts/build_android.py --google-mirror --evidence-group mobile-changes
python app/scripts/inspect_android_apk.py --evidence-group mobile-changes
python app/scripts/verify_mobile_changes_development.py
python app/scripts/freeze_mobile_changes_evidence.py
python app/scripts/encode_mobile_changes_media.py
python app/scripts/verify_mobile_changes_sources.py
```

原生工具安装、API 启动、平台配置与日常操作详见[中文指南](../README.zh-CN.md)／[English guide](../README.en.md)。检查器持有并清理自己的浏览器上下文、临时 H5 服务与隔离 PostgreSQL 数据库，不调用 Docker、WSL、虚拟机或模拟器。失败证据保留原始字节；只有准确最终来源的通过结果才计入本轮完成。

This increment advances GAP-01/18 and the local artifact/evidence portion of GAP-16. Remaining mobile knowledge/attachments and finance operations, real devices/platforms, model/Skills, cloud/TG, 120 GB/2 TB, security, performance and production recovery remain tracked in the [stable 28-gap assessment](../delivery/03-goal-gap-assessment.md).
