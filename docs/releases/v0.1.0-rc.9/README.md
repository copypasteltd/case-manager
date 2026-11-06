# v0.1.0-rc.9 · 当前同版冻结 / Matching frozen delivery

业务163／schema129固定于 `c72739820534cd178ee0a67b0fe8cebb7589c6d8`。本版包含155–163全部业务代码：Office新建／编辑与恢复、Skill发布治理、TG固定授权与披露、文件原登录及准确结果／归档／副本入口。八个旧冻结与全部旧证据保持不变；本次真实配置由用户后续提供。

All application changes through163 are fixed at this source. Native Linux server/Web/runtime, WeChat/Douyin builds and a fresh development Android APK match it. Five artifact categories use six downloads because complete historical media is losslessly split. Formal signing, actual cloud/model/platform/production acceptance and all23 packages remain open.

## 固定下载 / Fixed downloads

| 编号 / No. | 文件 / Artifact | MiB |
| --- | --- | --- |
| 1 | [case-manager-v0.1.0-rc.9-linux-amd64.tar.gz](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.9/docs/releases/v0.1.0-rc.9/case-manager-v0.1.0-rc.9-linux-amd64.tar.gz?raw=true) | 30.49 |
| 2 | [case-manager-v0.1.0-rc.9-miniapps.zip](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.9/docs/releases/v0.1.0-rc.9/case-manager-v0.1.0-rc.9-miniapps.zip?raw=true) | 1.08 |
| 3 | [case-manager-v0.1.0-rc.9-android-debug.apk](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.9/docs/releases/v0.1.0-rc.9/case-manager-v0.1.0-rc.9-android-debug.apk?raw=true) | 41.97 |
| 4 | [case-manager-v0.1.0-rc.9-source.zip](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.9/docs/releases/v0.1.0-rc.9/case-manager-v0.1.0-rc.9-source.zip?raw=true) | 11.12 |
| 5 | [case-manager-v0.1.0-rc.9-media.zip.001](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.9/docs/releases/v0.1.0-rc.9/case-manager-v0.1.0-rc.9-media.zip.001?raw=true) | 90.00 |
| 6 | [case-manager-v0.1.0-rc.9-media.zip.002](https://github.com/copypasteltd/case-manager/blob/codex/release-v0.1.0-rc.9/docs/releases/v0.1.0-rc.9/case-manager-v0.1.0-rc.9-media.zip.002?raw=true) | 53.33 |

[逐文件SHA256 / Checksums](SHA256SUMS.txt) · [完整清单 / Manifest](release-manifest.json) · [服务器构建记录 / Server build](server-build.json)

## 获取、核对与启动 / Fetch, verify and use

1. 通过上表下载文件，或用原生Git获取产物分支；逐一核对SHA256SUMS与manifest。 Source tag `v0.1.0-rc.9` fixes business163. Artifact branch `codex/release-v0.1.0-rc.9` contains the exact sealed bytes.
2. 服务器包内包含Web、秘书运行时、三个原生Go程序、全部129条正常升级及原生部署脚本。先配置真实PG、HTTPS、OSS＋R2、运行身份及Codex／TG，再按部署指南安装与执行正常升级；本机开发PG100／API没有变更。没有在本机启用Linux虚拟环境。
3. 小程序和APK使用本地开发配置。实际访问需指定真实HTTPS／平台身份后重新构建；APK为开发签名，正式密钥配置后使用既有正式APK／AAB工程。真机验证按用户要求不列入本轮门槛。
4. 下载源码ZIP可取得完整应用、依赖锁与文档；历史验证证据保留在源码标签Git中，所有公开媒体由以下两卷完整提供。
5. 在秘书会话明确选定文件／目录，原登录失效后按准确保存草稿重新授权恢复。归档与副本入口固定准确版本／副本，继续核对普通权限。OA、项目、文件、财务及监控操作指南见[当前主文档](https://github.com/copypasteltd/case-manager/blob/main/docs/README.zh-CN.md)和[English guide](https://github.com/copypasteltd/case-manager/blob/main/docs/README.en.md)。

## 重组全部媒体 / Reassemble all media

两卷都必须下载到同一目录，卷顺序、每卷摘要和全文摘要固定在manifest。整包约143.33MiB，每卷最多90MiB；不要单独解压分卷。使用源码中原生Python工具，输出文件必须尚不存在：

```powershell
python app/scripts/release_archive_parts164.py --manifest docs/releases/v0.1.0-rc.9/release-manifest.json --directory docs/releases/v0.1.0-rc.9 --output case-manager-v0.1.0-rc.9-media.zip
```

The helper verifies every part and the ordered full SHA256 before opening the output, refuses missing/corrupt/reordered/path-escaping parts and preserves existing output. It uses ordinary native files, with no Docker/WSL/emulator. Full archive SHA256: `448cd72408616890885cce5d3a5d066b2da706dafd8005c5602894f9e652933d`.

## 证据范围 / Evidence boundaries

- 新鲜产物：Linux三个Go程序、Web／运行时、两小程序、当前Android JS／开发APK；实际APK检查10项、223个当前模块及全部21注册页面，两小程序标记检查、完整源码／媒体与私有材料检查。
- 163业务／媒体保留：native两组10条测试＋1包，Web527／共享移动553／runtime66／八项draft Skills、真实本地API／PG的Web和H5录制。没有重新执行完整业务回归。
- 原生JVM：28个XML来自实际 `:app:testDebugUnitTest UP-TO-DATE`，并非本次重新运行28项测试。正式Android签名／平台／真实服务／生产验收不由开发包替代。
- 首次包检查错误要求20页面，实际21页面全部在图中。仅main164的检查／证明工具修正该断言，保留首次失败与原／新工具摘要；冻结源码ZIP保持163原始字节，运行检查采用main当前工具。[精确说明](https://github.com/copypasteltd/case-manager/blob/main/docs/quality/164-frozen-release-verification.md)。
- 旧GitHub Releases API403不重试，本版通过授权SSH标签和独立产物分支交付；没有声称创建网页Release或部署真实服务器。排除项仍为真机、网络／实网120GB性能、历史迁移／容灾与AI费用控制。
