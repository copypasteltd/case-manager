# 通知失败人工重试验证 / Notification recovery verification

增量110完成GAP-08的站内人工恢复子功能；完整23项工作包继续开放。AI仅移出费用控制。[设计、权限、使用与命令](../development/110-notification-retry.md)。

当前源码检查：后端229个顶层测试／1383条结果，Web195、共享移动289、运行时55；八项客户端命令全部重新执行通过，微信／抖音各20页。Android开发APK通过18＋4项静态检查，26条JVM结果来自Gradle缓存。Web与Taro H5各5组实际浏览器／API／租约Worker操作通过；新增7 PNG、2段静音MP4、2个GIF，原222份媒体字节保留。另有2个Linux交叉构建及本地普通schema84→85升级；不声明真机、真实云／模型／TG或生产验收。

Current-source checks: 229 backend top-level tests / 1383 records, 195 Web, 289 shared mobile and 55 runtime tests. All eight client commands reran and passed; both miniapps retain 20 pages. A development APK passes 18 + 4 static checks; its 26 JVM records came from Gradle cache. Web and Taro H5 each pass 5 actual browser/API/leased-worker check groups. New media: 7 PNGs, 2 silent MP4s and 2 GIFs; all 222 historical media retain exact bytes. Two Linux cross-builds and a local schema84-to85 upgrade also pass. No device, live cloud/model/TG or production acceptance is claimed.

| 编号 | 核验 | 结果和边界 |
|---|---|---|
| NTR-110-01 | 全量原生后端 | 229个顶层／1383条结果，全部发现项执行、无skip。[结果](evidence/notification-retry-backend/result.json)／[原始日志摘要](evidence/notification-retry-backend/raw-log-archive.json)。按单个批次进程顺序执行；未修改PG耐久设置、断言或清理期限。 |
| NTR-110-02 | 人工恢复业务 | 新增1个顶层、14个子场景：独立读写权、正文404、MFA、严格字段、准确revision、幂等及并发唯一子任务、原失败不变、叶节点异常列表、真实Worker／去重、游标绑定、四渠道读取、拒绝Agent／TG执行会话、DB固定输入／证据、撤权接收人、过时事件、自动退避不绕过、审计失败回滚和不可变历史。 |
| NTR-110-03 | 客户端 | [八项最终命令](evidence/notification-retry/client-checks.json)全部重新执行；Web195、移动289、运行时55。新增Web10／移动8项契约和来源检查；不以静态断言代替真实业务验证。 |
| NTR-110-04 | 微信／抖音 | 两端各20页完成构建；共享页接入重试和结果。Taro H5与四渠道fixture读取不等于真实AppID登录或正式发布。 |
| NTR-110-05 | Android | 原生Metro＋Gradle开发APK；18项包检查＋4项功能内容检查。26条JVM为缓存，保留实际任务状态与4个XML。无真机／模拟器。 |
| NTR-110-06 | 真实页面操作 | [Web](evidence/notification-retry-web/result.json)与[Taro H5](evidence/notification-retry-mobile/result.json)各5组；独立人类登录／MFA、明确提交、原失败不变、实际租约Worker站内写入、等待中详情刷新、历史追溯和页面宽度。无响应替换。自建浏览器、服务及隔离DB均关闭清理。 |
| NTR-110-07 | 媒体来源 | 7 PNG、2段静音MP4、2个GIF，来自认证后的合成页面像素；已[目视核对](evidence/notification-retry/visual-review.json)7张最终截图及两个GIF的首／中／末帧。GIF变速以[编码记录](evidence/notification-retry/media.json)实际倍率为准；未包含密码或MFA输入。原222份媒体按[原日期来源](evidence/notification-retry/reused-evidence.json)字节保留。 |
| NTR-110-08 | 普通持久升级 | 本地schema84→85，历史费用表行数及内容摘要不变；新增重试表、独立角色和3项触发约束。只替换核对身份的自有API及2个Worker；临时测试库0，外部接入仍关闭。不是生产部署。 |
| NTR-110-09 | 契约与源码 | 原320路径／698模型逐项语义保留；现323路径／398操作／705模型。32组最终来源核对；2个Linux amd64 ELF交叉构建，未执行Linux隔离边界。 |

投递技术回执不展示业务正文、自由原因、payload或会话秘密；组织运维权不授予项目内容权。创建成功只表示新任务入队。只有实际Worker结果表示站内写入或略过；不表示员工已读或TG外发。原失败从叶节点列表消失也不表示投递成功。

