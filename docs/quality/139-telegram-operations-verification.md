# 增量139 · 关键验证、原出处与延期

按用户加速冻结要求，只执行与本次组织TG通知运维相关的关键检查。新代码、真实集成和正式发布验收分别记录，不将候选包、合成群或单元检查记为生产通过。

| 范围 | 本轮执行 | 证据 |
|---|---|---|
| 原生HTTP／耐久PG | 新运维场景＋原有TG发送套件；四渠道真实人工登录，供应商结果／群身份合成 | evidence/telegram-operations/native-final.json |
| 类型与本地构建 | Web、共享移动／运行器类型检查；Web、微信、抖音构建 | 同目录typecheck／web-build／weapp-build／douyin-build的最新记录 |
| 实际Web | 组织列表→历史→原群人工确认→刷新及独立HTTP回读；Go另核对一条持久审计与四条事件 | evidence/telegram-operations-web/result.json、business-proof.json |
| 媒体 | 三张实际截图、实际认证后MP4、明确标注的三截图GIF | evidence/telegram-operations/media.json、visual-review.json |
| 当前来源 | 精确源码、原始失败尝试、历史资料与冻结标签分别核对 | evidence/telegram-operations/final-source-check.json |

普通native运行中的browser_workbench是显式浏览器保留入口；未开启浏览器时直接返回，不能把该pass记录算作额外业务场景。实际浏览器运行单独开启该入口，并在浏览器关闭后从数据库核对结果。

首次测试账号未将秘书角色设为要求MFA，导致会话准备被拒绝；后续补正确的隔离设置。原接口的无效输入是422、无MFA写入403、撤权隐藏404，若干新增断言最初误写400／404／412，修正断言而没有降低应用授权或修改被测响应。失败JSON与私有日志哈希全部保留。最终源绑定原生执行重新覆盖修正后的场景。

浏览器成功执行后仅修正一个测试断言412→404；业务Go／SQL／契约／客户端字节未变化。browser-fixture-adjustment.json保留前后哈希与准确替换，校验器逐字重建当时捕获来源，不把旧来源伪写成新来源。当前验证器在捕获后新增，属于独立只读检查，未参与原浏览器运行；记录该来源差异。类型／构建以各次原始源摘要为准，早期Web验证前后仅增加显式重新验证开关，最新记录覆盖最终客户端。

发布检查首次因清单提前引用尚未生成的自身报告而中止；移除循环输入后重新执行，未减少凭据、文件、来源或链接检查。首次范围检查发现旧可见余项表未同步GAP-07／18新描述，已按当前JSON更新并重跑；原失败记录保留。

延期：完整后端／运行器回归、全量Web／移动测试、新移动浏览器旅程、Android JS／JVM／APK、开发库100→105的实际升级、真实Bot／模型／Skills／Linux／云／平台／生产验收。结构105已在一次性原生PG全新安装；不声称执行既有生产升级。历史完整后端／运行器121、完整客户端136（Web452／共享移动537）、APK137业务136／结构103按原出处保留；不是本轮重新执行。所有旧证据／媒体与迁移1–104不改。

五项范围排除和八项draft Skill保持；AC／TC适用性核对只验证分类，不能增加业务通过数。未使用Docker、WSL、虚拟机或模拟器，自建浏览器和临时服务关闭；保留用户开发服务。

The focused proof uses real native durable PostgreSQL, authenticated HTTP and actual Web interactions with synthetic group identities/provider outcomes. It does not contact Telegram or a model. Prior full tests and APKs retain their original sources. Failed attempts and the single fixture-assertion adjustment are disclosed rather than overwritten. Full-goal and production acceptance remain false.
