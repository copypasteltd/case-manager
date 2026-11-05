# 157 · 整批功能开发与统一调试 / Batch implementation and debugging

本轮落实用户新增开发约束：先复核全部23个编号，完成本轮已明确、可实施的整批功能代码及共享接口，再进入统一编译、测试和集中修复。不得一边增加业务模块、一边逐模块宣称收口；发现的问题保留首轮记录，集中修复后按受影响范围复查。

这条约束不把未知公司制度、尚未提供的外部配置、复杂格式能力或实际验收变成已完成。完整编号映射见 [代码清单](157-batch-code-plan.json)；全部剩余项见 [交付清单](../delivery/05-functional-closure-157.md)。

## 整批交付

| 编号 | 内容 | 使用逻辑与边界 |
| --- | --- | --- |
| C01 | 秘书更新已有 DOCX／XLSX／PPTX | 本人固定准确文件和原版本，读取有界原件工作区，按准确单元修改；复用既有 Office 编辑器与授权、上传、扫描、晋升、人工正式提交流程。原件及未改动压缩部分保留；不等于复杂排版编辑器。 |
| C02 | Skill 评估、发布、暂停与回退依据 | 选择准确已安装配置，核对代码摘要和当前活动任务；发布须关联本人该配置的成功运行。决定追加留痕，暂停影响后续执行核对。回退先由部署身份安装原已发布代码，再关联旧发布；不自动恢复停用配置或回滚业务事实。 |
| C03 | 秘书草稿放弃恢复与已知对象清理 | 本人对已停止、尚未正式提交的准确草稿单独确认永久放弃恢复；不能删除事实或绕过活动恢复后继。达到至少24小时观察期后，才可进入普通已知上传对象清理。未知／不确定／复杂派生对象继续受保护。 |
| C04 | TG 八类请求转门户人工授权 | `/task <准确Skill编号> <要求>` 只登记请求；写操作不由群消息自动授权。原绑定员工在相同项目的私人门户会话核对请求、最近身份验证，并另选准确业务操作后发送。授权绑定原请求、配置、身份和版本。 |
| C05 | TG 完成结果独立审核回群 | 任务完成后，原员工读取准确结果，选择0起始位置及1–2000个 Unicode 字符片段，明确同意公开到原群和话题；复用实际投递台账、限流、未知结果与人工核对。未知结果不自动重发。 |
| C06 | 四端共享入口 | Web 及微信／抖音／Android共享源码同步上述入口和回执核对；Android源码与旧开发APK分别说明。 |
| C07 | 项目和财务入口修正 | 风险／问题的可选日期、概率可明确清空；里程碑日期继续必填。财务报表提示指向已经存在的现金、预测、摊销入口。 |
| C08 | 全部编号映射及统一执行器 | 23项逐一关联现有实现，保留整体验收状态；原生执行器先检查完整代码清单，再统一收集结果，不遇首个失败就转为单模块交付。 |
| C09 | 对应回归与使用约束 | 真实 Office 二进制、准确恢复、永久屏障、发布权限、TG授权及群披露用例；Web和共享移动采用同一组授权边界用例。合成模型驱动不代替真实模型。 |

## 数据与接口

新增普通结构升级114–117：`agent_skill_releases`、`agent_file_abandonments`、`telegram_human_handoffs`、`telegram_handoff_reviews`。历史1–113不改。新台账不可更新或删除，业务命令与审计在同一事务中完成。

- `GET/POST /v1/agent/runtime-profiles/{id}/skill-releases`：准确配置发布台账；历史按完整版本向前分页。
- `POST /v1/runs/{id}/file-drafts/{draft}/abandon-recovery`：本人停止草稿的永久恢复屏障；不是物理删除命令。
- `GET /v1/me/telegram/requests/{id}/handoff`：本人请求及授权资格；不会授予文件或项目写权限。
- 在普通 `RunCreate.telegram_handoff` 中提交准确请求授权；所有原有业务授权仍分别必需。
- `GET /v1/me/telegram/requests/{id}/outbound-preview` 与 `POST .../outbound-review`：准确结果预览与一次独立群披露决定。
- `company/read_office_workspace`：仅对本人已授权的准确原文件读取有界工作区；`create_file_draft` 的文本编辑与Office单元修改互斥。

`file-update` 升为 **1.6.0 draft**，其余七项继续原版本且全部仍 draft。新增发布台账不冒充真实评估，也不自动把任何包发布。没有发布记录的旧安装配置仍保留开发兼容行为；正式启用需要公司评估与实际部署。

## 运行和版本

只使用原生工具，不使用Docker、WSL、虚拟机或模拟器。现有开发PG100/API不替换；测试拥有独立临时数据库。完成浏览器检查后关闭全部自建上下文、页面、服务和临时数据库，保留用户其他进程。

先执行 `python app/scripts/run_batch_round.py freeze-code --attempt <唯一名称>`，再执行 `native-tests` 和 `client-checks`；每次保留源码摘要、失败与通过记录。修复后的复查使用新名称。构建验证与实际云、模型、Linux、正式签名及平台发布验收分别记载。

本轮源码157目标结构117；**八个旧冻结版不变，rc.8／开发APK154仍为业务153／结构113**。当前源码不因JS构建而进入旧APK，也不自动生成新冻结版。真实服务器、OSS/R2、Codex、TG、小程序资料按用户答复后续提供。

## English

Complete the declared implementation wave across all23 active package mappings before a unified check/debug wave. Failures are retained and fixed in batches; no package is accepted merely because an entry point exists.

The wave adds exact-original secretary Office edits, human Skill release decisions/runtime gates, permanent stopped-draft recovery abandonment with known-object retirement, eight-task TG intake with portal authorization, and independent exact-fragment disclosure to the original group. Ordinary permissions, recent human MFA, immutable evidence, current-version checks and independent human business acceptance remain required. Unknown objects, complex Office/layout and actual integration acceptance remain open.

Normal migrations114–117 extend schema117. File-update1.6.0 and all eight Skills remain drafts. Existing rc.8/APK154 are immutable business153/schema113 artifacts. Source157, local builds, controlled model drivers and synthetic media do not establish a new APK, actual provider integration or production acceptance.
