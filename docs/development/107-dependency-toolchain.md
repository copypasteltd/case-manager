# 增量107：开发与模板工具链 / Development and template toolchain

[中文指南](../README.zh-CN.md) · [English guide](../README.en.md) · [当前范围](../delivery/04-current-scope-and-acceptance.md)

本轮推进 GAP-26。Taro4.3.1 的实际 Webpack 路径使用旧版 HTML 压缩器、开发服务器；其微信样式工具及可选 Vite peer 还会安装旧版本。只修改版本清单不足以证明实际解析、同步接口、模板和开发服务器仍正确。当前实现与检查状态以本轮验证记录为准，整个 GAP-26 保持开放。

This increment advances GAP-26. The actual Taro4.3.1 Webpack consumers need synchronous template output and legacy development-server options. The implementation replaces those boundaries with disclosed adapters backed by maintained upstream packages; installed-tree, consumer and client checks must all agree. GAP-26 remains open.

## 1. 实现边界 / Implementation boundaries

| 编号 / ID | 实现 / Implementation | 行为与约束 / Behavior |
|---|---|---|
| TOOL-01 | `app/packages/template-minifier` | 本地包身份为 `@copypaste/template-minifier`。Taro 的 `require('html-minifier').minify()` 同步调用启动有界原生 Node 子进程，由真实 `html-minifier-terser@7.2.0` 解析，不保留旧压缩器源码。 |
| TOOL-02 | 同步边界 / Synchronous boundary | 只支持本项目调用的 `collapseWhitespace`、`keepClosingSlash` 布尔选项；拒绝回调、RegExp、访问器和未知选项，避免序列化时静默丢失。输入最多8 MiB，子进程输出最多16 MiB，15秒技术超时；禁用 shell，错误不回显模板内容。 |
| TOOL-03 | `app/packages/dev-server-compat` | 本地包身份为 `@copypaste/dev-server-compat`，继承真实 `webpack-dev-server@6.1.0`。转换旧 `https`、代理 `context` 和事件选项；冲突、旧 bypass 或中间件钩子必须显式迁移。 |
| TOOL-04 | 本地开发入口 / Local development entry | H5仅作本地检查入口，监听loopback，校验Host，禁止 `allowedHosts: all`；不开浏览器窗口。默认端口10086，可用 `CP_MOBILE_DEV_PORT` 设置1024–65535。该配置不替代生产HTTPS部署。 |
| TOOL-05 | 样式与可选peer / CSS and optional peers | 旧PostCSS消费者统一解析维护版8.5.32；可选Vite／React插件统一到8.3.5／6.1.2。应用继续使用Taro Webpack编译器；不据此宣称提供Taro Vite编译器适配。 |
| TOOL-06 | Node版本 / Node version | 声明Node `>=22.15.0 <23`；本轮使用项目专用22.23.4原生二进制。二进制、依赖和缓存不入Git，不修改全局Node，不使用Docker、WSL、虚拟机或模拟器。 |
| TOOL-07 | 开发模式peer与socket / Development peer and socket | 明确固定React Refresh Webpack插件0.6.3；兼容包的client/socket只转发真实上游的live client，保持同一连接，不复制旧版运行时。真实watch须编译无错误，并检查Host／CORS和页面执行。 |
| TOOL-08 | 原生浏览器 / Native browser | 与仓库既有检查一致，Windows默认使用本机Edge；可用CP_BROWSER_EXECUTABLE指定原生Chromium可执行文件。每次创建独立临时上下文并关闭全部自建页面和进程。 |

同版本Taro React插件在仓库根依赖中明确固定，并为其可选Vite／React peer设置显式override，以避免工作区内的旧peer范围与实际安装版本冲突。此配置继续使用Webpack编译器；必须由完整安装和npm ls验证，不能以安装命令退出0替代。

The same Taro React plugin is pinned at the monorepo root, with explicit overrides for its optional Vite/React peers. The project uses the Webpack compiler. Both a clean install and the installed-tree check must succeed.

