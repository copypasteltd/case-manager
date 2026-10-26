# 项目模板管理与固定修订初始化（增量67）

日期：2026-10-26；实现位于 `app/`，本文及验证资料位于 `docs/`。推进 GAP-01／07，沿用 PM-01／02 和既有授权、状态、审计及秘书准确计划约束。完整平台 Goal 继续执行。

## 1. 本轮交付边界

| 编号 | 已实现能力 | 关键约束 |
| --- | --- | --- |
| PTM-01 | Web 模板系列列表、状态／名称／系列筛选、分页、固定修订详情 | 有创建项目权的员工只读已发布修订；配置管理员可读草稿、停用修订及操作历史 |
| PTM-02 | 中性模板草稿的结构化编辑 | 目录名称逐行输入，任务含说明／优先级／验收要求，里程碑逐行输入；没有脚本、文件路径或可执行内容字段 |
| PTM-03 | 精确发布、停用与从旧修订建立新草稿 | 写入需要 `config.manage`、本人有效交互会话及最近10分钟MFA；操作固定记录版本和内容摘要，原因必填 |
| PTM-04 | 发布内容不覆盖，完整历史保留 | 模板ID对应一个固定修订；系列key和修订号不变。已发布内容在数据库层禁止改写；停用保留原内容及历史 |
| PTM-05 | 人工新建项目选择已发布模板 | 展示完整初始化内容；不隐式选“最新”。默认标准目录，不生成模板任务；选择模板后传准确ID |
| PTM-06 | 同事务初始化项目、成员、逻辑目录、任务和里程碑 | 对模板加共享行锁，保存 `template_id` 与 `template_version`，使用同一份内容初始化；失败全部回滚 |
| PTM-07 | 秘书准确计划与模板生命周期一致 | 发布新修订不改变旧计划指向；停用原固定修订拒绝旧计划提交；继续复用原计划／MFA／权限与业务事务 |

模板内容属于公司通用配置。编辑器明确要求使用可共享的中性结构，不保存项目原件、客户资料、账号、密钥或审批规则。拥有配置权限不因此获得项目内容权限。

本轮只生成逻辑结构。没有模板文件字节生成、自动成员授权、自动日期排程、模板依赖关系或跨项目应用器；也不把草稿设为可用模板。项目看板／基础时间线、更多项目聚合及完整跨OA／财务旅程仍在 GAP-01。移动端本轮只做兼容回归，未增加模板配置页面。

## 2. 模型与版本

`project_templates` 原有一行即一个修订，本轮保留该ID语义。新增记录 `version`、说明、创建人、来源修订、发布／停用时间与操作者。旧行未知的操作者和时间保持空值，不补造历史。

| 字段 | 语义 |
| --- | --- |
| `key` | 组织内模板系列，小写字母开头、1—64位字母／数字／连字符；创建后固定 |
| `revision` | 系列内单调递增修订号；新修订生成新ID |
| `version` | 当前修订记录的并发版本；草稿编辑、发布、停用递增 |
| `based_on_id` | 建立草稿时明确选择的旧固定修订；不随系列后续发布移动 |
| `content_hash` | key、名称、说明及完整schema的规范JSON SHA-256；发布／停用／建立修订须提交精确值 |
| `state` | `draft → published → archived`；草稿也可直接停用；停用不重新启用，需另建修订 |

同一系列最多一个草稿，由部分唯一索引及事务内系列锁共同约束。系列可以同时有多个已发布固定修订；发布新修订不会自动停用旧修订。员工选择时始终看到修订号，管理员须另行明确停用旧修订。序号取系列最大修订加一；并发建立只允许一份草稿成功。

操作历史 `project_template_events` 保存当时完整快照、内容摘要、记录版本、操作者、原因及时间。历史只增，应用身份不能更新或删除；管理后的模板更新须在同事务包含相应准确历史快照。新应用写入仅允许有创建人、版本1的草稿；兼容迁移身份读取的历史行，不开放接口伪造旧记录。

## 3. 结构与初始化

schema包含 `folders[]`、`tasks[]`、`milestones[]`，拒绝未知字段。最多100个一级目录、200项任务、100个里程碑，序列化schema不超过256KiB；名称不超过200字符，任务说明不超过20000字符，模板说明不超过2000字符。

目录复用既有 `StorageNameKey` 校验：拒绝路径分隔符、控制／隐藏字符、点目录、系统保留名和经Unicode／大小写标准化后的重复名称。模板中的标准目录“合同／交付／工作资料”复用已有逻辑目录，其余冲突直接拒绝，不再用宽泛 `ON CONFLICT DO NOTHING` 隐藏错误。

任务按模板顺序初始化 `sort_order`，状态为待开始、未分配，不设置日期或假定完成率；优先级与验收要求保留。里程碑状态开放，负责人使用实际项目负责人，创建人仍记录本次操作者，不伪造日期或完成事实。

