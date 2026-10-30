# 增量121验证 / Actual foreign-currency refund verification

本轮交付的是**未分配、未预留原付款本金的异币实收退款闭环**。费用或合同的异币退款归属仍明确拒绝；没有自动记汇差、自动费用确认或银行付款。完整 GAP-03／18 和其他未完成包保持开放。

This delivery covers actual foreign-currency cash refunds against unallocated, unreserved original principal. Foreign expense/contract attribution is explicitly rejected. No exchange difference, expense or bank transfer is created automatically. Full GAP-03/18 acceptance remains open.

## 当前源码检查 / Current source checks

<!-- FINAL_CHECK_COUNTS -->
当前源码：Go 244个顶层用例、1535条通过记录，覆盖54批；Web 317、共享移动端 414、runtime 56项通过。八项客户端命令均完成，微信／抖音各20条路由构建通过。Web及Taro H5各6组实际操作通过；6张截图、2段实际MP4、2份截图导览GIF已核对。Android开发包构建、18项结构检查与3项功能内容检查通过；26项JVM测试为Gradle缓存复用。92→93正常结构升级保留15组原事实；2份Linux ELF仅交叉构建。全部旧证据及327份旧媒体保持原字节，保留23项开放交付包和5项移出范围。

Current sources: 244 top-level Go cases and 1535 passed records across54 batches; Web 317, shared mobile 414 and runtime 56 pass. All eight client commands complete, including20 routes for each miniapp. Web/Taro H5 each pass six actual interaction groups; six screenshots, two actual MP4 recordings and two screenshot-tour GIFs are reviewed. The development APK builds and passes18 structure plus three feature-content checks;26 JVM results are reused from the Gradle cache. A populated92→93 normal schema upgrade preserves15 original fact groups; two Linux ELF binaries are cross-built only. Prior proof and327 prior media retain exact bytes. All23 open work packages and five exclusions remain traceable.
<!-- /FINAL_CHECK_COUNTS -->

| 编号 | 验证内容 / Verification | 证据 / Evidence | 适用边界 / Limit |
| --- | --- | --- | --- |
| V-01 | 全部发现的 Go 顶层用例及其子用例 / All discovered Go cases | [后端结果](evidence/refund-fx-backend/result.json)、[逐批记录](evidence/refund-fx-backend/batch-progress.json)、[实际模块计划](evidence/refund-fx-backend/package-execution-plan.json)、[原始日志哈希](evidence/refund-fx-backend/raw-log-archive.json) | 本机原生 PostgreSQL；事务和持久化设置保持开启 |
| V-02 | 八项客户端命令及完整输入摘要 / Eight client commands and exact inputs | [命令记录](evidence/refund-fx/client-checks.json)、[Web](evidence/refund-fx/web-build.json)、[移动端](evidence/refund-fx/mobile-build.json)、[runtime](evidence/refund-fx/runtime-build.json) | 微信／抖音各20路由编译，不代表平台审核 |
| V-03 | 原已发布92版账本正常升级至93 / Populated schema92→93 upgrade | [升级结果](evidence/refund-fx/schema-upgrade.json) | 15组原事实比较；不计入当前源码测试数量，不属于历史数据迁移或容灾验收 |
| V-04 | Web人工实际操作 / Actual Web interaction | [Web结果](evidence/refund-fx-web/result.json)、[完整输入](evidence/refund-fx-web/source-manifest.json) | 隔离合成资料、真实 API 与 PG；没有响应替换 |
| V-05 | 共享移动页面实际操作 / Actual shared mobile-page interaction | [Taro H5结果](evidence/refund-fx-mobile/result.json)、[完整输入](evidence/refund-fx-mobile/source-manifest.json) | 浏览器窄视口，不是真机、模拟器或小程序平台验收 |
| V-06 | Android开发包、内容与JVM缓存状态 / Development APK and JVM scope | [构建](evidence/refund-fx/build.json)、[18项结构检查](evidence/refund-fx/apk-inspection.json)、[3项功能内容检查](evidence/refund-fx/android-feature-content.json)、[JVM范围](evidence/refund-fx/android-test-scope.json) | APK未执行；开发签名；真实缓存状态单列，不重复计作新运行 |
| V-07 | Linux平台服务与broker交叉构建 / Two Linux cross builds | [ELF结果](evidence/refund-fx/linux-build.json) | 未在Linux执行，未验证cgroup边界，未部署 |
| V-08 | 已有开发库与所属服务正常升级 / Owned development services | [更新](evidence/refund-fx/development-update.json)、[运行状态](evidence/refund-fx/development-runtime.json)、[只读约束](evidence/refund-fx/development-project-resources.json) | 保留原记录；外部模型、TG、云存储与平台身份集成保持关闭 |
| V-09 | 当前媒体、旧证据字节与源码复核 / Media and exact retained bytes | [编码](evidence/refund-fx/media-encoding.json)、[目视复核](evidence/refund-fx/visual-review.json)、[保留清单](evidence/refund-fx/reused-evidence.json)、[源码清单](evidence/refund-fx/source-archive.json) | 实际截图及静音视口录像；GIF是截图导览；327份旧媒体原字节保留 |
| V-10 | 临时资源释放和最终当前源码核对 / Cleanup and final source binding | [临时数据留存](evidence/refund-fx/test-database-cleanup.json)、[临时PG关闭](evidence/refund-fx/native-cluster-closed.json)、[源码核对](evidence/refund-fx/final-source-check.json) | 只关闭本轮所属浏览器／临时服务／测试库；保留用户其他进程 |
| V-11 | 范围与原验收编号适用性 / Scope and original acceptance applicability | [范围](evidence/refund-fx/scope-check.json)、[适用性](evidence/refund-fx/acceptance-applicability-check.json)、[发布检查](evidence/refund-fx/repository-check.json) | 分类和发布检查不等于全部AC／TC业务通过 |

