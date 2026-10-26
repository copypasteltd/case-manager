# 逻辑数据字典与数据库约束

版本：v1.0-draft；日期：2026-10-23。本文是首发持久化契约，非已执行 DDL。实现迁移必须覆盖本表约束；分表/命名调整需映射记录，不能丢失语义。

已落地实际费用及实际资金的数据结构分别以[0036费用实施映射](35-canonical-actual-costs-and-incurred-reports.md)与[0037资金及结算实施映射](36-payments-refunds-and-settlement.md)为准。以下首发逻辑模型保留规划范围；不将规划中的简化表名、可变状态替代准确实施的不变账本。

订阅的准确实施映射见[0038订阅与预测协议](37-subscriptions-billing-and-forecast.md)：实际表名为 `subscriptions`、`subscription_revisions`、`subscription_prices`、`subscription_revision_files`、`subscription_periods`、`subscription_period_revisions`、`subscription_events`、`subscription_period_events`、`subscription_period_event_files`、`subscription_reminders`。以下 `billing_periods` 仍是早期逻辑名，不是另建的并行账期来源；实际费用沿用唯一 `actual_cost_entries` 与 `cost_links`，不由账期生成第二套实际费用。

## 1. 公共类型与关系

业务 ID 使用服务端生成 UUID；展示编号另列且组织内唯一。除特别说明，每张组织业务表有 `id uuid PK, org_id uuid FK, created_at timestamptz, updated_at timestamptz`；可变聚合根有 `version bigint NOT NULL >=1`。追加事件/修订不可更新内容，不强制为事件增加可变版本。

所有组织内关系同时校验 org_id，建议 `(org_id,id)` 复合唯一/FK，避免跨组织关联。业务发生日期用 date，时刻用 timestamptz UTC。size_bytes/offset/序号/外部 TG 数字 ID 使用 bigint（TG 可为负）；API 超 JS 安全整数的值用十进制字符串。

金额 numeric(20,6)、汇率 numeric(20,10)，API 字符串；百分比/比例有明确上下界。枚举使用 CHECK 或受控字典，并在迁移、API 和客户端共同更新。内容哈希 SHA-256 固定 64 位小写十六进制；密码/凭证哈希不是该内容哈希算法的复用。

## 2. 身份与授权

| 表 | 专有字段（公共列略） | 唯一/检查/关联 |
| --- | --- | --- |
| organizations | name, default_timezone, authorization_version | 首期单个组织；不等于 HR |
| users | login_key, display_name, status, credential_hash, auth_version, disabled_at | `(org_id,login_key)` 唯一；密码哈希不进 API |
| invitations | target_login, token_hash, expires_at, consumed_at, inviter_id | token_hash 唯一；事务消费一次 |
| sessions | user_id, token_hash, family_id, device_type, expires_at, revoked_at, auth_version | 刷新 token 单用；家庭重放可整族撤销 |
| mfa_credentials | user_id, encrypted_secret_ref, recovery_hashes, enabled_at | 密钥分权；不返回 secret |
| roles / role_actions | key, name / role_id, action | role key 组织内唯一；动作受登记 |
| role_bindings | user_id, role_id, scope_type, scope_id, valid_from/until | 用户/角色/范围有效组合唯一 |
| resource_grants | subject_type/id, resource_type/id, action, classification_ceiling, expires_at, grantor_id | 禁跨组织；授予者可授予范围内 |
| classification_policies | level, ai_allowed, channel_rules, policy_version | 版本追加；无配置默认拒绝 |
| external_identities | user_id, provider, subject, verified_at, revoked_at | `(org_id,provider,subject)` 唯一；TG subject 用数字字符串 |

停用不删除作者 FK；历史外部身份保留，重新绑定需要重新验证。认证/会话材料与业务消息分开加密、备份与访问。

## 3. 项目与知识

