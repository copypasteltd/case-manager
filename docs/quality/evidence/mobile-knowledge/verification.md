# 增量83：移动知识与项目博客 / Increment83: mobile knowledge and project blog

本轮验证的主体是完整手工知识工作流、评论原附件保留及按固定版本ID查阅接口。所有运行使用原生 Windows 工具；未启动 Docker、WSL、虚拟机、模拟器、真实云、真实模型或外部 TG 投递。精确结果与摘要由下列机器记录约束，不将旧录屏或单元测试作为真机验收。

This increment verifies complete manual knowledge workflows, preservation of original comment attachments and exact file-version-ID resolution. Native Windows tooling only is used. No virtualization, emulator, real cloud/model execution or external Telegram delivery is claimed. Machine records bind the exact source, artifacts and execution scope.

最终结果：25项复核通过；完整后端155顶层／986结果／894末级。新开发APK为42,881,123字节，SHA-256 `8108a630fd9bb90ba5fe2ceaa8e432beaba1967fa391f29f700193a5b8b08fbf`。原生任务标记为`UP-TO-DATE`，25项结果本次使用缓存。All25 final checks passed; native JVM reuse is explicitly cached.

## 1. 当前来源与证据 / Source-bound evidence

| 范围 / Scope | 记录 / Record | 说明 / Interpretation |
| --- | --- | --- |
| 完整后端 / Complete backend | [result.json](result.json)、[batch-progress.json](batch-progress.json) | 155顶层测试，当前LF来源八批完整新执行；首轮失败及通过记录另行保留 /155 discovered top-level tests freshly executed in eight batches at current LF source; earlier failed/passed records retained separately |
| 新后端回归 / Added regressions | [评论保留测试](../../../../app/backend/internal/httpapi/comment_attachment_integration_test.go)、[固定版本测试](../../../../app/backend/internal/httpapi/file_version_integration_test.go) | 隐藏附件不丢失、回执／重放不过滤失守、明确替换前核权、旧历史不变；固定版本不替换且持续核权 / Preservation, filtered replay, authorized explicit replacement, immutable history and exact authorized pins |
| Web | [web-build.json](web-build.json) | 新执行81项测试及正式编译；文字评论省略附件替换 /81 fresh tests and build; text-only edits omit replacement |
| Runtime | [runtime-build.json](runtime-build.json) | 新执行39项受控测试及类型／构建；真实模型未执行 /39 fresh controlled tests, types/build; no real model execution |
| Taro | [mobile-build.json](mobile-build.json)、[原始日志](mobile-verification.log) | 新178项、类型、微信／抖音各16页编译；152份来源 /178 fresh tests, types and two16-page mini-program compilations at152 sources |
| Linux | [linux-build.json](linux-build.json) | 按本轮完整Go／SQL／契约摘要重新编译platform；不执行或部署 / Fresh platform cross-compilation; never executed or deployed |
| 实际页面 / Actual UI | [结果](../mobile-knowledge-browser/result.json)、[805项来源](../mobile-knowledge-browser/source-manifest.json) | 12组实际流程、9项320／390／768宽度布局；两个上下文完整关闭 /12 flows, nine layouts and two closed contexts |
| Android | [build.json](build.json)、[17项包检查](apk-inspection.json) | 新开发APK、完整来源及产物摘要；没有真机运行或正式签名放行 / Fresh debug APK, full source/artifact digests and17 inspections; no device or release-signing acceptance |
| 原生测试范围 / Native test scope | [android-test-scope.json](android-test-scope.json)、[Gradle日志](native-build.log) | 四份XML共25项；按实际Gradle标记区分新执行与缓存，不能将缓存称为重新运行 /25 results in four XML files; the actual task marker distinguishes execution from cache reuse |
| 原始Java边界 / Pure Java foundation | [原25项来源证明](../android-upload/jvm-source-proof.json) | 原来源未变，准确复用；本轮不冒称重新执行该独立portable运行 / Exact unchanged-source reuse; not a new independent portable run |
| 日常服务 / Daily services | [development-runtime.json](development-runtime.json)、[development-project-resources.json](development-project-resources.json) | 原生日常服务、69项迁移及集成关闭状态的只读核对；不将日常旧二进制当成本轮新接口部署 / Read-only health/schema/disabled-integration inspection; no new deployment claim for existing daily binaries |
| 来源变更沿革 / Source lineage | [换行与检查器证明](source-normalization.json)、[首轮完整证据](attempts/before-lf-restoration/scope.json)、[归档等待修正](attempts/browser-04/scope.json) | 四份Go文件恢复原LF；检查器等待实际状态呈现，原业务代码不变；准确重构旧805项来源与完整后端摘要，并保留旧5份媒体 / Four original Go LF conventions restored; checker waits for rendered state; exact prior805-source/backend reconstruction and all five old media retained |
| 最终复算 / Final verification | [final-source-check.json](final-source-check.json) | 重新计算当前源码、完整产物、帧与资源清理；以passed字段为准 / Recompute current sources, artifacts, frames and cleanup; the passed field is authoritative |

