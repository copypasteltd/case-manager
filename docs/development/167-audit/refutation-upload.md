# 167 独立反证：续传原件身份

审查日期2026-11-06，固定源码`e36ec01a7b78eed742a7c1f6c831a4114b96bcde`。本审查由账号生命周期审查者另行接受指派，对`upload120gb.md`的U120-D01候选独立寻找反例；没有修改应用、运行测试／构建／服务／浏览器或调用虚拟化环境。只新增本文，不修改原审查、旧证据、旧迁移、冻结产物或验收状态。

## 结论

**未能推翻。普通人工Web／Android上传确实缺少“最初原件完整内容”约束，严格恢复要求下的代码缺口成立。** 这不是SHA-256碰撞，也不是云端完整校验失效：中段根本没有进入普通重选指纹。原报告把结论限定为“无法拒绝所有不同内容原件”、没有宣称云源与正式候选不一致，限定恰当。

原规范`docs/development/03-storage-protocol.md:16`仍写“不同源文件需拒绝恢复”。`docs/development/66-android-resumable-uploads.md:27`明确披露采样只是辅助、不是完整性证明，但这是既有实现的限制说明，没有提供取代原严格约束的新验收条件或用户取消要求。当前`docs/quality/04-current-acceptance-applicability.md:31-32`仍保留重选文件、缺片恢复、源权限失效、有界读取功能，仅移出真机、真实120GB及网络性能专项。严格原件绑定没有因此被取消。

因此应把它列为可开发的功能差异；若产品要正式收缩为“只拒绝采样／元数据不同的来源”，需先明确修订需求与验收，不能从增量66的诚实限制说明推导已经获准降低保证。本轮没有该用户范围变更证据。

## 已尝试的反证及实际结果

| 编号 | 尝试推翻的机制 | 沿调用链读取的证据 | 结果 |
| --- | --- | --- | --- |
| RU-01 | Web可能在初次选择另算完整SHA | `app/web/src/UploadQueue.tsx:140-149`只用`sourceFingerprint(source)`创建普通会话；`app/web/src/upload.ts:32-49`仅对`office-artifact-v1`且≤12MiB的特例整件摘要，普通分支是元数据＋头尾各64KiB。 | 没有普通原件完整摘要。 |
| RU-02 | Web重选后可能比较全部已上传分片 | `UploadQueue.tsx:113-116`替换runtime.source；`upload.ts:114-125`仅比较采样／大小，再resume并形成confirmed集合；`:125-136`只为missing片上传新source.slice；`:148-155`再次对账后finalize。 | 原有云端片不会和新选择原件同偏移字节比较。 |
| RU-03 | Android URI／SAF可能提供强内容版本 | `SAFUploadSource.java:29-40,42-74`读取名称／size／mtime，检查持久只读授权、stat大小、可随机定位；`:43`check只是打开并关闭，不遍历内容。`UploadManager.java:69-80`比较原元数据并重新采样；`UploadSource.java:21-49`仍只有头尾。 | URI读取权限与元数据稳定不能证明所有中段字节不变。 |
| RU-04 | Android重选或结尾检查可能全量读取 | `CPUploadModule.java:113-122`普通重选仅用sample比较后保存新URI；`:179-181`verifySource调用同一fingerprint。`UploadManager.java:66-67`只对office特例改用整件hash；`upload-transfer.ts:120-139`跳过confirmed片、最后verifySource、resume、finalize。 | 重选前后和结尾均重复采样，没有额外完整绑定。 |
| RU-05 | 分片签名／ETag可能绑定原件内容 | `app/backend/internal/files/uploads.go:529-584`签名参数为multipart、片号、长度、TTL，未提供初始原件分片摘要；`:486-520`ListParts核对实际片号、长度、ETag、重复及分页；`:593-626`Resume只比较opaque source_fingerprint并以云结果更新PG片状态。 | 能证明哪些字节片已在云端，不能证明它们均来自同一初始文件。 |
| RU-06 | 服务端完整SHA可能比对原件期望值 | `uploads.go:41-49,326-330,384-387`普通create仅固定source_fingerprint；`upload_jobs.go:311-384`按完整云manifest组装并冻结source；`upload_integrity.go:151-199`对这份冻结云对象完整计算SHA。`:198-199`只有office-artifact-v1才与初始声明完整SHA比较。 | 普通会话没有初始完整SHA供服务端比对，计算的是已经组装的对象。 |
| RU-07 | promotion／commit是否再次阻止混拼 | `upload_promotion.go:189-218`以`current.SHA256`作为复制期望值；`upload_commit.go:46-100`要求提交SHA等于服务端SHA，且候选完整回读SHA与之相等。 | 这些检查保护冻结对象至正式候选的准确字节传递，不能倒推出冻结前所有分片来自原件A。 |
| RU-08 | import或Agent约束是否覆盖普通create | `imports.go:513-542`只有找到file_import_attempts才比较预登记source_sha256；`:527-528`普通上传无关联直接返回nil。`migrations/0051_agent_file_updates.sql:93`等有Agent专用摘要约束。 | 独立导入／Agent路径的强约束不覆盖Web／Android普通人工上传，不应泛化该发现到所有上传。 |
| RU-09 | 人工提交是否等价明确接受更换原件 | `upload_commit.go:57-61,95-100`要求确认的是服务端已验证SHA和固定base；增量66`:40`也只说明确认ready候选SHA。没有预先保存的原件SHA或“明确更换原件”的特殊决定。 | 正式提交仍需人类确认，但这不满足resume前拒绝不同原件的原规范。 |

