# 增量119核验 / Increment119 verification

本轮新增原生发布工程代码，业务应用源码保持已发布118。40项本轮测试通过、无跳过；完整本地检查包包含784个文件、三个Linux ELF、Web、独立运行器和91项准确结构清单。它以118为基线并明确记录本轮未提交工程源码；不是正式CI或生产部署产物。

Increment119 executes new release-engineering tests and builds/inspects an actual server archive. The application regression remains the exact published118 evidence. No Linux service, HTTPS endpoint, real CI Runner, production database, cloud, model or Telegram acceptance is claimed.

| 编号 | 本轮检查 | 证据与实际范围 |
|---|---|---|
| 1 | 准确源码测试 | [最终40项](evidence/native-release/tests-v6.json)：归档路径／链接／重复成员／边界、固定顺序、配置、并发、切换、部分升级、中断、回退及秘密边界；原日志和源码ZIP留在私有目录 |
| 2 | 实际构建 | [构建记录](evidence/native-release/build.json)：三个Linux程序交叉构建，加Web及运行器两项新构建；真实工具版本与私有编译日志摘要保留 |
| 3 | 实际归档 | [最终核对](evidence/native-release/verification.json)：解开真实归档，核对784项准确字节、91项结构、依赖图、八个draft Skill、服务单元和已知私有值 |
| 4 | 工作流 | 使用已安装YAML库解析真实工作流，核对手动触发、原生标签、固定Action提交和限定上传范围；没有触发真实CI作业 |
| 5 | 本地执行边界 | Windows调用生产激活入口实际拒绝；状态机的进程效果及指针操作使用明确测试替身，未执行Linux符号链接、systemd、Nginx或生产SQL |
| 6 | 保留应用证据 | 全部应用输入与118准确对应；复用后端240顶层／1492结果、Web311、移动407、运行器56、原浏览器及开发包检查，均不是本轮重跑 |
| 7 | 保留媒体 | 316份已发布PNG／MP4／GIF及原118证据按Git原字节核对；本轮无新页面、无新截图或录屏 |
| 8 | 范围及发布 | [范围](evidence/native-release/scope-check.json)、[AC／TC适用性](evidence/native-release/acceptance-applicability-check.json)、[仓库预检](evidence/native-release/repository-check.json)；这些检查不执行完整业务或生产验收 |

本机不具备创建符号链接的权限。最初22项检查中涉及真实链接的11项失败，保留了对应源码和单项失败重现日志；随后使用明确依赖注入完成状态机检查，生产CLI仍只接受原生Linux root，不新增Windows部署替代入口。CI在真实Linux上使用原生链接测试实现；本轮尚未执行该CI。

第一次完整打包在三个公开依赖元数据文件处被拒绝，第二次发现宿主Path排序与规范POSIX路径顺序不同。已增加明确文件名规则和跨目录／大小写排序用例。v1的27项、v2的31项及其完整源码和构建日志按当时来源保留；它们不是最终通过证据。[早期记录摘要](evidence/native-release/preliminary-attempts.json)说明已保留的范围，不把缺失的原始完整控制台输出补写成日志。

首次最终记录生成使用相对构建路径，归档检查后编码记录路径失败；已规范为准确绝对路径并重新执行检查。该验证脚本修正不改变v3当时构建输入或归档。其后新增日志中断用例并修正协调器，事件修复后的v4构建及36项通过保留原来源；随后修正公开目录权限和连接URI规范化，最终40项测试与v5实际构建采用新来源。v3／v4旧归档及此前通过记录仅保留为旧来源证据。旧验证器在v3私有源码ZIP中保留。

最后四项新增用例实际重现并修复了“事件已落盘、pending尚未更新”、恢复目标落盘中断、事件临时写入中断及摘要链损坏边界。修复前四项均失败，准确源码和原日志见[中断重现](evidence/native-release/journal-interruption-attempt.json)；最终测试不覆盖或删除早期失败。完整事件采用原子落盘，只读状态按既有摘要锚点核对并采用完整尾部，恢复继续其已记录目标，链损坏在服务操作前拒绝。

最终审查补齐受限umask下公开发布／资源祖先目录的明确0755权限，以及libpq URI角色／数据库解码和连接参数白名单。修复前的四项定向检查有11条失败结果（含子用例），原来源见[连接与目录记录](evidence/native-release/permission-dsn-attempt.json)。Windows目录模式结果不等于POSIX权限执行；真实Linux权限仍需原生CI／目标服务器验收。

40项测试检查真实本地归档／文件／配置／操作记录，并注入系统进程和Windows指针原语；不是服务模拟器或虚拟机。Linux链接权限、实际系统身份、维护入口是否可访问、已运行旧进程范围、配置证书、迁移身份和真实Worker／模型资源隔离仍需服务器证据。实际跨云对象与完整业务旅程继续属于原工作包。

本轮没有新依赖或锁文件变化，沿用118明确引用的零公告结果，不声称重新查询公告或完成正式产物安全签核。Android仍为原118开发包检查范围，26条JVM结果保留原Gradle缓存属性。

23项工作包（14 P0／9 P1）、5项移出不变。GAP-24新增发布工具代码；真实Runner／系统服务／HTTPS／生产PG和正式发布安全分别继续在GAP-24／25／26，完整验收为GAP-28。没有新增真机、性能、历史迁移、容灾或AI费用控制门槛。

[设计](../development/119-native-production-release.md) · [完整操作手册](../operations/03-native-release-runbook.md) · [全部剩余项](../delivery/03-goal-gap-assessment.md)
