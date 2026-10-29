# 依赖修复验证 / Dependency integrity verification

本轮应用与源码检查已完成：标准安装、实际调用、客户端、Android及H5产物均已核对，最终来源检查31组通过。完整npm审计由63项（56 moderate／7 low）降至0项，未省略开发、可选或peer依赖。该结果不替代正式生产依赖与产物验收；GAP-26继续active，当前23个工作包（14 P0／9 P1）及5项移出不变。

[实现与命令](../development/108-dependency-integrity.md) · [原始证据](evidence/dependency-integrity/) · [全部当前待办](../delivery/03-goal-gap-assessment.md) · [当前验收适用性](04-current-acceptance-applicability.md)

| 编号 | 检查 | 实际结果与来源 |
|---|---|---|
| V-01 | 标准干净安装 | 原生Node22.23.4／npm10.9.9，标准`npm ci --no-audit --no-fund`通过；安装脚本启用，锁与清单字节不变，未使用force／legacy-peer。见[安装](evidence/dependency-integrity/clean-install.json)、[配置](evidence/dependency-integrity/install-config.json)。 |
| V-02 | 修复来源 | 7个直接公告包有明确处置；合并库保留MIT许可证、原入口SHA256及原问题合成复现。逆转3处写入和新增辅助函数后，源码哈希与原版一致。见[处置](evidence/dependency-integrity/remediations.json)、[原复现](evidence/dependency-integrity/deepmerge-before.json)。 |
| V-03 | 完整依赖回归 | 当前56项依赖／安全检查及2项原生JS全部通过，无跳过／取消；完整`npm ls --all --json`有效。源文件、ZIP、日志、完整树及失败尝试均保留。见[当前结果](evidence/dependency-integrity/security-checks.json)、[实际树](evidence/dependency-integrity/attempts/full02/installed-tree.json)。 |
| V-04 | 完整npm审计 | 63→0，全部严重级别为0。未设置公告忽略、依赖省略或改写原公告；自维护补丁另有源码与行为证据。包级传播告警不等于独立CVE数量。见[审计](evidence/dependency-integrity/dependency-audit.json)、[完整原始报告](evidence/dependency-integrity/audit-after.json)、[剩余清单](evidence/dependency-integrity/remaining-findings.json)。 |
| V-05 | 客户端 | 8项命令全部通过：类型检查，Web172／移动273／运行时55项，Web／运行时构建及微信／抖音构建；两小程序各20页。三份摘要由真实命令记录转换，没有重复计入测试执行。见[命令记录](evidence/dependency-integrity/client-checks.json)、[移动产物](evidence/dependency-integrity/mobile-build.json)。 |
| V-06 | Android | 当前原生JS／Gradle构建及18项APK检查通过；APK包含当前JS字节，URI适配与维护版解析器均出现在实际Metro包内。26条JVM结果来自Gradle缓存，未计为新测试执行。见[构建](evidence/dependency-integrity/build.json)、[APK](evidence/dependency-integrity/apk-inspection.json)、[URI来源](evidence/dependency-integrity/native-uri-bundle.json)、[JVM范围](evidence/dependency-integrity/android-test-scope.json)。 |
| V-07 | 实际H5与原生Edge | Taro watch服务5项、Edge页面3项通过；当前编译无错误，登录页读取／重载、Host／CORS符合检查。独立浏览器、上下文、临时进程及端口均关闭。未录入凭据、未新增业务媒体。见[服务](evidence/dependency-integrity/h5-server-check.json)、[浏览器](evidence/dependency-integrity/h5-client-check.json)。 |
| V-08 | 保留媒体与后端 | 121 PNG／45 MP4／45 GIF共211份媒体和7份原始记录保持发布字节。未改后端224个顶层测试／1355条结果及2个Linux交叉构建沿用原执行范围；不计为本轮重跑。见[清单](evidence/dependency-integrity/reused-evidence.json)。 |
| V-09 | 开发持久环境 | 原生只读核对通过：schema83、10项基础探测、0临时测试库、0激活运行配置、外部集成关闭，保留原有API与Worker进程。见[环境](evidence/dependency-integrity/development-project-resources.json)。 |
| V-10 | 最终源码 | 31组核对通过，覆盖当前源码、锁、原始输入、安装树、失败尝试、四端产物及保留证据；full_goal_complete=false、production_ready=false。见[最终记录](evidence/dependency-integrity/final-source-check.json)。 |
| V-11 | 范围、适用性与发布 | [范围12项](evidence/dependency-integrity/scope-check.json)和[适用性12项](evidence/dependency-integrity/acceptance-applicability-check.json)一致性检查通过。保留原30项AC／72项TC编号，分类执行业务用例与新增通过数均为0。发布检查记录为同目录`repository-check.json`；提交前执行`verify_repository_publication.py --staged`核对实际暂存字节。 |

