# 秘书执行无需AI费用额度 / Secretary execution without AI fee quotas

日期：2026-10-28。对应用户范围修订99、GAP-09的费用前置兼容子项。当前范围以[范围清单](../delivery/04-current-scope-and-acceptance.md)为准；本增量不完成真实Codex、执行器隔离、Skill发布、TG或生产接入验收。

## 1. 当前行为

员工按原流程登录，进入“公司秘书”，选择自己有权访问的会话和已安装的运行配置，提交请求。新请求不需要先发布AI月度Token策略，也不会因旧策略为0、历史预留或已记用量耗尽而被费用控制拒绝。这个规则由公共接单和执行路径实施，适用于四端及共享渠道接单代码。

技术性上下文／输出长度、工具调用次数、超时、执行租约、同会话唯一活动运行、每人／组织并发上限继续保留。身份、项目与资料权限、会话成员、资料分类、取消、授权变化和执行隔离仍影响接单或执行。公司的合同、采购、报销、订阅费用和项目预算继续按原业务规则工作。

真实模型、执行器和运行配置仍需单独安装／授权／配置。解除费用前置不会自动启用模型，也不会把8个draft Skill改成正式发布状态。

## 2. 实现范围

| 编号 | 源码 | 行为 |
| --- | --- | --- |
| CF-01 | `app/backend/internal/agent/admission.go` | 保留容量锁、事件和一次执行任务；移除费用预留调用 |
| CF-02 | `app/backend/internal/agent/execution.go` | 移除费用预留存在性硬门，保留身份、权限、租约、取消、期限和执行隔离核验 |
| CF-03 | `app/backend/internal/agent/execution_usage.go`、`execution_results.go` | 记录实际技术用量，验证非负、缓存包含关系、合理数值边界、幂等和冲突；结果仍依赖真实工具回执和来源 |
| CF-04 | `app/backend/migrations/0079_agent_execution_without_cost_quota.sql` | 技术用量外键直接指向准确租户／运行；原78份迁移和全部旧表保留 |
| CF-05 | `budget.go`、`budget_reports.go`、OpenAPI | 停用旧费用策略发布／核对写入，历史查询保留授权、范围与分页游标约束 |
| CF-06 | Web `AgentBudget.tsx`、Taro `pages/budget/index.tsx` | “秘书用量历史”只读页面；按本人、组织、员工、项目或Skill查询原记录，明确返回公司秘书 |

```mermaid
flowchart LR
    A[已登录员工／授权渠道] --> B[业务权限与并发检查]
    B --> C[运行记录与任务入队]
    C --> D[租约／配置／隔离与权限复核]
    D --> E[模型与受控工具]
    E --> F[验证来源与业务结果]
    F --> G[回复与技术用量追加记录]
```

实际模型与隔离环境仍属于开放验收项；本图说明代码流程。

## 3. 旧接口与历史数据

| 接口 | 当前结果 |
| --- | --- |
| `GET /v1/agent-budget` | 按原权限读取历史汇总及旧策略；不作为当前接单额度 |
| `GET /v1/agent-budget/entries` | 按原范围和绑定游标读取旧预留记录 |
| `POST /v1/agent-budget/policies` | 有权交互管理员返回 `410 AI_COST_CONTROL_RETIRED`，不新增策略 |
| `POST /v1/agent-budget/entries/{id}/reconcile` | 有权交互管理员返回同一410，不新增费用核对记录 |

未经身份验证或缺少管理员权限时继续执行原认证／授权拒绝。停写检查位于服务层，旧幂等键不能重新启用费用写入。OpenAPI把两个旧写入操作标记为deprecated并移除成功响应。

迁移79撤销应用角色对旧策略、预留、费用核对表的INSERT权限。保留原不可更新／删除触发器和租户绑定。技术用量表沿用 `agent_budget_usage` 这个历史存储名称，但不再引用费用预留；改为 `(org_id,run_id)` 外键直接引用 `agent_runs`。

原预留、未知用量、费用核对和策略历史不补造、不清零、不删除。新运行不新增费用预留，因此旧账本只展示旧预留记录；“未建立费用预留的运行”计数包含当前新运行，不能解释成升级前数量或模型费用为0。

