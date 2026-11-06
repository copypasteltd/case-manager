# REQ-UPLOAD-120GB：Web／Android 大文件与受控续传源码审查

审查日期：2026-11-06。固定源码：`e36ec01a7b78eed742a7c1f6c831a4114b96bcde`（增量166）。本报告只审查这一项需求；未运行测试、构建、服务、浏览器、真机、120 GB 传输或网络性能检查，未修改应用、旧迁移、旧证明或冻结产物。遵循 `spec-to-code-compliance/SKILL.md` 的逐项规格／执行路径核查方式，后续由另一审查者独立反证。

## 1. 原始要求与当前适用范围

原始需求摘录：

> “现有原件约 2 TB，最大单文件 120 GB”——`docs/01-requirements-and-scope.md:31`，U-15。
>
> “Web/Android 上传大文件，小程序负责查阅、审批与资料访问”——同文件`:35`，U-19。
>
> “服务器本地只保留临时操作、缓存和临时环境”——同文件`:28`，U-12。
>
> “不同源文件需拒绝恢复。”——`docs/development/03-storage-protocol.md:16`，resume。
>
> “禁止 base64 编码整个文件或读取到 JS ArrayBuffer 全量再上传。”——同文件`:28`，大文件内存约束。

120 GB 按十进制 `120,000,000,000` 字节解释（`docs/01-requirements-and-scope.md:47–48`）。正式文件使用稳定 file／version ID、不可覆盖对象，客户端分片直传、分页云端对账、固定源完整 SHA-256、安全扫描或受控例外、正式候选后独立提交和 OSS＋R2 复制，依据 `docs/04-storage-backup-and-recovery.md:30–41,47–69` 与 `docs/development/03-storage-protocol.md:7–40`。

当前范围明确移出真机和实网120 GB／性能专项，保留上传、续传及120 GB设计上限：`docs/delivery/04-current-scope.json:312–329`。本报告不会恢复这些发布门槛。GAP-18／20 与23个正式工作包继续开放；源码判断不等同云接入或正式验收。

## 2. 判定

**Verdict：partial；源码判断置信度高，整体规格解释置信度中。**

常规120 GB传输的数值范围、有界读流、直传、云端分片对账、固定源与正式候选完整摘要、扫描／受控例外、file／version事务和双云复制入口均有实际代码。没有发现120 GB文件大小被强制转换为32位整数，亦未发现该常规上传链把整个120 GB文件读入内存或复制进服务器本地盘。

**一项可确定的严格规格差异：当前重选检查无法保证拒绝所有不同内容的原件。** 普通Web／Android采用头尾采样，没有绑定初始原件完整内容摘要；因此不足以兑现 `03-storage-protocol.md:16` 的无条件“不同源文件需拒绝恢复”。`docs/development/66-android-resumable-uploads.md:27` 已明确披露采样仅辅助、不是完整性证明，所以本报告不把它描述为隐瞒限制，也不声称破坏了云端已冻结对象的摘要校验。如果当前产品只要求采样辅助重选，这一条应定为规格冲突需裁决；若保持原始严格要求，则存在应用代码补齐工作。

不能判为 absent／contradicted：完整主体链存在且大部分约束一致；不能把 partial 全部归因于未提供云配置：上述原件身份边界可直接从确定的计算与比较路径推出，配置不能增加未读取的中间字节。

## 3. 已落实的实际执行路径

