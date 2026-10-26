# 增量81验证 / Increment81 verification

发布日期：2026-10-27（Asia/Taipei）。构建／测试从前一日开始，JSON 时间使用 UTC。完整平台 Goal 保持 active；本记录只证明当前准确源码下的本地范围。

Release date: 2026-10-27, Asia/Taipei. Builds/tests began the previous day; JSON timestamps use UTC. The complete platform goal remains active.

## 功能与准确来源 / Behavior and exact sources

新增独立原生 Prometheus／Blackbox 状态桥接。成熟监控负责实际探测，桥接固定标签查询原采样与 timestamp，持久保存最终签名正文，再走原观测入口。回执增加来源、来源修订、序号和原始字节 SHA-256 绑定。旧 API 的不完整回执会被拒绝，须先升级 API 再启用桥接。

The native bridge reads existing metrics and original scrape timestamps, persists the exact signed request before delivery, then uses the original observation API. Its acknowledgement binds source, revision, sequence and payload digest. Upgrade the API before enabling the bridge; an old incomplete ACK is refused.

完整后端 SHA-256：`9422bb5963afb514669192f63bb151c69eab4a51f97f0add717eb3ccdfdef34e`。浏览器采集绑定全部787份当前后端、Web、移动、契约、原生部署及检查／录制辅助来源：`f4b53add6b468e92b9077a9c1acdc22d06cb5788ed553cfb67a9fa17431cd157`。均核对运行前后字节未变。

## 执行与复用 / Execution and reuse

| 层面 / Layer | 结果 / Result | 准确范围 / Scope |
| --- | --- | --- |
| [后端全量](result.json)／[八批进度](batch-progress.json) | 新执行154顶层、984条结果、892末级场景，零失败／跳过 | All discovered tests executed in eight batches, current complete Go/SQL/embedded contract digest |
| [桥接二进制](bridge-build.json) | Windows/amd64 与 Linux/amd64 构建通过 | Windows实际执行；Linux只编译，均未部署 / Windows executed; Linux compile only, neither deployed |
| [CLI密钥检查](key-generation.json) | 生成32字节seed，拒绝覆盖既有文件，私密临时目录删除 | Actual CLI, safe metadata only; no seed/public output published |
| [Web](web-build.json) | 新81项测试及生产构建通过 | Current complete Web/shared-contract source; no stale proof reuse |
| [移动](mobile-build.json)／[原输出](mobile-verification.log) | 新154项测试、类型及微信／抖音各16页编译通过 | Native Node/Taro; no actual platform/device claim |
| [Android构建](build.json)／[17项检查](apk-inspection.json) | Metro／Gradle新执行，冻结42,858,139字节开发包 | Current full portable/native source binding; debug APK, no device/release claim |
| [原生测试范围](android-test-scope.json)／[当次Gradle输出](native-build.log) | 25项结果，任务为 `UP-TO-DATE` | 四份XML完整冻结；不是25项新执行 / Four exact XML files retained; not newly executed tests |
| [实际浏览器](../status-bridge-browser/result.json)／[完整来源](../status-bridge-browser/source-manifest.json) | 六组流程、九项布局、十二次实际CLI调用；两上下文关闭 | Actual Web and Taro H5, native bridge and API; controlled Prometheus protocol endpoint, not an installed monitor |
| [Linux平台](linux-build.json) | 按当前后端新编译 | Not executed or remotely deployed |
| [运行时原证明](../deliverable-creation/runtime-build.json) | 完整运行时源码相同，准确复用39项受控测试 | Not rerun; actual model remains disabled |
| [日常只读核对](development-runtime.json)／[业务约束](development-project-resources.json) | 69迁移摘要、十项探测、原API/Worker身份通过；0测试库／活动模型／启用TG | Original daily processes preserved; no restart or integration activation |
| [最终来源核验](final-source-check.json) | 25项通过 | Exact current sources, artifacts, source-bound reuse, actual cache scope, media and cleanup |

Android产物 SHA-256 为 `4263eb232c2605f2e5f079ab8bab76030b2a2de9fd8dfc5946cd13fe555fda6e`，与增量80字节相同：本次共享生成类型增加回执字段，RN运行代码未变；报告仍绑定本次完整来源与新构建，没有把旧构建报告换名。

The Android artifact is byte-identical to increment80 because generated acknowledgement types changed without changing the RN runtime. Fresh Metro/Gradle invocations and current-source binding are recorded; the prior build report was not relabeled. The daily API was not restarted: its read-only check proves process/schema health, not deployment of the new ACK. New ACK behavior was exercised against an API built from current code in the isolated fixture.

## 实际行为 / Actual behavior