## 关键行为 / Business and security observations

1. 原付780 HKD、实收100 USD：持久化现金为100 USD、原本金退款为780 HKD；原付款正文摘要保持不变。现金报表按实际币种汇总，不把不同币种直接相加。
2. 原本金、实收币种精度、汇率方向、凭证来源和最终舍入必须一致；包括JPY无小数及超出JavaScript安全整数的金额。前端使用整数运算，后端使用准确有理数及数据库数值约束。
3. 保存草稿不能代替本次人工确认；登记须固定原付款当前版本、退款准确修订与摘要，并有近期MFA。已撤销会话、无权限人员、缺少确认和不正确报价均拒绝。
4. 同币和异币退款共用原付款本金限额。两笔各400 HKD本金并发争用780 HKD原款时仅一笔成功；幂等回放不重复记账。
5. 已分配或已预留原本金不能重复退款；费用释放须经现有明确操作。异币退款无论登记时还是之后都不能添加费用／合同归属；数据库也拒绝越过边界的写入。
6. 全额冲销保留原报价、凭证和历史，新建实际实收币种的相反现金事实，并恢复原本金。审计失败使整个事务回滚。
7. 共享移动端明确显示金额／汇率核对提示。新增回归验证错误能以可供页面显示的安全输入错误返回；没有将任意异常文本开放给用户。

The integration cases exercise exact principal/cash separation, supported-currency precision, one final rounding, current authorization/MFA, concurrency, idempotent replay, database guards, immutable reversal and audit rollback. Shared mobile validation now produces a useful input message while unexpected errors remain concealed.

## 预检与失败现场 / Earlier attempts

先保留旧现场，再修复并执行当前源码检查。以下失败或未完成结果**不计作最终通过**。A-14另保留19个完整通过、且源码摘要与最终输入一致的用例；它们只按原始日志中完成的顶层及子用例复用，不将失败的整个包记为通过：

