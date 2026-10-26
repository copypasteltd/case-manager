# Android 原生分片上传与恢复（增量66）

本增量推进 GAP-17／18：在[原生宿主](65-android-native-foundation.md)上接入真实上传协议、SAF持久读取授权、原生流式分片、前台服务与通知、本人恢复队列，以及准确候选的人工提交。代码位于 `app/`。120 GB 真机传输、真实 OSS／R2、跨境长任务和正式发布仍须验收；编译及小内容测试不代表已完成这些验证。

## 1. 实现入口

| 编号 | 实现 | 来源 |
| --- | --- | --- |
| ANU-01 | 系统文件选择与可撤销的持久只读授权 | [SAF读取](../../app/mobile/android/app/src/main/java/hk/copypaste/workspace/SAFUploadSource.java)、[原生桥](../../app/mobile/android/app/src/main/java/hk/copypaste/workspace/CPUploadModule.java) |
| ANU-02 | 私有恢复元数据、准确创建意图及目标固定 | [恢复记录](../../app/mobile/android/app/src/main/java/hk/copypaste/workspace/UploadStore.java)、[任务管理](../../app/mobile/android/app/src/main/java/hk/copypaste/workspace/UploadManager.java) |
| ANU-03 | long偏移与64 KiB原生流式直传 | [分片布局](../../app/mobile/android/app/src/main/java/hk/copypaste/workspace/UploadLayout.java)、[流式传输](../../app/mobile/android/app/src/main/java/hk/copypaste/workspace/UploadTransfer.java) |
| ANU-04 | 临时签名许可与请求头边界 | [许可检查](../../app/mobile/android/app/src/main/java/hk/copypaste/workspace/UploadPermit.java) |
| ANU-05 | dataSync前台服务、进度及暂停 | [服务](../../app/mobile/android/app/src/main/java/hk/copypaste/workspace/UploadServiceHost.java)、[Manifest](../../app/mobile/android/app/src/main/AndroidManifest.xml) |
| ANU-06 | 现有认证续期、对账及上传协议 | [协调器](../../app/mobile/src/platform/upload-transfer.ts)、[Android适配](../../app/mobile/src/platform/uploads.rn.ts) |
| ANU-07 | 上传队列、重选原件、固定基础版、正式提交 | [上传页面](../../app/mobile/src/pages/uploads/view.rn.tsx)、[目录／文件入口](../../app/mobile/src/platform/upload-entry.rn.tsx) |

当前注册页面15个，新增上传页的原生实现；微信／抖音没有大文件入口，访问该路由会说明本端查阅及审批的范围。原生基础视图同时修正了经 Babel 转换后的页面标记判断，页面可以使用已有的安全区域和滚动容器。

## 2. 文件与本机存储约束

系统选择器仅使用 `ACTION_OPEN_DOCUMENT`、`CATEGORY_OPENABLE` 和只读持久授权，不索取全盘、媒体、相机或录音权限。文件必须有明确大小、可随机定位的描述符、可核对的元数据，大小为1至120,000,000,000字节。虚拟文档、管道、未知大小和不能保留授权的来源拒绝接入，不将其整份复制到应用缓存。外部文档提供商自身是否下载或缓存文件由提供商决定，不能由本应用承诺。

私有 SQLite 只保存本人ID、随机句柄、系统URI、文件名称／媒体类型／大小／修改时间、采样指纹、服务端会话ID、固定创建正文／操作标识／准备时间、布局／存储纪元及本机状态。每人最多16份、本机最多64份记录。公开给 JS 的记录没有系统URI。文件字节、访问／刷新令牌、签名URL、许可头和云ETag不持久化。

创建准备固定目标目录、名称及可选原文件／基础版本，不允许暗中改用最新基础版。创建回执丢失时，显式核对沿用原正文和原幂等键，不自动另建会话。未关联服务端ID的准备超过48小时后停止使用，先检查本人服务端队列；需要新上传时由员工明确释放旧本机记录并重新准备。该时间界限小于后端7天幂等记录期限。

`android-saf-v1`采样指纹包含名称、大小、修改时间以及至多头尾各64 KiB；它只用于重选辅助，不是全文件真实性或完整性证明。服务端固定源的完整 SHA-256 与扫描／正式提交协议继续作为权威结果。

启动后记录可以重新列出，但不自动开始上传。退出登录立即停止当前传输、取消文件提供商操作，并在串行元数据任务中移除原用户保存的URI、释放不再引用的持久授权；保留其私有恢复元数据，后续须由本人重新选择原文件。释放本机记录不终止服务端会话，明确终止服务端是另一项操作。

## 3. 上传时序及中断

1. 员工在目录、文件详情或工作台进入队列，核对当前本人身份、目录能力、真实存储接入状态及固定基础版。
2. 通过系统选择器取得原文件；核对名称与目标后保存准确上传准备。
3. 员工在前台明确开始／继续，按系统版本请求上传通知权限，核对持久文件来源后启动私有 dataSync 服务。
4. 使用原创建意图建立或核对会话，或者读取既有本人会话；核对大小、指纹、目标、布局和存储纪元。
5. 先调用后端 `resume`，以真实云端 `ListParts` 更新已确认分片；只为缺片申请许可。
6. 一次仅传一个分片，64 KiB缓冲区，按long偏移打开原文件，不把分片或整文件传入JS。每片完成后通过原API记录有限回执。
7. 传输结束再次检查原文件并对账全部分片，确认字节及布局齐全后调用 `finalize`。校验／扫描／整理仍由现有服务端Worker完成。
8. 只有服务端 `ready` 候选显示完整SHA-256及固定基础版，经员工单独勾选后才能正式提交；不会因上传结束自动创建正式版本。正式文件的主端与副本状态分别回看。

