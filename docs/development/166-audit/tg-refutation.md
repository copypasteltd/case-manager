# TG-OUT-02／07 — write-grant 提示链的独立反证

审查基线：`ddf6a70cc2de5fba5d01f64d581c4a228052c1d7`，业务165／结构130。按单需求追踪授权、写入守卫、阶段选择及实际发送；只读源代码，不执行测试、构建、服务、浏览器或真实消息，不读取凭据。此记录不代替TG、模型或生产验收。

规范来源：[64-telegram-delivery.md](../64-telegram-delivery.md) 21行要求准备发送时核对当前授权及已有任务的原会话版本；26行要求范围或认证版本改变阻止后续准备，已经开始的外部请求保留真实回执。22行把自动提示限制为通用阶段及登录链接。[162批次说明](../162-batch-functional-closure.md) 12行另外保留原群结果披露审核。

原候选：`telegram_delivery_authorized`仍只查`telegram_run_links`，0125的write-grant激活记录却只进入`telegram_effective_run_links`，可能使授权版本失效后的grant结果继续发送。

## 裁定

| 编号 | 子断言 | 独立反证裁定 | 影响边界 |
| --- | --- | --- | --- |
| TG-R01 | 新write-grant的progress／result可直接绕过组织版本停止并成功外发 | **宽断言不成立**：当前两个INSERT守卫先拒绝没有legacy link的非空run。不能跳过守卫直接推演发送成功 | 未证明结果提示或业务正文泄露 |
| TG-R02 | write-grant阶段选择与当前数据库交付守卫兼容 | **源码缺口成立，置信度高**：Go读取effective run，数据库origin和current-phase INSERT仍要求legacy run；可产生23514并回滚整次准备 | 正常启用通知的授权任务可能阻断该Bot本轮准备；属功能实现问题，尚未运行复现 |
| TG-R03 | 激活前已提交的无run accepted提示也一定受激活后组织版本变化阻止 | **存在可达的遗漏，置信度高**：既有accepted行不重新INSERT，UPDATE授权使用legacy空链接短路，effective queued阶段仍匹配 | 仅固定“请求已受理”提示与原请求门户链接；不授予门户访问，不证明执行越权 |
| TG-R04 | human handoff及原群reviewed_result共用同一遗漏 | **本候选不成立**：handoff撤回普通入口，reviewed_result有独立当前授权、原登录、精确正文摘要及披露审核 | 不能把普通固定提示问题推广为审核正文外发漏洞 |

所有裁定都是源代码结论；未声称真实Bot接受消息，未把未执行用例记为通过。

## TG-R01／02 — 为什么新阶段先失败

实际调用与约束：

1. [0125 effective view](../../../app/backend/migrations/0125_telegram_write_grants.sql) 486–488行将原只读链接与`telegram_write_activations`合并，保留原legacy收据。[dispatchTelegramGrant](../../../app/backend/internal/agent/telegram_write_grants.go) 283–289行插入activation、把inbox置为dispatched并入普通任务队列，没有插入legacy `telegram_run_links`。
2. [PrepareDelivery](../../../app/backend/internal/telegram/delivery.go) 176–180行使用effective view选择当前run及阶段，209行把该非空run写入新delivery；accepted、progress、result的选择在72–83行。
3. 当前FK已经在[0117](../../../app/backend/migrations/0117_telegram_reviewed_results.sql) 49–50行改为指向`agent_runs`，所以**不能再用0063的原legacy FK否定路径**。真正仍生效的拦截是51–62行的BEFORE INSERT origin守卫：普通非空run必须具有精确legacy link，否则23514。
4. 同文件83–89行的普通INSERT守卫也只LEFT JOIN legacy link，要求`NEW.run_id IS NOT DISTINCT FROM l.run_id`，并以该legacy run计算阶段。即便单独放开origin守卫，此处仍拒绝grant的非空run。
5. Go在[delivery.go](../../../app/backend/internal/telegram/delivery.go) 213–214行遇到INSERT错误立即返回；336–341行只有成功取得prepared消息后才调用transport。因此不能将“authorizer返回true”直接推演为“新的grant结果已经发送”。

在当前ordinary部署权限下，grant激活和legacy原TG会话链接是两个独立来源；不能假定生产数据会人为补造legacy链接来穿过守卫。0125没有替换上述交付函数／守卫，检索范围见末尾。

这也暴露真实功能缺口：未曾创建accepted的grant首次queued提示，以及从queued进入progress／terminal的新阶段，会走非空effective run插入并受阻。准备事务先汇总一批候选再选发送项；该错误可以使该Bot这一轮准备整体回滚。后续轮次如何调度与真实发生频率没有执行验证，不据此声称永久全局阻断。

## TG-R03 — 仍成立的旧accepted行边界

