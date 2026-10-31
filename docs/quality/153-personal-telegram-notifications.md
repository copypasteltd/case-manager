# 153 · 私聊业务提醒核验与保留边界

本轮按用户加速要求保留集中功能和授权检查，跳过全量回归、额外移动UI录制、新Android打包与生产外部服务验证。没有启动Docker、WSL、虚拟机或模拟器；原PG100／API18080保持原样。当前源码113；冻结rc.7、APK152及其证据仍绑定151／112。

| 编号 | 当前检查 | 边界 |
| --- | --- | --- |
| 1 | 原生临时PG、实际HTTP、双向绑定、业务outbox与人工对账：14个子用例，15条通过记录（含1条组汇总） | 合成企业与个人身份；真实会话和数据库，不发送真实TG消息 |
| 2 | 统一Bot适配器：3个顶层测试及15个子用例，共18条通过记录 | 本地受控RoundTripper；包含个人消息参数省略及原秘书回复适配回归 |
| 3 | Web 4项、共享移动逻辑4项 | 检查错Bot／通知集合、固定回执源、精确消息号、用尽重试和不完整日志；不等于平台正式验收 |
| 4 | 当前工作区类型检查、Web／微信／抖音构建 | 本地配置构建；Android共享源码类型检查，没有新APK或签名 |
| 5 | 当前Web实际HTTP／PG页面、真实像素录屏、截图及截图导览GIF | 本人设置核对、人工确认未知回执、明确停用；接口送达是受控合成结果，人工对账确实提交数据库，不代表真实TG送达 |

执行清单与源码摘要保存在 [证据目录](evidence/notification-telegram/) 和 [Web交互证据](evidence/notification-telegram-web/)。最终来源检查核对实际结果、素材摘要、七个冻结引用及先前证据字节。当前范围与AC／TC适用性检查只核对分类，不能算业务用例执行。

## 修复过程中保留的失败

- 首次四端类型检查发现移动目标不支持 `Object.hasOwn`，改用兼容的原型方法；后续集中类型检查与构建重新执行。最初直接类型检查输出未作为通过证据。
- 首次原生用例错误地对 `member` 角色启用MFA，而合成经办人实际为 `project_manager`，导致登录没有挑战却调用登记接口；`native-focused.json` 保留失败，修正合成角色后重跑。
- 第二次原生用例发现等待重试的失效订阅要等到重试时刻才被清理。工作器现在每轮检查当前授权并抑制失效记录，只有真正到期的有效记录才可外发；`native-focused2.json` 保留失败，`native-focused3.json` 是完成后的集中执行。
- Web录制最初沿用秘书会话页面的地址限定，因此在开始采集前停止；一次归档路径修正未成功，又产生一次相同限制失败。两次Web失败结果分别保留在 `notification-telegram-web-attempt1/2`，均未算交互通过，临时资源已关闭。修正独立录制脚本为精确通知中心地址后重新执行。
- 初始编译命令曾选错本机Go路径；随后能编译的占位筛选没有匹配测试，不计为任何业务测试通过。正式适配器执行见独立实际结果。

## 仍未完成

GAP-08新增“本人私聊提醒＋投递回执”子闭环，尚未关闭完整工作包：群／其他渠道规则、整体业务聚合及实际服务商验收仍需处理。真实服务器／外部平台配置按用户答复“后续提供”；不把等待资料伪装成本地测试通过。

保持全部 [23项未完成清单](../delivery/05-functional-closure-153.md)，14 P0／9 P1。八项Skill仍draft；真实Codex、Linux隔离、TG群、OSS＋R2、小程序正式身份、正式Android、PG／部署及公司完整旅程仍有未完成验收。真机、网络性能／实网120GB、历史迁移／容灾与AI费用控制依旧移出。

English: This increment runs selected native HTTP/PostgreSQL, transport and client checks, plus an actual Web capture. Provider outcomes are controlled fixtures. Full backend/client regression, fresh Android packaging, mobile UI recordings and external production acceptance are deferred. Failed development attempts remain recorded. The seven frozen releases and prior evidence are unchanged; none of these checks establishes production readiness or real Telegram delivery.