## 2. 实际流程与边界 / Actual flows and boundaries

1. 项目会议纪要：类型、范围、Unicode标签、Markdown和两种附件方式，保存后读取正式资料及首个不可变修订。
2. 已发布内容编辑：产生新的当前正文，原已发布指针保留，未自动发布。
3. 第二次编辑及人工发布：准确原因、声明、完整回执与正式回读；录制期间不出现认证资料。
4. 历史恢复：首个正文复制为第四个新修订，原历史及独立已发布指针保留。
5. 固定附件评论：绑定实际正文，文字编辑不传附件替换字段，后继评论与原始评论历史均可核对。
6. 实际内容读取：固定历史原件与跟随当前版返回不同原始字节，脚本标记只作为文字呈现。
7. 真实并发编辑冲突409：保留本地文字，停止重提，明确关闭并重读。
8. 真实正文上下文冲突409：未提交评论保留，不把旧正文讨论混入新正文。
9. 受控异常回执：真实PATCH先提交成功，测试单独删回执字段；客户端标记未核对并不重复PATCH。
10. 受控附件过滤：保留真实完整摘要而过滤公开引用；客户端不开放正文覆盖／发布，完整重读后恢复。
11. 实际归档／退回草稿及其他三种公司级知识创建；归档时关闭正文和评论写入。
12. 实际中文／英文检索与22条游标分页；单独受控403清空旧列表；另一真实身份保持独立会话。

Flows1—8,11—12 use actual native HTTP/database commands or reads. Flow9 deliberately damages one successful real receipt. Flow10 deliberately filters a public representation while retaining its complete digest. The final list403 is separately labelled synthetic. These fault injections test client behavior; they are not claims that external platforms or real revocation infrastructure were exercised. Backend integration tests independently withdraw file availability and verify comment preservation, filtering, replay and explicit replacement.

## 3. 媒体与视觉核对 / Media and visual inspection

- 三张截图：[编辑](../../../media/screenshots/mobile-knowledge-editor.png)、[已发布正文](../../../media/screenshots/mobile-knowledge-published.png)、[评论与历史](../../../media/screenshots/mobile-knowledge-comments.png)。
- [MP4](../../../media/videos/mobile-knowledge-access.mp4)：390×900、2.8秒、无声；[GIF](../../../media/gifs/mobile-knowledge-access.gif)：17帧、2.84秒。
- [原始帧清单](recording-frames.json)包含13张实际JPEG的逐帧时间与摘要，原始录制2656ms；[来源证明](recording-provenance.json)、[编码记录](media.json)及[视觉抽样](visual-review.json)绑定来源和输出。
- 实际查看全部三张截图，以及GIF第0／5／11／16帧。观察到编辑资料、当前与已发布指针变化、准确版本名称及评论历史。原因与声明控件由实际UI流程核对，不在GIF抽样视口内。未做全帧OCR或真机视觉验收。

All three published screenshots and four GIF samples were inspected. Thirteen real JPEGs with recorded timing produce the silent MP4 and17-frame GIF. Capture preconditions exclude passwords, six-digit MFA fields and private proof capabilities. Synthetic IDs, hashes and script text are deliberate fixture content. Raw frames and all authentication material remain ignored locally. The [media catalog](../../../media/README.md) documents reproduction.

## 4. 保留的失败与重跑 / Retained failures and reruns

