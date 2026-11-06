# 增量162整轮验证 / Unified verification

本轮先完成全部C01–C06应用、接口、SQL、四端入口、运行器及用例，保存[测试前记录](evidence/batch-round-162/freeze-code-initial.json)和准确源码归档，再同时执行原生与客户端检查；全部23编号已在[代码计划](../development/162-batch-code-plan.json)逐项复核。正式工作包仍23项开放，不把源码或合成通过当作实际接入／正式接受。

| 项目 | 实际结果／来源 | 边界 |
|---|---|---|
| 原生HTTP／PG | [repair-4](evidence/batch-round-162/native-tests-repair-4.json)：31测试结果全部通过，另1包结果；2主用例家族、29子案例 | 包括16准确TG变更、完整本人分页、他人隔离、原依据失效、普通取消及旧任务创建家族；未执行全后端回归 |
| TG MCP目录 | [initial](evidence/batch-round-162/native-tests-initial.json)中的TestRound162TelegramProjectCatalog通过 | 该批整体失败记录保留；只复用其未更改的实际目录输入与特定通过用例，不将整批改写为通过 |
| Web | [客户端repair-3](evidence/batch-round-162/client-checks-repair-3.json)：522用例通过 | 既有520＋新增2；末次仅日期时间显示修正，未改测试依赖；类型与构建另重跑 |
| 共享移动 | 同一记录：240＋308＋独立2＝550用例通过 | 增量161的4项附加契约记录保留、未重跑；不合并为本轮新执行 |
| 运行器／Skill | 66用例、8个draft目录检查通过 | 没有真实模型、专用Linux执行、Skill发布或回退接受 |
| Web／微信／抖音构建 | 当前类型检查、Web产物和两小程序构建通过 | 本地开发配置；不替代真实AppID／域名／平台提审 |
| Android | 共用源码／类型检查；保留APK154与rc.8 | 无新Android原生JS bundle／APK／AAB、无正式签名、无真机／模拟器 |
| Web到期时间修正 | [web-time](evidence/batch-round-162/client-checks-web-time.json)：类型检查与Web构建通过 | 仅受影响检查重跑；其余单位／移动／运行器结果保留repair-3，末次原生结果保留repair-4 |

## 失败与集中修复

- [初始诊断](evidence/batch-round-162/initial-failures.json)：新增契约测试资料缺少强类型基础参数、移动选择器缺少disabled、移动Node测试须使用仓库既有相对导入、运行器版本断言需升级1.11.0；Web当批通过记录保留。第一批原生夹具未将管理员加入实际项目，群配置按普通权限拒绝。
- repair-1原生夹具重复使用同一MFA时段，实际返回CHALLENGE_INVALID；保留该失败并去除重复登录，未弱化MFA。
- [repair-3诊断](evidence/batch-round-162/repair-3-diagnostics.json)：统一业务场景暴露旧SQL立即约束仍只允许update_task，以及公开AgentMutationArguments遗漏可选参与人。全部在追加128和当前契约中集中补齐；回执重放比较完整规范JSON，保留所有字段而不要求PG jsonb的空格／键顺序相同。
- [repair-4诊断](evidence/batch-round-162/repair-4-diagnostics.json)：上一批15／16准确操作通过；新任务携带参与人仍遇旧12字段SQL门槛。追加完整的可选准确参与人数、唯一性、活动成员与实际提交／回执核对，同时保留原12字段协议。完整16操作、清单和旧任务创建家族随后通过。
- [页面复核修正](evidence/batch-round-162/web-time-repair.json)：首份Web截图的预授权时间只显示日期，无法核对10分钟期限；三份Web显示源码集中改为香港时分秒。保存修正前源码与Web首录，不覆盖既有媒体；仅重跑类型／Web构建与受影响Web录制，移动录制保留未改的准确输入。
- [交付检查修正](evidence/batch-round-162/verification-repair.json)：首次整体验证21／22通过，媒体不可变检查误包含须随更新维护的docs/media/README.md导航。保存原检查源码及失败记录，仅允许这个准确导航文件更新，其余历史证据／媒体／迁移字节检查不变；没有更改业务或重复业务测试。
- [提交格式修正](evidence/batch-round-162/publication-format-repair.json)：首批发布检查19／20通过，仅原生检查脚本第118空白行有尾空格。去除该行空格、保留其余准确字节；原脚本、原通过证明与首次发布检查另存，重新计算本轮最终证明。业务／捕获输入没有改变，不重跑无关功能用例。

初始源码ZIP、分批修复源码与每次失败记录均保留在[evidence目录](evidence/batch-round-162/)。客户端在公开契约更改后统一重跑；最后纯后端SQL／夹具修正不重复客户端检查。增量161和所有历史证据未改写。

## 页面与交付边界

Web和共享移动H5使用独立原生PG及合成员工／项目／Bot元数据。展示实际清单与普通取消，不连接真实Bot或模型；原群固定触发代码遮挡，登录／MFA／密码不进入录制。截图、真实静音录屏与由录屏转换的互动GIF按准确媒体清单核对，截图轮播GIF单独标记。

当前主媒体为[Web时间修正后录制](evidence/batch-round-162-functional-wave-web-time/result.json)与[共享移动首录](evidence/batch-round-162-functional-wave-mobile-initial/result.json)。两份均实际完成读取→填写原因并核对→普通取消→回执刷新，后端另核对两份授权没有生成任务。所有自建浏览器、临时服务与独立原生PG均已关闭，私有夹具已删除。[六张页面与两份GIF末帧复核](evidence/batch-round-162/visual-review.json)绑定准确图像哈希；[媒体导航](../media/batch-round-162/README.md)区分主媒体与保留的Web首录。

真实服务器／云／模型／TG／小程序／签名配置仍由用户后续提供。没有重做真机、网络性能／实网120GB、历史迁移、容灾、AI费用控制；未将这些移出项转记为通过。开发PG100/API、全部八冻结、APK154/business153/schema113不变；当前源码需普通升级至128。

All declared code precedes unified checks. Failures, actual inputs and grouped repairs stay visible; selected source-bound local checks never establish whole-goal or production acceptance. The personal inventory never starts a run, cancellation does not undo completed business, and grant consumption, actual business completion and independently reviewed group disclosure remain distinct.
