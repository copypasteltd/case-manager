package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/spend"
	"copypaste.hk/platform/internal/workflow"
	"github.com/jackc/pgx/v5"
)

// Only these prose fields can be proposed. Commercial facts remain the exact
// original snapshot and are never accepted from model output or the adopt API.
type OATextArguments struct {
	Kind                    string   `json:"kind"`
	BusinessID              string   `json:"business_id"`
	ExpectedBusinessVersion int64    `json:"expected_business_version"`
	BusinessRevisionID      string   `json:"business_revision_id"`
	BusinessContentHash     string   `json:"business_content_hash"`
	Fields                  []string `json:"fields"`
	Reason                  string   `json:"reason"`
}
type OATextDraftView struct {
	Fields      map[string]string           `json:"fields"`
	DraftSHA256 string                      `json:"draft_sha256"`
	SourceRefs  []SourceSnapshot            `json:"source_refs"`
	OperationID *string                     `json:"operation_id"`
	Result      *workflow.AgentBusinessView `json:"result"`
}
type OATextAdopt struct {
	DraftSHA256 string `json:"draft_sha256"`
	Reviewed    bool   `json:"reviewed"`
	Reason      string `json:"reason"`
}

func oaTextFields(kind string) []string {
	if kind == "contract" {
		return []string{"renewal_terms", "termination_terms", "notes"}
	}
	return []string{"purpose", "notes"}
}

// A stable line identity, never a mutable array position, binds each selection.
func oaTextLineID(field string) string {
	if !strings.HasPrefix(field, "line:") || !strings.HasSuffix(field, ":description") {
		return ""
	}
	id := strings.TrimSuffix(strings.TrimPrefix(field, "line:"), ":description")
	if !exactOAID(id) {
		return ""
	}
	return id
}

func oaTextFieldLimit(kind, field string) int {
	if slices.Contains(oaTextFields(kind), field) {
		if field == "purpose" {
			return 4000
		}
		return 20000
	}
	if (kind == "purchase" || kind == "claim") && oaTextLineID(field) != "" {
		return 1000
	}
	return 0
}

func validOATextSelection(kind string, fields []string, content json.RawMessage) bool {
	var body struct {
		Lines []struct {
			ID string `json:"id"`
		} `json:"lines"`
	}
	if err := json.Unmarshal(content, &body); err != nil {
		return false
	}
	for _, field := range fields {
		if oaTextFieldLimit(kind, field) == 0 {
			return false
		}
		if id := oaTextLineID(field); id != "" {
			count := 0
			for _, line := range body.Lines {
				if line.ID == id {
					count++
				}
			}
			if count != 1 {
				return false
			}
		}
	}
	return true
}

