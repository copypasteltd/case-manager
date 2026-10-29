# Increment112 evidence / 增量112证据

本目录记录准确草稿恢复的本轮实际检查与明确复用。总体说明见[验证文档](../../112-secretary-file-recovery-verification.md)，设计见[开发文档](../../../development/112-secretary-file-recovery.md)。

| 记录 | 内容 |
|---|---|
| client-checks.json及三份build摘要 | 八项实际命令、当前源码摘要、准确私有日志散列；摘要适配不重复执行测试。 |
| ../file-recovery-backend | 全部发现Go测试、每批结果及准确原始日志保全；真实原生PG。 |
| ../file-recovery-web、../file-recovery-mobile | 实际页面/API、完整捕获源码清单、截图和资源释放；移动使用Taro H5。 |
| build.json、apk-inspection.json、android-feature-content.json | 新开发APK、当前包与业务内容的静态证据；不执行设备。 |
| android-test-scope.json、android-unit-results | 26条JVM原始XML、Gradle任务状态及缓存／新执行范围。 |
| media.json、两份capture-manifest、visual-review.json | 实际认证页面帧、静音编码及目视核对；无密码或MFA录制。 |
| native-cluster.json、native-cluster-closed.json | 独立全耐久本机原生PG，未改原开发集群，结束后0测试库并关闭。 |
| development-update.json、development-project-resources.json | 普通schema86→87升级、历史费用数据保留、归属核对后的API／Worker替换、只读检查。 |
| linux-build.json | 两个Linux ELF交叉构建，不执行／部署Linux。 |
| reused-evidence.json | 原244份媒体及依赖证据按准确已发布字节保留；没有重新审计依赖。 |
| attempts | 失败／失效尝试独立保留；准确源码和原始日志在忽略目录，由公开摘要绑定。 |
| process-restoration.json、development-service-restoration.json | 已确认进程退出后的原生PG与3项自有服务恢复；不重新初始化、不重跑结构升级；中断片段不记为通过。 |
| final-source-check.json、scope-check.json、repository-check.json | 当前源码、范围与最终发布候选的分层核对。 |

凭据、连接串、私有原始日志、数据库、源码试跑ZIP、原始帧和APK不入Git。所有示例均为隔离合成数据。生产就绪、真实模型／云／TG以及正式发布继续开放，当前23项待完成、5项移出不变。
