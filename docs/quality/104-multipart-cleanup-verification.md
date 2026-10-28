# 增量104验证：未完成上传清理 / Multipart cleanup verification

本轮交付代码、四端入口、独立Worker、迁移82及相关文档。**23个完整工作包仍待完成（14 P0／9 P1），5项移出；GAP-05只完成已知未完成分片子流程。** 当前功能来源检查见[最终证明](evidence/multipart-cleanup/final-source-check.json)，完整原生回归见[后端记录](evidence/multipart-cleanup-backend/result.json)。真实云、模型、TG及生产发布仍未据此验收。

| 编号 | 本轮检查／实现 | 实际结果与边界 |
|---|---|---|
| 104-01 | 候选与固定清单 | 准确上传修订、尝试来源、目录、保留制度和存储绑定；1至50项，观察期至少24小时 |
| 104-02 | 人工批准、撤销、明确重试 | 近期MFA与当前权限；批准封存、入队、审计、Outbox同事务；幂等不重复排队 |
| 104-03 | 独立执行器 | 专用身份、持久队列、fencing及逐次回执；只确认准确UploadId不存在 |
| 104-04 | 错误与恢复 | 撤权／来源变化阻断；审计失败回滚；不确定云结果先核对；最终崩溃补记失败，人工重试保留旧轮次 |
| 104-05 | 完整原生后端回归 | 198个顶层测试，1236条最终通过记录；完整发现集，来源未变，分批执行 |
| 104-06 | 客户端测试及构建 | Web 159／移动共享 261／runtime 52；8项当前源码命令；微信／抖音各20页 |
| 104-07 | Android原生构建与检查 | 新开发APK、18项包检查及4项本轮业务内容核对；JVM26项，cached=true，新JVM执行=false；不启动设备或模拟器 |
| 104-08 | Web／共享Taro H5交互 | 真实原生API及隔离PG：盘点、明确选择、建立清单、观察期、历史回执及刷新 |
| 104-09 | 页面证据 | 6张新PNG、2段实际像素MP4及2个GIF；旧189份媒体逐字节保留 |
| 104-10 | Linux与日常开发环境 | Linux交叉构建未部署；自有PG升级82，API及日常Worker来源更新；无外部集成启用 |

## 复现与使用

实现、状态机、配置、云身份和API见[开发文档](../development/104-multipart-cleanup.md)。源码在 `app/backend/internal/files/multipart_cleanup*.go`、`app/backend/internal/storageconfig/multipart_cleanup.go`、`app/web/src/MultipartCleanup.tsx` 和 `app/mobile/src/pages/files/cleanup.tsx`；新文档与证据统一在 `docs/`。

```powershell
python app/scripts/verify_backend_batches.py --batch-size 20 --evidence-group multipart-cleanup-backend
python app/scripts/verify_multipart_cleanup_clients.py
python app/scripts/build_android.py --google-mirror --evidence-group multipart-cleanup
python app/scripts/inspect_android_apk.py --evidence-group multipart-cleanup
python app/scripts/verify_multipart_cleanup_android_content.py
python app/scripts/run_multipart_cleanup_browser.py
python app/scripts/run_multipart_cleanup_browser.py --mobile
python app/scripts/encode_multipart_cleanup_media.py
python app/scripts/verify_multipart_cleanup_sources.py
python app/scripts/verify_current_scope.py
python app/scripts/verify_repository_publication.py
```

需现有原生PG及忽略配置；Android本地构建显式指定 `CP_MOBILE_BUILD_PROFILE=local` 与 loopback API 地址。后端已有结果需按脚本要求检查来源后 `--resume`，不可覆盖历史已完成尝试。其余冻结／保留／环境脚本在 `app/scripts/`，对应JSON记录保留命令、时间和来源摘要。

本轮完整后端回归的原始日志按独立目录保存在忽略路径 `app/.local/multipart-cleanup-backend/`，其准确字节与通过记录逐批核对，见[归档摘要](evidence/multipart-cleanup-backend/raw-log-archive.json)。这不代表此前增量的私有通用日志仍然存在；历史公开JSON与媒体按原来源保留。

## 媒体与实际范围

| 客户端 | 盘点 | 观察期 | 回执 | 操作演示 |
|---|---|---|---|---|
| Web | [PNG](../media/screenshots/multipart-cleanup-web-inventory.png) | [PNG](../media/screenshots/multipart-cleanup-web-observation.png) | [PNG](../media/screenshots/multipart-cleanup-web-receipts.png) | [MP4](../media/videos/multipart-cleanup-web.mp4)、[GIF](../media/gifs/multipart-cleanup-web.gif) |
| Taro H5 | [PNG](../media/screenshots/multipart-cleanup-mobile-inventory.png) | [PNG](../media/screenshots/multipart-cleanup-mobile-observation.png) | [PNG](../media/screenshots/multipart-cleanup-mobile-receipts.png) | [MP4](../media/videos/multipart-cleanup-mobile.mp4)、[GIF](../media/gifs/multipart-cleanup-mobile.gif) |

录制只包含认证后的合成资料。已完成回执来自同一夹具的真实Go业务执行及原生PG事务，云接口受控；该次已批准清单的观察期由隔离测试推进，不声称实际等待24小时或实际云删除。新建观察清单未跳过正式观察期。所有自建浏览器／上下文／页面、5189服务及夹具数据库已释放；保留日常自有服务。每项启动、执行、编码、来源不变和清理结果均有单独记录。

## 失败尝试

早期API专项分别发现错误的权限SQL布尔参数、分片尝试键匹配及Web Cookie／CSRF夹具调用问题，修正后重新执行当前来源完整回归；先前失败不拼接为通过。原始诊断日志保存在忽略目录，脱敏记录见[专项尝试](evidence/multipart-cleanup/diagnostic-attempts.json)。

首轮Web composite typecheck缺少新共享文件的include，原失败命令记录保留在[失败尝试](evidence/multipart-cleanup/attempts/typecheck-includes/client-checks.json)。重跑曾复用私有日志路径，原typecheck原始字节未保留，仅保留其原摘要与脱敏错误，不声称原日志仍可复验。最终8项通过证明对应各自现有原始字节。首次Web浏览器导航25秒超时，原失败及资源释放记录保留在[导航尝试](evidence/multipart-cleanup-web/attempts/initial-navigation-timeout/result.json)，扩大有界导航时间后重新执行。

最终来源核验首轮误设清理专项至少22条记录，实际21条均通过。保留[原失败证明](evidence/multipart-cleanup/attempts/source-check-scenario-count/final-source-check.json)及诊断，改为逐项要求14个具名安全场景；未修改业务源码、测试结果或降低完整198项发现集要求。

依赖树未变，引用增量103已发布的70项审计结果（6 high／57 moderate／7 low），**不是本轮新审计或已修复**。JVM缓存状态严格来自本轮Gradle任务和XML原字节，不把缓存说成新执行。正式历史／草稿／孤儿对象清理、真实云IAM、全四端业务验收及[全部23项](../delivery/03-goal-gap-assessment.md)继续开放。

用户修订99继续有效：不做真机、网络性能／实网120GB专项、历史迁移／容灾或AI费用控制；保留四端、秘书及8项Skill、TG、隔离／权限／审计、技术限制、公司财务、正常PG升级与OSS＋R2复制。

English: All 198 discovered backend tests pass with 1236 result records. Web 159, mobile 261 and runtime 52 checks are current-source results. Four-client builds, synthetic browser flows, retained media and the actual Android JVM cache state are reported separately. Live-cloud, model, TG and production acceptance remain open; no excluded activity is treated as a pass.