| 编号 | 属性 | 入口与实际执行位置 | 源码判断 |
| --- | --- | --- | --- |
| U120-01 | 120 GB范围与精度 | Web `app/web/src/upload.ts:6–19,32–35`；Go `app/backend/internal/files/uploads.go:314–320`、`internal/objectstore/types.go:12–16`、`protocol.go:21–45`；Android `app/mobile/android/app/src/main/java/hk/copypaste/workspace/UploadLayout.java:8–22` | Web校验安全整数，Go `ParseInt(...,64)`，Android `long`。32 MiB默认布局为3577片；片号的int／int32受≤10000限制，与字节偏移分开。 |
| U120-02 | 持久数据尺寸 | `app/backend/migrations/0004_storage.sql:24,43–45,64,80`，`0018_upload_integrity.sql:12,26,31`；Android `UploadStore.java:18,29,35–36` | 大小、偏移和检查点为bigint／long；AndroidSQLite整数用getLong读取。没有以2 GB为上传总大小上限。 |
| U120-03 | Web有界分片直传 | `app/web/src/UploadQueue.tsx:67–76,103–116,144–149` → `upload.ts:71–94,110–155` | 一项活动上传、每批最多两片；仅传Blob.slice，浏览器直接PUT供应商短期许可。普通指纹只读取至多128 KiB。12 MiB以内Office专用全量arrayBuffer分支在`:36–39`，不扩大到120 GB普通文件。 |
| U120-04 | Android入口与文件授权 | `app/mobile/src/platform/upload-entry.rn.tsx:7` → `pages/uploads/view.rn.tsx:23–24,43,55–63` → `platform/uploads.rn.ts:18–25,48–55,66` → `CPUploadModule.java:80–90,101–123` | SAF只读持久URI；原始URI不提供给JS。已知大小、非虚拟、可随机定位文件才可进入普通上传；不以整件拷贝替代不可定位来源。 |
| U120-05 | Android原生流式与身份约束 | `SAFUploadSource.java:29–39,45–73`；`UploadManager.java:33–34,69–88,123–133`；`CPUploadModule.java:131–175`；`UploadTransfer.java:20–55` | 元数据／分片两个有界单线程队列、一项活动上传、一个在途片；long偏移与contentLength、64 KiB缓冲；分片调用只把≤10000的片号转int。大小通过十进制字符串穿过JS桥。 |
| U120-06 | 小程序范围 | `app/mobile/src/platform/uploads.ts:5–16`，`uploads.rn.ts:66`，`upload-entry.rn.tsx:7` | 微信／抖音适配器明确拒绝本机大文件传输；Android使用RN实现。保留四端目标，没有给小程序虚报120 GB上传能力。 |
| U120-07 | 控制端认证、目标与生命周期 | `app/backend/internal/httpapi/uploads.go:55–83` → `internal/files/uploads.go:197–234,252–277,332–387,400–434`；Android `platform/upload-transfer.ts:29–49,81–118` | 真实当前会话、owner、目录／文件权限、分类、基础版、存储纪元、过期和expected_version是服务端边界；客户端不持云端长期凭据。 |
| U120-08 | 断点对账及缺片补传 | Web `upload.ts:117–152`；Android `upload-transfer.ts:117–137`；Go `uploads.go:486–520,593–626` | 后端完整分页ListParts，校验连续布局所需的片号／大小／ETag、重复及分页推进；事务重置PG确认事实。客户端确认集合不等于云端事实，两端均先对账后补缺片。 |
| U120-09 | 源封存与完整摘要 | `internal/files/upload_jobs.go:311–384`；`upload_integrity.go:176–216`；`internal/objectstore/stream.go:12–46,63–100` | finalize封清单后完成multipart，准确Key／size／ETag固定源；SHA按固定源完整读取，带持久化检查点。显式读缓冲256 KiB，跨端每片流式传输并验证精确读取长度。 |
| U120-10 | 扫描与正式候选 | `internal/files/upload_integrity.go:198–250`；`upload_promotion.go:64–111,189–218`；`internal/objectstore/copy.go:66–127,149–156` | 扫描／独立受控例外绑定原始来源、完整SHA和大小。正式候选另有服务端独占Key，multipart推广后完整回读，SHA不符不得ready。 |
| U120-11 | 正式身份与双云 | `internal/files/upload_commit.go:46–124,127–172,187–199` | commit再验ready、完整摘要、准确候选、base和双云配置；事务创建file_version、更新head、主副本verified／另一副本queued与事件。复制中不会被标作双云完成。 |
| U120-12 | 云端字节和本地边界 | `internal/storageconfig/config.go:19–20,72–73`；`internal/objectstore/oss.go:127–161`、`r2.go:136–169`、`stream.go:17–33,74–97` | OSS／R2 Range＋If-Match，OSS可绑定原生版本；R2不伪造版本号。正式字节走供应商Store／multipart流，不在本上传／推广路径落盘。对整个项目所有其他派生处理不作超出本报告范围的存储证明。 |

上表Android类名均位于 `app/mobile/android/app/src/main/java/hk/copypaste/workspace/`；后端 `internal/...` 均相对 `app/backend/`。测试源码未执行；现有低容量／长偏移测试不能充当本次运行证据。当前开发APK的真实来源另按原冻结清单记录，不将审查HEAD视为新APK已运行。

