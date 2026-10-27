# 增量93准确验证 / Exact verification

本轮把文件正文索引接入原公司秘书MCP检索与固定版本读取。以下分别记录当前源码的原生检查、实际页面像素、受控合成对象、历史原字节和仍未完成的正式验收；不以数量计算整体完成率。

This increment connects existing secretary MCP tools to body discovery and immutable indexed DOCX reads. Native PostgreSQL/HTTP/MCP checks use isolated synthetic records and a controlled object source. No actual cloud, live model, Telegram or physical device was contacted.

| 范围 / Scope | 实际结果 / Result | 来源与限制 / Evidence and limits |
| --- | --- | --- |
| 后端 / Backend |167顶层／1,129结果／1,027末级，9批全量通过 | [result.json](result.json)、[batch-progress.json](batch-progress.json)；原生PostgreSQL与完整来源，不是单个Go进程 |
| 新协议 / New protocol |12个HTTP／数据库／MCP子场景；3个Unicode子场景 | 实际正文命中、字面字符、准确历史版本、两类摘要、待提取／不支持、游标、持久回执／精确重试、取消、回收／隔离及共同成员权限；不冒充模型判断质量 |
| 迁移／接口 / Schema/API |75项迁移全部与已发布92原字节一致；285路径／353操作／622 Schema | 新增可选text_index证据契约，无新数据库迁移；两个OpenAPI副本相同 |
| Web |130测试及生产构建通过 | [web-build.json](web-build.json)，完整声明来源与运行前后一致 |
| 运行时 / Runtime |51测试及构建通过 | [runtime-build.json](runtime-build.json)，当前八项draft Skill字节；真实读回执接受及伪造来源拒绝，不代表真实模型运行 |
| Taro |222测试、类型检查、微信／抖音各19页编译 | [mobile-build.json](mobile-build.json)；[原记录位置说明](mobile-record-location.json)，没有改写检查结果；编译不等于平台真机验收 |
| Android | 当前原生开发APK、18项产物检查通过 | [build.json](build.json)、[apk-inspection.json](apk-inspection.json)；无模拟器／虚拟化、无真机／正式签名／商店验收 |
| JVM |26结果、4份XML；本次Gradle为UP-TO-DATE | [实际缓存范围](android-test-scope.json)、[冻结日志](native-build.log)、[XML](android-unit-results/)；明确缓存复用，不称新执行26测试 |
| Linux | 当前后端交叉编译通过 | [linux-build.json](linux-build.json)，没有执行或部署生成产物 |
| Web／共享H5 | 各8项实际认证UI／API检查 | [Web记录](../agent-indexed-search-web/result.json)、[移动记录](../agent-indexed-search-mobile/result.json)；各一个录制视口，不称三宽度或实体设备 |
| 媒体 / Media |4PNG＋2MP4＋2GIF；完整目录78／29／29 | [媒体绑定](media.json)、[视觉复核](visual-review.json)；Web14帧、移动28帧，保留较快原时序，可调慢观看；不录制密码／MFA |
| 原生开发 / Development |75迁移、准确归属API及两worker、零隔离测试库通过核对 | [状态](development-runtime.json)、[资源／约束](development-project-resources.json)、[更新过程](development-update.json)；外部存储／模型／TG关闭 |
| 历史 / History | 已发布92完整启动／捕获来源从Git重建，91份证据／媒体保留原字节 | [historical-92.json](historical-92.json)，发布提交5de7553；8份历史媒体及10份入口[原快照](publication-before.json) |
| 依赖 / Dependencies | 相同锁文件沿用原90完整审计86项，20 high | [原审计](../dependency-hardening/dependency-audit.json)；0 critical、64 moderate、2 low。没有重新审计或生产安全放行 |

APK：42,927,831字节；SHA-256为 `b9939b8f8b91042df1de5648d00bf1608a2d2e6aa2073c7e40610c09d771f189`。开发APK、密钥、原始录屏帧、数据库和工具链保留在忽略路径，不提交Git。记录内不可变产物路径用于本机复核。

## 页面与MCP证据的关系 / UI and MCP evidence

实际页面演示：选文件／准确版本 → 核对DOCX正文和两类摘要 → Unicode续读 → 选择明确历史版本 → 同一文件的两个版本加入请求。没有隐式发送（implicit_sends=0），没有用模拟模型输出替代真实模型。浏览器在实际认证后开始取帧。

The controlled native integration fixture independently performs real MCP body search and fixed-version reads, commits durable read receipts and completes a result using those receipts. Later lifecycle and shared-authority negative cases change the dependencies. The browser explicitly verifies that the earlier result is now unavailable with empty body and source references; it does not claim that old citations remain visible. See each client's [acceptance record](../agent-indexed-search-web/acceptance.json).

