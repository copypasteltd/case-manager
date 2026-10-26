package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"github.com/jackc/pgx/v5"
)

type FileUpdateInput struct {
	FileID              string `json:"file_id"`
	BaseVersionID       string `json:"base_version_id"`
	ExpectedFileVersion int64  `json:"expected_file_version"`
}
type FileUpdateView struct {
	RunID         string           `json:"run_id"`
	FileID        string           `json:"file_id"`
	BaseVersionID string           `json:"base_version_id"`
	FileVersion   int64            `json:"file_version"`
	IntentSHA256  string           `json:"intent_sha256"`
	Target        fileUpdateTarget `json:"target"`
	Status        string           `json:"status"`
	Drafts        []FileDraftView  `json:"drafts"`
	OperationID   *string          `json:"operation_id"`
	Result        json.RawMessage  `json:"result"`
}
type fileUpdateTarget struct {
	Name           string `json:"name"`
	FolderID       string `json:"folder_id"`
	Classification string `json:"classification"`
	MediaType      string `json:"media_type"`
	FolderVersion  int64  `json:"folder_version,omitempty"`
}
type fileIntentRecord struct {
	FileUpdateView
	Mode                                               string
	OrgID, ActorID, ConversationID, BaseSHA256, Intent string
}
type FileDraftView struct {
	ID              string              `json:"id"`
	ContentSHA256   string              `json:"content_sha256"`
	SizeBytes       string              `json:"size_bytes"`
	Diff            files.AgentTextDiff `json:"diff"`
	State           string              `json:"state"`
	ScanState       string              `json:"scan_state"`
	PlanHash        *string             `json:"plan_hash"`
	ResultVersionID *string             `json:"result_version_id"`
}

