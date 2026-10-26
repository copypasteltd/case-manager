# 事件、后台任务、幂等与并发协议

版本：v1.0-draft；日期：2026-10-23。

## 1. 事务提交约定

每个正式命令：认证/授权 → 获取或登记幂等记录 → 锁定/版本检查聚合 → 校验不变量 → 写业务+audit+outbox+结果 → 提交。记录 in_progress 与实际业务提交必须有可恢复关联；进程在中间崩溃后通过 operation/业务唯一键确认，不永久卡死也不重复执行。

安全拒绝/外部网络失败可记录独立审计，不能通过回滚业务事务同时抹掉拒绝日志。对外通知、模型请求、云对象操作不包含在该事务里。

## 2. 事件 envelope

```json
{
  "event_id":"60523d0b-8aa9-4394-8c97-1016d821e54d",
  "event_type":"file.version.committed",
  "schema_version":1,
  "org_id":"c46829ca-16c4-4a52-a50e-dfa216e6fe69",
  "aggregate":{"type":"file","id":"ad6e8563-9086-4db6-8a4e-6be474935d07","version":4},
  "occurred_at":"2026-10-23T03:00:00Z",
  "correlation":{"request_id":"req-example","run_id":null,"causation_id":null},
  "payload":{"file_version_id":"7b50d68b-76d7-4080-b088-eb99f18583f6","storage_epoch":1}
}
```

payload 只包含消费必需 ID/安全字段，不放合同全文/密钥/带签名URL。消费者通过自身授权身份重新取业务事实。event_id 全局唯一，aggregate_version 用于识别迟到事件，但不能假设不同聚合之间全局有序。

## 3. 首发事件目录

| 事件 | 最小 payload | 消费者/幂等效果 |
| --- | --- | --- |
| identity.authorization.changed | subject_id, auth_version, scope_ref | 缓存/上下文失效；即时请求仍查版本，不能仅依赖投递 |
| project.created | project_id, template_revision | 模板文件 job；项目/模板/步骤唯一 |
| task.assigned / task.transitioned | task_id, version, assignee_ref | 站内提醒；过期分配可抑制 |
| post.published | post_id, revision_id | 检索索引；按revision生成 |
| file.version.committed | file_version_id, storage_epoch | replica/解析/索引，版本+类型+生成器唯一 |
| file.trashed / restored | file_id, version | 刷新索引；不即时硬删云对象 |
| request.submitted | request_id, revision_id, instance_id | 当前审批待办；实例/步骤唯一 |
| approval.step.decided | instance_id, step_id, decision | 后续步骤、申请结果、通知；流程推进本身在受控事务内 |
| cost.confirmed / reversed | cost_id | 月报重建；不再创建相同费用 |
| subscription.period.generated | subscription_id, period_id | 预测/提醒；账期唯一 |
| replica.failed / verified | replica_id, version_id | 运维告警/归档条件；不反向改文件内容 |
| backup.completed / failed | catalog_id, recovery_point | 运维通知；仅完成校验的恢复点可用 |
| service.state.changed | service_id, observation_id, state | incident关联和去重告警 |
| agent.run.completed / failed | run_id, result_ref | 安全结果通知；发送前重鉴权 |

## 4. Job 协议

job 状态 `queued → running → succeeded`，失败可 `retry_wait → running` 或 dead；取消 cancelled。领取使用短事务 `FOR UPDATE SKIP LOCKED` 等受控机制，写 lease_owner、lease_until 与递增 fencing_token 后提交，再执行外部 I/O。

heartbeat 候选30s、lease120s；每次状态/分片提交都比较 fencing_token。旧 Worker 仍能发出的已签云请求无法单靠 DB 锁物理撤回，故使用不可变源、独立目标 multipart/唯一 key、条件提交，并在切主时隔离旧凭证/网络，不能仅靠“租约过期”声称绝无外部副作用。

