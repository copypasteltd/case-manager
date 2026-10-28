# 需求追踪、验收与发布门禁

版本：v1.1-scope99；日期：2026-10-28。原AC-01—30编号保留；适用范围按修订99拆为适用／部分适用／移出，不把移出记成通过。

当前优先依据：[修订99范围](../delivery/04-current-scope-and-acceptance.md)与[机器清单](../delivery/04-current-scope.json)。真机、网络性能、历史数据迁移／容灾和AI费用专项不再是当前质量门；秘书、隔离／授权和四端功能继续。下方旧增量证据保持当时事实和范围，不重新成为当前门槛。

## 1. 追踪矩阵

| 用户/原AC | 细化需求 | 设计文档 | 工作包 | 必需用例 |
| --- | --- | --- | --- | --- |
| U-01/13；AC-30 | 基线规则、NFR-12 | governance/01、development/01 | 全部 | 环境记录、文档静态检查 |
| U-02/03/12/20；AC-25 | PRD-01/05、NFR-09 | engineering、data-dictionary、operations | WP-01/02/13 | TC-OPS-03/04 |
| U-04；AC-01/02 | IAM-01—07 | identity、data-dictionary | WP-02 | TC-IAM-01—07 |
| U-17；AC-03 | PM-01—09、PM-11 | projects、data-dictionary、clients、project-changes-and-budget-revisions、project-closeout-and-archive | WP-04 | TC-PM-01—04、TC-PM-06—07 |
| U-05；AC-09 | PM-10 | projects、clients | WP-05 | TC-PM-05 |
| U-16；AC-04 | OA-01—08 | workflows、data-dictionary | WP-06 | TC-OA-01—05/08 |
| U-16；AC-05/06 | OA-09—12 | workflows、events、[采购差异协议](../development/40-purchase-variance-approval-and-execution-baselines.md)、[执行基线页面](../design/13-purchase-variance-and-execution-baselines.md)、[采购完成协议](../development/41-purchase-completion-and-settlement-review.md)、[完成核对页面](../design/14-purchase-completion-and-settlement-review.md) | WP-07 | TC-OA-01—04/06—08 |
| U-16；AC-07/08 | FIN-01—12 | finance、data-dictionary | WP-08 | TC-FIN-01—07 |
| U-15/19；AC-10/11 | FILE-01—05/09—11、NFR-04 | storage-protocol、clients | WP-03/11 | TC-FILE-01—04/07/08 |
| U-11；AC-12/13/24 | FILE-06/07/13/14 | storage-protocol、recovery | WP-03/13 | TC-FILE-05/06/09 |
| U-08/09；AC-14/15/16 | AI-07—15 | tools-and-skills、storage-protocol | WP-09 | TC-AI-01/03/04/05/08 |
| U-07；AC-17/18 | AI-01—06/19/20 | events、tools-and-skills | WP-01/09 | TC-AI-02/06/07、TC-IAM-04 |
| U-10；AC-19 | AI-16—18 | secretary、events | WP-10 | TC-TG-01—04 |
| U-18；AC-20/21/22 | PRD-02/06、页面矩阵、FILE-03/05 | clients-and-pages、[移动项目协议](../development/45-mobile-project-collaboration.md)、[固定内容协议](../development/46-mobile-fixed-content-reading.md)、[移动业务草稿与送审](../development/47-mobile-business-drafts-and-submission.md)、[移动状态与事件](../development/32-service-status-and-signed-observations.md#10-taro-服务与事件实现) | WP-11/12/14 | TC-MULTI-01—08 |
| U-05；AC-23 | OPS-01—04 | status-nfr、operations | WP-10/13 | TC-OPS-01/02 |
| U-11/12；AC-25/26 | FILE-13/14、NFR-06/09 | backup-migration-recovery | WP-13 | TC-OPS-03—05 |
| U-15；AC-27 | FILE-08 | backup-migration-recovery | WP-13 | TC-FILE-10 |
| U-14；AC-28 | NFR-01—11 | status-nfr、test-strategy | WP-01/14 | 固定性能集、全部高风险回归 |
| U-18；AC-29 | PRD-06、全部首发需求 | 全套 | WP-14 | 全部必需TC与AC |
| 2026-10-23官网风格指令 | 设计体系、组件/跨端约束 | design/01—05 | WP-02/03—12/14 | VD-01—14；与原TC并行 |

路径缩写对应相邻目录文档标题，完整入口见 [总目录](../README.md)。原R-08已按需求AI-07—13细化为八个基础Skill，避免旧“首批三类”与首发OA辅助冲突。

## 2. 当前质量门（修订99）

| 门 | 当前放行证据 | 未满足时 |
| --- | --- | --- |
| G0 立项／接入准备 | 当前适用业务规则、账号／平台资格、服务器／持久PG及必要云／模型／TG配置 | 可先推进独立代码；未完成实际接入不宣称已接通 |
| G1 设计就绪 | 状态／不变量、精确API／工具Schema、正常／失败用例和对应页面 | 未定义的高影响接口继续细化 |
| G2 业务集成 | 项目／OA／文件／财务完整旅程，PG约束、身份／权限、金额和异步正确性 | 适用业务仍需补齐与核验 |
| G3 文件／AI功能 | 小样本上传／续传／校验与设计上限；实际云对象一致性；模型／Skill／TG、隔离／撤权／取消正确；无必配费用策略阻塞 | 不要求实网120GB、性能基准或AI费用预算；现有预算硬门适配仍需实现 |
| G4 四端产物／发布准备 | Web／微信／抖音／Android对应源码、类型／逻辑／API／H5流程、构建和APK／签名／发布配置；明确缓存范围 | 不要求实体设备或模拟器，也不要求2TB迁移／容灾演练 |
| G5 当前范围发布 | 23项适用工作包闭环，适用AC／TC证据、安全处置、真实接入、部署与业务／运维确认 |5项移出及其局部子项不阻塞，不记为测试通过；当前范围仍未自动完成 |

## 3. 发布检查与记录

对当前适用项记录通过／失败／未执行／阻塞／范围移出及证据。保留身份与秘密隔离、MFA／撤权、OA规则、公司财务口径、文件功能／副本／清理、TG披露、AI隔离／工具、四端产物、持久PG、HTTPS、监控及应用发布记录。

不要求真机、网络性能、实网120GB专项、2TB历史导入、PITR／恢复集／容灾演练或AI费用控制；数据库结构升级、普通文件恢复及应用版本回退仍保留。旧30项AC和TC需要按本范围逐项分类，这项整理仍计入GAP-28，不宣称已全部通过。

## 4. 文档与契约校验

新增文档必须在README可找到、所有相对链接可解析、需求/TC/工作包编号一致。OpenAPI检查JSON解析、内部引用、operationId唯一、路径参数、请求必填与实际行为、枚举与需求状态一致；生成器复跑应无差异。

本轮执行结果单独记录 [文档验证记录](03-document-verification.md)。它仅证明文档结构/机器契约的已执行静态检查，不证明业务规则已签核、接口已实现、网络/云服务已实测或产品测试通过。

新增设计用例的详细步骤、工作包衔接和设计门禁见 [设计实现与验收](../design/05-implementation-and-design-qa.md)，本次样板/色板的实际检查见 [设计验证记录](../design/06-design-verification.md)。

TC-MULTI-05的Taro公共页面本地子集已按[移动项目实施](../development/45-mobile-project-collaboration.md)和[独立证据](evidence/mobile-projects/result.json)执行：15条真实API／UI链、60组布局及20项测试／两端编译通过，来源和清理已复算。它保持WP-04／11／12的实际增量状态，不能替代01—03平台真机、G4完整指定端或G5正式发布；视觉复核的一项P3及全部原生／云／生产缺口仍保留。

TC-MULTI-07的公共客户端本地子集按[完整草稿及送审协议](../development/47-mobile-business-drafts-and-submission.md)验证：[8条OA实际UI／API链](evidence/mobile-oa/result.json)、[15条既有项目／移动回归](evidence/mobile-oa/projects-regression/result.json)、两组各60项布局及[45项逻辑测试／两端编译](evidence/mobile-oa/mobile-build.json)通过，[来源／清理复算](evidence/mobile-oa/final-source-check.json)与[8图复核](evidence/mobile-oa/visual-review.json)均匹配。普通本机附件、01—03真机、Android、真实云及完整移动业务继续验收，G4／G5及依赖安全门保持开放；历史证据没有被新范围覆盖。

TC-MULTI-08的本地功能子集由[7条真实状态UI／API链](evidence/mobile-status/result.json)、[8条既有OA回归](evidence/mobile-status/oa-regression/result.json)及[53项逻辑测试／两端编译](evidence/mobile-status/mobile-build.json)验证；[来源与清理复算](evidence/mobile-status/final-source-check.json)通过。覆盖挂起与时钟回拨、原字节验签、迟到历史、维护取消、事件去重／409比较／恢复复盘，以及真实撤权造成的会话失效跳转与重新登录后拒绝访问。原生时钟、01—03真机、实际采集／通知及生产仍待验收；自动结构检查与[截图复核的实际范围](evidence/mobile-status/visual-review.json)分别记录，不扩充为全部端已完成。
