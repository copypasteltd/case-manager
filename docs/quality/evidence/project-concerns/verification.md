# 增量84验证 / Increment84 verification

本轮新增秘书管理项目风险与问题的四个完整工具，以及 Web／Taro 的准确表单、状态确认、清空语义和回执展示。八个 Skill 继续为 draft，完整平台 Goal 未完成。方案、使用流程、参数和状态机见[开发文档](../../../development/84-secretary-project-concerns.md)。

This increment adds four complete secretary risk/issue tools and matching Web/Taro forms, explicit state confirmation, nullable clearing and genuine receipts. All eight Skills remain draft. The full platform goal remains open.

| 验证 / Check | 实际结果 / Actual result | 范围 / Scope |
| --- | --- | --- |
| 完整 Go／SQL／契约 / Complete backend | 156顶层、1,001结果、908末级，8批全部通过 / All passed | [Full result](result.json)、[batch ledger](batch-progress.json)；原生隔离 PostgreSQL，70项迁移 |
| 新业务集成 / New business integration | 14组场景通过 / Fourteen scenarios passed | 原业务创建／修改、双权限、风险转问题、事务失败回滚、准确重试、版本停止、原登录、取消与不可变历史 |
| 运行时 / Runtime | 43项及构建通过 / Tests and build passed | [Runtime proof](runtime-build.json)；四工具只进入 project-update，拒绝缺项及伪造完成证据 |
| Web | 88项及构建通过 / Tests and build passed | [Web proof](web-build.json)；完整共享规则与真实结果核对 |
| Taro | 183项、类型及微信／抖音编译通过 / Tests, types and both targets passed | [Mobile proof](mobile-build.json)；两个实际 TAP 套件合计，两个目标均为16页 |
| Android | 新42,885,880字节开发包，17项检查通过 / New debug APK and inspections passed | [Build](build.json)、[inspection](apk-inspection.json)；无模拟器或真机运行 |
| JVM | 25项结果，0失败／跳过 / Results without failures or skips | [Exact scope](android-test-scope.json)；实际标记为 `:app:testDebugUnitTest UP-TO-DATE`，本次使用缓存，不宣称重新执行 |
| Web 实际页面 / Actual Web | 8组流程、3种宽度 / Eight flows, three widths | [Browser proof](../project-concerns-web/result.json)；620份来源，1个自建上下文已关闭 |
| Taro 实际页面 / Actual Taro H5 | 8组流程、3种宽度 / Eight flows, three widths | [Browser proof](../project-concerns-mobile/result.json)；642份来源，1个自建上下文已关闭 |
| 原生开发 / Native development | API更新、70项迁移、10只读探针通过 / API reload, migrations and probes passed | [Update](development-update.json)、[runtime probes](development-runtime.json)、[business constraints](development-project-resources.json)；0测试库／活动模型／启用TG |
| Linux | 当前源码交叉编译通过 / Cross-compilation passed | [Proof](linux-build.json)；产物未执行、未部署 |
| 最终来源复核 / Final source verification | 25项全部通过 / All passed | [Recomputed checks](final-source-check.json)；源码、实际媒体、历史、缓存边界和清理共同核对 |

Android APK SHA-256: `53fdf08f805bb7536206875c008faee60da7e2d2cad4876fd6cb34ee2c913c42`。二进制位于忽略的本地制品目录，不提交至 Git。Backend source SHA-256: `315a9b73b39abc2d9760401ff673c3f4fb2c7e184eac588525fd21fd72b2c50c`。

The APK stays in the ignored local artifact directory. It is a development package, not a signed production release or device acceptance result. The owned PostgreSQL and notification worker were not restarted; external integrations remained disabled.

## 实际媒体 / Actual media

| 客户端 / Client | 原始像素采集 / Actual pixel capture | 输出 / Encoded output |
| --- | --- | --- |
| Web | 9张JPEG，1,853ms，1440×1100 / Nine actual frames | 2.0秒MP4；12帧／2,000ms GIF |
| Taro H5 | 9张JPEG，1,833ms，390×900 / Nine actual frames | 2.0秒MP4；12帧／2,000ms GIF |

录像展示认证后的完整事项表单转为准确预览。风险发生的新增问题影响明确可见；预览不执行业务。真实发送、重试和取消分别通过原生 API 核对，画面中的已提交历史来自同一个原业务夹具。保留 [Web](web-capture-manifest.json)／[mobile](mobile-capture-manifest.json) 的真实帧时间与摘要；[编码记录](media.json)绑定编码器、原始帧和输出摘要。[像素复核](visual-review.json)绑定四张截图及两份实际 GIF。

The clips show an authenticated complete form becoming an exact preview. The linked-issue effect is visible. Separate real API assertions verify send, retry and cancellation. Committed history comes from the same native business fixture. The original timing manifests, encoder record and visual review preserve the actual provenance. Passwords, MFA and private capability screens are excluded. MP4 durations include the bounded encoder frame cadence; GIF timing remains within the recorded tolerance.

本轮新增4张PNG及两对MP4／GIF；活动素材目录实际为 **36 PNG、11 MP4、11 GIF**，不把 `attempts/` 的诊断截图计为活动演示。[演示目录](../../../media/README.md)提供完整播放入口。

## 历史与失败记录 / Retained history and failures

增量83的采集按已发布提交 `afbb92608440c70a295623ff293b323bf07fd55b` 的805份原 Git blob 重建，摘要为 `8ca75b2a5a6190a980ea814fd32bb186c0c62122ddb4bd5498a697707345c3cd`。见[历史来源记录](historical-83.json)。旧结果保持原样，不以新源码重标旧执行。

Increment83 remains bound to its 805 original Git blobs. Its old records are unchanged; no prior check is relabeled as a current execution.

`attempts/` 保留两次迁移语法修正前的原SQL与诊断、测试辅助类型／夹具SQL问题、旧工具数量断言、运行时清单上限和结果引用类型补齐前的诊断，以及浏览器夹具MFA、录屏保护选择器、真实滚轮和候选列表操作修正前的记录。未冻结原失败源码的条目仅作为诊断，不能当作来源绑定的通过证明。曾通过但随后因采集脚本修正而过期的Web记录也单独保留，最终两端均按同一完整来源重新执行。

Failed attempts retain their actual diagnostics and, where frozen, exact original bytes. Diagnostic-only entries are not source-bound passing evidence. Successful Web captures superseded by later capture-helper fixes are retained separately. Both final clients were rerun against the same complete implementation digest: `13e210075540a94db51bd72aca2a1f8fcb215a0ad28b61063d845e9d95b6f785`.

## 复核与边界 / Recheck and boundaries

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_project_concerns_sources.py
python app/scripts/verify_repository_publication_development.py
python app/scripts/verify_repository_publication.py
```

浏览器、上下文、临时Web/H5服务和隔离夹具均已关闭，端口5189已释放，私有夹具文件已移除；保留日常原生 API／Worker／PostgreSQL。没有启动 Docker、WSL、虚拟机或模拟器，没有真实模型／TG外发／云调用。120GB真实传输、2TB迁移恢复、境内服务器与香港网络、四端实机／平台、依赖安全和生产切换仍按[28项差异](../../../delivery/03-goal-gap-assessment.md)推进。

All owned browsers, contexts, temporary review servers and disposable fixtures were closed. Port5189 and the private fixture file were released. Daily native services remain. No Docker/WSL/VM/emulator, real model/TG/cloud execution or production deployment occurred. Capacity, devices/platforms, dependency security and production release gates remain open.
