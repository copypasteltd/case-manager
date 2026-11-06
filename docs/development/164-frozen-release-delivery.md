# 增量164：当前同版冻结交付 / Matching frozen delivery

业务163／schema129固定于 `c72739820534cd178ee0a67b0fe8cebb7589c6d8`，版本 `v0.1.0-rc.9`。本次完整代码波次前置于163，统一测试与集中修复完成后构建当前Linux服务器、Web／秘书运行时、微信／抖音、Android开发APK、完整应用源码和全部公开媒体。共五类六文件；八个旧冻结、所有旧证据／媒体及1–129升级字节保持不变。

| 编号 | 实际交付 / Actual delivery | 边界 / Boundary |
| --- | --- | --- |
| R01 | 原生Linux amd64三个Go程序＋Web＋runtime，1537构建输入逐文件核对固定提交 | Windows原生交叉构建；实际Linux部署待提供配置 |
| R02 | 微信／抖音当前源码构建，157个小程序包文件 | 本地API配置；平台身份／发布待提供 |
| R03 | 新Android JS／开发APK，实际图223模块、21注册页面，APK检查10项通过 | 开发签名；正式密钥／HTTPS配置待提供；不做真机 |
| R04 | 全部应用源文件、依赖锁及文档ZIP；全部历史公开媒体ZIP分两卷 | 每卷<=90MiB，全文与每卷SHA256、重组工具，字节无删减 |
| R05 | 独立产物分支与不可变源码标签；[具体下载及使用](../releases/v0.1.0-rc.9/README.md) | 已授权SSH交付；不重试原GitHub Releases API403 |

All business changes155–163 are in this matching candidate. Packaging inspections are fresh; selected163 business/logic/browser/media evidence remains bound to its exact inputs. The28 JVM XML records are the actual UP-TO-DATE Gradle result, not newly executed tests. Source3838 packaged Git blobs and5788 expanded artifact members are checked. Formal cloud/model/platform/Linux/signing/production acceptance and all23 packages remain open; all eight Skills remain draft.

首个包检查按旧20页面断言失败，实际21页面全在图中。保留inspection-initial原失败；仅修正main164检查器及证明对照逻辑，明确固定前后摘要，完整163源码ZIP保留原字节。当前业务摘要只对这两份打包工具使用冻结原件重算，其他所有源码仍须准确相等；这不允许应用输入漂移，不重跑无影响构建或业务测试。

使用前核对外层SHA256、固定源码及服务器内层清单。真实部署仍需PG、OSS＋R2、HTTPS、原生身份、Codex、TG与小程序配置，按用户答复后续提供。目标环境执行正常101–129升级；本机开发PG100／现有API没有变更。正式Android打包工程继续等待公司密钥。设备、网络／实网120GB、历史迁移／容灾和AI费用控制不重新引入。

媒体原包约143.33MiB，Git外保留完整原件，交付90MiB及剩余分卷。封存前ZIP逐成员核对固定Git字节，分卷后顺序累计全文SHA256复核；原生重组工具额外7项检查通过。已有冻结不重分卷或改写。

[实际验证与保留失败](../quality/164-frozen-release-verification.md) · [完整23编号业务处置](../delivery/05-functional-closure-163.md) · [本轮媒体沿用准确163输入](../media/batch-round-163/README.md)

## 原生获取示例 / Native fetch example

在空目录按以下命令只取当前交付工具和产物；下载文件后先核对SHA256SUMS。该操作供接收者执行，没有在本机额外拉取或启动服务：

```powershell
git clone --depth 1 --filter=blob:none --sparse --branch codex/release-v0.1.0-rc.9 git@github.com:copypasteltd/case-manager.git case-manager-rc9
cd case-manager-rc9
git sparse-checkout set app/scripts docs/releases/v0.1.0-rc.9
python app/scripts/release_archive_parts164.py --manifest docs/releases/v0.1.0-rc.9/release-manifest.json --directory docs/releases/v0.1.0-rc.9 --output case-manager-v0.1.0-rc.9-media.zip
```

若通过网页下载，把manifest与两卷放在同一目录，将工具的manifest／directory改成那个实际目录即可；output须为尚不存在的ZIP。服务器包按内层manifest核对后在实际Linux按部署指南配置；平台与APK实际访问配置另行提供。公开下载需要仓库访问权限。