单次运行的输入／输出和工具次数错误码保留原协议名称，例如 `RUNTIME_USAGE_BUDGET_EXCEEDED`、`RUNTIME_TOOL_BUDGET_EXCEEDED`；这里仍表示技术执行边界，不是AI月度费用配额。实际SDK技术用量不等同于供应商账单。

## 4. 正常结构升级与使用

本次是应用结构版本78→79的正常升级，属于当前交付范围，不涉及已取消的2 TB历史资料导入或容灾工程。生产执行前仍需完成GAP-24／25的部署与持久化配置。

1. 停止旧版本API和相关执行工作进程接新任务，保留原数据库及已排队／进行中的运行记录。79会撤销旧费用表INSERT，旧版本API不适合与新结构混用。
2. 使用迁移角色执行与当前源码匹配的迁移命令。迁移1～78必须保持原校验和，79不删除业务表或历史记录。
3. 部署匹配的当前API／执行器代码并启动服务，核对结构清单、应用角色权限和真实运行配置。不要把旧的必配费用版本回滚到79结构上；应用版本回滚须选结构兼容的版本。
4. 从公司秘书发起授权请求；无AI费用策略、旧策略为0或旧账本已耗尽均不构成费用拒绝条件。仍可能因身份、权限、未安装运行配置、并发、取消或隔离条件被拒绝。
5. “秘书用量历史”仅供查看旧记录，不能编辑费用策略或提交费用核对。Web、微信、抖音与Android共享相应界面行为。

原生本地工具使用现有忽略文件中的开发配置：

```powershell
$env:PYTHONUTF8='1'
python app/scripts/dev_database.py start
python app/scripts/run_backend.py migrate
python app/scripts/run_backend.py api
```

已有日常开发服务需要重载时，可运行 `python app/scripts/update_cost_free_execution_development.py`。此脚本只处理本仓库原生回环地址、检查进程身份、保留数据库；不会启用模型、TG或云存储。检查结束的浏览器／临时审阅服务必须关闭，日常开发服务与用户原有进程保留。

完整安装、配置、四端构建和业务入口见[中文指南](../README.zh-CN.md)与[English guide](../README.en.md)。不得为本项目启动Docker、WSL或模拟器。

## 5. 开发与验证约束

- 通用测试安装不再自动插入虚构的AI费用额度，让其他集成用例也覆盖真实“无费用策略”默认状态。
- `TestAgentCostFreeExecution`覆盖12个集成场景，包括无策略、旧0额度、旧超额、真实读工具／结果回执、幂等、容量、取消、用量冲突／不可改写、跨租户错误绑定、旧接口停写与只读分页授权。
- 当前后端须全部执行；若仅清理隔离库超时，保留首轮失败及日志摘要，在同一源码摘要上仅复跑失败用例，不能把失败标为通过。
- UI改动须检查Web与共享Taro、两小程序构建及Android包；JVM缓存复用与新执行必须分别说明。截图应等待真实记录呈现，加载态素材不能充当已加载页面。
- [验证记录](../quality/100-secretary-execution-verification.md)记录源码摘要、构建、媒体、原证据复用、失败重试及开放验收；不以本地合成检查替代真实Codex／TG／云或生产验收。

## English implementation notes

New secretary admission and execution do not read or reserve monthly AI fee policies. Technical usage is append-only and references the exact tenant/run directly after schema79. Concurrency, context/output/tool limits, deadlines, cancellation, identities, authorization, tool receipts and confinement remain enforced.

Legacy fee policy/reconciliation writes return410 to authorized interactive managers; the application role also loses INSERT on those retired cost tables. Existing policies, reservations, reconciliations and unknown usage remain immutable and readable with scoped access. The archive is not a provider invoice and does not establish zero provider cost. Company finance and project budgets remain active.

Stop incompatible old services before applying79, preserve existing storage and queued records, then deploy matching code and verify its schema manifest/role permissions. This is a normal schema upgrade, not historical data import or disaster recovery. Real Codex, eight published Skills, confined execution, TG, cloud storage and production acceptance remain unfinished work packages.
