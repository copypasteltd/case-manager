# 秘书准确草稿恢复验证 / Exact draft recovery verification

增量112完成GAP-04／12的准确草稿跨运行恢复子流程，整个工作包继续开放。[设计、使用、协议与约束](../development/112-secretary-file-recovery.md)。

本轮源码核验：后端231个顶层测试／1410条结果，Web219、共享移动313、运行时55；八项客户端命令全部执行，双小程序各20页。Web与Taro H5各5组真实页面／API检查；新增7 PNG、2静音MP4、2 GIF，244份旧媒体保留原字节。Android开发APK通过18＋4项静态检查，26条JVM来自Gradle缓存；普通schema86→87升级、全耐久原生临时PG及2个Linux交叉构建已核对。无真机、模拟器、真实云／模型／TG或生产验收声明。下方旧增量的统计保留原执行日期与适用范围。

Current-source checks: 231 backend top-level tests / 1410 records, Web 219, shared mobile 313 and runtime 55; all eight client commands execute and each miniapp retains 20 pages. Web and Taro H5 each pass 5 actual page/API groups. New media: 7 PNGs, 2 silent MP4s and 2 GIFs; 244 previous media retain original bytes. A development APK passes 18 + 4 static checks; 26 JVM results came from Gradle cache. A normal schema 86 to 87 upgrade, fully durable temporary native PG and 2 Linux cross-builds are verified. No device, emulator, live cloud/model/TG or production acceptance is claimed. Older increment statistics below retain their original dates and scope.

| 编号 | 核验 | 实际证据与边界 |
|---|---|---|
| FR-V01 | 全部原生后端 | 231个顶层／1410条结果；全部发现项执行，无skip。每批至多20个名称，串行隔离库；[结果](evidence/file-recovery-backend/result.json)与[准确私有日志摘要](evidence/file-recovery-backend/raw-log-archive.json)。 |
| FR-V02 | 实际恢复业务 | 13组API／PG／MCP场景：原任务终态、准确选择与确认、过期草稿重建、严格工具字段、不可变来源、旧草稿／计划拒绝、二次恢复新扫描、正式提交原子回滚与幂等、新文件BOM／CRLF、隔离拒绝、上传中取消、工具依赖变化、后续隔离、会话撤销与新分片中止。使用受控原生证明、对象后端与扫描夹具，不是真实模型或云。 |
| FR-V03 | 跨端源码与构建 | [八项命令](evidence/file-recovery/client-checks.json)实际执行，Web219、移动313、运行时55；Web和移动各执行同一组14个共享边界算例，不能解释为28个不同业务场景；双小程序各20页。 |
| FR-V04 | 真实页面 | [Web](evidence/file-recovery-web/result.json)与[Taro H5](evidence/file-recovery-mobile/result.json)各5组：实际登录与MFA、原请求／差异读取、选择不自动发送、明确确认、新任务接单与持久来源、篡改摘要拒绝、原任务保持及页面宽度。无API响应替换。页面演示未执行真实模型。 |
| FR-V05 | Android | 当前源码生成新的Metro／Gradle开发APK，18项普通包检查＋4项恢复内容静态检查；26条JVM来自Gradle缓存，保留4份原XML与实际Gradle任务状态。不使用真机、虚拟机或模拟器；不声明正式签名／上架。 |
| FR-V06 | 媒体 | 新增7 PNG、2静音MP4、2 GIF；实际认证后合成页面像素，保留准确帧、时间及编码输入；[媒体证明](evidence/file-recovery/media.json)与[目视核对](evidence/file-recovery/visual-review.json)。原244份媒体保留原字节及适用范围。 |
| FR-V07 | 原生持久化 | 原开发实例正常schema86→87升级；历史AI费用／技术用量行数与内容摘要保留；新表只读／追加权限及5项恢复约束已核对；只替换核对过归属的API及2个Worker，外部接入关闭，不声明生产部署。 |
| FR-V08 | API与Linux | 325路径／401操作／713模型；全部旧路径语义保持，旧模型仅RunCreate及两类文件回执增加恢复字段。2个当前源码Linux amd64 ELF交叉编译，未在Linux执行或部署。 |
| FR-V09 | 资源释放 | 两端浏览器上下文、页面、临时服务与代理、隔离业务数据库结束后关闭；独立全耐久原生临时PG在全部检查后关闭。原开发PG与用户无关页面／进程保留。 |

