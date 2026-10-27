# 移动费用与资金完整操作 / Mobile expense and cash operations

增量89对应FIN-01—07、FIN-11—12、GAP-02／03／18。实现位于`app/mobile/`，接通已有Go财务接口；费用、资金与预测继续分别核算，原Web能力保留。当前验证以[交付证据](../quality/evidence/mobile-finance-operations/verification.md)记录的实际状态为准，完整四端生产目标继续开放。

Increment89 connects the existing authorized finance APIs to the shared Taro clients. Actual costs, cash facts and forecasts remain separate. The source-bound delivery record establishes the exact local verification scope; it does not establish real-device or production acceptance.

## 1. 用户入口与流程 / Entry points and workflows

| 入口 / Entry | 操作 / Actions | 结果 / Result |
| --- | --- | --- |
| 工作台 → 实际费用 → 起草实际费用 | 项目／公司、分类、永久来源、发生日、原币、类别、供应方固定修订、最多100项分摊和20份固定凭证；备注与原因 | 保存为费用草稿，不计入发生支出 |
| 费用原记录 → 修订／确认 | 保留原凭证引用；确认页核对原额、日期、供应方、分摊和凭证，完成人工声明及本人MFA | 当前修订被正式确认，不能原位改已确认金额 |
| 费用原记录 → 关联批准业务依据 | 项目／公司内的同币已批准采购／报销，或生效应付合同；固定当前修订 | 关联同一个规范费用，不再创建第二项费用 |
| 工作台 → 收付款 → 起草实际资金 | 支出、收入或内部转账；实际香港时间、原币、收付款方／批准报销员工、方式、外部流水、凭证 | 保存资金草稿；没有银行转账行为 |
| 资金原记录 → 登记实际资金 | 核对实际事实、凭证和当前修订，声明并MFA | 记为已登记资金，独立进入现金口径 |
| 已登记原支出 → 分配费用结算 | 同币同供应方的已确认原费用；员工结算另选本人对应批准报销和员工垫付行 | 更新准确费用结算，服务器锁定有效余额 |
| 已登记原支出 → 释放费用分配 | 从真实分页的原分配中选择仍有余额的`allocation`，明确本次释放额 | 新增释放流水，保留原分配历史 |
| 已登记原支出 → 起草收到退款 | 固定原付款、原币、项目／公司及收款对象，输入实际退款时间、金额和本次凭证 | 新建独立退款草稿，不能把退款当作费用冲销 |
| 退款 → 登记／关联退款分配 | 重读原付款版本；选择实际原分配并逐项输入金额，可在登记后继续补准确分配 | 原付款退款额、原分配可用额与费用已结算额保持一致 |
| 费用／资金 → 全额冲销原记录 | 明确真实生效日期／香港时间，声明并MFA | 新建完整反向事实并关联原记录，保留原月份、币种、分摊和凭证 |
| 原记录 → 财务修订／操作历史 | 分页查看当前可读的固定修订与不可变操作记录 | 能核对原日期、原额、固定往来方、分摊、凭证及操作原因 |

员工不能通过修改地址获得操作权限。页面进入时读取本人权限与当前原记录，按钮由服务端`can_*`决定，最终每条命令仍按实际权限、项目阶段、版本、修订及摘要检查。表单选择器只提供当前可读候选；候选为空不意味着业务事实不存在。

Employees open a canonical record, select an allowed action, review its exact current basis, and submit a separately confirmed command. The server remains authoritative for contextual permissions, project state, revisions, source relationships and balances. Empty authorized choices do not establish that no business records exist.

## 2. 金额、日期和依据 / Money, dates and fixed bases

- 金额保持十进制字符串，校验最多14位整数和6位小数，定点比较使用`BigInt`；不转换为JavaScript浮点。币种必须为准确三位代码，启用状态和币种精度由服务端核对，不新增默认汇率或折算。
- 费用日期使用`YYYY-MM-DD`；付款／退款／资金冲销使用香港本地`YYYY-MM-DDTHH:mm:ss`，转换成准确UTC时刻。例：香港2025-02-01 02:00对应UTC2025-01-31 18:00，业务日期仍为2月1日。非法日期、非闰年2月29日及24:00不能自动归一为另一日。
- 费用草稿可暂缺日期、币种、金额或凭证。若已填写分摊，各行须为有效正额且项目／公司范围不重复；确认时服务端要求分摊精确等于原额并具备完整供应方和凭证。
- 项目／资料分类改变时清除依赖旧范围的选择；编辑不改变原项目、分类或永久来源。原凭证当前不可读时默认保留固定版本ID，明确移除才删除引用，不用附件显示列表反写覆盖原引用。
- 普通资金分配固定费用ID、版本、修订和摘要。员工报销另固定批准修订、准确行、原申请员工与供应方，只接受`employee_advance`行，不能将供应商直付解释成员工垫付。
- 反向冲销与真实退款属于不同事实。不能冲销仍有有效结算的费用；先按真实业务解除或更正关联资金。不能把不允许的资金方向、冲销行或未登记草稿当作可分配支出。

