# 增量96：原生迁移的准确验证与外部验收边界

[实现与操作](../development/96-native-file-import.md) · [编号差异](../delivery/03-goal-gap-assessment.md) · [原始检查结果](evidence/file-imports/final-source-check.json)

## 1. 新执行的检查

本轮以固定后端来源执行完整原生 Go／PostgreSQL 回归。全部发现的顶层测试必须执行并通过，来源在检查期间不变；最终已通过181顶层／1,163结果、10批；数量和批次以[完整后端记录](evidence/file-imports-backend/result.json)为准，不能把单次专项运行代替完整回归。

| 项目 | 新执行内容 | 证据与限制 |
| --- | --- | --- |
| 迁移协议与来源 | 规范路径、120GB元数据上限、准确字节、链摘要、文件更新基准、缺失映射和被改计划拒绝 | 原生单元检查；不代表实际120GB传输 |
| 本地盘点与文件读取 | 真实合成文件、读前后身份／mtime／大小／SHA、运行中变更、空文件错误行、中止及输出不覆盖 | [真实CLI](evidence/file-imports/native-cli.json)，7项实际进程交互；临时源与输出已清理 |
| 原生API／数据库 | 完整清单后批准、近期MFA、不可变记录、准确来源、权限／目录变化、旧提交重放撤权 | 完整后端覆盖新的10个顶层测试；隔离合成数据 |
| 真实HTTP分片 | 云端实际接收合成字节但丢失成功回执，续跑对账原分片；只产生一个上传和一个逻辑文件 | [原生HTTP往返](evidence/file-imports/native-round-trip.json)；受控对象存储替身，不是真实OSS／R2 |
| 主端与副本 | 原扫描、正式候选、提交、实际受控复制读取／写入；复制前报告待完成，复制后内容证据成立 | 双端报告永不批准切换；报告读取已有核验凭据，不重新下载全库 |
| 失败与增量 | 活跃／正式提交会话不能重建；过期会话明确重试；新内容必须指定原逻辑文件和当前基础版本 | 新内容保留同一逻辑文件，正式版本历史保留 |
| API契约 | 293路径、363操作、635模型；既有路径／模型语义不变，嵌入契约与生成类型一致 | 新增迁移77，原76保持已发布字节 |
| Web／Taro／秘书运行时 | 新执行类型、Web135、移动238、运行时51、构建和draft Skill检查；微信／抖音各20页 | [8项客户端命令](evidence/file-imports/client-checks.json)；没有模型或平台真机执行 |
| Android | 新构建开发APK、当前源码绑定、18项实际包检查；JVM26项缓存／新执行据原任务逐项标识 | [包检查](evidence/file-imports/apk-inspection.json)、[任务范围](evidence/file-imports/android-test-scope.json)，不启动设备或模拟器 |
| 原生二进制 | 当前后端Linux交叉编译；迁移CLI构建Windows／Linux，实际运行Windows离线命令 | [平台Linux构建](evidence/file-imports/linux-build.json)、[CLI构建](evidence/file-imports/native-cli.json)，Linux不执行不部署 |
| 本地开发环境 | 迁移77、准确归属API及通知／报表worker；来源、进程和零隔离库核对 | [运行状态](evidence/file-imports/development-project-resources.json)，真实云／模型／TG关闭 |

当前开发APK为42,940,937字节，SHA256为`30eb371f3f1c7dc0cfd47b712cd879325bce96de04f2984e4394d14e8bc8a97d`，与原95相同。本轮共享契约类型增加，移动业务JS未改变；构建与包检查仍针对当前输入新执行，JVM任务为UP-TO-DATE复用。

## 2. 页面媒体与历史证据复用

本轮没有页面UI改动、浏览器调用或新页面采集。原95的90张PNG、33段MP4、33个GIF保持原字节及采集来源范围；任务讨论截图和两套录屏仍是原95合成Web／Taro H5操作，不改称迁移工具或设备演示。[页面媒体目录](../media/README.md)继续提供实际截图、视频和GIF。

原95源码与证据可从提交 `684d822d4d7e74308b6dc217aa0f972ed1542436` 回溯；原双语入口、台账、基线与迁移方案另存[原始快照清单](evidence/file-imports/publication-before.json)。依赖锁未变，保留原90审计86项（0 critical、20 high、64 moderate、2 low），生产安全门槛未通过；本轮没有重跑审计。

## 3. 尚未执行的真实验收

| 编号 | 未完成验收 | 本轮不能替代它的证据 |
| --- | --- | --- |
| GAP-22 | 真实2TB来源、完整账号／ACL／项目／分类映射、真实全量及增量账实核对 | 小型合成目录和来源键机制 |
| GAP-21／22 | 真实120GB、香港到境内链路、会话续期和中断／恢复、容量／吞吐及云复制 | 元数据边界、少量HTTP字节和受控存储替身 |
| GAP-20／23 | OSS／R2当前对象可用性／版本、真实凭据隔离、恢复及回切 | 数据库内既有副本核验凭据与本地worker流程 |
| 四端首发 | 微信／抖音平台及Android真实设备、签名发行、实际公司业务旅程 | 本地构建、开发APK和原H5录屏 |
| 秘书／通知／运维 | 真实模型、指定TG群及生产部署／监控／恢复 | 运行时测试和关闭外部接入的本机服务 |

报告是有当前权限的逐条观察，可能跨不同时间；文件后续版本或权限变化可以使后续报告不同。所有预期条目内容证据成立仍不等于源ACL完整等价、同时刻备份点或自动切换批准。目标归属账号仅做对照及可读性检查，不会改写实际创建人或自动授予编辑／管理权限。原件保留，由业务与运维独立验收后再决定正式切换。整体Goal保持active。

早期专项夹具与发布记录格式问题见[过程说明](evidence/file-imports/early-check-diagnostics.json)及[发布检查诊断](evidence/file-imports/attempts/02-raw-log-whitespace/diagnosis.json)。这些记录保留原状态；完整后端与最终来源记录是当前验收依据，原始构建日志没有做空白归一化。

## 4. English scope

Increment96 runs fresh native backend, CLI, API/real HTTP, client build/test and development-package checks against recorded sources. The controlled object provider receives actual synthetic bytes; a deliberately missing PUT success receipt is reconciled without creating another session or logical file. Replica evidence remains pending until controlled copy verification completes. Stored per-row receipts do not prove live object availability, source ACL equivalence, an atomic restore point or cutover approval.

The 156 existing page-media files retain their original increment95-or-earlier capture scope. No browser was used or new page media claimed. Real cloud, Hong Kong networking, 120 GB/2 TB, platform/device, model/TG and production restore/cutover acceptance remain open. The unchanged lock keeps the original dependency audit and its failed production security gate.
