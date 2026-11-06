# 159 · 完整业务草稿资料与准确修订 / Complete business preparation and exact revisions

本轮依照用户要求，在任何测试之前完成C01–C06全部应用、四端入口、迁移、Skill约束和用例代码，再统一检查、集中修复并只重跑受影响项。完整23项登记见[本轮差异表](../delivery/05-functional-closure-159.md)；代码冻结清单见[声明清单](159-batch-code-plan.json)。

## 用户流程

1. 在本人创建的私人秘书会话选择当前OA助手。完整新建支持合同、采购、报销和订阅；点击「整理完整…资料」，复用普通业务表单，填写商业条款、金额、行明细、固定版本凭证、付款计划或订阅日期和价格。
2. 点击「预览本次授权资料」。服务端依普通业务规则整理类型化资料，返回全部规范化字段；此时不保存普通业务、不生成执行授权或操作回执。完整核对金额、文本、条目、引用和操作依据，明确加入本次请求。
3. 修改现有草稿时，先选择当前普通记录，固定版本、修订、内容摘要、项目版本及资料分类。能发现记录不代表可修改；实际还须具备普通修改权和AI处理许可。已生效或不可编辑记录拒绝进入本流程。
4. 若目标的项目或分类与会话不同，额外要求组织级agent.plan.approve和agent.plan.commit，并由本人单独确认此准确对象范围；这不扩大其他工具的会话资料范围，也不转移业务归属。
5. 使用完整表单准备修改，查看完整前后差异和依据，单独确认后加入请求。发送时和实际执行时都核对原登录会话的近期MFA、当前权限、所有原有及新选引用。
6. 秘书仅能调用固定run_id＋intent_sha256的revise_business_draft。它不能替换金额、条目或目标，不能送审、激活订阅、生成费用或付款。保存后返回普通业务新修订、操作标识和实际记录入口；审批、履行、费用及现金仍分别办理。

## 数据和执行约束

正常追加迁移121，不修改1–120。agent_business_revision_intents固定原始本人输入、规范化完整参数、准确旧版本、范围指纹和原登录会话；agent_business_revision_commits追加普通原子更新的不可变回执。数据库检查运行状态、独占写授权、私人受众、原会话及普通操作结果。普通更新、秘书回执及审计在同一事务内，失败回滚全部；重试同一固定意图复用原操作。

完整参数JSON上限64KiB，差异展开有明确层级／字段数上限；超限整体拒绝，不截断后接受。更大资料在普通业务页办理。公司会话与准确目标所涉及分类均须明确允许AI处理，生产初始策略继续默认拒绝。

每次准备和执行重新检查业务阶段、版本、原修订、内容摘要、项目版本、当前权限及附件／往来方引用。跨范围确认只授权这一个目标，不允许新增其他对象。取消、期限、会话撤销或MFA过期停止执行。完成后引用继续依赖当前权限及准确新头；后续普通修改或业务状态改变会使旧运行的完整源读取失效，历史回执保持不可变。

四端复用现有完整业务编辑器的资料捕获模式，预览回调不调用普通保存。Web准备表单独立渲染，隔离外层秘书请求的提交事件，避免预览误触发请求。异步结果绑定会话、选择和页面生命周期，过期结果不加入请求。小程序保持既定查阅／审批定位，120GB上传仍属Web/Android；本轮未改变上传范围。

## 接口与运行器

- GET /v1/conversations/{conversation}/business-revisions/{kind}/{resource}：准确当前依据。
- POST /v1/conversations/{conversation}/business-revisions/prepare：只读准备完整修订。
- POST /v1/conversations/{conversation}/business-drafts/prepare：只读准备完整新建。
- POST普通runs：携带唯一business_revision授权；MCP revise_business_draft只接收固定运行与意图摘要。

准备接口保留身份、CSRF、契约、额外参数拒绝及拒绝审计；不要求写操作幂等键，因为不保存业务。最终run和普通变更仍使用现有幂等、审计及原子执行机制。

oa-assist1.6.0升级为L2，但仍draft；共同输入／输出契约增加agent_business_revision，使全部8个包摘要变化，实际安装须重新核验。其他包仍draft。工具定义、运行器白名单、预期工具集、后端约束及生成契约同步更新。

## 交付边界

目标schema121；开发PG100及现有API不升级。全部8个冻结版和旧证据不变。rc.8／APK154仍业务153／schema113，不包含155–159。本轮无新APK，158记录的Android JS索引超时保留，未重跑；真实平台／签名、云、Codex、TG、目标Linux和正式验收仍开放。真实配置继续按用户「后续提供」处理；不重新引入真机、网络性能、历史迁移／容灾或AI费用控制门槛。

Local native HTTP/PostgreSQL and controlled MCP checks establish bounded implementation behavior only. They do not establish real model execution, production deployment, company policy adoption or formal platform acceptance. Current checks, failures, repairs and source bindings are recorded in the [verification report](../quality/159-batch-functional-verification.md).
