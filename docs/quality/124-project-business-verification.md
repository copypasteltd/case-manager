# Increment124 · 项目业务与移动订阅检查 / Project business verification

本轮以六类项目业务连续新建和共享移动订阅／账期操作为范围，直接更新应用实现。[设计和使用](../development/124-project-business-journeys.md)列出操作、权限、财务口径和文本草稿约束；[当前范围](../delivery/04-current-scope-and-acceptance.md)继续保留23个开放工作包与5项移出。

## 实现与实际检查

| 编号 | 内容 | 结果与边界 |
|---|---|---|
| PB124-01 | 项目六类业务入口 | Web及共享移动合同、采购、报销、费用、资金、订阅的范围查询、新建、保存和返回；新建锁定项目与最低分类，关闭／归档／取消或缺权时停止新建。 |
| PB124-02 | 当前能力与隔离 | 人类会话业务能力接口核对组织／用户权限版本、项目、读取与创建权限；不授予后续写权限。未知／其他组织／额外查询和只读用户场景实际拒绝。 |
| PB124-03 | 列表口径修复 | 合同／采购／报销公司过滤真正生效；费用／资金项目草稿可查。实际项目份额查询、金额报表及项目发生支出保持原口径，混用范围／游标拒绝。 |
| PB124-04 | 移动订阅管理 | 新建、修订、生命周期、供应方核实、调价、生成／异常账期、归档／恢复及修订索引；账期费用关联、重估、人工核对、免付／作废／重开与固定历史。类型／构建及既有服务端订阅集成套件通过；本轮浏览器实际执行新建及账期免付，未把每条新增移动命令都宣称已实际点击通过。 |
| PB124-05 | 原生 HTTP／PostgreSQL | 六个选定顶层套件、94条结果、零失败／跳过：[backend.json](evidence/project-business/backend.json)。新建六类项目A／B／公司记录，核对范围、版本、分页隔离、关闭状态、权限及真实PG金额；保留原订阅、费用、资金与OA回归。不是全后端回归。 |
| PB124-06 | Web 与共享移动端 | Web339、共享移动434项；全工作区类型检查、Web／微信／抖音编译和两项原生JS基础测试通过。新增范围／回执校验及订阅文本输入、深复制、日历、准确小数、草稿白名单与隔离用例。 |
| PB124-07 | Android JavaScript | 当前源码经已发布的四Worker原生Metro／Windows文件系统适配编译，保持与现有原生宿主相同的no-Expo／no-unimodules源映射检查：[native-bundle.json](evidence/project-business/native-bundle.json)。另从实际源映射逐份核对订阅管理、账期页面、项目业务契约与当前源内容一致（仅换行归一）：[android-js-content.json](evidence/project-business/android-js-content.json)。只编译当前JS，无新APK、签名、Gradle或设备验收声明。 |
| PB124-08 | 实际页面 | Web及Taro H5真实登录后，分别点击六类项目新建、保存、返回固定项目列表，并用真实API核对新对象、项目、分类、版本和金额。移动账期经本人MFA登记免付，保留原预测、固定历史及空实际费用事实。见[Web](evidence/project-business-web/result.json)／[移动](evidence/project-business-mobile/result.json)。 |
| PB124-09 | 媒体 | 6张实际截图、2段静音MP4、2个截图导览GIF，逐项摘要及视觉复核见[media.json](evidence/project-business/media.json)／[visual-review.json](evidence/project-business/visual-review.json)。MP4仅录制六类新建；移动免付在录屏结束后操作，第三张截图／GIF展示结果，无MFA录入。Web入口截图保留权限加载瞬间，后续六类入口与实际新建由录像和检查证明。 |
| PB124-10 | 自有资源 | 检查后关闭全部自建浏览器上下文、页面、临时服务、子进程及合成数据库。开发API／通知／报表Worker更新为当前后端，继续schema94；原PostgreSQL实例、旧业务历史及旧结构文件保留。 |

Taro H5与三类移动平台共用页面源码，使用真实原生API和隔离合成数据。它不建立微信／抖音身份、应用审核、正式Android产物、真实OSS／R2、Codex／Skill／TG、Linux隔离或生产验收结论。没有启用真机、模拟器、虚拟化、实网120GB专项、网络性能、历史数据迁移／容灾或AI费用控制。

## 当前依据与保留证据

| 记录 | 内容 |
|---|---|
| [执行计划](evidence/project-business/verification-plan.json) | 九条明确检查命令及输入；当前适配脚本及复用原生构建工具、当前源码和浏览器组分别绑定 |
| [最终源码检查](evidence/project-business/final-source-check.json) | 当前检查、实际页面、媒体与视觉复核、当前服务、清理状态及旧发布字节核对 |
| [源码清单](evidence/project-business/source-manifest.json) | 当前应用及选定工具逐文件摘要；浏览器分别保存完整捕获输入清单 |
| [旧证据保留](evidence/project-business/reused-evidence.json) | 发布提交`821a6e856d248187f925d7b24a045ca89941aef7`下全部原证据／媒体按原字节保存；121完整回归、runtime56项与Android开发APK保留原提交绑定 |
| [开发更新](evidence/project-business/development-update.json) | 原生API／通知／报表Worker来源更新、原历史事实前后一致；没有新增结构升级或重启PG |
| [只读资源检查](evidence/project-business/development-project-resources.json) | schema94、当前来源／归属、旧权限和数据保护、外部集成关闭、临时数据库零个 |
| [范围检查](evidence/project-business/scope-check.json) | 23 active：14 P0／9 P1；5 excluded，原范围证据保留 |
| [验收适用性](evidence/project-business/acceptance-applicability-check.json) | 原30 AC／72 TC仍逐项分类；分类不执行或通过业务验收 |
| [仓库发布检查](evidence/project-business/repository-check.json) | 中英文入口、源码／说明、相对链接、敏感资料与受影响文件检查；最终暂存字节另行只读核对 |

