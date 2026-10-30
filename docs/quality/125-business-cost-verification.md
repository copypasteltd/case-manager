# Increment125 · 批准业务费用核验 / Business expense verification

本轮交付三类准确批准业务的实际费用登记，覆盖Web和共享微信／抖音／Android页面。实际行为、接口和事务约束见[设计与使用](../development/125-business-cost-recording.md)；[当前范围](../delivery/04-current-scope-and-acceptance.md)继续保留23个开放工作包与5项移出。

## 实现与检查

| 编号 | 完成本轮内容 | 实际依据与边界 |
|---|---|---|
| BC125-01 | 三类业务入口 | 已批准采购／报销及有效应付合同，重新取得当前头版本、批准修订及摘要，服务固定费用范围／分类／币种。预览不预留金额或创建费用。 |
| BC125-02 | 一次确认与来源关联 | 原生事务组合普通费用创建、确认及来源关联；正常三条费用历史、正式凭证引用、四条审计与Outbox共同提交。任一步失败全部回滚。 |
| BC125-03 | 准确事实与冲突 | 独立实际日期／金额／供应方／正式凭证／精确分摊、两个明确核对及真实近期MFA。并发来源上限、晚期失败、旧执行依据、未知来源／范围注入、重复提交、请求体改变、只读身份、伪造MFA及Agent身份均实际核验。 |
| BC125-04 | Web与共享移动 | Web345项、移动436项通过，零失败／跳过。新增固定依据、精确小数、事实不预填、回执全量集合、固定路由及草稿权限／版本隔离用例。 |
| BC125-05 | 原生HTTP／PG | 七个选定顶层套件，115条通过结果、零失败／跳过：新组合费用、普通费用／资金／合同财务、OA审批、项目新建及采购履行。不是全后端回归。 |
| BC125-06 | 类型与构建 | 工作区类型、React／Vite、微信、抖音、原生JS基础检查及当前Android JS编译分别留档；JS源映射核对实际费用编辑、操作、业务详情和共享契约四份当前源码。无新APK／签名／Gradle或设备验收声明。 |
| BC125-07 | 实际Web／H5 | 两个客户端各通过真实页面登记合同／采购／报销费用，核对服务端固定凭证、25.01实际金额、20.00项目／5.01公司份额及三条历史。报表总额75.03、项目60.00、公司15.03与实际PG一致。每端四项场景观察，不把所有异常都称为页面点击验收。 |
| BC125-08 | 实际媒体 | 6张真实PNG、2段静音MP4、2个三张截图导览GIF。MP4只录制首个合同来源在本人MFA完成后的提交、结果和历史；另外两类来源由实际页面检查记录证明。GIF每张2秒，为明确标注的截图导览。 |
| BC125-09 | 自有资源与开发实例 | 检查后关闭自建浏览器上下文／页面、临时服务和隔离数据库。仅更新已核对归属的开发API、通知及报表Worker，保留原PG实例、schema94及旧业务／财务事实；外部集成保持关闭。 |

## 可查依据

| 记录 | 用途 |
|---|---|
| [执行计划](evidence/business-cost/verification-plan.json) | 九条明确命令、选定套件、最低数量和准确范围 |
| [后端](evidence/business-cost/backend.json) | 七个原生HTTP／PG套件、115条结果，绑定当前来源 |
| [Web](evidence/business-cost/web-tests.json)／[移动](evidence/business-cost/mobile-tests.json) | 345／436项实际执行结果 |
| [Android JS](evidence/business-cost/native-bundle.json)／[映射内容](evidence/business-cost/android-js-content.json) | 原生四Worker Metro Windows适配、no-Expo检查及四份当前源码内容核对 |
| [Web页面](evidence/business-cost-web/result.json)／[共享移动页面](evidence/business-cost-mobile/result.json) | 真实登录、本人验证、三种业务、准确事实与报表，含来源及清理结果 |
| [媒体](evidence/business-cost/media.json)／[视觉复核](evidence/business-cost/visual-review.json) | 逐项摘要、真实视口／截图来源及实际视觉查看范围 |
| [开发更新](evidence/business-cost/development-update.json)／[只读检查](evidence/business-cost/development-project-resources.json) | 自有进程、旧历史、当前来源、schema94、零遗留检查库、外部集成关闭 |
| [当前源码](evidence/business-cost/final-source-check.json)／[保留证据](evidence/business-cost/reused-evidence.json) | 当前执行与媒体、清理、原迁移及已发布证据／媒体原字节 |
| [范围](evidence/business-cost/scope-check.json)／[适用性](evidence/business-cost/acceptance-applicability-check.json) | 23 active／5 excluded；AC30／TC72逐项分类不是业务执行通过 |
| [仓库检查](evidence/business-cost/repository-check.json) | 双语说明、发布文件、媒体、链接、敏感资料及当前来源检查 |

## 纠正与保留过程

早期只读审批账号不需要MFA挑战，复用管理员会话发布三种策略，以及采购执行角色遗漏等测试准备问题，均在隔离数据中纠正，没有放宽应用权限。[尝试01](evidence/business-cost/attempts/01-fixture-setup/attempt.json)保留最新可见原生结果和明确的早期观察；未保存的旧原始日志没有补造。

