# 增量92准确验证 / Exact verification

本轮实现统一搜索及固定版本正文提取／索引／预览。以下均区分当前来源、本地合成资料、历史原字节及未完成的正式验收；不以测试数量代表整体完成率。

This record binds the actual search implementation to current source bytes, native checks and authenticated browser pixels. Synthetic objects use controlled HTTP/object fixtures; OSS/R2, models and Telegram were not contacted.

| 范围 / Scope | 结果 / Result | 来源与限制 / Evidence and limits |
| --- | --- | --- |
| 后端 / Backend | 165顶层／1,112结果／1,012末级，9批全部通过 | [result.json](result.json)、[batch-progress.json](batch-progress.json)；原生PostgreSQL完整发现执行，非一个Go进程 |
| 格式与链路 / Text flow | 20个解析子场景、9个HTTP／数据库子场景、独立只读凭据检查 | UTF-8、JSON、DOCX、非法XML／DTD／重复根／路径／膨胀／深度／取消；当前版本、ACL、生命周期、已发布知识、精确游标、损坏对象、IO期间来源变更、重试／租约、终态不可变 |
| Web | 126测试＋构建 | [web-build.json](web-build.json)，完整声明来源与执行期间来源一致 |
| 秘书运行时 / Runtime | 49测试＋构建 | [runtime-build.json](runtime-build.json)，八项Skill继续draft，本轮未增加秘书索引工具或真实模型运行 |
| 移动 / Mobile | 218测试、类型、微信／抖音各19页编译 | [mobile-build.json](mobile-build.json)，共享搜索响应验证实际加入测试清单；编译不等于小程序真机验收 |
| Android | 原生开发APK、18项包检查 | [build.json](build.json)、[apk-inspection.json](apk-inspection.json)；无模拟器／虚拟化，无真机／签名发布／商店验收 |
| JVM | 26结果，4份XML；本次Gradle任务UP-TO-DATE | [android-test-scope.json](android-test-scope.json)、[冻结日志](native-build.log)、[XML](android-unit-results/)；缓存复用，不称新执行26测试 |
| Linux | 当前后端原生交叉编译通过 | [linux-build.json](linux-build.json)；没有执行或部署产物 |
| Web／移动H5 | 各12项真实UI／API检查 | [Web记录](../file-search-web/result.json)、[移动记录](../file-search-mobile/result.json)；各一个录制视口，不声称三宽度或实体设备检查 |
| 媒体 / Media | 4PNG＋2MP4＋2GIF | [媒体校验](media.json)、[人工视觉复核](visual-review.json)；实际已登录合成页面，Web14帧，原时序较快，可放慢观看；录制不含登录密码／MFA |
| 开发环境 / Development | 75迁移、实际归属API／两worker、0隔离库，外部能力关闭 | [原生状态](development-runtime.json)、[资源与约束](development-project-resources.json)、[更新过程](development-update.json)；新增索引worker未接真实云凭据 |
| 历史 / History | 原91来源完整重建、164份证据／媒体保留原字节 | [historical-91.json](historical-91.json)，cb13bd3；另保留10份已发布入口[原快照](publication-before.json) |
| 依赖 / Dependencies | 相同锁复用原90审计86项，20 high | [原审计](../dependency-hardening/dependency-audit.json)；没有重新审计，没有生产安全放行 |

APK字节数：42,927,073；SHA-256：`1c77427a429d1f51818e14ad5380a40afc447677b3b0f39ba2fed29bce81418b`。二进制与签名密钥保留在被忽略的本地路径，不提交Git；记录中的不可变产物路径可用于本机核对。当前完整目录为74PNG／27MP4／27GIF。

## 实际浏览器检查 / Actual browser checks

通过原导航进入搜索，读取真实API的当前正文匹配、版本／索引时间／摘要，打开所选正式版本并前后翻页；确认历史正文不成为当前搜索命中，DOCX正文可查询，PDF名称仍可检索并显示不支持正文，百分号／下划线／反斜杠按字面查询；另检验历史版本明确预览及提取器／全文哈希／后续偏移。服务端另验证非编辑者、权限变更、回收站、隔离、过期游标和租约栅栏。均使用隔离合成业务与受控对象服务，不将其写作真实云验收。

Actual Web/H5 flows navigate, search, preview and page through exact version text. Unsupported formats remain explicit. UI response observations and backend authority/lifecycle/fencing cases have different scope. Both owned browser contexts and review servers closed; native fixtures exited and databases were dropped. Daily development services remain running.

## 保留尝试与限制 / Retained attempts and limits

- [01](attempts/01-fixture-start.json)：首个合成fixture缺少已注册管理员MFA，无法启动；保留日志／当时测试文件哈希及清理说明，没有完整旧来源冻结，不能当通过证明。
- [02](attempts/02-web-checker/archive.json)：Web检查器遗漏body选择器，失败；保留结果、准确来源ZIP与逐文件哈希。
- [03](attempts/03-web-before-layout-polish/attempt.json)：通过的早期Web在布局改进前被明确替代；完整来源ZIP、媒体、构建记录保留。移动当时212项，不用于本轮218项结论。
- [04](attempts/04-build-source-change/diagnostic.json)：先前归档／修改命令尚未结束便启动两端构建；命令通过但来源变化，验证脚本正确拒绝。未重建所有中间字节，只作诊断。
- [05](attempts/05-web-before-mobile-checker-fix/attempt.json)：通过的Web因共享移动检查器修正被替代；完整来源／媒体／结果保留。
- [06](attempts/06-mobile-button/attempt.json)：移动检查器期待了页面没有的箭头按钮后缀，未处理Promise退出；保留完整准确来源和失败日志哈希；清理fixture／服务，额外核对归属Edge进程为0。
- [07](attempts/07-development-with-active-fixture/diagnostic.json)：只读开发检查遇到自身仍运行的隔离浏览器库而拒绝；没有应用失败，浏览器结束后重新核对通过。

- [08](attempts/08-historical-document-links/diagnostic.json)：仓库链接检查把原入口快照当成当前指南；原字节不变，改用`.snapshot`扩展并在清单保留原逻辑路径，重新检查。

Earlier ad-hoc fixture corrections before the recorded full backend run do not have complete pre-edit source archives. Final claims rely on the stable full run and current browser/build records. Limited diagnostics are not presented as reconstructed passing evidence.

## 复核与剩余验收 / Recheck and remaining gates

在仓库根目录运行 `python app/scripts/verify_file_search_sources.py`，核对[最终来源](final-source-check.json)；候选／暂存仓库分别运行 `python app/scripts/verify_repository_publication.py` 与 `--staged`。原生重跑命令见[设计](../../../development/92-file-text-search.md)。不调用Docker、WSL、虚拟机或模拟器，浏览器检查须关闭所有自建资源。

仍需实际独立云账号／IAM／生命周期、中文和复杂文档样本、2TB索引性能、120GB传输、网络中断／恢复、四端实体设备、模型／TG、生产密钥、外部备份恢复及安全放行；PDF／OCR／其他Office格式尚未实现。统一搜索目前供人使用，不能推导秘书已能消费新索引。继续维护[28项稳定GAP](../../../delivery/03-goal-gap-assessment.md)，完整Goal未完成。