该路径要求正常存在的时序，而不是修改数据库或伪造身份：

| 步骤 | 当前源代码中的具体条件及结果 |
| --- | --- |
| 1 | 合法的`/task project-update authorize:…`已入inbox、尚在awaiting_executor；其grant未激活，没有任何effective run link。启用固定提示的原群／话题、本人绑定和用户认证版本均有效 |
| 2 | 交付worker先运行，并且Bot的共享发送预算尚未到期。`PrepareDelivery`176–209行先创建`run_id=NULL, phase=accepted, state=pending`；221–226行预算未到时**提交事务**。因此该行可以在外部发送前持久存在 |
| 3 | dispatcher随后正常激活已由人预授权的grant，写activation并把inbox改dispatched；agent run暂仍queued。新effective view有run，legacy view仍无对应run |
| 4 | 在下一次准备之前，组织授权版本提高；请求人的`auth_version`、本人绑定和群版本不变。例如[角色分配](../../../app/backend/internal/identity/roles.go) 182–189行给**另一名用户**分配角色：只提高该另一用户认证版本，同时提高本组织授权版本。无需假定当前请求人仍可执行业务；规范明确以授权版本失效停止旧任务 |
| 5 | [0063 authorizer](../../../app/backend/migrations/0063_telegram_delivery.sql) 10–18行仍看legacy link。grant的`l.run_id`为NULL，17行的OR直接满足，未核对activation／grant保存的组织授权版本，也未核对其原人类登录 |
| 6 | [delivery.go](../../../app/backend/internal/telegram/delivery.go) 134–160行的既有行清理使用同一旧authorizer判断allowed，同时用effective run判断阶段。当前仍queued，accepted未过期，既有行既不suppressed也不superseded |
| 7 | 同文件179行发现已有同phase行，故不重新INSERT；前述两个origin／INSERT守卫不会被再次触发。228–234行选择到此accepted行 |
| 8 | 261行把既有行UPDATE到sending；[0117守卫](../../../app/backend/migrations/0117_telegram_reviewed_results.sql) 92–103行保留不可变原身份，UPDATE只重查同一旧authorizer，因此不能补足activation授权版本校验。336–341行随后调用外部发送 |

上述路径必须在run尚queued时发生；若执行worker先把run推进到别的阶段，既有accepted会被归并，随后新阶段又会遇到TG-R02的INSERT错误。它不是“所有grant结果都会发送”的证据。

在生产Go及全部迁移内检索`UPDATE telegram_inbox`，已读到的撤回来源为群／绑定变更、本人手工撤回、dispatcher对尚未派发请求的检查以及human handoff。没有发现组织授权版本变化时自动撤回所有已派发inbox的通用路径。群变更撤回见[channels.go](../../../app/backend/internal/telegram/channels.go) 45–46行，绑定撤回见[bindings.go](../../../app/backend/internal/telegram/bindings.go) 179行；两者都不能替代上述“仅组织版本变化”条件。全部迁移中的organizations触发器检索只发现base_currency政策触发器，不把应用event发布推断为隐含收件箱撤回。

发送内容仍受[portalMessage](../../../app/backend/internal/telegram/delivery.go) 86–100行固定构造：通用中文阶段、配置的门户根地址和原inbox UUID，回原消息／话题，关闭链接预览。**本候选的实际外发边界只是原群中的一次固定受理提示及原请求标识**，没有项目正文、模型回复或任意文本。

门户[ownRequestSQL](../../../app/backend/internal/telegram/delivery_portal.go) 12–28行以当前组织及请求人查询；[telegram service](../../../app/backend/internal/telegram/service.go) 23–38行要求交互登录。因此链接本身不能扩大业务权限。执行侧也独立读取当前组织版本：[execution.go](../../../app/backend/internal/agent/execution.go) 81–93行读取当前版本，119–120行拒绝run保存版本与当前版本不符，132行另调用`telegramExecutionAuthority`；[telegram.go](../../../app/backend/internal/agent/telegram.go) 45–53行转入grant校验；[telegramGrantState](../../../app/backend/internal/agent/telegram_write_grants.go) 188–204行核对grant版本、当前配置及原人类登录。不能把交付提示遗漏描述成任务可继续写业务。

这不是规范已承认的“发送事务提交后网络已开始、无法原子撤回”窗口：上述组织版本变化发生在下一次`PrepareDelivery`之前。准备后才发生变化的实际回执仍应按原有FinishDelivery机制保存，不应为了补此缺口删除或伪造真实发送结果。

## TG-R04 — handoff、原群及正文披露的反证

