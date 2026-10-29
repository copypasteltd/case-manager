# 秘书准确草稿恢复 / Exact secretary file draft recovery

增量112补齐GAP-04／12中的跨运行草稿恢复子流程。员工明确选择已保存的草稿，在新的秘书任务中继续处理；服务端固定原目标、基础版本、准确内容摘要及依赖资料，重新上传、校验和扫描，再沿用现有正式提交流程。文件更新和新文件创建均适用。更广的文件格式、全文差异、Office／PDF／OCR、完整OA附件与真实模型验收继续开放。

Increment 112 adds human-selected recovery of an exact saved file draft into a new secretary run. Both updates and new-file drafts retain their original scope and content hash. A new run creates fresh storage and scanning evidence before the existing formal commit workflow. This advances GAP-04/12 without completing either package. Only AI fee control is excluded; authorization, audit, execution isolation and technical resource limits remain required.

## 使用 / Usage

1. 打开本人创建的私密秘书会话，选择“新建或更新文件”任务配置。原请求须已有准确草稿，且运行已明确结束；处理中、待审批、正在提交或结果未定的请求不能恢复。
2. 点击“选择旧草稿继续处理”，从当前会话实际消息中选择原文件请求。可以继续读取更早消息；页面重新读取原任务及其当前可见的实际回执。
3. 核对文件目标、基础版本、准确草稿、差异、大小和SHA-256。选择“本次恢复的准确草稿”，勾选核对声明，再点击“将准确草稿恢复加入请求”。选择或勾选本身不会发送请求。
4. 填写继续处理说明，点击“确认发送本次请求”。新任务回执显示原任务、准确草稿、内容摘要、恢复摘要和来源层数。若页面或请求结果不确定，先重新读取实际任务，核对是否已接单。
5. 实际执行器通过专用工具重建相同内容，重新校验与扫描。正式文件只有在新计划通过当前授权并成功提交后才改变。接单、生成草稿、扫描完成、等待审批和正式提交分别显示实际状态。

Open your own private conversation, choose the file-update profile, select an earlier file request and review its exact draft. Confirm the target, base, diff and digest, add that recovery to the request, then explicitly send a new message. The new receipt identifies the selected source. A queued receipt is not a file commit. Execution must produce fresh upload, integrity and scan evidence and complete the existing authorized plan/commit path.

原运行、原上传、原草稿、原计划与既有回执保持历史事实。新文件恢复仍指向原目录、名称、格式和分类；不会通过覆盖同名文件解决冲突。更新恢复仍依赖准确基础版本；文件或目录已变更时，员工须核对现行内容并发起新的编辑请求。

## 固定规则 / Invariants

| 编号 | 规则 | 实现要求 |
|---|---|---|
| FR-01 | 明确选择 | 请求固定原运行ID、草稿ID、内容SHA-256及`reviewed=true`；不接受“最近草稿”等可变引用。 |
| FR-02 | 身份与范围 | 当前人类交互会话、同一原发送人、本人私密会话、单人成员、相同会话修订及安全版本；服务端重查当前资料与业务授权。 |
| FR-03 | 原任务明确结束 | 允许`succeeded`、`failed`或`cancelled`且没有正式文件提交；`interrupted`结果未定，不能作为恢复来源。 |
| FR-04 | 准确内容 | 从不可变编辑与完整验证的原基础文件重建，或从不可变新文件正文重建；重建后的字节摘要必须等于员工选择值。BOM、CRLF及Unicode字节保留。 |
| FR-05 | 新存储与扫描 | 不沿用旧对象、旧扫描、旧计划或旧提交凭证；创建新上传，校验新对象并重新扫描，然后生成新计划及提交回执。 |
| FR-06 | 状态变化 | 原上传过期、失败或取消可以保留草稿编辑价值；已正式提交、被阻断或隔离的草稿不能借恢复绕过扫描。放行豁免同样不作为恢复依据。 |
| FR-07 | 依赖变化 | 原消息选定资料及原工具读取的依赖按准确快照合并；权限、分类、版本、正文摘要或依赖变化时停止。普通草稿预览也核对这些依赖。 |
| FR-08 | 执行中撤权 | 每个执行阶段、重放、提案与提交核对固定来源和当前授权；新运行固定的人类会话被撤销、原草稿后来隔离或上传中取消时，不产生新暂存提交，尝试中止新分片。 |
| FR-09 | 来源链 | 二次恢复保留前一准确草稿并展开外部依赖，至多8层，属于技术资源限制。达到上限后须人工核对现行文件并创建新的编辑请求。 |
| FR-10 | 回执原子性 | 不可变恢复依据、准确新草稿、专用工具证据与正式文件回执受数据库约束；业务事务与审计／工具回执原子提交。 |

Historical reading requires current business authority; it does not require the original human session to remain live. AI execution separately requires the human session pinned by the new recovery admission to remain valid. Restoring an expired upload therefore does not revive an old execution lease or identity session.

## API与工具 / API and tool contract

沿用`POST /v1/conversations/{id}/runs`、标准JSON信封、CSRF及`Idempotency-Key`。`file_recovery`与`file_update`、`file_create`、其他业务变更／OA／博客意图互斥；任务配置须属于`file-update` Skill。

