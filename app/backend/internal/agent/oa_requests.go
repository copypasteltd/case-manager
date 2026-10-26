package agent

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/workflow"
	"github.com/jackc/pgx/v5"
)

type OARequestInput struct {
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}
type OADraftArguments struct {
	workflow.RequestCreate
	RequestID       *string `json:"request_id,omitempty"`
	ExpectedVersion *int64  `json:"expected_version,omitempty"`
}
type OASubmitArguments struct {
	RequestID string `json:"request_id"`
	workflow.SubmitInput
}
type OARequestBasis struct {
	Business workflow.AgentBusinessView  `json:"business"`
	Request  *workflow.Request           `json:"request"`
	Preview  *workflow.SubmissionPreview `json:"preview"`
}
type OARequestView struct {
	RunID        string            `json:"run_id"`
	Tool         string            `json:"tool"`
	Kind         string            `json:"kind"`
	BusinessID   string            `json:"business_id"`
	RequestID    *string           `json:"request_id"`
	Arguments    json.RawMessage   `json:"arguments"`
	Before       OARequestBasis    `json:"before"`
	IntentSHA256 string            `json:"intent_sha256"`
	Status       string            `json:"status"`
	OperationID  *string           `json:"operation_id"`
	Result       *workflow.Request `json:"result"`
}
type oaRequestRecord struct {
	OARequestView
	OrgID, ActorID, ConversationID, SessionID, BaseSHA256, Intent string
}

var oaDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func oaRequestTool(tool string) bool { return tool == "draft_request" || tool == "submit_request" }

func executionOARequestText(ctx context.Context, tx pgx.Tx, st executionState, id string, version *string, offset, maxChars int) (any, []SourceSnapshot, error) {
	if id != st.Run.ID || version != nil || offset < 0 || offset > 524288 || maxChars < 1 || maxChars > 10000 {
		return nil, nil, core.Invalid("arguments", "请指定本次申请授权和有限字符范围")
	}
	r, err := executionOARequestState(ctx, tx, st)
	if err != nil {
		return nil, nil, err
	}
	if r == nil {
		return nil, nil, core.Missing()
	}
	// Public authorization and its actual result exclude the original session ID.
	raw, _ := json.Marshal(r.OARequestView)
	body := []rune(string(raw))
	if offset > len(body) {
		return nil, nil, core.Invalid("offset", "字符范围无效")
	}
	end := min(offset+maxChars, len(body))
	var next *int
	if end < len(body) {
		next = &end
	}
	src := oaRequestSnapshot(r)
	return map[string]any{"type": "agent_oa_request", "id": id, "resource_version": 1, "source_sha256": src.SHA256, "text": string(body[offset:end]), "offset": offset, "next_offset": next, "total_chars": len(body), "status": r.Status, "operation_id": r.OperationID, "untrusted_data": true}, []SourceSnapshot{src}, nil
}
func exactOAID(id string) bool { return core.ValidID(id) && id == strings.ToLower(id) }

