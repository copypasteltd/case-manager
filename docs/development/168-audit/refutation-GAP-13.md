# GAP-13 候选独立反证

日期：2026-11-06。不同审查者只读复核 `GAP-13.md` 的两个候选；没有测试、构建、应用代码修改、浏览器、服务、虚拟化或真实安装。结论只覆盖当前仓库可追踪的实现，不认定外部评估已通过。

## 反证结果

| 候选 | 判定 | 独立依据与范围 |
| --- | --- | --- |
| G13-C01：缺少部署身份受控的准确包／前置证据→profile 安装生产者 | **存活，代码缺口；高可信度** | 受控发布目录、摘要描述、已有配置查阅／停用、安装事件观察均存在；查无能据此创建 `agent_runtime_profiles` 的非测试生产者。缺少的是实际受控安装命令／部署入口及持久化来源绑定，不能归为“只差凭据”。 |
| G13-C02：发布清单未保存评测集版本及结果 | **限定存活，元数据保存／关联缺口；中高可信度** | 成功运行编号和业务结果不等于版本化评测集及逐项评定。157明确这是已安装配置上的阶段性交付，未撤销07规范的保存要求；可实现准确证据导入／绑定／不可变记录，不必为了本项新增自动模型评测器。真正公司评估内容及真假尚属外部验收。 |
| 附带推论：published 才能执行、成功运行才能登记 published，因此数据库发布治理必然死循环 | **作为独立缺陷驳回** | manifest 状态与数据库发布台账是两层。0114 无台账时明确保留旧安装兼容，不要求先有 published 台账才执行。当前八包 draft 被拒绝是正确保护。首包受控评估／产物批准／安装步骤仍缺在 C01／C02，不能据此把生产 draft 直接开放或虚构已有 pilot 执行通道。 |

## C01：尝试找到反例后的生产者追踪

1. `app/backend/migrations/0047_agent_runs_and_messages.sql:1`–`:17` 定义“部署身份安装”的不可变配置，`:139` 撤销应用身份 INSERT／UPDATE／DELETE。它建立表和约束，不建立安装 CLI。
2. `app/backend/migrations/0107_agent_runtime_operations.sql:10` 只从已存在 profile 写 retained_baseline；`:14`–`:27` 的 AFTER INSERT／UPDATE 触发器观察外部已发生的安装／停用，不产生 profile。这里不能用 `installed` 事件名称误认有安装生产者。
3. `app/backend/cmd/agent-native/main.go:41`–`:61` 的命令仅 serve／check／launch／probe／child／keygen／public-key／release-digest，没有数据库安装命令。`app/agent-runtime/src/describe-profile.ts:16`–`:19` 只输出准确配置／Skill摘要和 missing_tools，明确 `database_modified:false`；ready_for_controlled_installation 也不是执行安装。
4. `app/scripts/build_native_runtime_release.py:125`–`:134` 保存发布目录文件清单、摘要及八包仍 draft 的事实；未连接数据库或发布包状态。`app/deploy/release/controller.py:160`–`:162` 核对已安装 systemd 单元，管理应用发布指针，查无 profile 注册逻辑。`app/backend/cmd/platform/agent_worker.go:119`–`:123` 构造执行服务，没有安装逻辑。
5. `app/backend/internal/httpapi/agent_runtime.go:14`、`:30` 是已存在 `{id}` 的 Skill 发布台账；`:33`、`:46` 查阅已存在安装；`:52` 停用。`app/backend/internal/agent/runtime_operations.go:168`、`:195` 只插入人类停用决策。`skill_releases.go:64` 先读取既有 profile，`:139`–`:152` 先校验它及成功运行，再登记发布决定。均不能补出首次安装行。
6. `docs/development/105-native-agent-runtime.md:104` 明确“正式 profile 安装／发布流程仍属 GAP-09／13，本轮没有自动安装命令，不得手改 draft 状态绕过评测”；`docs/development/141-secretary-runtime-operations.md:27` 明确现有应用无新建／编辑／恢复配置接口。这些是缺口披露，不是当前永久移出安装实现的用户范围决定。

