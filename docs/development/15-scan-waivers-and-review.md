# 无法扫描文件的受控例外

日期：2026-10-23。迁移0020；源码 app/backend/internal/files/scan_waivers.go。承接[文件需求](../requirements/06-files-and-search.md)、[扫描边界](13-upload-integrity-and-scanning.md)与[正式提交](14-file-promotion-and-commit.md)。这是待公司配置的制度能力，不代表已批准任何生产例外。

## 1. 准入与默认拒绝

没有已发布策略就不能申请放行。策略只覆盖internal/project_restricted；confidential、恶意检测/隔离结果不进入本流程。申请仅适用于已有完整权威摘要、固定源与最新unscannable报告一致且会话有效的文件。

策略明确媒体类型精确值、大小上限、允许的无法扫描代码及单个独立审核人。可配置原因仅SCANNER_NOT_CONFIGURED、SCAN_SIZE_LIMIT、SCAN_ENGINE_SIZE_LIMIT、SCAN_CONTENT_LIMIT；不能把无效响应、传输错误或恶意检测直接改成安全。

媒体类型是输入元数据，不是格式识别或安全证明。显式审核承担例外判断；允许某MIME不代表完整扫描。已知可执行MIME被拒绝配置，伪装格式风险仍须独立审查和分类制度处理。

## 2. 角色与权限

file_scan_reviewer 是独立、要求MFA的内置角色，含file.scan.waive、folder.read、file.read，初始不绑定任何人。系统管理员不会自动获得内容或审批权；现有角色绑定明确人员、范围、分类上限与有效期。

发布策略需要config.manage及最近10分钟MFA，只建立制度，不赋予内容权限。申请时检查审核人当前具备目标范围读取和file.scan.waive权限。申请人不能是审核人；两人的当前权限分别核对。

审核可从Web/Android/微信/抖音有效会话进行，必须近期MFA；TG/模型不能代批。申请仍由Web/Android上传入口办理，小程序不取得大文件上传能力。

## 3. 固定申请与决定

申请绑定组织、会话/生命周期版本、扫描报告ID、来源摘要、权威SHA-256、策略ID/版本、分类、大小、名称/媒体类型、申请人、审核人、原因、显式风险确认与期限；这些输入不可更新。相同会话/报告仅一个pending申请。

申请使上传生命周期版本增加，但保持blocked/unscannable。审核人必须确认同一upload_version、scan_report_id及sha256；批准和驳回均需理由，批准需acknowledged_risk=true。

批准重新核对策略仍为申请快照且启用、审核人范围、申请人当前权限、分类、项目可写性、会话阶段/期限和存储纪元。重扫、撤权、分类变化、终止或制度变化后，旧pending不能继续批准。驳回只结束申请，不放行文件。

批准保存不可变决定、风险确认和MFA验证时间；上传变为verifying/waived，引用本次例外。决定、上传、审计及任务事件同事务；失败整体回滚。后续候选和commit保留waived及例外引用，原unscannable报告不改写。

已批准例外仅绑定特定来源，并在有效会话内用于候选/提交；新策略影响后续及未批准申请，不自动收回已正式保存的历史文件。历史例外撤销、已提交版本的重新处置尚需独立用例。

## 4. 接口

| 接口 | 用途 |
| --- | --- |
| GET /v1/storage/scan-waiver-policies | 有config.manage者读取各分类最新策略 |
| POST /v1/storage/scan-waiver-policies | expected_version从0开始；追加版本，可显式停用 |
| POST /v1/uploads/{id}/waivers | 本人有效上传/最新报告；固定摘要、原因及风险确认 |
| GET /v1/scan-waivers?state=... | 当前ACL允许的本人/指定审核人分页列表 |
| GET /v1/scan-waivers/{id} | 当前有权的申请人或指定审核人 |
| POST /v1/scan-waivers/{id}/decisions | 版本、报告/摘要、approve/reject、理由和风险确认 |
| POST /v1/scan-waivers/{id}/cancel | 申请人取消pending；action=cancel、版本和理由 |

所有命令遵守幂等，授权先于重放。策略发布增加组织授权版本；会话按既有机制取得当前授权。列表SQL先按身份、范围、分类和渠道过滤，再核对各项有效目标；游标绑定身份/授权版本/筛选。系统管理员不能据此枚举业务申请。

## 5. 记录与展现

scan_waiver_policies是追加历史；scan_waivers输入不可变；scan_waiver_decisions每申请一个最终决定且不可修改/删除。审计保留真实审核人和会话，不把秘书或后台任务伪装成审核人。

界面沿用语义令牌：clean显示“扫描通过”；waived使用警示色“例外放行”，提供原报告、固定摘要、申请及决定理由；pending显示“等待独立审核”。不能使用绿色安全盾或把“已批准”写成“文件安全”。

Web 已有申请/审核、固定报告、制度修订和会话内 MFA 页面，详见[上传队列与审核客户端](18-upload-queue-and-review-clients.md)。两个小程序与 Android 页面尚未完成，真实内容及独立人员的完整正向链仍需验收。隔离测试制度不导入真实环境，不替公司确定谁可批准或哪些大二进制可接受。