| 编号 | 早期结果 / Earlier result | 保留记录 / Preserved record |
| --- | --- | --- |
| A-01 | Web对话框与背景重复按钮定位失败 / Ambiguous dialog target | [原现场](evidence/refund-fx-attempts/01-web-dialog-target/preservation.json) |
| A-02 | Web日期输入格式不被浏览器接受 / Datetime input format | [原现场](evidence/refund-fx-attempts/02-web-datetime-input/preservation.json) |
| A-03 | 早期Web通过；旧移动测试编译器未解析共享模块；H5／Metro在修复前主动停止 / Earlier source, compiler failure and interrupted builds | [14项记录](evidence/refund-fx-preliminary/preservation.json) |
| A-04 | 移动端错误汇率被拒绝但提示不明确，随后补回归修复 / Unhelpful mobile validation notice, subsequently repaired | [原现场](evidence/refund-fx-attempts/03-mobile-error-notice/preservation.json) |
| A-05 | 修复移动提示前的Web通过结果，仅对应旧源码 / Earlier Web pass binds earlier inputs only | [原现场](evidence/refund-fx-attempts/04-earlier-web-pass/preservation.json) |
| A-06 | 早期八项客户端命令通过；移动413项；另一Metro在修改前主动停止；开发库只读校验因两条陈旧断言失败 / Earlier clients and stale verifier assertions | [17项记录](evidence/refund-fx-preliminary/02-before-mobile-validation/preservation.json) |
| A-07 | 移动冲销日期字段定位不完整，随后按真实字段名修正 / Mobile reversal field selector | [原现场](evidence/refund-fx-attempts/05-mobile-reversal-field/preservation.json) |
| A-08 | 全量回归发现历史版本契约误要求新字段：8批通过、第9批原结算用例失败；随后修正可选字段并重新执行全部用例 / Full regression exposed a required-field error on legacy revisions | [61项原记录](evidence/refund-fx-attempts/06-history-contract/preservation.json) |
| A-09 | 历史契约修复前的Web／移动6组操作各通过，仅对应之前源码 / Earlier interaction passes bind earlier inputs | [Web原现场](evidence/refund-fx-attempts/07-before-history-fix-web/preservation.json)、[移动原现场](evidence/refund-fx-attempts/08-before-history-fix-mobile/preservation.json) |
| A-10 | 移动冲销成功后，脚本整页重载与Taro跳转竞态；改为等待真实跳转完成 / Capture navigation raced the committed Taro redirect | [原现场](evidence/refund-fx-attempts/09-mobile-commit-navigation/preservation.json) |
| A-11 | Metro文件扫描诊断与构建IO并发调整，均不计完成产物 / Incomplete native filesystem diagnostics | [诊断](evidence/refund-fx-attempts/10-metro-filesystem-diagnostic/preservation.json)、[并发调整前](evidence/refund-fx-attempts/11-metro-io-concurrency/preservation.json) |
| A-12 | 导航脚本修复前的源码归档保留 / Source archive before the capture navigation repair | [保留记录](evidence/refund-fx-attempts/12-before-navigation-source/preservation.json) |
| A-13 | Windows目录链接异步读取停滞；保留兼容处理前现场 / Windows junction-read stall | [原现场](evidence/refund-fx-attempts/13-native-link-bridge/preservation.json) |
| A-14 | 本轮当前源码包累计20分钟超时，订阅审查出现JOB_LEASE_LOST；仅复用其中完整通过的19个顶层用例及其子用例，其余按4个顶层用例分批重跑 / Cumulative package deadline and lease failure | [失败与选择记录](evidence/refund-fx-attempts/14-backend-package-deadline/preservation.json) |
| A-15 | 链接读取后仍有原生元数据回调停滞，保留进一步兼容处理前现场 / Native metadata callback stall | [原现场](evidence/refund-fx-attempts/15-native-metadata-bridge/preservation.json) |
| A-16 | 提前剪枝生成目录前的构建源扫描现场 / Earlier source enumeration | [原现场](evidence/refund-fx-attempts/16-native-source-scan/preservation.json) |
| A-17 | 单进程已扫描61,978份真实文件中的11,155份；在文件系统兼容后改用四个实际工作进程继续构建，未省略源码或哈希 / Serial indexing replaced by four real workers | [原现场](evidence/refund-fx-attempts/17-native-serial-index/preservation.json) |
| A-18 | 第26批业务用例及清理已完成后受控停止；第26批全部重跑，其余批次按已发现的真实所属模块执行 / Controlled boundary stop and exact package selection | [原现场](evidence/refund-fx-attempts/18-package-selection/preservation.json) |
| A-19 | 首次发布扫描发现93号已执行升级脚本末尾空行，以及当时尚未生成的发布结果自链接 / Initial publication formatting and output-link check | [原结果](evidence/refund-fx-attempts/19-publication-format/repository-check.json)、[保留说明](evidence/refund-fx-attempts/19-publication-format/preservation.json) |

源代码归档、原始日志、失败截图和录制帧留在被忽略的本地目录，公开记录保存摘要与状态。当前客户端编译保留原有全部用例；没有通过删除用例、改业务期限或延长会话来取得通过。Android Metro采用披露的四个工作进程，保留同一应用入口、无Expo原生扩展约束和原生构建检查。

Android构建另使用由原Metro配置派生的临时配置，仅排除本机生成目录：`mobile/android/.gradle`、`mobile/android/app/build`、`mobile/android/build`、`mobile/dist`、`mobile/.taro`、`mobile/.swc`。原解析器、源码目录、入口、平台和签名检查均保留。本机打包进程针对仓库路径使用原生同步文件系统结果完成链接、元数据与内容读取，回调仍异步通知；仓库外路径沿用原函数，不替换源码或路径。文件系统线程数为128，派生配置及兼容代码哈希单列。

