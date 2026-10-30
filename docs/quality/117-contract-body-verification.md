# 增量117核验 / Increment117 verification

[业务方案与接口](../development/117-contract-body-drafting.md) · [全部未完成项](../delivery/03-goal-gap-assessment.md)。

| 编号 | 检查 | 实际结果与边界 |
|---|---|---|
| CB-V01 | 完整后端 | 239顶层／1485条结果，12个串行批次，所有发现用例执行，无跳过。嵌套结果不等于不同业务场景数。 |
| CB-V02 | 新API／PG | 七组覆盖准确完整文件、人类采用与幂等、未核对／错误哈希／近期MFA／原子回滚、原合同变化、执行前依据失效、未绑定历史来源、当前读取权限和停止运行；同时核对任务及事件流的全文、采用历史和撤权拒绝。 |
| CB-V03 | 客户端 | Web303、共享移动399、运行器56；九个共同边界场景在Web与移动重复执行，不算十八个不同场景；运行器新增独立模型工具边界检查。 |
| CB-V04 | 四端构建 | 八项类型／测试／构建命令，微信／抖音各20页；当前开发APK18＋3静态检查，JVM26来自Gradle缓存，四份XML保留。未运行设备、模拟器或平台审核。 |
| CB-V05 | 实际页面 | Web、Taro H5各25组，实际API／PG，不替换响应：选择准确依据、提交、等待／取消、完整文件全文、独立显式采用、原商业事实保留、重放、拒绝重复采用及重新读取历史。 |
| CB-V06 | 正常升级 | 已发布116的真实合同修订旅程先在89执行，再以当前migrator升90；18张原合同／引用／财务／历史AI表保持原字节摘要，新表为空且四项约束就绪；不算当前后端测试。 |
| CB-V07 | 媒体 | 6 PNG、2真实静音录屏及2截图导览GIF，认证前后分开；296份旧媒体原字节保留。 |
| CB-V08 | 开发与关闭 | 仅有归属证明的API和两项worker按当前源码重载，原生开发PG89→90，合同／现金／历史资料不变。最终耐久测试PG关闭且0临时库；早期中止集群已关闭，1个中止合成库离线私有保留，不冒充已清空。 |
| CB-V09 | 接口与Linux | 原路径不变，现334路径／410操作／736模型；两个Linux ELF只交叉构建，不是目标内核／真实模型验收。 |
| CB-V10 | 依赖与范围 | 当前完整依赖扫描0已知公告；只显式声明原已解析@noble/hashes2.4.0，解析包未变化。正式产物安全仍GAP-26；23 active／5 excluded、八项draft Skill和完整目标未完成状态保留。 |

实际页面最初发现事件流漏返正文视图，已修正并补API覆盖，最终全部后端按修正后源码重新运行。此前的已完成批次、中止批次和通过的旧源码升级／Linux构建保持原摘要，不借用为最终通过。原客户端通过记录因移动漏选九项而保留旧390计数，补测试入口后新执行为399。编译、类型、浏览器夹具和升级收集失败也保留原日志，不替代最终验收。

两次早期通过的Web尝试没有复制临时录屏清单和帧，后续捕获已覆盖原字节，无法恢复或重新核验。原源码、截图、结果和日志保留，原归档引用不改写；[归档失败原检查](evidence/contract-body-attempts/recording-retention-v1/original-failed-source-check.json)与[明确缺失说明](evidence/contract-body-attempts/recording-retention-v1/disclosure.json)保持公开。这两份旧录屏不计保存成功，也不计最终证据。最终本轮Web／移动录屏、清单和媒体独立核验；后续归档器已改为复制原清单及全部哈希核对后的帧。

| 证据 | 入口 |
|---|---|
| 准确源码 | [最终检查](evidence/contract-body/final-source-check.json)、[私有完整源码归档摘要](evidence/contract-body/source-archive.json) |
| 后端 | [全量](evidence/contract_body-backend/result.json)、[批次](evidence/contract_body-backend/batch-progress.json)、[原日志](evidence/contract_body-backend/raw-log-archive.json) |
| 客户端与页面 | [八项命令](evidence/contract-body/client-checks.json)、[Web](evidence/contract-body-web/result.json)、[Taro H5](evidence/contract-body-mobile/result.json) |
| Android | [构建](evidence/contract-body/build.json)、[18项检查](evidence/contract-body/apk-inspection.json)、[正文内容](evidence/contract-body/android-feature-content.json)、[JVM范围](evidence/contract-body/android-test-scope.json) |
| 升级与开发 | [实际89→90](evidence/contract-body/schema-upgrade.json)、[服务重载](evidence/contract-body/development-update.json)、[只读检查](evidence/contract-body/development-project-resources.json) |
| 媒体与依赖 | [捕获与编码](evidence/contract-body/media.json)、[目视核对](evidence/contract-body/visual-review.json)、[旧媒体](evidence/contract-body/reused-evidence.json)、[当前扫描](evidence/contract-body/dependency-audit.json) |
| Linux与资源 | [交叉构建](evidence/contract-body/linux-build.json)、[最终PG关闭](evidence/contract-body/native-cluster-closed.json) |
| 初期尝试 | [日志摘要](evidence/contract-body/preliminary-checks.json)、[原类型失败](evidence/contract-body-attempts/client-types-v1.json)、[移动入口修正前](evidence/contract-body-attempts/before-test-selection/preservation.json) |
| 浏览器与旧版升级 | [夹具参数](evidence/contract-body-attempts/01-fixture-flags/preservation.json)、[事件流缺项](evidence/contract-body-attempts/02-event-log-contract-body/preservation.json)、[升级收集](evidence/contract-body-attempts/upgrade-v1/preservation.json)、[修正前后端](evidence/contract-body-attempts/before-event-log-fix/preservation.json) |

These checks establish the bounded complete-text drafting and explicit human adoption workflow. Prior, interrupted and cached evidence retains its actual execution scope. Two earlier Web recordings were lost through temporary-path reuse; the original failed retention check is disclosed and these recordings receive no preservation or final-pass credit. Final current media are independently verified. Live model/Skills, cloud, formal platform artifacts and production acceptance remain open.
