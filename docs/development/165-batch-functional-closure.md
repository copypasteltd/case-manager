# 增量165整批功能收口 / Whole-code-first wave

本轮先逐项复核全部23工作包，再一次性完成C01–C05全部代码、前置用例与工具，保存完整源码后才开始统一检查。当前仍是23项正式工作包开放；不能把只读审查未发现缺陷写成全部完成。

| 编号 | 完整代码任务 |
|---|---|
| C01 | 订阅提醒按账期固定配置／时区排队；重估后按准确账期版本去重；追加130迁移和数据库守卫。 |
| C02 | 提醒公开历史显示准确版本，Web与共享移动同步；移动补充合同提供普通主合同入口。 |
| C03 | 快照变化、清空规则、重估、重复任务、SQL拒绝、权限与合成页面前置场景。 |
| C04 | 全量代码快照、统一选择检查、统一失败收集与集中修复、浏览器清理及截图／录屏／GIF工具。 |
| C05 | 全23编号处置、双语指南、更新说明与当前证据绑定。 |

FIN-10要求提前提醒与时区使用账期快照。旧后台／SQL使用订阅当前提前天数，改成空列表可漏掉旧账期提醒。另有明确重估后旧事件被正确抑制，却因按账期／天数去重而无法重新提醒的缺陷。本轮保留原事件和送达记录，新事件固定新账期版本，不把历史提醒改成当前提醒。

当前订阅启用后时区本身不可修改；未把不能由普通入口产生的跨时区修改误报为已发生业务缺陷。排队仍不代表送达或已读，当前资格及人员权限在发送时继续复核。

移动端现有补充协议仅缺少当前主合同阅读入口；使用现有合同读取和目标页权限，审批历史快照不添加当时未固定的资料。

All23 formal packages, eight draft Skills, unavailable company rules and actual configurations remain open. This source wave preserves nine freezes and APK164/business163/schema129. No new APK, real devices, network benchmark, historical migration/DR or AI fee gate is introduced. DevelopmentPG100/API remains unchanged.

[完整代码清单](165-batch-code-plan.json) · [AC03](165-audit/ac03-project.md) · [AC04](165-audit/ac04-contract.md) · [AC07](165-audit/ac07-subscription.md)

## 使用逻辑 / Usage

1. 订阅详情修改提前提醒天数，只影响以后新生成的账期；原预测继续使用其固定配置，即使当前规则改为空也不会撤销旧账期的固定计划。
2. 明确选择可重估账期并确认当前配置后，普通重估建立新版本，立即排队固定生成／提醒任务；作废后重新开启，以及实际费用关联后仍待核对的预测，也在同一普通事务中排队。暂停／取消资格与当前权限继续由后台复核。
3. 提醒历史逐行显示原提醒日、账期依据版本和实际站内投递状态。旧排队事件在账期版本变化后被抑制；已送达事实仍保留。点击账期阅读入口读取当前资料，业务修订历史另按普通历史权限查阅。
4. 微信／抖音／Android共享移动源码的补充合同详情提供“查看主合同”，目标页重新请求普通合同接口；没有权限时不显示主合同内容。

Changing reminder settings applies to newly generated periods. Existing forecasts retain their pinned configuration until explicit reforecasting. Every ordinary mutation that leaves a forecast queues an immediate, immutable generation input; old pending notifications remain subject to exact version and current-recipient checks. Delivery is neither read acknowledgement nor a payment.

## 正常升级与兼容 / Ordinary upgrade

追加 `0130_subscription_reminder_basis.sql`：旧提醒行不回填，旧outbox固定的aggregate version是历史依据；新行必须明确固定period_version。数据库按组织／账期锁串行检查旧、新行去重，并保持不可变历史。

先安排相匹配的新后端及普通130升级，再启用当前代码；旧129写入器不能继续创建未固定版本的新提醒。九个旧冻结保留原源码／产物，不声称能在130数据库上直接运行旧写入器，也不生成降级脚本。现有开发PG100/API不升级，本轮仅使用隔离原生测试库。

The migration is append-only and preserves all129 earlier migration bytes. Legacy reminder versions come from immutable outbox evidence without rewritten history. New writers must provide the exact current period version, so deploy matching server code with schema130; no downgrade or old-writer compatibility is claimed. This is an ordinary schema upgrade, independent of the excluded historical migration/DR work.

## 统一检查后的修复 / Repairs after the full wave

实际页面检查确认提醒列表可能在普通额外角色下触发原15秒语句超时。列表仍以同一组织、订阅、用户、渠道及AI标记调用普通权限函数，改为每条SQL一次求值；不延长期限、不改变授权。默认20条读取及无权用户拒绝已加入同一原生业务族；最终三个测试族共36条结果通过。

客户端消费者未随这些后端修复变化，首次七步客户端检查保留为准确当前证据：Web527／共享移动553、类型和Web／微信／抖音构建通过。当前Web与移动H5各两项实际UI／API检查通过，媒体与清理均绑定实际采集输入。发布收尾仅改两个核对／执行工具：移除空白行空格，明确媒体索引与历史媒体原件的边界；完整原工具、前后摘要及原失败保留，核对工具只对这两项还原采集输入，其余源码必须准确匹配。应用及媒体未重跑或改写。

原生库准备需串行初始化，避免已有共用临时密码文件守卫冲突；失败准备不计业务执行。详见[统一验证](../quality/165-batch-functional-verification.md)。