上游旧HTML压缩器没有修补版本，见[原始公告](https://github.com/advisories/GHSA-pfq8-rq6v-vf5m)。所选解析器的异步API见[维护者仓库](https://github.com/terser/html-minifier-terser)。开发服务器6的Node、ESM、Express、代理和中间件变化见[官方迁移说明](https://github.com/webpack/webpack-dev-server/blob/main/migration-v6.md)。这些API差异是适配层存在的原因。

The adapters retain their own package names. Their npm aliases preserve the real upstream package identities and versions for auditing. An adapter is not a renamed copy of a vulnerable implementation. Unsupported legacy options fail explicitly instead of changing generated output silently.

## 2. 安装与使用 / Install and use

先准备受支持的Node22版本；Windows本地可使用项目工具目录中的原生Node。下载来源及校验和见 `docs/quality/evidence/dependency-toolchain/native-node.json`。不提交工具本体。以下命令从仓库根目录执行：

```powershell
$env:PATH = (Join-Path (Get-Location) 'app/.tools/node-v22.23.4-win-x64') + ';' + $env:PATH
node --version
Set-Location app
npm ci --no-audit --no-fund
npm run test:dependency-security
npm ls html-minifier html-minifier-terser webpack-dev-server webpack-dev-middleware postcss vite --all
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
$env:CP_MOBILE_DEV_PORT='10086'
npm run build:preview --workspace @copypaste/mobile -- --watch
```

`Ctrl+C` 结束自己启动的开发服务。API地址必须显式设置；普通开发演示不输入真实业务资料。小程序和Android仍沿用各自构建命令，不需要启动H5服务。

Use a supported native Node22 runtime, run a normal `npm ci`, then run the consumer checks and standard client builds. The H5 watch server is a loopback-only review tool. Stop the server and close every owned browser/context after inspection. Production credentials, signing and external integration acceptance remain separate work packages.

## 3. 检查和证据 / Checks and evidence

本轮检查覆盖真实MiniPlugin模板生成、微信／抖音绑定及自闭合标签、中文和预格式空白、无效输入和选项拒绝、真实WXSS命名空间及rpx、原生Webpack资源编译／读取、恶意Host和路径、服务关闭；另启动真实Taro H5 watch并用浏览器读取及重载空登录页，随后关闭自建浏览器和进程。该检查不登录账号，不记录凭据或业务数据，不做网络性能基准。

完整审计包括开发、构建、optional和peer依赖。npm的包级传播记录不等于独立CVE数量。即使本轮高危记录减少，其余记录也须逐项保持开放，不能把开发依赖位置当作豁免。安装失败、旧嵌套解析及检查失败均需保留原始尝试。

产品UI、Go／SQL和原211份媒体保持已发布106的来源及字节；依赖敏感的客户端、Android和开发服务器另做当前检查。复用旧媒体不改写采集时间，JVM缓存不记作本轮新执行，Linux交叉构建不记作目标服务器运行。当前真实云／模型／TG与生产部署仍未验收。

Full audit scope includes development/build, optional and peer dependencies. Every unresolved finding stays in the register with its advisory chain and locations. Historical106 media and unchanged backend evidence retain their original provenance; fresh toolchain-sensitive results are recorded separately. No device, live integration or production readiness is claimed.

## 4. 后续处置边界 / Remaining remediation boundaries

本轮扫描剩余63个包级记录，其中7个包包含直接公告对象，56个包仅传播依赖告警；这不是独立CVE数量。原公告、范围、安装位置和传播关系见[直接公告包](../quality/evidence/dependency-toolchain/direct-advisory-packages.json)与[完整63项](../quality/evidence/dependency-toolchain/remaining-findings.json)。以下全部待处理，不把调用链分析记成修复或风险签核。

| 编号 | 直接公告包 | 后续实现及核验重点 |
|---|---|---|
| NEXT-01 | `@babel/core` | 检查实际Babel／Metro／Taro变换入口和source-map处理，修复后重做构建。 |
| NEXT-02 | `decode-uri-component` | 保留实际URI／query-string调用的返回值、无效编码处理和CommonJS接口。 |
| NEXT-03 | `deepmerge` | 核对实际对象／数组合并语义及原型边界，补React Native CLI配置及SVGR调用回归。 |
| NEXT-04 | `esbuild` | 处理实际变换及预构建调用的API差异；当前关闭预构建不构成安装依赖的豁免。 |
| NEXT-05 | `fast-xml-parser` | 核对React Native CLI Android／iOS配置XML调用、属性与实体处理、非法内容及资源限制。 |
| NEXT-06 | `send` | 核对实际静态文件中间件接口、编码路径、响应及服务关闭。 |
| NEXT-07 | `uuid` | 核对真实构建／SDK消费者使用的生成、解析和模块接口，继续验证Android产物。 |

The remaining63 package findings include7 packages with direct advisory objects and56 propagation-only packages. All remain open. Each fix must preserve its actual consumers and rerun the affected checks; a dependency being optional, cached or used only in development is not an exemption.

安全测试文件按顺序执行，保留全部40项断言；完整套件有900秒技术截止，超时以失败记录并清理本次进程树。压缩工作进程自身仍限15秒。中断尝试及源码快照留档，不能计作通过。

Security files run sequentially with all 40 assertions. The 900-second suite deadline records failure and stops its owned process tree; each minifier worker still has its 15-second limit. Interrupted attempts retain their original inputs and are never accepted as passes.

当前验收边界见[30项AC／72项TC适用性表](../quality/04-current-acceptance-applicability.md)。12项文档一致性检查只确认范围和原始编号，业务验收通过数为0；完整GAP-28仍开放。

公告复核日期：2026-10-29。以下是公告明确给出的修复版本，不表示本轮已安装或通过兼容性验收；选定候选仍要逐一核对真实消费者、正常安装及完整审计。

| 对应编号 | 公告给出的修复边界 | 下一轮验证输入 |
|---|---|---|
| NEXT-01 | Babel7.29.6；[原始公告](https://github.com/advisories/GHSA-4x5r-pxfx-6jf8) | 自建合成source-map路径、常规TS／JS、Metro／Taro变换；不读取真实业务文件。 |
| NEXT-02 | decode-uri-component0.5.0；[原始公告](https://github.com/advisories/GHSA-vcc3-ghjq-m6fr) | 正常Unicode／URL、畸形百分号、query-string及source-map-resolve的实际CommonJS调用。 |
| NEXT-03 | 公告尚无修补版本；[原始公告](https://github.com/advisories/GHSA-gcqx-w48m-w2h4) | 评估明确身份的替代／可审查补丁，核对数组、克隆、回调、特殊键及CLI／SVGR消费者；不靠降级消除告警。 |
| NEXT-04 | esbuild0.25.0；[原始公告](https://github.com/advisories/GHSA-67mh-4wv8-2f99) | Taro配置同步构建、define／alias／loader、target与真实H5编译；本地服务另核对Host／CORS。 |
| NEXT-05 | fast-xml-parser5.7.0；[原始公告](https://github.com/advisories/GHSA-gh4j-gqv2-49f6) | 真实CLI配置XML、属性／实体、注释与CDATA分隔符、正常字节和资源边界。 |
| NEXT-06 | send0.19.0；[原始公告](https://github.com/advisories/GHSA-m6fv-jmcg-4jfg) | Expo CLI实际静态路由、普通响应、编码路径／重定向及连接关闭。 |
| NEXT-07 | uuid11.1.1（公告另列12.0.1／13.0.1）；[原始公告](https://github.com/advisories/GHSA-w5hq-g745-h8pq) | Expo日志／SDK及xcode消费者、旧模块路径和生成接口、v3／v5／v6缓冲区边界。 |

These are advisory patch boundaries, not installed upgrades or completed remediations. Consumer compatibility, a clean installed tree, actual client/native outputs and a fresh full audit are still required. All remaining findings stay open.

React Refresh插件的安装与运行时peer要求见[维护者文档](https://github.com/pmmmwh/react-refresh-webpack-plugin)。这里验证当前源码的实际编译、登录页读取／重载和资源关闭；不把它扩展成完整HMR编辑、移动真机或业务旅程验收。
