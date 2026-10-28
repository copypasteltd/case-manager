# 增量98：依赖维护与四端兼容 / Dependency maintenance and four-client compatibility

[中文总览](../README.zh-CN.md) · [English guide](../README.en.md) · [准确验证范围](../quality/98-dependency-maintenance-verification.md) · [GAP-26](../delivery/03-goal-gap-assessment.md)

## 1. 问题与结果 / Problem and result

既有 Android 主机直接使用 React Native CLI／Metro 构建，移动工作区仍声明了未被该流程调用的 `@tarojs/rn-runner`。此外，Taro Doctor 的 glob 10.x 和旧原生代码生成器的 micromatch 3.x 存在公告记录。仅修改清单不足以证明加载版本已变化，因此本次同时核对锁文件、实际安装树和真实调用行为。

The Android host builds through the React Native CLI and Metro, while the mobile manifest still declared an unused Taro RN runner. The Doctor glob and legacy code-generator matcher also had published findings. This increment verifies the lock, the actual installed consumers and their behavior together.

| 项目 / Item | 处理与边界 / Change and boundary |
| --- | --- |
| `@tarojs/rn-runner` | 移除移动工作区未使用的开发依赖；保留 `@tarojs/rn-supporter`、Taro RN 组件、Metro 和 Android 原生主机。The supported native build remains available. |
| `glob@10` | 根 overrides 固定 `10.5.0`，Taro Doctor／Stylus 实际解析修复版本。The CLI keeps filenames as literal child arguments. |
| `micromatch` | 根 overrides 固定 `4.0.8`；旧 jscodeshift 调用实际使用维护版本，不再保留其嵌套3.x／braces2.x分支。Actual codemod ignore behavior is checked. |
| 框架 / Frameworks | 保留 Taro4.3.1、React18.2.0 和原生主机 React Native0.73.11。No framework downgrade, forced peer install or advisory suppression. |
| 原生边界 / Native boundary | 保留 `.rn` 适配、打包入口和 Expo 原生依赖泄漏检查，不以跳过检查完成构建。Native bundle boundaries remain enforced. |
| 业务 / Business | OA、项目、文件、秘书、身份和状态流程沿用原实现；无迁移或 API 变更。No business schema or API change. |

