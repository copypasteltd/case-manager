# GAP-25 — 生产 PostgreSQL 持久化

> “接入生产持久PostgreSQL，完成权限、连接、建表／结构升级、事务和实际读写核对；正式文件由OSS＋R2管理。”
>
> — `docs/delivery/03-goal-gap-assessment.md:196`

**结论：代码路径已实现；目标安装／真实读写验收 undecidable。新增确定代码缺项：0。** 本记录为增量168只读源码审查，基线为167及当前工作区；不执行测试、构建、数据库连接或服务操作，不关闭GAP-25。

---

**要求及边界。** `docs/delivery/04-current-scope-and-acceptance.md:123` 明确保留“生产持久PostgreSQL、建表／结构升级”，`:139` 区分正常结构升级与已移出的历史数据迁移。`docs/development/119-native-production-release.md:47` 要求运行／迁移连接同一地址和数据库、验证TLS、独立登录，并明确这些检查“不替代生产角色权限、证书和实际读写验收”。因此本项不要求恢复PITR、历史导入、灾备或性能门槛，也不将尚未提供生产资料推断为缺少连接实现。

**执行与持久化路径。**

- `app/backend/internal/platform/config.go:38` 缺少数据库连接即失败，不回退本地文件；`:65` 禁止生产API取得迁移连接。`worker_config.go:18`、`:21` 同样要求独立运行连接、禁止迁移凭据。`database.go:20` 通过pgx解析配置、建立真实PG连接池，固定UTC和查询／空闲事务超时；`:35` 连接检查失败会关闭连接池。
- 受控生产配置 `app/deploy/release/controller.py:93` 的 `database_identity` 解析URI和唯一非空参数，只允许TLS／超时等白名单，要求 `verify-full` 并拒绝地址或登录覆盖。`:249`–`:267` 检查全部启用运行服务及专用迁移文件，要求同一数据库身份指纹、运行账号非postgres/root、迁移与运行登录不同。仅看Go API的TLS子串判断不足以证明生产配置安全；本审查继续追踪了此实际部署入口，而非将该单独判断当作完整门禁。
- `cmd/platform/main.go:95` 的显式迁移命令只读取迁移身份，进入 `migrations/migrate.go:18`。迁移在PG advisory lock下逐文件排序，`:43` 对已应用SQL重新比对原字节SHA；新SQL及其台账在同一事务内执行、失败回滚、成功提交（`:53`–`:65`）。`0006_application_privileges.sql:1` 撤销公共建表权，条件授予应用角色业务表权并撤销台账及若干历史表修改权；`0017_schema_manifest.sql:2` 仅向应用角色暴露受控只读结构清单。
- API／上传Worker启动分别在 `cmd/platform/main.go:263`／`:216` 调用 `migrations.CheckCompiled`；`migrations/compatibility.go:17` 对结构名称、顺序、数量及逐文件SHA要求完全一致。`httpapi/server.go:56` 的就绪接口再次执行此检查。生产协调器 `controller.py:446`–`:453` 只接受准确结构或经明确同意的完整前缀向前升级；`:489` 禁止把普通发布变成数据库切换。旧应用回退仍受准确结构限制（`:469`）。
- `platform/commands.go:35`–`:46` 建立／复用真实PG事务；业务修改后，同一事务写审计、outbox和成功幂等回执，再提交（`:96`–`:134`），中途错误返回并由拥有者回滚。基础组织／用户、项目／任务、合同均为普通PG表（`0001_identity_platform.sql:1/:11`、`0002_projects_knowledge.sql:9/:40`、`0027_contract_revisions.sql:2`）。`identity/bootstrap.go:49` 要求显式事务，首次组织创建及角色授权随调用方事务提交，非内存或本地JSON替代。

**搜索与反证。** 使用 `rg --files` 定位migrations、platform、cmd及release；检索 `CP_DATABASE|CP_MIGRATION|sslmode|schema|migrate`，沿配置→原生发布→命令→PG→结构门禁和事务提交逐段阅读。非测试生产路径搜索 `synchronous_commit|full_page_writes|fsync|CREATE UNLOGGED|CREATE TEMP`：返回发布工具的落盘fsync，未返回关闭PG持久性或将业务表建成临时／unlogged表的代码。检索 `current_user|rolsuper|rolbypassrls|has_table_privilege` 返回既有集成用例中的角色检查，未将其当作本轮执行证明。生产数据库角色的实际权限和服务器持久设置仍须目标核对，规范亦明确保留该验收，不能据此另造“必须自动创建PG服务器／账号”的代码需求。首次存储元数据生产者及秘书配置安装候选分别归GAP-20／13，不在本项重复计数。

**未读／未验证。** 未接触用户生产PG、TLS证书、私有角色配置或磁盘；未穷尽每项业务SQL约束与所有133项历史迁移的业务正确性。没有启动PG、改变开发PG100/API、重跑既有集成测试或声称目标持久性／断电恢复通过。真实权限、连接、普通升级及读写验收继续开放；已移出的历史迁移与容灾保持移出。
