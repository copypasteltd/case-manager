# 增量103验证：构建依赖调用链修复

日期：2026-10-29（Asia/Taipei）。实现和完整操作见[开发说明](../development/103-dependency-consumer-remediation.md)。本轮推进GAP-26，整体仍为23项开放工作包（14 P0／9 P1），5项移出范围。

| 编号 | 实际检查 | 结果与来源 |
|---|---|---|
| V-01 | 干净安装 | 标准npm ci通过、锁字节不变；未使用force／legacy-peer。见[安装记录](evidence/dependency-remediation/clean-install.json)。 |
| V-02 | 实际调用 | 31项依赖检查＋2项原生JS检查通过，真实Metro／Taro、合成Git／HTTP／registry和图像死循环检查均在当前源码运行；所选依赖安装树有效。见[安全检查](evidence/dependency-remediation/security-checks.json)。 |
| V-03 | 便携客户端 | Web151、移动255、运行时52项测试；类型、Web／运行时及微信／抖音构建通过，两小程序各20页。见[命令记录](evidence/dependency-remediation/client-checks.json)。 |
| V-04 | Android | 原生Metro／Gradle构建、18项APK结构检查；源摘要同时绑定锁与本地适配源码。见[构建](evidence/dependency-remediation/build.json)。无设备／模拟器运行。 |
| V-05 | JVM | 26项结果：Gradle缓存，非本轮重新执行 / cached, not newly executed，原始任务状态／XML保留，见[范围](evidence/dependency-remediation/android-test-scope.json)。 |
| V-06 | 原生开发服务 | 只读核对schema81、当前任务API／Worker来源、无临时测试数据库，外部集成未启用。见[记录](evidence/dependency-remediation/development-project-resources.json)。 |
| V-07 | 保留证据 | 未改Go191顶层／1201结果及Linux交叉编译沿用102同源记录；107 PNG／41 MP4／41 GIF保持原字节和采集范围。见[复用清单](evidence/dependency-remediation/reused-evidence.json)。 |

| 完整扫描 | Critical | High | Moderate | Low | Total |
|---|---:|---:|---:|---:|---:|
| 原102锁，本轮新扫描 | 0 | 13 | 60 | 7 | 80 |
| 当前103锁 | 0 | 6 | 57 | 7 | 70 |

这是npm按包汇总的记录，含传播关系，不是独立CVE数量。两次扫描均完整纳入构建／开发依赖，原始报告、锁摘要及逐项剩余70条记录见[审计](evidence/dependency-remediation/dependency-audit.json)和[剩余清单](evidence/dependency-remediation/remaining-findings.json)。扫描命令返回1表示仍有告警；GAP-26和安全发布门槛继续开放。

首次安装仍解析到嵌套旧图像库，31项最终检查之前的30项初次检查有3项失败；原记录见[初次尝试](evidence/dependency-remediation/attempts/01-initial-resolution/result.json)。npm dedupe的peer冲突和准确锁节点重解析步骤见[解析记录](evidence/dependency-remediation/attempts/02-npm-resolution/result.json)。最终以标准干净安装、当前安装树和当前源码通过结果为准，不改写原失败。

完整审计首次达到180秒命令期限，未取得的报告不计成功；见[超时记录](evidence/dependency-remediation/attempts/03-audit-timeout/result.json)。确认原进程已停止后，改用直接Node／npm CLI执行重试，完整返回当前报告，依赖扫描范围保持完整。

原生构建成功后，归档脚本首次因Windows JVM混合编码而读取失败；见[归档尝试](evidence/dependency-remediation/attempts/04-native-log-encoding/result.json)。修复为从原始字节读取ASCII任务标记，日志保持原字节与摘要。JVM任务确认为UP-TO-DATE，26项历史结果单独标记缓存，4份XML逐项校验摘要。

English: Standard npm ci,31 real-consumer/security tests,2 native JS tests,Web151/mobile255/runtime52 tests and four-client local builds pass. Full audit80→70,high13→6; every remaining package finding stays open. Original189 media and unchanged Go191/1201 evidence remain historical; JVM cache scope is explicit. No device, emulator, network performance, cloud/model/TG or production acceptance is claimed.