正文索引证据包含提取器、原件SHA-256、正文SHA-256、索引时间和字符数。读取时重新检查正式版本、扫描、会话、全部成员权限与AI处理范围；不会把索引时间当作本次云访问时间。DOCX运行时从不可变派生记录读取，原UTF-8路径仍要求独立AI只读身份及原件校验。检索片段不是全文阅读或写授权；PDF／OCR／其他Office正文及DOCX写入不在本次实现。

## 原始失败与过程边界 / Original failures and limits

| 记录 / Record | 原状态与处理 / Original status and correction |
| --- | --- |
| [01 原生角色夹具](attempts/01-shared-role-cast/attempt.json) | SQLSTATE42804，合成角色INSERT参数缺UUID强制类型；完整当时后端源码ZIP保留，修正后由最终全量覆盖；不声称冻结当时Web／移动 |
| [02 Web认证观察](attempts/02-web-cookie-observation/attempt.json) | 七项页面流程通过后，检查器误要求Web Bearer头；实际为HttpOnly cookie。原失败报告及完整启动／捕获来源保留 |
| [03 运行状态观察](attempts/03-run-state-observation/attempt.json) | 检查器把ProposedResult的completed当Run状态；实际终态为succeeded。原失败／完整来源保留 |
| [04 历史结果撤回](attempts/04-historical-result-withdrawal/attempt.json) | 夹具后续权限变更应撤回旧结果；检查器误期望旧引用。改为校验实际unavailable／空正文／空引用，保留原失败及完整来源 |
| [05 开发检查观察](attempts/05-development-observation/attempt.json) | 复制检查器时误改现有SQL索引名且保留旧Skill版本断言；修正观察规则，未修改数据库。仅冻结准确脚本／报告，不称完整旧应用来源 |
| [06 来源清单观察](attempts/06-source-manifest-observation/attempt.json) | 组装检查器用较小捕获清单计算完整启动摘要；改用原ZIP内准确startup_manifest。仅冻结脚本／失败报告，无完整旧应用来源声明 |

| [07 发布文档检查](attempts/07-publication-whitespace/attempt.json) | 原需求文档结尾空行触发发布约束；保留原报告及准确文档，移除多余空行后重检；链接及敏感文件检查已通过 |

All original failed records remain failed and retain their recorded scope. No report was relabeled to manufacture a pass. Gradle caching, the unchanged dependency audit and historical-source reconstruction are explicitly separate from new execution. Source/assembly diagnostics are not product failures or evidence of deployment.

## 重建与维护 / Reproduction and maintenance

Windows原生PowerShell，从仓库根目录执行；使用已有本地工具链与被忽略配置。以下最终检查有来源绑定，相关源码改变后必须重新生成受影响的证据。

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_backend_batches.py --batch-size 20 --evidence-group agent-indexed-search
python app/scripts/verify_web.py agent-indexed-search
python app/scripts/verify_agent_runtime.py agent-indexed-search
python app/scripts/verify_mobile.py agent-indexed-search
python app/scripts/verify_linux.py agent-indexed-search
python app/scripts/run_agent_indexed_search_browser.py
python app/scripts/run_agent_indexed_search_browser.py --mobile
python app/scripts/encode_agent_indexed_search_media.py
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python app/scripts/build_android.py --google-mirror --evidence-group agent-indexed-search
python app/scripts/inspect_android_apk.py --evidence-group agent-indexed-search
python app/scripts/freeze_agent_indexed_search_evidence.py
python app/scripts/verify_agent_indexed_search_development.py
python app/scripts/verify_agent_indexed_search_sources.py
python app/scripts/verify_repository_publication.py
```

Android须先按[原生构建](../../../development/65-android-native-foundation.md)重新生成当前来源APK后检查，视觉复核也需对新素材执行。不要在同一检查过程中修改其绑定源码；Web／H5顺序执行以复用受控端口。发布前执行暂存内容检查，代码、双语指南、受影响素材及CHANGELOG一并提交推送，不强推。

No Docker, WSL, virtual machine or emulator was used. Each owned browser/context and temporary review server was closed, synthetic fixture databases were removed, and unrelated user tabs/processes were preserved. Daily native development services remain running under their existing ownership records.

真实IAM／OSS／R2、香港跨境网络、模型／TG、微信／抖音及Android设备、120GB上传、2TB纳管、双云恢复、依赖安全、正式签名／分发与生产放行仍按[28项差异](../../../delivery/03-goal-gap-assessment.md)推进。本轮不构成完整Goal完成。
