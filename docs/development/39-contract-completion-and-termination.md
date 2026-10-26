# 合同完成、终止与未结依据

日期：2026-10-24。状态：本地后端／Web已实现，完整回归、真实Web/API链和开发运行时已验证，见第6—7节。依据[OA-08](../requirements/04-workflows-and-oa.md)、[合同签署生效](25-contract-signature-and-activation.md)及[节点财务协议](38-contract-financial-reconciliation.md)。本章补齐法律生命周期的人工登记；采购差异审批、完整四端及真实银行/云/真机继续分别实施。

## 1. 操作与依据

仅生效合同允许登记完成或终止。操作者须独立的`contract.close`授权、实际人工会话和最近10分钟MFA；新增角色默认不绑定。固定合同版本、生效修订、财务头版本、当前来源集合指纹、实际发生日期、原因、1—20项正式凭证和本人对条款/材料的核实说明。系统记录人的判断，不输出法律结论。

完成必须具有明确的收付计划或明确无收付款；所有计划节点金额无差额、无未归属退款、当前依据已核对且条件满足；关联的实际费用须有完整有效结算；未终结补充协议须先处理。不能通过“确认全部”越过这些检查。

终止可以保留尚未结清的事实，但须明确确认未结清单并记录具体后续处理说明。存在仍生效的补充协议时先处理该协议，父合同不自动级联终止。未知计划、差额、未结费用和待处理补充协议持续显示，终止不代表付款或退款已经发生。

## 2. 不可变记录与后续财务

迁移`0040_contract_closure.sql`增加`contract_closures`及`contract_closure_files`：每合同至多一份不可变结束记录，保存准确生效修订、结束后的合同版本、财务头版本、完整JSON依据及其指纹、发生日期、核实/处置说明、人员、实际会话、MFA时间和凭证数量。固定引用登记为`contract_closure`，保留所有既有引用类型。应用仅能读取和新增两张事实表，不能更新或删除。

合同头与事实、固定文件、审计、outbox和幂等回执同事务提交。延迟约束同时核对合同阶段／版本／生效修订和凭证完整数量：禁止只有结束头而没有事实，或只有事实而合同未结束。合同商业内容和签署件不变，已结束的法律阶段不能普通编辑或重开。

快照固定各节点的真实来源与核对状态、关联费用及结算摘要、补充协议状态。来源冲销、新退款、结算分配或来源集合变化均可改变依据，不能只比较合同或财务头版本。历史查阅继续检查所有当前来源与当时引用对象的权限；受限时显示未知，不泄露旧金额或指纹。

完成/终止后仍允许有权员工登记真实的后续财务归属和更正；法律阶段保持原记录，来源变化单独提示“登记后的依据已变化，须跟进”。这些操作不会自动恢复合同生效或删除旧未结事项。

## 3. 接口与闭合契约

本增量实际接入以下3项操作，路径均以`/v1`开头：

| 方法与路径 | 输入／输出与行为 |
| --- | --- |
| GET `/contracts/{id}/closure-preview` | `ContractClosurePreviewResponse`；一致快照下的可见性、准确合同版本、依据、指纹、阻塞原因、完成／终止能力 |
| GET `/contracts/{id}/closure` | `ContractClosureResponse`；单份固定记录、准确凭证及`basis_state=current/changed`；无完整来源权限时404 |
| POST `/contracts/{id}/closure` | `ContractClosureInput`；成功201 `ContractClosureReceiptResponse`，返回合同ID／版本、结束记录ID、动作，不将金额历史写入可重放回执 |

命令固定`expected_version`、生效`revision_id`、`expected_finance_version`和`expected_fingerprint`；包含`action=completed/terminated`、`occurred_on`、`terms_verified`、`attestation`、`acknowledge_unsettled`、`unsettled_disposition`、`evidence_version_ids`和`reason`。核实说明、处置说明各不超过20,000字符，原因不超过200字符；凭证1—20项，版本ID不得重复。发生日期介于固定生效修订起始日和香港当前日期之间，不自动生成发生日期。

