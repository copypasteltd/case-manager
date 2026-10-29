# 审计查询与固定导出验证 / Audit console verification

增量109通过当前源码检查；GAP-07仅完成审计子功能，完整23项工作包仍开放。AI仅排除费用控制。应用实现和命令见[开发说明](../development/109-audit-console.md)。

当前源码核验：后端228个顶层测试／1368条结果，Web185、共享移动281、运行时55，微信／抖音各20页，Android开发APK及18＋4项静态检查、2个Linux交叉构建。Web与Taro H5各5组实际浏览器／API操作通过；新增7 PNG／2静音MP4／2 GIF，原211份媒体字节保留。26条JVM结果来自Gradle缓存。Web布局修正后3项相关命令重跑，5项未受影响命令保留本轮原始执行；不声明真机、真实云／模型或生产验收。

Current checks: 228 backend top-level tests / 1368 records, 185 Web, 281 shared mobile and 55 runtime tests; 20 pages per miniapp, a development APK with 18 + 4 static checks and 2 Linux cross-builds. Both real Web/Taro H5 journeys pass 5 check groups. New media: 7 PNGs, 2 silent MP4s and 2 GIFs; all 211 historical media retain exact bytes. The 26 JVM records came from Gradle cache. After the Web layout correction, 3 affected commands reran; 5 unaffected commands retain their original same-increment execution. No device, live cloud/model or production acceptance is claimed.

| 编号 | 核验 | 实际结果与边界 |
|---|---|---|
| AUD-109-01 | 原生后端全量 | 228个顶层／1368条结果，全部发现项执行，无skip；[最终记录](evidence/audit-console-backend/result.json)与[原始日志摘要](evidence/audit-console-backend/raw-log-archive.json)。 |
| AUD-109-02 | 审计功能 | 3个原生单元测试＋1个HTTP顶层／9个子场景：准确边界、并列时间分页、游标换条件／人员、四个会话渠道、独立导出权、MFA、幂等／固定引用、摘要、10,001条整单拒绝、空CSV、DB不可变约束、审计失败回滚、到期和撤销。 |
| AUD-109-03 | 客户端 | [命令记录](evidence/audit-console/client-checks.json)：Web185／移动281／运行时55；原8项命令通过后，只修正2个Web文件，类型／Web测试／Web构建3项重新执行，未受影响的5项保留原始记录与准确输入。 |
| AUD-109-04 | 微信／抖音 | 各20页构建，保留真实AppID／域名／身份发布验收；抖音不声明原生CSV文件出口。 |
| AUD-109-05 | Android | 原生Metro＋Gradle开发包，18项APK及4项功能内容静态检查；26条JVM为缓存，XML与任务状态保留。无真机／模拟器。 |
| AUD-109-06 | 实际浏览器 | [Web](evidence/audit-console-web/result.json)和[Taro H5](evidence/audit-console-mobile/result.json)各5组真实API操作；准确筛选、空结果、分页、详情、固定导出、本人历史与文件大小／SHA核对。所有自建上下文、服务和数据库清理完成。 |
| AUD-109-07 | 媒体 | 7 PNG、2段原速静音MP4、2个GIF来自认证后的真实浏览器像素；原211份媒体字节未变，见[媒体记录](evidence/audit-console/media.json)和[保留来源](evidence/audit-console/reused-evidence.json)。 |
| AUD-109-08 | 开发持久环境 | 普通schema83→84升级，历史费用记录摘要保留，API及2个Worker仅替换已核对身份的自有进程；临时数据库0，云／TG／模型接入未启用，不是生产部署。 |
| AUD-109-09 | 契约与源码 | 原314路径／687模型语义逐项保持；新增后合计320路径／395操作／698模型；20组最终来源核对通过。2个Linux amd64 ELF交叉编译，未在Linux执行。 |

HTTP四渠道检查使用隔离fixture的Web Cookie＋CSRF及微信／抖音／Android渠道会话，不是实际平台登录。组织边界通过现有单组织约束、未知组织外键和未知事件拒绝核对；不声称部署了两个真实租户。系统管理员可读明确授权的组织元数据，实际项目正文请求仍返回404。

失败与修正均保留：早期OpenAPI完整生成遗漏已有摊销扩展，源码守卫中止该不完整尝试；已把扩展加入统一生成流程，并逐项核对既有契约。新增回执测试的字面量类型错误在类型检查时被拒绝；修正后完整客户端命令通过。首轮浏览器准备请求遗漏Origin被403拒绝，修正脚本而未放宽CSRF。首次Web功能已通过，随后修正按钮对齐和导航标题并重新检查、重新采集最终媒体。

全量后端首次完整执行中，旧项目集成用例在DROP临时数据库时超过独立两分钟清理期限；初始失败及全部原始日志保留。仅清理超时的批次按同一后端摘要串行复跑，通过的其他批次保留原始执行，未放宽业务断言或更改数据库耐久设置。详见[清理复跑来源](evidence/audit-console-backend/result.json)。首次清理超时留下3个PostgreSQL已标记无效的测试库；按准确名称、创建时间、失败日志窗口、无活动连接和无效删除标志归属后，未使用FORCE完成清理，未声称读取不可连接库内的组织或迁移内容。最终零残留检查独立通过，见[清理回执](evidence/audit-console/fixture-cleanup.json)。

本轮没有重新查询依赖公告或重新执行增量108的56＋2依赖安全专项；锁文件、根manifest及workspace依赖字段保持一致，完整npm审计0项等结果保留增量108的原日期和范围。正式生产依赖与产物验收继续属于GAP-26。

No real-device validation, network performance/real120GB exercise, historical migration/disaster recovery or AI cost control is added as a gate. Live Codex/model/Skills, actual cloud/TG/miniapp identities, formal Android release and complete applicable business journeys remain in the [numbered work register](../delivery/03-goal-gap-assessment.md). Passing local checks does not establish production readiness.
