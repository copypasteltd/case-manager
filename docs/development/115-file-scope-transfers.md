# 跨范围文件整理 / Human reviewed file scope transfers

增量115实现员工明确发起的跨项目／分类文件及目录树移动。Web、微信、抖音、Android共用接口和计划校验。它推进GAP-18的文件操作子流程；整个GAP-18、GAP-11的AI变更计划、真实模型／Skill验收继续开放。[当前范围](../delivery/04-current-scope-and-acceptance.md) · [验证](../quality/115-file-scope-transfers-verification.md)。

## 1. 业务行为

普通“改名或移动”继续处理相同项目、相同分类的整理。跨项目或分类时，由独立入口选择实际目标目录，建立不可变计划，展示完整影响后明确确认。分析与执行分开，分析不会移动文件。执行修改目录归属和分类，不搬运OSS／R2对象字节，不重写正式内容版本、扫描记录或副本来源。

The user selects an actual authorized destination, reviews a fixed inventory and changed read-policy combinations, then explicitly confirms one exact plan. Execution preserves immutable content and object provenance. This is a human business action; no new agent tool or implicit permission grant is introduced.

| 编号 | 操作 | 结果与约束 |
|---|---|---|
| FT-01 | 选择来源 | 单个当前文件，或非根目录的完整子树；根目录不能改变项目归属。 |
| FT-02 | 选择目标 | 四端均从实际授权目录选择；根目录显示实际项目名称。确认目标的准确ID和版本；同项目同分类使用普通移动。 |
| FT-03 | 分析 | 固定来源、目标及其祖先路径、全体子项、正式内容历史摘要、读取策略矩阵和阻塞原因。可拟定受阻计划供核对，不能批准执行受阻计划。 |
| FT-04 | 本人核对 | 展示全部受影响项、分类前后、普通／AI读取策略组合数量及逐项增减；这些组合不是人数，不证明TG群或模型实际可访问。 |
| FT-05 | 明确确认 | 本人勾选完整清单声明并填写原因；执行要求最近10分钟MFA。Web可使用当前有效MFA，移动端明确完成步骤验证。 |
| FT-06 | 原子执行 | 重新检查当前来源、目标、祖先、全树、历史、权限和会话；与固定摘要一致才执行。任何不一致均不提交部分变更。 |
| FT-07 | 执行回执 | 固定计划获得唯一最终状态、版本2、完成时间与原因；幂等重试返回相同结果，不重复审计或事件。 |
| FT-08 | 撤销 | 计划本人明确撤销，保留原清单及最终回执；撤销不移动业务对象。 |

使用：登录 → 文件与资料 → 打开实际文件或非根目录 →“拟定跨范围移动”→ 选择实际目标目录 → 填写名称与整理原因 →“分析跨范围移动”→ 逐项核对完整计划 →“核对并执行跨范围移动”→ 确认原因、声明与MFA → 明确执行。移动端完成后用“打开移动后的资料”进入新地址，避免旧路由中的项目／目录参数指向原位置。Web刷新实际文件及所在目录，并更新当前授权信息。

Usage: sign in, open a file or non-root folder, select **拟定跨范围移动**, choose an actual destination, and analyze. Review every inventory item and visibility change before confirming with a reason and current MFA. The mobile receipt provides **打开移动后的资料** to open the destination using its new scope. Refresh the plan or owner history when a result is uncertain; do not create a duplicate to work around an unknown response.

## 2. 权限与阻塞

既有系统配置权限与业务权限共同生效。操作者须具有组织级`policy.manage`、`user.read`，还须对每个来源项具有读取及`file.move`／`folder.update`，对目标具有读取及创建权限。没有自动授予员工系统管理员角色；数据库升级不修改成员绑定。单纯拥有配置权限不获得业务内容。

仅接受真实Web／微信／抖音／Android人类会话；拒绝运行身份、TG入口及伪造会话渠道。计划只允许创建者读取与决定。历史计划也重新检查旧范围、当前对象范围及所列路径的读取权限；创建者身份不保留已经失去的业务访问权。

| 阻塞码 | 处理方式 |
|---|---|
| `RETENTION_HOLD` | 按原保留制度处理有效锁；移动不会解除锁。 |
| `BUSINESS_REFERENCE` | 先由原OA／项目／知识业务处理正式引用；移动入口不提供通用解除引用功能。 |
| `UPLOAD_ACTIVE` | 处理仍在进行的文件或目录上传。 |
| `RESTORE_ACTIVE` | 等待或按原流程处理历史恢复任务。 |
| `DOWNLOAD_PERMIT_ACTIVE` | 等待已发放的限时下载许可结束后重新拟定。 |
| `CLASSIFICATION_DOWNGRADE` | 目标分类不能低于任何来源项；移动不能用作降密。 |
| `CALLER_TARGET_READ_REQUIRED` | 员工须先按原权限流程获得目标范围读取权限；计划不会自动授权。 |