Node／Cost／Child／Snapshot／Preview／Input／Receipt／Record均使用独立闭合Schema。金额保持准确十进制字符串，汇总采用报表精度；现有合同节点最多100项，结束依据最多1,000项关联费用和1,000项补充协议，超过范围拒绝，禁止悄悄截断后宣称全部已核对。指纹是数据库JSON规范文本的MD5，用于当前集合变化检测，不作为签名或安全凭据。

同源OpenAPI为210路径、267操作、420个Schema；本次增加的是上述3项真实操作。其余规划路径不能仅凭契约存在被视为可用服务，文档契约／后端嵌入契约／生成TS类型同步维护。

## 4. 权限、锁与来源变化

`contract_closer`只授予`contract.read`与`contract.close`，要求MFA且默认不绑定，不隐含费用或资金读取权。预览在Repeatable Read事务中核对组织、人员与授权版本；按当前合同及全部费用、分配资金、历史归属来源、退款和当时补充协议检查可读范围。明确无收付款、无任何费用关联和财务事件时，不强行要求空财务轴的权限。其他缺少完整来源权限的预览返回`restricted`、空依据／指纹及两个false能力，不能将受限当成无未结事项。

正式命令在查找幂等回执之前再次检查当前授权、真实人工会话及近期MFA。锁顺序为组织／人员授权、组织内合同法律操作串行锁、相关项目、合同、关联费用、相关资金及退款族；固定按ID排序，取得锁后重新核对关联范围和准确版本。签署／生效使用同一法律操作锁；父合同结束和子协议生效并发时，只能有一个成功。数据库延迟约束进一步禁止已结束父合同下仍生效子协议。

财务依据包含当前来源修订／状态、节点当前核对、退款、每项关联费用及其有效结算和分配指纹、补充协议版本／阶段。新退款、来源冲销或另一路费用分配即使未改变合同财务头版本，也使旧预览失效。登记后再次计算当前集合，历史JSON保持固定，差异只改变`basis_state`。

| 情况 | 结果 |
| --- | --- |
| 合同／生效修订版本过期 | 409版本冲突 |
| 当前完整依据或财务版本变化 | 409 `CONTRACT_CLOSURE_BASIS_CHANGED` |
| 仍生效补充协议 | 409 `CONTRACT_EFFECTIVE_SUPPLEMENTS` |
| 未满足完成条件 | 409 `CONTRACT_UNSETTLED_OBLIGATIONS` |
| 未结终止未确认／未说明处理 | 422字段错误 |
| 超出结束依据范围 | 409 `CONTRACT_CLOSURE_SCOPE_LIMIT` |
| 实际会话或MFA不满足 | 当前身份／MFA门禁拒绝，不信任调用方伪造的认证时间 |

SQL同时校验真实会话、准确当前依据、结束动作条件和固定凭证数量；不能直接更新合同头或伪造凭证不完整的关闭事实。应用鉴权、业务事务、SQL约束承担各自检查，不将普通应用数据库权限视为可绕过API的用户授权接口。

## 5. Web界面及后续财务

Web在合同财务之后提供“完成与终止”分区，展示未结清单、实际费用结算和相关补充协议。操作表单固定打开时依据，核实复选项默认不勾选；MFA后由本人再次提交。409/412保留输入，不自动改绑当前快照。风格继承[官网规范](../design/README.md)，明暗主题、五档宽度及嵌套凭证选择分别验证。

终止后仍展示财务归属和真实后续结算能力；已完成／终止合同可以关联符合原规则的实际费用并登记准确资金／退款／归属更正。迁移仅扩展原财务阶段门禁，其余额度、币种、往来方、固定来源、余额预留、MFA与审计约束保留。法律阶段不因这些财务操作被重开，当前依据变化独立警示。详细状态、文案和布局见[页面设计](../design/12-contract-completion-and-termination.md)。

## 6. 定向验证与运行方法

实现入口：[迁移0040](../../app/backend/migrations/0040_contract_closure.sql)、[真实路由](../../app/backend/internal/httpapi/contract_closure.go)、[一致快照读取](../../app/backend/internal/finance/contract_closure_reads.go)、[事务命令](../../app/backend/internal/finance/contract_closure_mutations.go)、[Web分区与表单](../../app/web/src/ContractClosure.tsx)、[PG集成场景](../../app/backend/internal/httpapi/contract_closure_integration_test.go)。

