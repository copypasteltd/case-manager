# Increment126 · 费用付款核销核验 / Cost payment verification

本轮推进GAP-02／03／18的费用到实际资金、部分核销、人工异币与员工垫付闭环。实现与操作见[设计](../development/126-cost-payment-settlement.md)，整个[23项剩余工作](../delivery/03-goal-gap-assessment.md)继续开放，五项移出不变。

| 编号 | 本轮实现 | 检查范围 |
|---|---|---|
| CP126-01 | 准确费用付款入口 | Web与共享微信／抖音／Android费用详情、当前人类权限预览及固定版本／修订／摘要。 |
| CP126-02 | 实际付款与核销 | 普通创建／登记／分配同一PG事务、原费用保持、独立现金与核销额、剩余款、三条资金历史及不可变凭证／审计。 |
| CP126-03 | 异币和员工垫付 | 独立双币、精度与人工汇率；准确已关联批准报销、员工创建人、员工垫付明细与共用余额限额。 |
| CP126-04 | 失败与重放 | 晚期超额完整业务回滚、并发限额、旧费用／报销头、明确事实、来源／范围注入、固定请求体、真实近期MFA与撤权。拒绝安全审计独立保留。 |
| CP126-05 | 共享客户端约束 | 完整组合回执核对、固定路由、金额字符串、白名单移动文字草稿、当前授权与准确依据隔离。 |
| CP126-06 | 当前检查 | 选定原生HTTP／PG业务回归、Web及移动单元、工作区类型、Web／微信／抖音及Android JS编译；具体执行数量以以下源绑定记录为准。 |
| CP126-07 | 实际页面和媒体 | Web与Taro H5各执行供应方部分核销、人工异币及员工垫付，核对原费用／资金／分配／历史／报表；MP4只录首个已验证场景，GIF为三张真实截图导览。 |
| CP126-08 | 清理与同步 | 关闭所有自建浏览器／临时服务／隔离库；原生开发实例保持schema94及旧事实。双语使用说明、更新说明、演示素材与应用代码一起提交。 |

## 可查执行依据

| 记录 | 用途 |
|---|---|
| [计划](evidence/cost-payment/verification-plan.json) | 九条明确命令及九个选定HTTP／PG套件，非全后端回归。 |
| [后端](evidence/cost-payment/backend.json) | 当前选定实际原生API／数据库执行、套件与结果数量。 |
| [Web](evidence/cost-payment/web-tests.json)／[共享移动](evidence/cost-payment/mobile-tests.json) | 单元执行、失败／跳过及来源绑定。 |
| [Android JS](evidence/cost-payment/native-bundle.json)／[准确源码](evidence/cost-payment/android-js-content.json) | 当前四Worker原生Windows编译、源映射与四份实际模块内容。不是新APK／签名／Gradle验收。 |
| [Web页面](evidence/cost-payment-web/result.json)／[共享移动页面](evidence/cost-payment-mobile/result.json) | 实际UI→API→PG及自有资源清理；每端三类场景。 |
| [媒体](evidence/cost-payment/media.json)／[视觉查看](evidence/cost-payment/visual-review.json) | 六PNG、两MP4、两截图导览GIF，逐项摘要及实际查看范围。 |
| [开发更新](evidence/cost-payment/development-update.json)／[只读核对](evidence/cost-payment/development-project-resources.json) | 已核对归属的服务、schema94、旧事实与零遗留检查库。 |
| [源码](evidence/cost-payment/final-source-check.json)／[保留证据](evidence/cost-payment/reused-evidence.json) | 当前执行来源、历史迁移和旧发布证据／媒体原字节。 |
| [范围](evidence/cost-payment/scope-check.json)／[适用性](evidence/cost-payment/acceptance-applicability-check.json) | 23 active／5 excluded，AC30／TC72分类本身不执行用例。 |
| [发布检查](evidence/cost-payment/repository-check.json) | 文档、媒体、敏感资料、链接、来源及候选发布范围。 |

## 重复检查

```text
python app/scripts/verify_scoped_increment.py --plan docs/quality/evidence/cost-payment/verification-plan.json --run backend
python app/scripts/verify_scoped_increment.py --plan docs/quality/evidence/cost-payment/verification-plan.json --run clients
python app/scripts/run_workbench_browser.py --feature cost-payment
python app/scripts/run_workbench_browser.py --feature cost-payment --mobile
python app/scripts/encode_workbench_media.py --feature cost-payment
python app/scripts/verify_project_business_native_js.py --feature cost-payment
python app/scripts/verify_project_business_development.py --feature cost-payment
python app/scripts/verify_scoped_increment.py
python app/scripts/verify_current_scope.py
python app/scripts/verify_acceptance_applicability.py
python app/scripts/verify_repository_publication.py
```

已有完成结果先保留，再执行新尝试。原生Node22、Go、PG、Edge及FFmpeg；不启用本地虚拟化或模拟器。原始帧、凭据和完整私有日志不入Git。旧121完整回归、runtime及开发APK与125证据保留原绑定，不能改称126。

