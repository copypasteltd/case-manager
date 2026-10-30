# 原生发布操作手册 / Native release runbook

适用于增量119提供的Linux服务器发布工具。当前环境只完成本地检查和构建，目标服务器尚未执行本文。遵循[当前范围](../delivery/04-current-scope-and-acceptance.md)，以及[实现与约束](../development/119-native-production-release.md)。

## 1. 角色与准备

构建Runner使用独立非root账号，不承载业务系统，不保存生产秘密。生产管理员负责确认服务器、域名、证书、持久PG、系统账号、网络范围、已启用功能及版本批准。所有示例中的域名、身份和摘要都是占位，不能原样作为生产配置使用。

| 位置 | 用途及权限 |
|---|---|
| `/opt/copypaste` | root所有、0755；包含冻结版本和公开资源 |
| `/opt/copypaste/releases`、`/opt/copypaste/assets` | root所有、0755，允许服务遍历；已有权限不符时拒绝，由管理员检查 |
| `/opt/copypaste/releases/r-…` | 独立版本，目录0555；程序0555，其他文件0444 |
| `/opt/copypaste/current` | 协调器维护的当前版本链接 |
| `/opt/copypaste/assets` | 只读旧／新Vite资源；不在发布时清空 |
| `/etc/copypaste` | root所有、0755；各秘密子目录另设最小权限 |
| `/etc/copypaste/configurations` | root所有、0700；每个配置目录0700、文件0600 |
| `/etc/copypaste/active` | 协调器维护的配置链接；普通应用账号不能遍历配置目录 |
| `/etc/copypaste/private` | root秘密引用；与包和普通服务账号隔离 |
| `/var/lib/copypaste-release` | root所有、0755，供HTTPS入口确认维护标记；事件子目录0700、操作记录0600 |
| `/usr/local/lib/copypaste-release` | 经管理员核对的三份发布工具，root所有，目录0755、源码0644 |

准备原生systemd、Nginx、Python3、psql以及持久PG18；生产连接使用可信证书和验证主机名。为固定服务目录里的非root用户创建独立系统账号，核对其UID、组与目录权限，不能仅凭账号名称推断隔离已经生效。执行器另外完成[原生隔离安装与验收](../development/105-native-agent-runtime.md)。

首次采用新单元前盘点并停用旧模板／手工启动的同类进程。协调器管理固定 `copypaste-release-*.service`；不能假设它能够识别其他名称或其他管理员启动的进程。禁止把存在未受管旧Worker的环境直接升级数据库。

## 2. 构建和离线检查

在干净代码和已安装依赖的原生构建环境中执行；输出必须为新的 `app/.local/` 子目录：

```text
python app/scripts/build_production_release.py --output app/.local/release-build-UNIQUE
```

输出包含冻结 `copypaste-linux-amd64.tar.gz`、`manifest.sha256`、`build.json`，以及私有编译日志和输入源码归档。本地有未提交变更时会明确记录，不能把它称为干净正式CI产物。

从已授权的可信构建回执取得完整清单SHA-256；不要仅依赖与下载包放在同一不可信位置的摘要文件。该工具提供内容校验，本轮尚未建立正式产物签名签核。

Windows或Linux均可离线检查，不需要root或虚拟化；检查目录必须不存在：

```text
python -B app/deploy/release/bundle.py inspect --archive PACKAGE.tar.gz --manifest-sha256 FULL_SHA256 --output app/.local/inspection-UNIQUE
```

检查清单、Web、运行器、原生程序、91项结构摘要、固定服务模板及工具源码。将核对过的 `bundle.py`、`catalog.py`、`controller.py` 安装到上述root工具目录；后续通过该目录运行。不要以root执行来源未核对、普通账号可更改的发布工具。

## 3. HTTPS和服务单元

包内 `systemd/` 是完整固定单元目录。管理员按已启用功能安装对应单元到 `/etc/systemd/system/`，检查实际系统账号、文件权限、工作与状态目录后执行原生 `systemctl daemon-reload`。未配置的云／TG／模型／清理Worker保持停用。

HTTPS内容由 `catalog.nginx_site(实际域名)` 生成，对应示例在包的 `metadata/nginx.conf.template`。将实际内容放到 `/etc/nginx/conf.d/copypaste-release.conf`，证书位置固定为 `/etc/ssl/copypaste/fullchain.pem` 与 `privkey.pem`。确认全局Nginx包含此文件、没有冲突站点，并由管理员检查／加载。协调器会核对准确内容并执行 `nginx -t`，不会自动改写站点或签发证书。

公网入口为HTTPS；API绑定 `127.0.0.1:8080`。`/health/` 不对公网显示；直传大文件走获准对象存储地址，不经过16MiB的JSON／应用入口。静态资源路径保留旧版本内容，HTML不缓存。维护标记路径及其可遍历权限必须实际验证。

本轮未执行目标Nginx语法、证书、外网HTTPS或就绪联调；这些仍是服务器发布验收项。

## 4. 准备版本化配置

创建一个新配置编号，例如 `company-20261030-a`。目录中至少有以下三份文件；启用通知等其他服务时再增加其固定环境文件，禁止夹带其他文件。

`deployment.json`：

