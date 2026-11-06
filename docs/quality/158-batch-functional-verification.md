# 增量158统一验证与边界 / Increment158 unified verification and limits

本轮先完成C01–C06全部应用、四端入口、数据库约束、Skill、接口和测试用例，然后执行统一检查；不是逐模块开发、调测、发布。23个当前工作包均有实现位置及余项映射，见[批次计划](../development/158-batch-code-plan.json)与[完整差异](../delivery/05-functional-closure-158.md)。

The complete declared application/contract/client/constraint/Skill/fixture wave precedes unified checks. All23 active packages remain individually mapped. A package with local application code is not automatically an accepted company workflow or real integration.

## 检查方式 / Method

原生Windows工具；临时原生PostgreSQL隔离合成资料；真实HTTP、MCP边界和普通业务命令；受控执行器与对象存储替身。开发PG100/API不改，不使用Docker、WSL、虚拟机或模拟器。浏览器演示使用真实本地Web/API/PG，录制认证后的实际页面像素，排除凭据、MFA资料和真实业务记录。

Native Windows tools and disposable native PG execute actual HTTP/MCP/ordinary application commands with synthetic records and controlled execution/storage doubles. No real cloud, model or provider acceptance is inferred. The existing development database/API are preserved.

## 首次检查及集中修复 / Initial findings and batch fixes

| 编号 | 首次发现 | 集中修复与证据 |
|---|---|---|
| F01 | 后端合同草稿预检给事务专用函数传入了较窄的查询接口，导致编译失败 | 接受原调用已经传入的真实事务；保留native-tests-initial.json |
| F02 | Web组合TypeScript项目未列出4个新共享输入文件 | 一次补齐明确输入；首批502项Web、共享移动548＋15及运行器62项通过，类型检查失败仍保留，构建当时未执行 |
| F03 | 全批审阅补充订阅工具的活动运行配置及任务未过期检查 | 同批修复，避免用已停用配置执行固定草稿 |
| F04 | 全批审阅发现文字派生清理SQL使用固定Web渠道 | 改为当前经过验证的人类会话渠道，保留四端原权限 |

[F01–F04修复记录](evidence/batch-round-158/batch-fixes-2.json)。接口生成曾选错源枚举位置，在写出文件前失败；保留[生成首次记录](evidence/batch-round-158/code-generation-initial.json)，修正后重新生成全部接口类型。测试日志、源码摘要、退出状态和首批失败分别保留，不把编译失败计为业务测试执行。

批次3修复旧Office场景留下的排队任务、普通财务角色和时间字段校验；原始Office关联用例通过，后续两项继续暴露问题。批次4证明原生MFA和文件目录范围已修正，进入实际工具写入及清理观察检查。批次5继续按整批发现集中修复：不可变计划仅授予SELECT/INSERT，改用现有普通文件锁串行化；订阅夹具使用完整合法输入；仅在合成MCP失败后按同一个原子命令键读取底层诊断，不向模型或公开接口披露数据库细节。所有成功路径仍执行实际MCP调用。

补充记录：[批次3](evidence/batch-round-158/batch-fixes-3.json) · [批次4](evidence/batch-round-158/batch-fixes-4.json) · [批次5](evidence/batch-round-158/batch-fixes-5.json)。保留[批次3完整来源清单](evidence/batch-round-158/native-batch-3-source-inventory.json)，后续仅复查发生变化的业务草稿／文字清理流程，其他已执行记录继续标注其原出处。

客户端检查独立绑定完整Web／移动／共享接口／运行器／Skill／原生构建适配器输入；后端专用SQL或夹具修正不伪造新的客户端执行，也不迫使重建未变化的客户端。全批代码冻结仍绑定全部应用源码。实际浏览器／原生夹具捕获另外绑定当时完整相关源码，并逐次记录资源关闭。

已知文字清理在批次5通过；业务草稿的实际回执写入发现SQL白名单缺项（`23514 agent_tool_receipts_tool_check`），导致整个工具事务回滚，仍按失败保留。首次白名单补丁的保护断言误认为旧定义止于112，而实际最新定义在114，因此该补丁没有写出；批次6重复了旧状态，不计作修复成功。后续以实际最新白名单为准，只追加新命令，保持旧迁移与旧值完整。

白名单最终以114真实定义为准，普通迁移119仅追加固定草稿工具。批次7已实际完成四种草稿的保存／重复调用／任务完成／业务读回，但其清理回调错误停止已经完成的工作而失败；批次8仅修正该夹具清理，不放宽生产任务租约。

补充：[白名单修复](evidence/batch-round-158/batch-fixes-7.json) · [完成任务夹具修复](evidence/batch-round-158/batch-fixes-8.json)。

Android JS依赖索引在900秒限制后超时（实际步骤903.699秒），完整10步记录仍是failed；另外9步成功，Web／微信／抖音构建及类型／业务用例通过。遵照用户已授权的选择性验证延期和加速收口，本轮暂缓重跑该项，保留原失败、编译输入和[延期处置](evidence/batch-round-158/client-scope-disposition.json)。确认当前仓库中该Metro／jest-worker进程为0。当前4端源码保留，但只有3项当前客户端构建成功；不声明Android JS成功，不提供新APK，不用旧APK154替代158功能。

