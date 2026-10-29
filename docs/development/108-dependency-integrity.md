# 依赖修复与调用完整性 / Dependency fixes and consumer integrity

增量108继续实现GAP-26：处理增量107中仍带直接公告的7个包，并验证它们在Taro、React Native及Expo中的实际调用。完整结果以[本轮验证](../quality/108-dependency-integrity-verification.md)和[原始记录目录](../quality/evidence/dependency-integrity/)为准。依赖版本清单、实际安装树、修复行为和四端产物必须相符。

Increment 108 addresses the seven packages with direct advisories in the previous audit. It verifies the actual Taro, React Native and Expo consumers, rather than treating an updated manifest as proof. The linked verification report distinguishes fresh execution, retained evidence and remaining production work.

## 修复边界 / Changes

| 编号 | 原调用边界 | 实现与保留约定 |
|---|---|---|
| DEP-108-01 | deepmerge：React Native CLI配置、webpack-chain及SVGR声明依赖 | 仓库自维护MIT派生包`@copypaste/deepmerge-safe`，所有3处目标属性写入改为自身数据属性定义，避免目标含`__proto__`时修改结果原型；保留数组、clone、Symbol、特殊值及customMerge接口。 |
| DEP-108-02 | decode-uri-component：旧source-map-resolve／query-string及Taro路由 | `@copypaste/uri-decoder-compat`提供原同步CommonJS函数，委托明确别名的上游0.5.1；同时保留ESM默认导入。核对Unicode、重复参数、加号及长畸形输入。 |
| DEP-108-03 | Babel源码映射读取 | 统一7.29.7；实际调用同时验证包内合法source map可读、包外source map不被读取。 |
| DEP-108-04 | Taro配置esbuild加载 | 统一0.28.2；执行实际`requireWithEsbuild`，核对TypeScript、alias、define和SWC CommonJS输出。 |
| DEP-108-05 | React Native Android／iOS配置XML | fast-xml-parser5.7.0；执行实际Android主Activity和iOS scheme配置读取，核对合法XML、非法XML、CDATA／注释分隔符处理。iOS仅配置解析，不增加iOS客户端。 |
| DEP-108-06 | Expo静态文件中间件 | 旧send0.x统一0.19.2，保留Express5自己的1.x系列。实际HTTP核对普通文件、目录重定向和目录穿越；直接执行真实重定向格式化函数验证转义。 |
| DEP-108-07 | xcode、Expo日志／遥测的UUID | 统一仍提供CommonJS入口的11.1.1；执行xcode标识生成、Expo v4及v3／v5／v6写入边界。不改变React18／React19的既有隔离安装关系。 |

deepmerge在本轮查询时没有上游修复版本。因此这里是**有明确身份、许可证及源代码的仓库补丁**，不是声称上游4.3.1已修复。`app/packages/deepmerge-safe/UPSTREAM.json`保存原版本、原入口SHA256、公告与改动；`LICENSE`保留上游MIT声明。安全测试包括原问题的合成复现与修复后验证，不能用重命名后的审计缺席代替行为验证。

The deepmerge implementation is a repository-maintained patch, not an upstream fixed release. Its upstream entry checksum, advisory, license and three changed write sites are disclosed. Future updates must compare the patch against upstream, rerun the poisoning and compatibility cases, and retain this provenance. The URI adapter delegates to the maintained parser; it does not embed the old implementation.

Taro RN路由会在客户端运行时加载URI适配包。Metro的watchFolders因此显式包含其真实源码目录，保证正常分包与文件摘要计算不依赖node_modules符号链接的偶然遍历。维护版解析器别名仍由根node_modules提供。

## 安装与使用 / Install and use

使用Node `>=22.15.0 <23`。从仓库根目录进入`app/`后执行：