| 表 | 专有字段 | 不变量 |
| --- | --- | --- |
| counterparties | name, type, contact_fields, classification | 最小必要对方资料；不自动合并同名主体 |
| projects | project_no, name, type, scope, owner_id, start_date, due_date, state, template_version, classification | 编号组织内唯一；起止有效；owner 有效 |
| project_members | project_id, user_id, project_role, authorization_version | 项目/用户唯一；撤销驱动授权版本 |
| project_templates | key, revision, state, task_and_folder_schema | 已发布不可覆盖；project 固定 revision |
| tasks | project_id, parent_id, milestone_id, title, body, assignee_id, priority, state, resume_state, start_at, due_at, completed_at, review_required | 父任务/里程碑同项目；done 时间必填 |
| task_dependencies | project_id, predecessor_id, successor_id, kind | 两任务不同且同项目；pair 唯一；同项目图锁+环检测 |
| milestones | project_id, title, due_at, owner_id, state | achieved 必须满足任务/交付规则 |
| comments | resource_type/id, author_id, body, revision, attachment_versions | 编辑留修订；不能跨越 ACL |
| risks / issues | project_id, owner_id, severity, state, description, mitigation/resolution, due_at | risk realized 可关联 issue_id |
| change_requests | project_id, state, scope_delta, budget_delta, schedule_delta, approval_instance_id, base_project_version | approved 不等 implemented；实施再校验版本 |
| deliverables | project_id, name, current_revision_id, state, acceptance_criteria | 固定提交修订 |
| deliverable_revisions | deliverable_id, seq, file_version_ids, description, submitted_by/at | `(deliverable_id,seq)` 唯一、不可变 |
| acceptance_decisions | revision_id, decided_by/at, decision, reason | 一次有效终态决定；退回再提新修订 |
| project_budgets / budget_lines | project_id, revision, state / budget_id, category, currency, amount | 修订唯一；approved 不改行金额 |
| project_archives | project_id, project_version, schema_version=1 的完整 basis, fingerprint, summary, lessons_learned, follow_up, business_verified, financial_verified, actor_id, session_id, mfa_verified_at, reason, created_at | 准确 closing 项目、当前来源无阻塞、真实近期身份验证；归档阶段／来源／附件与记录同事务，历史不可改写 |
| project_archive_files | archive_id, version_id | 0—20 个固定正式版本，登记 project_archive 保留引用；提交后不能追加、替换或删除 |
| project_links | project_id, resource_type/id, relation | 只链接不复制；访问目标再鉴权 |
| posts | project_id nullable, type, title, classification, state, current_revision_id, author_id | 内部发布；草稿可见性受控 |
| post_revisions | post_id, seq, base_revision_id, body, summary, author_id, source_refs | 序号唯一；发布旧版恢复生成新修订 |
| post_file_links | post_revision_id, file_id, pinned_version_id nullable, follow_current | pinned/follow 二选一；引用不授予读权限 |
| tags / resource_tags | name_key, display_name / tag_id, resource_type/id | 组织内规范化 tag 唯一 |

## 4. 申请与业务台账

| 表 | 专有字段 | 不变量 |
| --- | --- | --- |
| workflow_definitions | key, revision, state, conditions, sequential_steps, self_approval_policy | `(key,revision)` 唯一；发布内容不可覆盖 |
| requests | kind, business_type/id, applicant_id, project_id, state, current_revision_id | state 使用需求规范；申请人不能客户端冒充 |
| request_revisions | request_id, seq, payload_schema_version, payload, content_hash, attachment_version_ids, currency, total_amount | 提交后不可变；JSON 必须对应版本化 Schema |
| approval_instances | request_revision_id, definition_revision_id, rules_snapshot, state, current_step_no | 同修订一次有效实例；快照不随模板变 |
| approval_steps | instance_id, step_no, assigned_user_id, state, expected_auth_version | `(instance_id,step_no)` 唯一；active 当前一步 |
| approval_actions | step_id, actor_id, decision, reason, request_version, decided_at | step 终态动作唯一；追加且不可伪造 actor |
| contracts | contract_no, direction, type, counterparty_id, project_id, owner_id, lifecycle, active_revision_id, parent_contract_id | 编号唯一；补充协议链禁止环 |
| contract_revisions | contract_id, seq, currency, amount, tax_amount, start_date, end_date, renewal_terms, draft_file_version_id, signed_file_version_id, approval_instance_id | 签署件固定版本；effective 需批准与证据 |
| contract_milestones | contract_id, revision_id, direction, due_date, amount, currency, fulfillment_state, evidence_refs | 不把应收计费用；节点与有效版本关联 |
| purchases | purchase_no, project_id, requester_id, state, request_id, currency, approved_total | 订单发生额与批准额差异校验 |
| purchase_items | purchase_id, line_no, description, quantity, unit, unit_price, tax, discount, line_total | line_no 唯一；quantity>0，定点求和 |
| purchase_orders / order_items | purchase_id, supplier_id, external_ref, currency,total / order_id, purchase_item_id, quantity, unit_price | 外部参考按供应商范围去重 |
| receipt_records / receipt_lines | order_id, accepted_by, date, evidence / record_id, order_item_id, quantity, result, reverses_line_id | 锁订单行，累计净验收不超规则；退货反向追加 |
| expense_claims | claim_no, applicant_id, project_id, request_id, currency, total, settlement_state | 单据一种币种；结算状态由分配派生 |
| expense_items | claim_id, line_no, expense_date, supplier_id, category, amount, receipt_ids, linked_cost_id | 明细总和=头额；关联已有费用不新建第二笔 |
| receipts | file_version_id, sha256, vendor_hint, amount_hint, date_hint, ocr_state, reviewed_by, duplicate_candidate | 原件与 OCR 候选分开；相似不是唯一性约束 |

