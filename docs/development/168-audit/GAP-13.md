# GAP-13 — 八项 Skill 发布与安装治理源码核对

本记录为增量168整批代码开发前的独立只读审查，基线 `930cdde97b9d5d2e85d892664b0857a10a811739`。未运行测试、构建、浏览器、服务或虚拟化；未修改应用代码。候选差异待另一审查者反证，不代表正式验收完成。

## 规范与当前包

- `docs/development/07-agent-tools-and-skills.md:110`：“发布链 draft→tested→pilot→published→disabled，已发布包按 hash 不可变。”
- 同文件`:112`：“发布清单保存 Runtime/模型端点配置版本、Skill版本、工具版本、评测集版本及结果。”
- `docs/development/157-batch-functional-closure.md:12`：发布关联本人准确已安装配置的成功运行；暂停影响后续核对；回退先安装原已发布代码。`:32` 明确无发布台账的旧安装配置保留开发兼容，不据此误报默认放行漏洞。
- `docs/development/105-native-agent-runtime.md:104` 明示：“正式 profile 安装／发布流程仍属 GAP-09／13，本轮没有自动安装命令，不得手改 draft 状态绕过评测。”

下列各 `app/skills/<id>/manifest.json:2`–`:6` 均直接读取，全部 `draft`、`codex-sdk@0.164.0`；没有提升状态。

| Skill | 版本 |
|---|---|
| file-update | 1.7.0 |
| incident-summary | 1.2.0 |
| information-organize | 1.3.0 |
| oa-assist | 1.6.0 |
| project-briefing | 1.2.0 |
| project-create | 1.2.0 |
| project-update | 1.11.0 |
| subscription-review | 1.4.0 |

## 已追踪的实现链

1. `app/skills/manifest.schema.json:14,28,86` 定义状态、准确SDK兼容性及用例引用；`app/agent-runtime/src/skills.ts:96` 加载八包，校验路径/工具/授权/输入输出；`:118`–`:122` 将公共及清单Schema、输入输出Schema、指令、排序后的引用文档原字节一起计入摘要。`check-skills.ts:5`–`:7` 只输出实际包并核对数量，不声称执行模型。
2. `app/scripts/build_native_runtime_release.py:88,104,125,133` 构建完整运行依赖、技能及引用文档清单，记录每文件摘要与实际draft状态。`app/backend/internal/nativeruntime/linux_server.go:84` 校验整个发布目录；`config.go:165,196,200` 核对配置、模型、包及运行绑定。
3. `app/agent-runtime/src/describe-profile.ts:16`–`:19` 只描述准确配置/缺失工具；`worker.ts:19`–`:22` 重核版本及摘要；`codex-adapter.ts:127`–`:130` 经 `SkillPackage.assertRunnable`，后者 `skills.ts:89`–`:94` 拒绝draft与错摘要/缺工具。生产调用不启用pilot参数，不把台账pilot当作草稿执行授权。
4. `app/backend/internal/agent/skill_releases.go:50,114,129,139` 及 `migrations/0114_skill_release_governance.sql:24`–`:55` 约束当前原人会话/MFA、独立read/manage、组织锁、准确配置/活动任务依据、本人成功运行、匹配原发布的回退及不可变决定。`runtime_operations.go:168`–`:203` 是显式审阅停用。`runs.go:170,441`、`execution.go:151` 重核启用状态。
5. HTTP入口 `app/backend/internal/httpapi/agent_runtime.go:14,30`；Web `app/web/src/AgentRuntime.tsx:28` 与共享移动 `app/mobile/src/pages/secretary/runtime.tsx:31` 调用同一发布台账；`app/packages/contracts/src/agent-runtime.ts:21`–`:23` 保留当前决定与回退限制。

## 候选必须代码差异（待独立反证）

| 编号 | 规范与源码差异 | 可实施的代码边界 |
|---|---|---|
| G13-C01 | 包构建、摘要描述与“已经安装后”的治理均有实现；未找到部署身份受控安装入口。`0047_agent_runs_and_messages.sql:1`–`:17,139` 定义配置与禁止应用身份写入，105文档仍明确安装流程开放。 | 补独立部署工具：准确包/Schema/SDK/工具/配置校验、前置验收证据与批准记录、事务性不可变配置安装、拒绝不完整/伪造证据。不能用手改draft或人工SQL代替完整流程；实现工具不等于执行真实安装。 |
| G13-C02 | 07规范要求评测集版本及结果；现有 `skill_releases.go:16`–`:48`、`0114:3`–`:15` 仅关联成功运行编号，未保存版本化评测集/逐项结果。157的成功运行规则已实现，但没有证明原目标被取消。 | 安装/发布前置记录应绑定准确源码/配置、评测集版本/摘要与结果；真实验收未提供时拒绝正式启用并保持draft。需父审查者判定157是阶段性交付还是替代原目标。 |

缺凭据不是以上代码差异的依据。真实Linux隔离、模型、公司评测/制度、TG与正式安装/发布仍属外部验收；本审查既未执行也未伪造通过。八包保持draft，GAP-13保持开放。

## 搜索与未读范围

使用 `rg --files` 定位，再搜索 `assertRunnable|loadSkills|agent_profile_release_enabled|skill-releases`；在backend/scripts非测试Go/Python/SQL搜索 `INSERT INTO agent_runtime_profiles` 未找到应用安装入口。对上述发布/契约/检查器搜索 `evaluation_set|assessment.*(version|result|set)|评测集` 无结果。不存在的早期猜测文件名已由实际路径更正，不作为缺失证据。

本审查未逐一执行八包业务工具、用例与真实provider，也未审查整个仓库的部署身份运维脚本。结论只覆盖列明的包、发布治理与实际调用链；独立反证可补充其他受控安装/评测记录入口。