```sh
npm ci --no-audit --no-fund
npm run test:dependency-security
npm run typecheck
npm run test --workspace @copypaste/web
npm run test --workspace @copypaste/mobile
npm run test --workspace @copypaste/agent-runtime
npm run build:web
npm run build:runtime
npm run build:weapp
npm run build:tt
```

标准安装启用安装脚本，不使用`--force`、`--legacy-peer-deps`、审计忽略项或省略依赖范围。提交的锁文件使用可移植的`packages/...`相对链接。npm首次更新曾保留旧嵌套项并产生错误相对链接；独立源码目录只清除候选中的冲突记录，再由普通`npm install --package-lock-only`重新解析，最终仍须通过标准`npm ci`及完整`npm ls --all --json`。

Use an ordinary install with lifecycle scripts enabled. Do not replace it with force, legacy-peer, omitted dependency scopes or direct edits to installed packages. The isolated lock-resolution helper records the exact stale candidate entries removed; the generated lock is accepted only after the normal clean install and actual installed-tree checks.

Windows原生环境的来源验证入口（从仓库根目录执行；同名已完成证据不会被悄悄覆盖）：

```powershell
python app/scripts/verify_dependency_integrity_security.py --label full01
python app/scripts/audit_dependency_integrity.py
python app/scripts/verify_dependency_integrity_clients.py
python app/scripts/freeze_dependency_integrity_clients.py
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python app/scripts/build_android.py --google-mirror --evidence-group dependency-integrity
python app/scripts/inspect_android_apk.py --evidence-group dependency-integrity
python app/scripts/freeze_dependency_integrity_android.py
python app/scripts/verify_dependency_integrity_h5.py
python app/scripts/verify_dependency_integrity_sources.py
```

原生Android构建依赖私有工具目录中的JDK／SDK及项目原生构建路径，不启动模拟器。`--google-mirror`仅使用仓库已有的Gradle依赖镜像配置。正式签名、发布配置及目标生产产物仍按GAP-16／24／26验收。

## 验证边界 / Evidence boundaries

新增16项调用与攻击样本回归和既有40项合并为56项，另保留2项原生JS检查。全部使用隔离的合成资料；Windows不允许尖括号文件名，因此重定向的实际HTTP文件场景使用合法目录，转义场景直接调用真实格式化函数。source-map场景用合成HTTPS名称及内存读取器，不访问外网。

技术期限用于结束卡住的检查，不是网络性能指标；未新增网络性能测试、实网120GB专项、真机／模拟器、历史数据迁移或容灾。AI费用控制继续移出，秘书／Skill／TG、授权、审计、执行隔离及技术资源限制继续保留，公司OA／财务不受影响。

产品界面及Go／SQL业务实现没有在本轮修改。原211份截图／录屏／GIF保留原始字节及采集范围；客户端依赖变化对应的构建、实际调用和本地H5服务另行执行。后端224个顶层测试／1355条结果及两个Linux交叉构建沿用原证据，不计为本轮重跑。

This increment changes dependency execution, not business screens or database schema. Existing UI media and unchanged Go evidence retain their original capture/execution scope. Native Edge checks own an isolated browser context and close that context, browser, temporary Taro process and listening port in cleanup. A local development APK, clean audit or synthetic smoke check does not establish live model/cloud/TG integration or production readiness.

## 公告来源 / Primary advisory sources

- [deepmerge](https://github.com/advisories/GHSA-gcqx-w48m-w2h4)、[decode-uri-component](https://github.com/advisories/GHSA-vcc3-ghjq-m6fr)、[Babel](https://github.com/advisories/GHSA-4x5r-pxfx-6jf8)
- [esbuild](https://github.com/advisories/GHSA-67mh-4wv8-2f99)、[fast-xml-parser](https://github.com/advisories/GHSA-gh4j-gqv2-49f6)、[send](https://github.com/advisories/GHSA-m6fv-jmcg-4jfg)、[uuid](https://github.com/advisories/GHSA-w5hq-g745-h8pq)
