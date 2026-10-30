package agent

import (
	"context"
	"encoding/json"
	"strconv"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/workflow"
	"github.com/jackc/pgx/v5"
)

// ContractBodyInput binds one editable contract to a separately selected new
// text file. It is context for file-update, never authority to change a contract.
type ContractBodyInput struct {
	ContractID      string `json:"contract_id"`
	ExpectedVersion int64  `json:"expected_version"`
	BaseRevisionID  string `json:"base_revision_id"`
	ContentHash     string `json:"content_hash"`
	Reason          string `json:"reason"`
}

type ContractBodyFile struct {
	DraftID  string           `json:"draft_id"`
	Evidence fileref.Evidence `json:"evidence"`
	Content  string           `json:"content"`
}

type ContractBodyView struct {
	RunID            string                      `json:"run_id"`
	Input            ContractBodyInput           `json:"input"`
	Before           workflow.AgentBusinessView  `json:"before"`
	IntentSHA256     string                      `json:"intent_sha256"`
	FileIntentSHA256 string                      `json:"file_intent_sha256"`
	Status           string                      `json:"status"`
	File             *ContractBodyFile           `json:"file"`
	OperationID      *string                     `json:"operation_id"`
	Result           *workflow.AgentBusinessView `json:"result"`
}

type ContractBodyAdopt struct {
	IntentSHA256  string `json:"intent_sha256"`
	VersionID     string `json:"version_id"`
	ContentSHA256 string `json:"content_sha256"`
	Reviewed      bool   `json:"reviewed"`
	Reason        string `json:"reason"`
}

type contractBodyRecord struct {
	ContractBodyView
	OrgID, ActorID, ConversationID, SessionID string
	BaseSHA256, Intent                        string
}

func contractBodySnapshot(r *contractBodyRecord) SourceSnapshot {
	return SourceSnapshot{SourceReference: SourceReference{Type: "agent_contract_body", ID: r.RunID}, ResourceVersion: 1, SHA256: r.IntentSHA256}
}

func contractBodyBasis(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, id string, writable bool) (workflow.AgentBusinessView, error) {
	b, err := oaBasis(ctx, tx, a, c, "draft_business_text", "contract", id, nil, []string{a.UserID}, writable, nil)
	return b.Business, err
}

func prepareContractBody(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, runID string, in ContractBodyInput, file *fileIntentRecord, message string) (*contractBodyRecord, error) {
	if !exactOAID(in.ContractID) || !exactOAID(in.BaseRevisionID) || !oaDigestPattern.MatchString(in.ContentHash) || core.RequireVersion(in.ExpectedVersion) != nil || core.Name(in.Reason) != nil || file == nil || file.Mode != "create" || file.Recovery != nil {
		return nil, core.Invalid("contract_body", "请明确选择可编辑合同、准确修订及新建正文文件位置")
	}
	if err := identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return nil, err
	}
	ai := a
	ai.RunID = runID
	b, err := contractBodyBasis(ctx, tx, ai, c, in.ContractID, true)
	if err != nil {
		return nil, err
	}
	if b.Version != in.ExpectedVersion || b.CurrentRevisionID != in.BaseRevisionID || b.ContentHash != in.ContentHash {
		return nil, core.Fail(409, "CONTRACT_BODY_SOURCE_CHANGED", "合同原始资料已变化，请重新核对")
	}
	// A contract attachment cannot be more classified than its contract, and
	// copying commercial facts into a less classified file would disclose them.
	if file.Target.Classification != b.Classification || c.Classification != b.Classification {
		return nil, core.Invalid("contract_body", "会话、正文文件目录必须与合同分类完全一致")
	}
	base, _ := json.Marshal(b)
	intent, _ := json.Marshal(map[string]any{"run_id": runID, "input": in, "before": b, "base_sha256": core.Digest(base), "file_intent_sha256": file.IntentSHA256, "actor_id": a.UserID, "actor_auth_version": a.AuthVersion, "organization_auth_version": a.OrgAuthVersion, "session_id": a.SessionID, "conversation_id": c.ID, "conversation_revision_id": c.CurrentRevisionID, "context_security_version": c.ContextSecurityVersion, "request": message})
	if len(intent) > 524288 {
		return nil, core.Invalid("contract_body", "合同依据超过受控起草范围，请先整理资料")
	}
	return &contractBodyRecord{ContractBodyView: ContractBodyView{RunID: runID, Input: in, Before: b, IntentSHA256: core.Digest(intent), FileIntentSHA256: file.IntentSHA256, Status: "authorized"}, OrgID: a.OrgID, ActorID: a.UserID, ConversationID: c.ID, SessionID: a.SessionID, BaseSHA256: core.Digest(base), Intent: string(intent)}, nil
}

