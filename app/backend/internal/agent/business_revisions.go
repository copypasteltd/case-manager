package agent

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/spend"
	"copypaste.hk/platform/internal/workflow"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

type BusinessRevisionBasis struct {
	Kind           string          `json:"kind"`
	ID             string          `json:"id"`
	Version        int64           `json:"version"`
	RevisionID     string          `json:"revision_id"`
	ContentHash    string          `json:"content_hash"`
	Title          string          `json:"title"`
	ProjectID      *string         `json:"project_id"`
	ProjectVersion int64           `json:"project_version"`
	Classification string          `json:"classification"`
	ScopeSHA256    string          `json:"scope_sha256"`
	CrossScope     bool            `json:"cross_scope"`
	Content        json.RawMessage `json:"content"`
}
type BusinessRevisionInput struct {
	Kind               string          `json:"kind"`
	ResourceID         string          `json:"resource_id"`
	ExpectedVersion    int64           `json:"expected_version"`
	BaseRevisionID     string          `json:"base_revision_id"`
	ContentHash        string          `json:"content_hash"`
	ScopeSHA256        string          `json:"scope_sha256"`
	CrossScopeReviewed bool            `json:"cross_scope_reviewed"`
	Arguments          json.RawMessage `json:"arguments"`
}
type BusinessRevisionView struct {
	RunID        string                `json:"run_id"`
	Input        BusinessRevisionInput `json:"input"`
	Basis        BusinessRevisionBasis `json:"basis"`
	IntentSHA256 string                `json:"intent_sha256"`
	Status       string                `json:"status"`
	OperationID  *string               `json:"operation_id"`
	Result       *BusinessDraftReceipt `json:"result"`
}
type PreparedBusinessRevision struct {
	Input BusinessRevisionInput `json:"input"`
	Basis BusinessRevisionBasis `json:"basis"`
}
type businessRevisionRecord struct {
	BusinessRevisionView
	OrgID, ActorID, ConversationID, SessionID, Intent string
}

