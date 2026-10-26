# 增量64：TG 群通知、投递回执与人工对账验证

验证日期：2026-10-26（Asia/Taipei）。本轮实现见[开发文档](../../../development/64-telegram-delivery.md)。源码与证据相互核对，完整平台 Goal 保持执行。

| 验证 | 已通过的实际范围 | 证据 |
| --- | --- | --- |
| 完整后端 | 7 批、140 个顶层测试、846 条结果、766 个末级场景；无失败／跳过，全部发现的测试执行 | [结果](result.json)、[批次](batch-progress.json) |
| Web | 54 个测试及生产编译；登录保持 TG 请求链接、本人回执与追加对账 | [构建](web-build.json)、[浏览器 3 项](../telegram-delivery-web/result.json) |
| 移动 | 62 个测试、类型检查、微信／抖音编译，均注册 14 个页面；H5 完成 MFA 对账及独立 API 回执核对 | [构建](mobile-build.json)、[浏览器 2 项](../telegram-delivery-mobile/result.json) |
| 秘书运行时 | 34 个测试、构建与八个 draft Skill 包检查；通知未加入模型工具 | [运行时](runtime-build.json) |
| Linux | linux/amd64 交叉编译；产物未执行、未部署 | [编译](linux-build.json) |
| 原生发送命令 | 显式 disabled 配置立即拒绝启动，所属命令退出；未进入发送循环 | [启动检查](delivery-startup.json) |
| 开发环境 | 63 项迁移、3 个投递保护触发器、延迟事件外键、事件只增权限；新入口通知／发送预算／投递记录和活动模型均为 0 | [开发核对](development-telegram-delivery.json)、[运行底座](development-runtime.json) |
| 最终来源与回收 | 25 项核对，两个浏览器上下文、临时 Vite／H5 与一次性数据库全部释放；所属 API、通知 worker 保持可用 | [来源核对](final-source-check.json) |

完整后端 SHA-256：`cd059e07b85c2446a6d48013ac8d5b28f30086bc7739805f4b10bcf3aa6ef8cb`。Web／移动／运行时和浏览器使用各自完整来源清单核对，当前 OpenAPI 为 263 路径／328 操作／573 schemas，文档与内嵌副本一致。

新增原生场景覆盖 15 项投递生命周期：默认关闭、持久准备与披露范围、近期进程状态、未知不自动重试、MFA／本人／版本／幂等、人工重试、供应商冷却、5 次上限仍保留全局冷却、进度合并、真实 30 秒租约过期、在途配置变化、多进程预算、群迁移暂停、超过100条积压的范围撤销及绑定撤销。HTTP transport 另覆盖准确回执、错误位置、重定向、5xx、网络异常、明确拒绝、限流与过大回执。

初次全量批次在补充限流边界时因源码改变失效，已保留[失效批次记录](attempts/before-final-freeze-batch-progress.json)，不计入通过结果；补查积压队列后又保留[候选范围修正前的批次](attempts/before-backlog-fix-batch-progress.json)，随后从最终冻结源码完整重跑。聚焦用例也修正了新场景遗留等待请求的测试隔离。移动浏览器的按钮文字及独立 API 核对地址已修正，再以同一应用源码完成整链；失败截图保留在浏览器证据目录。没有通过放宽业务权限、跳过 MFA 或伪造发送成功来通过验证。

边界：模拟 transport 与受控秘书执行不代表真实 Telegram、模型、股东群和出站网络已验收；源码包保持 draft，未增加 TG 财务工具。微信／抖音编译与 Taro H5 不代表原生设备、平台发布或 Android 120GB 传输完成。云存储／2TB 纳管、真实状态采集、部署与恢复、四端首发继续按差异清单推进。