早期尝试均保留在[attempts](evidence/notification-retry/attempts/)：首次原生专项发现全新Bootstrap缺少新角色，已补全而未放宽权限；第二次测试将现有400 INVALID_CURSOR错误期望为422，已按既有协议修正但仍要求拒绝。首次客户端类型检查发现composite工程缺少显式共享文件入口，已修正配置。最终失败叶节点列表与等待中刷新分别增加后，旧专项／客户端通过记录保留各自原源码与日志；本轮最终全量后端及全部八项客户端命令以新源码重新执行，不把旧通过记录当作最终来源。

首次H5浏览器准备在固定存活的业务fixture内执行构建，耗尽该存活时间，fixture退出后浏览器因描述文件不存在而未启动。14个业务子场景虽通过，整轮仍记为失败。保留原生／构建／浏览器日志及准确源码，未延长Go fixture期限或清理期限。修正为先构建H5，再启动隔离业务，通过仅限本机的临时HTTP转发连接实际API；不替换响应。Web和H5均按最终辅助脚本重新核验采集，前一次Web成功只作为原日期历史记录保留。

随后Web五组实际页面操作已通过，但最终原生测试库DROP超过独立两分钟期限；同一时段全量后端的一批也出现相同清理超时。仅将失败的顶层集成测试组在原PG上串行重跑，清理仍然超时；原生PG日志记录到与失败窗口重叠的132.018秒checkpoint，其中同步128.318秒，超过原120秒清理期限。保留准确两行日志的[诊断来源](evidence/notification-retry/native-cleanup-diagnostic.json)，不将其描述为网络或容量测试。三次相关完整尝试继续保留为失败。

当前Go／SQL／契约和业务断言保持不变，仅将该失败顶层组在C盘临时目录的另一套原生Windows PostgreSQL18.6上重跑，得到11条通过结果；其fsync、synchronous_commit、full_page_writes全部开启，原fixture／清理期限均未修改。首次initdb因非ASCII程序路径失败，服务尚未启动；保留原错误及源码后沿用仓库现有ASCII路径适配器，不修改PG设置。最终后端汇总保留228个原成功顶层组及该1个重新执行的顶层组，12批原始来源通过13段日志精确绑定；同一批中原成功组不重复执行。最终Web与H5在后端结束后依次使用该原生临时PG运行，完成后关闭临时PG，库内无遗留合成测试库。

原开发PG的2个已标记无效、无活动连接的遗留合成测试库，分别按准确名称、失败时间窗口和归属核对后额外清理；未使用FORCE、未重启原PG。保留[第一次清理](evidence/notification-retry/fixture-cleanup.json)及[第二次清理](evidence/notification-retry/attempts/10-second-owned-fixture-cleanup/fixture-cleanup.json)的计划和回执。无效库不能连接，不声称读取其库内资料；原开发库及服务保留。

首次按新准备流程执行的H5在定位失败行按钮时超时，未执行任何浏览器业务组；原生14个子场景及隔离库清理均通过，整轮页面检查仍为失败。保留[准确源码、六段原始日志及前一版Web证明](evidence/notification-retry/attempts/11-h5-locator/attempt.json)，修正辅助脚本在行内重复限定页面根的定位方式，改为固定任务ID并断言唯一行。未修改应用业务代码或跳过实际点击；最终H5与Web均以修正后的辅助脚本重新采集。

首次并行的Android打包长时间停在文件发现阶段，未生成本轮业务bundle；归档准确输入和日志后只停止已核对创建时间、命令和PID的自有构建树。该次记为interrupted，不记为通过或JVM失败。最终Android记录来自后续单独执行的构建。

最终APK的18项基础检查通过后，第一次功能内容检查错误地要求包内包含仅服务端使用的`notification.delivery.retry`字符串。客户端实际以服务器核验后的`can_retry`控制重试表单；后端独立组织授权已有真实数据库场景覆盖。保留[失败断言与准确脚本](evidence/notification-retry/attempts/12-android-content-predicate/attempt.json)，修正为核对读入口、详情／历史路由、状态契约及实际`can_retry`界面分支。相同APK通过修正后的4项内容核对；应用源码及APK均未因断言修正而重建。

本轮不重新声明依赖公告查询或依赖安全专项：增量108的完整npm审计0项及56＋2专项保留原执行日期，锁文件、根manifest和workspace依赖字段未变。正式生产产物安全仍在GAP-26。

Real Codex/model/Skill execution, actual OSS/R2 and TG, formal Android signing/release, real miniapp identity/release, production deployment and complete applicable business journeys remain in the [numbered register](../delivery/03-goal-gap-assessment.md). Device verification, network performance/real120GB exercise, historical migration/disaster recovery and AI cost control remain excluded. No local pass is promoted to production readiness.