业务编号允许有空洞，不以 `MAX(no)+1` 并发取号；使用事务序列/计数器与唯一约束。不把 JSON payload 当作跳过数据验证的通道；金额等索引/约束字段应规范化。

## 5. 订阅、费用与结算

| 表 | 专有字段 | 不变量 |
| --- | --- | --- |
| subscriptions | vendor_id, product, plan, owner_id, project_id nullable, state, cycle, anchor_day, timezone, next_charge_date, cancel_requested/effective_at, payment_method_label | 不保存卡号/CVV；暂停/取消不抹历史 |
| subscription_prices | subscription_id, valid_from/to, currency, amount, change_reason | 区间不重叠；账期固定价格版本 |
| billing_periods | subscription_id, start_date, end_date, charge_type, price_id, state, expected_charge_date | 半开区间；四字段唯一；重跑不重复 |
| expected_costs | billing_period_id nullable, source_key, currency, amount, expected_date, state | 唯一来源；不是实际费用 |
| actual_cost_entries | source_key, vendor_id, entry_kind, occurred_on, currency, amount, base_currency, fx_rate/date/source, base_amount, category, state, reverses_id, confirmed_by/at, reversed_at | source_key 唯一；expense/reversal 带符号汇总；reversed原记录仍属历史事实；原币汇率不改 |
| cost_links | cost_id, source_type/id, relation | 费用与多个业务单关联，报表只按 cost_id 计一次 |
| cost_allocations | cost_id, project_id nullable, amount, base_amount | 每成本分摊和等于原/折算额；null 项目为公司费用 |
| payment_records | source_key, record_kind, direction, currency, amount, paid_on, counterparty_id, external_ref, method_label, evidence_version_ids, state, reverses_id, reversed_at | payment/reversal；不自动执行资金；永久 source_key 唯一；冲销分配同事务 |
| payment_allocations | payment_id, cost_id, amount_in_payment_currency, amount_in_cost_currency, fx_snapshot | 同时锁付款/费用余额；默认同币种；禁止超配 |
| report_snapshots | kind, period, timezone, base_currency, as_of, source_ids_digest, totals, unresolved_counts, export_file_version_id | 口径固定、可追溯；缺汇率不计为零 |

跨行合计、区间重叠、依赖图和权限不能只靠前端：使用 DB 约束/触发器或受控事务用例+并发测试。选择机制写入迁移说明。

## 6. 文件与派生物

| 表 | 专有字段 | 不变量 |
| --- | --- | --- |
| folders | project_id nullable, parent_id nullable, name, name_key, classification, state | 同父 name_key 唯一，根目录使用明确 root_scope 避免 NULL 唯一漏洞；禁止环 |
| files | folder_id, name, name_key, classification, current_version_id, state, trashed_at, legal_hold | 文件/目录共享名称域或同父锁统一校验 |
| file_versions | file_id, seq, base_version_id, object_key, provider_source_id, storage_epoch, size_bytes, sha256, media_type, source, author_id, executor_id, scan_state, scan_waiver_ref, availability | 版本序号唯一；只正式对象可 available；unscannable需授权例外证据；正式字节不可变 |
| upload_sessions | owner_id, target_folder_id, file_id nullable, base_version_id, size_bytes, source_fingerprint, part_size, provider_upload_id, draft_key, frozen_source_id, state, expires_at, final_digest | 固定目标/大小/分片；draft 不等正式版 |
| upload_parts | session_id, part_no, size_bytes, provider_etag, acknowledged_at | `(session_id,part_no)` 唯一；云端 ListParts 最终权威 |
| file_replicas | version_id, provider, bucket, object_key, source_id, state, expected/actual_sha256, verified_at, last_error | `(version_id,provider,bucket)` 唯一；verified 需完整校验证据 |
| replica_parts | replica_id, upload_id, part_no, source_offset, size_bytes, etag, fencing_token | 可恢复固定源；旧租约不可提交 |
| derived_artifacts | version_id, kind, generator_version, state, output_key, text_ref, classification | `(version_id,kind,generator_version)` 唯一；继承限制 |
| search_documents | resource_type/id, version_id, title, text, tags, project_id, classification, index_version | 检索 ACL 在查询实时生效，不依赖索引清理 |
| retention_holds | resource/version_id, kind, reason, owner_id, valid_until | hold 优先于普通过期清理 |