全仓 `INSERT INTO agent_runtime_profiles` 正向反例仅找到集成测试中的隔离 synthetic／controlled-test fixture，如 `app/backend/internal/httpapi/agent_project_catalog_integration_test.go:173`、`agent_oa_requests_integration_test.go:92`。这些未经部署包／前置评估绑定的夹具 INSERT 不能当正式安装入口。未执行这些代码。

最小可实施边界是部署专用入口消费准确包／SDK／Schema／工具／配置与经过明确审阅的前置证据、安装身份、原因，在普通组织锁和不可变事件规则下原子注册新 profile；不向应用角色开放任意 profile 写入，不覆盖／复活旧配置，不自动把仓库 draft 变为可执行。实现入口后实际服务器安装仍未验收。

## C02：规范优先级与保存内容

- `docs/development/07-agent-tools-and-skills.md:110` 规定 draft→tested→pilot→published→disabled；`:112` 原文要求“发布清单保存 Runtime/模型端点配置版本、Skill版本、工具版本、评测集版本及结果”。
- 后来的 `docs/development/157-batch-functional-closure.md:12` 说明本轮只关联准确已安装配置及本人成功运行；`:32` 明说“新增发布台账不冒充真实评估，也不自动把任何包发布”，正式启用仍需公司评估／部署。`docs/delivery/03-goal-gap-assessment.md:188` 仍把逐包安装、真实样本评估、公司发布／回退列为开放要求。没有用户指令或该增量中的明确文字取消07:112；“真实评估待提供”不取消代码保存其版本／结果的要求。
- `app/backend/internal/agent/skill_releases.go:16`–`:48` 的输入／输出只有 assessment_run_ids，`:139`–`:150` 检查本人该配置 succeeded/completed；`app/backend/migrations/0114_skill_release_governance.sql:3`–`:15` 和 `:43`–`:45` 亦仅保存／验证成功运行编号。它们已有真实结果引用，不能称为完全没有评估证据，但没有评测集版本／摘要、用例输入身份与逐项评定绑定。
- `app/skills/manifest.schema.json:86` 的 test_case_refs 只是用例编号数组；例如 `app/skills/oa-assist/manifest.json:31` 的 TC-AI-01／02／03 不固定公司评测数据集版本或逐项结果。打包文件摘要和一般业务运行成功也不能推导该数据。

因此保留有限代码要求：记录外部评测集的版本／内容摘要、准确包／配置关联及结果引用或有界结果内容，并随安装／发布决定固定保存。此判断不要求现在编造数据集、运行真实模型、导入未准备好的公司资料或把失败／待验收改成通过。

## 首次发布与未定边界

`app/agent-runtime/src/skills.ts:89`–`:94` 默认只允许 published；`codex-adapter.ts:127`–`:130` 调用时未传 pilot=true；`worker.ts:19`–`:22` 同时重核版本和准确包摘要。当前没有可用的生产 draft／pilot 后门。`app/backend/migrations/0114_skill_release_governance.sql:19`–`:21` 的无台账返回 true 与 `runs.go:170` 的门禁证明台账不是强制前置循环；发布包本身仍必须经独立批准流程成为可执行产物。首次评估可以由明确受控的独立前置过程提供，不应凭本次审计强制创造一套未约定的在线 pilot。

未读／未验证：本机 Git 外的私有运维工具、目标服务器现有人工部署流程、公司是否已有版本化评测集／签批、实际原生运行／模型数据及其他 GAP 的业务工具正确性。若外部有安装工具，只能补为外部事实；尚不能当作当前仓库交付的代码。全部八包继续 draft，GAP-13继续开放。

## 搜索范围

使用 `rg --files` 定位 backend/cmd、deploy、scripts、agent-runtime 及 migrations；在上述路径及全仓代码类型搜索 `agent_runtime_profiles`、大小写无关多行 INSERT／COPY／MERGE、`describe-profile|install.*skill|runtime.*profile`、`assessment_run_ids|evaluation_set|assessment.*version|assessment.*result|评测集`。查看所有非测试 profile 引用文件清单，定位实际生产消费者；开发验证脚本仅核对数据库权限／计数，没有生产写入入口。全仓非测试 INSERT 检索返回零，含测试检索能返回上述具体 fixture，证明没有把排除模式下的零结果直接当作完整安装缺失的唯一依据。根目录之外生产代码搜索及 `.github`／deploy 命令检索未找到另外的 profile 生产者。