func patchOATextFields(kind string, content json.RawMessage, fields map[string]string) (json.RawMessage, error) {
	selected := make([]string, 0, len(fields))
	for field := range fields {
		selected = append(selected, field)
	}
	if !validOATextSelection(kind, selected, content) {
		return nil, core.Invalid("fields", "所选正文或明细已不属于准确业务修订")
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(content, &body); err != nil {
		return nil, err
	}
	var lines []map[string]json.RawMessage
	changedLines := false
	for field, text := range fields {
		encoded, _ := json.Marshal(text)
		if id := oaTextLineID(field); id != "" {
			if lines == nil {
				if err := json.Unmarshal(body["lines"], &lines); err != nil {
					return nil, err
				}
			}
			for _, line := range lines {
				var lineID string
				_ = json.Unmarshal(line["id"], &lineID)
				if lineID == id {
					line["description"] = encoded
				}
			}
			changedLines = true
		} else {
			body[field] = encoded
		}
	}
	if changedLines {
		body["lines"], _ = json.Marshal(lines)
	}
	return json.Marshal(body)
}

func validOATextArguments(in OATextArguments) bool {
	if !slices.Contains([]string{"contract", "purchase", "claim"}, in.Kind) || !exactOAID(in.BusinessID) || !exactOAID(in.BusinessRevisionID) || !oaDigestPattern.MatchString(in.BusinessContentHash) || core.RequireVersion(in.ExpectedBusinessVersion) != nil || core.Name(in.Reason) != nil || len(in.Fields) == 0 || len(in.Fields) > 102 {
		return false
	}
	seen := map[string]bool{}
	for _, field := range in.Fields {
		if seen[field] || oaTextFieldLimit(in.Kind, field) == 0 {
			return false
		}
		seen[field] = true
	}
	return true
}
func readOATextDraft(ctx context.Context, tx pgx.Tx, r *oaRequestRecord) error {
	var fields, result, sources []byte
	var draft OATextDraftView
	err := tx.QueryRow(ctx, `SELECT d.fields,d.draft_sha256,d.source_snapshots,a.operation_id,a.result_data FROM agent_oa_text_drafts d LEFT JOIN agent_oa_text_adoptions a ON a.org_id=d.org_id AND a.run_id=d.run_id WHERE d.org_id=$1 AND d.run_id=$2`, r.OrgID, r.RunID).Scan(&fields, &draft.DraftSHA256, &sources, &draft.OperationID, &result)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal(fields, &draft.Fields); err != nil {
		return err
	}
	if err = json.Unmarshal(sources, &draft.SourceRefs); err != nil {
		return err
	}
	r.Status = "drafted"
	if draft.OperationID != nil {
		if err = json.Unmarshal(result, &draft.Result); err != nil {
			return err
		}
		r.Status = "adopted"
	}
	r.TextDraft = &draft
	return nil
}
func executionOATextDraft(ctx context.Context, tx pgx.Tx, st executionState, r *oaRequestRecord, args json.RawMessage, observed []SourceSnapshot) (map[string]any, []SourceSnapshot, error) {
	if r.Tool != "draft_business_text" {
		return nil, nil, runtimeFailure("OA_REQUEST_INTENT_MISMATCH")
	}
	var in struct {
		RunID        string            `json:"run_id"`
		IntentSHA256 string            `json:"intent_sha256"`
		Fields       map[string]string `json:"fields"`
	}
	if err := strictArguments(args, &in); err != nil {
		return nil, nil, err
	}
	var intent OATextArguments
	if err := json.Unmarshal(r.Arguments, &intent); err != nil {
		return nil, nil, err
	}
	if in.RunID != r.RunID || in.IntentSHA256 != r.IntentSHA256 || len(in.Fields) != len(intent.Fields) || !validOATextArguments(intent) || !validOATextSelection(r.Kind, intent.Fields, r.Before.Business.Payload.Content) {
		return nil, nil, runtimeFailure("OA_REQUEST_INTENT_MISMATCH")
	}
	for _, field := range intent.Fields {
		text, exists := in.Fields[field]
		if field == "purpose" || oaTextLineID(field) != "" {
			text = strings.TrimSpace(text)
			in.Fields[field] = text
		}
		limit := oaTextFieldLimit(r.Kind, field)
		if !exists || strings.TrimSpace(text) == "" || !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') || utf8.RuneCountInString(text) > limit {
			return nil, nil, core.Invalid("fields", "请仅提供本人选择字段的完整有限正文")
		}
	}
	raw, _ := json.Marshal(in.Fields)
	digest := core.Digest(raw)
	if r.TextDraft != nil && r.TextDraft.DraftSHA256 != digest {
		return nil, nil, runtimeFailure("OA_TEXT_DRAFT_ALREADY_PROPOSED")
	}
	if r.TextDraft == nil {
		sources, err := mergeExecutionSources(st.Sources, observed, []SourceSnapshot{oaRequestSnapshot(r)})
		if err != nil {
			return nil, nil, err
		}
		bound, _ := json.Marshal(sources)
		if _, err := tx.Exec(ctx, `INSERT INTO agent_oa_text_drafts(run_id,org_id,fields,draft_sha256,source_snapshots) VALUES($1,$2,$3,$4,$5)`, r.RunID, r.OrgID, raw, digest, bound); err != nil {
			return nil, nil, err
		}
		if err := readOATextDraft(ctx, tx, r); err != nil {
			return nil, nil, err
		}
	}
	src := oaRequestSnapshot(r)
	return map[string]any{"status": "succeeded", "committed": false, "operation_id": nil, "resource_refs": []ResultReference{}, "source_refs": []ResultReference{reference(src)}, "safe_summary": "业务正文建议已保存，等待本人逐字段核对采用", "retryable": false, "next_action": "查看原文与建议，核对后采用草稿", "content": r.OARequestView}, []SourceSnapshot{src}, nil
}