## 4. U120-D01：采样重选不等于完整原件身份

**确定事实与逻辑反例，未运行重现。**

1. 取两个普通二进制文件A／B：同name、size、mtime、媒体类型，头尾各64 KiB相同；文件中段某个字节不同。可以是两个不同目录下的同名文件，修改时间相同。此反例不要求攻击云对象，也不需要SHA-256碰撞。
2. Web `upload.ts:41–49` 对两者计算的输入完全相同；Android `UploadSource.java:29–47` 也只编码元数据和头尾，输入同样完全相同。Android `CPUploadModule.java:114–121` 和 `UploadManager.java:69–80` 的重选／开始检查遂不能区分A／B。
3. 若A已有部分云端分片，普通resume只比较同一个采样字符串（`uploads.go:598–599`）。ListParts确认后，Web `upload.ts:124–128`、Android `upload-transfer.ts:120–122` 保留旧片，缺片读取重选的B。选择A／B两个中段差异落在不同分片，可使组装结果既不是A也不是B。
4. `upload_integrity.go:176–198` 计算组装后的固定云源完整SHA；与预先确定的原件SHA比对仅存在于`office-artifact-v1`有界专用分支（`:198–199`）。普通上传没有这一比较。随后候选回读／commit精确核对的是该组装结果的权威SHA，而非“初始所选A的SHA”。

因此现有机制能拒绝采样不同、大小不同、目标／纪元变化的续传，也能验证云源到正式候选的精确字节一致性；它不能无条件证明续传仍是最初原件。报告没有声称所有断点续传均失败、发生了实际业务损坏，或普通员工能绕过文件／版本权限。

**可行的完整修复边界（本次未实现）：**

- 为普通上传建立版本化、不可变的 `expected_source_sha256`／摘要算法；作为调用者声明的预期内容约束，不能直接当作已验证云内容。与owner、目标、size、布局、纪元及原创建意图一同固定。新增迁移，不重写旧迁移、旧报告或冻结产物。
- Web分段增量计算整份SHA-256，Android用long长度和固定缓冲完整读取；均支持取消、独立摘要进度、严格短／长读检查，不整份arrayBuffer、base64或临时拷贝。完整读取是本机功能步骤，不是网络性能验收。
- 每次重新选择／恢复先核对完整预期摘要。摘要不符在签新片前停止；服务端完整读冻结云源后还必须再次与预期摘要比较，不符拒绝扫描推广／正式提交，防止摘要后原件变化或客户端伪报造成混拼。
- 旧采样会话不得自动升级为已有完整原件证明。制定明确兼容路径，例如保留旧会话状态并要求新建严格会话；如要允许人工绑定升级，必须单独设计准确原件核对、权限和不可变决定，不能从现有混拼对象倒推原件事实。已经提交的旧版本保持不变。
- 整批代码完成后，用小型A／B同采样、不同中段用例同时覆盖Web／native／HTTP链；含长偏移但无需真实120 GB数据，另查取消、原件中途变化、旧会话兼容、摘要冲突和commit阻断。不要将这些待写用例记成已通过。

## 5. 与代码差异分开的前置条件／未核实事项

| 编号 | 状态 | 具体边界 |
| --- | --- | --- |
| U120-C01 | 公司制度／配置前置，非新发现的漏实现 | 现有ClamAV适配器 `internal/filescan/clamd.go:50,80,190–193` 上限2,000,000,000字节；120 GB会得到`unscannable/SCAN_SIZE_LIMIT`。`upload_promotion.go:76–104` 已有准确来源、公司政策、独立审核、MFA、风险确认及有效期的例外通道。必须有被公司采纳的扫描／例外方案；不能把未配置时的阻断叫作120 GB已可正式使用，也不能删掉扫描边界换取“上传成功”。其他大文件扫描器不是已实现能力。 |
| U120-C02 | 真实配置／正式接入仍开放 | OSS／R2桶、角色权限、CORS／ETag暴露、独占不可变Key前提、数据库配额、迁移及native worker运行配置未由本审查验证。供应商SDK内部、实际权限隔离和真实云响应未运行，不能从适配器源码推导生产环境通过。 |
| U120-C03 | 当前允许的未验证运行条件 | Web浏览器文件API及Android SAF提供商是否保留可随机读取的120 GB文件、长时间前台服务是否被终止，只有源码限制和既有设计说明；此轮不补真机或网络性能专项。提供商自身可能下载／缓存原件（`66-android-resumable-uploads.md:21`），不等同应用主动整件缓存。 |
| U120-C04 | 限定审查，不作全项目无缓存断言 | `GeneratedUploadSource.java:26–42` 存在有界Office专用生成文件与base64桥；它不是普通120 GB上传路径，输入另受16 MiB编码及专用大小约束。扫描／推广的显式流边界已读，依赖库内部和其他独立派生处理本次未穷尽。 |

