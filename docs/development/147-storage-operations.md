# 增量147 · 组织文件复制运维 / Organization file replication operations

本轮补集中异常查阅、准确副本核对及固定人工重试记录，继续收口GAP-07／08／18。主线结构111；现有v0.1.0-rc.4仍固定业务145／结构110，APK146不包含147。23项工作包仍active，5项移出和八项draft Skill保持。见[全部余项](../delivery/05-functional-closure-147.md)及[验证](../quality/147-storage-operations-verification.md)。

## 使用 / Usage

管理员在账号管理中按职责授予“文件复制运维查阅人”或“文件复制运维操作人”。查阅人仅发现组织运维元数据；重试操作人仍须拥有该文件的普通更新权限。系统管理员也不会由此取得私人项目的文件内容权限。新安装通过普通bootstrap提供两项角色，既有安装经正常迁移111补齐。

Web从“文件复制运维”进入；微信、抖音和Android共用“我的工作空间→核对副本异常与人工重试”。默认查“复制失败”，可以按OSS／R2、副本状态和当前任务状态过滤，分页各20条。选择准确副本后核对文件／版本编号、当前或历史版本、来源可核验状态和任务进展；只有普通文件查阅授权通过时才显示打开文件入口。

可重试时，核对副本修订与任务、勾选明确确认、填写1–200 Unicode码点原因，并在最近10分钟内完成本人两步验证后保存。保存时重新检查所有权限、组织授权版本、当前副本修订和完整核对依据。页面出现“人工重试已排队”后，仍须刷新当前副本核对复制与校验结果；历史回执固定保留当时状态。

An organization reader can inspect only replica/job metadata. A retry requires both operations permissions, ordinary current `file.update`, a live human Web/WeChat/Douyin/Android session, recent real MFA and explicit review of the exact version/basis. The unchanged normal worker performs copying later. A saved queue receipt is historical evidence; it never establishes successful replication.

## 接口与固定数据

| 方法／路径 | 内容／约束 |
| --- | --- |
| GET `/v1/storage/replicas` | provider／state／job_state精确白名单；绑定当前经办人和筛选条件的游标；limit1–100 |
| GET `/v1/storage/replicas/{id}` | 当前元数据、can_open／can_retry、固定retry_block；不接受额外查询字段 |
| POST `/v1/storage/replicas/{id}/retry` | expected_version、basis_sha256、reviewed=true、reason；标准幂等键／Web CSRF和Origin |
| GET `/v1/storage/replica-retries` | 准确replica_id、绑定当前经办人的不可变历史分页；没有编辑或删除接口 |

组织台账仅返回副本／文件／版本编号、修订、存储端代号、业务及副本状态、核验来源是否可用、当前任务编号／状态／尝试数、时间和固定故障代码。它不返回文件名、内容、桶／对象键、访问端点、凭据、原始云错误、任务payload、会话、审核原因或原件哈希。未知故障统一映射OTHER_STORAGE_FAILURE。独立操作权限：`storage.operations.read`、`storage.operations.retry`。

核对依据SHA-256覆盖完整公开快照，包含任务updated_at、当前文件修订、版本归属、存储epoch／主端及来源可用性；因此即使副本修订未变，任务进展变化也会409拒绝旧核对。分页不构造全局时间点快照，工作器变动后应重新查询。账号或授权变化触发现有共享会话失效／清理机制。

## 实现、并发和审计

后端在同一原生PG事务中锁定原有项目／公司文件命名空间、目标副本、文件版本及当前任务，按原执行器顺序避免引入相反锁序。核对版本／依据后，嵌套调用原`files.Service.RetryReplica`：原有失败／缺失／任务耗尽、不可清理原件、非当前主端目标及可核验源规则继续执行。不会复制第二套工作器或放宽原文件权限。

内部命令键由外部幂等键摘要派生，保存原`replica.retry.requested`审计、普通出站事件和普通业务操作编号；然后一次性插入独立固定台账及`storage.replica.retry.recorded`审计。任一台账／审计／命令保存失败，原普通重试、版本、事件和两个幂等键全部回滚。回放前仍重查当前本人、近期MFA、运维和文件权限；合法回放保留原历史回执，不用后来工作器状态改写。

迁移111通过严格快照字段／类型白名单、原普通命令和审计摘要关联、精确当前副本及本人会话／权限绑定，约束插入；UPDATE／DELETE即使由测试所有者尝试也被不可变触发器拒绝。保存原因仅进入私有台账／既有授权审计，不进入组织元数据历史响应。

共享TypeScript构造函数验证明确核对、目标／修订／依据及原因。新响应和JSONB回放均校验准确before、排队after、普通操作编号和目标版本；键顺序不影响相等。失败或不确定结果阻止继续提交，用户重新核对详情和历史。Web和Taro使用现有品牌组件／交互，不新增移动注册页。

## 部署和边界

按原生升级手册正常应用结构1–111；当前保留开发PG100并未升级或重启，需应用101–111后才能运行新主线。迁移1–110、旧四次冻结和既有证明／媒体原字节保留。实际复制继续使用原OSS＋R2后端和工作器配置；本轮没有真实云写入或生产部署。

Fresh focused checks cover native HTTP/PG, four authenticated channel types, shared Web/mobile logic, Web and both miniapp builds, plus one real Web journey with an isolated synthetic object store. Full regression, additional mobile UI and a new APK are deferred under the accelerated delivery request. The existing rc.4 downloads remain the fixed ready-to-review delivery; this source increment is not silently added to them. Formal Linux/cloud/model/TG/platform/Android and company-rule acceptance remain open.

[失败副本](../media/screenshots/storage-operations-web-failure.png) · [核对窗口](../media/screenshots/storage-operations-web-review.png) · [排队回执](../media/screenshots/storage-operations-web-queued.png) · [真实无声MP4](../media/videos/storage-operations-web.mp4) · [三截图导览GIF](../media/gifs/storage-operations-web.gif)