下列早期结果均按当时的真实结果保留，没有改写为成功。完整回归的失败与成功重跑采用相同源码、锁、断言及期限。

| 编号 | 尝试 | 原始结果及处理 |
|---|---|---|
| A-01 | [install01](evidence/dependency-integrity/install01.json) | 安装命令退出0，但随后实际树仍包含旧嵌套合并库／URI库及错误链接，不能作为干净安装验收。原记录只证明该命令完成。 |
| A-02 | [focused01](evidence/dependency-integrity/attempts/focused01/result.json) | 16项中13通过／3失败：残留依赖、Windows盘符被误作source-map URL scheme，以及Windows不支持尖括号文件名。保留完整输入ZIP、原日志和无效依赖树；改用合成HTTPS名字／内存读取器及真实重定向函数。 |
| A-03 | [dedupe02](evidence/dependency-integrity/dedupe02.json) | 正常去重因既有React18／19的不同平台依赖关系触发ERESOLVE而失败。未强制统一React或使用legacy-peer；[resolution03](evidence/dependency-integrity/resolution03.json)在独立源码目录中清除8条冲突候选记录，由正常npm解析生成可移植锁，再通过V-01干净安装。 |
| A-04 | [focused02](evidence/dependency-integrity/attempts/focused02/result.json) | 15通过／1失败，完整实际树有效。UUID的`dist/cjs/package.json`只有模块类型，验证辅助函数误将它当成带版本的包清单；继续向上查找具备名称／版本的清单后，完整回归通过。UUID实际调用在本次已通过。 |
| A-05 | [full01](evidence/dependency-integrity/attempts/full01/result.json) | 56项中55通过／1失败：既有合成开发服务器HTTP请求触及原10秒期限；2项原生JS与完整树通过。保留全部输入和日志。 |
| A-06 | [full02](evidence/dependency-integrity/attempts/full02/result.json) | 与A-05相同源码、锁、断言和期限，56＋2及完整树全部通过；未放宽原有超时或删除断言。 |

发布清单修正记录：[首次发布检查](evidence/dependency-integrity/publication-attempts/01-code-list-classification/result.json)及[当时清单](evidence/dependency-integrity/publication-attempts/01-code-list-classification/baseline-before.json)按原字节保留。首次格式检查把ZIP／原始日志列作代码／指南而失败；修正格式输入分类，原始证据保持不变，文件／敏感信息／链接／产物检查范围继续覆盖这些文件。

[两次早期命令日志来源](evidence/dependency-integrity/early-attempt-logs.json)保留原始结果SHA256及脱敏公开日志；完整回归每次保存全部输入和实际安装树。npm安装仍有上游弃用提示，原始日志保留，不把0条审计公告表述为生产安全签核。

本轮没有改变业务页面或Go／SQL实现，因此沿用原211份媒体及原后端记录；依赖敏感的调用、客户端构建和实际H5另行新执行。H5关闭／重载时的提前关闭流信息保留在私有原始日志及[脱敏归档](evidence/dependency-integrity/h5-log-archive.json)中，检查仍要求当前编译成功、页面无运行错误及所有自建资源释放。

本轮未启动Docker、WSL、虚拟机、模拟器或真机。技术期限用于终止卡住的检查，不是网络性能验收；不增加实网120GB、2TB迁移、容灾或AI费用门槛。AI秘书、八项Skill、TG、授权、审计、执行隔离与技术资源限制，以及公司OA／财务、OSS＋R2及正常PG结构升级继续保留。

The complete npm audit changes from 63 findings to 0 with all dependency scopes included. The local merge patch has disclosed upstream provenance and behavior checks. All 56 dependency cases, 2 native JS cases, 8 client commands and the actual H5/native Edge check pass. The fresh development APK passes 18 checks, its bundle includes both URI sources, and all 31 source checks pass. The 26 JVM results came from Gradle cache. Scope and publication records are tracked separately in V-11. Historical UI media and unchanged backend/cross-build results retain their original scope. The full 23 work packages remain active; no live model/cloud/TG or production acceptance is inferred.
