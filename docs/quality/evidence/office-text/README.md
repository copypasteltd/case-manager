# Office text evidence / Office 正文证据

增量120的当前源码结果与已发布历史证据分别保存。使用[验证矩阵](../../120-office-text-verification.md)查看具体执行、失败尝试、来源摘要与限制；不得仅凭截图、测试计数或开发APK宣布正式上线。

Current source results, retained historical bytes and interrupted attempts are identified separately. See the [verification matrix](../../120-office-text-verification.md) for execution scope, provenance and limitations. No artifact here establishes real cloud/model/device or production acceptance.

- `client-checks.json`及三个build摘要：八项命令的实际输出、原始日志摘要和完整输入。
- `../office-text-backend/`：全部发现后端测试的同源批次、最终结果和私有原始日志清单。
- `schema-upgrade.json`：已发布结构91到92的正常升级，保留既有正式和正文事实。
- `../office-text-web/`、`../office-text-mobile/`：真实HTTP与合成正式版本的五组页面检查、准确正文摘要及截图清单。
- `media.json`、`visual-review.json`：实际页面采样帧、MP4／GIF及七张截图；素材在`docs/media/`。
- `build.json`、`apk-inspection.json`、`android-feature-content.json`：本轮开发APK结构、当前业务bundle及Office功能内容；APK不提交Git。
- `android-test-scope.json`、`native-build.log`、`android-unit-results/`：26项JVM结果及其真实Gradle缓存状态。
- `linux-build.json`：两个当前来源的ELF交叉构建，未执行／部署。
- `development-update.json`、`development-project-resources.json`、`native-cluster-closed.json`：所有权核对、保留事实的开发升级及所属临时资源关闭。
- `reused-evidence.json`、`source-archive.json`、`dependency-audit.json`：316份原媒体与全部旧证据原字节、私有当前源码存档及沿用锁文件的历史依赖证据。
- `final-source-check.json`、`scope-check.json`、`repository-check.json`：最终来源一致性、当前范围与发布检查。

All23 work packages remain active; five exclusions and eight draft Skills remain unchanged. AI fee controls are excluded. Authorization, audit, isolation and technical resource bounds remain required.
