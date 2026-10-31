# 增量148：当前功能冻结与完整交付 / Current functional freeze

## 1. 交付决定

主线147增加复制运维，但rc.4／开发APK146仍固定145／结构110。本轮关闭这个可直接完成的交付差异：以业务147提交`4155ce9c19304b8c65f5636e09707a5eba1e21cd`和结构111生成rc.5。四端范围保留，原四个冻结版本不移动。当前版本是功能冻结候选，真实服务配置、正式签名及公司完整验收仍按23项工作包继续跟踪。

业务代码、所有111项结构及旧证明／媒体原字节不改。本轮只增加六个准确输入构建／检查／封存／Git发布工具、清单和交付文档；没有重做界面或继续扩展业务功能。

## 2. 包与源码边界

| 编号 | 产物 | 内容与边界 |
| --- | --- | --- |
| D148-01 | Linux amd64归档 | 三个原生程序、Web、秘书JS／Skill包、111项结构、systemd模板及发布工程；793项包文件，交叉构建，未在目标Linux运行 |
| D148-02 | 两小程序ZIP | 微信／抖音共151项输出，实际JS包含复制台账、依据核对及重试路径；还需真实平台身份及域名 |
| D148-03 | Android开发APK | 实际JS／DEX／权限／签名／20页注册核对，185项选定源码映射一致；含147复制运维，开发签名和本地API |
| D148-04 | 完整应用源码ZIP | 全部应用实现、测试、锁文件、结构和方案／开发文档固定Git147；历史大体积质量证据由完整Git标签提供 |
| D148-05 | 媒体ZIP | 全部冻结docs/media，含147真实页面截图／无声操作MP4／截图导览GIF；没有重新录制 |

源码与媒体共3289项Git blob逐字节还原；五包展开5227项成员，检查路径、归档完整性、已知本机私密值与高置信度凭据模式。每包小于100MiB。该扫描不包含像素OCR，不声称形式化秘密清除或生产安全签核。源码标签和ZIP固定147，148构建工具和本说明在main。完整历史证明用冻结Git标签；较小交付源码ZIP不包含docs/quality/evidence。

## 3. 员工使用逻辑

登录后按公司角色及项目范围工作。项目关联任务、交付、文件、博客和OA；合同、采购、报销按照既有版本／审批状态推进；审批不直接产生现金，费用、付款、核销和退款各有准确事实与历史。秘书文案和计划须显式人工审阅／采用，真实模型未配置时不伪造回复。

新增复制运维由Web `/storage-operations`及共享移动“设置→复制运维”进入。先筛选存储端、副本及任务状态，再查看当前元数据和不可变历史；有权限的人员确认当前版本及basis后明确重试。后端重新核对组织权限、普通文件读／写、当前会话及近期MFA；状态变化拒绝旧依据，刷新后重审。幂等回执保留原请求事实，实时状态另行读取。它复用原复制工作器，不授予组织元数据读取者额外文件内容权限。

详细角色、状态、HTTP路径与失败语义见[147开发文档](147-storage-operations.md)。本轮不新增角色／动作，也不替换真实云端配置。

## 4. 管理员部署顺序

1. 从[固定下载](../releases/v0.1.0-rc.5/README.md)获取包、SHA256SUMS及内层可信清单摘要。源码和媒体解压到一个新目录。
2. 在可信源码下运行归档inspect，核对外层文件摘要和内层清单分别对应的内容。
3. 根据[原生发布手册](../operations/03-native-release-runbook.md)准备目标Linux、独立系统身份、持久PG、HTTPS和组织外私有配置；暂存、正常升级、激活并核对逐项就绪。
4. 新库应用结构1–111；现有开发PG100若连接当前后端，必须正常应用101–111。本轮保持原PG及API原进程、原结构，不将本地升级冒充生产部署。
5. 配置真实OSS＋R2用途身份／桶、Codex根broker专用凭据和Linux执行边界、TG群／Bot、监控目标及平台资料。凭据留私有路径，不提交Git或录入演示。
6. 小程序及APK目前固定本地 `http://127.0.0.1:18080`。连接真实服务器前按HTTPS API、平台AppID／域名及正式Android发布配置重建。真实平台发布与公司制度需对应工作包签收。

不启动Docker、WSL、虚拟机、模拟器或真机；本轮未新建浏览器／临时数据库。保留用户原服务。

## 5. 可重现工具与固定引用

| 步骤 | 工具 | 输入／结果 |
| --- | --- | --- |
| 服务器 | `build_candidate_148_server.py` | HEAD精确147；原生Node22.23.4；1355项实际服务器输入与Git字节一致 |
| 移动端 | `build_candidate_148.py` | 两小程序及Android本地构建；原始Gradle日志用JSON无损封装，缓存任务如实保留 |
| APK检查 | `inspect_candidate_148.py` | 实际APK、DEX、源码映射、签名和权限；10项检查 |
| 封存 | `seal_candidate_148.py` | 五包与摘要、来源清单、完整源码／媒体还原和私密值扫描 |
| 准备引用 | `publish_candidate_148.py` | 独立Git index生成产物提交和新标签，拒绝覆盖已有引用，不自行push |
| 当前核验 | `verify_candidate_148.py` | 19项来源／包／引用／保留证据检查；只读核验，不重执行业务测试 |

这些一次性工具只接受原冻结HEAD和全新私有输出目录；已经存在的标签／目录须保留，不为重跑而覆盖。复核现有交付应使用只读检查；全新重建需在独立原生检出中按相同输入操作，并保留差异／失败记录。

源标签：`v0.1.0-rc.5`；产物分支：`codex/release-v0.1.0-rc.5`；固定产物提交：`0f7a0c0158c7b86a8301a7f6d6798d6608f1951f`。授权SSH推送后回读五次冻结共15项准确引用；无强推。GitHub API沿用144的403事实，本轮不重试现有凭据，不宣称已创建Releases网页。

## 6. 验证取舍与后续

本轮新做封包与实际内容检查；147业务测试／媒体原字节沿用，28项JVM XML为UP-TO-DATE缓存，未重执行业务、完整回归或浏览器录制。145运行器／136完整客户端／121完整后端按原出处保留，不汇总成148新通过数。

23项仍active，14 P0／9 P1。真实Linux／生产PG／OSS＋R2／Codex／TG／平台及正式Android、公司规则与完整UI旅程未验收，八Skill仍draft。真机、网络性能／实网120GB、历史迁移／容灾与AI费用控制已移出当前门槛；AI授权／隔离／审计／技术限制、普通恢复和正常结构升级继续保留。

The release resolves the current artifact/source mismatch without widening the business implementation. Current packaging is fresh; prior147 business/media and earlier full-regression proofs retain their original provenance. Formal signing, production integration and all23 complete work-package acceptances remain open. See [quality record](../quality/148-frozen-release-verification.md) and [numbered remaining work](../delivery/05-functional-closure-148.md).
