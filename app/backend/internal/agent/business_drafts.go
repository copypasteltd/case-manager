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

type BusinessDraftInput struct {
	Kind      string          `json:"kind"`
	Arguments json.RawMessage `json:"arguments"`
}
type BusinessDraftReceipt struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	Version        int64           `json:"version"`
	RevisionID     string          `json:"revision_id"`
	ContentHash    string          `json:"content_hash"`
	Title          string          `json:"title"`
	ProjectID      *string         `json:"project_id"`
	Classification string          `json:"classification"`
	State          string          `json:"state"`
	Content        json.RawMessage `json:"content"`
	SourceKey      string          `json:"source_key,omitempty"`
}
type BusinessDraftView struct {
	RunID        string                `json:"run_id"`
	Input        BusinessDraftInput    `json:"input"`
	IntentSHA256 string                `json:"intent_sha256"`
	Status       string                `json:"status"`
	OperationID  *string               `json:"operation_id"`
	Result       *BusinessDraftReceipt `json:"result"`
}
type businessDraftRecord struct {
	BusinessDraftView
	OrgID, ActorID, ConversationID, SessionID, Intent string
}

func businessDraftKind(kind string) bool {
	return kind == "contract" || kind == "purchase" || kind == "claim" || kind == "subscription"
}
func businessDraftArguments(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in BusinessDraftInput) (json.RawMessage, error) {
	if !businessDraftKind(in.Kind) || c.Type != "private" || c.CreatorID != a.UserID {
		return nil, core.Missing()
	}
	canonical, e := core.CanonicalJSON(in.Arguments)
	if e != nil || len(canonical) > 65536 {
		return nil, core.Invalid("business_draft", "请提供有限且完整的准确业务草稿资料")
	}
	var prepared any
	var project *string
	var class string
	switch in.Kind {
	case "contract":
		var v contracts.Create
		if strictArguments(canonical, &v) != nil {
			return nil, core.Invalid("arguments", "合同草稿字段无效")
		}
		v, e = contracts.PrepareAgentCreate(ctx, tx, a, v)
		prepared = v
		project, class = v.ProjectID, v.Classification
	case "purchase", "claim":
		var v spend.Create
		if strictArguments(canonical, &v) != nil {
			return nil, core.Invalid("arguments", "采购或报销草稿字段无效")
		}
		v, e = spend.PrepareAgentCreate(ctx, tx, a, in.Kind, v)
		prepared = v
		project, class = v.ProjectID, v.Classification
	case "subscription":
		var v finance.SubscriptionCreate
		if strictArguments(canonical, &v) != nil {
			return nil, core.Invalid("arguments", "订阅草稿字段无效")
		}
		v, e = finance.PrepareAgentSubscriptionCreate(ctx, tx, a, v)
		prepared = v
		project, class = v.ProjectID, v.Classification
	}
	if e != nil {
		return nil, e
	}
	if !sameProject(project, c.ProjectID) || class != c.Classification {
		return nil, core.Invalid("business_draft", "新业务草稿必须与本人会话项目和资料分类完全相同")
	}
	raw, e := json.Marshal(prepared)
	if e != nil {
		return nil, e
	}
	return core.CanonicalJSON(raw)
}
func prepareBusinessDraft(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, run string, in BusinessDraftInput, message string) (*businessDraftRecord, error) {
	if e := identity.RequireInteractiveSession(ctx, tx, a, false); e != nil {
		return nil, e
	}
	ai := a
	ai.RunID = run
	args, e := businessDraftArguments(ctx, tx, ai, c, in)
	if e != nil {
		return nil, e
	}
	in.Arguments = args
	intent, _ := json.Marshal(map[string]any{"run_id": run, "actor_id": a.UserID, "session_id": a.SessionID, "actor_auth_version": a.AuthVersion, "organization_auth_version": a.OrgAuthVersion, "conversation_id": c.ID, "conversation_revision_id": c.CurrentRevisionID, "context_security_version": c.ContextSecurityVersion, "input": in, "request": message})
	return &businessDraftRecord{BusinessDraftView: BusinessDraftView{RunID: run, Input: in, IntentSHA256: core.Digest(intent), Status: "authorized"}, OrgID: a.OrgID, ActorID: a.UserID, ConversationID: c.ID, SessionID: a.SessionID, Intent: string(intent)}, nil
}
func insertBusinessDraft(ctx context.Context, tx pgx.Tx, r *businessDraftRecord) error {
	_, e := tx.Exec(ctx, `INSERT INTO agent_business_draft_intents(run_id,org_id,actor_id,conversation_id,session_id,kind,arguments,intent,intent_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, r.RunID, r.OrgID, r.ActorID, r.ConversationID, r.SessionID, r.Input.Kind, r.Input.Arguments, r.Intent, r.IntentSHA256)
	return e
}
func readBusinessDraft(ctx context.Context, tx pgx.Tx, org, id string) (*businessDraftRecord, error) {
	r := &businessDraftRecord{}
	var result []byte
	e := tx.QueryRow(ctx, `SELECT i.run_id,i.org_id,i.actor_id,i.conversation_id,i.session_id,i.kind,i.arguments,i.intent,i.intent_sha256,c.operation_id,c.result_data FROM agent_business_draft_intents i LEFT JOIN agent_business_draft_commits c ON c.org_id=i.org_id AND c.run_id=i.run_id WHERE i.org_id=$1 AND i.run_id=$2`, org, id).Scan(&r.RunID, &r.OrgID, &r.ActorID, &r.ConversationID, &r.SessionID, &r.Input.Kind, &r.Input.Arguments, &r.Intent, &r.IntentSHA256, &r.OperationID, &result)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	r.Status = "authorized"
	if r.OperationID != nil {
		r.Status = "committed"
		if json.Unmarshal(result, &r.Result) != nil || r.Result == nil {
			return nil, runtimeFailure("BUSINESS_DRAFT_RECEIPT_INVALID")
		}
	}
	return r, nil
}
func businessDraftSnapshot(r *businessDraftRecord) SourceSnapshot {
	return SourceSnapshot{SourceReference: SourceReference{Type: "agent_business_draft", ID: r.RunID}, ResourceVersion: 1, SHA256: r.IntentSHA256}
}
func businessDraftSource(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, id string, members []string) (SourceSnapshot, error) {
	r, e := readBusinessDraft(ctx, tx, a.OrgID, id)
	if e != nil {
		return SourceSnapshot{}, e
	}
	if r == nil || r.ActorID != a.UserID || r.ConversationID != c.ID || c.Type != "private" || c.CreatorID != a.UserID || len(members) != 1 || members[0] != a.UserID || core.Digest([]byte(r.Intent)) != r.IntentSHA256 {
		return SourceSnapshot{}, core.Missing()
	}
	ai := a
	ai.RunID = r.RunID
	if r.OperationID == nil {
		if _, e = businessDraftArguments(ctx, tx, ai, c, r.Input); e != nil {
			return SourceSnapshot{}, e
		}
	} else {
		v := r.Result
		if v == nil || !sameProject(v.ProjectID, c.ProjectID) || v.Classification != c.Classification {
			return SourceSnapshot{}, core.Missing()
		}
		if v.Kind == "subscription" {
			if e = finance.AgentSubscriptionDraftRead(ctx, tx, ai, v.ID, r.Input.Arguments); e != nil {
				return SourceSnapshot{}, e
			}
		} else {
			if _, e = workflow.AgentBusinessRead(ctx, tx, ai, v.Kind, v.ID, false); e != nil {
				return SourceSnapshot{}, e
			}
			if e = workflow.AgentSnapshotReferences(ctx, tx, ai, workflow.Snapshot{Kind: v.Kind, ProjectID: v.ProjectID, Classification: v.Classification, Content: v.Content}); e != nil {
				return SourceSnapshot{}, e
			}
		}
	}
	return businessDraftSnapshot(r), nil
}
func executionBusinessDraftState(ctx context.Context, tx pgx.Tx, st executionState) (*businessDraftRecord, error) {
	r, e := readBusinessDraft(ctx, tx, st.Actor.OrgID, st.Run.ID)
	if e != nil || r == nil {
		return r, e
	}
	if st.Profile.SkillID != "oa-assist" || r.ActorID != st.Actor.UserID || r.ConversationID != st.Conversation.ID {
		return nil, runtimeFailure("BUSINESS_DRAFT_AUTHORITY_MISMATCH")
	}
	human := st.Actor
	human.RunID = ""
	human.SessionID = r.SessionID
	if e = identity.RequireInteractiveSession(ctx, tx, human, false); e != nil {
		return nil, e
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	if _, e = businessDraftSource(ctx, tx, ai, st.Conversation, r.RunID, st.Run.audience); e != nil {
		return nil, e
	}
	if r.OperationID == nil {
		args, e := businessDraftArguments(ctx, tx, ai, st.Conversation, r.Input)
		if e != nil {
			return nil, e
		}
		want, _ := core.CanonicalJSON(r.Input.Arguments)
		if string(args) != string(want) {
			return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
	}
	return r, nil
}
func businessDraftExpectedContent(in BusinessDraftInput) (json.RawMessage, string, string, error) {
	var content any
	var title, source string
	switch in.Kind {
	case "contract":
		var v contracts.Create
		if json.Unmarshal(in.Arguments, &v) != nil {
			return nil, "", "", core.Missing()
		}
		content, title = v.Content, v.Title
	case "purchase", "claim":
		var v spend.Create
		if json.Unmarshal(in.Arguments, &v) != nil {
			return nil, "", "", core.Missing()
		}
		content, title = v.Content, v.Title
	case "subscription":
		var v finance.SubscriptionCreate
		if json.Unmarshal(in.Arguments, &v) != nil {
			return nil, "", "", core.Missing()
		}
		content, title, source = v.SubscriptionContent, v.Product, v.SourceKey
	default:
		return nil, "", "", core.Missing()
	}
	raw, e := json.Marshal(content)
	return raw, title, source, e
}
func (s *Service) executionBusinessDraft(ctx context.Context, tx pgx.Tx, st executionState, args json.RawMessage) (map[string]any, []SourceSnapshot, error) {
	r, e := executionBusinessDraftState(ctx, tx, st)
	if e != nil {
		return nil, nil, e
	}
	if r == nil {
		return nil, nil, runtimeFailure("BUSINESS_DRAFT_INTENT_REQUIRED")
	}
	var in struct {
		RunID        string `json:"run_id"`
		IntentSHA256 string `json:"intent_sha256"`
	}
	if strictArguments(args, &in) != nil || in.RunID != r.RunID || in.IntentSHA256 != r.IntentSHA256 {
		return nil, nil, runtimeFailure("BUSINESS_DRAFT_INTENT_MISMATCH")
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	ai.SessionID = r.SessionID
	if r.OperationID == nil {
		cmd := core.Command{Actor: ai, RequestID: core.NewID(), Route: "/internal/agent-business-draft/create", Key: "agent-business-draft:" + r.RunID, Body: r.Input.Arguments}
		commands := platform.Commands{DB: s.DB, Tx: tx}
		var saved core.Result
		var receipt BusinessDraftReceipt
		switch r.Input.Kind {
		case "contract":
			var input contracts.Create
			_ = json.Unmarshal(r.Input.Arguments, &input)
			service := contracts.Service{DB: s.DB, Commands: commands}
			saved, e = service.Create(ctx, cmd, input)
			if e == nil {
				raw, _ := json.Marshal(saved.Data)
				var v contracts.Contract
				if json.Unmarshal(raw, &v) != nil {
					return nil, nil, runtimeFailure("BUSINESS_DRAFT_RECEIPT_INVALID")
				}
				body, _ := json.Marshal(v.Content)
				receipt = BusinessDraftReceipt{ID: v.ID, Kind: "contract", Version: v.Version, RevisionID: v.CurrentRevisionID, ContentHash: v.ContentHash, Title: v.Title, ProjectID: v.ProjectID, Classification: v.Classification, State: v.Lifecycle, Content: body}
				if v.CreatorID != ai.UserID || v.ApprovalState != "not_submitted" {
					return nil, nil, runtimeFailure("BUSINESS_DRAFT_RECEIPT_INVALID")
				}
			}
		case "purchase", "claim":
			var input spend.Create
			_ = json.Unmarshal(r.Input.Arguments, &input)
			service := spend.Service{DB: s.DB, Commands: commands}
			saved, e = service.Create(ctx, cmd, r.Input.Kind, input)
			if e == nil {
				raw, _ := json.Marshal(saved.Data)
				var v spend.Record
				if json.Unmarshal(raw, &v) != nil {
					return nil, nil, runtimeFailure("BUSINESS_DRAFT_RECEIPT_INVALID")
				}
				body, _ := json.Marshal(v.Content)
				receipt = BusinessDraftReceipt{ID: v.ID, Kind: v.Kind, Version: v.Version, RevisionID: v.CurrentRevisionID, ContentHash: v.ContentHash, Title: v.Title, ProjectID: v.ProjectID, Classification: v.Classification, State: v.Lifecycle, Content: body}
				if v.CreatorID != ai.UserID || v.ApprovalState != "not_submitted" {
					return nil, nil, runtimeFailure("BUSINESS_DRAFT_RECEIPT_INVALID")
				}
			}
		case "subscription":
			var input finance.SubscriptionCreate
			_ = json.Unmarshal(r.Input.Arguments, &input)
			service := finance.Service{DB: s.DB, Commands: commands}
			saved, e = service.CreateSubscription(ctx, cmd, input)
			if e == nil {
				raw, _ := json.Marshal(saved.Data)
				var v finance.Subscription
				if json.Unmarshal(raw, &v) != nil {
					return nil, nil, runtimeFailure("BUSINESS_DRAFT_RECEIPT_INVALID")
				}
				body, _ := json.Marshal(v.SubscriptionContent)
				receipt = BusinessDraftReceipt{ID: v.ID, Kind: "subscription", Version: v.Version, RevisionID: v.CurrentRevisionID, ContentHash: v.ContentHash, Title: v.Product, ProjectID: v.ProjectID, Classification: v.Classification, State: v.State, Content: body, SourceKey: v.SourceKey}
				if v.CreatorID != ai.UserID {
					return nil, nil, runtimeFailure("BUSINESS_DRAFT_RECEIPT_INVALID")
				}
			}
		}
		if e != nil {
			return nil, nil, e
		}
		expected, title, source, e := businessDraftExpectedContent(r.Input)
		if e != nil {
			return nil, nil, e
		}
		want, _ := core.CanonicalJSON(expected)
		got, _ := core.CanonicalJSON(receipt.Content)
		if !core.ValidID(saved.OperationID) || receipt.Version != 1 || receipt.State != "draft" || receipt.Kind != r.Input.Kind || receipt.Title != title || receipt.SourceKey != source || !sameProject(receipt.ProjectID, st.Conversation.ProjectID) || receipt.Classification != st.Conversation.Classification || string(want) != string(got) {
			return nil, nil, runtimeFailure("BUSINESS_DRAFT_RECEIPT_INVALID")
		}
		raw, _ := json.Marshal(receipt)
		_, e = tx.Exec(ctx, `INSERT INTO agent_business_draft_commits(run_id,org_id,operation_id,business_id,result_data) VALUES($1,$2,$3,$4,$5)`, r.RunID, r.OrgID, saved.OperationID, receipt.ID, raw)
		if e != nil {
			return nil, nil, e
		}
		r, e = readBusinessDraft(ctx, tx, r.OrgID, r.RunID)
		if e != nil {
			return nil, nil, e
		}
	}
	src, e := businessDraftSource(ctx, tx, ai, st.Conversation, r.RunID, st.Run.audience)
	if e != nil {
		return nil, nil, e
	}
	return map[string]any{"status": "succeeded", "committed": true, "operation_id": r.OperationID, "resource_refs": []ResultReference{{Type: r.Input.Kind, ID: r.Result.ID}}, "source_refs": []ResultReference{reference(src)}, "safe_summary": "已按本人固定资料创建业务草稿；请另行核对、送审及履行", "retryable": false, "next_action": "查看准确草稿并继续普通业务流程", "content": r.BusinessDraftView}, []SourceSnapshot{src}, nil
}
func executionBusinessDraftText(ctx context.Context, tx pgx.Tx, st executionState, id string, version *string, offset, limit int) (any, []SourceSnapshot, error) {
	if id != st.Run.ID || version != nil || offset < 0 || limit < 1 || limit > 10000 {
		return nil, nil, core.Invalid("arguments", "请按本次授权读取有限业务草稿资料")
	}
	r, e := executionBusinessDraftState(ctx, tx, st)
	if e != nil {
		return nil, nil, e
	}
	if r == nil {
		return nil, nil, core.Missing()
	}
	raw, _ := json.Marshal(r.BusinessDraftView)
	body := []rune(string(raw))
	if offset > len(body) {
		return nil, nil, core.Invalid("offset", "正文位置无效")
	}
	end := min(offset+limit, len(body))
	var next *int
	if end < len(body) {
		next = &end
	}
	src := businessDraftSnapshot(r)
	return map[string]any{"type": "agent_business_draft", "id": id, "resource_version": 1, "source_sha256": src.SHA256, "text": string(body[offset:end]), "offset": offset, "next_offset": next, "total_chars": len(body), "untrusted_data": true}, []SourceSnapshot{src}, nil
}

// Keep ordinary read authority for the selected creation scope, including AI
// classification policy, before returning human-provided data to a model.
func businessDraftReadScope(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, kind string) error {
	return authz.Require(ctx, tx, a, kind+".read", authz.Resource{Type: kind, ProjectID: c.ProjectID, OwnerID: &a.UserID, Classification: c.Classification})
}
