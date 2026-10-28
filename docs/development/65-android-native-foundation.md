# Android 原生宿主与业务接入（增量65）

当前来源（增量95）/ Current source: 共享包保持20页，任务详情新增讨论、固定版本附件、明确成员提及及准确通知定位；原生上传继续保留。当前开发APK：42,940,937 bytes，SHA-256 `30eb371f3f1c7dc0cfd47b712cd879325bce96de04f2984e4394d14e8bc8a97d`；18项包检查通过，26项JVM明确为Gradle UP-TO-DATE缓存而非新执行。源码摘要 `85b2eec0d19a1525dbd16f1c78859179bfeafbcff1ea8c86e457ef2c81f27dce`。实际包位于忽略的本机产物目录，不提交APK／签名密钥。见[本轮方案](95-task-discussion-and-mentions.md)、[构建来源](../quality/evidence/task-mentions/build.json)、[检查](../quality/evidence/task-mentions/apk-inspection.json)与[缓存范围](../quality/evidence/task-mentions/android-test-scope.json)。No real-device, signing, store, background or120GB acceptance is claimed. 下文各增量说明保留其历史范围。

当前来源补充（增量94）/ Current-source addendum: 原生宿主已包含后续上传与当前20页共享业务包，新增手工目录／文件整理、回收恢复和正式版本列表；下文14页与“尚无上传”是65阶段历史。The original14-page/no-upload baseline below is historical. 当前原生开发APK与18项包检查、26项JVM的实际Gradle缓存范围见[文件方案](94-mobile-file-workspace.md)、[包证据](../quality/evidence/mobile-files/build.json)及[缓存范围](../quality/evidence/mobile-files/android-test-scope.json)。未作真机、正式签名或分发验收。

本文记录增量65交付时的实现与限制。后续原生上传代码及当前验收边界见[增量66](66-android-resumable-uploads.md)，当前整体状态以[差异表](../delivery/03-goal-gap-assessment.md)为准。

本增量推进 GAP-16／18：建立 Taro／React Native Android 工程，接入现有移动业务、原生请求、私有临时阅读文件和本地签名构建。完整四端正式首发仍按原 Goal 验收，Android 120 GB 原生上传属于 GAP-17，尚未实现。

## 当前补充：增量92

当前原生工程包含19个业务页面和增量66的Android分片上传；下文14页及“尚未实现”保留为增量65历史。增量92增加统一搜索和固定版本正文预览，独立RN样式／原生导航继续保留。当前开发APK、18项包检查及26项JVM实际Gradle缓存范围见[增量92验证](../quality/evidence/file-search/verification.md)。未启动模拟器；真机、大文件、后台／断点恢复、签名与分发仍须独立验收。APK、签名私钥和工具链不提交。

Current source has19 registered pages, native uploads and unified search/text previews. The14-page descriptions below are historical increment65. Consult increment92 evidence for the current development APK, package inspection and exact JVM cache scope; no device or distribution acceptance is claimed.

## 1. 已实现的工程边界