| 记录 / Attempt | 实际问题 / Observed problem | 处理 / Resolution |
| --- | --- | --- |
| [typecheck-01](attempts/typecheck-01/result.json) | API字段名和运行时完整知识类型不匹配 / Incorrect authorization field and optional generated types | 使用authorization_version及经运行时校验的完整类型 / Correct API name and validated required types |
| [typecheck-02](attempts/typecheck-02/result.json) | 项目classification类型可空 / Optional project classification | 明确默认值 / Explicit fallback |
| [browser-01](attempts/browser-01/result.json) | 新建误用post.create，实际能力为post.write / Wrong creation action | 与原后端权限一致，完整重跑 / Match original service authority and rerun |
| [browser-02](attempts/browser-02/result.json) | 自动检查的嵌套定位器作用域不正确 / Incorrect nested locator scope | 修正检查器，无业务放宽 / Correct checker scope |
| [browser-03](attempts/browser-03/result.json) | 检查器没有点击读取内容按钮便等待正文 / Checker waited before the explicit read action | 实际点击后核对原始内容 / Perform explicit content reading |
| [browser-04](attempts/browser-04/browser/result.json) | 归档接口返回后，页面回读尚未完成，负断言抢先 / Negative assertion raced the authoritative UI reread after archive | 等待确认表单卸载及实际状态显示，再检查只读控件；保留检查器原字节 / Await dialog unmount and rendered state before assertions; retain original checker bytes |
| [before-lf-restoration](attempts/before-lf-restoration/scope.json) | 提交前发现四份原LF文件被格式化为CRLF，差异不必要扩大 / Four original LF files were formatted as CRLF | 恢复LF，旧完整通过证据及5份媒体逐字节存档，受影响后端／Linux／页面重新执行 / Restore LF, retain complete passed proofs and five exact media, then rerun affected checks |
| [source-preflight-01](attempts/source-preflight-01/result.json) | 缺少独立APK检查记录，复核按前置条件停止 / Missing APK inspection prerequisite | 实际执行17项包检查，再完成24项复核 / Run actual inspection, then final verification |
| [development-01](attempts/development-01/development-project-resources.json) | 两个超时清理留下的测试库 / Two residual test databases | 验证严格测试库名称、同一测试管理员及零连接后清理；[清理记录](fixture-cleanup.json)，复核剩余0 / Scoped inactive-fixture cleanup and zero residual databases |
| [backend-01](attempts/backend-01/result.json) | 第5批两次原生测试库DROP清理时限超时，均保留准确原始结果 / Two native database-drop cleanup deadlines in batch5 | 相同来源续跑失败批及未执行批，不改旧结果、不关闭耐久性 / Rerun unchanged-source failed/pending batches; retain failures and database durability |

Each failed browser attempt closed its owned contexts, server and fixture database. Its source manifest/result remain intact. Backend failed-batch raw output and progress are retained before resume; a failed attempt is not counted as a pass. Source hashes rather than timestamps or appearance decide whether evidence is current.

## 5. 原生复现命令 / Native reproduction

准备仓库已有原生工具链、隔离数据库与私有配置；从仓库根执行。先归档已有证据，采集脚本顺序使用本机5189端口；不接管原有夹具或浏览器。Android需明确开发API地址。No credentials belong in commands committed to Git.

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_backend_batches.py --batch-size 20 --evidence-group mobile-knowledge
# Only resume the exact unchanged-source incomplete run when necessary:
# python app/scripts/verify_backend_batches.py --batch-size 20 --evidence-group mobile-knowledge --resume
python app/scripts/verify_web.py mobile-knowledge
python app/scripts/verify_agent_runtime.py mobile-knowledge
python app/scripts/verify_linux.py mobile-knowledge
python app/scripts/run_mobile_knowledge_browser.py
python app/scripts/verify_mobile.py mobile-knowledge
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python app/scripts/build_android.py --google-mirror --evidence-group mobile-knowledge
python app/scripts/inspect_android_apk.py --evidence-group mobile-knowledge
python app/scripts/freeze_mobile_knowledge_evidence.py
python app/scripts/encode_mobile_knowledge_media.py
python app/scripts/verify_mobile_knowledge_history.py
python app/scripts/verify_mobile_knowledge_normalization.py
python app/scripts/verify_mobile_knowledge_development.py
# Inspect the actual new screenshots/GIF before writing a new visual-review record.
python app/scripts/verify_mobile_knowledge_sources.py
python app/scripts/verify_repository_publication.py
python app/scripts/verify_repository_publication.py --staged
```

真实四端、120GB单文件／2TB迁移、OSS/R2、真实模型／TG、依赖安全及生产恢复等门槛继续留在[28项差异表](../../../delivery/03-goal-gap-assessment.md)。八项Skill仍为草稿；本轮没有启用集成或执行生产部署。

The full platform goal remains open. Real four-client/device,120GB/2TB, OSS/R2, model/Telegram, dependency-security and production-recovery acceptance remain in the stable gap register. All eight Skills remain draft, and this increment does not enable integrations or deploy production.