原业务解除阻塞后必须重新拟定，旧计划不会自动变为可执行。目录树同时包括有效和回收站子项，逐项保留原状态。任何后代无读取／变更权限、归属不一致、目标只读、目录循环、共享名称冲突，均拒绝建立可执行依据。

## 3. 固定依据与事务

| 项目 | 固定或重检的内容 |
|---|---|
| 来源与目标 | 类型、ID、版本、名称、父目录、项目和分类；来源与目标祖先路径。 |
| 全树 | 稳定排序的完整文件／目录清单、各版本与状态，文件当前内容版本ID。 |
| 内容历史 | 正式版本ID、序号、内容摘要、大小、格式、扫描／可用状态、原名；响应不披露对象密钥或桶。 |
| 权限矩阵 | 当前有效员工 × 5个渠道 × 普通／AI场景；完整矩阵含未变组合的摘要，响应列出发生变化的组合。 |
| 授权版本 | 创建时员工与组织授权版本；执行后组织授权版本增加，使旧秘书上下文、许可及游标依据失效。 |
| 身份 | 创建者、实际来源会话／渠道；执行可来自同一员工的另一人类客户端，但原会话仍须有效，执行会话须有近期MFA。 |
| 时限 | 最长30分钟，进一步收窄至组织内下一次角色生效／到期或资源授权到期边界；PG时钟在执行前后重检。 |

建立与决定取得组织独占锁；命名空间及项目按固定顺序锁定，与普通文件写入协调。读取使用一致事务快照并检查当前授权。执行重新分析，固定摘要完全一致才更新，每个对象均按准确版本更新且必须影响一行；完成清单和授权失效与最终回执在同一事务中提交。

`file_scope_transfers`（schema88）保存不可变输入、快照和计划摘要。只允许`pending/version1`进入一次`executed`或`cancelled/version2`。触发器拒绝改写原依据、删除、伪造缺少真实当前人类会话的决定，且执行回执要求完整对象结果、分类约束及组织授权版本增加。业务服务负责当前业务权限和全部依赖重检；表触发器不能替代服务权限检查。

幂等键绑定准确方法、路径和内容。四端检查回执的资源、创建者、摘要、准确版本和最终状态；失配或未知结果显示需要刷新核对，避免继续写入。陈旧读取及读取失败不显示旧计划为可执行。

## 4. 接口与源码

| 方法 | 路径 | 用途 |
|---|---|---|
| POST | `/v1/file-scope-transfers` | 创建固定输入、分析和计划。 |
| GET | `/v1/file-scope-transfers/{id}` | 读取本人计划并核对当前依据；拒绝额外查询参数。 |
| POST | `/v1/file-scope-transfers/{id}/execute` | 提交准确版本、摘要、原因，人工执行。 |
| POST | `/v1/file-scope-transfers/{id}/cancel` | 明确撤销本人计划。 |
| GET | `/v1/files/{id}/scope-transfers` | 本人文件计划历史，固定过滤与签名分页。 |
| GET | `/v1/folders/{id}/scope-transfers` | 本人目录计划历史，固定过滤与签名分页。 |

创建输入：`kind`、`resource_id`、`expected_version`、`target_folder_id`、`expected_target_version`、可选`new_name`、`reason`。决定输入：`expected_version`、`plan_hash`、`reason`。服务器使用当前实际状态；不接受客户端提交的人员清单、权限判断或对象路径作为授权。

| 源码位置 | 内容 |
|---|---|
| `app/backend/internal/files/scope_transfer_*.go`、`scope_transfers.go` | 模型、依赖／权限分析、事务和不可变回执。 |
| `app/backend/internal/httpapi/file_scope_transfers.go` | 严格HTTP入口、幂等命令及分页。 |
| `app/backend/migrations/0088_file_scope_transfers.sql` | 新表、不可变依据和最终决定约束。 |
| `app/packages/contracts/src/scope-transfer.ts` | 四端共享输入和回执完整性检查。 |
| `app/web/src/ScopeTransfers.tsx` | Web实际目标选择、完整计划、本人历史与确认窗口。 |
| `app/mobile/src/pages/files/scope-transfers.tsx` | 微信／抖音／Android共享页面流程；保持既有20个页面路由。 |

## 5. 明确边界

单份计划最多100个文件与目录，分析最多100名当前有效员工；目录路径最多32层、快照最多2MiB，分析／决定最长30秒。超限拒绝，不隐藏多余项。采用既有Unicode规范化的跨文件／目录名称唯一规则。目标分类统一应用于清单，禁止任何项降密。

尚未包含：有正式业务引用文件的跨范围联动改写、任意超限大树的多计划原子移动、AI跨范围整理工具、运行预授权和TG披露流程。上述内容继续按GAP-11／18及相应业务约束处理。实际OSS＋R2仍需GAP-20验收；本功能不触发对象复制或云配置变更。AI费用控制仍排除，公司的OA／财务、授权、审计和技术限制继续保留。

The local checks and recordings establish this bounded human workflow. They do not establish live cloud/model/TG integration, platform approval, company policy adoption or production readiness. All23 work packages and five exclusions retain their current status.