项目创建仍通过原 `project.create`、成员管理与分类授权，并校验有效成员。读取模板时要求其当前已发布并在事务中共享锁定；项目记录和生成内容取同一份schema。停用与创建竞争时按锁串行：创建先取得锁并成功提交的项目保留；停用先提交则后续创建拒绝。项目、逻辑目录、模板任务、里程碑、审计、Outbox和幂等回执属于原命令事务，任一错误不留下半初始化项目。

本轮schema无异步文件生成。`initialization_state=complete`表示此次逻辑结构已事务提交，不表示对象存储文件、副本或后续业务完成。旧项目保存的模板ID／修订不因新版本发布或停用改写。

## 4. API与授权

| 路径 | 行为／范围 |
| --- | --- |
| `GET /v1/project-templates` | 状态、q、key及签名游标分页；先按当前角色过滤可读状态 |
| `GET /v1/project-templates/{id}` | 精确固定修订；普通创建人看不到草稿／停用修订 |
| `GET /v1/project-templates/{id}/history` | 仅配置管理员可读；游标绑定本人、授权版本及模板ID |
| `POST /v1/project-templates` | 新系列初稿；拒绝已有系列key，不自动创建第二个系列 |
| `PATCH /v1/project-templates/{id}` | 完整替换草稿内容，要求 `expected_version`；不能改变系列或修订身份 |
| `POST .../{id}/publish` | 精确内容摘要和记录版本，发布完整固定修订 |
| `POST .../{id}/retire` | 精确停用；拒绝新初始化与旧秘书计划，不删除项目或历史 |
| `POST .../{id}/revise` | 从明确旧修订复制中性结构为下一独立草稿；旧内容继续保留 |

全部写入使用原幂等协议、当前组织／用户锁、审计与Outbox。不能凭管理员身份跳过最近MFA。TG及Agent运行不能调用此管理服务；本轮没有给模型增加模板管理工具或配置权限。

接口契约由[生成器](../../app/scripts/build_openapi.py)生成到[OpenAPI](openapi.json)、后端嵌入副本及共享TypeScript类型。新增字段 `Project.template_version` 为可选，旧客户端仍可读取项目，未替换原路由。

## 5. Web与失败处理

项目空间可进入模板目录；配置管理员侧栏具备项目模板入口。详情展示目录／任务／里程碑完整结构、系列、修订、记录版本、摘要、发布／停用事实及操作历史快照。旧资料无历史或时间时明确提示未知。

结构编辑通过表单，不要求员工手写JSON。保存冲突保留所有人工输入；必须读取当前修订、展开内容比较并明确接受新的记录基线后才允许重新保存。当前已发布或停用则禁止继续编辑。后续再次冲突须重新读取比较，不自动追用新版本。

发布／停用／新修订各自独立弹窗，固定打开时的准确依据、影响说明、原因、确认勾选和真实MFA。网络结果未知时冻结该命令内容，并提供使用原幂等键原样核对；不自动重发写入。普通项目创建表单也复用按相同完整内容稳定的幂等键；选择后模板被停用返回明确冲突并保留输入，不默默改成标准目录或新修订。

浅／深主题复用现有颜色和组件。布局测试等待字体、媒体查询和两帧布局稳定后检查320／390／1440宽度；页面截图只包含明确虚构样本。

## 6. 实施与验证入口

核心实现：[模板服务](../../app/backend/internal/project/template_management.go)、[初始化器](../../app/backend/internal/project/templates.go)、[项目创建](../../app/backend/internal/project/service.go)、[路由](../../app/backend/internal/httpapi/project_templates.go)、[迁移0064](../../app/backend/migrations/0064_project_template_management.sql)、[Web页面](../../app/web/src/ProjectTemplates.tsx)、[项目表单](../../app/web/src/Projects.tsx)。

```powershell
python app/scripts/test_backend.py --package ./internal/httpapi -run '^(TestProjectTemplateManagement|TestAgentPlans)$' -timeout=12m -v
python app/scripts/verify_backend_batches.py --batch-size 20 --evidence-group project-templates
python app/scripts/run_project_templates_browser.py
python app/scripts/verify_web.py project-templates
python app/scripts/verify_mobile.py project-templates
python app/scripts/verify_project_templates_development.py
```

只对所属原生开发环境应用迁移与更新API的脚本为[开发更新器](../../app/scripts/update_project_templates_development.py)，校验精确loopback地址及记录的PID／创建时间；不重启PG／通知Worker／日常Vite，不创建业务样本，不开启模型、云或TG。正式部署仍按GAP-24／25另行验收。

本轮完整回归期间发生会话中断，确认原进程已退出后，由[恢复脚本](../../app/scripts/restore_project_templates_development.py)重启所属API／通知Worker／Vite，沿用已恢复的原生PG数据目录。该脚本拒绝覆盖存活的记录进程及已占用端口，不重新初始化PG；[恢复记录](../quality/evidence/project-templates/development-restoration.json)与最初的更新记录分别保留。完整后端通过 `--resume` 仅恢复未完成批次，来源摘要必须与已完成批次一致。

最终结果及边界见[本轮验证记录](../quality/evidence/project-templates/verification.md)。实体设备、生产环境、业务真实模板审核和完整项目验收仍未由本轮本地验证证明。
