# 固定清单批量文件整理 / Human reviewed batch files · Increment127

## 业务结果

文件中心可一次选择1至50个文件，执行批量移动、移入回收站或从回收站恢复。Web及共享微信、抖音、Android页面均接入实际接口。操作分为固定清单分析、本人核对、一次执行与持久回执；分析不改变文件。

移动限相同项目及分类，不改名、不改正式内容头。跨项目或分类继续使用增量115的完整权限变化计划。回收站操作保留对象与全部正式版本；恢复是撤销回收，不是把历史内容指针倒退。历史内容恢复继续使用现有异步恢复流程。

## 使用流程

1. 在文件中心或具体目录勾选文件，或“选择已加载文件”；上限50，范围和状态筛选切换后重新选择。
2. 选择批量操作。移动时逐级浏览并选择实际目标目录；填写1至200字符的原因。
3. 点击“分析批量清单”。服务固定每个文件的ID、修订、内容头、项目、分类、名称、原目录、完整祖先路径、保留锁和阻塞原因；移动另固定准确目标及祖先路径。
4. 核对完整清单、目标、有效期、阻塞及摘要。存在名称冲突、回收状态不符、保留锁或只读项目时显示阻塞，不能执行。
5. 点击“核对并执行整批操作”，重新填写确认原因、勾选本人确认并完成近期MFA，再明确提交。整个事务成功后逐项显示结果修订与状态。
6. 从任一相关文件的“本人的批量整理记录”重新打开，包括回收站文件。未执行计划可由创建人明确撤销；撤销不写文件。

执行要求员工已经按公司角色制度启用MFA。它沿用原文件／目录操作授权，不隐式赋予管理员角色。测试夹具只把合成项目经理角色设置为要求MFA，没有更改生产默认角色。

Web成功后刷新列表，同时保留当前结果窗口。共享移动页面保留完整回执，关闭批量工作区后刷新列表。移动页面隐藏即清除临时选择；MFA、勾选、文件选择不写草稿或持久存储。结果不明时操作被阻止，先从原文件记录重新核对。

## 接口契约

| 方法 | 路径（位于/v1） | 结果 |
|---|---|---|
| POST | `/file-batch-plans` | 固定且不可编辑的待核对计划，201 |
| GET | `/file-batch-plans/{id}` | 本人计划、当前依据一致性及可执行状态 |
| GET | `/files/{id}/batch-plans?limit=10&cursor=…` | 与该文件相关的本人计划；签名游标分页 |
| POST | `/file-batch-plans/{id}/execute` | 本人、近期MFA、明确确认后执行整批 |
| POST | `/file-batch-plans/{id}/cancel` | 本人明确撤销固定计划 |

创建：`action=move|trash|restore`、`files=[{id,expected_version,current_version_id}]`、仅移动时非空的`target={id,expected_version}`、`reason`。文件按ID排序固定，重复、缺失、超限及隐藏目标拒绝。

决定：`expected_version`、`plan_hash`、`reason`、`acknowledged=true`。所有写请求保留现有幂等键、固定输入及当前权限检查。计划ID和原始选择不能由客户端替换为任意批处理查询。未知或重复查询参数拒绝。

## 事务、权限及并发

- 创建、执行采用组织排他锁，和普通命令持有的组织共享锁协调。一次计划最多50个文件，服务30秒时限；文件及项目锁仍由普通命令取得。
- 每个来源需要当前`file.read`及对应`file.move`、`file.trash`或`file.restore`；原目录、完整祖先及目标需可读，移动目标需`folder.create`。不增加宽泛批量授权。
- 只允许真实且未撤销的Web、微信、抖音、Android人类会话；agent run、Telegram或伪造会话／渠道不被接受。执行另外复核实际会话近期MFA。
- 所有读取需创建人身份及原固定范围、当前文件与目录的可见性。历史页中任一计划不可见则该页失败，不返回部分隐藏来源资料。
- 计划有效期至多30分钟，并截断在下一个已知授权生效／失效时间边界。执行复核员工和组织授权版本、所有固定元数据、目录路径、保留锁、状态及共享文件／目录名称空间。
- 执行在一个PG事务中组合现有单文件移动、回收或恢复命令。派生幂等键隔离每个子命令；它们原有的审计及Outbox保持。一项失败时，前面文件、成功审计、Outbox、幂等成功记录及整批完成记录全部回滚；失败请求的拒绝审计仍保留。
- 相同来源的并发计划只允许一个基于旧修订的操作成功；重复准确请求返回原操作及当前计划。重放前仍核对当前权限、可见性和执行MFA。

## 数据及恢复

正常结构升级0095新增`file_batch_plans`，保留原94份迁移字节。固定输入和快照保存为UTF-8文本，`sha256(input + LF + snapshot)`绑定准确字节；总量512KiB。来源逐项对应真实文件元数据。

状态仅允许`pending/v1 → executed/v2`或`cancelled/v2`。SQL触发器保护固定字段、真实人类原始／决定会话、单次决定和每个实际单文件修订结果，禁止删除完成证据。回执固定原正式头、项目、分类、名称、来源及目标结果；不以当前列表代替历史回执。

本轮本地正常升级94→95，复核现有财务／OA／文件、正式版本、目录和共享名称空间行数与内容摘要；不执行历史业务导入或容灾项目。

## 当前边界

本轮关闭了四端批量移动／回收／恢复这一子功能，GAP-18仍保留完整四端正常、失败、冲突及上传旅程验收。真实云存储、正式平台、Linux／模型、生产数据库与发布产物保持各自未完成项。新Android检查为JS编译及准确模块来源核对，旧121开发APK保留原证据，不是当前127发布包。

验证与素材见[实际检查](../quality/127-file-batch-verification.md)，完整余项见[23项清单](../delivery/05-functional-closure-127.md)。

## English operation and implementation notes

Select 1–50 explicit files, choose move/trash/untrash, review an immutable persisted inventory, then acknowledge and execute with the creator's current human session and recent MFA. Same-scope moves retain names and formal heads. Existing cross-scope transfers and historical-content restoration keep their separate workflows.

One transaction composes ordinary commands, preserves their checks, idempotency, audit and Outbox, and rolls back all successful child facts on any failure. Owner-bound history can reopen the receipt from every related file. Fixed source/target/path/retention metadata, actor/organization authority and expiry are checked again before execution; a blocked or stale inventory must be proposed again. No privileged batch role, model control, object deletion or cloud shortcut is introduced.

Migration95 adds a guarded plan/receipt table without rewriting original migrations or files. Local Web/H5 journeys, shared-client builds and native JS compilation have their stated scope; company policy, real cloud/model/platform/production acceptance and all 23 remaining packages remain explicit.