current_version FK 指向同 file_id，需复合 FK/事务约束；正式版本、签署件和验收引用不级联删除。清理只由受控 job，保留 tombstone 与清理审计。

## 7. 秘书、渠道、任务与运维

| 表 | 专有字段 | 不变量 |
| --- | --- | --- |
| conversations / conversation_members | type, project_id, channel_binding_id, context_security_version / conversation_id,user_id | private/project/channel 分开；成员变化上下文失效 |
| messages | conversation_id, seq, role, actor_id, body, source_refs, visibility, created_at | 会话 seq 唯一；不保存私有推理 |
| agent_runs | conversation_id, initiating_message_id, actor_id, channel, state, skill_id/version, runtime_version, auth_snapshot, config_version, budget_reserved, last_event_seq, resume_of | 身份服务器注入；正式结果持久化 |
| run_events | run_id, seq, type, safe_payload, created_at | `(run_id,seq)` 唯一；追加，按序重播 |
| action_plans / action_items | run_id, hash, expires_at, state, auth_version / plan_id, seq, action, resource_id, base_version, params_hash, result_ref | 批准固定完整 hash；逐项结果 |
| approval_grants | plan_id, plan_hash, grantor_id, policy_version, scope, expires_at, revoked_at | 不复用到另一计划/变更版本 |
| tool_executions | run_id, invocation_key, tool_name, input_hash, state, business_commit_ref, safe_result, error_code | invocation_key 唯一；重试不换键 |
| skill_releases | skill_id, version, manifest_hash, state, allowed_tools, test_evidence, runtime_range | 发布不可覆盖；可独立停写/回滚 |
| usage_records | run_id, provider_request_id, usage_kind, units, estimated/actual_amount, currency | provider_request_id+kind 去重；不重复结算 |
| channel_bindings | provider, external_chat_id, topic_id, allowed_scope, max_classification, state | chat/topic 唯一；数字 ID 不用昵称 |
| inbound_updates | bot_id, external_update_id, received_at, safe_payload_ref, state | `(bot_id,external_update_id)` 唯一，持久化后 ack |
| notifications / deliveries | event_id, recipient_id, template_version, state / notification_id, channel, dedup_key, state, external_ref, attempt | dedup_key 唯一；unknown 不冒称失败可无风险重发 |
| hosts / services | name, owner_id, source_ref / host_id, project_id, endpoint_policy_ref, state, observed_at | 无采集为 unknown；凭据只引用 |
| probe_results | service_id, source, observed_at, received_at, sequence, result, latency | 服务/来源/序列去重；保留窗口 |
| incidents / maintenance_windows | service_scope, state, severity, timeline / start_at,end_at,scope,reason | 维护与原始观测分开 |
| jobs | kind, dedup_key, payload_version, payload, state, attempt, available_at, lease_until, lease_owner, fencing_token, last_error | dedup_key 唯一；仅有效 token 可 ack |
| outbox_events | event_id, type, schema_version, aggregate_type/id/version, payload, occurred_at, published_at | 与业务同事务；重复投递可处理 |
| processed_events | consumer, event_id, result_ref | consumer/event_id 唯一；防重复副作用 |
| idempotency_records | actor_scope, route_key, idempotency_key, request_hash, state, response_ref, expires_at | 作用域组合唯一；同键不同参数冲突 |
| audit_events | actor/executor, channel, request/run/job_id, action, resource/version, before/after_digest, result, reason, occurred_at | 追加、分级、无秘密；失败审计另通路 |
| backup_catalogs | snapshot_id, db_backup_ref, db_time, file_manifest_ref, schema_version, crypto_key_version, copy/verification_state | DB 与文件引用一致；可恢复点不是仅“上传完成时间” |
| storage_epochs | epoch, active_primary, changed_by/at, fence_evidence | 唯一有效主端；切换旧 worker/凭证失效 |
| config_revisions | namespace, revision, content, hash, approved_by, effective_at | 不含明文密钥；历史追加 |

