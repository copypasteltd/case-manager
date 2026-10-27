# 增量90：依赖修复与多端兼容 / Dependency hardening and client compatibility

[中文使用指南](../README.zh-CN.md) · [English guide](../README.en.md) · [差异表](../delivery/03-goal-gap-assessment.md) · [准确验证](../quality/evidence/dependency-hardening/verification.md)

本轮推进 GAP-26，修复实际依赖和构建兼容问题。项目、OA、财务、文件、知识、秘书和状态的业务规则继续采用既有实现；完整四端目标、120 GB／2 TB、真实设备、云、模型、TG、恢复与生产验收继续开放。没有新增迁移、接口或已发布 Skill。

This increment addresses GAP-26 through dependency changes and compatibility checks. Existing business workflows remain available. Full Web/WeChat/Douyin/Android delivery and real device, cloud, model, Telegram, capacity, recovery and production acceptance remain open.

## 1. 审计口径 / Audit basis

| 口径 / Basis | critical | high | moderate | low | 合计 / Total |
| --- | ---: | ---: | ---: | ---: | ---: |
| 原增量65保留审计 / Historical increment65 feed | 11 | 32 | 69 | 2 | 114 |
| 相同已发布89锁文件，本次更新前重新查询 / Fresh feed on unchanged published89 lock | 11 | 30 | 58 | 2 | 101 |
| 本轮实际新锁文件 / Current changed lock | 0 | 20 | 64 | 2 | 86 |

**101 → 86** 才是本轮同次核对的改动前后比较。114 → 101 发生在锁文件未变的情况下，属于公告库变化，不能算修复。包级告警包括传播关系，不能等同独立漏洞数量或可利用路径。moderate 增加也原样保留，不因 critical 归零隐去。

完整 `npm audit --json` 包括开发／构建依赖，没有 `--omit=dev`、忽略公告或调整严重等级。[当前审计](../quality/evidence/dependency-hardening/dependency-audit.json)绑定锁文件摘要和原始报告摘要；[全部剩余项](../quality/evidence/dependency-hardening/remaining-findings.json)保留范围、节点、传播、修复建议和待核对的可达性。`passed` 表示审计命令完整执行，`production_security_gate_passed` 仍为 `false`。

Only the fresh 101-to-86 comparison measures this change. Advisory feed changes account for the earlier 114-to-101 difference. Zero critical findings is not production security approval. Remaining high/moderate findings, including propagated package findings, remain fully visible.

## 2. 实际代码与版本 / Implementation and pins

| 内容 / Area | 当前实现 / Current implementation | 约束 / Reason and constraint |
| --- | --- | --- |
| 构建下载解包 / Archive downloads | 私有 `@copypaste/archive-extraction` 0.1.0，调用维护中的 `@xhmikosr/decompress` **11.1.4** | 原 `decompress` 4.2.1 无修复版本；不复制或继续使用其解析／写入逻辑 |
| CommonJS 入口 / Compatibility boundary | 原调用 `require('decompress')(input, output, options)` 返回 Promise；内部动态导入维护包 | 实际 `download` 调用仍可工作；私有包使用自身名称和版本，不冒充上游修复版本 |
| Swiper | **12.1.2** | 修复公告中的原型污染输入；需要跨主版本后重新验证共享客户端 |
| node-tar | **7.5.22** | 修复旧版本解包／路径相关问题；不声称测试了巨量资源耗尽攻击 |
| webpack | **5.104.1**，移动直接版本与根覆盖一致 | 包含已报告修复；5.112.2 移动了 Taro 内部引用的 `EntryDependency`，实际编译失败后退回兼容版本 |
| 其他明确升级 / Other selected updates | adm-zip 0.6.1；serialize-javascript 7.1.4；lodash-es 4.18.1；node-forge 1.4.1；移动直接 Babel core 7.29.7 | 同时核对真实依赖树，不能只看 `package.json` 声明 |
| 按版本族覆盖 / Scoped overrides | PostCSS 8 → 8.5.32；semver 7 → 7.8.5；http-cache-semantics 4 → 4.3.0；xmldom 0.7 → 0.8.15 | 旧主版本族和其他调用者仍按真实锁文件列出；根 Babel core 7.28.5 仍在剩余项中，不宣称所有同名包均已修复 |

准确的改动前／后节点、下载地址和完整性摘要见 [selected-pins.json](../quality/evidence/dependency-hardening/selected-pins.json)。`app/package.json` 的根级 overrides 约束工作空间；本地入口使用 `$decompress` 引用。安装后运行指定节点的 `npm ls`，已发现并修正 npm 一次安装保留旧版本的问题。

运行要求为 Node **22.14.0 至 23 之前**；本机验证使用 22.14.0，不新增 Node 18／20 支持承诺。Taro 4.3.1 与 React Native 0.73.11 保留现有主版本。没有接受自动审计给出的 Taro 1.x／3.x 降级建议。

The local adapter preserves the existing callable CommonJS/Promise contract while delegating parsing, path/link checks and filesystem writes to the maintained package. Root overrides and the actual resolved tree both matter. A newer webpack was rejected after a real build failure; the compatible patched version is pinned explicitly.

## 3. 有界安全回归 / Bounded security regressions

`npm run test:dependency-security` 实际检查 12 种行为：普通 TAR／GZIP 与 strip；真实 CommonJS `download` 经回环 HTTP 下载并解包 ZIP；输出前缀同名目录穿越；外部硬链接；归档软链接链；已有 Windows junction；map 后路径越界；公告中的 Swiper 原型污染；HTML 安全序列化；实际 Expo plist 使用的 XML 解析／序列化；当前 node-tar 正常解包；node-tar 对父路径及外部硬链接的跳过。