func insertContractBody(ctx context.Context, tx pgx.Tx, r *contractBodyRecord) error {
	before, _ := json.Marshal(r.Before)
	input, _ := json.Marshal(r.Input)
	_, err := tx.Exec(ctx, `INSERT INTO agent_contract_body_intents(run_id,org_id,contract_id,actor_id,conversation_id,session_id,input_data,before_data,base_sha256,file_intent_sha256,intent,intent_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, r.RunID, r.OrgID, r.Input.ContractID, r.ActorID, r.ConversationID, r.SessionID, input, before, r.BaseSHA256, r.FileIntentSHA256, r.Intent, r.IntentSHA256)
	return err
}

func readContractBody(ctx context.Context, tx pgx.Tx, org, id string) (*contractBodyRecord, error) {
	r := &contractBodyRecord{}
	var input, before, result []byte
	err := tx.QueryRow(ctx, `SELECT i.run_id,i.org_id,i.actor_id,i.conversation_id,i.session_id,i.input_data,i.before_data,i.base_sha256,i.file_intent_sha256,i.intent,i.intent_sha256,a.operation_id,a.result_data FROM agent_contract_body_intents i LEFT JOIN agent_contract_body_adoptions a ON a.org_id=i.org_id AND a.run_id=i.run_id WHERE i.org_id=$1 AND i.run_id=$2`, org, id).Scan(&r.RunID, &r.OrgID, &r.ActorID, &r.ConversationID, &r.SessionID, &input, &before, &r.BaseSHA256, &r.FileIntentSHA256, &r.Intent, &r.IntentSHA256, &r.OperationID, &result)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(input, &r.Input); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(before, &r.Before); err != nil {
		return nil, err
	}
	r.Status = "authorized"
	if r.OperationID != nil {
		if err = json.Unmarshal(result, &r.Result); err != nil {
			return nil, err
		}
		r.Status = "adopted"
	}
	return r, nil
}

// The complete bytes come from the immutable single-edit create draft. Both
// the ordinary committed version and its SHA/size must match before disclosure.
func contractBodyFile(ctx context.Context, tx pgx.Tx, a core.Actor, r *contractBodyRecord) (*ContractBodyFile, error) {
	i, err := fileIntentRecordRead(ctx, tx, a.OrgID, r.RunID)
	if err != nil {
		return nil, err
	}
	if i == nil || i.Mode != "create" || i.IntentSHA256 != r.FileIntentSHA256 {
		return nil, core.Missing()
	}
	if i.OperationID == nil {
		return nil, nil
	}
	var saved files.CommitResult
	if json.Unmarshal(i.Result, &saved) != nil || !saved.Committed || saved.Version.BaseVersionID != nil {
		return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	e, err := fileref.Resolve(ctx, tx, a, saved.Version.ID, r.Before.ProjectID, r.Before.Classification)
	if err != nil {
		return nil, err
	}
	var sameScope bool
	if err = tx.QueryRow(ctx, `SELECT f.classification=$3 AND d.project_id IS NOT DISTINCT FROM $4::uuid FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id WHERE f.org_id=$1 AND f.id=$2`, a.OrgID, e.FileID, r.Before.Classification, r.Before.ProjectID).Scan(&sameScope); err != nil {
		return nil, err
	}
	if !sameScope {
		return nil, core.Missing()
	}
	var draft, sha, size string
	var edits []byte
	err = tx.QueryRow(ctx, `SELECT d.id,d.edits,d.content_sha256,d.size_bytes::text FROM agent_file_commits c JOIN agent_file_drafts d ON d.org_id=c.org_id AND d.run_id=c.run_id AND d.id=c.draft_id WHERE c.org_id=$1 AND c.run_id=$2`, a.OrgID, r.RunID).Scan(&draft, &edits, &sha, &size)
	if err != nil {
		return nil, err
	}
	var full []files.AgentTextEdit
	if json.Unmarshal(edits, &full) != nil || len(full) != 1 || full[0].Start != 0 || full[0].End != 0 || len(full[0].Text) > 49152 || e.ScanState != "clean" || e.FileID != saved.File.ID || e.SHA256 != sha || e.SizeBytes != size || e.Name != i.Target.Name || e.MediaType != i.Target.MediaType || strconv.Itoa(len([]byte(full[0].Text))) != size || core.Digest([]byte(full[0].Text)) != sha {
		return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	return &ContractBodyFile{DraftID: draft, Evidence: e, Content: full[0].Text}, nil
}

func contractBodySource(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, id string, members []string) (SourceSnapshot, error) {
	r, err := readContractBody(ctx, tx, a.OrgID, id)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if r == nil || r.ActorID != a.UserID || r.ConversationID != c.ID || len(members) != 1 || members[0] != a.UserID || core.Digest([]byte(r.Intent)) != r.IntentSHA256 {
		return SourceSnapshot{}, core.Missing()
	}
	ai := a
	ai.RunID = id
	if _, err = contractBodyBasis(ctx, tx, ai, c, r.Input.ContractID, false); err != nil {
		return SourceSnapshot{}, err
	}
	if err = workflow.AgentSnapshotReferences(ctx, tx, ai, r.Before.Payload); err != nil {
		return SourceSnapshot{}, err
	}
	if r.Result != nil {
		if err = workflow.AgentSnapshotReferences(ctx, tx, ai, r.Result.Payload); err != nil {
			return SourceSnapshot{}, err
		}
	}
	if _, err = contractBodyFile(ctx, tx, ai, r); err != nil {
		return SourceSnapshot{}, err
	}
	return contractBodySnapshot(r), nil
}

func contractBodyViewRead(ctx context.Context, tx pgx.Tx, a core.Actor, r *contractBodyRecord) (ContractBodyView, error) {
	view := r.ContractBodyView
	file, err := contractBodyFile(ctx, tx, a, r)
	if err != nil {
		return view, err
	}
	view.File = file
	if file != nil && view.OperationID == nil {
		view.Status = "file_committed"
	}
	return view, nil
}

func executionContractBodyState(ctx context.Context, tx pgx.Tx, st executionState) (*contractBodyRecord, error) {
	r, err := readContractBody(ctx, tx, st.Actor.OrgID, st.Run.ID)
	if err != nil || r == nil {
		return r, err
	}
	if st.Profile.SkillID != "file-update" || r.ActorID != st.Actor.UserID || r.ConversationID != st.Conversation.ID {
		return nil, core.Missing()
	}
	human := st.Actor
	human.SessionID = r.SessionID
	if err = identity.RequireInteractiveSession(ctx, tx, human, false); err != nil {
		return nil, err
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	b, err := contractBodyBasis(ctx, tx, ai, st.Conversation, r.Input.ContractID, true)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(b)
	if core.Digest(raw) != r.BaseSHA256 {
		return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	return r, nil
}

func executionContractBodyText(ctx context.Context, tx pgx.Tx, st executionState, id string, version *string, offset, maxChars int) (any, []SourceSnapshot, error) {
	if st.Profile.SkillID != "file-update" || id != st.Run.ID || version != nil || offset < 0 || offset > 524288 || maxChars < 1 || maxChars > 10000 {
		return nil, nil, core.Invalid("arguments", "请读取本次合同正文依据及有限字符范围")
	}
	r, err := executionContractBodyState(ctx, tx, st)
	if err != nil {
		return nil, nil, err
	}
	if r == nil {
		return nil, nil, core.Missing()
	}
	raw, _ := json.Marshal(r.ContractBodyView)
	body := []rune(string(raw))
	if offset > len(body) {
		return nil, nil, core.Invalid("offset", "字符范围无效")
	}
	end := min(offset+maxChars, len(body))
	var next *int
	if end < len(body) {
		next = &end
	}
	src := contractBodySnapshot(r)
	return map[string]any{"type": src.Type, "id": id, "resource_version": 1, "source_sha256": src.SHA256, "text": string(body[offset:end]), "offset": offset, "total_chars": len(body), "next_offset": next, "untrusted_data": true}, []SourceSnapshot{src}, nil
}

// Human adoption changes exactly draft_file_version_id through the normal
// contract revision command. No runtime tool can call this endpoint.
func (s *Service) AdoptContractBody(ctx context.Context, command core.Command, runID string, in ContractBodyAdopt) (core.Result, error) {
	if !oaDigestPattern.MatchString(in.IntentSHA256) || !oaDigestPattern.MatchString(in.ContentSHA256) || !exactOAID(in.VersionID) || !in.Reviewed || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("contract_body", "请核对完整正文、准确文件版本并填写采用依据")
	}
	var r *contractBodyRecord
	var file *ContractBodyFile
	result, err := s.Commands.Do(ctx, command, func(tx pgx.Tx) error {
		a := command.Actor
		if e := identity.RequireInteractiveSession(ctx, tx, a, true); e != nil {
			return e
		}
		if _, e := runRecord(ctx, tx, a.OrgID, runID, true); e != nil {
			return e
		}
		run, c, e := s.readableRun(ctx, tx, a, runID)
		if e != nil {
			return e
		}
		if a.RunID != "" || run.ActorID != a.UserID || c.Type != "private" || c.CreatorID != a.UserID || run.State != "succeeded" || run.Outcome == nil || *run.Outcome != "completed" || run.CancelRequestedAt != nil {
			return core.Missing()
		}
		var same bool
		if e = tx.QueryRow(ctx, "SELECT conversation_revision_id=$3 AND context_security_version=$4 FROM agent_runs WHERE org_id=$1 AND id=$2", a.OrgID, runID, c.CurrentRevisionID, c.ContextSecurityVersion).Scan(&same); e != nil {
			return e
		}
		if !same {
			return core.Fail(409, "CONTRACT_BODY_CONTEXT_CHANGED", "会话范围已变更，请重新发起正文起草")
		}
		if e = require(ctx, tx, a, c, "agent.run"); e != nil {
			return e
		}
		if _, e = NotificationRun(ctx, tx, a, runID); e != nil {
			return e
		}
		r, e = readContractBody(ctx, tx, a.OrgID, runID)
		if e != nil {
			return e
		}
		if r == nil || r.IntentSHA256 != in.IntentSHA256 {
			return core.Fail(409, "CONTRACT_BODY_SOURCE_CHANGED", "正文起草依据已变化，请重新核对")
		}
		file, e = contractBodyFile(ctx, tx, a, r)
		if e != nil {
			return e
		}
		if file == nil || file.Evidence.VersionID != in.VersionID || file.Evidence.SHA256 != in.ContentSHA256 {
			return core.Fail(409, "CONTRACT_BODY_FILE_CHANGED", "完整正文文件尚未提交或版本不一致")
		}
		ai := a
		ai.RunID = runID
		b, e := contractBodyBasis(ctx, tx, ai, c, r.Input.ContractID, r.Result == nil)
		if e != nil {
			return e
		}
		if e = authz.Require(ctx, tx, a, "contract.update", authz.Resource{Type: "contract", ID: b.ID, ProjectID: b.ProjectID, OwnerID: &b.CreatorID, Classification: b.Classification}); e != nil {
			return e
		}
		if r.Result == nil {
			raw, _ := json.Marshal(b)
			if core.Digest(raw) != r.BaseSHA256 {
				return core.Fail(409, "CONTRACT_BODY_SOURCE_CHANGED", "合同资料已变更，请重新起草并核对")
			}
		}
		return nil
	}, func(tx pgx.Tx) (core.Result, error) {
		if r.Result != nil {
			return core.Result{}, core.Fail(409, "CONTRACT_BODY_ALREADY_ADOPTED", "此完整正文已经采用，请查看保存的合同修订")
		}
		var content contracts.Content
		if e := json.Unmarshal(r.Before.Payload.Content, &content); e != nil {
			return core.Result{}, e
		}
		content.DraftFileVersionID = &file.Evidence.VersionID
		patch := contracts.Patch{Content: content, ExpectedVersion: r.Before.Version, BaseRevisionID: r.Before.CurrentRevisionID, Reason: in.Reason}
		inner := command
		inner.Route = "/internal/agent-contract-body-adopt"
		inner.Key = "agent-contract-body:" + runID
		inner.Body, _ = json.Marshal(patch)
		service := contracts.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB, Tx: tx}}
		saved, e := service.Update(ctx, inner, r.Input.ContractID, patch)
		if e != nil {
			return core.Result{}, e
		}
		ai := command.Actor
		ai.RunID = runID
		b, e := workflow.AgentBusinessRead(ctx, tx, ai, "contract", r.Input.ContractID, false)
		if e != nil {
			return core.Result{}, e
		}
		raw, _ := json.Marshal(b)
		if _, e = tx.Exec(ctx, `INSERT INTO agent_contract_body_adoptions(run_id,org_id,operation_id,version_id,content_sha256,reviewed,reason,result_data) VALUES($1,$2,$3,$4,$5,true,$6,$7)`, runID, command.Actor.OrgID, saved.OperationID, file.Evidence.VersionID, file.Evidence.SHA256, in.Reason, raw); e != nil {
			return core.Result{}, e
		}
		r, e = readContractBody(ctx, tx, command.Actor.OrgID, runID)
		if e != nil {
			return core.Result{}, e
		}
		view, e := contractBodyViewRead(ctx, tx, command.Actor, r)
		if e != nil {
			return core.Result{}, e
		}
		return core.Result{Status: 200, Data: view, Event: core.Event{Type: "agent.contract_body.adopted", ResourceType: "agent_run", ResourceID: runID, Version: 1, Payload: map[string]any{"contract_id": r.Input.ContractID, "file_version_id": file.Evidence.VersionID, "content_sha256": file.Evidence.SHA256, "business_operation_id": saved.OperationID, "reason": in.Reason}}}, nil
	})
	if err == nil && result.Replayed {
		run, e := s.GetRun(ctx, command.Actor, runID)
		if e != nil {
			return core.Result{}, e
		}
		result.Data = run.ContractBody
	}
	return result, err
}