源码清单枚举已提前跳过原本排除的生成目录。[文件范围核对](evidence/refund-fx/android-source-enumeration.json)确认选取的292个路径与已发布120版完全一致，没有新增排除或遗漏；所有当前文件内容仍重新计算摘要。

Earlier source archives, raw logs and pixels remain private with public hashes and original status. Failed, incomplete and earlier-source outcomes receive no final-source pass credit. A-14 separately retains 19 fully completed current-source cases and their passed children from exact raw logs; the failed package itself is never classified as passed. All existing mobile tests remain present. No business deadline, session lifetime or fixture cleanup period was relaxed.

后端保留前25批的既有通过记录，其中第2批仅按A-14复用19个完整通过的用例；第26批整批重跑。剩余执行从同一源码的完整发现清单选择每个名称实际所属的全部模块，避免反复启动没有匹配用例的模块；全部244个顶层用例仍须逐项通过。`-count=1`、`-p 1`、每个包20分钟期限、原生持久PG、业务期限和会话规则均保持原值。实际命令、保留批次、模块映射及运行脚本摘要另列，不把受控停止的第26批记为通过。

The first25 batches retain their recorded passes against identical sources; batch2 includes only the19 completed cases disclosed in A-14. Batch26 reruns in full. Remaining commands select every package actually containing each selected name from the same complete discovery manifest. All244 top-level cases must still pass. `-count=1`, `-p 1`, the20-minute package deadline, durable native PG, business leases and session rules are unchanged. Actual commands, retained batches, package mappings and runner hash are recorded separately; the stopped batch26 receives no pass credit.

此前超时残留的一份合成测试数据库，先完整保存为私有归档并读取其目录，再核对无活动连接后清理；没有强制终止连接。归档及摘要在关闭临时PG后继续保留，不涉及原开发库、历史业务迁移、还原执行或容灾验收。

One synthetic fixture left by the earlier timeout is retained in a complete private archive with a readable catalog before cleanup, after confirming no active connections. No connection is force-terminated. Its archive and hashes remain after the temporary cluster closes. Development/production databases, historical migration, restoration and DR acceptance are outside this operation.

The four-worker Android wrapper derives its configuration from the original Metro config and excludes only six generated output/cache directories. A Windows host bridge uses actual synchronous filesystem results for checkout-only link, metadata and read operations while callbacks remain asynchronous. Outside-checkout calls retain their original implementation. The resolver, source roots, entry, platform and signature checks stay in effect. Source enumeration now prunes the same old exclusions before traversal and retains exactly292 previously selected paths. No source or path result is fabricated.

## 发布边界 / Release boundary

- 0093升级脚本已在开发库执行，其字节保持不变。发布检查只对该文件固定摘要 `29008d3fce7a9803c19cf9f0ad1d374ed692d9f7ca1b05ff240d4fbd8e9207b0` 的末尾空行作例外，并在结果中列明；行末空格及其他文件沿用原规则。不会通过修改已执行升级脚本或历史校验值来修复格式。
- Git对同一路径单独保留末尾空行约定，发布检查同时固定以上完整摘要；其他路径的Git格式规则保持原值。
- 保留23项active（14 P0／9 P1）、5项excluded及全部28个旧编号；八项Skill继续draft。
- 原30个AC与72个TC的适用性分类保持单独记录。该分类不能代替完整业务执行。
- 不做真机／模拟器、网络性能／实网120GB、历史迁移／容灾、AI费用控制。秘书、TG、授权、隔离、审计及技术资源限制保持在范围内。
- 依赖审计结果按相同锁文件复用，**本轮没有新查漏洞库**；正式产物安全验收和生产就绪仍开放。

All23 work packages remain active, five exclusions remain traceable, and all eight Skills remain draft. Model, Linux isolation, cloud accounts, platform publication, production PG and deployment acceptance remain separate. Retained lock-bound dependency audit data is not a new advisory lookup or a production-security sign-off.

The already applied schema93 bytes remain fixed. Publication permits only the exact-hash EOF blank line disclosed above and reports that exception explicitly. Trailing spaces and all other files retain the ordinary rule; applied migration bytes and historical checksums are preserved.

Git retains the EOF convention only for that migration path; publication additionally pins its complete hash. Git whitespace rules for all other paths remain in effect.

设计与使用见 [增量121](../development/121-actual-foreign-refunds.md)，完整未完成项见 [差距清单](../delivery/03-goal-gap-assessment.md)。
