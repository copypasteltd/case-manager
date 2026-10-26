# 文件保留、固定引用与清理评估

本协议实现 [文件需求](../requirements/06-files-and-search.md) 中的保留约束和清理前依赖核对，承接 [历史恢复](19-history-restoration.md)。当前实现保留制度、保留锁、统一固定版本引用及不可变评估记录。物理删除执行器、恢复集目录和完整业务引用接入仍待完成；不存在可由评估记录直接调用的删除接口。

## 1. 不变量与范围

1. 当前内容版本始终排除。回收站不是两个桶的立即硬删除指令。
2. 法务标记、有效保留锁、知识/评论/业务快照/交付/有效恢复集固定引用优先于保存天数。
3. 每次评估固定文件元数据版本、内容版本、制度修订、时间与依赖数量，计算规范 JSON 的 SHA-256，不修改原件、版本或当前指针。
4. 评估是已发生事实的记录，不是后续删除授权或租约。执行前必须重新核对并封锁新的引用和读取依赖；不能复用旧评估跳过检查。
5. 未发布制度不代填90天或180天。制度未启用也会阻断评估候选；发布制度不会启动删除。

## 2. 制度修订

四类制度独立保存：formal_history、trash、draft、multipart。发布需组织 config.manage、人工客户端、有效会话及最近10分钟 MFA；每类 expected_version 首次为0，此后严格递增。历史修订不可更新或删除，组织授权版本随发布增加。

retain_seconds 为1天至3650天，observation_seconds 为24小时至365天。这是实现的有效范围，不是公司已批准的保存时长。Web 以天和小时填写，实际保存整数秒。观察期也不是已满足的观察证据，后续清理计划须单独记录观察起止和批准。

正式历史按该版本创建时间计算保留期限。文件已进回收站时，还必须存在并满足 trash 制度，其期限从 trashed_at 计算；两者取较晚截止时间，任一缺失、停用或尚未到期均阻断。当前版本即使在回收站也持续受保护。草稿/分片制度当前可独立修订，其对象盘点和清理执行尚未接线，不把正式版本评估当作草稿清理结果。

## 3. 保留锁与授权

新增 file_retention_manager 角色，包含 file.retention.manage、file.read、folder.read，要求 MFA；迁移和新组织初始化只建立未绑定角色，不自动给任何人内容权限。仍检查项目成员、分类上限、渠道以及严格受限资料的精确对象授权。

读者可读取文件法务标记与有效保留锁数量。详细依据、创建、释放、评估及读取评估记录要求同一文件的 read 与 retention.manage；写操作限定 Web/Android/微信/抖音人工会话，Agent 和 TG 不可绕过。创建和释放还要求最近 MFA。归档项目的法律保护仍可维护，不以业务可写状态阻止保留管理。

保留锁可针对整个文件或一个固定内容版本，类型为 legal、business、investigation。原因必填；期限可空，表示持续保留；有期限时须晚于当前时间且不超过十年。版本必须属于同一文件/组织。版本级锁同样阻止文件移入回收站。

释放是独立、不可变、唯一的记录，保存经办人、时间及原因；原锁不可改写或删除。并发释放最多一条结果；同一幂等键重试返回原结果。释放某项锁不清除 files.legal_hold 或其他锁、引用。已过期锁保留历史，可追加明确释放记录。文件行锁使添加/释放与回收站状态操作串行核对。

## 4. 固定版本引用

file_version_references 保存组织、文件、准确版本、来源类型及来源记录标识。来源类型覆盖 knowledge_revision、comment、approval_snapshot、contract_revision、purchase_receipt、claim_revision、deliverable、recovery_set，后续模块须按业务事务接入；枚举存在不代表相应模块已实现。

迁移回填既有知识固定引用和评论附件。数据库触发器在新增不可变正文修订/评论的同一事务自动登记，复合外键拒绝跨文件、跨组织错绑。重复同一来源/版本不产生重复引用。知识 follow-current 来源不固定旧版本；其读取时指向的当前版本另受保护。旧知识修订和旧评论的固定附件持续保留，不因发布新修订被静默解绑。

引用及释放记录不可变。引用释放须由各业务生命周期检查后同事务追加 file_reference_releases；当前没有通用用户解锁接口，保留管理人不能借评估界面解除合同、审批或恢复集的引用。注册表只保存关联标识，不复制受限文档标题或正文。

## 5. 评估内容