## 8. 索引、删除与容量

合同财务的实际追加模型由迁移0039和[节点归属协议](38-contract-financial-reconciliation.md)细化：`contract_finance_heads` 独立于商业合同版本；`contract_finance_events` 追加费用/收付/退回/解除/核对事实；`contract_finance_event_files` 固定核对凭证并注册引用保护。来源修订、实际会话/MFA、金额上限、原退回关系与完整头由复合外键和延迟约束共同校验。客户收款退回占用真实对外付款的可分配余额；历史固定快照和当前投影分别读取。

法律结束的实际模型由迁移0040和[完成／终止协议](39-contract-completion-and-termination.md)细化：`contract_closures`按组织／合同唯一，固定准确生效修订、结束合同版本、财务版本、完整依据及指纹、发生日期、人员／实际会话／MFA、核实和未结事项处理；`contract_closure_files`固定1—20项结束凭证并登记版本引用。两表追加且禁止改写删除；延迟约束保证合同头、法律动作、准确版本和凭证数量一致。后续财务保持独立，不能覆盖旧法律事实或普通重开合同。

采购差异的实际模型由迁移0041和[执行基线协议](40-purchase-variance-approval-and-execution-baselines.md)细化：`purchase_variances`为独立提案头，`purchase_variance_revisions`保存完整不可变前后商业内容和固定履行集合，`purchase_variance_files`固定新旧文件并集。`purchase_plan_activations`只记录人工明确启用，不替换原`spend_revisions`；订单另保存当时基线修订及价格。提案、父采购、有效订单和实际费用共同校验，不以获批提案自动替换当前执行依据。

采购完成的实际模型由迁移0042和[完成核对协议](41-purchase-completion-and-settlement-review.md)细化：`purchase_completions`按组织／采购唯一，固定原批准修订、完成采购版本、完整交付／差异／费用／资金集合、变化指纹、三项人工核实、明确核实方式、说明、处置、实际日期与实际会话／MFA；`purchase_completion_files`固定1—20项正式凭证并登记不可变版本引用。完成头、履行事件、凭证数量和准确依据由独立约束校验。后续仅允许追加准确来源退货，实际财务仍分别留存；历史集合与当下不同通过读取投影表达，不更新旧完成记录。

追加迁移0043同时比较当前执行金额与有效订单占用金额；即使数量已经全部交付，仍须明确处理未使用的金额额度。原订单价格、实际费用与历史完成集合各自保留，不从新的执行额度反推它们应当改价。

常用索引：`(org_id,state,updated_at,id)` 分页；任务 `(project_id,assignee_id,state,due_at)`；审批 `(assigned_user_id,state)`；费用 `(org_id,occurred_on,currency,state)`；文件 `(folder_id,name_key)`；job `(state,available_at,lease_until)`；run_event `(run_id,seq)`；probe `(service_id,observed_at)`。以真实 query plan 验证，不为每列盲建索引。

高频探测/事件可按日期分区或保留窗口清理，需保留关联事件证据。2 TB 主要在对象存储，DB 容量还受对象数、版本数、正文、审计和探测频率影响，盘点前不伪造精确行数。保留对象数与平均大小两维模型，不能只按总 GB 估算查询性能。

迁移0044与[项目交付协议](42-project-deliverables-and-acceptance.md)扩展已有交付表：`deliverables`固定创建人、协议版本、负责人、可选里程碑及当前修订；`deliverable_revisions`固定名称／准则／负责人／里程碑／项目分类和版本、交付版本、说明、原因、提交人及真实会话、内容指纹；`deliverable_revision_files`按序固定1—20项正式版本并建立不可变引用；`acceptance_decisions`固定准确修订指纹、下一交付版本、实际决定人／会话／MFA及两项人工声明。决定、修订、文件引用不可改写，四项延迟一致性约束校验头、最新修订和决定。旧记录保持协议0，不补造创建或验收来源。
