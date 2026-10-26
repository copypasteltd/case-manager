# 增量66验证记录

本轮实现[Android原生分片上传与恢复](../../../development/66-android-resumable-uploads.md)，推进 GAP-17／18；完整四端正式首发与真实120 GB设备验收仍开放。

## 1. 通过范围

| 验证 | 结果及实际边界 |
| --- | --- |
| [原生构建](build.json) | 离线业务包与Android工程编译通过，来源期间不变；APK为42,886,489字节，已按内容摘要单独保存 |
| [APK检查](apk-inspection.json) | 17项通过：准确开发应用ID、arm64-v8a、SDK23—34、禁止系统备份、真实签名、15页、实际原生上传类、私有dataSync服务、必要权限及无开机恢复广播 |
| Android工程单元测试 | 25项通过，其中14项新增上传边界／实际HTTP测试，其余11项既有请求与阅读缓存测试；没有设备执行 |
| 独立JVM | [25项当前来源核对](jvm-source-proof.json)；与Android工程执行的是同一批纯Java场景，不算50项独立用例；不覆盖SAF／SQLite／系统通知的设备行为 |
| [移动业务](mobile-build.json) | 79项测试、类型检查及微信／抖音编译通过；新增13项上传协调场景，签名／认证API依赖为受控替身 |
| [原生Babel转换](native-js.json) | 2项通过，实际执行当前Taro原生转换 |
| [原生页面函数](native-layout.json) | 2项通过，实际转译／执行当前原生基础视图并替换平台宿主，核对cpClass后的安全区域／滚动和普通容器；不是AndroidUI设备测试 |
| [Taro H5真实API](../android-upload-mobile/result.json) | 2项通过，既有本人TG回执与近期MFA对账旅程回归；浏览器、所属服务及测试库已回收；不能替代原生上传UI验收 |

实际HTTP测试包含超过32位的文件偏移、有限小内容PUT、64 KiB读取上界、准确长度／回执、取消、不重定向／不重试，以及供应商错误和签名地址脱敏。120,000,000,000字节场景只验证布局与头尾采样的有界行为，没有传输120 GB。

协调测试覆盖仅缺片补传、未知ack先停止再手动对账、未知创建沿用原键、旧准备期限、准确目标／纪元／布局、错误许可、通知暂停、注销后的迟到回执、最终云端完整性核对、凭据续期及不自动正式提交。

## 2. 制品与来源

APK SHA-256：`6d692bdbb05eba4530c8be83f15b3d89142ad0bddf9ea4981e3b53252323fae9`。

保存的开发APK（仅本机保留，未纳入Git：`app/.local/android/artifacts/android-upload/6d692bdbb05eba4530c8be83f15b3d89142ad0bddf9ea4981e3b53252323fae9/app-debug.apk`）包含本次业务包；其SHA-256为 `4d6f55223f7d47bba84217e378c037e937185ae11f178b439b7e9a07344bc246`。完整原生来源摘要为 `057cb5071df0caf881f550bee6732c67570370981f51e1120f27b681019ba5ac`。详细路径及摘要见构建报告，Gradle可变输出不作为唯一保留产物。

最终来源、各端回归及资源回收以[最终来源检查](final-source-check.json)为准。全部命令运行于原生Windows，没有调用Docker、WSL、虚拟机、模拟器或实体设备，也未连接真实云／Bot／模型。

## 3. 复用验证与未放行项

本轮未修改Web、秘书运行时和后端来源；最终核对分别比对其来源，复用增量65的[Web54项及生产编译](../android-native/web-build.json)、[运行时34项／八包检查](../android-native/runtime-build.json)，以及增量64的[完整后端](../telegram-delivery/result.json)：7批140个顶层测试、846条结果／766个末级场景。没有将来源核对计作新一轮全量执行。

当前依赖锁文件与[增量65审计](../android-native/dependency-audit.json)一致：114项告警，critical 11、high 32、moderate 69、low 2；安全门仍未放行。八项Skill仍draft，真实模型与TG外发没有启用。

GAP-17的核心上传代码已有实现和本地证据；实体设备文件提供商／持久授权／SQLite／通知／后台限制／权限撤销／杀进程／重启、真实120 GB香港网络、OSS＋R2完整源及副本摘要、资源和耗时仍须实际验收。正式签名升级、目标版本兼容、生产部署与整体四端首发继续开放。
