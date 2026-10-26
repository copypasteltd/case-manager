# 分类、AI许可与访问渠道策略发布

日期：2026-10-23。本文落实 IAM-04、分类/渠道独立属性及敏感配置二次认证，关联[身份权限需求](../requirements/02-identity-and-permissions.md)和[秘书资料读取](29-agent-context-and-source-projection.md)。这是已实现的组织配置能力；允许 AI 处理并不表示真实模型已启用或已经获得业务动作权限。

## 1. 策略层级与当前边界

每个组织的 internal、project_restricted、confidential 各有一条当前生效修订。生效规则取同一分类最大的 policy_version；没有规则时默认拒绝。ai_allowed 与 allowed_channels 独立保存，并始终与当前业务权限、项目成员及资源授权相交。

| 分类 | AI 处理 | 渠道修改限制 |
| --- | --- | --- |
| internal | 默认关闭，具有 policy.manage 的人工可显式发布新修订 | 必须保留 Web 与发布人当前管理渠道，防止整体锁定管理入口 |
| project_restricted | 默认关闭，可由有权人工显式启用或撤回 | 可选择 Web/微信/抖音/Android；空集合表示全部拒绝 |
| confidential | 当前不开放启用 AI 的配置路径 | 仅员工入口，准确对象授权仍是独立条件 |

Telegram 不接受通过此发布接口加入。TG 的群白名单、身份与共同披露范围尚未完成，不能仅加一个渠道字符串就开放群内正文。后续独立渠道准入落地后再扩展该发布规则。

发布不会产生新角色、项目成员、资源授权或会话成员。系统管理员能配置策略，仍不能因此查看项目、文件或会话。已有下载、屏幕副本或外部发送不能通过修改策略从接收方设备上撤回。

## 2. 数据修订与迁移

0032_classification_policy_publication.sql 在既有 classification_policies 上增加 published_by、reason、base_policy_id、组织/ID 唯一键和准确引用约束。旧行保持原值；未记录的发布人和原因保留 NULL，不编造迁移作者或历史发布说明。

| 字段 | 含义 |
| --- | --- |
| id / org_id / level | 不可变修订身份及组织/分类范围 |
| policy_version | 同一分类连续递增版本，从 1 开始 |
| ai_allowed | 是否通过分类层允许 AI 处理，不能替代其他权限 |
| allowed_channels | 不重复、合法、规范排序的渠道集合 |
| base_policy_id | 新发布所依据的准确前序修订 |
| published_by / reason | 实际人工发起人与 1—200 字原因 |
| created_at | 数据库记录的发布时间 |

更新或删除历史行由不可变触发器阻断，迁移身份也不能普通 UPDATE 覆盖。应用账号撤销 UPDATE/DELETE，仅保留 SELECT/INSERT。版本连续、前序准确、渠道集合和规范化由数据库触发器校验。

新应用发布必须包含完整作者、原因和前序；数据库同时拒绝新发布取消 internal 的 Web 入口、启用 confidential AI、开放 Telegram。受控迁移/测试补充缺失历史时可保留原有未知出处，但不能伪装为已通过人工发布流程；正常产品接口不提供此旁路。

## 3. 权限、事务与重放

读取和发布均仅允许实际员工交互会话，Agent run 身份被拒绝。权限为 policy.manage；发布另外要求当前数据库会话在最近十分钟内完成 MFA。不能仅相信客户端或 Actor 中的时间戳。

发布命令取得组织独占授权锁，验证发起人和当前会话、MFA、策略管理权限、准确 expected_version/base_policy_id，再保存新修订、组织 authorization_version、审计、outbox、幂等结果。它们在同一事务内提交；审计失败不能留下生效规则或授权版本。

没有内容差异返回 POLICY_UNCHANGED，不制造新版本。并发发布时只有一个准确前序可成功；另一请求返回版本冲突，或在组织权限已变化时返回 AUTHORIZATION_CHANGED。不得自动改用新基础版本重新发布员工尚未核对的输入。

