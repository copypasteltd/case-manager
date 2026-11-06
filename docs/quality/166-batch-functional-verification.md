# 增量166验证 / Batch166 verification

结论：本轮7项代码/接口/消费者/用例/工具/文档先完成并封存，才统一执行检查；首次结果收齐后集中修复。当前源核对21项通过。全部23正式包（14P0/9P1）与八项draft Skills仍开放，以下结果只属于已声明的本地范围。

| 编号 | 实际执行 | 结果及证据边界 |
|---|---|---|
| V01 | 四个原生HTTP/PG业务族 | 40条结果通过：完整项目结项/归档、当地日期、TG预授权三阶段/组织失效、原普通TG发送；首次业务检查即通过 |
| V02 | Web及共享移动业务 | Web527；共享移动553（240+308+5）通过 |
| V03 | 客户端7步骤 | 类型、Web/移动业务及补充契约、Web/微信/抖音构建通过；无新Android JS或APK |
| V04 | 原生受控发布 | 修复后42条通过；首次40成功/2错误的记录保留，普通进程/指针效果使用注入替身 |
| V05 | 监控计划 | 4条原生Python用例通过，含固定IP/私网拒绝、无秘密读取、拒绝覆盖/重复JSON、独立接收凭据 |
| V06 | 私密配置/投影 | 三个Go测试入口及其子例通过；未将Windows测试当作Unix权限、软链接分支或systemd验收 |
| V07 | 官方固定版本工具 | promtool配置/规则/五个规则场景、amtool配置、Blackbox config.check共5命令通过，均不监听、不探测/发送真实服务 |
| V08 | Web实际API/页面 | 2条检查、2张截图、实际静音MP4、截图导览GIF及实际录屏GIF通过；合成数据/本人正常登录 |
| V09 | Taro H5实际API/页面 | 同样2条检查、2张截图和三项媒体通过；不是微信/抖音运行或Android正式验收 |
| V10 | 当前源/原始输入/素材 | 21项核对通过，检查全部当前消费者、三份完整代码封存、精确原始助手脚本及媒体哈希 |

## 失败、修复与复用

1. 首次发布工程的两条新增用例误放到没有fixture helper的类，报AttributeError；在整批结束后集中移到独立Fixture测试类，复测发布工程42条。没有修改应用实现；原生业务、客户端、监控计划/Go配置和官方工具的通过证据按未变消费者保留。
2. 首次Web页面脚本点击了已经展开的订阅组，使内容隐藏并等待超时。修复为仅在折叠时展开；保留失败源码、结果和失败页面，所有资源已关闭。重录Web通过；H5首次执行通过。不重播原生业务/客户端检查。
3. 首次最终证据工具把Path当作字典索引，随后来源核对发现元数据使用read_text归一化换行而非归档原字节。修复路径连接并以原始归档/read_bytes计算摘要，保留原始脚本、异常说明、首个失败核查及修正前元数据。页面录制后的变化仅限这个证据脚本；当前工具逐字重构录制时的唯一旧助手文件，要求其他全部代码/工具字节相同。没有重录、修饰已通过素材或重新宣称业务测试。

[首次源码封存](evidence/batch-round-166/code-wave-initial-archive.json) · [集中修复](evidence/batch-round-166/unified-repair.json) · [页面脚本修复](evidence/batch-round-166/browser-repair.json) · [证据工具修复](evidence/batch-round-166/post-capture-verifier-repair.json) · [换行元数据校正](evidence/batch-round-166/evidence-byte-metadata-correction.json) · [全部当前源核对](evidence/batch-round-166/final-source-check.json)。后者对修正前版本进行精确约束，不允许任意应用变化豁免。

## 保留与未执行

保留121全回归、163 runtime66、164开发APK/business163/schema129及九次冻结，均未改写/重跑。没有当前Linux服务安装、Linux node_exporter运行、实际目标/Bot/watchdog接收回执、云存储/平台/Codex/正式签名验收。官方Windows组件完整包经固定摘要下载校验后只运行检查命令；Linux锁值来自官方发布摘要，不冒充Linux执行。

本轮不调度Docker/WSL/虚拟机/模拟器；不做真机验证、网络性能/实网120GB、历史迁移/容灾、AI费用控制。普通授权、审计、资源限额、PG升级与OSS＋R2/普通文件恢复保留。真实资料仍按用户“后续提供”，不反复追问。

三次实际页面尝试（首次失败Web、成功Web、成功H5）均记录关闭自建浏览器/上下文/临时服务并删除临时fixture，原生临时PG停止；开发PG100/API保留。浏览器当前展示四条当地日期样本、2条阻塞与2条日期生效，不把日期生效等同完整项目归档。

English: all seven declared implementation tasks and fixtures preceded the unified batch. Four selected native business families (40 result records), Web527/shared-mobile553, seven client steps, the repaired42-test release suite, native plan/private-config and pinned upstream parser/rule checks pass. Actual Web/H5 sessions each pass two API/UI checks with two screenshots and silent recording/GIFs; owned resources are closed. Failed fixtures, the initial collapsed-group capture and exact proof-helper/newline metadata repairs remain visible. No production/Linux/device/provider acceptance, new APK or full regression is claimed.
