# 增量100 · 秘书执行验证 / Secretary execution verification

日期：2026-10-28。实现说明见[开发文档](../development/100-secretary-execution-without-cost-quota.md)，当前范围见[修订99](../delivery/04-current-scope-and-acceptance.md)。本次关闭GAP-09的费用前置适配子项，完整GAP-09与其他22个工作包仍开放；5项移出不算完成。

## 1. 当前源码检查

| 编号 | 项目 | 实际范围／结果 |
| --- | --- | --- |
| V-01 | 原生后端 | 189个顶层测试、1181条结果；全部当前Go／SQL／内嵌契约源码摘要一致 |
| V-02 | 新业务回归 | 无策略／旧0额度／旧超额、真实读工具与结果、幂等、并发、取消、技术用量、旧接口停写和历史授权等12个场景通过 |
| V-03 | Web | 135项测试、类型和构建通过 |
| V-04 | 共享移动端 | 248项测试、类型通过；微信／抖音本地构建各20页 |
| V-05 | Agent运行时 | 51项测试、构建、8项Skill清单检查通过；Skill仍draft，未调用真实模型 |
| V-06 | Android | 新Metro产物与新开发APK；18项包结构检查＋3项本功能载荷检查通过；未运行真机／模拟器 |
| V-07 | JVM | 26项结果为Gradle UP-TO-DATE复用，**本次不是26项新执行**；XML原字节和原97来源保留 |
| V-08 | Linux | 当前后端linux/amd64交叉编译通过；未执行或部署 |
| V-09 | 浏览器 | Web和Taro H5各4项实际流程断言；登录、只读账本、真实筛选、390／768／1440宽度及返回秘书 |
| V-10 | 开发环境 | 原生PG结构78→79；原历史表为空，升级前后数量／摘要一致；应用角色权限、10条探针、进程身份及0个临时库通过 |

当前开发APK为42,945,032字节，SHA-256：`7b1703a3e3b6197092f7cf78b08490ba117f9b9cb34510cc19b8d43e3785267e`。实际包内JS与新Metro摘要一致，包含只读账本标题，未包含旧月度费用编辑表单。缓存范围见[Android记录](evidence/cost-free-execution/artifact-summary.json)。这不建立Android真实运行或正式签名发布结论。

## 2. 失败、复跑和清理

完整首轮10个原生批次中，`TestCommentAttachmentPreservation`与`TestDownloadPermitExpiryAndClassificationChanges`的业务断言未报告失败，但2分钟隔离库清理期限到期，因此首轮整体标记失败。保留[首轮结果](evidence/cost-free-execution-backend/attempt01/result.json)与[原批次记录](evidence/cost-free-execution-backend/attempt01/batch-progress.json)，未增加清理期限或关闭数据库持久性。所有其他当前源码通过结果保留；这两项随后单独串行复跑并通过，见[最终原生结果](evidence/cost-free-execution-backend/result.json)。

最初环境检查仍发现2个残留测试库。只处理名称符合严格测试前缀、无连接、PostgreSQL invalid标志为真，且在本仓库PG日志中有准确失败DROP语句的两库；未触及日常业务库、其他数据库或重启PG。清理后重新检查0个临时库，见[受限清理](evidence/cost-free-execution/fixture-cleanup.json)。初次开发检查JSON被后续检查替换，保留的是明确标注的[输出结果说明](evidence/cost-free-execution/attempts/development-outcome-note.json)，不把它称为原JSON。

定向检查最初遇到原生PG未启动、工具参数写错、单公司测试准备与404隐藏资源语义不符；修正后12场景通过。失败日志哈希与原因保留在[定向尝试](evidence/cost-free-execution/attempts/focused-attempts.json)，完整结果以当前源码的全量后端记录为准。

首次浏览器检查因错误调用`page.innerText()`失败，修正后功能检查通过；视觉复核又发现列表刷新时就截图。两次原记录和素材保留在[浏览器尝试](evidence/cost-free-execution/attempts/)，最终采集新增准确记录可见的等待条件，随后两端重新检查、截图和录屏。原始帧留在忽略目录，公开逐帧摘要与编码记录，不修改原失败证明。

首次汇总自检还发现客户端Web构建与临时Web审阅服务的日志路径同名，原Web构建日志已被覆盖、不可恢复。原检查记录及失败汇总保留在[日志冲突记录](evidence/cost-free-execution/attempts/client-log-collision/note.json)；客户端改用独立日志命名空间后，八条命令全部重新执行，当前摘要匹配新的原始日志。应用源码未因该修正改变；未把原日志缺失记录计作当前成功证明。

原生执行用例使用隔离合成资料及测试控制器／许可，验证实际服务、事务、工具和结果落库流程；没有真实模型调用或OS隔离环境认证。历史数据保留在合成用例中验证；日常开发库旧费用表本来为空，空表升级摘要一致不等同于存量业务数据迁移验收。

## 3. 媒体与原证据

| 客户端 | 截图 | 原速录屏 | GIF |
| --- | --- | --- | --- |
| Web | [PNG](../media/screenshots/secretary-cost-free-web.png) | [MP4](../media/videos/secretary-cost-free-web.mp4) | [GIF](../media/gifs/secretary-cost-free-web.gif) |
| 共享Taro H5 | [PNG](../media/screenshots/secretary-cost-free-mobile.png) | [MP4](../media/videos/secretary-cost-free-mobile.mp4) | [GIF](../media/gifs/secretary-cost-free-mobile.gif) |

新增2张PNG、2段MP4、2段GIF，合成数据、真实浏览器像素、无密码／MFA画面，全部自建浏览器上下文、页面和5189临时服务均关闭。原97 PNG／35 MP4／35 GIF保留Git对象和文件哈希；当前规范媒体总数99 PNG／37 MP4／37 GIF。旧审计按原来源复用，仍80项（13high／60moderate／7low），没有刷新审计源或宣称安全门关闭，见[复用清单](evidence/cost-free-execution/reused-evidence.json)。

## 4. 可复核命令与边界

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_cost_free_execution_backend.py
# 仅完整首轮失败时运行；严格保留原证明，仅复跑失败顶层用例：
python app/scripts/retry_cost_free_execution_backend.py
python app/scripts/verify_cost_free_execution_clients.py
python app/scripts/verify_cost_free_execution_development.py
python app/scripts/verify_cost_free_execution_sources.py
python app/scripts/verify_current_scope.py
python app/scripts/verify_repository_publication.py
```

已有证据目录时先归档已有尝试；不得覆盖原结果来伪装重跑。Android与浏览器命令、来源摘要和原日志哈希见各[证据记录](evidence/cost-free-execution/)。原生开发API18080、PG14660及日常通知／报表工作进程保留，外部存储、模型、TG和小程序平台凭据未启用。

真机、网络性能／实网120GB、历史资料迁移／容灾及AI费用控制已移出当前范围。真实Codex、执行隔离、Skill发布／评价、TG、OSS＋R2、平台接入、正式Android发布、持久PG和完整业务验收仍需完成。未使用Docker、WSL、虚拟机或模拟器。

English: All189 current-source backend cases pass with1181 result records; the first cleanup failures and the two serial retries remain visible. Web135/mobile248/runtime51 checks pass. Android has a fresh bundle/APK,18 structural and3 feature-content checks;26 JVM results are cached, with no device-runtime claim. Both actual Web/H5 archive flows and new captures pass with owned resources closed. The complete goal and production acceptance remain open.