Primary advisories: [glob CLI command injection and patched10.5.0](https://github.com/advisories/GHSA-5j98-mcp5-4vw2), [micromatch and patched4.0.8](https://github.com/advisories/GHSA-952p-6rrq-rcjv). The glob advisory concerns CLI command execution; it does not establish exploitation of the library API in this application.

## 2. 审计口径 / Audit accounting

本次在同一维护窗口分别对原发布锁与调整后锁执行完整扫描：

```text
npm audit --json --include=dev --include=optional --include=peer
```

| 当前公告源 / Current advisory feed | Critical | High | Moderate | Low | Total |
| --- | ---: | ---: | ---: | ---: | ---: |
| 原97锁，本次新扫描 / Published97 lock, freshly scanned | 0 | 20 | 63 | 8 | 91 |
| 调整后的98锁 / Updated98 lock | 0 | 13 | 60 | 7 | 80 |

数字是 npm 按依赖包汇总的审计记录，包含传播记录，不能当成独立 CVE 数量。原增量90记录为86项（0／20／64／2）；同一旧锁在新的公告源中变为91项，这个差异不计作本次修复。`npm audit` 返回1表示仍有告警；扫描完整结束不等于安全门槛通过。

Counts are npm package findings, including propagated findings, rather than unique CVEs. The older86-finding record remains historical. The current before/after comparison is91→80, with high20→13. Both scans include all dependency scopes; the production security gate stays open.

锁图减少120个安装位置、增加4个位置；版本重定位不等于120个独立包被修复。准确节点变化见[锁图差异](../quality/evidence/dependency-maintenance/lock-graph-delta.json)，完整原始扫描见[更新前](../quality/evidence/dependency-maintenance/audit-before.json)与[更新后](../quality/evidence/dependency-maintenance/audit-after.json)。

## 3. 安装、调用与回归 / Install, consumers and regression

在 `app/` 执行现有安装流程，使用提交的锁文件。禁止通过强制安装、忽略 peer 错误、缩小审计范围或删除告警条目获取通过。

```powershell
Set-Location app
npm ci
npm run test:dependency-security
npm ls glob micromatch braces --all
python scripts/verify_dependency_maintenance_clients.py
python scripts/freeze_dependency_maintenance_clients.py
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python scripts/build_android.py --google-mirror --evidence-group dependency-maintenance
python scripts/inspect_android_apk.py --evidence-group dependency-maintenance
python scripts/freeze_dependency_maintenance_android.py
```

证据目录已存在时应先归档原尝试，再重新执行需要写入的检查；不得用后续结果覆盖失败记录。原生 Android 工具链按[主指南](../README.zh-CN.md)准备，Google Maven 镜像参数仅是本次实际构建的下载源选择，不改变依赖校验。以上流程不启动 Docker、WSL、虚拟机或模拟器。

Tests invoke the Doctor's actual glob CLI with filenames containing spaces, brackets and an ampersand, collect literal child arguments and check that no shell marker is created. A real legacy codegen jscodeshift run transforms selected JavaScript while preserving directory, ignore-file and brace-pattern exclusions. Temporary synthetic files are confined to a verified `.local/` child and removed afterwards. Existing archive/link, CommonJS download, XML and native Babel checks remain.

14项依赖检查与2项原生JS检查之外，还重新执行类型检查、Web135／移动248／运行时51项测试、Web与运行时构建、微信／抖音各20页构建，以及原生 Android 打包及18项包检查。JVM26项结果按实际 Gradle 任务是否执行／缓存单独记录，不预设“新执行”。

本轮实际结果：Metro重新执行完成；Gradle assemble与JVM26均为UP-TO-DATE，APK保留与原97相同字节。18项包检查为本次执行，不将相同APK或缓存测试称为新产物／新测试执行，见[产物范围](../quality/evidence/dependency-maintenance/artifact-summary.json)。

## 4. 剩余风险与推进 / Remaining findings and next work

全部80项保留在[剩余清单](../quality/evidence/dependency-maintenance/remaining-findings.json)，状态均为开放。13项 high 包括旧 CLI 的 git-clone／got／HTTP 缓存链、HTML 压缩器、图像解析器、旧 PostCSS／Vite／Webpack 开发服务及其传播记录。需要进一步核查实际输入、调用链和交付产物，再兼容升级或替换；位于开发依赖不构成自动豁免。

In particular, [image-size parser findings](https://github.com/advisories/GHSA-5p2g-fcmc-qvqq) require a compatible Metro migration: a patched major has a different exported API. This increment does not claim that an untested major override is safe. The remaining findings need bounded reproduction, call-site review, an accepted remediation and affected-client rebuilds before closure.

本次只推进 GAP-26。完整四端业务、真实微信／抖音身份、香港真机网络、真实云／模型／TG、120GB文件、2TB迁移、生产部署和恢复仍按原28项GAP验收；8项Skill保持draft。

## 5. 文档与媒体维护 / Documentation and media maintenance

更新根入口、中英完整指南、索引、台账、差异表与更新记录。界面源码未改，沿用原97 PNG／35 MP4／35 GIF；逐项核对原发布 Git 对象与 SHA-256，不重新标记为98采集或设备验收。后端189顶层／1183结果和 Linux 交叉编译沿用97的相同 Go 源码证据，明确与新执行的客户端检查分开。

The original UI media and unchanged Go evidence remain byte-identical and historical. Fresh dependency/client/Android records have their own exact input hashes. Completed updates continue to commit code, both guides, affected media and change notes together to the authorized repository without force-pushing.