新组合回归首次将独立拒绝审计计入“成功业务应全部回滚”的总数，导致测试失败。已改为核对成功业务审计不增加，同时保留拒绝审计；应用权限及事务规则没有放宽。测试编译期字段／类型和移动选择器类型问题也在最终源绑定执行前修正。没有把早期结果写成最终通过。

## English evidence scope

Current checks are explicitly scoped to nine selected native HTTP/PostgreSQL suites, complete configured Web/mobile unit suites, workspace types and Web/WeChat/Douyin/Android JS compilation. Actual Web and Taro H5 flows independently exercise partial supplier cash, reviewed foreign cash and an exact approved employee advance. Screenshots use synthetic records; silent MP4 records only the first post-MFA interaction, and GIF is a disclosed three-screenshot tour. No credentials are recorded. Retained121 full/runtime/APK and published125 evidence keep their original source scope. All23 packages, five exclusions and formal platform/cloud/model/production acceptance remain explicit.

## 保留的早期尝试 / Retained attempts

- [attempt01](evidence/cost-payment/attempts/01-journal-kind/README.md)：首次页面已成功登记，检查器把既有历史类型写错；保留失败，修正检查器后再检查。
- [attempt02](evidence/cost-payment/attempts/02-before-web-outcome-and-owned-temp/README.md)：保留Web结果不明确保护修复前的执行／页面记录，以及四Worker Android JS的900秒失败。未将旧Web成功记录视为最终Web源码验收。
- [attempt03](evidence/cost-payment/attempts/03-local-check-timeouts/README.md)：最终源码未变，Web两条既有用例超过原5000ms限制；第二次Web预览服务启动超过就绪期限，未产生页面结果。记录失败与完整自有资源清理，保持原断言／期限再执行。

组合付款请求在断线、服务端失败或不可解析回执后可能已经落库。Web将这类情况标为结果待核对，阻止编辑来源和请求体再发起另一笔；该Web表单只允许关闭并按原永久资金来源标识查阅，不提供表单内重试。后端相同请求的幂等重放继续保留；移动沿用现有未知结果流程。此保护的新增单元断言与最终源码一起执行。

源码核对读取改为最多8个并发文件、每批8个，仍按原文件顺序计算`文件名 + NUL + 原字节 + NUL`。全部1200文件／14169633字节摘要与此前串行执行一致，显式子集顺序和替换字节语义也核对一致，见[等价检查](evidence/cost-payment/digest-read-equivalence.json)。命令、覆盖文件、前后摘要核对、断言和超时未放宽。

The source verifier now reads at most eight independent files per batch and hashes the exact original canonical order. Its full1200-file digest equals the earlier serial execution, including explicit-subset and replacement-byte semantics. This reduces repeated native file-open latency without dropping source coverage or changing command deadlines or assertions.

最终原生后端原命令重跑：九套件、140条通过记录，零失败／跳过；Web352／352、共享移动438／438及工作区类型检查通过。此前失败的隔离库已按准确OID、名称、本轮创建时间及零会话核对后清理，见[清理记录](evidence/cost-payment/attempt03-fixture-cleanup.json)；开发服务只读检查确认schema94、零临时库、集成关闭及旧事实保留。构建和最终总检查以各独立记录为准。

本轮共享H5的三类成功页面记录在Web结果不明确保护修复前产生；修复仅涉及Web消费者，没有改变H5页面、共享契约或后端。保留其原始全构建摘要与执行时间，并按页面自身准确源码清单逐字核对当前文件，不把旧的更宽构建摘要改写成当前全源码摘要。Web页面在修复后重新执行成功。Android最终编译使用计划中归属本仓库、未进入监视目录的临时目录；四Worker、原生文件桥、真实源码映射、禁止Expo映射与900秒期限均保持，早期超时保留而不归因于未经证明的单一原因。

The successful shared-H5 capture keeps its original execution time and broad build digest. A later Web-only uncertain-outcome repair did not change its actual mobile consumers, shared contracts or backend; its exact browser source manifest is checked against current bytes. Web was rerun after that repair. The final Android bundle uses the plan's owned ignored temporary directory, with four workers, native filesystem access, actual source maps, the no-Expo guard and the900-second deadline unchanged. The earlier timeout is retained without assigning an unproven sole cause.

最终九条计划命令全部通过，Android JS记录耗时817.526秒，构建仍使用四Worker及原900秒期限，源码映射1108份来源中核对本轮四个实际消费者。未新建APK。构建期间只读预读本机已安装依赖59877个文件／380550773字节，未更改字节、解析结果或构建期限；这属于本机已有缓存环境的编译证据，不声明冷启动／性能结果。Web与共享H5媒体共六PNG、两MP4及两三图导览GIF，实际查看范围按视觉记录；两端检查及隔离数据库均已清理。

All nine planned commands passed. The Android JS record reports817.526 seconds, four workers and the unchanged900-second deadline; four current consumers were verified within1108 source-map entries. No APK was created. A concurrent read-only prefetch read59877 real installed dependency files (380550773 bytes), without altering bytes, resolution or deadlines. This is a local compilation with filesystem cache, not a cold-start or performance claim. All ten media artifacts retain their hashes and disclosed visual-review scope.