一次执行计划的Vitest输出参数缺少`--outputFile=`前缀，导致没有选择实际用例并在应用目录生成默认报告。浏览器来源检查正确拒绝了同时发生的文件清单变化；原结果保留在[尝试02](evidence/business-cost/attempts/02-report-option/attempt.json)。改正后将原报告移到Git外，重新固定计划并执行当前检查。该问题期间实际通过的七个后端套件因计划／来源变化仍不算最终通过。

首个费用浏览器模式遗漏了公共帮助器的允许模式项，在浏览器启动前停止；隔离数据库和临时服务已正常退出。修正仅涉及检查模式白名单，原失败与旧来源后端结果保留在[尝试03](evidence/business-cost/attempts/03-browser-variant-and-superseded-plan/README.md)。最终结果重新绑定准确输入，旧结果不改写成通过。

移动连续场景首次在直接改写H5路由后点击了上一笔合同的入口；服务按实际提交的合同依据登记，检查因与目标采购不符而保持失败。[尝试04](evidence/business-cost/attempts/04-mobile-page-transition/README.md)保留该结果。随后增加准确标题等待仍失败，确认Taro恢复了带旧合同参数的缓存页面，不能将此解释为单纯等待不足；[尝试05](evidence/business-cost/attempts/05-h5-cached-hash-navigation/README.md)保留这一纠正。最终检查通过产品本身的“首页 → 合同／采购／报销列表 → 准确业务行”导航，再等待准确标题并操作费用入口。没有放宽来源或回执校验，也不将直接改写参数化H5缓存路由称为已支持的使用方式。

首轮Android JS编译在原900秒超时边界退出，没有生成通过证明；原始退出记录保留在[尝试06](evidence/business-cost/attempts/06-native-bundle-timeout/README.md)，原日志保留在Git外。重试继续使用同一命令、四Worker、原生Windows文件读取适配与no-Expo检查，不以旧源映射替代当前编译。

同命令重试在核对进程归属后主动结束，非零结果保留在[尝试07](evidence/business-cost/attempts/07-worker-bridge-diagnostic/README.md)。已安装Metro源码明确用空`execArgv`启动文件映射Worker，实际子进程命令也没有父进程的`--require`。本轮仅在自有打包环境设置固定`NODE_OPTIONS`，把同一原生文件读取适配传给这些子进程；不继承外部Node选项、不替换源文件或链接目标，保持四Worker及no-Expo检查。不能将该观察扩写成已完全证明全部耗时原因。帮助器输入变化后，原先通过的八项命令记录保留在[尝试08](evidence/business-cost/attempts/08-before-worker-bridge-inputs/README.md)，九项最终命令全部重新执行；页面媒体不消费该帮助器，继续按其独立来源清单核对。

随后一次自有进程诊断的关闭代码误用了ESM上下文不存在的`require`，导致该次打包退出；[尝试09](evidence/business-cost/attempts/09-diagnostic-close-error/README.md)保留非零结果。临时诊断脚本已改为Node22的内置模块入口，未改变业务输入与最终命令来源，重新执行相同编译。

## 可重复命令

```text
python app/scripts/verify_scoped_increment.py --plan docs/quality/evidence/business-cost/verification-plan.json --run backend
python app/scripts/verify_scoped_increment.py --plan docs/quality/evidence/business-cost/verification-plan.json --run clients
python app/scripts/run_workbench_browser.py --feature business-cost
python app/scripts/run_workbench_browser.py --feature business-cost --mobile
python app/scripts/encode_workbench_media.py --feature business-cost
python app/scripts/verify_project_business_native_js.py --feature business-cost
python app/scripts/verify_project_business_development.py --feature business-cost
python app/scripts/verify_scoped_increment.py
python app/scripts/verify_current_scope.py
python app/scripts/verify_acceptance_applicability.py
python app/scripts/verify_repository_publication.py
```

使用原生Node22、Go、PostgreSQL、Edge和FFmpeg；已有完成结果先保留，再执行新尝试。浏览器顺序执行并关闭全部自有资源，最后检查零隔离数据库。依赖、原始录制帧、凭据及日志不入Git。

## English scope and remaining work

Current source-bound checks cover seven selected native HTTP/PostgreSQL suites (115 passing records),345 Web and436 portable mobile tests, workspace types, current Web/WeChat/Douyin builds, native JS primitives and Android JS with the disclosed Windows filesystem adapter and original no-Expo map guard. The actual map contains all four changed native business/expense modules with current source contents, modulo line endings. Current JS compilation is not a signed APK.

Real Web and Taro H5 journeys record all three approved source kinds, inspect fixed facts and ordinary history, and verify75.03 incurred equals60.00 project plus15.03 company shares. Two silent MP4s record the first source after MFA; six PNGs and two labelled screenshot-tour GIFs accompany them. See the visual record for the actual decoded samples inspected. Browser contexts, temporary servers and disposable databases are closed.

Prior published proof/media bytes remain unchanged at commit`3e8c1be6b001797968665c07f0383728e4832862`; earlier121 full regression/runtime/APK remain bound to their original sources. All23 packages remain active, including company policies, complete cross-module business acceptance and real cloud/model/TG/Linux/platform/production work. No real-device/emulator, network-performance/live120GB special, historical migration/DR or AI fee-control acceptance is introduced.
