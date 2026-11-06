# 增量161：完整代码波次后的选定验证 / Selected unified verification

本轮C01–C08的代码、接口、升级、四端共享调用、运行器／Skill与用例全部写齐后，才生成`freeze-code-initial.json`并开始统一检查。全部23项工作包在代码计划中逐项梳理。初始失败保留，修复集中成批，只重跑受影响范围；没有逐模块发布。所有验证均用Windows原生工具；没有Docker／WSL／虚拟机／模拟器。

| 编号 | 检查／证据 | 实际解释 |
|---|---|---|
| V01 | [完整代码记录](evidence/batch-round-161/freeze-code-initial.json)、[代码计划](../development/161-batch-code-plan.json) | 两批首次检查之前，8项实现及23项处置均已记录；这不等于23项正式验收完成。 |
| V02 | 本轮任务协作、一次性TG更新、普通范围转移、固定原群通知四组原生HTTP／PG流程 | 固定版本与当前权限、原始会话／近期MFA、不可变回执、一次消费／回放、越权拒绝及旧授权失效；选定执行不等于全后端回归。见[最终选定原生记录](evidence/batch-round-161/native-tests-batch-11.json)：10条结果（9pass含8条测试及1条包结果，1条浏览器fixture因本命令未启用而明确skip；实际浏览器fixture另有执行记录）。 |
| V03 | [早期选定检查](evidence/batch-round-161/native-tests-batch-2.json)、[后续边界检查](evidence/batch-round-161/native-tests-batch-8.json) | 原有普通范围转移、任务创建、历史恢复、扫描例外、组织工具和TG人工转接的成功条目分别保留；这些整批记录含其他失败／跳过，不能把整份标成通过。 |
| V04 | [当前客户端检查](evidence/batch-round-161/client-checks-batch-13.json) | UI修复后类型检查、微信、抖音构建通过；Web构建保留batch3的同一Web输入，绑定实际不变输入。未新跑Android JS、未新建APK，也没有正式签名产物。 |
| V05 | [保留客户端单测处置](evidence/batch-round-161/retained-client-dispositions.json) | 本轮先前Web520、移动主组548＋新增4、运行器66通过；初始正式签名输入Python3项／8个draft目录核对通过。之后移除Web TG表单一个不支持的MFA属性，并修复移动恢复队列加载根；类型／小程序构建及移动页面另验；上述单测未再次执行。 |
| V06 | [原生命令构建](evidence/batch-round-161/native-command-build.json)、[准确保留依据](evidence/batch-round-161/native-build-disposition.json) | Windows服务构建成功；此后仅浏览器合成fixture的客户端通道及移动UI加载根改正，命令编译所用的应用／SQL／运行器未变，未重建命令。不是Linux安装或生产验收。 |
| V07 | [实际页面与交互](../media/batch-round-161/README.md) | Web任务参与人→完整26项关联分页→普通保存；共享移动端参与人→保存回执→准确任务→本人历史恢复队列。六张截图、两段无声实际视口录屏、两项交互GIF和两项截图导览；合成资料、不调用真实模型。 |
| V08 | [初始失败／批次修复](evidence/batch-round-161/batch-repairs.json)、[最终检查](evidence/batch-round-161/final-source-check.json) | 原始代码ZIP＋逐批差异保留，失败不覆盖；最终源、媒体哈希、浏览器与临时服务清理、范围及旧冻结不可变性分别核对。 |

## 保留失败与准确修复

首次统一检查发现Go规范化JSON入参、类型／空值、MCP定义顺序、旧用例枚举／版本等问题。后续批次修复真实约束连接：PL/pgSQL分支记录字段、准备事件与执行队列分离、普通范围命令后的独立终态结果、不可变授权记录行锁，以及原群入站回执与审计字段类型。通知回放用例改为规范化JSON比较，未改变通知身份或重发语义。

本地媒体保留合成登录通道不匹配、原生初始化临时文件冲突、录制页面白名单和页面文案／保存回执定位失败。首次回溯记录关于角色变化的诊断是暂定，后续确认合成HTTP token需匹配固定Android请求头。实际浏览器另行登录。后续原生集群准备串行执行；录屏转换等所有自建会话关闭后进行。每次失败均保留原始结果，成功素材另起编号。

最后视觉复核发现的H5重复加载占位已修复：加载期间不挂载独立队列子树，完成读取后只挂载一个当前队列。保留中间Busy属性类型失败；最终类型与微信／抖音构建重跑通过，mobile6检查唯一队列。后端及Web应用字节未变，准确复用原证据；不把视觉查看记为正式平台验收。

## 当前未执行的接受事项

本轮没有全后端／全客户端回归，没有新增Android JS／APK／AAB／正式签名，没有真实Codex、Linux隔离、OSS／R2、TG提供方、微信／抖音平台、目标服务器或生产PG验收。配置仍按用户安排后续提供。八项Skill仍draft、23项工作包仍开放；详见[编号清单](../delivery/05-functional-closure-161.md)。

DevelopmentPG100/API未改；源码schema127正常升级124–127，旧迁移1–123不改。八个冻结标签不改；rc.8／APK154仍为business153/schema113，不含155–161。用户移出的五项不作为发布门槛，也不记为通过。

The complete declared code/fixture wave precedes testing. Current selected native business checks, current client type/build checks and actual Web/H5 interactions are distinct from retained unit/build evidence. Initial failures and exact source archives remain immutable. A synthetic native Bot update is not real Telegram delivery; queue acceptance is not delivery or independent business acceptance. No broad regression, real service, formal signing or production readiness is asserted.
