# 增量160统一验证记录 / Whole-wave verification

本轮先实现C01–C06全部应用、契约、迁移、服务配置与用例，逐项复核23个工作包，记录首次完整代码冻结后才启动统一检查。顺序和来源见 [代码计划](../development/160-batch-code-plan.json) 与 [首冻证据](evidence/batch-round-160/freeze-code-initial.json)。范围内已有代码不等于23项正式验收全部结束。

| 编号 | 实际检查 | 结果与来源 | 限制 |
| --- | --- | --- | --- |
| V01 | 首次统一后端／客户端检查 | 保留编译、TS输入清单／类型等失败；集中修复第一批 | 失败原件不覆盖，不计为通过 |
| V02 | 第一批选定原生HTTP／PG业务用例 | 92条结果：88 pass、3 fail、1 skip；其中10套不受后续SQL修复影响的业务套件85条pass保留 | 整批仍为失败；浏览器等待夹具未执行不算业务通过 |
| V03 | 受影响的终态上传HEAD用例 | 第二批5条结果全部pass（3子例、父例、包）：全部已知位置成功、404不完整、原授权撤销即停止 | 只读受控存储传输；不是实际OSS／R2验收 |
| V04 | 客户端统一检查 | 当前类型检查、Web **520**（56文件）、共享移动 **548＋18＝566** 全通过；18含本轮新增11例 | 18例使用本批重新编译的测试；不把首轮旧输出重复计入 |
| V05 | 当前构建 | Web、微信、抖音全部通过；移动浏览器另行构建Taro H5 | 无本轮Android JS重试／新APK；158失败保留 |
| V06 | 运行器与Skill目录 | 初次统一检查中运行器64、8个draft包目录检查通过；源码未变，之后明确复用 | 未重跑，真实模型／Skill安装验收开放 |
| V07 | 原生发布工具 | Windows原生打包／文件／控制器40项通过，含独立只读观察worker配置；当前服务器含新增worker入口原生编译通过 | 注入进程效果，不代表Linux/systemd部署 |
| V08 | 实际Web＋本机原生HTTP／PG | 当前准确采购结项页面及完整7项事件清单2项检查通过，2PNG＋无声MP4＋截图导览GIF＋实际交互GIF | 合成业务；不是云端、模型或全部新模块UI验收 |
| V09 | 实际共享移动H5＋本机HTTP／PG | 准确业务入口、类型化订单表单、不可变验收历史、原验收行退货入口4项检查通过，4PNG及对应录像／两类GIF | 检查查阅／表单入口，没有提交实际订单或退货；不是Android／小程序真机 |
| V10 | 资源收尾 | 全部7个本轮隔离原生PG集群关闭；浏览器、临时Web／H5／API、构建／驱动进程关闭；私有夹具删除 | 开发PG100及用户原API未改 |

第一批代码修复包括共享源文件加入TS工程、命令回执／申请字段、完整差异提案类型守卫、空态参数／未使用项和隔离夹具编译错误。第二批只改已知上传观察回执分支的SQL参数uuid显式转换；修复前完整文件留存，核验器重建第一批来源后保留不受影响的85条通过记录，仅重跑受影响HEAD套件。当前各端输入未受第二批影响，不重复构建。

浏览器首轮记录也保留：Web初次清单选择了空分组，宽泛计数匹配不能证明7项完整事件。人工检查后收紧准确分组／对话框／7个入口的选择器，第二次通过；原结果不作为该业务UI验收。移动初次因Taro外层与内部输入框同时命中而失败，改用内部输入选择器后第二次通过。两项只调整检查驱动，未修改应用；参见 [调整说明](evidence/batch-round-160/capture-review-adjustment.json)。首张移动履行截图仍含读取中的提示，只证明导航；后续订单图已显示完整执行依据，不宣称首图完成了全部结项读取。

查看 [当前截图／录像／GIF](../media/batch-round-160/README.md)、[最终来源核验](evidence/batch-round-160/final-source-check.json) 与 [集中修复台账](evidence/batch-round-160/batch-repairs.json)。当前来源核验只校验所列证明及不可变旧字节，不重放业务测试。

保留全量后端回归121、旧各端证明和全部8个冻结版；**rc.8／APK154仍是business153/schema113，不含155–160**。源代码schema123，正常追加122–123升级；现有开发PG100/API不升级。23个工作包、8个draft Skills、真实服务器／OSS＋R2／Codex／TG／平台配置与正式验收继续开放。不执行用户移出的真机、网络性能、历史迁移／容灾、AI费用控制。

English: All six declared implementation/fixture tasks precede unified validation. Selected native results are separated into85 unaffected passing test records from the otherwise failed batch and five current affected-suite records. Web520/shared-mobile566, current Web/WeChat/Douyin builds and actual synthetic native-API Web/H5 navigation/form checks pass. Initial failures, wrong-group capture and locator correction remain disclosed. Runtime64/catalog checks are explicitly retained from the first attempt. No new APK, full regression, real provider/model/platform or production acceptance is claimed.