早期页面准备尝试未给合成员工授予无业务动作的MFA要求角色，直接请求注册导致422；13个业务场景已通过，但尚未打开浏览器。该失败与准确源码ZIP／私有原始日志分别保留在[attempt01](evidence/file-recovery/attempts/01-browser-mfa-role/attempt.json)，没有放宽MFA。修正仅对显式启用的浏览器夹具生效。

修正浏览器夹具期间，首轮完整后端回归的源码发生变化；辅助器将第4批标记为失效并拒绝后续批次。前三批只对旧源码有效，没有并入最终结果。[attempt02](evidence/file-recovery/attempts/02-backend-source-refresh/attempt.json)保留原批次记录和准确日志；旧源码来自attempt01的源码ZIP，修正后当前源码另存ZIP。最终完整回归从头重新发现并执行所有批次，全部绑定当前源码。

当前源码回归在第5批及Android打包期间又发生外部进程中断；原句柄消失，Windows进程检查确认对应测试／构建／PG均已停止。[attempt03](evidence/file-recovery/attempts/03-process-interruption/attempt.json)保留准确中断日志与原辅助器。已经完整通过的第1～4批绑定相同当前源码与准确日志，因此保留原执行事实；第5批从头重跑，其余批次继续执行，没有把中断片段拼成通过。Android从头构建新的完整产物，原中断bundle不作成功依据。

原生PG按准确原目录恢复，没有重新初始化；开发服务恢复使用独立脚本，不重跑schema86→87，也不覆盖原升级证明。最初恢复辅助器遗漏显式端口参数，开发PG临时启动在默认loopback5432，被断言拒绝；核对准确原生进程、目录、端口后只停止该自有进程，再以14660启动。应用角色无权读取`data_directory`的拒绝结果保持，未扩权；该辅助失败另行保留。本轮最终检查均在恢复后的实际资源上重新核对。

早期局部开发日志保留在忽略目录：编译夹具、取消状态断言、严格字段错误码及实际扫描枚举修正不计入最终通过。已通过的局部试跑不代替最终全量回归。依赖专项沿用增量108实际日期的完整审计0项及既有安全结果，本轮不声称重新检索公告或重新审计；根manifest／锁及依赖字段不变，仅新增contracts源码导出和移动测试选项。

首次仓库发布检查因输入清单提前列入尚未生成的`repository-check.json`而中止，未生成通过报告；[attempt04](evidence/file-recovery/attempts/04-publication-bootstrap.json)保留实际错误及检查器摘要。第二次完整检查的安全与链接检查通过，但误将媒体及准确原始构建日志按代码／指南行尾规则处理，[attempt05](evidence/file-recovery/attempts/05-publication-input-classification.json)保留实际失败报告。已将生成报告作为输出、将媒体与原始日志排除出文本排版规则；这些文件仍参加完整候选／暂存字节、敏感信息及链接检查，并保持原字节。后续完整候选检查与实际暂存字节检查分别执行，不以失败尝试计为通过。应用源码与业务测试输入未变。

真实Codex接入、Linux隔离验收、八项Skill正式发布／评测、实际OSS／R2／TG、正式Android与小程序发布、生产部署及公司完整业务旅程继续见[23项稳定编号](../delivery/03-goal-gap-assessment.md)。设备验证、网络性能／实网120GB、历史数据迁移／容灾和AI费用控制保持移出；不把取消范围记为通过。
