package agent

import (
	"context"
	"encoding/json"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type ScopeTransferInput struct {
	PlanID          string `json:"plan_id"`
	ExpectedVersion int64  `json:"expected_version"`
	PlanHash        string `json:"plan_hash"`
	Reviewed        bool   `json:"reviewed"`
	Authorized      bool   `json:"authorized"`
	Reason          string `json:"reason"`
}
type NotificationInput struct {
	RequestID string `json:"request_id"`
	TelegramDisclosureInput
}
type FixedDeliveryView struct {
	RunID        string          `json:"run_id"`
	Kind         string          `json:"kind"`
	Input        json.RawMessage `json:"input"`
	Preview      json.RawMessage `json:"preview"`
	IntentSHA256 string          `json:"intent_sha256"`
	OperationID  *string         `json:"operation_id"`
	Result       json.RawMessage `json:"result"`
}
type fixedDelivery struct {
	FixedDeliveryView
	SessionID, Channel string
}

func fixedDeliveryRecord(ctx context.Context, tx pgx.Tx, org, run string) (*fixedDelivery, error) {
	var v fixedDelivery
	var output []byte
	e := tx.QueryRow(ctx, `SELECT i.run_id,i.kind,i.input,i.preview,i.intent_sha256,i.session_id,i.origin_channel,c.operation_id,c.result_data
 FROM agent_fixed_deliveries i LEFT JOIN agent_fixed_delivery_commits c ON c.org_id=i.org_id AND c.run_id=i.run_id WHERE i.org_id=$1 AND i.run_id=$2`, org, run).Scan(&v.RunID, &v.Kind, &v.Input, &v.Preview, &v.IntentSHA256, &v.SessionID, &v.Channel, &v.OperationID, &output)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if len(output) > 0 {
		v.Result = output
	} else {
		v.Result = json.RawMessage("null")
	}
	return &v, nil
}
func fixedDeliveryHuman(ctx context.Context, tx pgx.Tx, st executionState, v *fixedDelivery) (core.Actor, error) {
	a := st.Actor
	if a.Channel == "telegram" || v.Channel != a.Channel {
		return a, core.Forbidden()
	}
	a.SessionID = v.SessionID
	a.RunID = ""
	return a, identity.RequireInteractiveSession(ctx, tx, a, true)
}
func prepareFixedDelivery(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, run string, scope *ScopeTransferInput, notice *NotificationInput) (*fixedDelivery, error) {
	if scope == nil && notice == nil {
		return nil, nil
	}
	if scope != nil && notice != nil || c.Type != "private" || c.CreatorID != a.UserID {
		return nil, core.Forbidden()
	}
	members, e := memberIDs(ctx, tx, a.OrgID, c)
	if e != nil {
		return nil, e
	}
	if len(members) != 1 || members[0] != a.UserID {
		return nil, core.Forbidden()
	}
	if e = identity.RequireInteractiveSession(ctx, tx, a, true); e != nil {
		return nil, e
	}
	v := &fixedDelivery{FixedDeliveryView: FixedDeliveryView{RunID: run, Result: json.RawMessage("null")}, SessionID: a.SessionID, Channel: a.Channel}
	if scope != nil {
		if !core.ValidID(scope.PlanID) || core.RequireVersion(scope.ExpectedVersion) != nil || !scope.Reviewed || !scope.Authorized || core.Name(scope.Reason) != nil {
			return nil, core.Invalid("scope_transfer", "请先完整核对普通范围计划及可见性清单，再明确授权本次准确执行")
		}
		ai := a
		ai.RunID = run
		p, e := files.AgentScopeTransferPreview(ctx, tx, a, ai, scope.PlanID)
		if e != nil {
			return nil, e
		}
		if p.Version != scope.ExpectedVersion || p.PlanHash != scope.PlanHash {
			return nil, core.Conflict()
		}
		v.Kind = "scope_transfer"
		v.Input, _ = json.Marshal(scope)
		v.Preview, _ = json.Marshal(p)
		if len(v.Preview) > 64000 {
			return nil, core.Invalid("scope_transfer", "完整计划超过秘书单次读取上限，请缩小准确范围")
		}
	} else {
		if !core.ValidID(notice.RequestID) || !notice.Reviewed || !notice.DisclosedToGroup || core.RequireVersion(notice.ExpectedVersion) != nil || notice.StartChar < 0 || notice.MaxChars < 1 || notice.MaxChars > 2000 {
			return nil, core.Invalid("notification", "请先核对准确结果与将公开到原群的固定片段")
		}
		p, e := fixedNotificationPreview(ctx, tx, a, c, run, notice.RequestID)
		if e != nil {
			return nil, e
		}
		if p.RunVersion != notice.ExpectedVersion || p.BodySHA256 != notice.BodySHA256 || notice.StartChar > p.TotalChars || notice.MaxChars > p.TotalChars-notice.StartChar {
			return nil, core.Conflict()
		}
		if c.ProjectID != nil {
			source, e := runRecord(ctx, tx, a.OrgID, p.RunID, false)
			if e != nil {
				return nil, e
			}
			conv, e := metadata(ctx, tx, a.OrgID, source.ConversationID, false)
			if e != nil {
				return nil, e
			}
			if conv.ProjectID == nil || *conv.ProjectID != *c.ProjectID {
				return nil, core.Forbidden()
			}
		}
		v.Kind = "notification"
		v.Input, _ = json.Marshal(notice)
		v.Preview, _ = json.Marshal(p)
	}
	raw, _ := json.Marshal(map[string]any{"run_id": run, "actor_id": a.UserID, "session_id": a.SessionID, "origin_channel": a.Channel, "actor_auth_version": a.AuthVersion, "organization_auth_version": a.OrgAuthVersion, "conversation_id": c.ID, "conversation_revision_id": c.CurrentRevisionID, "context_security_version": c.ContextSecurityVersion, "kind": v.Kind, "input": v.Input, "preview": v.Preview})
	v.IntentSHA256 = core.Digest(raw)
	// Run FK is deferred; the complete human intent is fixed in this transaction.
	_, e = tx.Exec(ctx, `INSERT INTO agent_fixed_deliveries(run_id,org_id,actor_id,session_id,origin_channel,kind,input,preview,intent,intent_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, run, a.OrgID, a.UserID, a.SessionID, a.Channel, v.Kind, v.Input, v.Preview, string(raw), v.IntentSHA256)
	return v, e
}
func fixedDeliverySource(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, run string) (SourceSnapshot, error) {
	v, e := fixedDeliveryRecord(ctx, tx, a.OrgID, run)
	if e != nil {
		return SourceSnapshot{}, e
	}
	if v == nil {
		return SourceSnapshot{}, core.Missing()
	}
	var actor, conversation string
	if e = tx.QueryRow(ctx, "SELECT actor_id,conversation_id FROM agent_runs WHERE org_id=$1 AND id=$2", a.OrgID, run).Scan(&actor, &conversation); e != nil {
		return SourceSnapshot{}, e
	}
	if actor != a.UserID || conversation != c.ID || c.Type != "private" || c.CreatorID != a.UserID {
		return SourceSnapshot{}, core.Missing()
	}
	human := a
	human.RunID = ""
	human.SessionID = v.SessionID
	human.Channel = v.Channel
	if e = identity.RequireInteractiveSession(ctx, tx, human, v.OperationID == nil); e != nil {
		return SourceSnapshot{}, e
	}
	if v.Kind == "scope_transfer" {
		var in ScopeTransferInput
		_ = json.Unmarshal(v.Input, &in)
		if v.OperationID != nil {
			e = files.AgentScopeTransferResultReadable(ctx, tx, human, in.PlanID)
			if e == nil {
				current := a
				current.RunID = ""
				e = files.AgentScopeTransferResultReadable(ctx, tx, current, in.PlanID)
			}
		} else {
			ai := a
			ai.RunID = run
			_, e = files.AgentScopeTransferPreview(ctx, tx, human, ai, in.PlanID)
		}
	} else {
		var in NotificationInput
		_ = json.Unmarshal(v.Input, &in)
		var current TelegramDisclosurePreview
		current, e = fixedNotificationPreview(ctx, tx, human, c, run, in.RequestID)
		if e == nil && (current.BodySHA256 != in.BodySHA256 || current.RunVersion != in.ExpectedVersion) {
			e = runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
	}
	return SourceSnapshot{SourceReference: SourceReference{Type: "agent_fixed_delivery", ID: run}, ResourceVersion: 1, SHA256: v.IntentSHA256}, e
}
func executionFixedDelivery(ctx context.Context, tx pgx.Tx, st executionState) (*fixedDelivery, error) {
	v, e := fixedDeliveryRecord(ctx, tx, st.Actor.OrgID, st.Run.ID)
	if e != nil || v == nil {
		return v, e
	}
	if st.Profile.SkillID != "information-organize" {
		return nil, core.Forbidden()
	}
	if _, e = fixedDeliverySource(ctx, tx, st.Actor, st.Conversation, st.Run.ID); e != nil {
		return nil, e
	}
	return v, nil
}
func (s *Service) executionFixedTool(ctx context.Context, tx pgx.Tx, st executionState, tool string, args json.RawMessage) (map[string]any, []SourceSnapshot, error) {
	var in struct {
		RunID        string `json:"run_id"`
		IntentSHA256 string `json:"intent_sha256"`
	}
	if e := strictArguments(args, &in); e != nil {
		return nil, nil, e
	}
	v, e := executionFixedDelivery(ctx, tx, st)
	if e != nil {
		return nil, nil, e
	}
	if v == nil || in.RunID != st.Run.ID || in.IntentSHA256 != v.IntentSHA256 {
		return nil, nil, runtimeFailure("FIXED_INTENT_REQUIRED")
	}
	scope := tool == "propose_scope_transfer" || tool == "commit_scope_transfer"
	if scope != (v.Kind == "scope_transfer") {
		return nil, nil, core.Forbidden()
	}
	human, e := fixedDeliveryHuman(ctx, tx, st, v)
	if e != nil {
		return nil, nil, e
	}
	source := SourceSnapshot{SourceReference: SourceReference{Type: "agent_fixed_delivery", ID: st.Run.ID}, ResourceVersion: 1, SHA256: v.IntentSHA256}
	if tool == "propose_scope_transfer" || tool == "propose_notification" {
		return map[string]any{"status": "succeeded", "committed": false, "operation_id": nil, "resource_refs": []ResultReference{}, "source_refs": []ResultReference{reference(source)}, "content": v.FixedDeliveryView, "safe_summary": "已核对人工批准的固定计划；尚未执行或入队", "retryable": false, "next_action": ""}, []SourceSnapshot{source}, nil
	}
	if v.OperationID == nil {
		cmd := core.Command{Actor: human, RequestID: core.NewID(), Route: "/internal/agent-fixed-deliveries/" + v.Kind, Key: "agent-fixed:" + st.Run.ID, Body: v.Input, ExclusiveOrganizationLock: true}
		var result core.Result
		if scope {
			var i ScopeTransferInput
			_ = json.Unmarshal(v.Input, &i)
			service := files.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB, Tx: tx}}
			result, e = service.DecideScopeTransfer(ctx, cmd, i.PlanID, files.ScopeTransferDecision{ExpectedVersion: i.ExpectedVersion, PlanHash: i.PlanHash, Reason: i.Reason}, true)
		} else {
			var i NotificationInput
			_ = json.Unmarshal(v.Input, &i)
			service := Service{DB: s.DB, Commands: platform.Commands{DB: s.DB, Tx: tx}}
			result, e = service.ApproveTelegramDisclosure(ctx, cmd, i.RequestID, i.TelegramDisclosureInput)
		}
		if e != nil {
			return nil, nil, e
		}
		raw, _ := json.Marshal(result.Data)
		if rawResult, ok := result.Data.(json.RawMessage); ok {
			raw = rawResult
		}
		var operation string
		if e = tx.QueryRow(ctx, "SELECT id FROM idempotency_records WHERE org_id=$1 AND actor_id=$2 AND idempotency_key=$3 AND route_key=$4 AND state='succeeded'", human.OrgID, human.UserID, cmd.Key, cmd.Route).Scan(&operation); e != nil {
			return nil, nil, e
		}
		_, e = tx.Exec(ctx, "INSERT INTO agent_fixed_delivery_commits(run_id,org_id,operation_id,result_data) VALUES($1,$2,$3,$4)", st.Run.ID, human.OrgID, operation, raw)
		if e != nil {
			return nil, nil, e
		}
		v.OperationID = &operation
		v.Result = raw
	}
	content := v.Result
	kind, id := "file_scope_transfer", ""
	summary := "准确范围变更已执行；旧授权已终止，需要重新读取当前权限"
	if scope {
		var i ScopeTransferInput
		_ = json.Unmarshal(v.Input, &i)
		id = i.PlanID
		content, _ = json.Marshal(map[string]any{"plan_id": id, "state": "executed", "old_authority_invalidated": true})
	} else {
		var receipt TelegramDisclosureReceipt
		if json.Unmarshal(v.Result, &receipt) != nil {
			return nil, nil, runtimeFailure("RUNTIME_RESULT_INVALID")
		}
		kind, id = "telegram_delivery", receipt.DeliveryID
		summary = "固定片段已入原群投递队列；实际送达、失败或不确定状态需按投递账本核对"
	}
	return map[string]any{"status": "succeeded", "committed": true, "operation_id": v.OperationID, "resource_refs": []ResultReference{{Type: kind, ID: id}}, "source_refs": []ResultReference{reference(source)}, "content": content, "safe_summary": summary, "retryable": false, "next_action": ""}, []SourceSnapshot{source}, nil
}

// Scope transfer commits invalidate old authority immediately. Their exact
// operation/lease receipt is terminal; it never refreshes an old run's stamp.
func (s *Service) terminalizeScopeTransfer(ctx context.Context, tx pgx.Tx, st executionState, lease ExecutionLease, output json.RawMessage) error {
	var version int64
	if e := tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1", st.Actor.OrgID).Scan(&version); e != nil {
		return e
	}
	if version != st.Actor.OrgAuthVersion+1 {
		return runtimeFailure("SCOPE_TERMINAL_VERSION_MISMATCH")
	}
	_, e := tx.Exec(ctx, `INSERT INTO agent_scope_transfer_terminals(run_id,org_id,job_id,lease_owner,fencing_token,authority_stamp,credential_sha256,new_organization_version,output)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, st.Run.ID, st.Actor.OrgID, lease.Job.ID, lease.Job.LeaseOwner, lease.Job.FencingToken, lease.AuthorityStamp, lease.credentialSHA256, version, output)
	if e != nil {
		return e
	}

	fixed, e := fixedDeliveryRecord(ctx, tx, st.Actor.OrgID, st.Run.ID)
	if e != nil || fixed == nil || fixed.OperationID == nil {
		return runtimeFailure("SCOPE_TERMINAL_RECEIPT_MISSING")
	}
	var original ScopeTransferInput
	if e = json.Unmarshal(fixed.Input, &original); e != nil {
		return e
	}
	result := ProposedResult{Status: "completed", Summary: "准确普通范围计划已执行，旧授权已终止。进一步操作须新建任务并重新核对权限。", ResourceRefs: []ResultReference{{Type: "file_scope_transfer", ID: original.PlanID}}, SourceRefs: []ResultReference{{Type: "agent_fixed_delivery", ID: st.Run.ID}}, OperationIDs: []string{*fixed.OperationID}, MissingFields: []string{}}
	sources, _ := json.Marshal([]SourceSnapshot{{SourceReference: SourceReference{Type: "agent_fixed_delivery", ID: st.Run.ID}, ResourceVersion: 1, SHA256: fixed.IntentSHA256}})
	rawResult, _ := json.Marshal(result)
	body := result.Summary
	_, e = tx.Exec(ctx, `INSERT INTO conversation_messages(id,org_id,conversation_id,run_id,seq,role,body,author_id,audience_ids,source_snapshots) SELECT $1,$2,$3,$4,coalesce(max(seq),0)+1,'assistant',$5,$6,$7,$8 FROM conversation_messages WHERE org_id=$2 AND conversation_id=$3`, core.NewID(), st.Actor.OrgID, st.Conversation.ID, st.Run.ID, body, st.Actor.UserID, st.Run.audience, sources)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO agent_run_results(run_id,org_id,authority_stamp,result,usage,source_snapshots,body) VALUES($1,$2,$3,$4,$5,$6,$7)`, st.Run.ID, st.Actor.OrgID, lease.AuthorityStamp, rawResult, json.RawMessage(`{"model_usage_accepted":false,"reason":"ordinary_scope_transfer_terminated_authority"}`), sources, body)
	if e != nil {
		return e
	}
	completed := "completed"
	if e = executionTransition(ctx, tx, st, "succeeded", "run.completed", nil, &completed); e != nil {
		return e
	}
	return s.Queue().CompleteInTx(ctx, tx, lease.Job)
}
func (s *Service) scopeTerminalReconciled(ctx context.Context, lease ExecutionLease, controller bool) (bool, error) {
	var exists bool
	e := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_scope_transfer_terminals t JOIN agent_runs r ON r.org_id=t.org_id AND r.id=t.run_id
 JOIN organizations o ON o.id=t.org_id JOIN agent_fixed_deliveries i ON i.org_id=t.org_id AND i.run_id=t.run_id
 JOIN users u ON u.org_id=t.org_id AND u.id=r.actor_id JOIN sessions se ON se.org_id=t.org_id AND se.id=i.session_id
 JOIN agent_fixed_delivery_commits c ON c.org_id=t.org_id AND c.run_id=t.run_id JOIN file_scope_transfers p ON p.org_id=t.org_id AND p.id=(i.input->>'plan_id')::uuid
 WHERE t.org_id=$1 AND t.run_id=$2 AND t.job_id=$3 AND t.lease_owner=$4 AND t.fencing_token=$5
 AND ($6 OR (t.authority_stamp=$7 AND t.credential_sha256=$8)) AND r.state='succeeded' AND r.outcome='completed'
 AND o.authorization_version=t.new_organization_version AND u.status='active' AND u.auth_version=r.actor_auth_version
 AND se.revoked_at IS NULL AND se.expires_at>clock_timestamp() AND se.user_id=r.actor_id AND se.device_type=i.origin_channel AND se.auth_version=u.auth_version
 AND p.state='executed' AND EXISTS(SELECT 1 FROM jobs j WHERE j.org_id=t.org_id AND j.id=t.job_id AND j.state='succeeded')
)`, lease.OrgID, lease.RunID, lease.Job.ID, lease.Job.LeaseOwner, lease.Job.FencingToken, controller, lease.AuthorityStamp, lease.credentialSHA256).Scan(&exists)
	if e != nil || !exists {
		return exists, e
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	var a core.Actor
	var plan string
	e = tx.QueryRow(ctx, `SELECT r.org_id,r.actor_id,r.actor_auth_version,o.authorization_version,i.session_id,i.origin_channel,i.input->>'plan_id' FROM agent_fixed_deliveries i JOIN agent_runs r ON r.org_id=i.org_id AND r.id=i.run_id JOIN organizations o ON o.id=r.org_id WHERE i.org_id=$1 AND i.run_id=$2`, lease.OrgID, lease.RunID).Scan(&a.OrgID, &a.UserID, &a.AuthVersion, &a.OrgAuthVersion, &a.SessionID, &a.Channel, &plan)
	if e != nil {
		return false, e
	}
	if e = identity.RequireInteractiveSession(ctx, tx, a, false); e != nil {
		return false, e
	}
	e = files.AgentScopeTransferResultReadable(ctx, tx, a, plan)
	return e == nil, e
}

func executionFixedText(ctx context.Context, tx pgx.Tx, st executionState, id string, version *string, offset, max int) (any, []SourceSnapshot, error) {
	if id != st.Run.ID || version != nil {
		return nil, nil, core.Missing()
	}
	v, e := executionFixedDelivery(ctx, tx, st)
	if e != nil || v == nil {
		return nil, nil, e
	}
	raw, _ := json.Marshal(v.FixedDeliveryView)
	part, next, total, e := fileTextSlice(string(raw), offset, max)
	if e != nil {
		return nil, nil, e
	}
	source := SourceSnapshot{SourceReference: SourceReference{Type: "agent_fixed_delivery", ID: id}, ResourceVersion: 1, SHA256: v.IntentSHA256}
	return map[string]any{"type": "agent_fixed_delivery", "id": id, "resource_version": 1, "source_sha256": v.IntentSHA256, "text": part, "offset": offset, "next_offset": next, "total_chars": total, "untrusted_data": true}, []SourceSnapshot{source}, nil
}

// Reading another run's prose into a fresh model turn rechecks every dependency
// under AI policy, even though the original human can read the result.
func fixedNotificationPreview(ctx context.Context, tx pgx.Tx, human core.Actor, current Conversation, run, request string) (TelegramDisclosurePreview, error) {
	var empty TelegramDisclosurePreview
	if e := authz.RequireOrganization(ctx, tx, human, "telegram.handoff.disclose"); e != nil {
		return empty, e
	}
	p, e := TelegramHandoffResult(ctx, tx, human, request)
	if e != nil {
		return empty, e
	}
	original, e := runRecord(ctx, tx, human.OrgID, p.RunID, false)
	if e != nil {
		return empty, e
	}
	c, e := metadata(ctx, tx, human.OrgID, original.ConversationID, false)
	if e != nil {
		return empty, e
	}
	if current.ProjectID != nil && (c.ProjectID == nil || *c.ProjectID != *current.ProjectID) {
		return empty, core.Forbidden()
	}
	ai := human
	ai.RunID = run
	if e = require(ctx, tx, ai, c, "conversation.read"); e != nil {
		return empty, e
	}
	var raw []byte
	if e = tx.QueryRow(ctx, "SELECT source_snapshots FROM agent_run_results WHERE org_id=$1 AND run_id=$2", human.OrgID, p.RunID).Scan(&raw); e != nil {
		return empty, e
	}
	var before []SourceSnapshot
	if json.Unmarshal(raw, &before) != nil {
		return empty, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	refs := make([]SourceReference, 0, len(before))
	for _, v := range before {
		if v.Type == "agent_fixed_delivery" {
			return empty, core.Forbidden()
		}
		refs = append(refs, v.SourceReference)
	}
	after, e := sourceSnapshots(ctx, tx, ai, c, refs, []string{human.UserID}, true)
	if e != nil {
		return empty, e
	}
	if len(before) != len(after) {
		return empty, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	for i, v := range before {
		if v.SHA256 != after[i].SHA256 || v.ResourceVersion != after[i].ResourceVersion {
			return empty, runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
	}
	if len([]byte(p.Body)) > 64000 {
		return empty, core.Invalid("notification", "原结果超过单次模型预览上限，不能截断后代为授权")
	}
	return p, nil
}
