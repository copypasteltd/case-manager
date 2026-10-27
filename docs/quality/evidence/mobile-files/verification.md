# 增量94准确验证 / Increment94 exact verification

应用实现：[移动文件与目录整理](../../../development/94-mobile-file-workspace.md)。本轮新增共享移动功能，保持Web、微信、抖音与Android完整目标；Goal仍active，不作为生产放行。

The shared mobile file workspace advances the full four-client goal. Fresh runs, unchanged-source reuse, controlled response faults and external acceptance are distinct.

## 本轮新执行 / Fresh current-source checks

| 检查 / Check | 结果 / Result | 范围 / Scope |
| --- | --- | --- |
| [移动构建](mobile-build.json) |233/233，零失败／跳过；类型、微信、抖音构建PASS |185项完整来源不变；两小程序各20页，含文件中心；未运行平台 |
| [Android构建](build.json) | RN bundle与原生Gradle均PASS | 本机原生构建，无模拟器／真机／正式分发 |
| [APK检查](apk-inspection.json) |18/18 PASS；20页实际业务图均纳入 | 开发包，不是商店／真机验收 |
| [JVM范围](android-test-scope.json) |26项，`> Task :app:testDebugUnitTest UP-TO-DATE` | Gradle缓存复用，`new_test_execution=false`；原日志与4套XML原字节保留 |
| [实际浏览器](../mobile-files-browser/result.json) |9组流程、8项布局、2个身份／上下文均清理 | 真实本机Go API、隔离PostgreSQL、受控对象字节和Taro H5 |
| [原生只读检查](development-project-resources.json) |75迁移、归属API／worker、零隔离库PASS | 模型、TG、云关闭；不改变业务资料或启用外部接入 |
| [媒体清单](media.json)及[直接视觉检查](visual-review.json) |4PNG、2MP4、2GIF | 认证后实际像素，合成资料，无密码／MFA；全目录82／31／31 |

开发APK：42,935,958字节，SHA-256 `2fb9b78c523408cbe41695cf7996b9cccdf1248a55a85d3a705e68f0a267dc0d`。产物保留在忽略的 `app/.local/android/artifacts/mobile-files/2fb9b78c523408cbe41695cf7996b9cccdf1248a55a85d3a705e68f0a267dc0d/app-debug.apk`，APK、签名私钥及工具缓存不进入Git。移动完整来源摘要见mobile-build；Android来源见build；浏览器从启动、编译到检查绑定同一945项清单，摘要 `2809ab3a2d3a87c66c423cdbcc3aee3b4455b418ab26b1bcb9af0388a54f7172`。

The APK is a locally retained development artifact. Its exact digest and source records do not establish native execution, signing/distribution or production readiness.

## 九组实际流程 / Nine actual flows

1. UI新建目录，再改名／选择真实同范围目录移动；核对父目录、分类、项目、源修订、原因与完整回执。
2. UI文件改名／移动、回收与原位恢复；修订连续加一，当前正式内容保持不变；回收时读内容按钮禁用。
3. 展示两个实际正式版本，准确读取历史第一版；原SHA-256与UTF-8文字核对，嵌入脚本作为文字，不执行；不替换为第二版。
4. 实际服务器重名409：保留输入并阻止直接重提。
5. 实际API并发改名后旧源修订409：保留文字、不覆盖同事的修订。
6. **明确故障注入**：浏览器拦截并转发真实POST，服务器实际提交200后丢弃浏览器响应；禁重提，刷新读取准确已提交记录，仅加一版。
7. **明确故障注入**：真实创建已提交，浏览器仅删回执一个字段；不宣布成功、不清空草稿，独立读取实际记录。
8. 创建23个实际同父目录资料，名称筛选与不透明游标20+3；不混入其他目录。
9. 第二身份实际查阅，真实移除项目成员使旧会话401；刷新后清除文件并返回登录。

The first five and final two flows exercise actual server rules without fabricated successes. Flows6/7 label browser-only response faults after real commits. Fixture object bytes are controlled; no actual cloud/model/TG/device is invoked.

历史样本只有真实两版；新版本列表具备游标加载，但本轮不声称已用20版以上历史实测。23项分页是目录数据。8项布局是两个页面各375×812竖屏、900×390横屏、1024×900平板及375×812暗色／减少动效／200%根字号；检查溢出与受测操作控件高度，不能替代实体设备、系统字体、输入法、屏幕阅读器和安全区验收。

History uses two actual versions;23-folder pagination is separate. Eight H5 layout checks are browser-only. A root-font override is not native dynamic-type or complete accessibility acceptance.

## 逐项复用 / Explicit unchanged-source reuse

