# 秘书知识草稿与准确内部发布

本增量推进 GAP-11／12／13／18，代码在 `app/`、文档在 `docs/`。秘书可以按员工明确目标生成并保存知识草稿，或发布员工已核对的准确修订。复用原知识业务流程，完整平台仍按 [28项差异表](../delivery/03-goal-gap-assessment.md)推进。

## 1. 用户流程

| 编号 | 员工操作 | 实际业务结果 |
| --- | --- | --- |
| KNOW-A01 | 在本人私密会话选择项目简报任务，指定知识类型、标题、分类、固定附件，并填写写作要求后发送 | `draft_post` 生成有限正文、摘要及标签，通过原知识服务保存真实草稿与首修订 |
| KNOW-A02 | 选择可编辑文档及其准确基础版本，发送修订要求 | 保存新正文修订；已有内部发布版保留，普通读者继续看到原发布版 |
| KNOW-A03 | 阅读完整待发布正文、摘要、标签、分类、附件和修订摘要，勾选确认后另行发送 | `publish_post` 只发布准确修订；不同时修改内容，不发送站外通知 |
| KNOW-A04 | 查询运行、消息和事件进度，进入知识详情 | 区分已受理授权、草稿已保存及已内部发布；读取真实修订与操作号 |

支持原有 `project_log / meeting_note / sop / article` 四种类型。项目和公司资料分别处理，内部发布只改变公司系统中的发布指针。普通只读简报仍可使用，未带知识授权时不能写入。

## 2. 授权与固定依据

`RunCreate.post_write` 为两种互斥输入。每个运行只有一个知识写入意图，与项目／任务、文件和OA操作互斥；历史引用不能重新授权。仅支持 `web / android / wechat / douyin` 的本人真实交互会话，TG暂未接通。

| 分支 | 员工固定的字段 | 模型可提供的字段 |
| --- | --- | --- |
| 新建 `draft_post` | 类型、标题、分类、当前会话项目、最多20个准确文件版本、操作说明与原始写作消息 | 有限正文、摘要、标签 |
| 修订 `draft_post` | 上述字段，加 `post_id / expected_version / base_revision_id / content_hash` | 有限正文、摘要、标签；不改类型、分类或项目 |
| `publish_post` | 四个准确基础字段、说明、完整原文依据及原始消息 | 本次 `run_id / intent_sha256`；没有正文修改参数 |

`Post.content_hash` 根据固定正文修订、项目、类型、分类、标题、正文、摘要、标签及**未过滤**附件计算，先计算摘要再过滤页面不可见的附件。哈希不包含发布指针或当前人的可操作标记，发布同一修订不会改变内容哈希。旧发布版的读者只获得其有权修订的哈希。

授权在接单时绑定原登录会话、员工／组织授权版本、会话修订、安全版本、目标项目基础状态及完整原文。所有工具、执行签发和完成判断重查原登录有效性；模型及公开回执不包含登录会话标识。版本或项目基础变化后停止，不能静默换用最新修订。

## 3. 内容与来源约束

正文和摘要各最多20000个Unicode字符，标签最多20个、每项1—50字符且不重复；标签不能带首尾空白。工具参数总计不超过64KiB，原始授权资料最多512KiB。较长中文正文仍须同时满足字节上限；不将字符数当作字节数。

知识附件只能是准确正式文件版本，不能“跟随当前版”。原文的所有原始附件也逐项检查；失权附件即使被普通页面隐藏，仍阻止秘书处理原文。固定附件须符合原文件可用、扫描、项目、分类和AI处理策略，不接收本地路径、对象键、URL或凭据。

生成内容运行的全部显式来源及后来工具观察的来源，都必须与目标文档同项目，且分类不高于目标文档。公司文档不能吸收项目资料，低分类文档不能承接高分类来源。每次读取输出前及后续写入／完成时重查；不能只限制最终引用而忽略模型已经观察的资料。模型将资料内容视作不可信数据，不从正文中的指令取得发布或其他业务权限。

生成草稿不做自动发布，不变更对象授权或资料分类。更新已发布文档仅产生未发布修订，原 `published_revision_id` 保留。内部发布使用原 `post.publish` 权限、准确基础修订和完整来源检查；归档文档与结项／归档／取消项目继续只读。

