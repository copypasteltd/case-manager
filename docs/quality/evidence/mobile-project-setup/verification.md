# 增量75验证记录

实现见[移动项目立项、资料与成员管理](../../../development/75-mobile-project-setup-and-members.md)。本轮使用原生Windows、隔离PostgreSQL、真实登录及HTTP命令和实际Taro表单，不使用Docker、WSL、虚拟机或模拟器。

| 范围 | 实际结果 | 证据 |
| --- | --- | --- |
| 移动输入／会话／草稿 | 最终99项通过，0失败／取消／跳过，含8项新项目输入测试、2项共享草稿边界测试 | [移动构建](mobile-build.json) |
| TypeScript及小程序 | 类型检查、微信65个产物／抖音62个产物文件均通过，两端各16个注册页面 | [实际四阶段](mobile-build.json) |
| 真实API／H5 | 7条完整流程通过，真实模板结构、新项目及版本、准确成员与负责人均回读核对 | [浏览器](../mobile-project-setup-browser/result.json)、[完整来源](../mobile-project-setup-browser/source-manifest.json) |
| 页面布局 | 立项／成员两页320／390／768共6项通过；已实际观察390px截图 | [立项截图](../mobile-project-setup-browser/project-creation.png)、[成员截图](../mobile-project-setup-browser/project-members.png) |
| 新Android | 最终开发APK 42,818,596字节，17项包检查通过；准确摘要55198a6020314e5665415919279173d8a14f2876213bfe5aae016d5d34978cc2 | [构建](build.json)、[检查](apk-inspection.json) |
| Android原生测试 | Gradle实际任务为UP-TO-DATE，保留25项通过结果、4份XML摘要，本轮未新执行 | [执行范围](android-test-reuse.json)、[原日志](native-build.log) |
| 独立纯Java | 增量66最终准确来源与4份XML／25项结果未变，未重新执行 | [原证明](../android-upload/jvm-source-proof.json) |
| 后端／运行时／Web | 未改动，完整来源复算一致；准确复用增量74的146顶层／919条结果／832末级场景、39项运行时及81项Web构建证据 | [后端](../deliverable-creation/result.json)、[运行时](../deliverable-creation/runtime-build.json)、[Web](../deliverable-creation/web-build.json) |
| Linux | 后端来源与冻结Linux产物摘要不变，准确复用74编译；本轮未重建、执行或部署 | [原编译](../deliverable-creation/linux-build.json) |
| 日常开发 | 新只读核对10个探针、69项迁移准确摘要、原API与Worker身份，0测试库／活动模型／启用TG | [通用核对](development-runtime.json)、[当前核对](development-project-setup.json) |
| 最终来源与文档 | 22项来源／资源核对、当前文档链接及28个稳定GAP编号 | [来源](final-source-check.json)、[文档](documentation-check.json) |

最终移动来源为`02321b7c81c79aa800aa1227769a7fbb59efdc24e13b7141d95d059699ef3514`；实际H5夹具／构建／浏览器三阶段来源相同，为`6c7a85971cf40767764255f518f3d6cf039ba1b919e14f79964d98e6006d547b`。两种清单覆盖范围不同，不能直接互比。完整后端来源保持`61754e7e60bdac1ad937c5890330de0fa872deaa72d116f428348ef46bdfb10d`。

实际流程包括：非法日历不发送创建；准确发布模板同事务初始化；草稿明确清空起止日期；真实服务端已提交后中断浏览器回执，页面锁住重发并重新读取；授权本人另一个请求制造实际并发409，输入保留；激活后范围／日期控件只读且普通PATCH不含受控字段；实际候选角色变更／移除／重新加入；独立移交后原owner为manager；通过原MFA保护角色接口给新负责人授予准确本项目业务角色，再由新负责人将原owner降为viewer。受影响旧登录返回401，新MFA登录无编辑、成员配置或深链接写表单。没有注入业务状态、改库权限或绕过原命令。

失败尝试保留如下，均已关闭所属浏览器和临时服务、退出原生夹具并清理一次性数据库：

| 尝试 | 发现与处理 | 原记录 |
| --- | --- | --- |
| 01 | 产品错误：本地非法日期的Error被通用读取失败覆盖；改用MobileError显示具体原因，最终仍确认无非法创建请求 | [记录](../mobile-project-setup-browser/attempts/01-validation-message/result.json) |
| 02 | 检查器误把子目录当作根目录查询结果；改为从真实项目根目录查询实际children，业务代码未变 | [记录](../mobile-project-setup-browser/attempts/02-folder-hierarchy/result.json) |
| 03 | 检查器使用无该受限项目资格的系统管理员制造并发，真实API正确404；改用实际授权owner的独立请求 | [记录](../mobile-project-setup-browser/attempts/03-restricted-project-access/result.json) |
| 04 | 检查器将项目成员身份误当业务动作授权，真实API正确404；改由原角色绑定命令授予接任者准确本项目动作，再用接任者变更原owner资格 | [记录](../mobile-project-setup-browser/attempts/04-membership-without-business-role/result.json) |

早期通过的[移动](attempts/01-before-selector-review/mobile-build.json)及[Android](attempts/01-before-selector-review/build.json)构建发生在模板标签及错误提示修正之前，仅保留历史，不充当最终来源证据；当前均已从最终产品源码重新构建。最终检查器使用实际轮盘手势、真实显示文字及实际ID核对，不使用组件状态注入。

4个所属上下文与浏览器均关闭、端口5189释放、私有夹具删除；原PG、API、通知Worker、Vite及已有路径别名保留。新APK仍为arm64-v8a、本地API配置和开发签名，未宣称实体设备执行或正式发布。真实模型、八项Skill发布、云／TG／小程序平台／设备、120GB／2TB、安全／性能与生产部署／恢复等继续按[完整差异表](../../../delivery/03-goal-gap-assessment.md)推进。
