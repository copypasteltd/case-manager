# 增量101 · OA 正文起草验证 / OA prose verification

日期：2026-10-28。实现及四端操作见[开发文档](../development/101-oa-prose-drafting.md)，当前范围见[修订99](../delivery/04-current-scope-and-acceptance.md)。本轮完成GAP-12的限定正文字段起草／人工采用子项，完整GAP-12及其他22个工作包继续开放；5项移出不计为完成。

## 1. 当前源码与真实本地检查

| 编号 | 项目 | 结果与范围 |
| --- | --- | --- |
| V-01 | 原生后端 | 190个顶层测试、1190条最终结果；实际运行于原生PostgreSQL，全部当前Go／SQL／内嵌契约来源一致 |
| V-02 | 正文业务回归 | 合同／采购／报销各一条真实起草→核对→采用；另含业务过期、取消、采用回执失败回滚、当前权限变化、额外已读来源变化，共8项新子场景；MCP工具边界另有1项 |
| V-03 | Web | 142项测试、类型和构建通过 |
| V-04 | 共享移动端 | 255项测试、类型通过；微信／抖音本地构建各20页 |
| V-05 | Agent运行时 | 52项测试、构建及8项Skill清单检查通过；OA包1.3.0、所有Skill仍draft |
| V-06 | Android | 新Metro包及新开发APK；18项包结构＋4项本功能载荷检查通过；未运行真机或模拟器 |
| V-07 | JVM | Gradle缓存／UP-TO-DATE复用，本次没有新执行26项JVM测试；保存原XML字节、实际Gradle任务及原97执行来源 |
| V-08 | Linux | 当前后端linux/amd64交叉编译通过；未执行或部署 |
| V-09 | 浏览器 | Web与Taro H5各3项实际流程检查；390／768／1440宽度、实际人工核对界面、合同与采购真实采用及报销真实意图发送 |
| V-10 | 开发环境 | 结构80、正常业务权限、10条探针、API及两工作进程来源一致；0个临时测试库，外部集成未启用 |
| V-11 | API | OpenAPI含299条路径、370个操作、651项schema；生成契约与内嵌契约一致 |

开发APK为43,790,523字节，SHA-256为`f305ba7d587efb3f0175ca157dbfc910f19fcde3b1249c2fdf79b4143b205aa8`。包内JS与当前Metro产物完全一致，包含实际正文建议、人工核对、采用端点及字段／来源协议；见[构建与范围](evidence/oa-prose/artifact-summary.json)。开发签名及载荷检查不建立正式Android发布或设备运行结论。

## 2. 事务、权限与来源

受控执行夹具调用实际MCP边界、Go服务和隔离PostgreSQL事务，检查原文与建议、只修改授权正文字段、所有商业事实保留、真实业务操作号、版本递增、同键重放、新键重复拒绝及不可变建议／采用回执。没有创建或提交审批申请，也没有使用真实Codex模型。后端还检查采用回执落库失败时业务和幂等记录一起回滚，来源资料变化时整个建议／任务撤回可见性。

Web和Taro H5通过实际登录／MFA、Cookie／Bearer与原生API完成合同、采购人工采用，并从真正的报销选择器发送一次队列请求。浏览器中模型建议由明确的合成控制夹具预先产生；报销排队成功不表示模型推理成功。所有自建浏览器上下文／页面、5189审阅服务及含凭据的夹具已关闭或删除；日常原生API18080、PG14660及通知／报表工作进程保留。

## 3. 尝试记录与可追溯范围

第一轮后端检查启动后，浏览器夹具补充了合成Alice的MFA准备步骤。运行器检测到测试来源变化，完成已运行批次后拒绝继续，未把这轮534条通过记录计作当前成功证明。原进度和说明保留在[来源变更尝试](evidence/oa-prose-backend/attempts/fixture-source-change/)。随后从固定源码重新执行全部测试；当前首轮的`TestHistoryRestoration`、`TestPlatformIntegration`中三处隔离测试库DROP清理超时；第五批还触及20分钟Go包总时限，6个顶层用例未产生完成结果。保留首轮失败和原日志摘要后，串行复跑两项失败用例，并补执行`TestInAppNotifications`、`TestMiniappMFAAndRevocation`、`TestMiniappMFAEnrollmentAndVerify`、`TestMiniappProofBoundaries`、`TestMiniappRefreshRevocationRace`、`TestMobileContentBrowserFixture`，最终全部通过；未延长测试时限或关闭数据库持久性。见[最终原生结果](evidence/oa-prose-backend/result.json)。