func parseOARequest(in OARequestInput) (json.RawMessage, error) {
	canonical, err := core.CanonicalJSON(in.Arguments)
	if err != nil || !oaRequestTool(in.Tool) || len(canonical) > 65536 {
		return nil, core.Invalid("oa_request", "请选择已开放的准确申请操作")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(canonical, &fields) != nil || len(fields) == 0 {
		return nil, core.Invalid("oa_request", "申请操作必须包含明确字段")
	}
	for _, value := range fields {
		if string(value) == "null" {
			return nil, core.Invalid("oa_request", "准确申请字段不能使用空值")
		}
	}
	valid := false
	if in.Tool == "draft_request" {
		var args OADraftArguments
		if strictArguments(canonical, &args) == nil {
			valid = slices.Contains([]string{"contract", "purchase", "claim"}, args.Kind) && exactOAID(args.BusinessID) && exactOAID(args.BusinessRevisionID) && oaDigestPattern.MatchString(args.BusinessContentHash) && core.RequireVersion(args.ExpectedBusinessVersion) == nil && core.Name(args.Reason) == nil
			valid = valid && ((args.RequestID == nil && args.ExpectedVersion == nil) || (args.RequestID != nil && args.ExpectedVersion != nil && exactOAID(*args.RequestID) && core.RequireVersion(*args.ExpectedVersion) == nil))
		}
	} else {
		var args OASubmitArguments
		if strictArguments(canonical, &args) == nil {
			valid = exactOAID(args.RequestID) && exactOAID(args.RevisionID) && exactOAID(args.WorkflowDefinitionID) && oaDigestPattern.MatchString(args.ContentHash) && oaDigestPattern.MatchString(args.PreviewHash) && core.RequireVersion(args.ExpectedVersion) == nil && core.Name(args.Reason) == nil
		}
	}
	if !valid {
		return nil, core.Invalid("oa_request", "请核对业务、申请修订、内容摘要、审批预览及操作说明")
	}
	return canonical, nil
}

func readOARequestRecord(ctx context.Context, tx pgx.Tx, org, runID string) (*oaRequestRecord, error) {
	var r oaRequestRecord
	var before, result []byte
	err := tx.QueryRow(ctx, `SELECT i.run_id,i.org_id,i.actor_id,i.conversation_id,i.session_id,i.tool,i.kind,i.business_id,i.request_id,i.arguments,i.before_data,i.base_sha256,i.intent,i.intent_sha256,c.operation_id,c.result_data
 FROM agent_oa_intents i LEFT JOIN agent_oa_commits c ON c.org_id=i.org_id AND c.run_id=i.run_id WHERE i.org_id=$1 AND i.run_id=$2`, org, runID).
		Scan(&r.RunID, &r.OrgID, &r.ActorID, &r.ConversationID, &r.SessionID, &r.Tool, &r.Kind, &r.BusinessID, &r.RequestID, &r.Arguments, &before, &r.BaseSHA256, &r.Intent, &r.IntentSHA256, &r.OperationID, &result)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(before, &r.Before); err != nil {
		return nil, err
	}
	r.Status = "authorized"
	if r.OperationID != nil {
		r.Status = "committed"
		if err = json.Unmarshal(result, &r.Result); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

func oaBasis(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, tool, kind, businessID string, requestID *string, members []string, writable bool, human *core.Actor) (OARequestBasis, error) {
	var out OARequestBasis
	var err error
	if c.Type != "private" || c.CreatorID != a.UserID {
		return out, core.Missing()
	}
	out.Business, err = workflow.AgentBusinessRead(ctx, tx, a, kind, businessID, writable)
	if err != nil {
		return out, err
	}
	if !sameProject(c.ProjectID, out.Business.ProjectID) || classRank(out.Business.Classification) == 0 || classRank(out.Business.Classification) > classRank(c.Classification) {
		return out, core.Missing()
	}
	for _, user := range members {
		reader := a
		reader.UserID = user
		if _, err = workflow.AgentBusinessRead(ctx, tx, reader, kind, businessID, false); err != nil {
			return out, err
		}
		if requestID != nil {
			r, e := workflow.AgentRequestRead(ctx, tx, reader, *requestID)
			if e != nil {
				return out, e
			}
			if r.Kind != kind || r.BusinessID != businessID || !sameProject(c.ProjectID, r.ProjectID) {
				return out, core.Missing()
			}
			out.Request = &r
		}
	}
	if writable && tool == "draft_request" {
		if err = workflow.AgentDraftPermission(ctx, tx, a, out.Business, out.Request); err != nil {
			return out, err
		}
	}
	if tool == "submit_request" && writable {
		if requestID == nil {
			return out, core.Missing()
		}
		actor := a
		if human != nil {
			actor = *human
		}
		preview, e := workflow.SubmissionPreviewInTx(ctx, tx, actor, *requestID)
		if e != nil {
			return out, e
		}
		out.Preview = &preview
	}
	return out, nil
}

func prepareOARequest(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, runID string, in OARequestInput, members []string, message string) (*oaRequestRecord, error) {
	if err := identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return nil, err
	}
	canonical, err := parseOARequest(in)
	if err != nil {
		return nil, err
	}
	r := &oaRequestRecord{OARequestView: OARequestView{RunID: runID, Tool: in.Tool, Arguments: canonical, Status: "authorized"}, OrgID: a.OrgID, ActorID: a.UserID, ConversationID: c.ID, SessionID: a.SessionID}
	var draft OADraftArguments
	var submit OASubmitArguments
	ai := a
	ai.RunID = runID
	if in.Tool == "draft_request" {
		_ = json.Unmarshal(canonical, &draft)
		r.Kind, r.BusinessID, r.RequestID = draft.Kind, draft.BusinessID, draft.RequestID
	} else {
		_ = json.Unmarshal(canonical, &submit)
		request, e := workflow.AgentRequestRead(ctx, tx, ai, submit.RequestID)
		if e != nil {
			return nil, e
		}
		r.Kind, r.BusinessID, r.RequestID = request.Kind, request.BusinessID, &submit.RequestID
	}
	r.Before, err = oaBasis(ctx, tx, ai, c, r.Tool, r.Kind, r.BusinessID, r.RequestID, members, true, &a)
	if err != nil {
		return nil, err
	}
	if in.Tool == "draft_request" {
		b := r.Before.Business
		if b.Version != draft.ExpectedBusinessVersion || b.CurrentRevisionID != draft.BusinessRevisionID || b.ContentHash != draft.BusinessContentHash || (draft.RequestID != nil && (r.Before.Request == nil || r.Before.Request.Version != *draft.ExpectedVersion)) {
			return nil, core.Conflict()
		}
	} else {
		p := r.Before.Preview
		if p == nil || p.RequestVersion != submit.ExpectedVersion || p.RevisionID != submit.RevisionID || p.ContentHash != submit.ContentHash || p.Workflow.ID != submit.WorkflowDefinitionID || p.PreviewHash != submit.PreviewHash {
			return nil, core.Fail(409, "SUBMISSION_PREVIEW_CHANGED", "送审预览已变化，请重新核对")
		}
	}
	before, _ := json.Marshal(r.Before)
	r.BaseSHA256 = core.Digest(before)
	intent, _ := json.Marshal(map[string]any{"run_id": runID, "actor_id": a.UserID, "actor_auth_version": a.AuthVersion, "organization_auth_version": a.OrgAuthVersion, "conversation_id": c.ID, "conversation_revision_id": c.CurrentRevisionID, "context_security_version": c.ContextSecurityVersion, "session_id": a.SessionID, "tool": r.Tool, "kind": r.Kind, "business_id": r.BusinessID, "request_id": r.RequestID, "arguments": json.RawMessage(canonical), "before": r.Before, "base_sha256": r.BaseSHA256, "request": message})
	if len(intent) > 512*1024 {
		return nil, core.Invalid("oa_request", "本次准确申请资料超过512KiB，请拆分整理后重新核对")
	}
	r.Intent, r.IntentSHA256 = string(intent), core.Digest(intent)
	return r, nil
}

func insertOARequest(ctx context.Context, tx pgx.Tx, r *oaRequestRecord) error {
	before, _ := json.Marshal(r.Before)
	_, err := tx.Exec(ctx, `INSERT INTO agent_oa_intents(run_id,org_id,actor_id,conversation_id,session_id,tool,kind,business_id,request_id,arguments,before_data,base_sha256,intent,intent_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, r.RunID, r.OrgID, r.ActorID, r.ConversationID, r.SessionID, r.Tool, r.Kind, r.BusinessID, r.RequestID, r.Arguments, before, r.BaseSHA256, r.Intent, r.IntentSHA256)
	return err
}

func oaRequestSnapshot(r *oaRequestRecord) SourceSnapshot {
	return SourceSnapshot{SourceReference: SourceReference{Type: "agent_oa_request", ID: r.RunID}, ResourceVersion: 1, SHA256: r.IntentSHA256}
}

func oaRequestSource(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, id string, members []string) (SourceSnapshot, error) {
	r, err := readOARequestRecord(ctx, tx, a.OrgID, id)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if r == nil || r.ConversationID != c.ID || r.ActorID != a.UserID || core.Digest([]byte(r.Intent)) != r.IntentSHA256 {
		return SourceSnapshot{}, core.Missing()
	}
	requestID := r.RequestID
	if r.Result != nil {
		requestID = &r.Result.ID
	}
	ai := a
	ai.RunID = r.RunID
	if _, err = oaBasis(ctx, tx, ai, c, r.Tool, r.Kind, r.BusinessID, requestID, members, false, nil); err != nil {
		return SourceSnapshot{}, err
	}
	for _, user := range members {
		reader := ai
		reader.UserID = user
		if err = workflow.AgentSnapshotReferences(ctx, tx, reader, r.Before.Business.Payload); err != nil {
			return SourceSnapshot{}, err
		}
		if r.Before.Request != nil {
			if err = workflow.AgentSnapshotReferences(ctx, tx, reader, r.Before.Request.Payload); err != nil {
				return SourceSnapshot{}, err
			}
		}
		if r.Result != nil {
			if err = workflow.AgentSnapshotReferences(ctx, tx, reader, r.Result.Payload); err != nil {
				return SourceSnapshot{}, err
			}
		}
	}
	return oaRequestSnapshot(r), nil
}

func executionOARequestState(ctx context.Context, tx pgx.Tx, st executionState) (*oaRequestRecord, error) {
	r, err := readOARequestRecord(ctx, tx, st.Actor.OrgID, st.Run.ID)
	if err != nil || r == nil {
		return r, err
	}
	if st.Profile.SkillID != "oa-assist" || r.ActorID != st.Actor.UserID || r.ConversationID != st.Conversation.ID {
		return nil, runtimeFailure("OA_REQUEST_AUTHORITY_MISMATCH")
	}
	human := st.Actor
	human.SessionID = r.SessionID
	if err = identity.RequireInteractiveSession(ctx, tx, human, false); err != nil {
		return nil, err
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	if _, err = oaRequestSource(ctx, tx, ai, st.Conversation, r.RunID, st.Run.audience); err != nil {
		return nil, err
	}
	if r.OperationID == nil {
		basis, e := oaBasis(ctx, tx, ai, st.Conversation, r.Tool, r.Kind, r.BusinessID, r.RequestID, st.Run.audience, true, nil)
		if e != nil {
			return nil, e
		}
		raw, _ := json.Marshal(basis)
		if core.Digest(raw) != r.BaseSHA256 {
			return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
	}
	return r, nil
}

func (s *Service) executionOARequest(ctx context.Context, tx pgx.Tx, st executionState, tool string, args json.RawMessage) (map[string]any, []SourceSnapshot, error) {
	r, err := executionOARequestState(ctx, tx, st)
	if err != nil {
		return nil, nil, err
	}
	if r == nil {
		return nil, nil, runtimeFailure("OA_REQUEST_INTENT_REQUIRED")
	}
	canonical, err := parseOARequest(OARequestInput{Tool: tool, Arguments: args})
	if err != nil {
		return nil, nil, err
	}
	want, _ := core.CanonicalJSON(r.Arguments)
	if tool != r.Tool || string(want) != string(canonical) {
		return nil, nil, runtimeFailure("OA_REQUEST_INTENT_MISMATCH")
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	if r.OperationID == nil {
		command := core.Command{Actor: ai, RequestID: core.NewID(), Route: "/internal/agent-oa/" + tool, Key: "agent-oa:" + r.RunID, Body: canonical}
		business := workflow.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB, Tx: tx}}
		var result core.Result
		if tool == "draft_request" {
			var in OADraftArguments
			_ = json.Unmarshal(canonical, &in)
			if in.RequestID == nil {
				result, err = business.CreateRequest(ctx, command, in.RequestCreate)
			} else {
				result, err = business.ReviseRequest(ctx, command, *in.RequestID, workflow.RequestRevise{ExpectedVersion: *in.ExpectedVersion, ExpectedBusinessVersion: in.ExpectedBusinessVersion, BusinessRevisionID: in.BusinessRevisionID, BusinessContentHash: in.BusinessContentHash, Reason: in.Reason})
			}
		} else {
			var in OASubmitArguments
			_ = json.Unmarshal(canonical, &in)
			result, err = business.SubmitRequest(ctx, command, in.RequestID, in.SubmitInput)
		}
		if err != nil {
			return nil, nil, err
		}
		raw, err := json.Marshal(result.Data)
		if err != nil {
			return nil, nil, err
		}
		var request workflow.Request
		if json.Unmarshal(raw, &request) != nil || !core.ValidID(result.OperationID) || request.BusinessID != r.BusinessID || request.Kind != r.Kind || request.ApplicantID != ai.UserID || (tool == "submit_request" && (request.State != "in_review" || request.Workflow == nil || request.ActiveInstanceID == nil || len(request.Steps) == 0)) || (tool == "draft_request" && request.State != "draft") {
			return nil, nil, runtimeFailure("OA_REQUEST_RECEIPT_INVALID")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO agent_oa_commits(run_id,org_id,operation_id,request_id,result_data) VALUES($1,$2,$3,$4,$5)`, r.RunID, r.OrgID, result.OperationID, request.ID, raw); err != nil {
			return nil, nil, err
		}
		r, err = readOARequestRecord(ctx, tx, r.OrgID, r.RunID)
		if err != nil {
			return nil, nil, err
		}
	}
	src, err := oaRequestSource(ctx, tx, ai, st.Conversation, r.RunID, st.Run.audience)
	if err != nil {
		return nil, nil, err
	}
	return map[string]any{"status": "succeeded", "committed": true, "operation_id": r.OperationID, "resource_refs": []ResultReference{{Type: "request", ID: r.Result.ID}}, "source_refs": []ResultReference{reference(src)}, "safe_summary": "已按本次明确授权保存申请草稿或提交审核；审批、签署和付款仍分别处理", "retryable": false, "next_action": "", "content": r.OARequestView}, []SourceSnapshot{src}, nil
}