| 编号 | 实现 | 必须保持的约束 |
| --- | --- | --- |
| AND-01 | Taro 4.3.1、React Native 0.73.11、React 18.2.0；Android Java 宿主 | 现有14个注册页面共用业务协议；使用原生视图与导航；不以 H5 充当 Android 安装包 |
| AND-02 | Metro 工作区解析、共享接口包 `exports`、`.rn` 文件及 Babel 原生控件转换 | JS 使用真实工作区路径，原生编译使用项目已有 ASCII 路径映射；动态样式类和表单行为必须保留 |
| AND-03 | `CPHTTP` 与 `MobileClient` | 设备身份为 `android`；沿用认证版本、幂等键、当前用户范围和未知写入结果语义；访问令牌仅留内存 |
| AND-04 | 固定 API origin 的 OkHttp JSON 请求 | 仅允许 `/v1/` 下 GET／POST／PATCH；拒绝路径穿越、任意 URL、重定向和连接失败自动重试 |
| AND-05 | 有界请求与取消 | 请求1 MiB、响应2 MiB、16个在途请求、20秒总截止时间；严格 UTF-8；取消只作用于对应请求；错误不携带凭据或原始服务响应 |
| AND-06 | 原生私有阅读缓存 | 仅应用缓存目录 `cp-read/`，随机句柄、格式白名单、最多4个32 MiB文件；每次桥接不超过64 KiB；只能读取已完成的本进程句柄 |
| AND-07 | 固定版本文本、图片与系统文档查看 | 沿用既有内容格式、版本条件及完整性校验；文本最多1 MiB；系统查看器仅获一个文件的临时只读授权，返回后撤销；显式用户操作才打开查看器 |
| AND-08 | 临时数据与屏幕 | 启动清扫失败阻止新内容阅读；退出阅读清理缓存；禁止系统备份；宿主窗口设置 `FLAG_SECURE`。外部查看器可能保留用户主动打开的内容，不能由宿主保证撤回其副本 |
| AND-09 | Android 主题与表单 | 使用共享色彩令牌、系统／明暗偏好、安全区域、输入与选择器、复选、加载和禁用状态、状态播报；AsyncStorage仅保存外观偏好 |
| AND-10 | 可核验离线产物 | debug APK 内嵌业务包，关闭开发服务器支持；arm64-v8a；签名密钥只在忽略的本地构建缓存；校验 APK 内业务包与本次产物一致 |

源码入口：[原生工程](../../app/mobile/android/settings.gradle)、[原生宿主](../../app/mobile/android/app/src/main/java/hk/copypaste/workspace/MainApplication.java)、[请求边界](../../app/mobile/android/app/src/main/java/hk/copypaste/workspace/ApiHTTP.java)、[缓存边界](../../app/mobile/android/app/src/main/java/hk/copypaste/workspace/ReadCache.java)、[移动端适配](../../app/mobile/src/platform/taro.rn.ts)、[原生打包](../../app/mobile/native/build-bundle.cjs)。

## 2. 原生模块与平台一致性

当前离线业务图使用手势、页面、安全区域、设备信息和外观存储等原生模块。没有使用 Expo、WebView、地图、定位、相机等扩展；宿主按实际依赖链接。打包检查在依赖图出现 Expo 扩展时失败，新增相关 API 必须同时完成宿主链接、权限及设备验收，不能只增加 JS 调用。

原生基础控件在 Babel 的 RN 分支转换，共享页面保留原始 Taro 组件接口。请求、主题、应用根和内容查看器使用 `.rn` 适配，微信／抖音保持原客户端实现。H5 仅作为本地浏览器回归环境。

React 及 ReactDOM 在工作区统一为18.2.0，避免原生端和 Web 各装一份 React。Taro 的样式检查依赖固定在其兼容版本。完整依赖审计仍是 GAP-26 的放行条件，不能据编译成功判断依赖安全。

