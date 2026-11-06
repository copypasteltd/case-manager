# GAP-05 — 普通文件生命周期与已知对象清理

> “文件阅读、下载、预览、历史版本、恢复和回收站；恢复旧内容生成新版本，不删除后续历史” — `docs/requirements/06-files-and-search.md:19`（FILE-05）。
>
> “普通文件删除进入回收站，双云不跟随立即硬删；保留/签署引用/法务保留/有效备份引用检查后才可能清理” — 同文`:21`（FILE-07）；清理条件见`:64`（FILE-14）。当前修订99移出的历史迁移／容灾不重新作为门槛。

**Verdict:** implemented on the traced ordinary/known-object paths; complete formal acceptance remains undecidable. Confidence: high for named paths, not an exhaustive storage/security audit. Base `930cdde`；仅源码核对，未运行测试、构建、浏览器或服务。

**What this demands:** 可追溯的同范围整理、回收／原位恢复、准确历史内容恢复为新正式版本；已知清理必须核对当前权限、制度、引用／保留、观察期和准确对象来源，先设永久屏障，逐次保存真实结果。未知外部键身份不能据猜测取得删除权。

**Where enforcement lives / Paths walked:**

- `httpapi/files.go:68` → `files/files.go:149` 的 `MoveFile` 重查源／目标权、同范围及版本，仅更新目录／名称元数据；`:216–305` 的 Trash/Restore/changeState 变更业务状态并保留版本；历史恢复模式明确导向异步任务，不倒退指针。
- `history_restores.go:226` 建立固定 source/base；`history_restore_jobs.go:125` 重核主体、目标、精确来源与存储纪元；`history_restore_commit.go:14–160` 只有 ready 候选、固定基础版、完整候选SHA与来源相符才插入新 `file_versions`、写 `restored_from_version_id`、推进 head 和新副本任务。旧版本没有删除分支。
- `retention_assessment.go:124–208` 汇总当前 head、法务／业务保留、固定引用、恢复／上传／许可、派生对象、双副本证据及最新已发布制度；报告只是盘点，执行仍重查。
- `object_retirement_commands.go:118–205` 建立固定清单／观察窗口、批准时重新盘点并写永久键屏障；`retention.go:60–63` 保留／观察均至少86400秒；`object_retirement.go:153–174` 复核当前本人会话、MFA和普通保留管理权。
- `object_retirement_worker.go:70–116,180–225` 在真实删除前再次重核批准／续作人的当前会话及完整清单，调用 `RemoveFrozen`；`:118–177` 原子写回执、审计、事件，只有所有项目确认不存在才完成。`objectstore/original_cleanup.go:14–26,52–128` 不将普通404视为不存在；OSS须准确原生版本，R2沿用受管独占命名空间和永久屏障，不宣称 DeleteObject CAS。迁移0099`:87–124` 使屏障和回执不可改／删。
- 已知未完成分片由 `multipart_cleanup.go:119–197` 限定本人当前文件／目录范围、真实上传身份、制度、过期许可，排除已完成／正式内容；已完成未提交对象由 `completed_upload_retirement.go:79–244,458` 盘点准确原上传与全部已知晋升，并保护活跃任务／秘书恢复和不确定来源，沿用同一固定清单执行器。
- `derived_retirement.go:86–158,222–332` 仅在历史原件已完整清理后，观察并人工复核准确数据库派生正文；外部派生对象数仍单列，操作只清除已知正文，不制造外部删除回执。
- Web入口 `web/src/Files.tsx:60,123–125` 和共享移动入口 `mobile/src/pages/files/index.tsx:47,54,96,111` 接通上述清理／历史恢复；`retirement.tsx:23–27` 明确区分文件与尚无正式文件身份的目录清单。`deploy/release/catalog.py:11` 与 `cmd/platform/main.go:68` 已接入167受控 original-cleanup，未把安装代码当作目标机已启动。

**Searched:** 在 `files/` 查询 Move/Trash/Restore/history_restore、retirement/cleanup 的声明及来源／屏障／回执分支；读取所列函数。查询HTTP files/file_batches/object_retirement/derived_retirement/multipart_cleanup的路由，再追到Web/shared-mobile入口；查询迁移0099/0104/0120的不可变约束。查询未知来源相关 `UNCERTAIN_OBJECT_PROTECTED`、`UnknownExternalCount`，结果为受保护或单列状态，未见当前路径把任意输入桶／键当作删除凭据。

**How reached / Open questions:** 未确认新增可直接修复的GAP-05代码差异。当前边界不是“所有对象都已清理”：未知孤儿／外部派生身份、公司保留制度采纳、真实云身份／独占命名空间安装与端到端删除验收仍开放。未穷尽所有SDK、签名／复制重试、批处理／范围迁移细枝及并发交错；未执行本轮或历史测试，不把源码存在等同正式四端／云验收。
