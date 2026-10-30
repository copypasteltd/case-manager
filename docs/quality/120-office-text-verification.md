# Office 正文验证 / Office text verification

增量120在 GAP-04／18 内增加 XLSX／PPTX 的有界文字读取，沿用正式版本、准确对象副本、扫描、当前权限与不可变派生证据。当前工作包仍为23项（14 P0／9 P1），五项移出；本轮不是完整 Office 编辑、真实云／模型或生产验收。

Increment120 adds bounded XLSX/PPTX text to the formal-version pipeline. Current authorization, scan results, exact object identity and immutable derivations remain required. All23 packages and five exclusions remain unchanged. The checks below distinguish current local execution, controlled dependencies, retained historical evidence and outstanding production acceptance.

## 验证矩阵 / Verification matrix

| 编号 | 检查 | 证据及实际边界 |
| --- | --- | --- |
| V120-01 | 解析与完整后端回归 | [完整批次](evidence/office-text-backend/batch-progress.json)、[最终结果](evidence/office-text-backend/result.json)、[原始日志清单](evidence/office-text-backend/raw-log-archive.json)；原生PG，13批覆盖全部242个顶层用例、1526条通过结果，应用输入固定。受控对象读取不代表OSS／R2实云。 |
| V120-02 | Web／移动／运行器 | [八项命令](evidence/office-text/client-checks.json)：Web313、共享移动409、运行器56；Web类型／构建、微信／抖音各20路由构建、八项Skill结构检查。未执行真实平台或模型。 |
| V120-03 | 原有数据正常结构升级 | [91→92](evidence/office-text/schema-upgrade.json)：从已发布119源码和结构91建立有真实测试记录的数据库，再用当前迁移升级；12组事实摘要保留，9条旧正文派生保留，无对象读取／正文回填。这是普通升级，不是历史迁移或容灾项目。 |
| V120-04 | Web实际交互 | [五组检查](evidence/office-text-web/result.json)：当前正文检索、两种准确版本预览、逐侧依据比较、秘书资料API同摘要、窗口布局；真实HTTP与合成正式版本。 |
| V120-05 | Taro H5实际交互 | [五组检查](evidence/office-text-mobile/result.json)：与Web相同的真实业务接口和390px页面；不能替代微信、抖音或Android实际平台验收。 |
| V120-06 | 媒体 | [编码清单](evidence/office-text/media.json)、[视觉复核](evidence/office-text/visual-review.json)：7张PNG、两组实际页面MP4／GIF。登录后录制，无密码／MFA／真实业务资料。GIF是有时间依据的实际页面采样帧，并非模型生成画面。 |
| V120-07 | Android开发包 | [构建](evidence/office-text/build.json)、[结构／签名检查](evidence/office-text/apk-inspection.json)、[Office内容](evidence/office-text/android-feature-content.json)；有披露的单worker Metro调用，保留原生主机构建的无Expo保护及全部Gradle／APK校验。仅开发签名arm64包，不启动设备或模拟器。 |
| V120-08 | Android JVM范围 | [26项范围说明](evidence/office-text/android-test-scope.json)、[Gradle日志](evidence/office-text/native-build.log)：本轮26项为缓存复用，非新执行；实际任务标记与XML保留。原始APK、签名和工具留在Git外。 |
| V120-09 | Linux交叉构建 | [两个ELF目标](evidence/office-text/linux-build.json)：平台及原生代理可编译，不表示Linux执行、cgroup隔离或生产发布通过。 |
| V120-10 | 开发环境与清理 | [开发服务](evidence/office-text/development-project-resources.json)、[开发升级](evidence/office-text/development-update.json)、[测试集群关闭](evidence/office-text/native-cluster-closed.json)：只处理本轮所有的测试资源，保留原开发服务；云、TG、模型外连继续关闭。 |
| V120-11 | 来源、保留与发布 | [最终来源](evidence/office-text/final-source-check.json)、[原证据保留](evidence/office-text/reused-evidence.json)、[源码存档](evidence/office-text/source-archive.json)、[仓库检查](evidence/office-text/repository-check.json)。旧316份媒体及全部已发布证据保留原字节；依赖审计沿用相同锁文件的既有证据，未宣称新审计。 |
| V120-12 | 当前范围 | [范围核对](evidence/office-text/scope-check.json)、[AC／TC适用性](04-current-acceptance-applicability.md)：分类核对不等于执行全部30 AC／72 TC。AI秘书、TG、隔离、授权和审计保留，仅AI费用控制移出。 |

上述链接由最终检查生成。最终完成状态以各JSON中的`passed`及来源摘要为准，不以本表中的用例名称代替结果。

## 本轮重点 / Meaningful cases

