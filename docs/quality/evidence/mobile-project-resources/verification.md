# 增量76验证记录

实现见[移动里程碑、风险与问题](../../../development/76-mobile-project-milestones-risks-and-issues.md)。本轮使用原生Windows、隔离PostgreSQL、真实登录、实际HTTP命令与Taro表单，不调用Docker、WSL、虚拟机或模拟器。

| 范围 | 实际结果 | 证据 |
| --- | --- | --- |
| 移动输入／会话／草稿 | 最终114项通过，0失败／取消／跳过；新增13项事项输入／回执／状态测试、2项共享草稿测试 | [移动构建](mobile-build.json) |
| TypeScript及小程序 | 类型检查、微信65个产物／抖音62个产物文件通过；两端各16个注册页面 | [四阶段构建](mobile-build.json) |
| 真实API／H5 | 8条流程通过，实际新记录、版本、状态、关联问题、当前项目范围与资格均核对 | [结果](../mobile-project-resources-browser/result.json)、[完整来源](../mobile-project-resources-browser/source-manifest.json) |
| 页面布局 | 里程碑创建／风险跟进／问题创建，各320／390／768，共9项通过；3张390px截图已实际观察 | [截图观察及摘要](../mobile-project-resources-browser/visual-review.json) |
| 新Android | 最终开发APK 42,825,161字节，17项包检查通过；准确包摘要673c7f6312710a4b20e640ebdc139bb157c5343df0ad3d0d200f21b583958892 | [构建](build.json)、[包检查](apk-inspection.json) |
| Android原生测试 | Gradle实际任务为UP-TO-DATE，保留25项通过结果，冻结当次日志、4份XML及准确摘要；本轮未新执行 | [执行范围](android-test-reuse.json)、[日志](native-build.log)、[4份XML](android-unit-results/) |
| 独立纯Java | 未改动，保留增量66的准确来源与4份XML／25项结果，本轮未新执行 | [原证明](../android-upload/jvm-source-proof.json) |
| 后端／运行时／Web | 本轮未改动，完整来源复算一致；准确复用增量74的146顶层／919条结果／832末级场景、39项运行时和81项Web证据 | [后端](../deliverable-creation/result.json)、[运行时](../deliverable-creation/runtime-build.json)、[Web](../deliverable-creation/web-build.json) |
| Linux | 完整后端来源及准确原产物摘要一致，准确复用增量74；本轮未重建、执行或部署 | [原编译](../deliverable-creation/linux-build.json) |
| 日常开发 | 新10个只读探针、69项迁移摘要、原API及Worker身份通过；0测试库／活动模型／启用TG | [通用核对](development-runtime.json)、[当前核对](development-project-resources.json) |
| 最终来源及文档 | 22项来源与资源核对通过；当前文档链接及28个稳定GAP编号另存准确记录 | [来源](final-source-check.json)、[文档](documentation-check.json) |

移动构建来源为`5bde74fe67c650d7fe086d797263ab19fa9f40e7659241e8feac3fd61043a428`，125个文件，运行期间未变。实际原生夹具／H5编译／浏览器三阶段来源一致，为`48c4021710834a11d011e12198b90f9d8af2148118441564493cf2478b505138`，592个文件；两种清单覆盖范围不同，不直接互比。Android完整来源为`4f646ac76739e4cd6483eaa4e9ed302c0f8e290bc1ee0f6ac3b7bdd886deef0f`，冻结包准确绑定该来源；完整后端来源保持`61754e7e60bdac1ad937c5890330de0fa872deaa72d116f428348ef46bdfb10d`。未改动部分只复用原证明，不计作本轮新执行。

实际流程包括：非法日期不发创建请求；香港时区与六位微秒、新负责人、错过状态及编辑准确保存；缺少关联证据时422且版本不变，经原任务接口创建并完成真实关联任务后才达成，之后重开／取消；风险概率未设置仍显示措施，原接口不能清空的截止时间在本地明确拦截；真实并发409保留输入，重新读取后再明确修改；风险应对／关闭／重开。

风险转问题在原事务成功后中断实际浏览器响应，核对只有一次提交、一个实际问题，旧确认锁住，重新读取后沿真实问题ID处理。问题调查、解决、关闭、重开保留实际解决说明，空解决说明不发送请求。独立问题表单创建／编辑不隐含状态变化。另一实际可读项目的风险不能放进错误项目编辑表单。原业务角色绑定和负责人移交后，将原负责人设为viewer，旧登录401，新MFA登录下三类新建深链接、列表及详情均不提供写入口；已取消项目的实际manager也不能通过三类深链接新建。

外部深链接分别在独立真实登录中验证，避免把Taro的浏览器POP缓存实例当作新页面。共创建9个所属上下文，短检查完成后立即关闭，最终全部关闭；浏览器、原生夹具、一次性数据库与临时服务均已清理，端口5189释放，私有夹具删除。未注入组件或业务状态，未伪造成功响应。

失败尝试保留如下，所属浏览器与临时服务均已关闭、原生夹具退出0并清理测试数据库：

| 尝试 | 发现与处理 | 原记录 |
| --- | --- | --- |
| 01 | 检查器通过URL hash重复进入已缓存的项目页，风险创建按钮未出现；改用实际项目详情及列表按钮 | [记录](../mobile-project-resources-browser/attempts/01-cached-route/result.json) |
| 02 | 产品正确拒绝清空期限，检查器的宽泛选择器同时匹配提示及错误；改为准确错误区域和文字 | [记录](../mobile-project-resources-browser/attempts/02-clear-deadline-selector/result.json) |
| 03 | 六条实际业务链、九项布局通过；错误项目深链接仍显示旧问题编辑缓存，改查独立打开方式 | [记录](../mobile-project-resources-browser/attempts/03-cached-deep-link/result.json) |
| 04 | 完整刷新正确清空内存登录，检查器到达登录页；保留该安全行为，不引入持久令牌 | [记录](../mobile-project-resources-browser/attempts/04-reload-clears-memory-session/result.json) |
| 05 | 通过首页再改hash仍显示旧缓存；最终改为独立真实登录、首次打开目标页面并立即关闭 | [记录](../mobile-project-resources-browser/attempts/05-cached-home-navigation/result.json) |

早期114项通过的[移动构建](attempts/01-before-time-parser-review/mobile-build.json)发生在整秒与微秒解析复核前，仅保留历史；最终已按新源码重新通过四阶段。最终三张截图见[里程碑](../mobile-project-resources-browser/milestone-creation.png)、[风险](../mobile-project-resources-browser/risk-follow-up.png)和[问题](../mobile-project-resources-browser/issue-creation.png)；它们只证明各自可见区域，完整动作由实际流程断言支持。

原PG、API、通知Worker、Vite及既有路径别名保留。所有8项Skill仍draft，未启用真实模型、云或TG。真实平台／设备、120GB／2TB、依赖安全、性能及生产部署／恢复继续按[完整差异表](../../../delivery/03-goal-gap-assessment.md)推进；本地编译及H5不代替正式四端验收。
