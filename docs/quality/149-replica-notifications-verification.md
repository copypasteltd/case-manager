# 增量149：复制失败／恢复通知核验与延期

| 编号 | 对象 | 当前实际范围 |
| --- | --- | --- |
| Q149-01 | 原生HTTP／PG | 实际复制工作器摘要失败、失败通知、已读、人工重试排队、完整字节校验、恢复新未读、重复抑制、初次正常复制无误报、延迟事件与撤权；含四端真实会话 |
| Q149-02 | 记录数 | 新套件12业务子场景＋1未启用浏览器钩子＋1组汇总为14条pass，另既有通知回归1条，总15；不将钩子或汇总计为独立业务通过 |
| Q149-03 | 便携客户端 | Web5、共享移动逻辑5；类型、Web／微信／抖音当前构建通过；无新APK／Metro／JVM／移动UI执行 |
| Q149-04 | 实际Web | 3项实际操作，390／768／1440无横向溢出；真实API重新读取通知、打开准确版本／副本、确认序号2已读 |
| Q149-05 | 媒体 | 三原始PNG；2.8秒实际无声操作MP4；6秒三截图导览GIF。已查看三PNG，并抽样查看MP4／GIF；不是逐帧完整审查 |
| Q149-06 | 来源核验 | 22项当前来源／记录核对；捕获时1361项构建输入原字节保持，只读核验模块在捕获后新增并单独绑定；浏览器来源清单另外绑定实际代码及执行检查器 |
| Q149-07 | 复用 | 冻结148／业务147开发APK和交付证明，完整客户端136、完整后端121、运行器145按原来源保留；原证明未重新执行 |
| Q149-08 | 清理与发布 | 自建浏览器／上下文、临时服务器／API及耐久测试PG全部关闭；原开发PG100／API未动。五次冻结与结构1–111不修改 |

证据：[当前目录](evidence/replica-notifications/)、[实际Web](evidence/replica-notifications-web/)。首轮`native-focused.json`失败：测试账号尚未要求MFA，登录没有注册挑战，测试准备因此被拒。`native-mfa-policy.json`通过：仅隔离测试角色明确启用MFA要求，再走真实注册和四端会话。失败报告及准确原测试源码均保留；生产权限未改。

`capture-source-scope.json`在新增当前只读核验模块之前，准确保存捕获时1361项构建输入及原摘要。当前核验逐项确认原输入仍一致，仅增加`verify_replica_notifications.py`，后者由当前摘要和自身SHA另行绑定。未改写浏览器原摘要，也不据后加检查文件宣称曾执行它。实际浏览器`source-manifest.json`另有完整执行来源清单。

本轮不运行完整业务／运行器回归、额外移动UI或新APK。开发PG100没有升级，结构111及五次冻结仍准确原字节。实际云、真实模型／TG、目标Linux／生产PG、正式Android签名／平台发布和公司完整验收均未完成。八Skill仍draft，23工作包保持active；本轮只完成GAP-08／18中的具体子项。

移出项不重设门槛：真机、网络性能／实网120GB、历史迁移／容灾、AI费用控制。保留AI授权／隔离／审计／技术资源限制、正常升级、普通恢复、PG和OSS＋R2。未使用Docker、WSL、虚拟机或模拟器。

Fresh149 checks cover the concrete in-app worker-failure/recovery loop and exact-version entry. Synthetic storage transport executes the real application worker; it does not establish real OSS/R2 acceptance. Full suites and APK148 stay retained. No production deployment or full-goal completion is claimed.

[使用与开发](../development/149-replica-failure-recovery.md) · [本轮及全部待完成项](../delivery/05-functional-closure-149.md)
