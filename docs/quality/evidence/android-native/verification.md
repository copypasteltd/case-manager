# 增量65验证记录

本目录对应[Android原生宿主与业务接入](../../../development/65-android-native-foundation.md)。实体设备、正式签名升级、120 GB 上传和生产部署尚未验收；完整 Goal 保持执行。

## 1. 已通过的回归

| 范围 | 结果／证据 |
| --- | --- |
| Web | [54项测试及生产编译](web-build.json)，来源在运行期间保持一致 |
| 移动公共业务 | [66项测试、类型检查、微信／抖音编译](mobile-build.json)，14个注册页面；新增4项Android请求适配测试 |
| 移动端真实 API 浏览器回归 | [2项通过](../android-native-mobile/result.json)：登录／MFA、原TG请求及未知回执、近期MFA人工对账；浏览器和所属临时服务已关闭，测试库已释放 |
| 原生 JS 转换 | [实际 Taro／RN Babel 转换测试2项](native-js.json)通过；动态样式类、原生输入等平台组件转换与第三方模块边界有效 |
| 独立 JVM 边界 | 11项通过：准确 origin／路径、版本条件、重定向隔离、有界严格响应、取消／不重试、请求约束，以及私有缓存范围、容量、完成／清理、句柄及重启边界 |
| 秘书运行时 | [运行时回归](runtime-build.json)，记录实际测试、构建和8项draft Skill检查；未启用真实模型或渠道 |
| 完整后端 | 复核当前摘要仍为 `cd059e07b85c2446a6d48013ac8d5b28f30086bc7739805f4b10bcf3aa6ef8cb`，与[增量64完整测试](../telegram-delivery/result.json)一致；复用其7批140个顶层测试、846条结果／766个末级场景证据，不另算一次全量执行 |

## 2. 原生构建核验

[最终构建](build.json)与[APK检查](apk-inspection.json)均已通过：实际运行 Android 工程的11项单元测试，生成42,187,943字节的该次开发 APK（仅本机保留，未纳入Git：`app/.local/android/artifacts/android-native/fe4d85dfc0b6b948/app-debug.apk`）。14项包检查确认开发应用ID、arm64-v8a、SDK范围、禁止系统备份、实际签名、原生引擎及14页业务包；内嵌业务包与本次离线产物摘要一致。仅保留网络权限及 AndroidX 签名保护的应用私有接收权限。增量66构建前已[按原摘要保留该次制品](artifact-preservation.json)，未修改原构建报告。

APK SHA-256：`4cb75d6bd7de93a2601c720d5e6e75ac9a77d860592081d81f584817a0aee8ab`。原生来源摘要为 `fe4d85dfc0b6b94859f689567bcbd31ddef6db5828db8a188efab147aefb86c3`，构建期间未变化。[最终来源核对](final-source-check.json)12项全部通过，将实际 APK、多端回归、后端复用与浏览器资源回收绑定到当前来源。未运行实体设备，也未作正式发布。

构建过程只使用原生 Windows 进程。固定 JDK17 的发布方校验值及现有安装可执行文件比对已通过；SDK自动下载关闭，现有SDK仅用于读取 platform／build-tools 34。当前宿主不需要本机 C++ 编译器、NDK、虚拟机或模拟器。

首轮 H5 业务场景已通过，但旧来源收集器把新生成的 Android 文件计入摘要，造成来源变化判定失败；记录保存在[首次尝试](../android-native-mobile/attempts/01-generated-native-files/result.json)。修复收集范围后重跑，最终2项通过。原[增量64移动链接](../telegram-delivery-mobile/result.json)更新为同一轮成功记录，并标明其规范证据位置；没有按两轮成功重复计数。

初次包检查发现依赖带入悬浮窗／安装归因权限；移除相关权限并排除安装归因库后重新构建，最终14项检查通过。先前失败记录保留在[独立尝试目录](attempts/02-extra-native-permissions/apk-inspection.json)，不作为通过证据。

## 3. 尚需完成的首发验证

[依赖审计](dependency-audit.json)按最终锁文件重新执行，记录114项告警，其中critical 11、high 32、moderate 69、low 2，GAP-26生产安全门仍未通过。开发 APK 与正式发布分开；设备冷启动、权限撤销、外部文档查看器、后台／杀进程、正式签名升级、Android 版本兼容、真实服务器，以及 GAP-17 的120 GB原生上传仍需后续实现与实测。
