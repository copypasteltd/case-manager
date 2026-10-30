# 文件、目录、版本、检索与复制需求

增量128实际代码补充：PDF、扫描PDF、PNG及JPEG按准确正式版本生成有界派生正文；PDF每页内嵌文字与OCR分别标注，所有页成功后发布，完整页数和引擎摘要固定。四端搜索、预览、比较和秘书资料复用同一证据。8 MiB／20页／2 MiB为当前提取边界，120GB原件上传目标独立。OCR误读、简繁体、版式及真实样本验收按[实现约束](../development/128-document-text-ocr.md)；Office创建编辑与更多真实格式验收继续开放。

增量123补充：复杂 DOCX 已进入固定版本的正文预览、当前版本检索、完整文字比较及秘书资料读取；`docx-v2` 单独保存，不覆盖旧提取证据。修订保留、字段不计算，引用关系／限额／原件边界见[实现与使用](../development/123-docx-text-extraction.md)。PDF／OCR、Office创建编辑及真实样本核对仍开放；23项和5项移出保持不变。

增量120当前补充：XLSX／PPTX已接通有界准确版本正文预览／检索／比较与秘书资料读取；Excel公式与缓存、隐藏内容、PPT备注及明确原件边界见[Office正文设计](../development/120-office-text-extraction.md)。PDF/OCR、Office编辑和复杂DOCX仍开放；范围按[修订99](../delivery/04-current-scope-and-acceptance.md)。

版本：v1.0-draft；日期：2026-10-23；关联 U-08/11/12/15/19、R-04/12。

## 1. 文件对象与操作

| ID | 必须能力与约束 |
| --- | --- |
| FILE-01 | 逻辑目录 stable ID，项目/公司归属、名称、父目录、分类、授权；父子不成环，同一父目录下规范化名称唯一 |
| FILE-02 | 文件 stable ID 与版本 ID 分离，重命名/同权限范围移动只改元数据，120 GB 不重传；跨项目移动先分析权限变化，不自动放宽 |
| FILE-03 | 版本不可变，current_version 指向唯一正式版；大小/媒体类型/SHA-256/原文件名/作者/执行者/来源均可查 |
| FILE-04 | Web/Android 120 GB 分片直传、断点恢复、暂停/取消/刷新凭证；前端和服务器均不可整文件入内存 |
| FILE-05 | 文件阅读、下载、预览、历史版本、恢复和回收站；恢复旧内容生成新版本，不删除后续历史 |
| FILE-06 | 每个正式版本独立展示主端可用、复制排队/中/失败/已校验、最近错误和校验时间；不能只设 backed_up=true |
| FILE-07 | 普通文件删除进入回收站，双云不跟随立即硬删；保留/签署引用/法务保留/有效备份引用检查后才可能清理 |
| FILE-08 | 2 TB 导入保存清单、来源映射、权限、摘要和双云证据，失败逐条重跑；迁移后先保留来源 |

名称规范化：保留显示名称，比较键使用统一 Unicode/大小写策略，在 P0 样本冻结；禁止 `/\\`、空名称、点目录、控制字符、平台保留名，不能让用户名称决定真实对象 Key。对象键由 ID 生成，防止目录穿越与编码混淆。

## 2. 上传与正式提交

用户预选目标目录、file_id（更新时）、base_version_id、size_bytes、MIME、源指纹；服务端创建 upload_session 并固定分片大小/主存储纪元。会话 ID 不能作为无认证下载入口。

会话状态 `created → uploading → finalizing → verifying → ready → committed`；可进入 paused/expired/aborted/failed，paused 可回 uploading；可重试失败需要服务器分类，不能无条件跳 ready。ready 表示草稿字节验证完成，仍非正式版本。

FILE-09：客户端只写 draft 隔离区的指定 multipart/part；finalize 由服务器复核云端分片列表与总大小，冻结源对象标识；扫描/校验后由服务端生成正式不可变对象。不得先扫描一个仍能被原签名覆盖的 Key 再入库。大对象使用支持的 multipart copy/流式复制，不假设普通单次 Copy 可处理 120 GB。

FILE-10：commit 再查操作者权限、目录/文件状态、存储纪元、草稿摘要、base_version、授权计划；事务更新 current_version + audit + outbox。对象存在但事务失败是未引用候选，保留期后回收；事务已提交但响应丢失通过幂等结果查询确认。

120 GB 基线按 120,000,000,000 字节；32 MiB 候选约 3577 片。SHA-256 必须流式增量计算，可由服务端读取精确已冻结对象计算权威值；客户端摘要只能辅助比对，ETag 不替代整文件摘要。校验读取涉及的带宽/时间需计费评估。

Web 刷新后重新选择同源文件，检查大小/mtime/受控指纹，必要时内容验证；不匹配不能盲续传。Android 保存服务器会话和 durable URI/权限，系统杀进程后可重建任务并只补缺片；不承诺无限后台存活。用户撤销源文件权限时清楚提示重新授权。

