# 增量167：原生受控发布闭环独立反证

审查基线：`e36ec01a7b78eed742a7c1f6c831a4114b96bcde`。这是在财务专项报告完成后单独指派的部署反证，仅阅读源码和规范。没有运行控制器、测试、构建、服务、浏览器、Linux／systemd、Docker／WSL、云接口；没有修改应用代码。下文“失败”指由明确源码依赖推出的行为，不是本次实际执行日志。

## 1. 结论

两个候选均**确认成立**。在列出的其他后台模式和外部原生资产依赖中，没有再确认一个同等证据完整的新代码缺口。实际账号、证书、外部工具和模型／存储配置仍待提供，不能把这些正常安装责任计作缺少应用实现。

| 编号 | 裁定 | 影响 | 当前状态 |
|---|---|---|---|
| DEP167-01 | confirmed，置信度高 | 生产包中的独立发布控制器漏带顶层必需模块；按照运行手册只安装随包发布工具时，所有控制器CLI在进入子命令前即无法导入 | 需要代码修复；本审查未修复、未执行 |
| DEP167-02 | confirmed，置信度高 | 已实现的原件清理后台命令没有进入固定服务目录，无法通过当前版本化配置、单元生成、停启、回退／恢复体系受控运行 | 需要代码修复；不是未知云凭据或用户移出的容灾项 |

## 2. DEP167-01：独立发布工具依赖不闭合

**规范。** `docs/development/119-native-production-release.md:13–14`将原生协调器和固定服务目录作为普通发布工具；`docs/operations/03-native-release-runbook.md:21`规定 `/usr/local/lib/copypaste-release` 为经管理员核对的“三份发布工具”；`:93–95,107`直接从该独立目录运行 `controller.py stage/status/activate/recover`。这条使用路径没有要求目标服务器另放完整源码仓库或开发PYTHONPATH。

**实施链。**

1. `app/scripts/build_production_release.py:103–106`创建 `metadata/release-tools`，准确拷贝集合为 `('bundle.py', 'catalog.py', 'controller.py')`。
2. 当前 `app/deploy/release/controller.py:24–26`在模块顶层依次导入 `bundle`、`catalog` 和 `telegram_config.validate_telegram_file`。后者没有条件开关，即使本次只运行 `status` 或TG关闭，也必须先成功导入。
3. 实际模块是 `app/deploy/release/telegram_config.py`，不在上述拷贝集合中。控制器没有相应的内嵌实现或备用导入路径。`controller.configuration:245–248`还实际调用该函数核对固定私密TG文件，所以不是可删除的闲置导入。
4. `build_production_release.py:115–118`只是把已经拷贝的树编入清单；清单完整性不会凭空补齐该依赖。既有候选封装检查从源目录调用 `app/deploy/release/bundle.py` 检查压缩包（例如 `app/scripts/seal_candidate_164.py:70`），不是从归档独立目录导入控制器，因此不能据此反驳本缺口。

**确定的触发。** 在干净目标环境，仅把包内 `metadata/release-tools` 安装到手册规定目录，然后运行其 `controller.py status`。Python在进入CLI解析前需要 `telegram_config`；目录缺少该模块且没有其他安装来源时会发生模块导入失败。这里不是声称所有可能带全局同名模块的Python环境必定失败，而是发布包本身未满足其独立使用契约。源目录运行成功不证明此交付闭环成立。

**完整可行修复。**

1. 在发布工具依赖闭包中加入 `telegram_config.py`，用一个固定受审集合同时控制复制、源码绑定、产物检查和手册安装数量。保留严格TG文件解析，不用吞掉导入错误、从网络安装或放宽普通环境解析代替修复。
2. 打包清单继续覆盖每个实际工具的准确字节和路径；离线校验同时要求完整工具集合及可解析的顶层依赖。更新中文／英文指南和“三份”旧表述。
3. 在整轮源码保存后统一验证：仅复制完整随包工具到新的隔离目录，用不继承开发模块路径的Python执行帮助入口／导入；覆盖TG关闭时也需要模块及缺少任一固定依赖明确失败的场景。此检查不需要root、真实Bot、Linux服务或服务器配置。若检查实际归档，须以归档解出的工具为输入，不再从源目录代跑。
4. 旧冻结包和既有证据保持原样；新发布的元数据、工具清单和说明同步绑定当前修复来源。

## 3. DEP167-02：原件清理进程缺少受控发布接线

