# 增量105验证：Codex原生执行控制器

日期：2026-10-29（Asia/Taipei）。实现、安装、边界与目标服务器验收见[开发手册](../development/105-native-agent-runtime.md)。完整工作包仍为23项（14 P0／9 P1），5项移出；GAP-09／10／13继续开放。

| 编号 | 检查 | 实际结果与证据 |
|---|---|---|
| V-01 | 完整后端 | 最终固定源码219个顶层／1338条pass结果，所有发现测试执行、无skip。原生PG、11个串行批次；见[结果](evidence/native-agent-backend/result.json)和[原始日志归档摘要](evidence/native-agent-backend/raw-log-archive.json)。 |
| V-02 | 运行时 | 当前55项、Skill校验和编译通过；受控SDK客户端和loopback HTTP，未调用真实模型。见[运行时](evidence/native-agent-runtime/runtime-build.json)。 |
| V-03 | 打包 | 4项打包边界、真实发布目录的Skill／profile两个入口通过，685文件、八项draft，输入与输出逐字节匹配。见[打包](evidence/native-agent-runtime/package-checks.json)。 |
| V-04 | Linux | 当前platform和agent-native两个ELF64 amd64产物编译通过；未执行、未部署。见[交叉编译](evidence/native-agent-runtime/linux-build.json)。 |
| V-05 | 原客户端 | Web159、移动261、微信／抖音各20页及Android构建按104来源保留，当前输入摘要相同；26条JVM结果仍按原UP-TO-DATE范围。见[保留记录](evidence/native-agent-runtime/reused-evidence.json)。 |
| V-06 | 媒体 | 113 PNG／43 MP4／43 GIF＝199份，核对104发布Git对象与SHA256；没有新UI采集，不声明当前后端的浏览器执行。 |
| V-07 | 原生开发服务 | schema82及现有API／通知／报表Worker更新和只读核对见[更新](evidence/native-agent-runtime/development-update.json)与[检查](evidence/native-agent-runtime/development-project-resources.json)。未启动模型控制器／TG／云集成，最终临时测试库为0。 |
| V-08 | 发布安全 | 未改锁，沿用103完整依赖审计70项（6 high／57 moderate／7 low），所有剩余包记录继续开放；本轮没有新扫描声明。 |

首次回归执行中的代码复核补强了工具结果内的图片／文件输入拒绝。当前批次正常结束后，源码摘要守卫拒绝把前后版本混用；归档的5批与停止原因见[初次记录](evidence/native-agent-runtime/attempts/01-before-tool-output-hardening/result.json)。它不是最终通过结果，最终219项从头重新执行，未复用中间结果。早期本地策略检查曾发现Go正则重复上限问题，已改成字符集合与独立长度检查后重跑；该诊断不作为最终证明。

内核观察测试使用显式合成快照；原生控制器在Windows拒绝启动，不存在生产“总是成功”验证器。本轮没有实际Linux单元、真实模型／CLI生成、真实云／TG或生产验收，也未做真机、模拟器、网络性能、实网120 GB、历史迁移、容灾或AI费用配额。目标服务器需要继续验证实际拒绝访问、资源限制、取消、重启回收、固定CLI HTTP兼容和八项Skill评测。

English: Final backend219/1338,runtime55,4 packaging tests,2 packaged entry points and2 Linux cross-builds pass. Kernel snapshots and SDK clients are controlled fixtures. Existing UI evidence remains historical104 evidence with unchanged inputs and exact199 media. No Linux execution, real model/cloud/TG, production readiness or completed full goal is claimed.