POST 为某个准确内容版本生成不可变 retention_assessments。文件元数据版本不匹配返回409；授权失败在幂等重放之前拒绝。报告不包含桶、对象路径、云凭据或其他业务记录标题，固定引用仅按类型计数。评估摘要由规范 JSON 计算，并与完整快照同事务保存。

| 阻断原因 | 核对依据 |
| --- | --- |
| POLICY_REQUIRED / POLICY_DISABLED | 正式历史制度，以及回收站状态下的附加制度 |
| RETENTION_PERIOD_ACTIVE | 已知保留期限中的较晚截止时间 |
| CURRENT_VERSION | 文件当前正式指针 |
| LEGAL_HOLD / RETENTION_HOLD | 文件级标记、此版本或整个文件的有效未释放锁 |
| BUSINESS_REFERENCE | 未释放的固定引用 |
| RESTORE_ACTIVE | 未过期且尚未终止/提交的来源恢复任务 |
| UPLOAD_ACTIVE | 未过期且仍以此版本为基础的上传会话 |
| DOWNLOAD_PERMIT_ACTIVE | 尚未到期的固定下载许可 |
| REPLICA_EVIDENCE_INCOMPLETE | 非 verified 副本，或不足两种提供商的完整验证证据 |
| DERIVED_OBJECT_REVIEW_REQUIRED | 尚需核对的派生对象输出 |

依赖数量来自一个 SQL 语句快照，记录 checked_at；制度发布与命令共享组织锁。eligible_for_review 仅表示报告中上述阻断项为空，不等于可以删除。execution_available 当前始终为 false。后续修改引用、到期或释放不会改写历史报告。

## 6. 接口与界面

| 接口 | 用途 |
| --- | --- |
| GET /storage/retention-policies | 配置管理人读取各类当前修订 |
| POST /storage/retention-policies | 发布新修订，201 |
| GET /files/{id}/retention | 文件读者获取保护摘要与管理能力 |
| GET /files/{id}/retention-holds | 管理人分页读取依据与释放历史 |
| POST /files/{id}/retention-holds | 添加保留锁，201 |
| POST /retention-holds/{id}/release | 追加释放依据 |
| POST /file-versions/{id}/retention-assessments | 保存评估快照，201 |
| GET /retention-assessments/{id} | 在当前权限下读取已保存评估 |

列表游标绑定组织、身份、授权版本、渠道和文件；分页前鉴权。全部变更沿用幂等、审计及 Outbox 同事务。审计失败回滚业务记录与幂等结果，不留下半项保留或孤立评估。

Web 在文件详情展示保护摘要；管理人可读依据、添加文件/版本保留、明确释放和生成版本评估。MFA 在原表单内再验证，保留当前填写且不自动提交。制度页面从“存储制度”进入，四类分别修订；没有默认启用/代填保存天数。仅使用已有品牌、语义状态和焦点 Token，支持双主题、320至1440宽度及滚动模态框。

## 7. 验证与执行边界

原生 PostgreSQL 场景覆盖保留与回收站、不可变记录、幂等重放、权限隔离、固定附件引用、制度修订、回收站附加制度、审计回滚、并发释放和撤权后拒绝重放。原有文件/知识场景继续验证，不能只检查新接口编译。

浏览器脚本为 app/scripts/check_retention.mjs，证据在 [retention/result.json](../quality/evidence/retention/result.json)。脚本通过真实登录和 MFA，给管理员建立仅限专用元数据项目、最长一小时的临时角色，并在 finally 撤销；保留锁通过真实页面建立及释放。夹具没有云对象、扫描报告或传输成功证据；制度表单填写后取消，不代替公司发布待决策的保存时长。所有自建浏览器与上下文均关闭。

物理清理后续必须具备恢复集清单、全部 OA/交付引用接入、对象引用/读取屏障、观察/批准记录、独立最小删除身份、存储纪元隔离、结果未知后对账、分提供商删除证据和派生物策略。主端清理不得立即传播为次端清理。

2026-10-23 核实的 [OSS DeleteObject](https://www.alibabacloud.com/help/en/oss/developer-reference/deleteobject) 区分版本删除与删除标记；[R2 S3 兼容表](https://developers.cloudflare.com/r2/api/s3/api/) 未列出 DeleteObject 的 If-Match 条件支持。设计不能把 GET/HEAD 的 If-Match 或 AWS S3 的同名功能推断为两个提供商的原子条件删除能力。执行器须依据当时实际支持的能力设计并专项验证，当前没有调用任何真实删除。
