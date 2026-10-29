# 文件版本正文比较验证 / File version comparison verification

增量113推进GAP-04／18，整个工作包继续开放。[协议、使用和实现约束](../development/113-file-version-comparison.md)。

当前源码核验：后端234个顶层测试／1424条结果，Web238、共享移动332、运行时55；八项客户端命令实际执行，双小程序各20页。Web与Taro H5各6组实际页面／API检查；新增7 PNG、2静音MP4、2 GIF，255份旧媒体保留原字节。Android开发APK通过18＋4项静态检查，26条JVM来自Gradle缓存；schema保持87，无新增或重跑迁移；独立全耐久原生PG及2个Linux交叉构建已核对。不声明真机、模拟器、真实云／模型／TG或生产验收。下方旧增量统计保留原日期与适用范围。

Current-source checks: 234 backend top-level tests / 1424 records, Web 238, shared mobile 332 and runtime 55; all eight client commands execute and both miniapps retain 20 pages. Web and Taro H5 each pass 6 actual page/API groups. New media: 7 PNGs, 2 silent MP4s and 2 GIFs; 255 previous media retain original bytes. A development APK passes 18 + 4 static checks; 26 JVM results came from Gradle cache. Schema remains 87 with no new or replayed migration. Full-durability native temporary PG and 2 Linux cross-builds are checked. No device, emulator, live cloud/model/TG or production acceptance is claimed. Older increment statistics below retain their original dates and scope.

| 编号 | 核验 | 证据与实际边界 |
|---|---|---|
| VC-V01 | 完整后端 | 234顶层／1424条结果；全部发现项执行，无skip。至多20个名称一批、串行原生隔离库；[完整结果](evidence/file-comparison-backend/result.json)、[准确私有日志摘要](evidence/file-comparison-backend/raw-log-archive.json)。 |
| VC-V02 | 算法 | 两个测试族覆盖307组固定／随机输入：独立还原两侧完整内容，以独立LCS动态规划核对小输入最少编辑；还测Unicode、空行、空正文、末尾换行、长行、上下文省略计数、资源拒绝与取消。600行变化触发完整中段替换降级，不截断冒充完整差异。 |
| VC-V03 | 实际API／PG | 11组：未就绪拒绝、逐页完整还原、前后文／方向、同版本／规范化相同与原件不同、无业务写入、越权／跨文件／不支持、严格参数与固定摘要、回收站／隔离／分类变更、元数据修订、会话撤销、CSV／JSON／DOCX实际提取。原件由受控对象读取器提供，经实际索引Worker及事务；不声明真实云。 |
| VC-V04 | 四端客户端 | [八项命令](evidence/file-comparison/client-checks.json)均实际执行；Web238、移动332、运行时55。Web／移动各执行同一19个共享边界用例，不能算38个不同业务场景。微信／抖音各20页构建；不表示平台提交成功。 |
| VC-V05 | 实际页面 | [Web](evidence/file-comparison-web/result.json)与[Taro H5](evidence/file-comparison-mobile/result.json)各6组：实际认证、选择不读取、准确历史版本、差异与省略、脚本文字、全文分页摘要还原、原件／正文区别、篡改固定摘要与真实改名后的旧页拒绝、清除旧结果、页面宽度。无响应替换、状态注入或直接数据库修改代替页面操作。 |
| VC-V06 | Android | 当前源码生成新Metro／Gradle开发APK，18项包检查＋4项业务内容静态检查；26条JVM来自Gradle缓存，保留四份实际XML及任务状态。未启用实体设备、模拟器或虚拟机；正式签名／发版继续GAP-16。 |
| VC-V07 | 媒体 | 7 PNG、2静音MP4、2 GIF，实际认证后合成页面像素；[准确帧与编码](evidence/file-comparison/media.json)、[目视核对](evidence/file-comparison/visual-review.json)。255份历史媒体按已发布字节保留。 |
| VC-V08 | 持久化与服务 | schema仍87，无迁移新增或重跑。只替换核对归属的开发API和两项Worker，历史费用／用量资料摘要及行数保留；[更新](evidence/file-comparison/development-update.json)、[只读检查](evidence/file-comparison/development-project-resources.json)。未启用真实云、模型、TG或生产接入。 |
| VC-V09 | API与Linux | 326路径／402操作／717模型；原325路径／713模型内容不变。当前源码两个Linux amd64 ELF交叉构建；未执行目标Linux内核隔离或部署。 |
| VC-V10 | 资源释放 | 两端自有浏览器上下文、页面、HTTP服务和移动代理、隔离测试数据库均结束并关闭；所有后端检查后关闭独立全耐久原生临时PG，0残留测试库。原开发PG和用户其他进程／页面保留。 |

开发服务首次更新时，已核对归属的API终止命令返回非零，辅助器停止，未继续启动替代进程。后续独立检查确认原API已退出、18080端口已释放、两项原Worker仍运行。修正为“命令执行后重新核对实际归属与退出状态”，再启动／检查当前源码服务；不凭终止命令的返回值认定服务已就绪。[该辅助失败](evidence/file-comparison/attempts/01-development-stop-exit-code/attempt.json)保留准确原辅助器摘要，不重新标记成通过；没有改应用源码或复用失效业务测试。

运行入口：原生测试数据库由 `file_comparison_native_cluster.py prepare` 创建；`verify_file_comparison_backend.py` 执行完整发现项；`verify_file_comparison_clients.py` 执行八项命令；`run_file_comparison_browser.py` 分别检查Web与 `--mobile`；Android使用 `build_android.py --google-mirror --evidence-group file-comparison` 和两项包检查；Linux使用 `build_file_comparison_linux.py`。原生集群结束时 `file_comparison_native_cluster.py stop`；之后运行 `verify_file_comparison_sources.py`、`verify_acceptance_applicability.py`、`verify_current_scope.py` 与仓库候选／暂存区检查。凭据和准确私有日志不入Git。

依赖保留增量108实际日期的完整npm审计0项及安全结果，本轮不声称重新审计或更新公告。根manifest、锁与依赖字段不变，仅新增contracts源码导出和移动测试选择。真实Codex、Linux隔离、八项Skill正式发布／评测、实际OSS／R2／TG、正式Android与小程序、生产发布及完整公司旅程继续见[23项稳定编号](../delivery/03-goal-gap-assessment.md)。真机、网络性能／实网120GB、历史数据迁移／容灾及AI费用控制维持移出；取消范围不记成测试通过。