The original10-step client record remains failed because its Android JS dependency indexing exceeded900seconds. Nine steps passed. This explicitly deferred validation is separate from functionality and formal acceptance, following the user's acceleration/selected-validation direction. No new Android JS success or APK is claimed.

四组初次实际Web捕获均因共用录制器的旧路由白名单而失败；对应四组原生业务夹具均正常结束，浏览器上下文、临时Web/API和隔离PG均关闭。保留初次捕获记录及[修正前工具原文](evidence/batch-round-158/browser-before-fix-1/)；[统一修正](evidence/batch-round-158/browser-fixes-2.json)先进入准确资料页，再开始录制，并仅加入普通付款UUID路径。凭据／MFA／私有能力遮罩和查询限制继续保留，不变更业务代码。

## 当前结果 / Current results

| 项目 / Check | 实际结果 / Actual result | 范围 / Limit |
|---|---|---|
| 完整代码先行 | 首次代码冻结早于全部统一检查；后续每批修正均先固定源码 | 23个工作包有实现位置与开放验收映射，不等于23项验收完成 |
| 当前受影响原生HTTP／PG | batch8通过：2个顶层流程、4种草稿子场景；45条测试／包结果记录 | 45不是45个独立业务用例；关联旧用例保留batch3原出处 |
| Web | 502项通过 | 完整当前Web常规脚本；没有真实生产数据 |
| 共享移动 | 240＋308＋15＝563项通过 | 包含新8项及既有7项关注用例；非真机 |
| 运行器／Skill | 62项、目录与类型检查通过 | 全部8项Skill仍draft，未接真实Codex |
| 当前客户端构建 | Web、微信、抖音通过 | Android JS900秒索引超时，完整客户端batch记录仍failed，按明确处置延期；没有新APK |
| 实际页面／录屏／GIF | 四组通过；6张截图、4段无声MP4、4个截图导览GIF | [实际素材与来源](../media/batch-round-158/README.md)；前三组保留第二次通过记录，仅重跑第四组准确版本定位 |
| 保留旧冻／服务 | 8个tag、旧迁移与证据保持；开发PG100/API保留 | 当前目标结构120，须对实际目标正常升级；不声称生产部署 |

提交前范围检查首次发现总差异表仍保留旧余项文字；保留[首次范围检查](evidence/batch-round-158/scope-check-initial.json)，将总表的23个余项／验收列与结构化清单同步后复查，不涉及应用修改。

每次应用修正后先固定完整源码；失败记录保留，仅复查受影响集合。客户端批次不把失败改写为通过；最终发布检查如通过，只表示上述明确限定的源码／证据范围符合交付规则。

The final source/publication proof verifies this explicitly limited delivery scope. It does not turn the failed Android step or any formal open acceptance into a pass.

第二次统一页面捕获中DOCX、汇兑及普通草稿三组通过；文字清理组因三个版本均有“盘点此版本的派生资料”按钮产生定位歧义。仅修正该分支为“通知对应版本”中的准确按钮，并将回执滚动到可视区域；业务代码不变。保留[修正前控制器](evidence/batch-round-158/browser-before-fix-2/check_batch_round158.mjs.source)及[唯一替换说明](evidence/batch-round-158/browser-fixes-3.json)。第三次文字清理捕获通过，其他三组原通过记录不重播；最终校验以原控制器核对这三份来源清单，并验证其与当前控制器只有该分支的一次准确替换。四组原生夹具退出0，自建上下文、临时Web/API及隔离PG均关闭。

All four actual browser/native journeys pass. The first three keep their successful second-capture records, whose controller bytes are verified against the archived source; only the derived-text selector branch changes before the fourth group's successful retry. Screenshots and GIF frames were visually inspected, and all published media hashes are pinned. This reuse does not imply rerunning the unaffected groups.

## 复用与延期 / Retained and deferred

- 旧8个冻结版本、迁移1–117、全部旧证据和媒体保持原字节；本轮新增普通迁移118–120。
- rc.8／APK154仍是业务153／结构113，不包含155–158。Android当前JS编译不等于新APK或正式签名。
- 保留121完整后端回归、136既有四端证明及154开发APK出处；本轮仅执行注明的选择集合，不声称重跑完整后端回归。
- 真机、网络／实网120GB专项、历史迁移／容灾、AI费用控制继续移出。现有正常权限、AI执行隔离和技术限制、普通结构升级、OSS/R2复制与版本恢复继续保留。
- 公司会计／审批／保留制度、复杂Office与版式、未知外部派生对象、真实服务器／云／Codex／TG／平台配置、8项draft Skill的真实安装验收及正式生产签名仍开放。用户明确真实资料后续提供。

All23 work packages, all8 draft Skills and formal integration/production acceptance remain open. No new frozen release, production deployment or formal Android approval is claimed by this source increment.