当前使用的版本组合来自 [Taro RN 说明](https://nervjs.github.io/taro-docs/en/docs/react-native/)及[官方原生壳工程](https://github.com/NervJS/taro-native-shell/tree/0.73.0)的兼容路径；本项目宿主按实际业务依赖缩减。Android／React Native 的版本升级、目标 SDK 和不同设备的兼容性需要另行验证。

## 3. Windows 原生构建

要求原生 Node 22、Python 3.11+、Android SDK platform／build-tools 34及已接受的 SDK 许可。构建脚本使用固定 JDK17；不启动 Docker、WSL、虚拟机、模拟器或设备。已有 SDK 只读使用，工具和 Gradle 缓存保存在本项目。

从仓库根目录运行 PowerShell：

```powershell
$env:PYTHONUTF8='1'
python app/scripts/setup_android_native.py
npm.cmd --prefix app ci --ignore-scripts
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
$env:CP_MOBILE_BUILD_PROFILE='local'
python app/scripts/build_android.py
```

JDK 下载核对固定发布方 SHA-256，已有安装不覆盖，检查其可执行文件与已验证压缩包是否一致。断网复核使用 `setup_android_native.py --offline`。如 Google Maven 在构建机器上不可达，可显式增加 `build_android.py --google-mirror`；仅当前 Gradle 命令的 Google Maven 仓库切换为阿里云镜像，TLS保留，不改机器代理或全局 Gradle 设置。

`CP_ANDROID_SDK_ROOT` 可指定已有 SDK。`CP_MOBILE_API_ORIGIN` 必须明确提供；正常配置只接受 HTTPS，`local` 只允许 `localhost`／`127.0.0.1` 的 HTTP。设备的回环地址属于设备本身，实体设备访问开发机需要显式 ADB reverse 或受信任的 HTTPS 地址；本增量没有自动连接或安装任何设备。

| 输出 | 路径／说明 |
| --- | --- |
| APK | `app/mobile/android/app/build/outputs/apk/debug/app-debug.apk`；开发调试签名，应用ID `hk.copypaste.workspace.debug` |
| JS包 | `app/mobile/android/app/src/main/assets/index.android.bundle`；生成文件，不入库 |
| 本地工具与过程日志 | `app/.tools/`、`app/.local/android/`；不入库；签名密钥和源映射不得作为用户文档公开 |
| 验证证据 | [验证记录](../quality/evidence/android-native/verification.md)及同目录结构化报告 |

脚本对原生来源做前后摘要，并核对 APK 内嵌业务包。非零退出、构建时修改来源、未产出对应文件都不能作为通过。Gradle 自身的增量缓存可以复用；不能把旧 APK 的存在当成本次构建成功。

## 4. 验证与验收

纯 Java 边界测试可独立运行，不需要 SDK、设备或 C++ 工具链；其来源就是宿主使用的 `ApiHTTP`／`ReadCache`。项目中使用已固定的 JDK 和 Gradle wrapper，从 Android 目录执行：

```powershell
.\gradlew.bat --no-daemon -p ..\native\jvm-tests test
```

执行前将 `JAVA_HOME` 指向 `app/.tools/jdk17/` 下已安装的 JDK17，并将 `GRADLE_USER_HOME` 指向 `app/.local/android/gradle-home/`。完整构建还运行 Android 工程的 `testDebugUnitTest`。JS侧使用 `npm.cmd --prefix app run test:native-js --workspace @copypaste/mobile`，验证实际 Babel 转换；现有 MobileClient 测试覆盖设备身份、版本条件、取消和写入未知状态。

| 验收编号 | 本阶段要求 | 正式首发的额外要求 |
| --- | --- | --- |
| AND-AC-01 | 原生工程编译、边界测试、APK签名与内容检查 | 约定实体设备冷启动、登录／MFA、撤权、退出和任务回看 |
| AND-AC-02 | 现有14页进入离线包、平台模块选择正确 | 逐端页面矩阵、后台切换、返回键、系统字体、输入法与深浅主题实测 |
| AND-AC-03 | 有界固定版本缓存、文档授权回收 | 实体设备撤权／杀进程、低磁盘、无查看器、不同文档查看器与缓存清理实测 |
| AND-AC-04 | 其他客户端测试及编译回归 | 微信／抖音真机、主体／域名／审核，以及正式 Android 签名和升级链 |
| AND-AC-05 | 构建来源、依赖及 APK 摘要可追踪 | 依赖安全处置、正式服务地址、部署环境、目标 Android 版本与原生库兼容性放行 |

开发 APK 不属于正式发布。当前没有 Android 原生上传、持久文件 URI、后台分片、杀进程恢复、120 GB 设备验证或正式签名升级。该部分继续按 GAP-17 推进，首发范围不减少。真实服务器、云存储与模型／TG等整体验收也不由 Android 编译代替。