三次原DROP超时留下invalid测试库；仅清理准确名称、postgres归属、无活动连接且在本仓库PG日志中有失败DROP记录的三库，逐一绑定失败用例与时间。处置见[受限清理记录](evidence/oa-prose/fixture-cleanup.json)。首次环境检查随后发现另1个有效测试库：其目录创建时间落在首轮中断的`TestMiniappProofBoundaries`执行窗口，80条迁移摘要与标准合成公司／账号匹配，项目、文件、合同、采购报销、申请和Agent运行均为空，且无活动连接。按准确名称正常删除，没有强制断开连接；见[中断夹具清理](evidence/oa-prose/interrupted-fixture-cleanup.json)。原环境失败检查保留在[环境尝试](evidence/oa-prose/attempts/development-before-interrupted-fixture-cleanup/)。最终只读环境检查确认0个测试库；未重启PG或处理业务／无关数据库。

浏览器首次准备因合成员工尚未满足MFA门槛而停止，尚未打开浏览器。补充MFA夹具后两端真实流程重新通过；原说明在[准备尝试](evidence/oa-prose/attempts/browser-fixture-preparation/outcome.json)。定向开发时修正SQL CASE括号、JSON对象键序比较和取消请求结构等问题，最终完整测试覆盖修正后源码；早期诊断不计为成功证明。

首次日常开发更新已应用结构79→80，随后在旧API尚未释放端口时安全停止。第二次实际更新记录检查的是既有结构80，再启动准确归属的三个进程。首次原日志未保留，只有明确标注的[观察结果说明](evidence/oa-prose/attempts/development-restart-note.json)；不将第二次80→80记录伪称原79→80证明。旧费用表原本为空，数量／摘要一致不代表历史业务资料迁移验收。

## 4. 截图、录屏与原证据

| 客户端 | 采用前 | 采用后 | 原速静音录屏 | GIF |
| --- | --- | --- | --- | --- |
| Web | [PNG](../media/screenshots/secretary-oa-prose-web-before.png) | [PNG](../media/screenshots/secretary-oa-prose-web-after.png) | [MP4](../media/videos/secretary-oa-prose-web.mp4) | [GIF](../media/gifs/secretary-oa-prose-web.gif) |
| Taro H5 | [PNG](../media/screenshots/secretary-oa-prose-mobile-before.png) | [PNG](../media/screenshots/secretary-oa-prose-mobile-after.png) | [MP4](../media/videos/secretary-oa-prose-mobile.mp4) | [GIF](../media/gifs/secretary-oa-prose-mobile.gif) |

本轮新增4张PNG、2段MP4及2段GIF，均为合成资料与真实浏览器像素。录屏逐帧摘要、编码产物摘要与时长在[媒体记录](evidence/oa-prose/media.json)，截图视觉检查和准确字节在[复核记录](evidence/oa-prose/visual-review.json)。截图为滚动后的可见视窗，不宣称一个视窗覆盖整页；未录入密码或MFA秘密。

原99 PNG／37 MP4／37 GIF的173个Git对象保留；当前总数103 PNG／39 MP4／39 GIF。原增量98依赖审计按未变锁文件复用，仍80项（13high／60moderate／7low），未刷新公告源或关闭生产安全门。缓存JVM、历史媒体及审计与新执行证据的边界见[复用清单](evidence/oa-prose/reused-evidence.json)。

## 5. 复核命令及交付边界

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_oa_prose_backend.py
# 仅首轮失败时串行复跑失败／未完成顶层用例，保留原证明：
python app/scripts/retry_oa_prose_backend.py
python app/scripts/verify_oa_prose_clients.py
python app/scripts/verify_oa_prose_development.py
python app/scripts/verify_oa_prose_sources.py
python app/scripts/verify_current_scope.py
python app/scripts/verify_repository_publication.py
```

已有同名结果先按attempt归档，不覆盖原失败来模拟成功。Android、Linux、浏览器与媒体的命令／脚本摘要在各[证据记录](evidence/oa-prose/)。最终来源检查只核对已有实际证明，不会替代重新执行测试或模型推理。

真机、网络性能／实网120GB、历史2 TB迁移／容灾及AI费用控制已移出范围。四端、真实Codex、执行隔离、Skill评估／发布、TG、OSS＋R2、平台接入、正式Android发布、持久PG、全业务功能验收和80项依赖风险仍在范围内。未使用Docker、WSL、虚拟机或模拟器。

English: All190 current-source backend tests pass with1190 result records. Eight new prose journeys cover all three document kinds and adverse authority/source/transaction cases. Web142/mobile255/runtime52 checks pass. Android has a fresh bundle/APK,18 structural and4 feature-content checks; the exact JVM execution/cache scope is recorded separately. Actual authenticated Web/H5 adoption and claim-intent flows pass with owned resources closed. The complete goal and production acceptance remain open.