## 4. 持久化与事务

迁移 [0057](../../app/backend/migrations/0057_agent_post_writes.sql)增加 `agent_post_intents` 和 `agent_post_commits`。两表仅追加，应用角色只有SELECT／INSERT；六个触发器校验不可改写、原始交互身份、私密会话、准确业务依据、原员工消息、实际正文／发布指针、真实幂等操作和同事务成功工具回执。

知识正文修订、发布指针、审计、Outbox、幂等结果、知识提交和工具回执在同一事务中保存。回执或证据失败整体回滚。内部路由为 `/internal/agent-post/{tool}`，固定键为 `agent-post:{run_id}`。同一运行同参数重试返回原结果；已提交后更换正文不会追加另一修订，员工须另发修订请求。取消保留已提交业务事实，提交前取消不产生文档。

公开运行携带 `post_write` 原始依据和实际结果；`agent_post_write` 来源类型绑定意图摘要。`read_resource` 只分段读取本运行授权及实际结果，`inspect_operation` 返回有限状态。写入工具回执只携带对象、版本、摘要、发布状态及真实操作号，避免完整长正文和原文重复膨胀超过网关输出上限。

## 5. Web、Taro与运行时

Web／Taro秘书页均提供新建草稿、修订及内部发布三种选择；草稿附件取当时明确选中的固定文件版本，修订选择替换附件时保存所示完整列表。写作要求通过原消息输入。选择操作尚未发送；确认发送才建立一次授权，受理回执需与原始意图匹配。

发布预览展示完整正文、摘要、标签、分类、固定文件标识与版本；勾选前不能加入本次请求。发布失败或依据变化后需要重新核对，不能在旧授权内自动修正。真实保存回执可进入原知识详情／历史，任务消息不替代业务事实。

`project-briefing` 源码包1.1.0、L1、仍为draft；共五个工具：`read_resource / search_resources / inspect_operation / draft_post / publish_post`。普通无写入授权的简报按读取凭据核验；带知识意图的运行必须有本运行实际成功写入操作才能报完成。MCP、HTTP工具集合、原生约束证明及运行时来源Schema一致。详见 [技能SOP](../skills/project-briefing/SKILL.md)。

## 6. 验证与复现

[验证记录](../quality/evidence/secretary-post-writes/verification.md)统一记录实际来源和结果。集成验证使用原生PostgreSQL、真实认证HTTP及MCP，原生约束证明为受控依赖，不调用真实模型或云服务。

```powershell
$env:GOFLAGS='-p=1'
$env:GOMAXPROCS='2'
$env:PYTHONUTF8='1'
python app/scripts/test_backend.py --package ./internal/httpapi -run '^TestAgentPostWrites$' -timeout 6m
python app/scripts/verify_backend_batches.py --evidence-group secretary-post-writes --batch-size 20
python app/scripts/verify_web.py secretary-post-writes
python app/scripts/verify_mobile.py secretary-post-writes
python app/scripts/verify_agent_runtime.py secretary-post-writes
python app/scripts/verify_linux.py secretary-post-writes
python app/scripts/run_agent_browser.py --post-writes
python app/scripts/run_mobile_browser.py --post-writes
python app/scripts/verify_secretary_post_writes_development.py
python app/scripts/verify_secretary_post_writes_sources.py
python app/scripts/verify_secretary_post_writes_docs.py
```

两个浏览器验证按顺序运行，共用夹具文件不并发；所有所属浏览器／上下文、临时服务及隔离库都在结束或失败后清理。构建或H5通过不能替代微信、抖音和Android真机。遵守不调度Docker、WSL、虚拟化、模拟器及子Agent的约束。

## 7. 尚未完成

此增量不提供跨项目知识迁移、降低分类、任意外链抓取、更多附件格式转换、自动站外通知或TG群发布。里程碑／阶段工具、OA业务正文自动起草、其余Skill能力、真实模型和原生隔离、真实双云／120GB／2TB、四端真机与生产部署恢复继续按完整目标推进。八个manifest仍draft，没有将源码包版本提升当作正式发布。