func prepareFileIntent(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, runID string, in FileUpdateInput, members []string, request string) (*fileIntentRecord, error) {
	if !core.ValidID(in.FileID) || !core.ValidID(in.BaseVersionID) || in.FileID != strings.ToLower(in.FileID) || in.BaseVersionID != strings.ToLower(in.BaseVersionID) || core.RequireVersion(in.ExpectedFileVersion) != nil {
		return nil, core.Invalid("file_update", "请明确文件、准确基础版本及文件元数据版本")
	}
	ai := a
	ai.RunID = runID
	f, err := files.AgentEditableFile(ctx, tx, ai, in.FileID, in.BaseVersionID)
	if err != nil {
		return nil, err
	}
	if f.Version != in.ExpectedFileVersion {
		return nil, core.Conflict()
	}
	doc, err := contextFile(ctx, tx, ai, c, ContextSource{Type: "file", ID: in.FileID, VersionID: &in.BaseVersionID}, members)
	if err != nil {
		return nil, err
	}
	media := doc.Evidence[0].MediaType
	if media != "text/plain" && media != "text/markdown" {
		return nil, core.Invalid("file_update", "首期秘书编辑支持UTF-8文本与Markdown；其他格式需独立适配")
	}
	sha, err := documentDigest(doc)
	if err != nil {
		return nil, err
	}
	target := fileUpdateTarget{Name: f.Name, FolderID: f.FolderID, Classification: f.Classification, MediaType: media}
	intent, _ := json.Marshal(map[string]any{"run_id": runID, "file_id": in.FileID, "base_version_id": in.BaseVersionID, "file_version": f.Version,
		"actor_id": a.UserID, "actor_auth_version": a.AuthVersion, "organization_auth_version": a.OrgAuthVersion, "conversation_id": c.ID,
		"conversation_revision_id": c.CurrentRevisionID, "context_security_version": c.ContextSecurityVersion, "target": target, "base_sha256": sha, "request": request})
	return &fileIntentRecord{FileUpdateView: FileUpdateView{RunID: runID, FileID: in.FileID, BaseVersionID: in.BaseVersionID, FileVersion: f.Version, IntentSHA256: core.Digest(intent), Target: target, Status: "authorized", Drafts: []FileDraftView{}}, OrgID: a.OrgID, ActorID: a.UserID, ConversationID: c.ID, BaseSHA256: sha, Intent: string(intent)}, nil
}
func insertFileIntent(ctx context.Context, tx pgx.Tx, i *fileIntentRecord) error {
	target, _ := json.Marshal(i.Target)
	mode := i.Mode
	if mode == "" {
		mode = "update"
	}
	_, err := tx.Exec(ctx, `INSERT INTO agent_file_intents(run_id,org_id,actor_id,conversation_id,file_id,base_version_id,file_version,target_data,base_sha256,intent,intent_sha256,mode)
 VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid,$7,$8,$9,$10,$11,$12)`, i.RunID, i.OrgID, i.ActorID, i.ConversationID, i.FileID, i.BaseVersionID, i.FileVersion, target, i.BaseSHA256, i.Intent, i.IntentSHA256, mode)
	return err
}
func fileIntentRecordRead(ctx context.Context, tx pgx.Tx, org, id string) (*fileIntentRecord, error) {
	var out fileIntentRecord
	var target []byte
	err := tx.QueryRow(ctx, `SELECT i.run_id,i.org_id,i.actor_id,i.conversation_id,coalesce(i.file_id::text,''),coalesce(i.base_version_id::text,''),i.file_version,i.target_data,i.base_sha256,i.intent,i.intent_sha256,c.operation_id,c.result_data,i.mode
 FROM agent_file_intents i LEFT JOIN agent_file_commits c ON c.org_id=i.org_id AND c.run_id=i.run_id WHERE i.org_id=$1 AND i.run_id=$2`, org, id).
		Scan(&out.RunID, &out.OrgID, &out.ActorID, &out.ConversationID, &out.FileID, &out.BaseVersionID, &out.FileVersion, &target, &out.BaseSHA256, &out.Intent, &out.IntentSHA256, &out.OperationID, &out.Result, &out.Mode)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(target, &out.Target); err != nil {
		return nil, err
	}
	out.Status = "authorized"
	if out.OperationID != nil {
		out.Status = "committed"
	}
	out.Drafts = []FileDraftView{}
	return &out, nil
}
func fileIntentSource(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, id string, members []string) (SourceSnapshot, error) {
	var out SourceSnapshot
	i, err := fileIntentRecordRead(ctx, tx, a.OrgID, id)
	if err != nil {
		return out, err
	}
	if i == nil || i.ConversationID != c.ID {
		return out, core.Missing()
	}
	var audience []string
	if err = tx.QueryRow(ctx, "SELECT audience_ids FROM agent_runs WHERE org_id=$1 AND id=$2", a.OrgID, id).Scan(&audience); err != nil {
		return out, err
	}
	if i.Mode == "create" {
		return fileCreateSource(ctx, tx, a, c, i, members, audience)
	}
	if !slices.Contains(audience, a.UserID) {
		return out, core.Missing()
	}
	for _, user := range members {
		if !slices.Contains(audience, user) {
			return out, core.Missing()
		}
	}
	if _, err = contextFile(ctx, tx, a, c, ContextSource{Type: "file", ID: i.FileID, VersionID: &i.BaseVersionID}, members); err != nil {
		return out, err
	}
	if core.Digest([]byte(i.Intent)) != i.IntentSHA256 {
		return out, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	return SourceSnapshot{SourceReference: SourceReference{Type: "agent_file_update", ID: id}, ResourceVersion: 1, SHA256: i.IntentSHA256}, nil
}
func (s *Service) executionFileIntent(ctx context.Context, tx pgx.Tx, st executionState) (*fileIntentRecord, error) {
	i, err := fileIntentRecordRead(ctx, tx, st.Actor.OrgID, st.Run.ID)
	if err != nil || i == nil {
		return i, err
	}
	if st.Profile.SkillID != "file-update" || i.ActorID != st.Actor.UserID {
		return nil, core.Missing()
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	if _, err = fileIntentSource(ctx, tx, ai, st.Conversation, i.RunID, st.Run.audience); err != nil {
		return nil, err
	}
	if i.Mode == "create" {
		digest, e := createTargetDigest(ctx, tx, ai, st.Conversation, i.Target, st.Run.audience, i.OperationID == nil)
		if e != nil {
			return nil, e
		}
		if digest != i.BaseSHA256 {
			return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
		return i, nil
	}
	base := i.BaseVersionID
	expected := i.FileVersion
	if i.OperationID != nil {
		var result files.CommitResult
		if json.Unmarshal(i.Result, &result) != nil || !result.Committed || result.File.ID != i.FileID {
			return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
		base = result.Version.ID
		expected = result.File.Version
	}
	f, err := files.AgentEditableFile(ctx, tx, ai, i.FileID, base)
	if err != nil {
		return nil, err
	}
	if f.Version != expected || f.Name != i.Target.Name || f.FolderID != i.Target.FolderID || f.Classification != i.Target.Classification {
		return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	return i, nil
}
func fileUpdateViewRead(ctx context.Context, tx pgx.Tx, i *fileIntentRecord) (FileUpdateView, error) {
	rows, err := tx.Query(ctx, `SELECT d.id,d.content_sha256,d.size_bytes::text,d.diff,u.state,u.scan_state,p.plan_hash,u.committed_version_id
 FROM agent_file_drafts d JOIN upload_sessions u ON u.org_id=d.org_id AND u.id=d.id
 LEFT JOIN agent_file_proposals p ON p.org_id=d.org_id AND p.draft_id=d.id WHERE d.org_id=$1 AND d.run_id=$2 ORDER BY d.created_at,d.id`, i.OrgID, i.RunID)
	if err != nil {
		return FileUpdateView{}, err
	}
	defer rows.Close()
	view := i.FileUpdateView
	view.Drafts = []FileDraftView{}
	for rows.Next() {
		var d FileDraftView
		var diff []byte
		if err = rows.Scan(&d.ID, &d.ContentSHA256, &d.SizeBytes, &diff, &d.State, &d.ScanState, &d.PlanHash, &d.ResultVersionID); err != nil {
			return view, err
		}
		if err = json.Unmarshal(diff, &d.Diff); err != nil {
			return view, err
		}
		view.Drafts = append(view.Drafts, d)
	}
	return view, rows.Err()
}
