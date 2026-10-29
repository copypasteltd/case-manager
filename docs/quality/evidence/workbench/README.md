# 增量114证据 / Increment 114 evidence

本目录记录当前业务工作台的实际验证。Web与共享移动测试各执行同一19个边界用例；没有设备、模拟器、云、模型、TG或生产验收声明。

| 记录 | 适用范围 |
|---|---|
| final-source-check.json | 当前源码与全部证据绑定；整体goal仍未完成。 |
| client-checks.json、web-build.json、mobile-build.json、runtime-build.json | 八项实际命令、完整输入摘要与私有原始日志摘要。 |
| ../workbench-backend | 最终全量原生后端发现项、每批结果及原始日志保全。 |
| ../workbench-web、../workbench-mobile | 实际登录、三类业务链接、刷新、其他员工与实际撤权；全部自有资源关闭。 |
| build.json、apk-inspection.json、android-feature-content.json | 当前开发APK及静态包／业务内容；没有设备执行。 |
| android-test-scope.json、android-unit-results | 26条JVM的实际任务状态，准确区分缓存和新执行。 |
| media.json、visual-review.json、capture-manifest | 真实认证后像素、静音编码、准确帧与目视采样范围。 |
| native-cluster.json、native-cluster-closed.json | 独立全耐久原生PG，最终0临时库并关闭。 |
| development-update.json、development-project-resources.json | 归属核对后的开发服务替换；schema87及历史费用／用量资料保留。 |
| linux-build.json | 两个当前Linux交叉构建；没有Linux执行或部署。 |
| reused-evidence.json | 266份历史媒体与原依赖检查保留准确字节和日期。 |
| attempts | 初始化、Web类型包含修正、失效回归、移动刷新定位、测试前环境超时及补录导航等待失败；初版录屏替代记录单独保存原通过与来源核对，不计当前通过。 |

私有日志、连接凭据、数据库、工具、APK、原始帧和源码试跑归档不入Git。[设计](../../../development/114-business-workbench.md) · [核验](../../114-business-workbench-verification.md)。