现有 `MobileClient`继续独占访问／刷新凭据和正常续期，所有控制API仍经过现有认证、幂等键及当前权限。原生分片仅使用短期供应商许可；API凭据不送往对象存储。切换页面不卸载协调器。当前采用前台服务保留原生传输，控制步骤仍由同一RN进程的JS协调器推进；JS不再响应60秒、平台终止服务或进程死亡时停止，不宣称是独立于应用进程的后台执行器。

| 中断 | 实际行为 | 继续条件 |
| --- | --- | --- |
| 应用按钮／系统通知暂停 | 关闭本机API作用域、取消当前HTTP及文件提供商操作，服务退出 | 员工重新在前台继续，先核对云端 |
| PUT、ack或finalize结果未知 | 停止，不自动重试不确定写操作；不把本机进度当云端事实 | 重新获取会话并对账，避免补传已存在分片 |
| 令牌到期／正常轮换 | 每片控制请求沿用现有内存续期；失败即停止 | 当前登录仍有效或重新登录；不落盘凭据 |
| 文件授权撤销／原件移动 | 打开文件与传输期间检查持久授权，取消信号和描述符关闭终止读取 | 通过系统选择器重选与原采样相符的文件 |
| 杀进程／重启／系统停止服务 | 没有自动重启或开机广播，恢复记录显示暂停 | 本人重新登录并手动继续，重新读取会话与对账 |
| 服务端撤权、纪元／布局／目标变化 | 控制步骤拒绝；已经签出的许可仍受其短期限约束 | 当前权限、原目标和存储状态重新核对，不能客户端绕过 |
| ready／committed／blocked等服务端状态 | 展示真实状态，不继续发送分片 | 按服务端状态处理；正式提交和终止均需独立员工操作 |

## 4. 网络与服务边界

签名许可只接受HTTPS、443、官方OSS或R2域名后缀、准确分片编号及有效短期时间。允许必要的内容类型／MD5／`x-amz-*`／`x-oss-*`头，拒绝认证头、Cookie、Host、内容长度覆盖、重复头及控制字符；地址、原始云错误和许可头不进入日志／错误正文。

传输禁止重定向及连接失败自动重试，只接受200与有限ETag。每个分片10分钟总截止、15秒连接及60秒读写边界。元数据与分片分别使用有界单线程队列，长分片不会阻塞队列查询；同一应用只允许一项活动上传和一个在途分片。传输事件最多每250毫秒一次，通知最多每秒更新。

通知分别显示传输进度与云端对账进度，不含文件名或业务正文。Android13及以上需用户允许通知后开始；Android14声明 `FOREGROUND_SERVICE`、`FOREGROUND_SERVICE_DATA_SYNC` 及 `dataSync` 类型，服务不导出。当前targetSdk为34。更高目标版本、前台服务时限、厂商省电和长时网络限制需要随发布设备矩阵实测，不保证退出应用后一定传完。

平台依据：[SAF持久文档访问](https://developer.android.com/training/data-storage/shared/documents-files)、[启动前台服务](https://developer.android.com/develop/background-work/services/fgs/launch)、[前台服务类型](https://developer.android.com/develop/background-work/services/fgs/service-types#data-sync)。本次核对日期为2026-10-26；未启动浏览器UI、模拟器或虚拟化环境来读取这些资料。

## 5. 构建与验证

沿用[原生工具链](65-android-native-foundation.md)及真实工作区的 Node 依赖，从仓库根目录执行：

```powershell
$env:PYTHONUTF8='1'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
$env:CP_MOBILE_BUILD_PROFILE='local'
python app/scripts/build_android.py --google-mirror --evidence-group android-upload
python app/scripts/inspect_android_apk.py --evidence-group android-upload
python app/scripts/verify_mobile.py android-upload
```

Google Maven 镜像是当前网络可用时的显式构建选择，不改机器全局设置。正常部署要求HTTPS地址；本地回环构建不是实体设备可直接访问的生产地址。

APK按内容摘要另存到忽略的本地制品目录，避免下一轮Gradle输出覆盖已检查产物；结构化报告保留准确路径、来源与APK摘要。上一轮65的APK已单独[保留](../quality/evidence/android-native/artifact-preservation.json)。本轮实际通过结果以[验证记录](../quality/evidence/android-upload/verification.md)及最终来源报告为准。

## 6. 仍须完成的首发验收

| 编号 | 验收物 | 本次边界 |
| --- | --- | --- |
| ANU-AC-01 | 原生构建、25项Java边界／HTTP测试、移动协调与多端编译、实际包检查 | 测试包含真实本地HTTP与长偏移小内容，没有传输真实120GB |
| ANU-AC-02 | 约定实体设备的SAF／私有SQLite／通知／文件撤权／后台／杀进程／重启 | 本次仅实现代码与编译，设备行为尚待实际验证 |
| ANU-AC-03 | 真实120GB香港网络上传、断网与缺片恢复、内存／临时盘／耗时／源及目标摘要 | 依赖实体设备、真实样本及实际云配置，不能用布局测试代替 |
| ANU-AC-04 | 从选择原件到校验、扫描、正式版本、R2已核对副本的完整旅程 | 本次复用后端已验证协议，真实云／引擎与生产Worker仍须联调 |
| ANU-AC-05 | 正式签名升级、目标SDK／原生库兼容及依赖安全放行 | 开发调试APK不构成正式发布；GAP-16／26／28继续开放 |

GAP-17由“未实现模块”推进为“核心代码已实现、待真机／真实120GB验收”，完整Goal及四端同批首发范围不缩减。