幂等回放之前重新核对实际会话、MFA 和当前管理权限。符合条件时返回同一不可变发布回执，不重复追加规则、审计或组织授权版本。该回执表示当时发布的历史事实，当前规则以最新查询为准。

读取使用同一 Repeatable Read 快照及组织共享授权锁，避免一次响应混合两次配置。历史分页签名绑定组织、用户、渠道、run、用户/组织授权版本与准确分类；另一分类、身份或新授权版本不能使用旧游标。

## 4. API

| 方法和路径 | 请求与响应 |
| --- | --- |
| GET /v1/classification-policies | 返回三个分类的当前修订，items/next_cursor；当前集合无下一页 |
| POST /v1/classification-policies | level、expected_version、base_policy_id、ai_allowed、allowed_channels、reason；201 返回准确新修订 |
| GET /v1/classification-policies/{level}/history | cursor/limit，返回该分类不可变历史；默认 20、最大 100 |

新发布请求的全部字段必填，拒绝额外 actor、身份、权限等字段；渠道去重、分类限制、无内容变化和准确前序由业务层补充校验。读写均使用既有严格 JSON Schema 响应校验，写入继续要求 CSRF 与 Idempotency-Key。

## 5. Web

具有 policy.manage 的员工在“资料分类”进入“资料分类与渠道”。按三个分类分别显示当前版本、秘书处理许可、允许入口、发布时间与发布原因。历史未知原因按未知显示，不使用默认说明冒充历史事实。

“发布策略新修订”固定打开时的准确基础版，填写原因、选择入口和秘书处理许可，再验证当前身份并人工确认。confidential 的 AI 开关禁用，internal 的 Web 入口不可取消。界面仍以服务器规则为准，不把禁用控件当作权限防线。

版本/授权冲突保留当前输入并停止提交，提示关闭后核对新修订。保存成功刷新当前配置与相关查询；不能保留一个旧的成功预览作为新规则下的读取证明。历史窗口按准确修订显示规则、原因和时间，可继续加载更早记录。

风格沿用[品牌应用与文案规范](../design/09-brand-application-and-content-style.md)：暖灰白或深蓝灰画布、清楚的文字/边线、既有墨色主按钮和蓝色强调、普通表单及当前 MFA 组件，不创建新的 AI 主题色。

## 6. 验证与限制

TestClassificationPolicyPublication 的 9 个原生 PostgreSQL 叶场景验证：配置与内容分离、初始默认；实际 MFA/Agent 拒绝；非法/无变化输入；完整发布与幂等回放；准确前序并发；历史不可变与应用出处要求；审计失败全部回滚；历史游标；当前 AI/渠道规则与业务权限相交。

既有上传测试改为追加渠道修订，验证不可变制度下的相同原权限场景，不再通过 UPDATE 覆盖策略。定向回归同时包含秘书共同读取和完整上传控制；全量结果、来源摘要和浏览器链见[实施台账](../delivery/02-implementation-ledger.md)。

[隔离浏览器证据](../quality/evidence/agent-sources/result.json)于2026-10-23T09:47:39.204Z完成6条真实API/UI检查链，12个来源文件摘要为 `ee42d822a9cd6360fe3e0380858cd6f6c133c9b813a45745be576e8ee376acbb`，运行期间来源未变。覆盖配置与业务隔离、共享已发布/个人草稿、固定历史文件、第二设备策略冲突、实际MFA发布/历史、撤回AI许可、成员变更与旧安全版本拒绝；双主题五宽度通过。全部浏览器关闭、隔离数据库清理退出码0、所属Vite停止。1440深色正向预览及390深色策略表单已人工复核；弹窗截图采用当前视口，避免整页捕获把固定元素绘制到非实际位置。未访问真实模型或云原件。

本次不自动启用任何实际组织的 AI 策略。正向许可、共同阅读及撤回检查使用显式隔离的原生测试数据库与虚构公司资料。模型/服务部署、平台渠道资格、Telegram 共同披露、真实文件提取及生产配置审批仍需分别完成。
