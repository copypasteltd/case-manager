# 增量158：完整代码批次与统一检查 / Increment158: complete code wave and unified checks

本轮承接157，先一次写完C01–C06涉及的应用、接口、四端入口、数据库约束、Skill和测试用例，再开始统一验证。所有23个当前工作包均在[代码批次清单](158-batch-code-plan.json)内逐项映射；“代码存在”“本地证据通过”“公司规则／真实接入／正式验收”分别记录。不能把保留的旧包或单元测试变成完整目标验收。

This round completes the entire declared C01–C06 application/contract/client/constraint/Skill/fixture wave before unified verification. Every one of the23 active packages is mapped in the code plan. Existing code, passing local evidence, company policy adoption and real integration/formal acceptance are independent statuses.

| 编号 / ID | 本轮实现 / Implementation | 使用逻辑 / Usage | 验收边界 / Boundary |
|---|---|---|---|
| C01 | DOCX合同完整正文、二进制/配方/文字投影独立摘要 | 选择准确合同和新DOCX → 秘书按固定范围生成 → 扫描、提升、正式首版本 → 本人核对正文和原件 → 近期MFA后独立采用到合同修订 | 文本投影呈现标题、段落和表格；版式看原件；不支持XLSX/PPTX合同正文采用 |
| C02 | 汇兑预览、独立损益台账、撤销及完整历史 | 实际跨币费用分配 → 固定已发布汇率版本 → 两个事实日期折算 → 人工核对会计口径、MFA登记 → 依据变化时退出有效合计 | 公司会计政策须确认；不改原费用、现金或分配；不自动重估全部公司账目 |
| C03 | 合同／采购／报销／订阅固定资料的新建工具与准确回执 | 本人私聊填写固定草稿 → 选择oa-assist → 新建工具仅传run与intent摘要 → 普通命令原子保存草稿 → 跳转正式业务页继续完善、送审及履行 | 模型不能替换金额、对象、明细或凭证；草稿不是审批、启用订阅、费用或付款 |
| C04 | 已知派生文字盘点、观察计划、执行／取消和永久再提取屏障 | 准确历史原件已完成清理 → 盘点全部文字元数据 → 现行保留政策及MFA建立计划 → 至少24h观察 → 当前依据复核后清理DB正文并保留不可变证据 | 外部派生键没有可信存储原生身份的，继续待核实；不据任意键执行外部删除 |
| C05 | Web与共享Taro四端入口、统一类型与准确回执检查 | Web／Android／微信／抖音使用同一服务端权限及普通业务命令 | 小程序的文件能力仍以查阅／审批为主，不承担大文件或Office编辑上传；开发APK154仍是153／113，不包含155–158 |
| C06 | 全23项代码映射、整批原生／客户端用例与源代码绑定交付工具 | 代码批次冻结 → 后端／客户端统一检查 → 保留首次失败 → 集中修复 → 仅重跑受影响检查 → 当前UI截图、实录、GIF及发布检查 | 选定回归不等于全量回归；合成数据和受控模型／存储不等于真实云、Codex、TG或生产验收 |

## 准确资料与权限 / Exact facts and authority

合同DOCX通过既有有界Office配方生成和扫描／提升／正式提交路径。服务端重新生成固定配方，校验完整OOXML二进制SHA-256与大小，再返回`agent-contract-docx-v1`完整语义投影；投影SHA、配方SHA与原件SHA各有含义。人工采用仍使用准确原件版本和二进制摘要，沿用原合同全部商业事实，写入普通合同修订。合同依据、当前会话或文件权限变化均拒绝继续。

DOCX creation reuses the bounded ordinary Office recipe, scanning, promotion and first-version commit path. The backend regenerates the pinned recipe, verifies the original complete OOXML hash/size, and separately returns its complete semantic projection. Human adoption pins the original file version/binary digest and preserves commercial facts in an ordinary contract revision.

汇兑采用`reviewed_remaining_allocation_v1`：固定一条仍有效的原始跨币费用分配，以同一不可变人工汇率版本分别折算剩余费用单位（费用发生日）和剩余现金单位（实际付款日）。逐笔按基础币精度half-up；账面折算大于结算折算为收益，反之为损失。完整历史最多1000条，超界拒绝不完整合计；依据变化或撤销的记录均不参与有效合计。支持多基础币版本，合计严格按币种分列。此为明确人工采用的独立核对台账，不制造公司会计政策。

FX journals fix one remaining dual-unit root allocation and one immutable manually published quote book. Carrying cost uses the incurred date; settlement cash uses its actual paid date. Each value uses base-currency precision and half-up rounding. Positive carrying-minus-settlement is gain; the inverse is loss. Complete bounded history includes changed/revoked bases but excludes them from active totals, separately by base currency.

秘书业务新建记录原始本人会话、会话安全版本、权限版本和全部固定类型化资料。普通合同／采购／报销服务执行实际创建；订阅仅在准确持久化intent、运行中oa-assist及原本人有效会话下允许相同普通创建命令。任何其他AI调用仍拒绝。全部引用和新业务范围在送入模型之前及正式写入时重新核对。执行成功绑定普通幂等操作及业务修订，重复调用复用原回执。

Business drafts bind the original human session, conversation/security/auth versions and full typed payload. Ordinary business services perform the actual creation. The subscription service admits an AI caller only for the exact durable intent, running oa-assist task and still-valid original human session. All other AI calls remain denied. References and scope are rechecked before model disclosure and writing; repeat calls reuse the immutable ordinary operation.