| 原证据 / Existing record | 数量 / Count | 本轮声明 / Current claim |
| --- | --- | --- |
| [93全后端](../agent-indexed-search/result.json) |167顶层、1129结果、1027末级、9批 | 完整后端来源摘要相同；本轮未重新运行全量 |
| [93 Web](../agent-indexed-search/web-build.json) |130项及生产构建 | 完整Web检查来源相同，复用原结果 |
| [93运行时](../agent-indexed-search/runtime-build.json) |51项及构建 | 完整运行时／Skill来源相同，复用原结果 |
| [93 Linux](../agent-indexed-search/linux-build.json) | 只编译 | 后端来源相同；无执行／部署声明 |
| [90依赖审计](../dependency-hardening/dependency-audit.json) |86项：0critical／20high／64moderate／2low | lock完全相同，未重新扫描；安全放行仍false |

本轮无Go、API、75项迁移、依赖或八项draft Skill改动。原API仍285路径／353操作／622 Schema。完整来源等同性由[最终检查](final-source-check.json)逐项核对。

## 媒体、历史与诊断 / Media, history and diagnostics

- [目录MP4](../../../media/videos/mobile-files-directory.mp4)／[GIF](../../../media/gifs/mobile-files-directory.gif)：9.349秒，46个原始采集帧。
- [文件MP4](../../../media/videos/mobile-files-lifecycle.mp4)／[GIF](../../../media/gifs/mobile-files-lifecycle.gif)：10.393秒，51个原始采集帧。
- 两GIF本轮均原速1.0；静音，保留真实加载与页面滚动。PNG是实际390×900视口，长表单及历史须继续滚动；不是整页拼接。原始JPEG帧仅驻忽略目录，发布其摘要和时间清单。
- [已发布93](historical-93.json)：从提交54f340c1c17c59bc70ac640330a26590c5adad2f重建完整来源，89份证据／媒体含8份媒体保持原字节；[10份入口快照](publication-before.json)独立保留。
- [第一次诊断](attempts/browser-01/attempt.json)：三项实际流程已通过；浏览器直接hash回到Taro缓存页面后选错生命周期，改用真实导航。
- [第二次诊断](attempts/browser-02/attempt.json)：录制限定范围正确拦截测试经过Home；改用同文件页面的真实目录行。
- [第三次诊断](attempts/browser-03/attempt.json)：八项实际流程通过，第二身份导航目标在第一页20条之外；改用真实名称筛选。
- [原通过快速录制](attempts/browser-04-fast-capture/attempt.json)：九项全通过，原3秒画面过快；保留通过状态、完整来源和7份原媒体／清单，增加核对停留后重录。未将失败标为成功。
- [旧APK页面数诊断](diagnostics/05-apk-page-count/diagnostic.json)：旧检查器仍要求19页，保留准确脚本与失败报告后同步为20页并明确要求文件页面；这是有限检查诊断，非全历史来源。

- [发布清单范围诊断](diagnostics/06-publication-scope/diagnostic.json)：首次将二进制媒体／ZIP及原始日志加入代码与指南的空白清单而误报；保留报告和原基线后限定为代码／指南格式，媒体／日志／归档原字节不变，仍参加其他完整发布检查。

## 重建与边界 / Reproduction and limits

使用原生Windows Go／PostgreSQL、Node／Taro、JDK／Android工具及ffmpeg；不调度Docker／WSL／虚拟机／模拟器。凭据与配置只在忽略路径。已有证据不可直接覆盖；重跑浏览器前先归档准确尝试，检查其所有权与清理结果。

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_mobile.py mobile-files
python app/scripts/run_mobile_files_browser.py
python app/scripts/encode_mobile_files_media.py
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python app/scripts/build_android.py --google-mirror --evidence-group mobile-files
python app/scripts/inspect_android_apk.py --evidence-group mobile-files
python app/scripts/freeze_mobile_files_evidence.py
python app/scripts/verify_mobile_files_development.py
python app/scripts/verify_mobile_files_sources.py
python app/scripts/verify_repository_publication.py
```

浏览器／上下文、5189临时服务、隔离fixture与测试数据库已清理；保留日常开发归属API／PG／worker。后续更新代码与中英README、受影响文档／媒体和CHANGELOG一起检查、提交并推送，禁止强推。

真实微信／抖音、Android设备／签名／分发、真实云／跨境网络／模型／TG、120GB／2TB、完整业务角色旅程、安全修复与恢复验收继续在[28项差异](../../../delivery/03-goal-gap-assessment.md)跟踪。跨范围移动和保留清理执行器等功能不在本轮实现中。