## 6. 搜索记录与阅读范围

以下均是只读源码搜索／阅读；退出码1的无命中与路径误用区分记录，未将工具错误当作不存在证明。

| 搜索 | 已检查命中／修正 |
| --- | --- |
| `rg --files docs -g '*requirement*' -g '*scope*' -g '*storage*' -g '*upload*'` | 定位初始规格、当前范围、03／04／10／13／18／66存储及客户端说明。读取原始要求条目、上传流程、内存、恢复和当前排除条目；历史性能／迁移要求受当前范围覆盖。 |
| `rg --files app -g '*.java' -g '*.kt' -g 'AGENTS.md'` | 定位实际tracked Android Java宿主，无额外Kotlin上传实现。遵循当前根AGENTS；未启动Android工具。 |
| `rg -n '120|size_bytes|part_size|bigint|MaxObjectBytes|int64|int32' ...` | 已读上表尺寸、模型、SQL、协议与适配器位置。PowerShell把`docs/01*`／`migrations/0014*`当字面参数的错误调用随后改为目录＋`-g`；这些失败本身不构成缺失结论。 |
| `rg -n 'sourceFingerprint|fingerprint|SourceFingerprint|source_fingerprint|expected_sha|SHA256'`，限定Web upload、Android上传类和后端`upload*.go` | 命中普通头尾采样、Office完整SHA特例、服务端冻结源／候选完整摘要、commit摘要校对；U120-D01以这些实际计算／比较路径为依据。没有把简单命名搜索无命中单独当作缺失证明。 |
| `rg -n 'ReadAll|ReadFile|CreateTemp|TempFile|WriteFile|os.Create|MkdirTemp|int32\(|int\(' app/backend/internal/files -g 'upload*.go' -g '!**/*test.go'` | 返回的`checkpoint`为正则`int\(`在标识符结尾的误命中，人工阅读排除；没有整件读盘／写盘命中。objectstore的int32命中均为有限片号／分页标记，已读长度为int64的实际调用。 |
| Android上传类搜索`getInt(size...)`、`(int)size/offset/length`、`new byte[...size]`、`ByteArrayOutputStream`、`FileOutputStream`、`Base64`、`readAllBytes`等 | 没有普通大小／偏移32位转换或按整件size分配缓冲命中；发现并单独阅读GeneratedUploadSource专用Office分支，未误报为120 GB缓存。`UploadTransfer.java:38`按≤64 KiB安全缩窄读计数。 |
| HTTP、前端／native调用搜索 | 已沿`UploadQueue`／Android上传页→协调器→原生桥／流→HTTP→UploadService→job冻结／摘要／扫描→promotion→commit→复制事件追踪。小程序unsupported路径单列。 |

重点逐段阅读范围：`upload.ts:1–157`；`UploadQueue.tsx:20–76,103–118,140–152`；`upload-transfer.ts:29–49,78–151`；`uploads.rn.ts:18–25,48–69`；`uploads.ts:1–16`；`UploadLayout.java:1–24`；`UploadSource.java:1–51`；`SAFUploadSource.java:26–76`；`UploadTransfer.java:1–63`；`UploadManager.java:26–96,121–135`；`CPUploadModule.java:65–90,101–181,190–198`；`UploadStore.java:14–36,53–60`；`GeneratedUploadSource.java:23–44`；后端`uploads.go:197–277,308–396,486–520,593–627`；`upload_jobs.go:300–390`；`upload_integrity.go:168–256`；`upload_promotion.go:65–112,186–222`；`upload_commit.go:46–116,127–200`；`objectstore/protocol.go:21–47,154–167`、`stream.go:1–101`、`oss.go:127–161`、`r2.go:136–169`、`copy.go:62–137,149–156`及上表已列配置、SQL、调用入口行。长文件搜索输出未展示的片段不计作已逐行审阅。