**规范。** 当前范围保留普通已知对象清理和普通文件／版本能力；用户移出的“历史数据迁移与容灾”不是关闭普通已授权清理的指令。`docs/operations/03-native-release-runbook.md:25,49,86`要求已启用功能采用固定 `copypaste-release-*.service` 并与版本化配置绑定，升级前不得遗留未受管旧Worker。

**实施链。**

1. `app/backend/cmd/platform/main.go:68–69`明确将 `worker-original-cleanup` 路由到 `runOriginalCleanupWorker`。
2. `app/backend/cmd/platform/object_retirement_worker.go:14–38`确有业务运行循环：独立worker配置→独立原件清理存储身份→准确schema检查→`files.ObjectRetirementService.RunOne`。这不是仅有命令名字或文档占位。
3. `app/deploy/native/copypaste-original-cleanup-worker.service:10–12`已有旧路径单元：`/opt/copypaste/bin/platform worker-original-cleanup` 和旧普通环境文件。该单元不随当前发布指针运行。
4. 当前 `app/deploy/release/catalog.py:2–20`共23个固定服务项，含 `original-observation`、`multipart-cleanup`，**没有 `original-cleanup`**。`catalog.unit:33–36`拒绝未知服务。
5. `controller.configuration:229–231`强制 `deployment.json.units ⊆ SERVICES.keys()`，所以操作者无法以合法配置启用原件清理。`:235–241`只从该目录派生环境文件集合；`:260`又拒绝额外文件。手工添一个清理环境文件或任意unit名都不能成为当前受控配置。
6. `build_production_release.py:108–111`只遍历 `SERVICES`生成随包单元；`controller.py:433`及普通启动／停止路径`:150–156`也按同一固定集合控制。故问题贯穿打包、启用、切换与恢复，而不是单独缺少一个示例文件。

**独立反驳尝试。** 旧native单元能手工启动该命令，说明业务后台能力存在；但旧单元使用旧二进制路径和独立配置，且运行手册要求停用旧同类进程后再升级。使用该单元不是在当前受控发布体系中完成启用，无法消除本缺口。云凭据未提供会使已接线Worker保持关闭；这里在有正确凭据时仍先被固定目录拒绝，二者不同。

**完整可行修复。**

1. 把固定 `original-cleanup` 项加入当前目录，绑定准确子命令、单独受限系统身份和固定 `original-cleanup.env`。原件删除身份与HEAD-only观察、multipart清理身份应继续独立；只给真实已授权原件所需存储操作，不扩大观察身份权限。
2. 复用既有目录生成普通 `copypaste-release-original-cleanup.service`，使用 `/opt/copypaste/current/bin/platform worker-original-cleanup`、受控 `/etc/copypaste/active/original-cleanup.env`、独立状态／缓存目录和既有受限系统设置。不要新增绕过协调器的手工切换流程。
3. 让配置文件完整集合、指纹、单元预检、停启顺序、失败应用回退和中断恢复自然包含该固定项。文档明确真实身份／凭据准备与旧手工清理单元的盘点退役步骤；保留禁用时不启动的行为。
4. 整轮代码后统一检查：全部实际后台模式与目录的覆盖矩阵，启用／禁用清理的准确文件集合、未知模式拒绝、渲染命令／独立身份、切换与失败恢复包含新服务。使用本地进程替身只能证明状态机接线，不伪造Linux原件删除或云端验收。

## 4. 其余模式和依赖的覆盖核对

`app/backend/cmd/platform/main.go:62–93,181–221`共有21个业务后台命令。除上述原件清理外，其余20个都存在当前目录映射；API及两个独立二进制另计。核对的是实际命令分支，不是错误提示里不完整的命令说明文本。

