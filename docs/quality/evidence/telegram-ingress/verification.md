# 增量60：Telegram 本人绑定与入站验证

核对时间：2026-10-25 20:40（Asia/Taipei）。对应[功能契约](../../../development/60-telegram-binding-and-ingress.md)及[28项差异](../../../delivery/03-goal-gap-assessment.md)。

| 验证项 | 结果与证据 | 限定范围 |
| --- | --- | --- |
| 完整后端 | [全部结果](result.json)：7批、129个顶层测试、767条结果、693个末级场景通过，无失败／跳过 | 原生PG、认证HTTP、受控依赖；新增TG集成11组、协议及配置测试 |
| Web | [54项及生产构建](web-build.json)通过 | 包含实际TG接入页；布局修正后重新验证 |
| 移动 | [62项、类型及微信／抖音构建](mobile-build.json)通过 | 共享合同回归；13个公共页面不变，本增量未新增移动TG页或H5验证 |
| Runtime | [29项及八个draft包](runtime-build.json)通过 | TG尚未创建真实模型运行 |
| Linux | [linux/amd64编译](linux-build.json)通过 | 未执行或部署产物 |
| Web全流程 | [7项浏览器验证](../telegram-ingress-web/result.json)通过 | 两个独立门户身份、真实原生API及隔离PG；本机模拟TG入站 |
| 开发库／路由 | [开发核对](development-telegram-ingress.json)通过 | 59项迁移、8个TG守卫、受限表权限与既有文件／OA／知识／项目保护 |
| 最终来源与清理 | [来源检查](final-source-check.json)16项通过 | 测试库0、两个所属浏览器上下文关闭、临时Vite／端口关闭、私密fixture移除 |
| 文档 | [索引／链接／差异检查](documentation-check.json) | 运行后生成机器结果，保留28个稳定编号 |

后端来源：`70f55269ce094b1ab14dd25c99fb39e254be015250999d3a905b33798a10b58c`。Web、移动、Runtime及浏览器各自的来源清单／摘要保存在对应结果中；最终检查重新计算当前文件，不以历史增量的通过结果替代本轮来源。OpenAPI两份副本一致，共260路径／325操作／566Schema。

原生TG集成验证：错误secret／输入边界、入站失败使私聊证明回滚、证明与原门户确认分离、管理员近期MFA、准确群／话题和无正文忽略、并发去重及异内容冲突、成员变化暂停／撤回与历史分页、取消／撤销及归属不可转移、数据库不可变保护、MFA过期、原登录撤销。

浏览器通过实际页面完成发起绑定、数字身份证明、核对确认、管理员启用、本人请求及同一更新重试、成员变化暂停／历史、本人撤销。首轮因两条合法历史记录造成测试定位歧义，提前停止，不计通过；[失败记录](../telegram-ingress-web/attempt-01-selector-failure/result.json)及对应源码、截图保留。第二轮通过后修正页面间距和复选框，再完成最终全流程；[调整前记录](../telegram-ingress-web/attempt-02-before-spacing/result.json)单独保留。私聊链接截图遮罩，原始日志留在忽略的本地目录。

本聊天拥有的API与通知worker已更新，开发配置仍关闭真实TG接收，活动模型配置为0，八个Skill仍draft。未调用Docker、WSL、虚拟机、模拟器、真实Telegram／模型／云；未执行Bot注册、入群、setWebhook或对群发送。TG请求仅保存为awaiting_executor；秘书桥接、群披露、真实投递／对账、设备及生产验收仍待完成。
