# 增量151：选定检查与证据 / Selected verification

按用户要求缩减全量回归、额外移动UI与新APK核验。执行当前选定原生HTTP／持久PG、普通交付及秘书更新回归、共享表单／回执检查、完整秘书运行器测试、类型和Web／微信／抖音构建。四渠道HTTP登录检查不构成真机或平台提审。

[当前源证据](evidence/deliverable-submission/final-source-check.json)绑定本轮实际输入、日志摘要、失败尝试和结果。原始失败尝试保留，不覆盖后再声称首次通过。原验证与媒体按原字节保留；全量后端121、完整客户端136、冻结rc.6／APK150依然各属原源码。

[浏览器记录](evidence/deliverable-submission-web/result.json)展示真实Web表单、固定附件选择和授权入队，随后明确取消合成任务。没有运行真实模型。实际提交／幂等／撤销和独立验收边界由原生HTTP／PG测试另行证明。MP4来自认证后的真实页面像素；GIF为三张实际截图的六秒导览。浏览器、临时API、临时PG均由检查关闭。

当前源码结构112，原开发库100不升级；当前开发APK150仍业务149／结构111，本轮主源码新入口尚未进入旧包。后续冻结包另有独立版本清单。本轮没有真实云、模型、TG、正式Android签名或生产验收；全部23工作包保持跟踪，移出项不计通过。

English: Selected checks are source-bound. Web capture demonstrates reviewed authority and queue acceptance, then cancellation; it does not demonstrate model execution. Actual immutable business commits are independently tested against native PostgreSQL. Historical complete regressions and frozen artifacts are retained at their original source. No device, cloud, signing or production acceptance is claimed.
