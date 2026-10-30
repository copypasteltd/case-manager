# 增量119：原生生产发布工程 / Native production release engineering

本轮推进 GAP-24：提供完整服务器包的构建、校验、原生发布切换与故障处置代码，以及手动触发的原生 CI。业务应用源码仍为已发布118的准确版本；本轮没有增加业务页面，不重录界面。目标服务器、真实 CI Runner、HTTPS、系统服务与生产 PG 的实际验收仍未完成，GAP-24／25／26 保持开放。

This increment implements server packaging and controlled application deployment. It preserves the exact application sources and functional evidence from118. Local archive inspection and injected transition tests do not establish a running Linux deployment, kernel isolation, model integration or production readiness.

## 1. 交付与职责

| 代码 | 职责 |
|---|---|
| [构建器](../../app/scripts/build_production_release.py) | 编译三个 Linux ELF、Web与运行器；封装运行依赖、八项Skill、91项结构清单和固定服务模板 |
| [归档与离线检查](../../app/deploy/release/bundle.py) | 固定清单、全文件摘要、确定的归档元数据、限制大小、安全解包及独立检查入口 |
| [原生协调器](../../app/deploy/release/controller.py) | 发布锁、配置指纹、当前版本比较、维护状态、停服／结构升级／切换／检查、应用回退及中断处置 |
| [服务目录](../../app/deploy/release/catalog.py) | 固定服务命令、独立系统身份与经过检查的HTTPS入口内容；配置不能新增任意命令 |
| [原生CI](../../app/scripts/native_ci_release.py) | 专用非root Linux Runner，独立原生PG、完整回归、构建和只上传明确产物 |
| [工作流](../../.github/workflows/native-release.yml) | 只允许手动触发main，使用固定提交的官方Action，不自动部署 |
| [测试](../../app/deploy/release/test_native_release.py) | 实际本地文件／归档和发布状态机故障注入；明确区分Windows指针替身与Linux链接 |

操作顺序、目录、配置格式及命令见[完整运行手册](../operations/03-native-release-runbook.md)。[验证与限制](../quality/119-native-release-verification.md)记录实际执行范围。

## 2. 发布包约束

包内包括 `bin/platform`、`bin/agent-native`、`bin/status-bridge`，以及Web、独立运行器依赖图、Skill和服务单元。Node／Codex原生可执行文件由服务器另行固定；本包不把Windows可执行文件当成Linux程序。两小程序和Android的正式发版仍分别属于 GAP-19／16。

`release.json`固定目标架构、基线Git提交、准确构建输入摘要、工作区是否有未提交内容、全部迁移名称／摘要，以及每个文件的路径、长度、SHA-256和安装模式。部署标识取完整清单摘要的前24位；部署仍核对完整64位摘要，已存在目录不覆写。

| 边界 | 约束 |
|---|---|
| 文件 | 最多20,000个；单文件128MiB；总内容1GiB；清单8MiB |
| 路径 | 只接受固定包分区中的规范相对路径；拒绝绝对路径、返回上级、反斜线、重复成员及特殊文件 |
| 归档 | 不调用通用解包执行器，不采用包内owner／权限／链接元数据，不执行包内代码 |
| 依赖元数据 | 原运行依赖图中的 `.runkit_example.js`、`.eslintrc.yml` 明确允许；秘密点文件继续拒绝 |
| 冻结 | 文件和目录只读，二进制保留执行权限；文件写入同步，目标目录唯一；安装后再次核对准确清单与模式 |
| 静态资源 | 内容有摘要的Vite资源在独立只读目录保留；同名不同内容拒绝，保留已打开页面需要的旧资源 |
| 秘密 | 不打包环境文件、数据库、密钥、生产配置、开发缓存、日志或签名资料；本地实际产物另扫描已知私有值 |

确定的tar/gzip元数据保证相同包内容产生相同归档，不承诺不同编译器／依赖安装可重现相同二进制。构建记录另保存真实工具版本、原日志摘要和完整私有输入源码归档。本轮本地包明确标记“118基线＋本轮源码、工作区有变更”；正式CI要求干净提交。

## 3. 配置和身份

配置放在 `/etc/copypaste/configurations/<配置编号>`，不放进应用发布目录。每个配置只接受 `deployment.json`、专用 `migration.env` 和已启用服务需要的固定文件。环境文件使用唯一、普通的 `KEY=value` 行，不执行shell，不接受引号、反斜线或额外未知文件。

