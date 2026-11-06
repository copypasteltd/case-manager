# GAP-16：Android 源码独立核对（168）

基线 `930cdde97b9d5d2e85d892664b0857a10a811739`；读取当前工作树。仅静态阅读，未运行测试、构建、Gradle、Android JS、浏览器、服务或设备；未改应用源码。

**要求与判定。** `docs/delivery/03-goal-gap-assessment.md:190` 要求“补正式签名、版本升级及发布配置；核对目标SDK／原生库兼容、原生桥接、构建和APK内容”。`docs/development/06-clients-and-pages.md:79` 要求原生大文件、暂停／重试及授权修复；该历史段落中的真机要求已由现行差异表 `:205` 移出。本次未确认 GAP-16 范围内新增可直接修复的代码差异；不据此关闭正式包验收。

| 源码链 | 核对结果与定位 |
| --- | --- |
| Taro／RN 宿主及共享业务 | `app/mobile/index.js:1` 导入 RN／Taro 入口；`android/settings.gradle:4` 自动链接，`MainApplication.java:16` 禁开发服务器、`:18` 合并 PackageList 与 CPPlatformPackage；`CPPlatformPackage.java:12` 注册请求、阅读、上传、剪贴板。以上 Java 路径均位于 `app/mobile/android/app/src/main/java/hk/copypaste/workspace/`。`app/mobile/src/app.config.ts:2` 注册21页，包括业务、秘书、文件、财务、上传；共享168任务看板另归 GAP-18。 |
| 原生文件与恢复 | 同目录 `CPUploadModule.java:109` 取得持久 SAF 授权并完整摘要复选核对；`UploadSource.java:23` 64KiB流式 SHA256；`UploadTransfer.java:33` 有界传输；`UploadStore.java:29` 持久 owner 元数据；`UploadManager.java:43` generation隔离、`:49` 登出暂停及撤权、`:74` 开始前复核原件；`UploadServiceHost.java:44` 前台服务。`app/mobile/src/platform/upload-transfer.ts:83` 拒旧抽样身份并核对服务端会话；`uploads.rn.ts:60` 明确通知授权，`pages/uploads/view.rn.tsx:63` 提供复选修复。 |
| 阅读／Office／会话消费者 | `app/mobile/src/platform/taro.rn.ts:5` 登出清原生范围；`content-sink.rn.ts:27` 先清私有缓存；`office-upload.rn.ts:9` 固定 epoch 与认证，再导入有界、摘要核对的 Office 产物。Manifest `:17` 禁备份，`:21` 私有 FileProvider，`:22` 非导出 dataSync；阅读临时授权经 CPReadCacheModule `:65`。 |
| 签名／版本／源码绑定 | `app/mobile/android/app/build.gradle:20` 消费仓库外私有配置，`:27` 强制递增版本，`:48` 正式签名，`:96` 要求 HTTPS。`app/scripts/android_release.py:36` 验配置、`:71` 独立验签名／版本／包；`build_android.py:21` 枚举源码，`:124` 前后摘要比较，`:132` 比对包内当前 bundle，`:158` 输出来源。 |

**检索与边界。** 先 `rg --files` 定位，再检索 `GAP-16|Android|120GB`、`scope|generation|fingerprint|foreground`、`signing|release|source_sha256`；读上述生产者／消费者。一次猜测的 capability文档／build_android_package.py 路径不存在，未作为证据；通配参数报错后读取实际 index.js。未逐行重审所有共享业务、依赖、APK或真实签名身份。`docs/development/164-frozen-release-delivery.md:3` 的 APK164 固定业务163／结构129；新 Android JS／APK延期、正式凭据与平台验收待提供，均非“缺代码”。真机与网络性能专项不重新设门槛。