All original currencies and accounting facts remain distinct. Fixed source identities, exact decimal comparisons and explicit Hong Kong time conversion prevent silent repricing, cross-currency settlement or shifted accounting dates. The existing backend enforces complete confirmation, concurrency locks and reconciliation invariants.

## 3. 草稿、并发和未知结果 / Drafts, concurrency and uncertain results

新增财务草稿类型复用现有内存草稿预算：15分钟、最多32条、合计100,000字符，并绑定用户、登录纪元、授权版本及准确目标版本／修订／摘要。只允许已声明的可编辑字符串和分摊输入。往来方对象、文件选择、批准依据、分配原记录、MFA、人工核实、凭据、服务端响应及提交体都不进入草稿缓存。退出／身份变化后清除。

恢复文字后须重新选择新固定依据及凭证并重新核实。新建时永久`source_key`在文字草稿中保留，不能因返回页面而给同一经济事项生成另一个来源。编辑使用服务端原来源，原修订／版本变化后不把旧输入直接覆盖新事实。

提交复用现有`useCommand`的单次意图与幂等规则。双击不产生第二命令；403／404／409／412、登录变化、响应丢失或回执不完整时阻止继续提交并保留输入。先刷新原记录、列表与操作历史核对结果，不能换新请求号重发同一事项。

回执核对包含规范ID、准确下一版本、状态、原币、来源、范围、修订及内容。草稿保存还核对实际字段和原凭证集合；冲销读取实际反向记录；分配／释放／退款关联核对真实原分配流水。未能证明本次结果时保持“结果尚未核对”，不展示为成功。

The existing shared memory budget now admits explicitly bounded financial text inputs, while excluding evidence objects, confirmations and secrets. Commands retain a single intent key. Stale or uncertain outcomes stop further writes until actual records and history have been checked; a received HTTP response alone is insufficient.

## 4. 接口与四端边界 / APIs and client boundaries

使用原有`/costs`、`/payments`创建／修订、费用`confirm/reverse/links`、资金`record/reverse/allocations`、分配`release`、`refund-allocations`及两类`revisions/events`接口。业务选择器复用当前项目、往来方、文件固定版本、批准采购／报销／合同及准确分配的分页接口。本增量没有新增迁移、权限词、后台绕过接口或会计政策。

| 客户端 / Client | 本增量能力 / Capability | 验收范围 / Acceptance |
| --- | --- | --- |
| Web | 继续使用原费用、资金与完整报表工作流 | 原证据仅在完整声明来源仍匹配时复用 |
| 微信／抖音 | Taro共享费用／资金表单、选择、人工核实、MFA和原记录历史 | 本机类型／编译与H5真实API交互；平台真机另验 |
| Android | 相同Taro业务与现有原生控件、私有固定文件阅读及CSV交付 | 必须形成当前源码开发包；JVM缓存与新执行分别记录，不代替设备验收 |

不会自动使用第三方付款API、银行账户密钥或供应商扣款能力。凭证来自现有文件管理中的正式固定版本；普通本机附件及120GB原生上传、真实双云与生产资料迁移仍按原文件工作包推进。

The same Taro forms serve the two mini-program targets and Android. This increment changes no finance API, migration or financial policy. Existing formal file versions remain the evidence source. Actual platform/device operation, ordinary local attachments, real large files and cloud integration remain separate acceptance work.

## 5. 代码与验证 / Implementation and verification

- `app/mobile/src/platform/financial-input.ts`：类型、定点金额、香港时间、严格依据、字段序列化、候选与回执校验。
- `app/mobile/src/platform/drafts.ts`：有界财务文字草稿及准确目标绑定。
- `app/mobile/src/pages/finance/editor.tsx`：草稿、项目／公司分摊、固定往来方与凭证、员工及退款入口。
- `app/mobile/src/pages/finance/operations.tsx`：当前权限、人工核实／MFA、确认、关联、登记、分配、释放、退款与冲销。
- `app/mobile/src/pages/finance/history.tsx`：准确财务修订及操作历史分页。
- `app/scripts/run_mobile_finance_browser.py`：拥有隔离原生夹具、Taro H5及浏览器的完整生命周期，启动前冻结完整来源；结束清理全部自建资源。

验证结果、真实素材、失败保留、历史重建和待完成项见[准确交付记录](../quality/evidence/mobile-finance-operations/verification.md)及[媒体目录](../media/README.md)。不将通过编译或合成示例等同于实际公司财务验收、所有OA旅程或完整四端首发。
