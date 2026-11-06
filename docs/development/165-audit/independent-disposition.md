# 独立反证与代码处置 / Independent disposition

审查基线为 `3dc6deeba29298fbff101e818ac6b68e944db07a`。三份报告是有限源码审查，不是全部23项或30AC的完整合规结论。本记录区分源码证据与后续实际执行。

| 编号 | 独立核对与处置 |
|---|---|
| F-AC07-01 | FIN-10明确要求账期固定提醒规则。`pinSubscriptionPeriod` 固定配置修订，生成遇既有账期只保留，而 `queueSubscriptionReminders`／SQL0038守卫却使用当前配置；清空／修改规则可改变旧预测提醒。候选成立，C01按固定配置修复。 |
| F-AC07-02 | 显式重估增加账期版本，通知派发正确抑制旧版本；原 `(org_id,period_id,days_before)` 唯一键仍阻止新版本提醒。候选成立。新增版本去重，保留旧事件及当前权限复核。 |
| F-AC07-03 | 仅修改去重仍不足：日调度按配置／控制版本／日期去重，同配置重估不会自然获得当日新任务。C01在普通重估、重开及仍处预测状态的费用关联事务中原子排队固定输入任务，避免等到下一日调度；没有把全订阅版本加入日调度键而造成生成自触发循环。 |
| F-AC04-01 | 移动端创建补充合同时能选择主合同，但普通现有记录仅投影正文，缺少主合同阅读区；全源检索未发现另一入口。C02加入当前关系区，使用现有目标页授权读取。历史申请快照不补写未固定的关系。 |
| REFUTE-01 | 当前普通订阅更新通过 `subscriptionFixedCalendar` 固定启用后的时区，不能把任意改时区当作已证实用户旅程。本轮保留按账期时区求提醒日期的正确边界，但不虚构可变时区测试成功。 |
| REFUTE-02 | Web合同页已有“查看文件记录”以及独立的固定正文／签署件下载；当前文件入口不构成缺少固定版本访问的证据。未重写已有功能。 |
| REFUTE-03 | 已读项目任务依赖、交付、结项、归档服务器及SQL链未确认新偏差；这只能支持有限审查结论，不能证明完整公司制度或全量业务验收。 |

升级130不回填／修改旧提醒事实；旧行的准确版本从不可变outbox读取，新行要求明确版本。当前资格、暂停／取消、原收件人及普通权限继续在发送时检查。公开历史显示每条依据版本，原送达不改写为当前送达。

All changes belong to one declared code wave. Tests, source manifests and actual media are recorded separately under the round165 evidence group after the whole code snapshot. Real company rules, actual integrations, eight Skill releases and all23 formal packages remain open.
