# GAP-24 — 原生生产部署与发布工程源码审查

> “目标服务器可按记录部署、启动和检查就绪，发布配置及客户端版本一致。”
> — `docs/delivery/04-current-scope.json:430`
>
> “本地冻结包与构建检查不等于目标服务器上线。”
> — 同文件`:429`

**判定：**发布工程代码路径已存在；实际目标验收仍 `undecidable`。本范围新增必须开发的代码缺项 **0**；另发现一处当前操作手册与既有代码不一致。审查基线为 `930cdde97b9d5d2e85d892664b0857a10a811739` 加在途168工作区；不是执行或部署结果。

---

## 实际执行链

- `app/scripts/build_production_release.py:70` 保存准确输入源码及摘要，编译三个Linux ELF、Web和独立JS运行依赖图；`:110` 通过固定 `RELEASE_TOOLS` 复制完整安装工具，`:120` 绑定迁移名称及摘要，构建输入变化即拒绝。`app/scripts/build_native_runtime_release.py:88` 遍历实际运行依赖，排除测试、缓存和秘密目录；独立Node/Codex由目标策略固定，符合119设计`:23`，不是遗漏Windows二进制的缺陷。
- `app/deploy/release/bundle.py:62,71,92,123,146` 约束路径、数量、大小、普通文件和准确摘要；新目录逐字节写入后再核对，不采用tar链接、owner或特殊成员。`controller.py:285` 唯一暂存、只读模式和静态资源保留；同名不同内容拒绝。
- `controller.py:39,221,350` 核对root所有及不可改写的祖先、私有配置、固定服务集合、HTTPS域名、同一PG与迁移/运行独立登录，拒绝凭据覆盖及额外文件；启用broker时核对本次完整运行器和二进制摘要，并实际调用其 `check`。`nativeruntime/linux_server.go:22,37,96` 的callee校验原生root、systemd255、cgroup控制器、准确可执行文件与运行依赖图，失败拒绝；没有Windows代替路径。
- `catalog.py:2,11,42,95` 包含四个安装工具、独立清理身份/固定命令、各服务单元和TLS/维护入口。命令消费者在 `backend/cmd/platform/main.go:62–95,181–224` 存在，broker和监控对应各自二进制。协调器 `preflight:158` 核对已安装单元及Nginx内容，`healthy:170` 逐个检查服务而非“任一活动”，并读取实际API就绪响应；`httpapi/server.go:56` 的响应依赖准确编译迁移清单检查。
- `controller.py:483 → execute:431` 在原生锁内检查预期当前版本、完整schema前缀和明确升级授权，先持久记录及维护，停止受管服务后复核数据库；迁移成功及候选配置再次匹配才切换、启动和开放。异常仅在旧包/配置/当前schema完全匹配时恢复旧应用；否则保留维护及后续修复记录，不回滚数据库。`:387,512` 校验不可变事件链与准确操作编号，明确向前修复或恢复同结构旧应用。
- `.github/workflows/native-release.yml:3,16,22` 仅手动main、独立原生Runner和固定Action提交。`native_ci_release.py:44,69,96` 检查原生Linux非root、干净源码和无开发身份，创建独立PG、记录实际检查和打包；取消/失败路径只停止核对归属的测试PG。只上传冻结包/无秘密回执，不部署生产。

## 反证与搜索

使用 `rg --files` 定位 `app/deploy/release`、打包/CI脚本和工作流；搜索 `RELEASE_TOOLS|original-cleanup|worker-|health/ready|stage|activate|recover|schema|symlink`，逐一读上述生产入口、callee及异常分支。167曾缺少的 `telegram_config.py` 现由目录`:2`与构建器`:63`共同复制，原清理命令现由目录`:11`映射到真实CLI`:68`，不能再次报告为缺失。安装单元、证书和账号由管理员准备，119设计`:43`及当前手册`:23–25,49–55`明确把实际UID/权限/旧服务归属留作目标验收；未实现自动签证书或任意旧进程接管不构成新增强制代码要求。初始存储元数据及准确Skill安装分别归GAP-20/13，不重复计入本项。

## 文档问题 DOC24-01

`docs/operations/03-native-release-runbook.md:21` 仍写“三份发布工具”，`:45` 仍仅列 `bundle.py/catalog.py/controller.py`。它与 `catalog.py:2` 的四个实际文件及167设计`:10,21`冲突；直接按该旧清单部署会遗漏必需导入。应更新当前操作手册为安装完整 `metadata/release-tools/`（含 `telegram_config.py`），并以实际包清单替代旧91项schema描述。该项是现行文档修正，不是重新开发已存在的复制代码；历史167证明保持原字节。

## 未验证与保留项

未运行测试、构建、原生CI、systemd、Nginx、浏览器或服务；未读取私有配置，也未接触开发PG/API。目标服务器、证书、真实Linux账号/服务目录、生产PG权限、实际启动/就绪/失败处置及四端正式配置一致性仍待用户资料和正式验收。未逐一审查所有Worker业务正确性、模型隔离每条内核分支或第三方包安全性；它们由其他编号处理。本记录不关闭GAP-24，也不添加历史迁移、PITR、容灾或真机门槛。
