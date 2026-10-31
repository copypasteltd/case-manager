# 增量145：采购／报销逐明细正文起草 / Selected OA line prose

## 交付关系

这是 main 的后续功能源码，结构110，基于 `cea9ec622ad96b3520d6f36b466658de9e1e34f9`。三个冻结候选均保持原标签和原字节；[rc.3 固定交付](../releases/v0.1.0-rc.3/README.md)仍为业务143／结构109／APK144，不包含145明细起草。开发PG仍100，本轮没有更新用户已有数据库或后台服务。

当前23个工作包（14 P0／9 P1）仍active，5项移出范围不记通过，八项Skill仍draft。具体边界见[当前范围](../delivery/04-current-scope-and-acceptance.md)和[完整未完成表](../delivery/05-functional-closure-145.md)。

## 员工使用

1. 在采购或报销详情保存普通业务草稿。填写／调整金额、币种、数量、单价、税费、供应商、凭证和费用日期等仍走业务编辑及审批流程。
2. 进入本人私密秘书会话，项目范围必须与该业务一致。选择“协助业务申请”运行配置→“选择业务申请操作”→采购或报销→准确业务→“起草业务正文”。
3. 可选用途、备注和已存在的逐条明细说明。同名明细按原顺序编号展示，提交参数绑定实际明细UUID，而非名称或数组位置。切换业务或版本后恢复默认备注，避免携带上一份明细选择。
4. 填写操作说明并把准确操作加入本次请求，再发送。本次授权固定业务ID、版本、修订ID、内容摘要和所选字段。秘书读取准确原文后，只能保存一份不可变建议。
5. 任务以draft结束后，界面逐项展示原文及建议。本人填写采用依据并勾选全部核对，再点击“采用正文草稿”。服务端重新核对会话、当前人、当前业务权限、AI资料政策、全部来源和版本；成功生成普通业务新修订及真实操作收据。
6. 另行核对完整业务、准备申请和送审。采用正文不创建审批、不下单、不记账、不付款。业务已变化时重新选择当前版本并发起新任务，不自动重放旧建议到新版本。

真实使用依赖独立安装的准确 oa-assist1.4.0 配置和可用Codex运行环境。本地受控MCP示例不代表实际模型已部署；GAP-09／10／13保持开放。

## 字段和边界

| 业务 | 可选字段 | 单项字符上限 |
| --- | --- | --- |
| 合同 | renewal_terms、termination_terms、notes | 20000 |
| 采购／报销 | purpose | 4000 |
| 采购／报销 | notes | 20000 |
| 采购／报销 | line:<小写UUID>:description，必须来自准确原快照中的一条已有明细 | 1000 |

上限按Unicode码点计；最多100条已有明细加用途、备注，共102项，字段必须唯一。用途和明细说明首尾空白统一去除。工具调用总JSON仍限制64KiB，准确授权资料仍限制512KiB；任何总量／字段超限均整次拒绝，不静默截断或保存部分成功前缀。大量长说明应由员工减少本次选择后重新起草。

明细ID、顺序、未选择明细、数量、单位、单价、申报金额、税费、折扣、币种、日期、类别、供应商、凭证、缺票及重复票说明、资金来源等全部从原快照取得。模型或采用接口不能提供这些字段。正文里的付款指令仅作为待核对文字；界面通过React／Taro普通文本渲染，不能执行HTML、脚本、工具或付款。

## 接口与实现

沿用现有接口，没有新增模型采用工具：

- `POST /v1/conversations/{id}/runs`，`oa_request.tool=draft_business_text`，`arguments.fields`可包含准确明细键。Go在创建任务前检查字段语法及其在准确业务快照中的唯一存在性。
- `read_resource(type=agent_oa_request,id=run_id)`读取授权原文；MCP `draft_business_text(run_id,intent_sha256,fields)`须完整返回恰好所选键。运行器／MCP／OpenAPI分别约束结构，业务服务与PG再次限制准确选择和单项长度。
- `GET /v1/runs/{id}`返回不可变建议、来源及真实采用结果；`POST /v1/runs/{id}/oa-text-adoption`仍只接受建议摘要、本人核对和原因，金额／明细数组不是该接口输入。
- 采用时解码可信原Content，只按准确ID替换description，并调用现有spend.Update。业务写入、普通修订、幂等、审计、事件和采用收据在同一事务内完成。收据失败回滚全部业务改动。