配置根下的具体配置目录和文件仅root可读；systemd读取各自环境文件。监控桥接通过 `LoadCredential` 获取自己的JSON配置；模型与证明私钥继续由root执行控制器读取。实际角色、UID、文件祖先权限、密钥引用与网络权限仍须在目标服务器逐项确认。

公开版本与资源祖先目录在限制性umask下仍明确创建为0755；已有目录权限不符会拒绝，发布版本内部继续冻结。Windows检查不证明POSIX权限。

所有业务与迁移连接必须指向同一PG地址和数据库并启用 `sslmode=verify-full`；运行连接不能使用 `postgres`／`root` 登录，迁移与运行登录名按URI解码后必须不同。连接查询只允许唯一非空的TLS、application_name及connect_timeout参数；拒绝host／port／dbname／user／service／options等实际地址或身份覆盖，数据库名按真实解码结果比较。正常应用发布拒绝切换数据库。此检查不替代生产角色权限、证书和实际读写验收。

配置指纹覆盖全部准确文件内容。每次使用新配置编号；不编辑已采用配置。配置值不进入操作回执。原生模型配置必须固定候选发布目录、准确运行依赖图和broker二进制，不能沿用上一版不匹配的路径／摘要。

## 4. 发布和失败路径

1. 管理员离线核对可信来源的完整清单摘要，检查包；在原生Linux上暂存为新版本，不覆盖旧版。
2. 准备独立配置，安装并检查固定服务单元与HTTPS入口，确认系统身份、密钥和已运行旧服务范围。
3. 使用准确候选版本／配置、完整清单摘要与预期当前版本发起切换。目录权限、单元内容、配置指纹、HTTPS配置和当前数据库结构先检查。
4. 写入持久操作记录，关闭HTTP维护入口，停止当前及候选配置涉及的受管服务。
5. 重新读取数据库。仅允许准确相同结构，或明确选择 `--migrate` 且原结构是候选清单的完整前缀。迁移由专用身份执行；不运行向下迁移。
6. 完整核对新结构、包和配置后切换应用及配置指针，启动固定服务，逐服务检查活动状态并检查API准确就绪响应。
7. 保存当前版本和操作结果，再开放入口。旧发布、配置及内容资源保留。

若失败且原程序、配置、服务模板和当前数据库仍完全匹配，尝试恢复原应用／配置，重新检查就绪；记录本次失败及回退。若数据库已升级、部分升级、服务无法停止或旧配置变化，保留维护状态和待处置记录，要求明确继续修复。不得通过取消结构检查启动不匹配的旧程序。

进程中断后使用 `status` 查看准确操作编号，选择 `recover --mode forward` 或 `previous`。继续结构升级仍须明确 `--migrate`；恢复上一版要求当前结构与上一版完全一致。不是备份还原、历史迁移或容灾流程。

操作事件先写独立临时文件、同步并原子落盘，串联前一事件摘要；当前指针与待处理记录采用替换写入。若事件已完整落盘而pending尚未更新，状态读取核对整个事件链与原摘要锚点后采用完整尾部，不改写旧事件或只读状态文件；未完成临时文件不作为正式事件。恢复沿最新完整事件的目标继续，链损坏在服务操作前拒绝。操作锁防止两个管理员同时更改状态。root管理员仍是主机信任边界，不声称可以抵抗root主动篡改。

## 5. CI与发布授权

工作流只在人工选择main后运行，权限仅为读取仓库；构建Runner与生产主机分开。Runner需要原生Linux、Node22.15～22.x、Go1.27.3、Python3和PostgreSQL18工具；不创建容器、WSL、虚拟机或模拟器。GitHub支持使用标签选择自行管理的Runner，参见[官方说明](https://docs.github.com/en/actions/how-tos/manage-runners/self-hosted-runners)。

CI创建只监听回环地址的独立原生PG，使用随机测试凭据，保持同步持久化；执行完整后端与客户端检查、两小程序构建、服务器包构建。测试日志与数据库保留在私有目录；仅上传明确指定的冻结包及无秘密回执。取消或失败时仅停止准确归属本作业的PG；下一次清理工作区前先处理遗留的受控测试实例。

CI不取得生产迁移身份、SSH、模型／云密钥或Android签名私钥，也不执行生产激活。构建产物审核与实际服务器部署是独立步骤；本轮没有触发真实工作流。正式产物安全签核、Linux运行／隔离及完整业务验收继续开放。

## 6. 范围

23项工作包、14项P0／9项P1和5项移出不变。保留四端、Codex、八项Skill、TG、OSS＋R2、普通恢复及生产PG。仅AI费用控制移出，技术资源限制与业务授权继续保留；本轮不增加真机、网络性能、历史导入或容灾门槛。
