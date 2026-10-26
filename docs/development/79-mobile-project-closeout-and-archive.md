# 移动项目结项与固定归档 / Mobile project closeout and immutable archive

本增量接通既有 Go/PostgreSQL 结项协议，在现有 16 个 Taro 注册页面内加入“项目 → 项目结项与归档”。微信、抖音与 Android 共用页面；H5 用于本地真实接口验收和演示。原业务规则、表结构和 Web 页面未改动。对应 GAP-01／18，页面证据推进 GAP-16，完整四端和生产验收继续开放。

This increment connects the existing Go/PostgreSQL closeout protocol to the shared mobile project page. WeChat, Douyin and Android share the implementation; local H5 checks exercise the real API. It adds no parallel state machine, schema or Web rewrite. Full client/device and production acceptance remain open.

## 1. 操作路径与阶段

1. 从项目详情进入“项目结项与归档”，读取当前身份、项目、实际成员和结项依据。
2. 按当前权限执行启动、暂停、恢复、取消或受控重开，每次填写原因。阶段命令只改变原项目阶段；不会替代财务、合同或订阅处置。
3. 展开十三类事项，沿原记录链接核对和处理。尚有阻塞时不显示进入结项动作；不能在移动端把待处置改写成通过。
4. 完整核查且具备权限时，明确进入 `closing`。普通业务写入按后端规则只读；仍需处理业务时先恢复进行，再处理原记录。
5. 填写成果、经验、后续跟进三项复盘和归档原因；无后续事项也须明确说明。可选择 0—20 个当前可用的正式文件版本。
6. 分别确认业务闭环与财务／合同／订阅处置，再完成本次本人两步验证，最后明确提交归档。
7. 原归档回执还须与实际项目、实际归档历史核对。核对成功才清理文字输入并重读页面；响应丢失、依据冲突或证明不完整时先核对真实结果。
8. 归档历史保留当时清单、版本、三项复盘、声明、原因、操作者及固定文件。重开后旧记录标记依据变化；再次归档新增一份记录，不覆盖旧复盘。

Employees open Project → Closeout and archive, inspect the complete checklist, dispose original obligations, explicitly enter closing, complete the retrospective, select optional fixed file versions, attest business and financial closure separately, complete current MFA and confirm archive. Success requires the original receipt plus actual project/history reads. Reopening preserves old reviews; another archive creates a new immutable review.

| 动作 / Action | 原始允许阶段 / Original source states | 原始权限 / Action |
| --- | --- | --- |
| 启动 / Activate | draft → active；范围、负责人、成员须满足原规则 | project.update |
| 暂停 / Hold | active → on_hold | project.update |
| 恢复 / Resume | on_hold、closing → active | project.update |
| 进入结项 / Close | active、on_hold → closing；完整核查无阻塞 | project.update |
| 取消 / Cancel | draft、active、on_hold → cancelled | project.update |
| 重开 / Reopen | archived、cancelled → active | project.reopen |
| 归档 / Archive | closing → archived；完整依据、复盘、声明、近期本人 MFA | project.archive 及所有依据读取权限 |

项目 viewer 不获得写入入口，即使身份动作列表中还有其他范围的写权限。界面仅辅助判断；原后端会再次按当前成员、范围、版本和业务事实授权。阶段变更不隐含归档，没有通用 transition → archive 旁路。

## 2. 十三类核查

| 分类 | 页面核查含义 |
| --- | --- |
| 任务 tasks | 完成或取消；受阻、待审核仍需处理 |
| 里程碑 milestones | 达成或取消；逾期不是完成 |
| 交付 deliverables | 固定修订、实际人工验收及来源完整性 |
| 风险 risks | 关闭或转为实际问题，关联问题独立核对 |
| 问题 issues | 已解决仍需明确关闭 |
| 变更 changes | 已批准与已实施分开，依据不完整仍阻塞 |
| 预算 budgets | 历史版本参考；不据预算推算费用、承诺或余额 |
| 合同 contracts | 履约、补充协议与结算独立核对 |
| 采购 purchases | 完成／取消不免除费用、退款及变更核对 |
| 报销 claims | 批准、实际费用和垫付结算分开 |
| 实际费用 costs | 费用与完整有效分摊结算、冲销及原资金用途 |
| 资金 payments | 原付款、退款、冲销及实际分配分别核对 |
| 订阅 subscriptions | 内部取消、供应方实际停费、账期及真实发生分别核对 |

完整规则继续由[既有结项协议](44-project-closeout-and-archive.md)和原数据库约束定义。空清单只说明当前系统已录入资料及规则，没有证明现实中不存在未录入义务。不能用本次表单声明替代真实处置。

`GET closeout` 一次读取同一事务快照；客户端校验恰好十三种唯一分类、条目所属类别、真实 ID 和正版本、条目数与阻塞数。预算条目保留实际预算修订。后端完整核查上限 5000 项，超限拒绝，不截断为通过。页面每类先显示 20 个已读取事项，可继续展开显示；完整核查数量与当前显示数量分别说明。

权限受限类别必须是 `count=null`、`open_count=null`、空条目；整体 `visibility=restricted`、无 fingerprint、无 close/archive 能力。页面明确显示“尚未核实”，不会显示零项已通过。历史受限不显示复盘、数量、附件或游标。

## 3. 接口与准确回执