原生执行`python scripts/test_backend.py -run '^TestContractCompletionAndTermination$' -timeout=10m`，18个PG叶场景通过，覆盖：新增角色未绑定／管理员隔离、未结完成拒绝、终止确认、后续资金与费用、完整节点完成、新退款指纹变化、未结关联费用／分配、明确无义务与只读、未知计划、补充协议及生效竞争、必填凭证／日期、双设备、审计原子回滚、固定引用、真实DB会话/MFA、SQL伪造依据／指纹／MFA／缺件、历史ACL与撤权后幂等。

原生执行`python scripts/run_spend_browser.py --contract-closure`，使用独立临时PG、实际Web登录/MFA及两人顺序审批，不调用Docker／WSL／虚拟机／模拟器。[最终Web/API证据](../quality/evidence/contract-closure/result.json)于2026-10-23T23:10:27.135Z通过6条链，362个来源摘要`d5d8b363714ed2aeb532dccabe438fcfb8c741eed91a6719620f91a547a21bd1`，期间未变。覆盖未结终止、结束后资金与历史、完整完成、外部退款冲突保留、补充协议阻断和只读未知；零脚本或服务端错误。

两个表单及五组详情核对双主题320／390／768／1024／1440宽度，旧依据冲突另核对390px深色，代表截图经人工复核。临时角色撤销，所有自有浏览器上下文／标签已关闭，隔离数据库清理退出码0，所属Vite已停止。业务为明确虚构样本及受控15字节正式文件，无真实银行／云／模型／TG／平台／真机调用。

Web26项测试、生产构建和移动类型检查通过；这不代表完整移动合同登记、Android或生产部署已验收。

## 7. 完整回归与开发运行时

0039完整回归已独立归档为[历史记录](../quality/evidence/contract-finance/backend-0039-result.json)及[批次记录](../quality/evidence/contract-finance/backend-0039-batch-progress.json)，避免后续证据覆盖旧来源。[0040完整后端结果](../quality/evidence/contract-closure/backend-0040-result.json)于2026-10-23T23:30:04.051579+00:00通过480条结果、430个叶场景（73单元／357数据库），包括本增量18个PG叶场景，无失败或跳过。全部90个自动发现的顶层测试在12个串行批次中执行，源码摘要`f034c2f701d09f663549f7a99a44146383e41bad9a28aeb6e00969e23d3fb1a9`，各批期间未变、最终复算一致。这是同源完整套件分批验证，不是单一Go测试进程。

[开发运行时](../quality/evidence/contract-closure/development-runtime.json)于2026-10-23T23:16:20.350601+00:00确认40项迁移及摘要与源码一致，15项健康／未认证拒绝探测通过。应用无超级、绕过RLS、DDL、迁移台账读取、结束事实更新／删除权限；新增角色要求MFA且未绑定，三项延迟一致性约束和两项不可变保护存在。仅更新本机开发库及API，没有生产部署或生产worker。

[交付前来源复算](../quality/evidence/contract-closure/final-source-check.json)确认0040交付时的后端和362项浏览器来源仍与各自成功记录相同，两份OpenAPI字节一致。[当时开发Web/API与清理](../quality/evidence/contract-closure/development-web-and-cleanup.json)确认5178页面和就绪代理200、临时测试库为0、浏览器夹具已移除，全部所属浏览器／标签／授权和临时服务已正常清理。虚构业务及受控文件证据不代替真实云、银行、法律、平台或真机验收。

维护命令均从`app/`原生执行：`python scripts/verify_backend_batches.py`完整回归；`python scripts/run_spend_browser.py --contract-closure`真实Web/API链；迁移及重载后执行`python scripts/verify_contract_closure_development.py`；全部测试夹具结束后执行`python scripts/verify_contract_closure_sources.py`复算来源、检查原开发Web代理及临时库清理。已有批进度只能在摘要相同且完整的情况下`--resume`，新源码须归档旧记录后重新开始，不能混用不同来源结果。

0040交付时，采购差异／完成条件仍待实施；差异重审的后续进展见[0041实施协议](40-purchase-variance-approval-and-execution-baselines.md)。采购完成条件、完整财务筛选／导出／预算、统一收入／汇率／会计口径、移动财务和Android宿主、项目／保留／Agent／TG及部署闭环继续实施，项目目标不缩减。