| 实际模式／依赖 | 当前映射或约束 | 裁定 |
|---|---|---|
| `worker-upload`，verify／promote／replica／restore各small／large，共9个 | `catalog.py:11–13`与`main.go:181–221`逐一对应 | 没有漏项 |
| notification／subscription／report-export／file-text／original-observation／multipart-cleanup，共6个 | `catalog.py:5–10`对应独立命令分支 | 没有漏项 |
| telegram／telegram-delivery／notification-telegram，共3个 | `catalog.py:15–17,25–30,46–48`；`controller.py:243–248`；真实私密FILE解析在打包问题修复后仍应保留 | 三种TGworker均已接线，不误报166新增个人TG模式仍缺失 |
| agent／agent-reconcile，共2个 | `catalog.py:18–19,41–44,51–54`；`controller.py:234,354–360`固定broker依赖、候选运行目录和二进制摘要 | 目录覆盖；原生真实配置／安装验收另行开放 |
| agent-native独立二进制 | `build_production_release.py:84–89`构建；`catalog.py:3,43,51–54`启动／预检及可写根；`build_production_release.py:112–114`生成固定socket包装器 | 已存在代码路径。Linux Node／Codex、根私钥、库与实际UID/GID由管理员准备，是明确部署前置 |
| status-bridge独立二进制 | builder`:84–89`；`catalog.py:14,44,49–50`将JSON通过PID1凭据交给独立用户 | 不能把根私密配置不可由普通账号直接遍历视为bug，PID1负责凭据交付 |
| PDF／OCR原生profile | `cmd/platform/text_worker.go:38–45`读取显式绝对profile；`internal/documenttext/profile.go:111–137,139–192`固定工具／模型摘要和临时根；`catalog.py:53–54,82–84`允许该服务状态／缓存路径及PrivateTmp | 没有确认必须写只读应用目录的调用。应将临时根放到该服务可写状态／缓存中，并给外部只读工具和profile所需读取权限；不能把任意不匹配路径当默认必然故障 |
| profile未配置时的PDF／图片 | `docs/development/128-document-text-ocr.md:27–39`明确应用不捆绑下载原生工具；未配置时只保留普通文本／Office，PDF／图片不调度 | documented support/configuration boundary，不记缺实现或正式验收通过 |
| 原生broker工作根／socket | `nativeruntime/config.go:96–118`限制绝对、隔离及socket属于state根；`linux_server.go:129–142,172–178`创建／复核state与socket；示例`deploy/native/agent-native.example.json:2–5,12`与固定目录／包装器一致 | 默认受控布局有对应路径；不同布局须连同受审unit／包装器约束设计，不凭任意配置宣称默认代码不能工作 |
| 对象存储工作目录 | `cmd/platform/main.go:181–219`注入准确独立存储身份；`objectstore/protocol.go:154–159`、`checkpoint.go:129`使用有界流式摘要；本次相关目录搜索未见要求把120GB落地应用目录的实现 | 未确认需要另添一个通用可写应用根。真实对象身份、扫描政策、OSS/R2凭据、权限和容量仍待配置 |
| 独立监控 | `deploy/monitoring/monitoring.py:9–10,21–42`从源码工具生成私密配置／unit；`render.py:27–58,123–127`有五个独立进程、固定状态路径；手册`26-independent-monitoring.md:21–29`明确分别安装二进制、应用/监控主机units及可读非秘密配置 | 此组件明确独立于应用发布；应用包不替管理员安装另一台主机不构成隐藏缺实现。真实目标／接收人／watchdog待配置 |

后台业务数据操作仍须经过各自应用授权与存储约束，本报告没有重新审查删除安全、TG披露或模型隔离全部不变量。部署前置未提供的，不能写成已经投产。

## 5. 实际只读检索

- `rg --files app`筛选release、documenttext、systemd、monitor、broker文件；分别定位控制器及固定模板，没有扫描私密配置。
- `rg -n 'worker-|case |os.Args' app/backend/cmd -g '*.go' -g '!**/*test.go'`定位真实后台分支，随后逐行阅读 `main.go`及实际worker入口。
- `rg -n 'bundle.py|catalog.py|controller.py|release-tools|telegram_config.py|original-cleanup' app/scripts app/deploy/release -g '*.py' -g '!**/test*'`定位打包、目录、控制器导入及现有封装检查调用。
- `rg -n 'CP_|profile|ReadWritePaths|StateDirectory|WorkingDirectory|LoadCredential'`配合具体release、原生模板、documenttext、filescan、storageconfig文件，核对配置来源及可写路径。
- `rg -n 'TempDir|TempRoot|Mkdir|CreateTemp|TMPDIR|scratch'`对相关storage／upload worker目录未找到强制本地大文件缓存路径；正向阅读对象存储流式实现支持该有限结论，不推导真实内存／网络性能结果。
- 对 `app/deploy/monitoring`、部署运行手册、105／128／166说明逐段读取；确认外部根管理资产和真实安装的明确责任边界。

少数首次猜测的文件名不存在或输出过长，已用 `rg --files`及UTF-8限定行号片段纠正；不把错误检索或被截断内容算作已读证据。两个确认项来自可复核的打包集合、顶层导入、真实命令、固定配置约束和控制器调用链，均不依赖未执行测试的猜测。
