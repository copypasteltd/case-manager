# GAP-28 — 当前范围验收编号与证据边界

审查基线：`930cdde97b9d5d2e85d892664b0857a10a811739`，并只读查看正在编写的168批次工具。此次没有执行验证脚本、业务测试、构建或服务。

> “按已逐项分类的30项AC／72项TC继续补全四端业务旅程、可追溯证据、操作手册、平台／正式产物及当前范围发布验收；适用性分类不代表业务测试通过。”
>
> — `docs/delivery/04-current-scope.json:483`；同文要求见 `docs/delivery/03-goal-gap-assessment.md:198`。

**判定：** 编号、范围分类、证据关联与发布声明已有代码；完整正式验收仍为 partial／待外部事实判定。此次没有确认可独立追加的强制代码缺项，不能据此关闭GAP-28或其余22包。

## 要求与实现路径

- `docs/quality/04-current-acceptance-applicability.md:5` 明确全部30 AC／72 TC保留原文和摘要，分类不算通过；`:9`保留四端、正式产物、真实云／模型／TG、Linux隔离、生产PG、普通复制和恢复；`:16`把逐项执行证据交给增量记录。`:50`的AC-29继续要求共同首发资料和业务用例齐备。
- 原文追踪路径为 `docs/quality/02-traceability-and-release.md:9` 的用户／AC→细化需求→设计→工作包→TC矩阵，以及适用性JSON `:8`的原始AC／TC文件摘要与行数，`:22`的分类计数和逐项原文／行摘要。JSON `:5`–`:7`明确本文件通过业务数为0，完整目标与生产就绪均为false。
- `app/scripts/verify_acceptance_applicability.py:31`提取原始行并拒绝重复；`:76`的主流程核对修订99及四端，`:91`–`:102`核对两个原始来源摘要、总计102个唯一ID、原文和行摘要。`:103`–`:126`仅允许required／partial／excluded，分别保持acceptance_pending／scope_excluded，约束移出类别、保留AI权限与业务财务、派生计数、Markdown对应和23包开放。`:128`–`:135`产出源码绑定记录且执行／通过业务数仍为0；这条路径没有业务执行器。
- `app/scripts/verify_current_scope.py:39`–`:70`核对完整28编号、23 active／5 excluded、14 P0／9 P1、确切移出ID及人读表格，不把取消范围改成完成。当前范围没有因加速发版取消剩余普通功能。
- 已完成167的证据路径为 `docs/quality/current-functional-baseline.json:2`→其指定 `verify_batch_round167.py`；后者`:17`检查23包保持open，`:19`–`:21`读取整批源码归档并核对“测试前保存”。`:55`保留实际复用来源、延期验证及false正式验收声明。发布消费者 `app/scripts/verify_repository_publication.py:306`–`:321`实际调用当前指定verifier并对比保存记录、源码／脚本摘要，要求完整目标、生产就绪及正式Android验收为false。167检查报告不是本次168重跑的结果。
- 正在开发的 `app/scripts/development_wave.py:45`–`:61`在任何执行前要求全体active编号、每项独立审查记录、全部声明代码任务implemented、完整代码波次完成且acceptance_state仍open；`:99`–`:121`在执行前保存精确字节归档并拒绝源码不一致。工具存在不代表其检查已执行，也不证明尚在编写的168代码已全部完成。

## 搜索及反证

使用`rg --files`定位delivery／quality／scripts，再检索`GAP-28`、`AC-`、`TC-`、`acceptance-applicability`、`planning_scope`、`full_goal_complete`、`production_ready`、`retained`和`code_wave_complete`，找到上述清单、原文矩阵、真实验证入口与发布消费者。并非仅凭文档标题判定存在。

尝试反证“分类表等于完整通过”：表格正文、JSON、分类脚本、当前verifier及发布消费者均明确相反；23包也仍open。尝试把尚未执行的正式云／模型／平台／公司旅程列成缺少代码：当前要求及167完整编号表`:29`–`:31`保留了这些外部／延期条件，用户还要求代码整批完成后才统一测试，不能把延期重列为本轮先行测试门槛。没有找到要求新增通用测试引擎、自动关闭工作包或本轮重跑全部历史回归的原始强制条款。

## 未读与未验证

没有重新审计全部102项具体业务实现、所有历史证据字节、168新业务正确性、真实平台／云／模型／TG／服务器或公司制度采用情况；具体功能由各独立GAP审查承担。本报告不宣布这些检查通过，也不替代完成代码波次后应执行的适当验证、逐项失败修复、更新手册／媒体和提交发布检查。