func revisionScope(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, b *BusinessRevisionBasis) error {
	if c.Type != "private" || c.CreatorID != a.UserID || b.Classification == "confidential" {
		return core.Missing()
	}
	b.CrossScope = !sameProject(b.ProjectID, c.ProjectID) || b.Classification != c.Classification
	if b.CrossScope {
		for _, action := range []string{"agent.plan.approve", "agent.plan.commit"} {
			if e := authz.RequireOrganization(ctx, tx, a, action); e != nil {
				return e
			}
		}
	}
	if b.ProjectID != nil {
		if e := tx.QueryRow(ctx, "SELECT version FROM projects WHERE org_id=$1 AND id=$2", a.OrgID, *b.ProjectID).Scan(&b.ProjectVersion); e != nil {
			return e
		}
	}
	raw, _ := json.Marshal(map[string]any{"conversation_id": c.ID, "conversation_revision_id": c.CurrentRevisionID, "context_security_version": c.ContextSecurityVersion, "project_id": b.ProjectID, "project_version": b.ProjectVersion, "classification": b.Classification, "cross_scope": b.CrossScope, "actor_id": a.UserID})
	b.ScopeSHA256 = core.Digest(raw)
	return nil
}
func revisionBasis(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, kind, id string) (BusinessRevisionBasis, error) {
	b := BusinessRevisionBasis{Kind: kind, ID: id}
	if !businessDraftKind(kind) || !core.ValidID(id) || a.RunID == "" {
		return b, core.Missing()
	}
	if kind == "subscription" {
		v, e := finance.AgentSubscriptionRevisionBasis(ctx, tx, a, id)
		if e != nil {
			return b, e
		}
		b.Version, b.RevisionID, b.ContentHash, b.Title, b.ProjectID, b.Classification = v.Version, v.CurrentRevisionID, v.ContentHash, v.Product, v.ProjectID, v.Classification
		b.Content, _ = json.Marshal(v.SubscriptionContent)
	} else {
		v, e := workflow.AgentBusinessRead(ctx, tx, a, kind, id, false)
		if e != nil {
			return b, e
		}
		if v.Lifecycle != "draft" || (v.ApprovalState != "not_submitted" && v.ApprovalState != "returned" && v.ApprovalState != "withdrawn") {
			return b, core.Forbidden()
		}
		if e = authz.Require(ctx, tx, a, kind+".update", authz.Resource{Type: kind, ID: id, ProjectID: v.ProjectID, OwnerID: &v.CreatorID, Classification: v.Classification}); e != nil {
			return b, e
		}
		b.Version, b.RevisionID, b.ContentHash, b.Title, b.ProjectID, b.Classification, b.Content = v.Version, v.CurrentRevisionID, v.ContentHash, v.Title, v.ProjectID, v.Classification, v.Payload.Content
	}
	e := revisionScope(ctx, tx, a, c, &b)
	return b, e
}
func (s *Service) BusinessRevisionBasis(ctx context.Context, a core.Actor, conversation, kind, id string) (BusinessRevisionBasis, error) {
	tx, e := identity.BeginInteractiveRead(ctx, s.DB, a)
	if e != nil {
		return BusinessRevisionBasis{}, e
	}
	defer tx.Rollback(ctx)
	c, e := metadata(ctx, tx, a.OrgID, conversation, false)
	if e != nil {
		return BusinessRevisionBasis{}, e
	}
	if e = require(ctx, tx, a, c, "conversation.read"); e != nil {
		return BusinessRevisionBasis{}, e
	}
	ai := a
	ai.RunID = core.NewID()
	return revisionBasis(ctx, tx, ai, c, kind, id)
}
func (s *Service) PrepareBusinessRevision(ctx context.Context, a core.Actor, conversation string, in BusinessRevisionInput) (PreparedBusinessRevision, error) {
	tx, e := identity.BeginInteractiveRead(ctx, s.DB, a)
	if e != nil {
		return PreparedBusinessRevision{}, e
	}
	defer tx.Rollback(ctx)
	c, e := metadata(ctx, tx, a.OrgID, conversation, false)
	if e != nil {
		return PreparedBusinessRevision{}, e
	}
	if e = require(ctx, tx, a, c, "conversation.read"); e != nil {
		return PreparedBusinessRevision{}, e
	}
	ai := a
	ai.RunID = core.NewID()
	b, args, e := revisionArguments(ctx, tx, ai, c, in)
	in.Arguments = args
	return PreparedBusinessRevision{Input: in, Basis: b}, e
}
func (s *Service) PrepareBusinessDraft(ctx context.Context, a core.Actor, conversation string, in BusinessDraftInput) (BusinessDraftInput, error) {
	tx, e := identity.BeginInteractiveRead(ctx, s.DB, a)
	if e != nil {
		return in, e
	}
	defer tx.Rollback(ctx)
	c, e := metadata(ctx, tx, a.OrgID, conversation, false)
	if e != nil {
		return in, e
	}
	if e = require(ctx, tx, a, c, "conversation.read"); e != nil {
		return in, e
	}
	ai := a
	ai.RunID = core.NewID()
	in.Arguments, e = businessDraftArguments(ctx, tx, ai, c, in)
	return in, e
}
func revisionArguments(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in BusinessRevisionInput) (BusinessRevisionBasis, json.RawMessage, error) {
	b, e := revisionBasis(ctx, tx, a, c, in.Kind, in.ResourceID)
	if e != nil {
		return b, nil, e
	}
	if b.Version != in.ExpectedVersion || b.RevisionID != in.BaseRevisionID || b.ContentHash != in.ContentHash || b.ScopeSHA256 != in.ScopeSHA256 {
		return b, nil, core.Conflict()
	}
	if b.CrossScope && !in.CrossScopeReviewed {
		return b, nil, core.Invalid("cross_scope_reviewed", "请单独核对本次跨项目或资料分类授权及影响")
	}
	if len(in.Arguments) == 0 || len(in.Arguments) > 65536 {
		return b, nil, core.Invalid("arguments", "请提供有限且完整的准确草稿修订资料")
	}
	var prepared any
	switch in.Kind {
	case "contract":
		var v contracts.Patch
		if strictArguments(in.Arguments, &v) != nil {
			return b, nil, core.Invalid("arguments", "合同修订字段无效")
		}
		v, _, e = contracts.PrepareAgentRevision(ctx, tx, a, in.ResourceID, v)
		prepared = v
	case "purchase", "claim":
		var v spend.Patch
		if strictArguments(in.Arguments, &v) != nil {
			return b, nil, core.Invalid("arguments", "采购或报销修订字段无效")
		}
		v, _, e = spend.PrepareAgentRevision(ctx, tx, a, in.Kind, in.ResourceID, v)
		prepared = v
	case "subscription":
		var v finance.SubscriptionPatch
		if strictArguments(in.Arguments, &v) != nil {
			return b, nil, core.Invalid("arguments", "订阅修订字段无效")
		}
		v, _, e = finance.PrepareAgentSubscriptionRevision(ctx, tx, a, in.ResourceID, v)
		prepared = v
	}
	if e != nil {
		return b, nil, e
	}
	raw, e := json.Marshal(prepared)
	if e != nil {
		return b, nil, e
	}
	raw, e = core.CanonicalJSON(raw)
	return b, raw, e
}
func prepareBusinessRevision(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, run string, in BusinessRevisionInput, message string) (*businessRevisionRecord, error) {
	if e := identity.RequireInteractiveSession(ctx, tx, a, true); e != nil {
		return nil, e
	}
	ai := a
	ai.RunID = run
	b, args, e := revisionArguments(ctx, tx, ai, c, in)
	if e != nil {
		return nil, e
	}
	original, _ := core.CanonicalJSON(in.Arguments)
	if string(original) != string(args) {
		return nil, core.Invalid("arguments", "请先预览整理后的准确商业资料，再确认本次授权")
	}
	in.Arguments = args
	intent, _ := json.Marshal(map[string]any{"run_id": run, "actor_id": a.UserID, "session_id": a.SessionID, "actor_auth_version": a.AuthVersion, "organization_auth_version": a.OrgAuthVersion, "conversation_id": c.ID, "conversation_revision_id": c.CurrentRevisionID, "context_security_version": c.ContextSecurityVersion, "input": in, "basis": b, "request": message})
	return &businessRevisionRecord{BusinessRevisionView: BusinessRevisionView{RunID: run, Input: in, Basis: b, IntentSHA256: core.Digest(intent), Status: "authorized"}, OrgID: a.OrgID, ActorID: a.UserID, ConversationID: c.ID, SessionID: a.SessionID, Intent: string(intent)}, nil
}
func insertBusinessRevision(ctx context.Context, tx pgx.Tx, r *businessRevisionRecord) error {
	input, _ := json.Marshal(r.Input)
	basis, _ := json.Marshal(r.Basis)
	_, e := tx.Exec(ctx, `INSERT INTO agent_business_revision_intents(run_id,org_id,actor_id,conversation_id,session_id,kind,resource_id,input,basis,arguments,intent,intent_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, r.RunID, r.OrgID, r.ActorID, r.ConversationID, r.SessionID, r.Input.Kind, r.Input.ResourceID, input, basis, r.Input.Arguments, r.Intent, r.IntentSHA256)
	return e
}
func readBusinessRevision(ctx context.Context, tx pgx.Tx, org, id string) (*businessRevisionRecord, error) {
	r := &businessRevisionRecord{}
	var input, basis, result []byte
	e := tx.QueryRow(ctx, `SELECT i.run_id,i.org_id,i.actor_id,i.conversation_id,i.session_id,i.input,i.basis,i.intent,i.intent_sha256,c.operation_id,c.result_data FROM agent_business_revision_intents i LEFT JOIN agent_business_revision_commits c ON c.org_id=i.org_id AND c.run_id=i.run_id WHERE i.org_id=$1 AND i.run_id=$2`, org, id).Scan(&r.RunID, &r.OrgID, &r.ActorID, &r.ConversationID, &r.SessionID, &input, &basis, &r.Intent, &r.IntentSHA256, &r.OperationID, &result)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if json.Unmarshal(input, &r.Input) != nil || json.Unmarshal(basis, &r.Basis) != nil {
		return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	r.Status = "authorized"
	if r.OperationID != nil {
		r.Status = "committed"
		if json.Unmarshal(result, &r.Result) != nil || r.Result == nil {
			return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
	}
	return r, nil
}
func businessRevisionSnapshot(r *businessRevisionRecord) SourceSnapshot {
	return SourceSnapshot{SourceReference: SourceReference{Type: "agent_business_revision", ID: r.RunID}, ResourceVersion: 1, SHA256: r.IntentSHA256}
}
func businessRevisionSource(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, id string, members []string) (SourceSnapshot, error) {
	r, e := readBusinessRevision(ctx, tx, a.OrgID, id)
	if e != nil {
		return SourceSnapshot{}, e
	}
	if r == nil || r.ActorID != a.UserID || r.ConversationID != c.ID || c.Type != "private" || c.CreatorID != a.UserID || len(members) != 1 || members[0] != a.UserID || core.Digest([]byte(r.Intent)) != r.IntentSHA256 {
		return SourceSnapshot{}, core.Missing()
	}
	ai := a
	ai.RunID = r.RunID
	if r.OperationID == nil {
		_, raw, e := revisionArguments(ctx, tx, ai, c, r.Input)
		if e != nil {
			return SourceSnapshot{}, e
		}
		want, _ := core.CanonicalJSON(r.Input.Arguments)
		if string(raw) != string(want) {
			return SourceSnapshot{}, runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
	} else {
		b, e := revisionBasis(ctx, tx, ai, c, r.Input.Kind, r.Input.ResourceID)
		if e != nil {
			return SourceSnapshot{}, e
		}
		if r.Result == nil || b.Version != r.Result.Version || b.RevisionID != r.Result.RevisionID || b.ContentHash != r.Result.ContentHash || b.ScopeSHA256 != r.Input.ScopeSHA256 {
			return SourceSnapshot{}, runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
		// Recheck old and newly authorized references too, not only current labels.
		if r.Input.Kind != "subscription" {
			for _, content := range []json.RawMessage{r.Basis.Content, r.Result.Content} {
				if e = workflow.AgentSnapshotReferences(ctx, tx, ai, workflow.Snapshot{Kind: r.Input.Kind, ProjectID: r.Basis.ProjectID, Classification: r.Basis.Classification, Content: content}); e != nil {
					return SourceSnapshot{}, e
				}
			}
		} else {
			for _, content := range []json.RawMessage{r.Basis.Content, r.Result.Content} {
				if e = finance.AgentSubscriptionRevisionReferences(ctx, tx, ai, r.Input.ResourceID, content); e != nil {
					return SourceSnapshot{}, e
				}
			}
		}
	}
	return businessRevisionSnapshot(r), nil
}
func executionBusinessRevisionState(ctx context.Context, tx pgx.Tx, st executionState) (*businessRevisionRecord, error) {
	r, e := readBusinessRevision(ctx, tx, st.Actor.OrgID, st.Run.ID)
	if e != nil || r == nil {
		return r, e
	}
	if st.Profile.SkillID != "oa-assist" || r.ActorID != st.Actor.UserID || r.ConversationID != st.Conversation.ID {
		return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	human := st.Actor
	human.RunID = ""
	human.SessionID = r.SessionID
	if e = identity.RequireInteractiveSession(ctx, tx, human, true); e != nil {
		return nil, e
	}
	if _, e = businessRevisionSource(ctx, tx, st.Actor, st.Conversation, r.RunID, st.Run.audience); e != nil {
		return nil, e
	}
	return r, nil
}
func revisionExpectedContent(in BusinessRevisionInput) (json.RawMessage, error) {
	var content any
	switch in.Kind {
	case "contract":
		var v contracts.Patch
		if e := json.Unmarshal(in.Arguments, &v); e != nil {
			return nil, e
		}
		content = v.Content
	case "purchase", "claim":
		var v spend.Patch
		if e := json.Unmarshal(in.Arguments, &v); e != nil {
			return nil, e
		}
		content = v.Content
	case "subscription":
		var v finance.SubscriptionPatch
		if e := json.Unmarshal(in.Arguments, &v); e != nil {
			return nil, e
		}
		content = v.SubscriptionContent
	}
	return json.Marshal(content)
}
func (s *Service) executionBusinessRevision(ctx context.Context, tx pgx.Tx, st executionState, args json.RawMessage) (map[string]any, []SourceSnapshot, error) {
	r, e := executionBusinessRevisionState(ctx, tx, st)
	if e != nil {
		return nil, nil, e
	}
	if r == nil {
		return nil, nil, runtimeFailure("BUSINESS_REVISION_INTENT_REQUIRED")
	}
	var input struct {
		RunID        string `json:"run_id"`
		IntentSHA256 string `json:"intent_sha256"`
	}
	if strictArguments(args, &input) != nil || input.RunID != r.RunID || input.IntentSHA256 != r.IntentSHA256 {
		return nil, nil, runtimeFailure("BUSINESS_REVISION_INTENT_MISMATCH")
	}
	if r.OperationID == nil {
		ai := st.Actor
		ai.RunID = st.Run.ID
		ai.SessionID = r.SessionID
		cmd := core.Command{Actor: ai, RequestID: core.NewID(), Route: "/internal/agent-business-revision/update", Key: "agent-business-revision:" + r.RunID, Body: r.Input.Arguments}
		commands := platform.Commands{DB: s.DB, Tx: tx}
		var saved core.Result
		var receipt BusinessDraftReceipt
		switch r.Input.Kind {
		case "contract":
			var v contracts.Patch
			_ = json.Unmarshal(r.Input.Arguments, &v)
			svc := contracts.Service{DB: s.DB, Commands: commands}
			saved, e = svc.Update(ctx, cmd, r.Input.ResourceID, v)
			if e == nil {
				raw, _ := json.Marshal(saved.Data)
				var out contracts.Contract
				if json.Unmarshal(raw, &out) != nil {
					return nil, nil, core.Missing()
				}
				body, _ := json.Marshal(out.Content)
				receipt = BusinessDraftReceipt{ID: out.ID, Kind: "contract", Version: out.Version, RevisionID: out.CurrentRevisionID, ContentHash: out.ContentHash, Title: out.Title, ProjectID: out.ProjectID, Classification: out.Classification, State: out.Lifecycle, Content: body}
			}
		case "purchase", "claim":
			var v spend.Patch
			_ = json.Unmarshal(r.Input.Arguments, &v)
			svc := spend.Service{DB: s.DB, Commands: commands}
			saved, e = svc.Update(ctx, cmd, r.Input.Kind, r.Input.ResourceID, v)
			if e == nil {
				raw, _ := json.Marshal(saved.Data)
				var out spend.Record
				if json.Unmarshal(raw, &out) != nil {
					return nil, nil, core.Missing()
				}
				body, _ := json.Marshal(out.Content)
				receipt = BusinessDraftReceipt{ID: out.ID, Kind: out.Kind, Version: out.Version, RevisionID: out.CurrentRevisionID, ContentHash: out.ContentHash, Title: out.Title, ProjectID: out.ProjectID, Classification: out.Classification, State: out.Lifecycle, Content: body}
			}
		case "subscription":
			var v finance.SubscriptionPatch
			_ = json.Unmarshal(r.Input.Arguments, &v)
			svc := finance.Service{DB: s.DB, Commands: commands}
			saved, e = svc.UpdateSubscription(ctx, cmd, r.Input.ResourceID, v)
			if e == nil {
				raw, _ := json.Marshal(saved.Data)
				var out finance.Subscription
				if json.Unmarshal(raw, &out) != nil {
					return nil, nil, core.Missing()
				}
				body, _ := json.Marshal(out.SubscriptionContent)
				receipt = BusinessDraftReceipt{ID: out.ID, Kind: "subscription", Version: out.Version, RevisionID: out.CurrentRevisionID, ContentHash: out.ContentHash, Title: out.Product, ProjectID: out.ProjectID, Classification: out.Classification, State: out.State, Content: body, SourceKey: out.SourceKey}
			}
		}
		if e != nil {
			return nil, nil, e
		}
		expected, e := revisionExpectedContent(r.Input)
		if e != nil {
			return nil, nil, e
		}
		want, _ := core.CanonicalJSON(expected)
		got, _ := core.CanonicalJSON(receipt.Content)
		if !core.ValidID(saved.OperationID) || receipt.ID != r.Input.ResourceID || receipt.Kind != r.Input.Kind || receipt.Version != r.Input.ExpectedVersion+1 || receipt.State != "draft" || !sameProject(receipt.ProjectID, r.Basis.ProjectID) || receipt.Classification != r.Basis.Classification || string(want) != string(got) {
			return nil, nil, runtimeFailure("BUSINESS_REVISION_RECEIPT_INVALID")
		}
		raw, _ := json.Marshal(receipt)
		_, e = tx.Exec(ctx, "INSERT INTO agent_business_revision_commits(run_id,org_id,operation_id,business_id,result_data) VALUES($1,$2,$3,$4,$5)", r.RunID, r.OrgID, saved.OperationID, receipt.ID, raw)
		if e != nil {
			return nil, nil, e
		}
		if e = executionAudit(ctx, tx, st, "agent.business_draft.revised", map[string]any{"kind": receipt.Kind, "business_id": receipt.ID, "base_revision_id": r.Input.BaseRevisionID, "revision_id": receipt.RevisionID, "intent_sha256": r.IntentSHA256, "operation_id": saved.OperationID, "cross_scope": r.Basis.CrossScope}); e != nil {
			return nil, nil, e
		}
		r, e = readBusinessRevision(ctx, tx, r.OrgID, r.RunID)
		if e != nil {
			return nil, nil, e
		}
	}
	src := businessRevisionSnapshot(r)
	return map[string]any{"status": "succeeded", "committed": true, "operation_id": r.OperationID, "resource_refs": []ResultReference{{Type: r.Input.Kind, ID: r.Input.ResourceID}}, "source_refs": []ResultReference{reference(src)}, "safe_summary": "已按本人固定资料保存普通业务草稿新修订，审批及履行仍须另行办理", "retryable": false, "next_action": "查阅准确业务新修订并继续普通流程", "content": r.BusinessRevisionView}, []SourceSnapshot{src}, nil
}
func executionBusinessRevisionText(ctx context.Context, tx pgx.Tx, st executionState, id string, version *string, offset, limit int) (any, []SourceSnapshot, error) {
	if id != st.Run.ID || version != nil {
		return nil, nil, core.Missing()
	}
	r, e := executionBusinessRevisionState(ctx, tx, st)
	if e != nil {
		return nil, nil, e
	}
	if r == nil {
		return nil, nil, core.Missing()
	}
	raw, _ := json.Marshal(r.BusinessRevisionView)
	body := []rune(string(raw))
	if offset < 0 || limit < 1 || limit > 20000 || offset > len(body) {
		return nil, nil, core.Invalid("offset", "请使用准确分页范围")
	}
	end := min(offset+limit, len(body))
	var next *int
	if end < len(body) {
		next = &end
	}
	src := businessRevisionSnapshot(r)
	return map[string]any{"type": "agent_business_revision", "id": id, "resource_version": 1, "source_sha256": src.SHA256, "text": string(body[offset:end]), "offset": offset, "next_offset": next, "total_chars": len(body), "untrusted_data": true}, []SourceSnapshot{src}, nil
}