- XLSX按工作簿关系顺序读取共享／内联／富文本与坐标，处理Unicode转义、1900／1904日期标识、隐藏工作表／行列、合并范围和惰性公式／文件缓存；不将缓存数值写入财务事实。
- PPTX按实际幻灯片顺序提取形状、分组、表格、换行及讲者备注；隐藏幻灯片带标识，备注过滤页码等占位。严格与过渡命名空间均覆盖。
- ZIP/XML有界；拒绝路径越界、重复部件／关系、宏／活动内容、DTD、异常层级、歧义条件内容及超限；失败不发布部分正文，不执行外部关系或公式。
- HTTP集成经过正式提交、准确副本与扫描证据、真实队列和worker。验证旧`text-v1` Office不支持记录保留，新`office-v1`独立去重，当前版本检索不混入旧版本／重复正文，撤权／分类／扫描变化阻止读取。
- 全文比较绑定每侧原件、提取标识、正文摘要与完整分页；跨MIME的`mixed`比较独立说明原件相同与正文相同，不能以相同原件字节推断不同提取方式的正文相同。
- 秘书资料预览消费与人工预览相同的固定正文摘要，并在移除对象读取凭据后仍可读取已授权派生正文；这是读取链路验证，不是实际Codex生成质量或Skill评估。

## 保留的尝试 / Attempts retained separately

| 编号 | 记录 | 处理及最终计数 |
| --- | --- | --- |
| A120-01 | [初轮来源](evidence/office-text-attempts/01-before-mixed-provenance/preservation.json) | 初轮客户端／Web已运行，随后发现跨提取方式的相等说明需要修正；单独存档，最终客户端与页面均重新执行。 |
| A120-02 | [并行构建时后端中断](evidence/office-text-attempts/02-concurrent-backend/preservation.json) | 秘书订阅用例出现`JOB_LEASE_LOST`，停止本轮所有的测试进程树并保留原日志；只清理已核对、无连接的所属测试库。最终未改应用租约、断言或截止时间，未完成批次以单Go包方式重跑。此前完整通过且同源的第一批保留。 |
| A120-03 | [移动定位修正](evidence/office-text-attempts/03-mobile-locator/preservation.json) | 检查脚本嵌套定位与等待拒绝处理有问题；调整脚本后两端重新执行。应用源码不变。 |
| A120-04 | [窗口布局等待](evidence/office-text-attempts/04-web-layout-settling/preservation.json) | 四组业务通过后即时缩窗的布局断言失败；保持原宽度要求，等待真实布局稳定后重新录制。应用样式不变。 |
| A120-05 | [Android停滞尝试](evidence/office-text-attempts/05-android-stalled/preservation.json) | 初次Metro长时间未生成新包，中止已核对的所属构建树并保存日志；不将旧APK算成本轮构建。 |
| A120-06 | [Android索引诊断](evidence/office-text-attempts/06-android-filemap/preservation.json) | 再次观察到文件映射未就绪、两个已派发任务的子进程空闲。独立同索引器调用可返回，根因未确认。只对所属编译进程短暂连接诊断器并关闭；终止该尝试，改用同一Metro CLI的单worker调用，应用来源和保护不变。 |

Failure and interruption evidence is retained independently. Final source reports contain only completed checks; early captures, failed batches and cached JVM results are not relabelled as fresh successful executions. Controlled object transport and disabled model integrations remain explicit limitations.

增量119的发布工程源码继续保留，但其原打包证据绑定当时的结构91与来源；本轮未重新生成生产发布包。发布本轮源码时须生成新的结构92产物，不能把原119包解释成本轮包。

## 复现与运行边界 / Reproduction

使用原生Go、PostgreSQL、Node22及既有JDK／Android SDK。不要运行Docker、WSL、虚拟机、设备或模拟器。独立测试库和截图仅使用合成数据，私有日志／对象／APK／源码压缩存档留在`app/.local/`，不提交Git。

```powershell
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python app/scripts/verify_office_text_clients.py
python app/scripts/freeze_office_text_clients.py
python app/scripts/verify_office_text_backend.py --batch-size 20 --resume
python app/scripts/freeze_office_text_backend.py
python app/scripts/verify_office_text_upgrade.py
python app/scripts/run_office_text_browser.py --help
python app/scripts/build_office_text_android.py --google-mirror
python app/scripts/inspect_android_apk.py --evidence-group office-text
python app/scripts/verify_office_text_android_content.py
python app/scripts/freeze_office_text_android.py
python app/scripts/build_office_text_linux.py
python app/scripts/encode_office_text_media.py
python app/scripts/verify_office_text_sources.py
python app/scripts/verify_current_scope.py
python app/scripts/verify_acceptance_applicability.py
python app/scripts/verify_repository_publication.py
```

这些命令展示本轮使用的脚本入口，需先按双语指南配置原生工具；不能当作新克隆的一键复验。`--resume`用于本机已有的同源批次，只复用同一后端摘要的已完整通过结果，不能在来源变更后复用；新执行须先保留原证据并使用独立记录。浏览器脚本必须由其原生测试包装器建立隔离夹具，并在结束后关闭所有自建页面、上下文、代理、临时服务及测试库。已冻结的证据发生变化时先保留旧尝试，不覆盖既有发布记录。

仍需完成PDF／OCR、复杂DOCX、Office创建／编辑与版式、真实业务文件逐格式核对和四端完整业务旅程。真实Codex／八项Skill、Linux隔离、TG、云对象、平台及正式发布按各原工作包继续处理。当前移出项不重新成为发布门槛。

[设计与使用](../development/120-office-text-extraction.md) · [全部未完成项](../delivery/03-goal-gap-assessment.md) · [媒体入口](../media/README.md)