```json
{"format":1,"hostname":"case.example.invalid","units":["api"]}
```

`api.env` 的必要结构示例：

```text
CP_ENV=production
CP_HTTP_ADDR=127.0.0.1:8080
CP_PUBLIC_ORIGIN=https://case.example.invalid
CP_DATABASE_URL=postgresql://copypaste_app:REPLACE_FROM_SECRET_STORE@db.example.invalid/company?sslmode=verify-full
CP_CURSOR_KEY=REPLACE_WITH_RANDOM_SECRET
CP_MFA_KEY=REPLACE_WITH_BASE64_RANDOM_32_BYTES
```

`migration.env`：

```text
CP_MIGRATION_DATABASE_URL=postgresql://copypaste_migrator:REPLACE_FROM_SECRET_STORE@db.example.invalid/company?sslmode=verify-full
```

完整业务、存储和平台配置按[部署总手册](01-deployment-and-operations.md)及对应模块的实际参数补充。不要复制开发数据库、默认账号或真实密码到Git。运行身份与迁移身份使用不同数据库登录；所有运行连接与迁移连接指向同一持久数据库。登录名与数据库名按URI解码比较；查询仅允许TLS项、application_name和connect_timeout，均为唯一非空值。不要用host、port、dbname、user、service或options查询参数另行覆盖连接身份。

服务及环境文件映射以[固定目录](../../app/deploy/release/catalog.py)为准。监控JSON通过PID1凭据传给监控用户，其签名资料使用独立只读权限；模型和证明密钥只属于root broker。启用agent时，候选 `agent-native.json` 要固定本次暂存版本的准确绝对路径、运行依赖图摘要和broker摘要，同时准备另行固定的Linux Node／Codex与库路径。

## 5. 暂存、检查状态和激活

以下命令只在已获批准的原生Linux生产主机执行。首次没有当前版本时使用 `none`；后续必须填 `status` 返回的准确已提交版本。

```text
sudo python3 -B /usr/local/lib/copypaste-release/controller.py stage --archive PACKAGE.tar.gz --manifest-sha256 FULL_SHA256
sudo python3 -B /usr/local/lib/copypaste-release/controller.py status
sudo python3 -B /usr/local/lib/copypaste-release/controller.py activate --release-id r-EXACT_ID --manifest-sha256 FULL_SHA256 --configuration company-20261030-a --expected-current none --migrate
```

没有结构变化时不添加 `--migrate`。先核对候选程序／配置、已安装单元、HTTPS入口、身份与数据库，再执行切换。首次bootstrap管理员通过现有受控入口建立，不在发布包里保存固定密码。

成功后保留操作编号、完整摘要、配置编号、结构清单与实际就绪检查结果，并核对已启用Worker。仅API就绪不等于云复制、模型、TG或八项Skill已经验收。

## 6. 失败、中断与应用回退

先运行 `status`，检查维护状态、指针是否对应已提交版本及准确待处理操作编号。不要反复运行新激活命令，不手工删除维护标记掩盖失败。

```text
sudo python3 -B /usr/local/lib/copypaste-release/controller.py recover --operation-id EXACT_OPERATION_ID --mode forward
```

若实际结构还需要继续已批准的正常升级，明确增加 `--migrate`。若要恢复上一版，使用 `--mode previous`；它要求旧包、旧配置和当前数据库结构完全匹配。数据库已变化时，修复候选并采取明确前向处置，不能启动清单不匹配的旧程序。

已成功切换后，普通应用版本回退使用旧版本的准确包摘要与配置，再执行 `activate`，`--expected-current` 填当前版本；不传 `--migrate`。结构不一致会拒绝，且不恢复历史数据。需要更改程序或配置时使用新版本／新配置编号，不编辑冻结目录。

`status`会核对pending的摘要锚点和完整事件链；事件已落盘、pending尚未更新时采用完整尾部的阶段与目标，但不写入状态。`forward`沿该已记录目标继续，包括上次已记录的previous恢复目标；以状态显示的候选版本为准。临时半写文件不作为正式事件，事件链破损时停止服务操作并由管理员核查，禁止覆盖旧事件。

`events/` 保留操作事件及摘要链；根目录记录当前已提交状态及未完成操作。全部记录只含版本、配置指纹、结构摘要和状态，不包含配置值。保留失败发布和其源码，便于核对来源。

## 7. 原生CI配置

为本仓库准备专用、非root、无生产秘密的原生Linux Runner，标签为 `self-hosted`、`linux`、`x64`、`copypaste-release`。安装声明的Node、Go、Python、原生PG工具；如PG工具不在PATH，以仓库变量 `NATIVE_PG_BIN` 指定其目录。Runner不与生产主机共用账号或工作目录。

人工触发 `Native release build` 的main版本。工作流只构建并上传明确产物，不部署服务器；Node／Go／原生PG检查和源代码回归必须真实完成后才能采纳CI回执。当前工作流代码已提供，本轮没有注册Runner或触发真实作业。

失败作业的原始日志和PG只留在作业私有目录。再次清理工作区前先停止可核对归属的遗留测试PG；若无法确认归属，停止该Runner并由管理员核查，不能删除仍活动数据库目录。本轮不引入容器、模拟器、迁移或容灾任务。
