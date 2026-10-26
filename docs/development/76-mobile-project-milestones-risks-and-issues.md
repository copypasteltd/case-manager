# 增量76：移动里程碑、风险与问题闭环

Taro公共客户端在既有16个注册页面中增加里程碑、风险和问题的新建、资料编辑及明确状态跟进。复用原Go业务命令，不增加迁移、另建状态机或代替秘书／员工的授权。代码位于`app/`，文档与证据位于`docs/`。

## 1. 功能与入口

| 编号 | 功能 | 行为 |
| --- | --- | --- |
| MR-01 | 新建三类事项 | 从项目的里程碑／风险／问题列表进入；选择本项目实际有效负责人，保存后显示真实新ID、版本1及open状态 |
| MR-02 | 里程碑资料与状态 | 编辑标题、负责人和目标时间；明确执行达成、错过、取消或重开，使用事项自身版本 |
| MR-03 | 里程碑完成证据 | 达成仍由服务端检查至少一个已完成关联任务或已验收交付，且全部关联事项完成；没有证据时不改状态 |
| MR-04 | 风险登记与应对 | 编辑标题、说明、负责人、影响、概率、期限及措施；支持应对、关闭及重新打开 |
| MR-05 | 风险转关联问题 | 独立预览及确认实际发生；原事务新建同项目问题并返回实际issue_id，风险之后只读，沿实际链接继续处理 |
| MR-06 | 问题处理闭环 | 可独立登记或读取风险生成的问题，编辑资料，并明确调查、解决、关闭或重开；解决／关闭须填写实际解决说明 |
| MR-07 | 版本与结果核对 | 创建核对实际新记录，编辑与跟进核对原ID、原项目、准确下一版本、状态和完整相关字段；未知或冲突不自动重放 |
| MR-08 | 只读与范围 | 当前权限、项目成员角色、事项阶段及closing／archived／cancelled共同限制入口；不同项目的事项不能在错误项目深链接下打开写表单 |

风险概率未设置时仍显示应对措施和实际关联问题链接，不能用可选字段是否存在推断事项类型。已发生风险不通过重开绕回旧风险，后续跟进实际问题。

## 2. 原接口与状态规则

| 操作 | 原接口 | 版本与约束 |
| --- | --- | --- |
| 新建里程碑／风险／问题 | POST `/projects/{id}/milestones`、`/risks`、`/issues` | 原业务检查当前项目及有效成员并分配新ID；不伪造父项目expected_version |
| 编辑里程碑 | PATCH `/milestones/{id}` | 事项expected_version；仅open／missed |
| 里程碑状态 | POST `/milestones/{id}/transition` | expected_version／action／reason；错过依据服务端overdue，达成依据实际关联结果 |
| 编辑风险／问题 | PATCH `/risks/{id}`、`/issues/{id}` | expected_version及完整核对资料；编辑不隐含state变更 |
| 风险／问题状态 | 原PATCH接口 | 独立expected_version／state／reason；问题解决／关闭另带实际resolution |

风险原图：open→mitigating／closed／realized，mitigating→closed／realized，closed→open，realized无后续风险写操作。转为realized还要求原项目issue.create权限，成员资格不会自动授予业务动作。

问题原图：open→investigating／resolved，investigating→resolved，resolved→closed／investigating，closed→open。关闭事项只提供明确重开入口，不能直接编辑。里程碑保留原achieve／miss／cancel／reopen状态约束与证据检查。

## 3. 日期、文字与恢复

界面按香港时间输入`YYYY-MM-DD HH:mm`，可包含秒及最多六位小数；不取设备本地时区。也接受带明确时区的RFC3339输入，严格检查实际日历、小时、分钟、秒和偏移，拒绝自动滚入下个月。保存、读取和回执比较保留PostgreSQL六位微秒，不以毫秒截断把两个不同目标当作一致。显示转换分别处理整秒和微秒，避免依赖引擎对非标准小数精度的解析。

里程碑目标时间必填；新风险／问题允许不填期限。原风险／问题PATCH暂不支持清空已有期限或概率，移动表单会说明并拒绝这种输入，不发送被服务端忽略的null或把保留原值显示为清空成功。其他文字按原业务上限保留，标题统一首尾空白；解决说明保留实际全文。

八项`ResourceInput`文字加入原15分钟、32项、总10万字符的共享内存预算，绑定本人、登录代际、当前授权、种类、实际对象及版本。读写均复制白名单字段，不保存凭据、完整API结果、生成问题ID、确认声明或授权证据。状态操作的文字与原因按实际对象版本及所选动作分别隔离；关闭、刷新、撤权、新登录或新业务版本不得自动恢复旧授权。

HTTP成功后仍核对真实项目、实际新ID或原ID／下一版本、标题、负责人、精确时间、状态以及风险／问题字段；风险发生须返回真实关联问题ID。冲突、权限变化、网络响应未知或不完整成功回执会保留输入、锁住重复提交，并要求重新读取实际结果。前端中断不代表原事务回滚。

## 4. 实现位置

| 位置 | 作用 |
| --- | --- |
| [resource-forms.tsx](../../app/mobile/src/pages/project/resource-forms.tsx) | 三类新建／编辑、当前范围重读、独立状态预览与确认 |
| [project-resource-input.ts](../../app/mobile/src/platform/project-resource-input.ts) | 香港时间、微秒精度、严格输入、原命令及真实回执 |
| [resources.tsx](../../app/mobile/src/pages/project/resources.tsx) | 详情写入口、可选概率下的风险措施与实际关联问题 |
| [drafts.ts](../../app/mobile/src/platform/drafts.ts) | 文字白名单、共享限额、种类／动作／准确版本隔离 |
| [输入测试](../../app/mobile/src/platform/project-resource-input.test.ts)、[草稿测试](../../app/mobile/src/platform/drafts.test.ts) | 时间、实际成员、状态、准确字段、微秒差异、旧模板及内存隔离 |
| [真实流程检查](../../app/scripts/check_mobile_project_resources.mjs) | 原生PostgreSQL、真实登录、实际表单、完成任务、响应丢失及撤权 |

本轮未改Go、SQL、运行时、Web、依赖或八项Skill状态。未改动部分只有在完整当前源码摘要相同时才复用原证据。最终测试、微信／抖音编译、真实流程、新Android开发APK及失败尝试见[验证记录](../quality/evidence/mobile-project-resources/verification.md)。

本轮推进GAP-01／18，后续移动任务创建／依赖、项目变更实施／预算／结项、知识／附件及完整财务操作仍按[Goal差异表](../delivery/03-goal-gap-assessment.md)推进。真实模型、云／TG／平台／设备、120GB／2TB、安全／性能与生产部署／恢复继续保留原验收标准。