```json
{
  "file_recovery": {
    "source_run_id": "原运行UUID",
    "draft_id": "准确草稿UUID",
    "content_sha256": "准确内容的64位小写十六进制摘要",
    "reviewed": true
  }
}
```

此处仅展示新增字段；实际命令仍须提供原有的消息、客户端消息ID、任务配置等字段。202只说明新任务已接单。服务端在`file_update.recovery`或`file_create.recovery`返回固定来源，包含`recovery_sha256`和`depth`；客户端核对新旧运行不同、准确来源、目标、基础版本和内容摘要。

专用MCP工具`recover_file_draft`只接收`run_id`及`recovery_sha256`，不允许附带正文、编辑、目标或旧上传／计划ID。服务端读取固定编辑，重建原字节。恢复任务不能改用普通`create_file_draft`或`stage_file_create`更改正文。后续沿用已有检查、提案、审批和正式提交工具。该工具不属于Skill完成凭据；只有真实正式提交才允许声明文件操作完成。

| 错误 | 含义与处理 |
|---|---|
| `FILE_RECOVERY_RUN_UNSETTLED` | 原运行结果未定，先查询实际状态。 |
| `FILE_RECOVERY_CONTEXT_CHANGED` | 会话修订／安全范围变化，重新核对资料。 |
| `FILE_RECOVERY_UNAVAILABLE` | 没有准确草稿、摘要不符、已提交或扫描阻断。 |
| `FILE_RECOVERY_SOURCE_CHANGED` | 原文件、目录或固定来源已变化，重新创建编辑请求。 |
| `FILE_RECOVERY_DEPTH_LIMIT` | 已达8层技术上限，人工核对现行内容后重新发起编辑。 |
| `FILE_RECOVERY_SCOPE_REQUIRED` | 工具未匹配本次固定来源或恢复摘要。 |
| `FILE_RECOVERY_EXACT_BYTES_REQUIRED` | 恢复运行试图改用普通编辑工具。 |
| `RUNTIME_SOURCE_CHANGED` | 资料或授权在执行中变化；停止并核对实际结果。 |

## 持久化与开发 / Persistence and implementation

迁移87新增`agent_file_recoveries`，应用账号仅有SELECT／INSERT；新任务关联同组织的准确源草稿，固定规范化恢复正文、摘要、明确核对标记和层数。数据库禁止更改或删除来源，核对同人同会话、原任务终态、准确意图及新草稿内容，并以延迟约束要求对应意图及专用暂存工具证据。原历史记录不改写。

Migration87 is an ordinary application schema upgrade, not historical data migration or disaster recovery. It adds immutable provenance and deferred evidence guards. Existing AI fee history remains read-only and is not part of admission or execution policy.

| 层次 | 主要代码 |
|---|---|
| 固定来源与重新授权 | `app/backend/internal/agent/file_recovery.go`、`file_updates.go` |
| 准确内容和新上传 | `file_update_tools.go`、`file_creates.go`、`execution_tools.go` |
| MCP与隔离工具声明 | `app/backend/internal/agentworker/mcp.go`、`bridge.go` |
| 不可变数据库约束 | `app/backend/migrations/0087_agent_file_recovery.sql` |
| 跨端输入／回执 | `app/packages/contracts/src/file-recovery.ts`及共享边界算例 |
| Web | `app/web/src/ConversationFileRecovery.tsx`及会话／文件实际回执 |
| 微信／抖音／Android | `app/mobile/src/pages/secretary/file-recovery.tsx`及共享秘书页 |
| Skill | `app/skills/file-update`版本1.3.0；仍为draft；操作约束见`docs/skills/file-update/SKILL.md` |

```powershell
python app/scripts/file_recovery_native_cluster.py prepare
python app/scripts/verify_file_recovery_backend.py
python app/scripts/verify_file_recovery_clients.py
python app/scripts/run_file_recovery_browser.py
python app/scripts/run_file_recovery_browser.py --mobile
python app/scripts/file_recovery_native_cluster.py stop
```

检查使用原生工具与隔离合成数据，不使用本地Docker、WSL、虚拟机或模拟器。首次运行前按仓库原生依赖指南准备工具；已有证据目录需先检查及归档，脚本不会把旧结果自动覆盖成通过。浏览器、临时服务及测试数据库均有明确归属和结束清理。

本轮记录的进程中断可使用`verify_file_recovery_backend.py --resume`继续：仅复用准确源码、完整测试集合和原始日志一致的已完成批次；整个未完成批次重新执行。该入口核对本轮明确保存的中断记录，不是忽略失败或跨版本复用。原生PG恢复与开发服务恢复分别使用`resume_file_recovery_processes.py`和`restore_file_recovery_services.py`，只对核对过的本轮资源适用；不会重新初始化集群或重复结构升级。

测试、实际媒体与证据范围见[本轮验证](../quality/112-secretary-file-recovery-verification.md)。当前总计仍为23项待完成（14 P0／9 P1）、5项移出。真实Codex、八项Skill正式评测、目标Linux隔离、OSS／R2及TG接入、正式多端发布与生产验收继续按[稳定编号](../delivery/03-goal-gap-assessment.md)推进。