## 无需执行即可成立的确定性构造

令A与B的名称、大小、mtime、媒体类型相同，文件大于两个采样区。它们头尾各64KiB相同，但中段不同。Web的JSON元数据＋两个样本，以及Android的writeUTF／writeLong元数据＋两个样本，对A、B的输入字节分别完全相同，所以同一平台指纹必然相等，**不涉及攻破SHA-256**。

使用默认32MiB布局的96MiB普通文件即可说明，不需要真实120GB。令B与A在1MiB及33MiB位置各有一处不同，头尾不动。先上传A第1片后暂停，再重选B：

1. Web／Android源检查的输入相同，均可通过；Android的内容提供者返回相同元数据及可读相同大小文档即可，不要求应用获取额外全盘权限。
2. Resume返回A第1片已存在。两端代码跳过该片，只上传B第2、3片。
3. 云端完成对象M为`A[0:32MiB] + B[32MiB:96MiB]`。由于1MiB与33MiB都修改，M既不等于A，也不等于B。
4. 服务端冻结M并完整计算`SHA256(M)`。扫描若正常通过，promotion完整复制M，commit核对的也是`SHA256(M)`；当前没有`SHA256(A)`期望值阻断。

以上是依据已读分支的源码推导，未制造文件、请求云或执行传输。实际环境若缺扫描／存储配置仍可能在其他正确边界阻断；不能用这些无关配置缺失否定普通来源身份比较不足，也不能把此推导记成实测损坏记录。

## 严格要求下的最小完整修复边界

- 对新普通会话保存版本化、不可变的初始完整内容期望摘要；它是调用者声明的约束，不替代服务端完整读后的权威摘要。
- Web与Android均用有界增量读取计算整件摘要，支持取消、进度、准确短／长读检查；首次签片和每次重选续传前比较。不得对120GB用整件ArrayBuffer／base64或整件本地复制。
- 服务端对冻结云对象完整计算后与初始期望摘要比较，不一致不得ready／commit。必须保留这一步，单靠客户端续传检查无法排除读取期间变化或伪报。
- 旧sample会话不能被解释为已经具有初始完整原件证明；追加正常迁移与明确兼容状态，原正式版本及旧证据保持不变。最直接的安全兼容是停止旧会话续传并明确由员工新建严格会话，不能用混拼对象计算结果倒填“原件SHA”。
- 所有代码、契约、平台适配及前置fixtures写完之后才统一验证。小型同采样异中段A／B案例可证明功能边界；本地长偏移／有界读检查不会重新引入已取消的网络性能或实网120GB门槛。

## 搜索与读取记录

只读定位覆盖`rg --files docs`中的03／66协议，`rg --files app/web/src app/mobile`中的上传文件；全后端上传`source_fingerprint`／`expected_sha`／`SHA256`，普通create／Resume／Sign／cloudParts／Finalize／job冻结／fixedSource／RunIntegrityJob／promotion／commit，以及import／Agent特殊路径。逐段实际阅读范围：

```text
app/web/src/upload.ts:1-50,71-155
app/web/src/UploadQueue.tsx:62-80,99-118,140-150
app/mobile/android/app/src/main/java/hk/copypaste/workspace/UploadSource.java:1-51
app/mobile/android/app/src/main/java/hk/copypaste/workspace/SAFUploadSource.java:20-76
app/mobile/android/app/src/main/java/hk/copypaste/workspace/UploadManager.java:33-88,123-134
app/mobile/android/app/src/main/java/hk/copypaste/workspace/CPUploadModule.java:100-128,145-184
app/mobile/android/app/src/main/java/hk/copypaste/workspace/UploadTransfer.java:18-57
app/mobile/src/platform/upload-transfer.ts:70-154
app/backend/internal/files/uploads.go:18-106,308-389,400-435,486-522,529-627
app/backend/internal/files/upload_jobs.go:300-390
app/backend/internal/files/upload_integrity.go:72-108,110-256
app/backend/internal/files/upload_promotion.go:185-224
app/backend/internal/files/upload_commit.go:42-116
app/backend/internal/files/imports.go:513-549
app/backend/migrations/0016_upload_control.sql:33-36
app/backend/migrations/0018_upload_integrity.sql:10-47
docs/development/03-storage-protocol.md:1-40
docs/development/66-android-resumable-uploads.md:19-44
docs/quality/04-current-acceptance-applicability.md:27-37
```

曾以错误的`docs/requirements/03-storage-protocol.md`定位，已通过`rg --files docs`更正为`docs/development/03-storage-protocol.md`；不存在10-web-client／13-mobile-client的错误路径不作为否定证据。超长搜索输出仅用于定位，否定结论依据上列直接计算、跳片、冻结与最终比对代码。

全部23个正式工作包仍开放，真实配置由用户后续提供，本文不要求重新提供，不宣称当前APK、云、真机、真实大文件或生产验收通过。