| 接口 | 请求／结果要点 |
| --- | --- |
| GET /projects/{id}/closeout | 当前项目、as_of、13 组、visibility、fingerprint、can_close、can_archive |
| GET /projects/{id}/members | 当前成员及项目角色；与身份动作共同控制入口 |
| POST /projects/{id}/transition | expected_version、原 action、reason；准确项目版本加一且只变为预期阶段 |
| POST /projects/{id}/archive | expected_version、expected_fingerprint、三项复盘、reason、两项 true 声明、固定 file_version_ids；201 项目及归档回执 |
| GET /projects/{id}/archives?limit=10&cursor=… | 全部历史当前授权核查后分页；受限不泄漏历史数量或游标 |

归档 fingerprint 是原协议的 32 位十六进制并发依据标识（MD5），不是文件校验摘要或密码学签名。文件 SHA-256 仍独立保留 64 位摘要。项目版本和 fingerprint 均来自当前真实响应，不能由客户端构造“通过”依据。

三项复盘各 1—20000 字，原因 1—200 字；界面和内存草稿使用平台字符串长度，因此含补充平面字符时可能比后端 rune 限制更严格。所有附件版本必须不同；按原服务返回的 UUID 顺序序列化并核对，选取顺序不影响准确文件身份。保留原始文件名、版本序号、十进制字节文本和 SHA-256，避免大字节数浮点失真。候选文件使用既有正式版本选择器，禁止将未提交原件当成正式附件。

阶段回执须属于同一项目、恰好版本加一、目标阶段准确且保留原项目范围／负责人／模板／日期等头部字段。归档回执还须准确匹配本次复盘、原因、当前操作者、旧依据 fingerprint、固定附件和全部分类条目。随后再次 GET 实际项目及最新十份历史，查到本次 archive ID，确认实际项目归档版本与当前依据、当时条目及创建时间一致。任一证明缺失，200／201 不会直接变为成功提示或清空输入。

历史每次按原游标读取十份；拒绝混入其他项目、缺失分类、重复记录、受限但泄漏条目的响应。后端超过 500 份历史时拒绝本次完整核查；客户端不改为跳过旧记录。操作者姓名标为“当前姓名”，不冒充当时冻结显示名；旧事项链接按当前权限读取当前原记录，固定核查内容仍是归档当时版本。

## 4. 冲突、会话与输入恢复

- `409`／权限变化／证明不完整／响应未知：沿用共享命令和幂等约束，保留已输入文字、禁止自动重放，先读真实项目及历史。
- “保留文字和选定版本，重新核查”先重新读取身份、权限版本、完整依据与成员；仅同一有效身份且仍有完整归档权限时回填当前页面的文字和选定版本。
- 重新核查重建表单，两项声明和 MFA 重新完成；若实际已归档，则直接展示实际阶段与历史，不再保留可提交的旧归档表单。
- 跨页面只保留有界会话文字：以用户、会话纪元、授权版本、项目版本及本次 32 位 fingerprint 绑定；共享 15 分钟、32 条、100000 字符预算。重新读取不会延长输入寿命。
- 附件选择、验证器代码、MFA 标志、声明、令牌和 API 结果不进入恢复草稿。授权变化／退出使旧句柄失效；离开页面关闭所属请求，迟到结果不能恢复旧上下文。
- 真实后端仍校验最近 10 分钟交互会话 MFA；页面 9 分钟本地状态仅是保守交互提示，不是服务端授权证明。验证不自动提交归档。

Unknown results, permission changes and concurrent revisions preserve input and block replay. Explicit reread refreshes identity, source facts and membership. Retained notes remain bounded to the exact session, authorization version, project revision and closeout fingerprint. Attachments, attestations, MFA and credentials are never restored as drafts. Server-side authorization and recent interactive MFA remain authoritative.

## 5. 实现与验证入口

| 位置 | 职责 |
| --- | --- |
| [closeout.tsx](../../app/mobile/src/pages/project/closeout.tsx) | 核查、阶段表单、复盘、重新读取、异步证明与授权历史分页 |
| [project-closeout.ts](../../app/mobile/src/platform/project-closeout.ts) | 完整范围校验、原命令输入和实际回执／回读证明 |
| [project-closeout.test.ts](../../app/mobile/src/platform/project-closeout.test.ts) | 12 项有效／无效依据、权限、文件与结果证明测试 |
| [drafts.test.ts](../../app/mobile/src/platform/drafts.test.ts) | 新归档 fingerprint 及共享预算、白名单、授权失效测试 |
| [实际浏览器检查器](../../app/scripts/check_mobile_closeout.mjs) | 原 API／H5、真实并发、断网、分页、限制角色及所属资源清理 |
| [验证记录](../quality/evidence/mobile-closeout/verification.md) | 本次执行、准确复用、失败历史、APK 与媒体摘要 |

原生检查命令从仓库根目录执行，不启动虚拟化或模拟器：

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_mobile.py mobile-closeout
python app/scripts/run_mobile_closeout_browser.py
python app/scripts/build_android.py --google-mirror --evidence-group mobile-closeout
python app/scripts/inspect_android_apk.py --evidence-group mobile-closeout
python app/scripts/verify_mobile_closeout_development.py
python app/scripts/verify_mobile_closeout_sources.py
```

先安装和配置本地原生工具，详见[中文指南](../README.zh-CN.md)／[English guide](../README.en.md)。媒体生成还需要原生 FFmpeg；浏览器检查结束关闭全部所属上下文、临时服务和隔离数据库。保留失败尝试原始字节，修正后重新采集准确来源。最新结果与命令细节以验证记录为准，不把 APK 编译或 H5 页面演示当成 Android／微信／抖音真机通过。

移动变更实施／预算编辑、其他模块完整移动操作、真实模型／云／TG／平台／设备、120 GB／2 TB、安全、性能、部署及恢复仍由[28 项稳定差异](../delivery/03-goal-gap-assessment.md)继续跟踪。
