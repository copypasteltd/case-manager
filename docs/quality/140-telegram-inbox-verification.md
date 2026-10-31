# 增量140 · 组织TG入站调度验证

本轮按用户“主功能闭环、适当跳过部分验证、加速冻结”的指示执行关键检查。未把旧证据重新记作当前测试，未运行Docker、WSL、虚拟机、模拟器、真机或网络性能专项。

| 分类 | 当前执行与实际证据 | 未扩大声明 |
|---|---|---|
| 后端／数据库 | 两组实际HTTP与原生耐久PG；14个新入站业务场景、13个既有TG调度场景 | 30条通过记录包含两组父记录及正常运行中未启用的浏览器hold；实际业务27项，非30项 |
| 身份与事务 | Web／微信／抖音／Android真实交互会话；独立权限、私密正文边界、过滤游标、历史完整分页、MFA、版本、幂等、审计回滚、不可变约束和调度／撤回竞争 | Bot、群、运行配置及提供方结果为合成；既有调度检查使用受控执行器，非真实模型 |
| 旧请求 | 合成旧schema空任务类型，组织详情可读、范围不成立、不得恢复任务授权 | 隔离设置只模拟旧请求形态，不是实际旧库升级／历史迁移 |
| 客户端 | 当前完整可移植源码的类型、Web、微信和抖音本地构建 | 未重新执行全量Web／共享移动单测、完整移动UI、原生JVM／Android打包 |
| 实际Web | 三项实际认证交互；查询与详情、明确撤回、刷新及独立HTTP读回；Go释放夹具后另查一条回执／审计、两条历史及零任务链接 | 无真实群、Bot API、模型或公司资料 |
| 媒体 | 三张实际页面PNG、认证后真实视口MP4、三截图各两秒GIF；逐项摘要及人工抽帧检查 | GIF为截图导览；视频未声称逐帧完整人工审片；无新移动录制 |
| 资源 | 检查后关闭本轮全部所属浏览器、上下文、页面、临时Web服务、夹具及原生PG | 保留原个人开发进程／PG，不关闭用户其他页面与进程 |

## 实际执行出处

- [原生最终记录](evidence/telegram-inbox/native-final.json)：`TestTelegramInboxOperations`和`TestTelegramDispatch`实际执行；没有Go缓存复用或跳过的业务项。常规运行内`browser_workbench`未启用，实际浏览器另单独启用。
- [当前类型](evidence/telegram-inbox/typecheck-2.json)、[Web构建](evidence/telegram-inbox/web-build-2.json)、[微信构建](evidence/telegram-inbox/weapp-build-2.json)、[抖音构建](evidence/telegram-inbox/douyin-build-2.json)：Node22，本地API构建档。
- [实际Web](evidence/telegram-inbox-web/result.json)、[持久业务读回](evidence/telegram-inbox-web/business-proof.json)、[媒体](evidence/telegram-inbox/media.json)、[人工视觉检查](evidence/telegram-inbox/visual-review.json)。
- [最终完整源码核对](evidence/telegram-inbox/final-source-check.json)、[当前范围](evidence/telegram-inbox/scope-check.json)、[原AC／TC适用性](evidence/telegram-inbox/acceptance-applicability-check.json)、[发布文件检查](evidence/telegram-inbox/repository-check.json)。这些完整性核对不会重跑延期的功能测试。

`verify_telegram_inbox.py`以只读方式重算源码、构建／执行出处、不可变旧证据、媒体、清理和冻结引用。后端空任务类型兼容没有改变客户端；后续视觉检查修正Web原日期格式，明确显示香港时分秒，避免同日排队期限无法区分。四项客户端检查随后全部重新执行，第二组绑定最终当前源码；最终浏览器也在最终完整源码执行。原生检查绑定的全部后端与其测试保持未变，不因Web显示调整重跑既有后端场景。

## 保留尝试与修正

首次原生和Web操作均通过，随后源码审查发现早期请求可能有NULL任务类型，普通SQL布尔式可能返回NULL。增加显式false兼容及旧请求场景；首次成功记录、当时完整两份变更源码、浏览器出处与三张PNG保留在[兼容前记录](evidence/telegram-inbox/attempts/before-legacy-compatibility/native-final.json)及同目录，不能当作当前后端执行。

新增旧形态场景的第一次尝试在隔离数据准备阶段遇到PostgreSQL待执行约束触发器，无法直接恢复guard。该事务完整回滚；改为先执行隔离事务内待处理约束，再恢复guard。保留[失败记录](evidence/telegram-inbox/attempts/legacy-fixture-trigger-flush/native-final.json)与原始测试源码／私有日志摘要。没有放宽生产guard、断言或权限；最终原生重新执行两组完整选定场景。

Web时分秒显示调整前的第二次浏览器、三张PNG、MP4、GIF及原显示源码保留在[原媒体记录](evidence/telegram-inbox/attempts/before-web-time-detail/media.json)与同目录。最终媒体重新录制和编码；编码器首次拒绝覆盖旧捕获清单，已先归档该准确清单再编码，不将旧清单换标签冒充新录制。

首次发布文件检查只有“本文指向本次发布报告的链接”未通过：检查开始时报告尚未生成。该次报告已[原样保留](evidence/telegram-inbox/publication-initial-link-snapshot.json)，生成文件随后进入待发布清单再重新检查。凭据、历史证据及功能检查均未因此放宽。

## 复用、延期与发布范围

保留121的完整后端／运行器回归、136的Web452／共享移动537项完整回归、137的开发APK与业务136／结构103出处，本轮不重新签收。个人开发库仍100；本轮新空库106安装通过，现有开发库100→106及生产PG验收未执行。平台身份、真实模型、八项Skill、Linux隔离、实际TG、OSS／R2、外部告警和生产发布继续按各GAP推进。

两冻结候选`v0.1.0-rc.1`／`rc.2`与下载分支不变，APK137不含138–140。真机、实网120GB、网络性能、历史迁移／容灾、AI费用控制仍为移出项，不计测试通过。当前仍23 active／5 excluded，30项AC／72项TC分类不等于业务验收完成，八项Skill保持draft。

[功能设计与使用](../development/140-telegram-inbox-operations.md) · [本轮及全部23项](../delivery/05-functional-closure-140.md)
