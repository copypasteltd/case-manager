# 增量90验证 / Increment90 verification

本轮验证依赖修复、四端构建兼容及现有实际财务流程，完整平台Goal仍未完成。源码与机器记录绑定如下；本地／合成检查不替代设备、真实云、模型、TG、容量、安全和生产验收。

This increment checks dependency changes, affected builds and actual existing finance workflows. The full platform goal remains active. Native local/synthetic evidence does not establish device, cloud, model, TG, capacity, security or production acceptance.

| 范围 / Area | 实际结果 / Actual result | 执行边界 / Scope |
| --- | --- | --- |
| 完整npm审计 / Complete audit | 旧锁101 → 新锁86；critical11 → 0，high30 → 20 | 64 moderate、2 low保留；所有依赖范围，生产安全门未通过 |
| 安全回归 / Security regression | 12／12通过 | 真实有界归档、路径／链接、原型及序列化；不运行资源炸弹 |
| 原生JS / Native JS | 2／2通过 | 现有Babel兼容行为 |
| Web | 112项及构建通过 | 新锁后实际重新执行 |
| 秘书运行时 / Runtime | 47项及构建通过 | 新锁后重新执行；八项Skill仍draft |
| 移动 / Mobile | 204项、类型与两小程序各18页 | 实际重新执行；小程序开发工具／设备未调用 |
| Android | 42,921,174字节，18项包检查 | 原生bundle与Gradle构建调用完成；APK与89字节相同 |
| JVM | 26项，`> Task :app:testDebugUnitTest UP-TO-DATE` | 缓存复用，无新增JVM执行、无真机 |
| 实际页面 / Actual UI | 9组流程、390／430／768px | 真原生API、独立合成库、共享Taro H5；实际遵守MFA限流等待 |
| 媒体 / Media | 新4PNG／2MP4／2GIF；总62／23／23 | 身份验证结束后录制；实际像素检查四图和两GIF的首／中／末帧 |
| 后端 / Backend | 原88：161顶层／1057结果／960末级，9批 | 完整来源未变，只复用原路径；本轮不宣称重跑全量 |
| Linux | 原88编译记录 | 完整后端摘要相同，只编译，未运行／部署 |
| 开发环境 / Development | 73迁移、3个自有服务、零隔离测试库 | 原开发进程已退出后恢复；云／模型／TG均关闭 |
| 历史 / History | 已发布89完整启动／捕获来源从Git重建；103份证据／媒体原字节保留 | 包含8份原89媒体，不重绑为新版本 |

当前锁SHA-256：`66bf226646cc7a8159214db65a6f46ff8a516e194e52c588ba475c63ed50434c`。

Android APK SHA-256：`7e2bc06a0ba7e24e0dd57bec782520ef0091d6eee3dda32e6098f9bb047ed06a`。仅为忽略目录中的开发产物，未将APK、签名私钥、工具、缓存、数据库或私有配置提交。

浏览器完整启动来源：`19ed7c438f42815262e9cb07ef074b53b30cc58bda5d3d8b9aad12d37970dd3a`；实际捕获范围：`5f64134ea0c783c56ab1a04494da05d8122b12afdf5fc92ef7fca1634d13d28a`。两者按各自完整清单计算，不混用；前后来源不变，9项检查均通过。

本轮具体完成：费用100的项目60／公司40；真实409及已提交响应丢失的重复提交阻止；本人MFA和独立采购关联；付款100分配60再释放20；退款10分两次准确映射后净结算30；员工报销100结算60／待付40，真实资金冲销后恢复0／100；释放余额并按独立日期冲销费用；固定修订、不可变历史及3种宽度。准确结果见[业务回执](../dependency-hardening-mobile/business-proof.json)。

[完整审计](dependency-audit.json) · [剩余全部项](remaining-findings.json) · [安全检查](security-checks.json) · [选择节点](selected-pins.json) · [Web](web-build.json) · [运行时](runtime-build.json) · [移动](mobile-build.json) · [Android](build.json) · [包检查](apk-inspection.json) · [JVM缓存范围](android-test-scope.json) · [页面结果](../dependency-hardening-mobile/result.json) · [媒体](media.json) · [像素核对](visual-review.json) · [最终来源](final-source-check.json) · [发布检查](repository-check.json)

## 失败与限制 / Retained failures and limits

1. [resolution-01](attempts/resolution-01/diagnosis.json)：webpack5.112.2移动了内部EntryDependency，Taro编译失败；npm首次安装也保留过三个无效覆盖版本。换用兼容5.104.1并核对真实依赖树后重验。原报告／日志和其他来源保留；旧锁在归档前因工作目录错误被更新，只保留旧摘要，**不宣称旧锁原字节完整可重现**。
2. [startup-01](attempts/startup-01/attempt.json)：原生PG未运行，隔离fixture未就绪；未打开浏览器，保存完整来源ZIP。确认原开发进程已退出后恢复既有自有服务，没有停止无关进程或重新初始化已有数据库。
3. [browser-01](attempts/browser-01/attempt.json)：完成5组流程后命中既有每用户每15分钟8次MFA限流；完整来源、清单与失败结果保留，原JPEG仅在忽略目录。检查器等待真实窗口后仅重试身份验证，不改变应用限流、数据库计数或时钟，最终9组通过。

The first attempt lacks its original complete lock bytes; that limitation is disclosed, not reconstructed as a pass. Later failures retain complete source archives and cleanup records. Final evidence binds a separately complete current lock. All original89 evidence/media bytes remain unchanged.

另保留[发布编码修正](attempts/publication-01/diagnosis.json)：来源检查通过后，JSON写入BOM导致发布读取失败；修正为UTF-8无BOM并重新核对，原结果和检查器字节未丢弃。该记录不等同业务功能失败。

首次[发布链接检查](attempts/publication-links-01/diagnosis.json)在枚举候选时尚无自引用的repository-check.json，生成该记录后重新核对。保留初始失败报告；应用来源与业务检查未变。

## 清理与后续 / Cleanup and remaining acceptance

浏览器已关闭1个自有上下文，退出浏览器；临时API／Go测试正常退出，独立数据库清理通过，5189服务已停、私有fixture已移除。日常开发PG／API／通知worker／只读派生报表worker按归属保留，真实云、模型和TG仍关闭，全部8项Skill为draft。未调用Docker、WSL、虚拟机、模拟器或真实设备。

Current owned temporary resources are closed. Normal owned native development services are separate from the temporary fixture. All integrations remain disabled and eight Skills remain draft. No virtualization, emulator or device was invoked.

GAP-26保持P0：86项剩余告警须继续按真实调用范围处置；0 critical不能视为生产安全通过。完整28项差异、四端同批首发、120GB／2TB、真实模型／TG／双云、备份恢复和生产放行均继续推进。仓库更新须同步中英README、相关协议、CHANGELOG和受影响媒体后提交推送。

[修复与复现方案](../../../development/90-dependency-hardening.md) · [原88后端证据](../finance-reports/result.json) · [原88 Linux](../finance-reports/linux-build.json) · [89历史来源核对](historical-89.json) · [发布前原文](publication-before.json)
