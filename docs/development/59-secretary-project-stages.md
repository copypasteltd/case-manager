# 秘书里程碑与项目阶段

本增量继续推进GAP-11／12／13／18，代码在app/，文档在docs/。在原准确更新协议中增加update_milestone、transition_milestone、transition_project；既有项目／任务更新继续复用同一真实业务服务。完整目标见[编号差异表](../delivery/03-goal-gap-assessment.md)。

## 1. 功能与业务约束

| 编号 | 员工发送的准确操作 | 实际业务处理 |
| --- | --- | --- |
| STAGE-01 | 选择已有里程碑及版本，指定标题、项目成员负责人或带时区目标日期 | 原UpdateMilestone保存新版本；未选择字段保持原值，不接受清空负责人／日期 |
| STAGE-02 | 选择里程碑及achieve／miss／cancel／reopen动作 | 原TransitionMilestone检查状态；achieve要求存在关联任务或交付，且全部完成或已验收；miss要求已逾期 |
| STAGE-03 | 选择当前会话项目及activate／hold／resume／close／cancel／reopen动作 | 原项目状态机、范围／成员、结项阻塞与重开权限全部保留 |
| STAGE-04 | 明确发送、核对实际结果、查询历史或取消 | 受理不代表业务完成；真实操作号、对象和新版本构成提交回执，同参数重试只返回原操作 |

归档继续要求原结项核查、近期MFA、成果复盘和准确确认，archive不在秘书工具中。不能通过更改关联任务或交付来制造里程碑达成；不开放项目成员、项目负责人、分类、任务人工验收要求等字段。里程碑新建和关联事项编辑继续使用原业务页面。

## 2. 授权与原始登录

原RunCreate.mutation协议扩展为六项准确写操作。每个运行只接受一个工具、一个对象、准确基础版本和明确字段／动作，不能从普通消息、引用或历史授权取得新的写权限。复用原项目会话共同权限和AI处理政策。

所有新建准确更新意图均绑定原始web／android／wechat／douyin交互登录。执行开始、工具读取／写入和完成时重查原登录、员工和组织授权版本；原登录被撤销、过期或轮换后停止。签名私有意图保存原登录ID，公开运行、模型上下文和分段意图读取只暴露公开范围与业务结果。

迁移[0058](../../app/backend/migrations/0058_agent_project_stages.sql)为原仅追加表增加可空session_id并保留历史，不回填伪造登录。迁移前没有绑定登录的未完成意图不能继续执行，员工须重新发送；历史事实仍按原权限可读。新意图及提交触发器拒绝空登录，并核对实际项目内对象、版本、结果状态和新增里程碑字段。原不可改写、准确业务幂等、原员工消息及同事务工具回执约束继续生效。

## 3. 读取、冲突与事务

里程碑内部读取包含当前标题、负责人、目标日期、状态和版本，使用原milestone.read及项目分类／共同AI政策。read_resource的milestone类型只允许当前运行明确授权的目标；其来源为固定agent_mutation，不提供任意里程碑枚举或新公共资料类型。

执行前后核对固定基础或本运行真实提交后的准确结果。并发人工修改、权限变化或原登录撤销不能静默重基；员工重新读取后另发决定。里程碑的关联任务／交付及项目结项阻塞由原业务在实际提交事务中再次核对。

真实业务变更、审计、Outbox、幂等结果、agent_mutation_commits及工具回执在同事务中提交；回执失败整体回滚。取消前未写入不产生业务修改；取消后保留已经完成的历史事实。

## 4. 客户端与Skill

Web／Taro准确更新选择器按工具切换项目、任务或里程碑，分页读取可见对象并重新获取当前版本；里程碑负责人从实际项目成员中选择。阶段动作显示原状态、明确动作与原因。选择操作不发送，员工填写消息并确认发送后才形成一次授权。

project-update源码包1.1.0仍为draft，准确工具集合为read_resource、inspect_operation及六项写操作。三项新增工具均需本运行真实业务回执才能报completed；Skill说明或inspect_operation不构成业务完成证据。真实模型、原生隔离和八项Skill发布尚未因此完成。

## 5. 验证

本轮验证记录保存在[secretary-project-stages](../quality/evidence/secretary-project-stages/verification.md)。原生PG与真实认证HTTP／MCP验证实际业务，受控原生证明不代表实际模型隔离；H5／构建不替代两小程序或Android真机。

```powershell
$env:GOFLAGS='-p=1'
$env:GOMAXPROCS='2'
$env:PYTHONUTF8='1'
python app/scripts/test_backend.py --package ./internal/httpapi -run '^(TestAgentProjectStages|TestAgentRunMutations)$' -timeout 6m
python app/scripts/verify_backend_batches.py --evidence-group secretary-project-stages --batch-size 20
python app/scripts/verify_web.py secretary-project-stages
python app/scripts/verify_mobile.py secretary-project-stages
python app/scripts/verify_agent_runtime.py secretary-project-stages
python app/scripts/verify_linux.py secretary-project-stages
python app/scripts/run_agent_browser.py --project-stages
python app/scripts/run_mobile_browser.py --project-stages
python app/scripts/verify_secretary_project_stages_development.py
python app/scripts/verify_secretary_project_stages_sources.py
python app/scripts/verify_secretary_project_stages_docs.py
```

仍需推进其他业务工具、里程碑创建与关联事项秘书操作、完整移动覆盖、真实模型／TG／双云、120GB／2TB、真机、生产部署和恢复。本增量不代表完整Goal完成。