// Adoption runs normal human business commands in the same transaction as the
// immutable receipt. A model never gets a tool for this endpoint.
func (s *Service) AdoptOAText(ctx context.Context, command core.Command, runID string, in OATextAdopt) (core.Result, error) {
	if !oaDigestPattern.MatchString(in.DraftSHA256) || !in.Reviewed || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("draft_sha256", "请完整核对正文建议并确认采用依据")
	}
	var r *oaRequestRecord
	var c Conversation
	result, err := s.Commands.Do(ctx, command, func(tx pgx.Tx) error {
		a := command.Actor
		if err := identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
			return err
		}
		if _, err := runRecord(ctx, tx, a.OrgID, runID, true); err != nil {
			return err
		}
		run, conversation, err := s.readableRun(ctx, tx, a, runID)
		if err != nil {
			return err
		}
		c = conversation
		if a.RunID != "" || run.ActorID != a.UserID || c.Type != "private" || c.CreatorID != a.UserID || run.State != "succeeded" || run.Outcome == nil || *run.Outcome != "draft" || run.CancelRequestedAt != nil {
			return core.Missing()
		}
		var sameContext bool
		if err = tx.QueryRow(ctx, "SELECT conversation_revision_id=$3 AND context_security_version=$4 FROM agent_runs WHERE org_id=$1 AND id=$2", a.OrgID, runID, c.CurrentRevisionID, c.ContextSecurityVersion).Scan(&sameContext); err != nil {
			return err
		}
		if !sameContext {
			return core.Fail(409, "OA_TEXT_CONTEXT_CHANGED", "会话范围已变更，请重新发起正文起草")
		}
		if err = require(ctx, tx, a, c, "agent.run"); err != nil {
			return err
		}
		if _, err = NotificationRun(ctx, tx, a, runID); err != nil {
			return err
		}
		r, err = readOARequestRecord(ctx, tx, a.OrgID, runID)
		if err != nil {
			return err
		}
		if r == nil || r.Tool != "draft_business_text" || r.TextDraft == nil || r.TextDraft.DraftSHA256 != in.DraftSHA256 {
			return core.Fail(409, "OA_TEXT_DRAFT_CHANGED", "正文建议已变化，请重新核对")
		}
		ai := a
		ai.RunID = runID
		basis, err := oaBasis(ctx, tx, ai, c, r.Tool, r.Kind, r.BusinessID, nil, []string{a.UserID}, r.TextDraft.Result == nil, nil)
		if err != nil {
			return err
		}
		b := basis.Business
		if err = authz.Require(ctx, tx, a, b.Kind+".update", authz.Resource{Type: b.Kind, ID: b.ID, ProjectID: b.ProjectID, OwnerID: &b.CreatorID, Classification: b.Classification}); err != nil {
			return err
		}
		if r.TextDraft.Result == nil {
			raw, _ := json.Marshal(basis)
			if core.Digest(raw) != r.BaseSHA256 {
				return core.Fail(409, "OA_TEXT_SOURCE_CHANGED", "业务原文已变更，请重新发起正文起草")
			}
		}
		return nil
	}, func(tx pgx.Tx) (core.Result, error) {
		if r.TextDraft.Result != nil {
			return core.Result{}, core.Fail(409, "OA_TEXT_ALREADY_ADOPTED", "此正文建议已采用，请查看保存的业务修订")
		}
		// Decode only trusted original content, then replace the authorized prose.
		patched, err := patchOATextFields(r.Kind, r.Before.Business.Payload.Content, r.TextDraft.Fields)
		if err != nil {
			return core.Result{}, err
		}
		inner := command
		inner.Route, inner.Key = "/internal/agent-oa-text-adopt/"+r.Kind, "agent-oa-text:"+r.RunID
		var saved core.Result
		if r.Kind == "contract" {
			var content contracts.Content
			if err = json.Unmarshal(patched, &content); err != nil {
				return core.Result{}, err
			}
			patch := contracts.Patch{Content: content, ExpectedVersion: r.Before.Business.Version, BaseRevisionID: r.Before.Business.CurrentRevisionID, Reason: in.Reason}
			inner.Body, _ = json.Marshal(patch)
			service := contracts.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB, Tx: tx}}
			saved, err = service.Update(ctx, inner, r.BusinessID, patch)
		} else {
			var content spend.Content
			if err = json.Unmarshal(patched, &content); err != nil {
				return core.Result{}, err
			}
			patch := spend.Patch{Content: content, ExpectedVersion: r.Before.Business.Version, BaseRevisionID: r.Before.Business.CurrentRevisionID, Reason: in.Reason}
			inner.Body, _ = json.Marshal(patch)
			service := spend.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB, Tx: tx}}
			saved, err = service.Update(ctx, inner, r.Kind, r.BusinessID, patch)
		}
		if err != nil {
			return core.Result{}, err
		}
		ai := command.Actor
		ai.RunID = runID
		business, err := workflow.AgentBusinessRead(ctx, tx, ai, r.Kind, r.BusinessID, false)
		if err != nil {
			return core.Result{}, err
		}
		raw, _ := json.Marshal(business)
		if _, err = tx.Exec(ctx, `INSERT INTO agent_oa_text_adoptions(run_id,org_id,operation_id,result_data) VALUES($1,$2,$3,$4)`, runID, command.Actor.OrgID, saved.OperationID, raw); err != nil {
			return core.Result{}, err
		}
		if err = readOATextDraft(ctx, tx, r); err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 200, Data: r.OARequestView, Event: core.Event{Type: "agent.oa_text.adopted", ResourceType: "agent_run", ResourceID: runID, Version: 1, Payload: map[string]any{"reason": in.Reason, "business_id": r.BusinessID, "draft_sha256": in.DraftSHA256, "business_operation_id": saved.OperationID}}}, nil
	})
	if err == nil && result.Replayed {
		run, e := s.GetRun(ctx, command.Actor, runID)
		if e != nil {
			return core.Result{}, e
		}
		result.Data = run.OARequest
	}
	return result, err
}