## 保留的修正过程

首次后端测试仍期待“未分配但归属该项目的资金草稿”从概览消失，与本轮明确的记录卡片行为不符；已修改期待，金额断言保持。关闭项目测试改用没有未结业务的新项目，以保留正常结项保护。公司范围财务查询仍只计算实际分摊，因此未分摊草稿不伪装公司发生额；普通查询与项目资料游标互换返回实际协议错误。旧失败依据保留在[尝试01](evidence/project-business/attempts/01-capture-route-and-overview-test-adjustment/attempt.json)。

浏览器脚本先后修正业务路由录制白名单及页面正文选择器，原失败和关闭结果保留在[尝试01](evidence/project-business/attempts/01-capture-route-and-overview-test-adjustment/attempt.json)与[尝试02](evidence/project-business/attempts/02-checker-page-text-selection/attempt.json)。检查脚本依赖清单在测试期间修改过，94条结果虽通过仍没有作为最终依据；[尝试03](evidence/project-business/attempts/03-verification-plan-source-change/attempt.json)保留来源不一致结果。

常规双Worker Metro在六分钟以上未推进，仅终止已核对归属的该进程树，失败记录保留在[尝试04](evidence/project-business/attempts/04-two-worker-metro-stall/attempt.json)。单Worker及收敛环境仍出现停滞，早期“继承npx环境”的猜测经检查被排除，未当作已确认原因。最终只复用增量121已发布的四Worker Metro、生成目录排除和Windows原生文件系统兼容，调用其bundle适配函数，不调用旧main、Gradle或旧证据写入。保留准确配置、原生resolver、minification、源映射与no-Expo检查；没有替换源文件或生成APK。诊断见[尝试05](evidence/project-business/attempts/05-native-cli-inherited-environment/attempt.json)与[尝试06](evidence/project-business/attempts/06-unadapted-native-filesystem/attempt.json)。更改计划／帮助脚本后，九项最终检查重新绑定固定输入。

开发只读检查曾与合成测试库并行，观察到临时库时正确保持未通过；原诊断保留，所有页面及测试结束后重新核对零临时库。不会把运行中的检查库误作遗留库删除。

另一次Web检查在并行构建时出现五个5000ms超时，应用来源未变化、没有业务断言失败。原记录与并行资源观察保留在[尝试07](evidence/project-business/attempts/07-concurrent-resource-observations/attempt.json)，最终按原测试条件单独重跑。JS检查包装器随后补齐Windows无窗口子进程的显式标准输出／错误句柄，恢复与原生构建器一致的实际编译日志；[尝试08](evidence/project-business/attempts/08-native-standard-handles/attempt.json)保留先前静默进程停止及旧来源记录。没有将停止、超时或来源变化的运行计为最终通过。

## 可重复检查

```text
python app/scripts/verify_scoped_increment.py --plan docs/quality/evidence/project-business/verification-plan.json --run backend
python app/scripts/verify_scoped_increment.py --plan docs/quality/evidence/project-business/verification-plan.json --run clients
python app/scripts/run_workbench_browser.py --feature project-business
python app/scripts/run_workbench_browser.py --feature project-business --mobile
python app/scripts/encode_workbench_media.py --feature project-business
python app/scripts/verify_project_business_native_js.py
python app/scripts/verify_project_business_development.py
python app/scripts/verify_scoped_increment.py
python app/scripts/verify_current_scope.py
python app/scripts/verify_acceptance_applicability.py
python app/scripts/verify_repository_publication.py
```

使用已安装原生Node22、Go、PostgreSQL、Edge与FFmpeg；依赖、原始录制帧、日志、凭据及数据库留在Git外。已存在的记录先保留再执行，浏览器步骤顺序运行；全部关闭后再做开发资源检查。录制／编码完成后查看真实输出，视觉复核不得仅靠命令退出码。

## 继续开放

GAP-01／02：从任务、交付、OA审批与履行到最终结项的完整旅程、公司实际角色和审批制度；GAP-03：财务制度、退款费用／合同异币归属、汇差及完整财务旅程；GAP-18：新增命令的逐项四端正常／失败／冲突业务验收、正式文件引用联动及批量生命周期。移动订阅目前展示修订索引，完整历史详情仍沿用Web入口。

所有23个开放工作包的稳定编号、内容和完成标准列在[剩余事项表](../delivery/03-goal-gap-assessment.md)。本轮功能子项与本地通过不等同整个工作包或生产发布完成。

## English verification scope

Current checks bind application inputs to six selected native HTTP/PostgreSQL suites (94 result records),339 Web and434 portable mobile tests, workspace types, Web/WeChat/Douyin builds, native JS primitives and the published four-worker Windows-adapted Android JS bundle with the original no-Expo map guard. Real Web/H5 flows create all six project business drafts and verify exact scope/version/classification and unchanged incurred totals. The mobile browser also executes a fresh-MFA period waiver and verifies immutable history with no invented actual cost.

MP4s contain genuine six-module draft creation, including navigation/loading states. The period waiver occurs after recording to exclude MFA inputs; its resulting state is in the mobile screenshot and GIF tour. Six PNGs, both MP4 sample sequences and all three stages of each GIF were visually inspected. No every-frame, physical-device, cloud/model/platform/production or current APK acceptance is claimed. Earlier attempts, the two-worker stall and source-superseded results remain explicit; prior121 full regression/runtime/APK retain their original source scope. All23 packages and five exclusions remain recorded.