输入、输出和保护哨兵都位于本轮拥有的 `.local/dependency-security-*` 子目录，不访问真实业务文件。结束时检查实际绝对路径与父目录后清理；HTTP 服务在 finally 中关闭。没有运行巨大炸弹文件、长期占用内存／磁盘的攻击，也没有用本地检查替代生产渗透或依赖可达性审查。

页面核实沿用每个组织／用户每15分钟最多8次MFA step-up的限制。检查器实际收到429时等待服务端窗口后只重试身份验证，业务提交继续采用原意图和回执；不清空限流表、不改变时钟、不放宽应用策略。

另外执行 2 项现有原生 Babel JS 兼容检查，以及受影响 Web、秘书运行时、移动端与原生开发包构建。MFA、权限和业务回执的既有测试与真实页面操作继续保留。

## 4. 安装与核对逻辑 / Installation and verification

在原生终端中使用已准备的 Node、Python、Go、PostgreSQL；Android 构建还需要原生 JDK17／SDK34。禁止 Docker、WSL、虚拟机及模拟器。首次环境准备见[使用指南](../README.zh-CN.md)，不要复制他人的 `.local`、凭据或签名密钥。

```powershell
Set-Location app
npm ci
npm run test:dependency-security
npm run test:native-js --workspace @copypaste/mobile
npm audit --json
```

`npm audit` 在存在剩余问题时会返回 1，应保存完整 JSON 并逐项处理，不将其解释为网络命令成功后的安全通过。本轮实际使用 npm install／update 完成版本解析和安装；没有声称对一台全新机器执行过 npm ci。

来源绑定的完整验证入口：

```powershell
python app/scripts/verify_dependency_security.py
python app/scripts/audit_dependencies.py
python app/scripts/verify_web.py dependency-hardening
python app/scripts/verify_agent_runtime.py dependency-hardening
python app/scripts/verify_mobile.py dependency-hardening
python app/scripts/run_dependency_hardening_browser.py --mobile
```

已有同名审计／捕获时应先保留原尝试，脚本拒绝覆盖。页面检查使用原生 PostgreSQL 的独立合成库、真实 API、共享 Taro H5 与有界浏览器。检查后关闭全部自建页面／上下文／临时 API／临时服务，移除私有 fixture。日常开发服务按实际归属单独核对，不停止用户无关进程。

Android 使用既有 `build_android.py --google-mirror --evidence-group dependency-hardening`，指定本地 profile 与回环 API；实际检查开发 APK、包内容及 JVM 结果。缓存 JVM 结果和新执行结果分开记载，不把构建包视为真机或商店通过。

Published results distinguish newly executed checks, exact-source reuse, cached JVM outputs and real-world acceptance. Reproduction requires the native prerequisites. Existing evidence must be archived before a rerun.

## 5. 失败记录和媒体 / Failures and media

最初 webpack 5.112.2 的编译失败及被替代审计、Web／运行时结果保留在 `attempts/resolution-01`。该尝试在保存旧锁文件前发生了工作目录错误，因此仅保留旧锁摘要、报告、日志和其他来源字节，**不宣称保留了完整旧锁原字节**。这一限制明确写入 diagnosis，不重建成伪造证据。最终构建与页面检查使用完整冻结的新锁和来源。

页面检查另有一次触发现有MFA限流的失败，完整来源、结果与采样清单保留，原JPEG帧仅放在忽略目录。另一次页面启动因原生数据库未运行而失败，浏览器尚未打开；`attempts/startup-01` 保存完整当时来源 ZIP。恢复既有原生开发环境后重跑，所有最终结果单独记录。增量89的完整启动／捕获来源从已发布 Git 对象重建，其 103 份证据和媒体保留原字节。

本轮媒体使用独立 `dependency-hardening-*` 文件名，演示实际费用、资金、退款和员工结算流程，以确认依赖变更后页面行为。录制在登录之后进行，不包含密码、MFA 或真实业务。新图片／视频不替换旧版本的来源绑定；Taro H5 录像不等同微信、抖音或 Android 真机。

## 6. 剩余处置 / Remaining work

保留 GAP-26 的 P0 状态，继续处理 Vite／开发服务、旧 Taro／RN 构建链、旧 glob／micromatch／PostCSS 等剩余项。先逐项确认调用者与实际输入边界，再决定兼容补丁、替换或独立框架升级；对没有修复版本或跨主版本的项说明迁移成本和验收矩阵。开发依赖仍能影响构建产物，不能仅凭 `devDependency` 分类豁免。

后续每次改变锁文件，必须重新审计并更新来源、双语说明、CHANGELOG、受影响检查和媒体，提交并推送对应内容。完整四端的真机、云、模型、TG、容量、安全、恢复和生产放行不因本轮结果自动关闭。

上游依据 / Upstream references: [decompress 链式绕过](https://github.com/advisories/GHSA-hrh2-vp3x-79xf)、[Swiper 公告](https://github.com/advisories/GHSA-hmx5-qpq5-p643)、[node-tar 公告](https://github.com/advisories/GHSA-23hp-3jrh-7fpw)、[webpack 公告](https://github.com/advisories/GHSA-8fgc-7cc6-rx7x)、[npm overrides](https://docs.npmjs.com/cli/v11/configuring-npm/package-json/#overrides)。精确当前范围以本轮原始审计和实际锁文件为准。