后端主要在 `app/backend/internal/agent/oa_text.go`、`oa_requests.go`，普通财务事实仍由spend服务处理。共享 `app/packages/contracts/src/oa-text.ts`统一字段选择、名称、原文、采用校验和结果比对；Web与Taro三端共用，采用结果必须匹配仅替换授权正文后的完整原Content。

## 结构与安装

普通结构升级新增 `0110_oa_line_prose.sql`，不改1–109的已应用字节，不改旧建议、意图或采用历史。新增数据库字段限制函数和意图插入触发器；扩展既有建议触发器，继续要求运行中准确授权和正式人工命令收据。旧的顶层正文建议和重放保持兼容。

使用现有原生部署流程正常升级至110，重建Web／两小程序，按正式安装流程重新计算并安装oa-assist1.4.0准确摘要。不能将旧运行配置摘要标为新包已安装；全部八项包仍draft。Android本轮仅更新共享源码、定向逻辑与类型检查；最新可下载APK仍144／业务143／结构109，使用145需后续单独重建，正式Android发布仍GAP-16。

仓库现有 `python app/scripts/run_backend.py`、`python app/scripts/dev_database.py`及[部署手册](../operations/01-deployment-and-operations.md)继续使用。本轮不启动Docker、WSL、虚拟机、模拟器或真实设备，不执行生产数据迁移／容灾及网络性能专项。

## 验证和媒体

见[本轮定向验证及保留记录](../quality/145-oa-line-prose-verification.md)。只执行相关HTTP／PG、MCP、共享Web／移动逻辑、Skill运行器、类型、Web与两小程序构建和实际Web交互。完整回归、Taro H5额外旅程、新APK和正式外部接入继续延期／开放。

[原文／建议](../media/screenshots/oa-line-prose-web-before.png) · [真实采用](../media/screenshots/oa-line-prose-web-adopted.png) · [逐明细选择](../media/screenshots/oa-line-prose-web-selection.png) · [实际无声录屏MP4](../media/videos/oa-line-prose-web.mp4) · [六秒截图导览GIF](../media/gifs/oa-line-prose-web.gif)

## English operating notes

On an editable purchase or reimbursement draft, open an owned private secretary conversation in the exact same project scope. Select the business, choose business prose and explicitly select the existing line descriptions, purpose or notes. Identical descriptions retain separate line identities. Send the fixed request, compare original/proposed prose, then explicitly review and adopt it yourself.

The model saves an immutable suggestion and returns draft. Adoption uses current ordinary business update authority and the exact original version, producing one business revision and a durable receipt. Every other commercial fact and unselected line is copied from the original snapshot. Approval, order fulfillment, ledger and cash commands remain separate. Unknown lines, wrong-kind or commercial fields, duplicates, stale bases, changed authority, cancellation and missing/overlong output are refused; receipt failures roll back the business update.

There are at most102 selected fields, including100 existing descriptions. Line descriptions allow1000 Unicode code points; purpose4000; notes/contract clauses20000. The64KiB whole-tool-call bound still rejects oversized output atomically. Schema110 is an ordinary upgrade; old migrations and immutable records retain their bytes. oa-assist1.4.0 remains draft and needs a new exact installation digest and actual model acceptance.

This is main145 source, not a retagged release. rc.3/APK144 retain business143/schema109; all three freezes and downloads remain immutable. Current selected proofs, earlier failed schema compilation, retained full regression and deferred APK/external acceptance are disclosed independently. All23 work packages remain active. GitHub Releases API account403 remains the existing limitation; previously published SSH download assets remain available.
