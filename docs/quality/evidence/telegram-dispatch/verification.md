# 增量61：TG请求调度与独立秘书会话验证

日期：2026-10-25。功能约束见[实现文档](../../../development/61-telegram-agent-dispatch.md)。本记录只证明固定来源的本地受控范围，完整平台Goal继续执行。

| 证据 | 实际结果 | 证明边界 |
| --- | --- | --- |
| [完整后端](result.json)／[分批状态](batch-progress.json) | 17批、132个顶层测试、790条结果、714个末级场景全部通过，无失败、跳过或取消 | 所有发现的原生Go／PostgreSQL测试；新增TG调度与原业务回归 |
| [Web](web-build.json) | 54项及生产构建通过 | 固定当前来源 |
| [Web全流程](../telegram-dispatch-web/result.json) | 7项通过，2个独立上下文已关闭 | 本人结果、会话筛选、管理员配置／隐私、真实队列、重复请求、取消与成员变化撤回 |
| [移动](mobile-build.json) | 62项、类型、微信／抖音编译通过 | 公共页面仍13个，不代表真机验收 |
| [Taro全流程](../telegram-dispatch-mobile/result.json) | 3项通过，Android渠道与实际API，1个上下文已关闭 | H5构建与浏览器来源一致；TG会话／运行链接及结果，门户无新任务输入 |
| [Runtime](runtime-build.json) | 31项及八包校验通过 | TG专用SDK哈希、只读工具；模型为受控替身，来源包仍draft |
| [Linux](linux-build.json) | linux/amd64编译通过 | 未执行产物或部署 |
| [dispatcher命令](dispatcher-startup.json) | disabled时拒绝启动，预期错误及进程退出确认 | 实际原生命令，不启动调度循环或外部服务 |
| [开发环境](development-telegram-dispatch.json) | 60项迁移、14个TG触发器、追加表权限、测试库0 | 所属API／通知worker；活动模型配置0、TG接收／调度关闭 |
| [最终来源](final-source-check.json) | 21项通过 | 当前代码、构建、浏览器、原生服务和资源清理一致 |
| [文档与28项差异](documentation-check.json) | 独立检查记录 | 索引、需求、操作约束、台账及稳定编号 |

后端固定来源：`645da5440b862cea45e352f7caf8b9ba388a84e7a09df12148a49fb63290e43c`。回归期间来源未变；各报告记录来源清单和摘要。所属Web／Taro浏览器上下文、临时Vite／H5服务和隔离库均清理；本地所属主API、通知worker及Web入口保留。

新增原生验证包含：无配置和仅Web配置不创建任务；事件摘要工具未接通时保留原因；并发仅一个真实运行／工作项；同会话串行；SQL关联故障整体回滚；员工上下文隔离；门户注入／改成员拒绝；TG原生证明与MCP仅三个读取工具；真实引用及结果；配置退休、群范围失效和绑定撤销阻断执行；不可变来源和配置哈希。

开发库未登录POST冒烟探测使用空请求体，验证授权先于正文解析，并在报告保留此范围。Windows本地短连接携带未读正文时观察到间歇连接重置；空体探测连续20次得到401。此探测不证明全部Windows短连接行为，携带真实业务正文的功能另由本轮HTTP／MCP集成与Web流程验证，生产网络仍需实测。

未调用真实Codex／TG／OSS／R2，未在生产启用dispatcher。正式Skill发布、原生隔离证明、事件／订阅工具、群披露与Bot投递、四端真机、120 GB／2 TB及恢复性能仍按[差异表](../../../delivery/03-goal-gap-assessment.md)继续验收。