重试采用有上限指数退避+抖动，记录 attempt、error_code、first_failed_at、next_retry_at。403/固定源丢失/参数不合法进入终态；429/短时网络可重试。dead 进入人工队列；“重试”沿用业务幂等键，可新 job ID 但不得生成新经济事项/文件版本。

小文件复制、大文件复制、解析、通知、报表、备份、AI 调度各自队列/并发上限。任务 payload 版本化；不把任意 shell 命令作为 job payload。定时任务使用确定周期键，例如 `subscription:{id}:{period}`，重启补跑按业务日期对账不重复。

## 5. 并发与锁规则

| 场景 | 保护方式 | 不允许的结果 |
| --- | --- | --- |
| 同时更新项目/文件 | expected_version / base_version 条件更新 | 覆盖他人新版 |
| 双设备审批 | step 行锁+终态唯一动作 | 同步骤既通过又退回 |
| 付款/费用多方分配 | 按 ID 稳定顺序锁余额及分配记录 | 累计超过有效金额 |
| 同项目加依赖 | project 图锁/串行化+环检测 | 两个单独无环检查共同成环 |
| 目录移动/同名 | 范围锁+规范化名唯一+环检测 | 环或重复名称 |
| 生成账期 | 周期自然键 unique | 同次重启重复预测/费用 |
| 当前版本切换 | file 行锁+版本序号 unique | 两个 current、序号重复 |
| 幂等冲突 | actor+route+key unique+request_hash | 同键换金额/目标仍成功 |

DB serialization/deadlock 允许有限内部重试，但必须保留同幂等键；非幂等外部请求不放在可能重跑的事务闭包中。

## 6. 消费与投递一致性

Outbox relay 可能在发出后、标记前崩溃而重复投递。consumer 使用 `(consumer,event_id)` 唯一、与自身业务效果同事务提交。只有 DB 内效果能这样保证；TG/邮件超时应标记 delivery_unknown 并查询/受控重发，不能声称全链路恰好一次。

事件积压年龄、可运行任务数、租约过期率、dead 数、重复键、重试流量、单任务字节/费用必须有指标。日志/指标不能泄露 payload 敏感正文。

## 7. 当前代码实现与验证边界

`app/backend/internal/jobqueue` 已实现 PostgreSQL 持久化队列与 Outbox 消费基础。领取只处理指定种类、车道及到期任务；每次领取或接管递增 attempt 和 fencing_token。租约范围15—300秒，续租、失败、完成都核对当前 owner/token 和未过期租约。耗尽次数后崩溃的任务由 sweep 转 dead，避免永久滞留 running。

业务提交使用 CheckInTx/CompleteInTx：先按组织、项目和业务资源的稳定顺序锁定，再锁任务并核对租约，同事务保存业务结果与任务完成。CompleteInTx 后事务回滚不会留下成功任务。该保证只覆盖数据库；云端正在执行的请求仍需唯一候选 Key、源条件和纪元隔离。

Enqueue 必须接收调用方事务，组织与业务去重键相同但种类、车道、payload版本、内容或重试上限不同会拒绝。数据库触发器禁止事后修改这些执行输入；失败记录仅接受受控错误码。重试时长由调用方明确传入，尚未接入统一指数退避调度器。

DispatchOne 按消费者名称和 event_id 记录独立投递，纯 Build 函数只能将版本化事件映射为受控任务输入，不能执行云请求或业务写入。入队与 processed_events 同事务；不使用全局 published_at 取代各消费者进度。未知事件版本、输入无效、去重输入矛盾会生成 `event.rejected.v1` dead 记录，并保存该拒绝记录的引用；这代表已进入可检查的拒绝队列，不代表业务成功，也不会挡住后续有效事件。

原生数据库验证覆盖并发领取唯一胜者、过期接管及旧 token 拒绝、事务回滚、车道隔离、重试与耗尽回收、Outbox 回滚、独立消费者、未知版本拒绝。上传控制已接入独立常驻执行器、受控handler、心跳和退避；其他业务执行器、人工重试页面及监控告警尚未实现。测试不构成真实云传输、通知送达或完整任务系统的验收。