- [recordTelegramHandoff](../../../app/backend/internal/agent/telegram_handoff.go) 81–86行保存原人类授权并把普通inbox置withdrawn／`human_authorized_handoff`。普通authorizer只接收awaiting_executor／dispatched，不能把此任务的结果当普通result自动播到群。
- [telegramHandoffAuthority](../../../app/backend/internal/agent/telegram_handoff.go) 92–105行核对原用户、当前组织版本、未撤销原登录、原群版本及绑定；任务私有化不等于群披露授权。
- 正文发送使用[0117独立authorizer](../../../app/backend/migrations/0117_telegram_reviewed_results.sql) 12–27行：精确handoff／review／run、完成状态、正文SHA256及字符范围、当前用户与组织版本、原人类session和当前`telegram.handoff.disclose`。当前组／话题和绑定也是固定依据。
- [reviewedResultMessage](../../../app/backend/internal/telegram/reviewed_delivery.go) 14–28行恢复原审核actor，调用[TelegramHandoffResult](../../../app/backend/internal/agent/telegram_disclosure.go) 43–79行再次检查交互登录、本人完成结果及原来源当前可读性；再按准确hash和最多2000字符构造被审核片段。`PrepareDelivery`247–254行对来源变化显式suppressed，随后sending UPDATE再走独立reviewed authorizer。
- reviewed_result INSERT origin例外只接受精确handoff与披露，见0117的53–56行；它没有因为grant进入effective view而自动获得此例外。不能把TG-R03扩展为reviewed_result越权外发。

## 一次性修复范围建议

所有明确代码和前置用例先完成，再统一检查；这里不修改应用或运行任何检查。

1. **作为一项完整迁移修复三处一致性**：普通`telegram_delivery_authorized`、普通origin守卫、普通INSERT current-phase守卫，统一解析精确effective来源，同时保留legacy原TG会话与human reviewed_result的独立语义。只替换Go查询或只改一个SQL函数都不足以闭环。
2. grant分支不能机械地将legacy JOIN换成effective JOIN后仍要求`telegram_conversation_scopes`，因为授权grant来自原人类会话。应核对精确activation／grant／run、本人、原群／话题／绑定及其版本、原授权版本与当前组织／用户版本、原人类会话及明确规定的失效条件；可复用当前grant安全语义，不能凭“有effective run”放行。
3. 既有`run_id=NULL` accepted行保持不可变身份；准备前以其**当前inbox的精确effective来源**重新授权，授权已失效则按普通规则suppressed。不要回填旧行run_id、改写旧事件、重发unknown或把原发送事实清除。
4. 新grant普通阶段在有效时允许精确effective run通过origin及current-phase守卫；不得让任意agent run、跨inbox、跨组织、原人类handoff或未激活grant借此伪造来源。固定portal-only消息、原回复／话题、Bot归属、预算、24小时与五次尝试的既有约束继续生效。
5. 先准备一批前置用例：有效grant首次accepted／progress／result可准备；激活前提交NULL accepted后仍queued时，组织版本N→N+1且请求人版本不变不得新准备；本人版本／session／群／绑定／配置失效；跨grant／inbox伪造；legacy正常与原会话过期；handoff及准确reviewed_result；已开始发送的实际回执／unknown保持不可变。之后按受影响输入统一执行，不推演真实服务商成功。

## 已读范围、搜索和未验证

已读：0060／0063／0117／0125相关迁移及到0130的函数定义检索；TG delivery阶段归并、准备、finish和transport调用；本人门户与reviewed-result构造；原TG派发、write-grant激活／执行校验、human handoff／披露；角色分配和项目成员授权版本写入；64及162的相关规范。

可复查的有界搜索：

```text
rg -n 'FUNCTION (telegram_delivery_authorized|guard_telegram_delivery_origin|guard_telegram_delivery)\(' app/backend/migrations -g '*.sql'
rg -n 'UPDATE telegram_inbox' app/backend/internal app/backend/migrations -g '*.go' -g '*.sql' -g '!**/*_test.go'
rg -n 'TRIGGER.*organizations|ON organizations|ON public.organizations|telegram.*authorization_version|authorization_version.*telegram' app/backend/migrations -g '*.sql'
rg -n 'telegramExecutionAuthority|telegramHandoffAuthority|OrgAuthVersion|authorization_version' app/backend/internal/agent -g '*execution*.go' -g '*sources*.go' -g '!**/*_test.go'
```

第一项只找到0063定义普通authorizer、0063初版guard、0117新版guard及origin，0125–0130没有替换；第二项找到本文逐项界定的有限撤回／派发来源。期间错误的Windows通配路径及猜测文件名查询均未作为缺失证据。

未验证：真实PostgreSQL迁移／触发器执行、并发具体时序、消息预算运行数据、真实TG响应、真实模型执行、真实Linux安装；不读取开发或生产数据库反推成功。最终修复前应由另一位审查者独立挑战上述可达性与影响边界。