## 3. 扫描与文件能力

| 格式组 | 首发纳管/替换 | 阅读/预览 | 秘书编辑 |
| --- | --- | --- | --- |
| txt / Markdown | 必须 | 文本预览、差异 | 必须，编码/换行/摘要保留 |
| 结构化 JSON/CSV 等 | 必须 | 逐适配器和大小限制 | 通过样本/Schema 校验才开放 |
| 文本 PDF/Office | 必须 | 内容提取与安全预览候选，Q-14 冻结 | 不承诺无损原排版修改；生成新版需专项适配器 |
| 扫描件/票据/图片 | 必须 | OCR 候选，原件保留 | 仅辅助字段/文案，不伪称修改原票据 |
| 视频/压缩包/镜像等大二进制 | 必须 | 元数据/受控下载；转码不默认包含 | 不整份入模型；不执行包内程序 |

FILE-11：扫描状态 pending/clean/blocked/unscannable；无法扫描与已安全不能同义。可接受的不可解析大二进制按分类策略与授权人员明确放行，记录 waiver 与原因；严格受限/可执行风险不默认放行。解析必须限制 CPU、内存、时间、输出、嵌套/解压比，禁宏、外链自动抓取和未知脚本。

ClamAV候选实现存在超过2GB的引擎设计限制；120GB文件不得宣称由其完整扫描。当前实现对超过显式扫描策略上限的来源阻断，后续按分类及授权豁免制度处理，见[完整性与扫描](../development/13-upload-integrity-and-scanning.md)。

## 4. 检索、引用与派生物

FILE-12：文件名、标签、目录、项目、正文搜索受 ACL；结果包含 resource_id、version_id、更新时间、授权片段和命中类型。正文索引仅使用已可用版本；索引滞后应展示时间，最新状态以 DB 为准。改分类/撤权优先阻断查询，再重建索引，不等待异步清理才生效。

预览、提取文本、缩略图、向量与摘要均派生自固定版本，拥有独立生成状态和相同或更严格的分类。缓存键包含组织、资源版本和授权安全版本；不能全员共享受限搜索结果。预览生成失败不影响原件版本可追踪。

## 5. 双云与回收

FILE-13：主端成功与副本成功分开提交。跨云源固定为不可变正式对象；每次复制记录源标识、目标上传、已确认分片、摘要、重试和校验。新/导入版本在有内容校验证据前不能标 verified；普通小文件有独立车道，120 GB 不能堵住票据备份。

FILE-14：清理条件同时满足：不为 current、未被合同/交付/有效记录固定引用、未法务保留、超业务保留期、不被仍需保证可恢复的备份集引用、已过待删观察窗口。副本保留与数据库恢复点一起评估。普通删除只改业务可见状态，不发跨云立即 purge。

异步复制失败时主文件仍可用，但界面明确风险与告警；重要项目归档可要求所有必要交付副本已 verified。对象丢失报 missing，不能根据存在同名文件自动认定恢复成功。

## 移动手工整理补充（增量94） / Mobile organization addendum

公共Taro文件中心必须沿用原权限与业务命令：实际父目录继承项目／分类；整理仅选同范围的真实可写目录；提交绑定准确源修订、原因和本人明确确认。回收／原位恢复保持正式版本，历史阅读必须固定准确版本且重新核对当前授权。缺字段、冲突和响应丢失不清空输入或隐式重试，先读取实际结果。目标与确认不从文字草稿恢复。目录／文件／版本各自分页；大文件传输继续由Web／Android承担。

Shared mobile commands inherit the actual parent scope, bind exact source revisions and require explicit confirmation. Trash/untrash preserve formal content. Version reading rechecks current authorization. Incomplete, conflicting or uncertain results retain text and stop repeats. Selection/confirmation are not draft fields. Collections paginate independently; Web/Android retain large-file responsibility.

[实现与范围 / Implementation and scope](../development/94-mobile-file-workspace.md) · [准确验证 / Exact verification](../quality/evidence/mobile-files/verification.md)。跨范围移动、永久清理与正式四端／云／容量验收仍按原需求进行。

## 任务讨论固定依据（增量95） / Fixed discussion evidence

任务评论可明确附加准确正式文件版本，最多20份。正文修订默认保留完整原附件，明确替换才采用本次完整列表；原关联不可核对时拒绝替换。文件新版本不会改变历史讨论依据，读取附件继续核对当前权限、扫描与准确版本状态，不把不可见附件当作已删除，也不在表单中加载120GB原件。真实流程和回执约束见[任务讨论协议](../development/95-task-discussion-and-mentions.md)。Task discussion attachments pin immutable formal versions and retain live access checks; historical content never follows a newer file head.
