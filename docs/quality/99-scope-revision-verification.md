# 修订99：范围与清单验证 / Scope revision verification

> 本文记录修订99当时的规划与代码差异。费用前置问题已在增量100实现，后续功能验收见[增量100](100-secretary-execution-verification.md)；原99检查记录保留。

[当前范围](../delivery/04-current-scope-and-acceptance.md) · [机器清单](../delivery/04-current-scope.json) · [23项剩余工作](../delivery/03-goal-gap-assessment.md)

本轮依据用户指令调整交付范围，第四项以用户更正后的“AI费用控制”为准。保留四端、秘书、Skill、TG、隔离与权限；取消真机、网络性能、数据迁移／容灾及AI费用专项。

| 范围 | 验证口径 |
| --- | --- |
| 编号与数量 | 原28个稳定ID；23 active、5 excluded；active为14 P0＋9 P1；本次不新增完成项 |
| 排除项 | GAP-14／17／21／22／27不再阻塞当前范围；局部移出内容同步修改保留项 |
| AI更正 | 仅费用控制移出；执行隔离、授权、审计和技术资源约束继续保留 |
| 已知代码差异 | 当前接单仍存在AGENT_BUDGET_UNCONFIGURED；解除必配预算前置条件计入GAP-09，未宣称已修改运行时 |
| 原实现与检查 | 功能基线继续为98，原源码、测试、APK和历史记录保留；没有重跑或扩大原生／云／模型／设备验收范围 |
| 媒体 | 97 PNG／35 MP4／35 GIF逐项保留原Git对象；本次不启动浏览器或采集新媒体 |
| 发布 | 验证清单、范围标记、文档链接、候选／暂存字节及秘密隔离后提交推送 |

当前范围机器检查入口为 `python app/scripts/verify_current_scope.py`。它检查清单计数、排除／保留语义、17份原发布快照、旧功能来源摘要、媒体对象及本次文档的一致性；它不执行模型、云、TG或业务写入。

本轮记录放在[独立证据目录](evidence/scope-revision-99)，原始范围快照见[publication-before](evidence/scope-revision-99/publication-before.json)。范围取消不会覆盖旧检查失败、把缓存改称新执行，或自动建立生产就绪结论。

首次发布核对发现旧质量文档及其原始快照的尾空行，失败摘要保留在[原尝试](evidence/scope-revision-99/attempts/01-original-snapshot-format/repository-check.json)。当前文档已整理格式；17份原始快照保持原Git字节，不参与当前代码／指南格式整形，继续由来源校验及全仓库秘密／路径扫描覆盖。
