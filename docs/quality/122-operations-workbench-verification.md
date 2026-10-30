# 增量122核验：事件处置与文件存储待办

本轮为范围明确的功能增量。正式业务代码完成事件待办、文件存储待办及共享移动端人工副本重试；完整工作包仍按原23项登记，不把子功能或移出项记为整体通过。

## 1. 已执行的当前检查

| 编号 | 检查 | 实际结果与范围 |
|---|---|---|
| V122-01 | 原生 Go／PostgreSQL 集成 | 5个选定顶层套件，45条通过结果，0失败、0跳过；包含原首页、服务状态、复制回归和两组新增用例。不是完整后端回归 |
| V122-02 | Web 单元测试 | 330通过，0失败、0待执行 |
| V122-03 | 共享移动单元测试 | 427通过，0失败、0跳过、0取消 |
| V122-04 | 四端共享类型检查 | 当前工作区检查通过 |
| V122-05 | Web／微信／抖音构建 | 三项当前编译通过；小程序使用明确的本地审阅配置，不等于平台审核发布 |
| V122-06 | Native JS | 2项通过；没有构建本轮Android APK，不以此代替原生产物验收 |

原始回执位于[本轮证据目录](evidence/operations-workbench/verification-plan.json)，各项保存实际执行命令、开始／结束时间、输入摘要、结果及私有日志摘要。原始日志留在Git以外，不公开连接信息。

### 源码绑定与测试准备修正

上述8项命令实际执行时的完整输入摘要为 `d4826614460775a38c4ead6829674427493868748762d8f0bf2ee1e384cbcb1e`，执行前后保持一致。随后仅在显式启用的 `TestOperationsWorkbenchBrowserFixture` 中补充一次既有MFA准备调用；生产代码、已执行用例正文和执行命令均未改变。

[原Go源码快照](evidence/operations-workbench/fixture-correction/before-mfa.go.txt)与[原执行计划](evidence/operations-workbench/fixture-correction/before-plan.json)保留实际字节。[检查器](../../app/scripts/verify_scoped_increment.py)仅接受这一条准确差异，重建原执行输入摘要后核对，且要求修改发生在上述可选浏览器准备函数内；不能借此接受其他实现或验收变更。页面检查使用修正后的实际准备代码执行。不把原执行摘要改写为当前摘要。

## 2. 业务验证依据

事件用例核对完整授权计数后最多5条样本、实际当前时间线编号、负责人、所有受影响服务的可读性、跨渠道一致性、角色／项目撤销，以及正常调查、观察、解决后退出首页。

文件用例先让受控目标存储返回实际错误字节，使真实复制Worker失败；通过普通HTTP接口申请重试后，待办仍保留为待验证。随后Worker重新复制并核对完整字节摘要，待办才消除。另核对创建者边界、当前正式版本、完整有效祖先目录、不可用优先级与权限撤销。受控存储不代替真实OSS／R2验收。

共享协议覆盖新分组、允许状态、准确详情路由、空／重复／不一致汇总、重试输入和准确queued回执。人工重试保留原授权、会话、幂等和修订条件，事件修改保留近期MFA。

## 3. 页面、演示与开发服务

| 编号 | 检查 | 结果 |
|---|---|---|
| V122-07 | Web实际页面 | 4组完整操作检查通过；实际API／原生PG，浏览器上下文与临时服务关闭 |
| V122-08 | Taro H5实际页面 | 4组完整操作检查通过；实际API／原生PG，浏览器上下文与临时服务关闭 |
| V122-09 | 演示媒体 | 6份PNG、2份无声MP4、2份GIF；全部查看，摘要一致 |
| V122-10 | 开发服务 | 现有API／通知／报表服务加载当前后端；15组既有业务／历史摘要一致；结构93，无数据库重启或结构升级 |
| V122-11 | 只读运行检查 | 10项探针、结构及权限检查通过；临时测试数据库0；外部云／模型／TG／小程序身份未启用 |

[Web回执](evidence/operations-workbench-web/result.json)、[共享移动页面回执](evidence/operations-workbench-mobile/result.json)、[媒体摘要](evidence/operations-workbench/media.json)、[实际视觉核对](evidence/operations-workbench/visual-review.json)、[开发重载](evidence/operations-workbench/development-update.json)及[只读运行核对](evidence/operations-workbench/development-project-resources.json)分别保留。

两端均使用普通事件编辑器追加解决记录，再回到首页核对消除；使用普通副本重试接口确认后首页仍显示待验证，执行受控Worker完成真实字节校验后才消除。H5代理只转发实际API字节，没有替换成功响应。两段MP4只记录最后的核验刷新，GIF明确为实际截图导览；并不声称录制整个编辑旅程。移动截图保留实际滚动视口，初始截图中文件组位于屏幕下方。

准备阶段4次未通过尝试保留在[attempts](evidence/operations-workbench/attempts/)：冷链接、缺少MFA准备、误启用其他测试模式及直接执行测试二进制的包目录配置。没有将这些尝试记为通过；最终页面检查使用准确原生二进制和正确包目录，数据库及所有自建浏览器、页面、代理、构建和临时服务正常释放。

开发重载曾在启动受管API后遇到Windows进程元数据查询超时。核对准确PID、启动时间和Python启动命令后恢复私有所有权记录，并按原检查续跑；仅将进程查询等待上限由20秒延长为60秒。其他进程保留。最终15组记录摘要一致，没有结构升级，全部服务及源码检查通过。

## 4. 保留证据与验收边界

最终功能源码检查18项、范围一致性12项及AC／TC适用性12项均通过。发布文件检查另见[仓库检查记录](evidence/operations-workbench/repository-check.json)，提交前还要对实际暂存区执行只读核对。[当前源码结果](evidence/operations-workbench/final-source-check.json)、[范围结果](evidence/operations-workbench/scope-check.json)与[适用性结果](evidence/operations-workbench/acceptance-applicability-check.json)分别保存；适用性分类不等于执行或通过业务用例。

- 增量121完整后端244个顶层用例／1535条通过结果、runtime56项、Android开发APK及26条JVM缓存结果保留原提交 `6e5607f908df0480df027b14976e9076bc812e2a` 绑定。本轮不声称重新执行这些检查，也不将旧APK标为122。
- 所有原已发布证据与327份媒体保持原字节，由最终检查逐一核对。README和媒体索引可更新，但不重写历史检查结果。
- 结构仍93，未新增或重写任何迁移。开发服务的受控重载不等于生产发布。
- 八项Skill仍draft，真实Codex、Linux隔离、TG、云、平台和生产验收保持开放。仅AI费用控制移出；授权、审计、隔离和技术资源限制保留。
- 不使用Docker、WSL、虚拟机、模拟器或真机，不执行网络性能、实网120GB专项、历史迁移或容灾。

Increment122 uses explicitly scoped current-source checks: five selected native Go/PostgreSQL integration suites (45 passing records),330 Web tests,427 portable mobile tests, shared type checks, Web/mini-program compilation and two Native JS checks. The only later source adjustment is one existing MFA setup call inside an opt-in browser fixture; original inputs are retained and reconstructed exactly. Published increment121 full regression, runtime and Android artifacts retain their original source bindings. Actual UI/API flows, owned synthetic byte verification and cleanup are recorded separately; they do not establish cloud, platform, model or production acceptance.