1. 当前来源新修订从unknown开始；Windows CLI产生两条准确采样后恢复正常。重启CLI读取同一采样，返回NO_NEW_SAMPLE，事实／序号不增长。
2. 三个实际失败观测触发down，两次成功恢复operational；Web打开原始签名材料并实际验证摘要／签名。
3. Prometheus up=0时实际CLI退出75，不新增健康或故障事实。后端真实时间过期测试核对旧证据转unknown；未用可视化假计时替代。
4. 真正数据库入库后刻意丢失完整回执，重开journal精确重发同一正文／签名／序号，实际API重放返回同一事实；来源轮换后拒绝旧修订。
5. Taro H5只读员工看同一故障及原始签名事实，没有来源／服务写入控件。Web浅／深色390/768/1440与移动320/390/768布局共九项，无横向溢出。

The final full regression also covers complete journal corruption, partial-tail repair, exclusive OS locks and compaction; malformed/future/stale/multiple/rewritten metrics; wrong or incomplete ACKs; controlled retries; DNS/IP/redirect/proxy boundaries; and separation of Prometheus read tokens from observation delivery. A controlled metrics endpoint supplies synthetic samples. No actual Blackbox target, cloud service, TG message or model was invoked.

## 失败历史 / Preserved failed attempts

- [focused-01](attempts/focused-01/README.md)：真实Windows O_APPEND句柄不能截断残尾，以及集成检查错误预期201（原API发布为200）；分别修正持锁O_RDWR／Seek和断言。
- [focused-02](attempts/focused-02/README.md)：64条中间结果通过。之后补强Bearer隔离和代理测试，最终以154顶层全量回归绑定当前来源。
- [browser-01](attempts/browser-01/README.md)：两次检查器把数值序号2按字符串比较；本目录准确保留第二次失败报告／来源／脚本。首次未在重跑前冻结，不声称有该原始报告。两次均完成自身资源清理，均不计为通过。
- [临时密钥检查器](attempts/key-checker-01.json)：首次把CLI公钥的纯Base64输出误当JSON；CLI成功。改为可复现的密钥检查脚本后通过，私密临时目录始终由上下文清理。

Failed product/checker attempts are preserved and separated from the final source-bound pass. Intermediate focused results are not substituted for the final regression.

## 媒体与复现 / Media and reproduction

新增三张实际PNG、6.0秒无声MP4和36帧GIF；录制原始时长5.781秒、28个实际JPEG源帧。MP4以10fps编码造成的小幅尾部取整由ffprobe校验。原JPEG不提交；[公开帧清单](recording-frames.json)、[来源](recording-provenance.json)、[编码记录](media.json)和[实际视觉抽查](visual-review.json)绑定准确输出。查看了三张完整视口截图及GIF的0/12/24/35帧，没有声称逐帧审片或全像素OCR。

Three new actual PNGs, a 6.0-second silent MP4 and a 36-frame GIF derive from 28 actual JPEG captures over 5.781 seconds. Authentication/MFA is excluded. MP4 retains recorded intervals with bounded frame-rate tail rounding. Media contains synthetic records and public IDs/digests; raw frames and private fixture files stay ignored. Prior media retains its historical commit/source provenance.

在已准备原生工具／数据库／依赖的仓库根目录，按顺序运行；浏览器复现会产生新随机ID／时间／签名，因此新证据与媒体应独立归档，不覆盖已发布的历史冻结文件：

```powershell
$env:PYTHONUTF8='1'
python app/scripts/build_status_bridge.py
python app/scripts/verify_status_bridge_key.py
python app/scripts/verify_backend_batches.py --batch-size 20 --evidence-group status-bridge
python app/scripts/verify_web.py status-bridge
python app/scripts/verify_mobile.py status-bridge
python app/scripts/build_android.py --google-mirror --evidence-group status-bridge
python app/scripts/inspect_android_apk.py --evidence-group status-bridge
python app/scripts/verify_linux.py status-bridge
python app/scripts/run_status_bridge_browser.py
python app/scripts/freeze_status_bridge_evidence.py
python app/scripts/encode_status_bridge_media.py
python app/scripts/verify_status_bridge_development.py
# Inspect actual rebuilt images/GIF, then update the visual review record.
python app/scripts/verify_status_bridge_sources.py
python app/scripts/verify_repository_publication.py
```

Reproduction creates new random identities, timestamps and signatures. Archive a new evidence set before recreating these commands; never overwrite historical frozen bytes merely to make verification pass. Fixtures share port5189 and a private ownership file, so capture runs must be sequential. All owned browser contexts, temporary servers, signing material and isolated databases are released. No Docker, WSL, VM or emulator is used; unrelated user services/tabs remain untouched.

## 剩余放行 / Remaining acceptance

真实Prometheus／Blackbox与目标许可、主机CPU／内存／磁盘、主应用外独立告警、香港链路、四端真机、120GB／2TB、云／模型／TG、依赖安全和生产部署／恢复保持开放。8项Skill仍draft，真实集成关闭。完整编号与责任见[Goal差异表](../../../delivery/03-goal-gap-assessment.md)。

The current bridge passes the local source-bound scope. Real monitor/target deployment, host metrics, independent alerts, actual integrations/devices, scale, security and production recovery remain separate acceptance gates.
