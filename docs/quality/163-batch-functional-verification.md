# 增量163统一验证 / Whole-wave verification

完整C01–C07源码与前置用例先完成，全部23编号逐项复核后才保存初始源码快照并开始统一测试；首次失败、集中修复及每次实际输入分别保留。详细结果在完成统一检查后补入，不以计划代替通过。

需求一致性复核追踪AI-09文件写入、AI-10准确整理及PM-05归档的实际调用链，并独立反向核对登录守卫、异步I/O、最小范围回执与目标页权限；另核对GAP07／08复制运维入口。其余编号保持现行实现／配置／规则及正式验收边界，未声称全量安全审计或全部业务通过。

新迁移129只追加原始登录来源与守卫。旧原文、哈希、回执、八个冻结和以前媒体／升级保留；旧无会话来源可读但不可执行，明确新恢复绑定新登录。正式已提交副本复制仍使用独立普通权限，不被员工退出取消。

No real model, Bot, cloud, production Linux/PG or formal Android/platform acceptance is inferred. No Docker/WSL/VM/emulator is used. Real devices, network/live120GB performance, historical migration/DR and AI fee controls remain excluded. Classification does not pass business cases.

## 统一检查实际结果 / Executed checks

- 测试前保存完整源码ZIP和逐文件摘要，C01–C07全部implemented，23编号处置齐全。
- 首轮native失败保留；旧工具数量断言过时和恢复用例未明确取消分别修复，仅重跑受影响两组，10个测试条目及1包结果通过。其余首轮通过按原输入保留，未冒称整轮全量回归。
- 首轮客户端保留类型错误与因此跳过构建。集中补齐既有固定回执类型与旧fixture授权字段后，当前九步骤通过：Web527／共享移动553（240+308+5）／runtime66／八项draft Skills／类型／Web／微信与抖音构建。
- 13个新Python工具和3个新采集JS完成语法检查。实际浏览器与同版164产物结果分别记录，不以源码通过代替。

[首轮及修复处置](evidence/batch-round-163/initial-and-repair-disposition.json) · [准确计数](evidence/batch-round-163/test-summary.json)

采集首轮保留：Web已读取授权状态，但录制等待了不存在的普通读取响应；移动录制把公开secretary页面路径误判为secret字段。采集脚本改为现有event-log响应，并严格白名单核对移动路径和kind／id／run参数；不放宽密码、MFA或私有能力展示检查。仅采集脚本变化，既有应用用例保持准确输入保留，当前媒体在相同采集源码下重新生成。

当前Web-final／mobile-final均通过真实本地API与PG读取，最终采集源码一致。4张准确截图、2段实际MP4、2段操作GIF和2段截图轮播已生成；4张截图与2个GIF终帧逐张复核，实际摘要记录于visual-review。自建Edge上下文、临时HTTP／构建进程／PG、私有fixture全部清理。[媒体入口](../media/batch-round-163/README.md)。

源码／证据20项、范围12项、适用性12项及候选仓库20项通过。发布清单曾提前列入尚未生成的自身输出，引发FileNotFoundError；移除虚构路径并生成实际结果后重新核对，记录保留于publication-initial-failure.json。此修复仅涉及发布元数据，不改变应用输入。

媒体交付预检发现旧rc.8完整媒体已93.5MiB，此后新增约51.3MiB，超过单Git文件上限。C06打包修复保留完整ZIP于Git外，以每卷<=90MiB交付，逐卷及重组全文SHA256固定在manifest；重组工具拒绝缺卷、损坏、乱序、越界路径和覆盖现有输出。封存前完整ZIP逐成员核对源Git字节，历史媒体不删减。此前通过的源码证明另存source-check-before-media-parts.json，当前证明在打包修复后重新核对；应用和实际采集输入不变。

打包修复实际检查：媒体分卷7项（真实ZIP按字节重组、截断／等长损坏、缺卷、乱序、越界路径、全文摘要和已有输出保护）通过；当前源码证据21项通过，15个Python工具语法检查通过。只重跑受影响打包检查；保留原先20项通过证明及对应完整源码快照。