派生清理仅清空已知DB文字正文；原文字摘要、字符／页数、提取算法与引擎信息留在不可变计划中。原件须由普通已完成history清理计划证实为purged，不能是当前正式版本；活动保留、正在提取、政策停用、文件或资料变化会阻止执行。每个计划有至少24h观察和7天执行窗口。执行／取消决定与文字清空、审计在同一事务中；执行后不能改写、删除证据或重新提取该版本。新计划不绕过原件清理或未知外部对象的核实。

Known DB-derived plaintext is cleared only after ordinary historical-original retirement completed. Immutable plans retain the old text digest, count, extractor and native-engine/page evidence. Current heads, holds, queued extraction, disabled policy or changed facts block execution. Plans observe at least24h and have a7-day execution window. Decision, clearing and audit are atomic; permanent barriers prevent evidence rewriting or text recreation.

## 数据库、客户端与技能 / Database, clients and Skills

正常追加迁移118–120：独立汇兑台账、固定业务草稿intent与普通回执、派生文字观察及清理。迁移1–117、全部历史证明／素材与8个冻结tag保持不变。开发PG100／既有API未升级或重启；使用当前后端前须对实际目标执行正常升级。无需Docker、WSL或模拟器。

Normal append-only migrations118–120 add independent FX journals, fixed business-draft intents/receipts, and derived-text retirement. Migrations1–117, earlier proofs/media and8 frozen tags remain unchanged. Existing developmentPG100/API stay untouched. Upgrade the intended target normally before using this backend; no Docker, WSL or emulator is used.

`file-update1.7.0`、`oa-assist1.5.0`和其余6包全部保持draft。共享引用schema改变全部8包摘要；实际安装与发布需逐包重新核验，不沿用旧摘要的评估或安装证明。新MCP工具为`company/create_business_draft`；不会授予shell、审批、费用登记或付款权限。

All8 Skills remain draft, including file-update1.7.0 and oa-assist1.5.0. The shared reference schema changes every package digest, so real installation/release requires dedicated reverification of all8. The new MCP tool cannot acquire shell, approval, expense or payment authority.

## 本轮检查证据 / Current evidence

执行记录集中在`docs/quality/evidence/batch-round-158/`及各实际浏览器捕获目录；首次接口生成的schema归属错误已保留，随后在代码实现阶段修正。统一代码记录的`tests_executed:false`只证明代码批次先完成；实际结果以之后的不可变检查记录和最终校验为准。

Current immutable execution records live under the round158 evidence directories. The initial schema-generation ownership error is retained and was corrected during implementation. The first code-wave record deliberately says tests_executed:false; actual outcomes are established only by subsequent execution records and the final source check.


## 固定开发轮次规则 / Fixed development-round protocol

1. 从完整23项清单核对所有本轮可实现事项；公司规则、真实配置和明确不支持内容单独保留，不能用默认商业事实代填。
2. 一次完成全部声明编号的后端、数据库约束、四端、接口、Skill和相关测试用例；代码波次全部标记完成后才允许开始统一检查。
3. 统一检查收集全部首次失败，再集中修复；已有失败记录、原证据与旧冻结产物不覆写。修复后仅复查受影响集合。
4. 分别核对真实页面、准确业务回执、截图／录屏／GIF和资源关闭，再同步双语指南、完整差异、更新说明及发布来源。

All declared numbered code is completed before unified checks. Batch debugging preserves first failures and repeats affected checks. Client input manifests are independent of backend-only changes; actual browser captures bind their own complete reviewed inputs. Unprovided company rules/configuration and unsupported behavior remain explicit open work.


当前构建边界 / Current build limit：Web、微信、抖音构建成功；Android JS本机依赖索引900秒超时，按已有加速及选择性验证延期授权保留失败、暂缓重跑。四端源码、移动类型与563项用例保留；没有当前Android JS成功或新APK声明。详见[验证与延期](../quality/158-batch-functional-verification.md)。


## 主要接口 / Main interfaces

完整字段、枚举与错误模型以[当前OpenAPI](openapi.json)、共享生成类型和服务端严格解码为准。所有写入使用普通命令幂等、当前版本／资料摘要和审计；可见资料不自动成为可写授权。

| 接口 / Interface | 行为 / Behavior |
|---|---|
| `POST /v1/conversations/{id}/runs` + `business_draft` | 本人固定四种完整普通Create资料；返回标准化准确intent，模型工具仅提交run/hash |
| MCP `company/create_business_draft` | 复用普通创建服务；返回原子操作ID、普通对象／修订／摘要和准确原始内容；重复调用复用结果 |
| `GET /v1/payments/{id}/fx-differences` | 完整有界历史、有效／依据变化／撤销状态及按基础币分别合计 |
| `GET /v1/payments/{id}/fx-differences/basis` | 显式allocation_id与fx_book_id，返回固定依据JSON及SHA，不能自动挑商业事实 |
| `POST /v1/payments/{id}/fx-differences`、`POST .../{journal}/revoke` | 近期MFA、准确当前版本与reviewed；独立登记或不可变撤销，不改费用、现金及分配 |
| `GET/POST /v1/files/{file}/versions/{version}/derived-retirement` | 当前普通文件保留权限、完整元数据／阻止原因／历史，或明确建立观察计划 |
| `POST /v1/derived-retirement-plans/{id}/execute`、`.../cancel` | 重新核对原件、政策、资料与人类MFA；锁普通文件、固定决定、清空已知正文、原子审计 |

The generated contract is authoritative for complete fields. Human command receipts establish actual ordinary writes; read/queue acceptance, a draft or a model summary cannot establish approval, contract effectiveness, payment, recovery or verified cleanup.
