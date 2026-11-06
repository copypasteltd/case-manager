# 增量165统一检查 / Whole-wave verification

全部23编号处置及C01–C05代码／前置用例／工具完成后，先保存完整源码，再统一执行所选HTTP/PG业务族和客户端检查。初始完整源码、首次失败、集中修复及各次准确输入均保留；本轮没有逐模块发版。

本轮基于AC03/04/07部分源码审查及独立反证，不声称全量合规审计。普通订阅时区已固定，未虚构可变时区的业务失败。

## 当前实际执行 / Current executions

| 检查 | 实际结果 | 准确记录 |
|---|---|---|
| 原生HTTP／PostgreSQL | 三个业务族通过，36条具名结果 | [native repair4](evidence/batch-round-165/native-tests-repair-4.json) |
| Web | 527项通过 | [客户端七步检查](evidence/batch-round-165/client-checks-initial.json) |
| 共享移动 | 553项通过（240＋308＋5） | 同上；当前客户端消费者字节与首次检查一致 |
| 类型及客户端构建 | 类型检查、Web、微信、抖音构建通过 | 同上；没有新Android构建 |
| Web／移动H5实际页面 | 两组各两项业务／API检查通过，所有自建浏览器、服务及隔离PG已关闭 | [Web](evidence/batch-round-165-functional-wave-web-verified/result.json)／[移动](evidence/batch-round-165-functional-wave-mobile-final/result.json) |
| 当前代码与证据绑定 | 24项核对通过；完整前置源码、当前消费者、媒体字节、旧冻结及范围均核对 | [最终源码核对](evidence/batch-round-165/final-source-check.json) |
| 当前范围 | 12项通过；23开放、5移出、14P0／9P1；原30AC／72TC适用性另外12项核对通过，未新增业务验收通过数 | [范围记录](evidence/batch-round-165/scope-verification.json)／[适用性清单](04-current-acceptance-applicability.md) |
| 仓库发布 | 工作文件20项核对通过；敏感内容、禁止路径、失效本地文档链接均无发现 | [当前发布核对](evidence/batch-round-165/scope-publication-check.json)；首次失败另存 |

原生用例覆盖旧账期固定规则、当前规则清空、新旧提前天数隔离、重估及重开立即排队、重复任务去重、版本失效抑制、当前版本送达、不可变事件、无费用／现金副作用、错误SQL输入拒绝、默认20条分页与无权用户拒绝。页面实际查看版本1／2／3／5、原七天规则与新的投递状态，并点击补充协议的普通主合同入口。

## 保留失败及集中修复 / Retained failures and batch fixes

1. 首次原生检查发现测试租约标识含不受支持的斜线，以及SQL别名与触发器`OLD`冲突；修复后统一重跑三族。随后在同一提醒链补齐重开／仍处预测状态的变更排队，再统一检查。各次输入不覆盖。
2. 首次页面前置流程误要求普通独立审批人提供不存在的MFA挑战；仅移除该浏览器分支的强制登录步骤，仍使用真实独立普通审批会话。[准确差异](evidence/batch-round-165/fixture-only-adjustment.json)保留；当前repair4直接执行最新完整测试源码。
3. 一次Web录屏跨完整页面重载失败；改为普通SPA中间导航，保留真实主合同链接点击和接口核验。另一轮Web提醒读取返回500，原生PG日志确认15秒查询超时；旧查询的一次重试通过仅保留为诊断，不能冒充修复。本轮将同一准确订阅权限检查固定为每条语句一次求值，保留原参数、权限和15秒期限，并增加默认20条及无权用户业务用例。[诊断](evidence/batch-round-165/reminder-query-diagnosis.json)。
4. repair3在原生库初始化阶段因共用临时密码文件守卫拒绝重叠准备，未执行业务测试；保留[准备失败](evidence/batch-round-165/native-preparation-repair-3-failure.json)。后续串行初始化，repair4通过；不能把准备失败写成业务通过。
5. 原移动主合同PNG采到路由切换中的侧滑，原件保留。指南使用同一次实际录屏最后稳定画面的原生提帧，绑定视频／图片摘要；不修改产品或重绘页面。[提帧依据](evidence/batch-round-165-functional-wave-mobile-final/derived-parent-frame.json)。
6. 首次仓库发布检查20项中有两项未过：执行工具两处空白行各含16个空格，以及新增媒体索引被过宽的旧媒体保留规则拒绝。原[失败报告](evidence/batch-round-165/scope-publication-check-initial-failed.json)与[原23项源码核对](evidence/batch-round-165/final-source-check-before-publication-repair.json)保留。仅修正执行工具空白和核对工具，媒体保留规则只放行Markdown索引。两工具完整原文、前后摘要及[限定修正记录](evidence/batch-round-165/publication-source-adjustment.json)固定；重建完整采集输入时只替换这两项，其他所有源码必须逐字一致。当前应用、用例及原媒体不变，未为纯工具修正重跑业务或补录。

详见[汇总](evidence/batch-round-165/test-summary.json)、[全部修复处置](evidence/batch-round-165/initial-and-repair-disposition.json)、[人工视查范围](evidence/batch-round-165/visual-review.json)及[实际媒体](../media/batch-round-165/README.md)。凭据／登录／MFA画面不进入公开记录，底层私有日志不提交。

## 保留与待验 / Retained and open

运行时66项保留163原执行，本轮未重跑；历史全量回归保留121。APK164和rc.9仍固定业务163／schema129，九个冻结与旧证据／媒体／迁移1–129不变。当前main165新增普通130升级；开发PG100/API未改动。

完整回归、正式Android、实际云／模型／平台／生产及公司制度确认仍待验。全部23包及八项draft Skills开放。真机、网络性能、历史迁移／容灾、AI费用控制仍按现行范围移出；本轮查询修复不是网络性能专项。

All declared code and fixtures preceded the unified selected checks. Current native3/36, Web527, shared mobile553, Web/WeChat/Douyin builds and both actual UI/API captures pass. Failed preparations, query timeout, fixture and recording repairs retain their original inputs. Runtime66, the prior full regression and APK164 are retained evidence, not new executions. This is a source increment, not complete-goal or production acceptance.

Two post-capture publication helpers have exact retained before/after hashes: whitespace-only runner cleanup and the evidence checker. Captured inputs are reconstructed from those two original helpers while every other source byte must match; application consumers and recordings remain unchanged. The final24-check proof records this bounded adjustment explicitly.
