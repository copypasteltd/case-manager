package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/identity"
	"github.com/jackc/pgx/v5"
)

// A new interactive request authorizes exact saved bytes, never continuation of
// the old lease, a new target, an automatic rebase or a reused storage receipt.
type FileRecoveryInput struct {
	SourceRunID   string `json:"source_run_id"`
	DraftID       string `json:"draft_id"`
	ContentSHA256 string `json:"content_sha256"`
	Reviewed      bool   `json:"reviewed"`
}

type FileRecoveryView struct {
	SourceRunID    string `json:"source_run_id"`
	DraftID        string `json:"draft_id"`
	ContentSHA256  string `json:"content_sha256"`
	RecoverySHA256 string `json:"recovery_sha256"`
	Depth          int    `json:"depth"`
}

type fileRecoveryBody struct {
	SourceRunID        string           `json:"source_run_id"`
	DraftID            string           `json:"draft_id"`
	ContentSHA256      string           `json:"content_sha256"`
	SourceIntentSHA256 string           `json:"source_intent_sha256"`
	SessionID          string           `json:"session_id"`
	Depth              int              `json:"depth"`
	Sources            []SourceSnapshot `json:"sources"`
}

type fileRecoveryRecord struct {
	Body fileRecoveryBody
	Raw  string
}

func (r *fileRecoveryRecord) view() *FileRecoveryView {
	return &FileRecoveryView{SourceRunID: r.Body.SourceRunID, DraftID: r.Body.DraftID, ContentSHA256: r.Body.ContentSHA256, RecoverySHA256: core.Digest([]byte(r.Raw)), Depth: r.Body.Depth}
}

func readFileRecovery(ctx context.Context, tx pgx.Tx, i *fileIntentRecord) error {
	var raw, sha string
	err := tx.QueryRow(ctx, "SELECT recovery_body,recovery_sha256 FROM agent_file_recoveries WHERE org_id=$1 AND run_id=$2", i.OrgID, i.RunID).Scan(&raw, &sha)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	r := &fileRecoveryRecord{Raw: raw}
	if json.Unmarshal([]byte(raw), &r.Body) != nil || core.Digest([]byte(raw)) != sha {
		return runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	i.RecoveryRecord, i.Recovery = r, r.view()
	return nil
}

func insertFileRecovery(ctx context.Context, tx pgx.Tx, i *fileIntentRecord) error {
	if i.RecoveryRecord == nil {
		return nil
	}
	r := i.RecoveryRecord
	_, err := tx.Exec(ctx, `INSERT INTO agent_file_recoveries(run_id,org_id,source_run_id,source_draft_id,content_sha256,recovery_body,recovery_sha256,depth,reviewed)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,true)`, i.RunID, i.OrgID, r.Body.SourceRunID, r.Body.DraftID, r.Body.ContentSHA256, r.Raw, i.Recovery.RecoverySHA256, r.Body.Depth)
	return err
}

func fileRecoverySources(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, r *fileRecoveryRecord, ai bool) error {
	if r == nil {
		return nil
	}
	if ai {
		human := a
		human.RunID, human.SessionID = "", r.Body.SessionID
		if err := identity.RequireInteractiveSession(ctx, tx, human, false); err != nil {
			return err
		}
		var available bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_file_drafts d
 JOIN agent_file_intents i ON i.org_id=d.org_id AND i.run_id=d.run_id JOIN agent_runs old ON old.org_id=d.org_id AND old.id=d.run_id
 JOIN upload_sessions u ON u.org_id=d.org_id AND u.id=d.id
 WHERE d.org_id=$1 AND d.run_id=$2 AND d.id=$3 AND d.content_sha256=$4 AND i.intent_sha256=$5
 AND old.actor_id=$6 AND old.conversation_id=$7 AND old.state IN ('succeeded','failed','cancelled')
 AND u.state NOT IN ('committed','blocked') AND u.scan_state NOT IN ('quarantined','blocked','infected','unscannable','waived')
 AND NOT EXISTS(SELECT 1 FROM agent_file_commits fc WHERE fc.org_id=old.org_id AND fc.run_id=old.id))`, a.OrgID, r.Body.SourceRunID, r.Body.DraftID, r.Body.ContentSHA256, r.Body.SourceIntentSHA256, a.UserID, c.ID).Scan(&available); err != nil {
			return err
		}
		if !available {
			return core.Fail(409, "FILE_RECOVERY_UNAVAILABLE", "原草稿状态已变化，恢复已停止，请核对实际结果")
		}
	}
	return fileSourcesCurrent(ctx, tx, a, c, r.Body.Sources, []string{a.UserID}, ai)
}

func fileSourcesCurrent(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, sources []SourceSnapshot, members []string, ai bool) error {
	refs := make([]SourceReference, 0, len(sources))
	for _, snapshot := range sources {
		// The original exact file intent is independently fixed by the recovery
		// row. External dependencies are flattened, so no recursive trust chain
		// or historical lease is needed to validate their current authority.
		if snapshot.Type == "agent_file_update" || snapshot.Type == "agent_file_create" {
			return runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
		refs = append(refs, snapshot.SourceReference)
	}
	current, err := sourceSnapshots(ctx, tx, a, c, refs, members, ai)
	if err != nil {
		return runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	before, _ := json.Marshal(sources)
	after, _ := json.Marshal(current)
	if string(before) != string(after) {
		return runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	return nil
}

// Every selected and tool-read dependency stays attached to visible saved
// drafts. Sources are bounded and flattened; the self intent is not recursive.
func fileDraftSources(ctx context.Context, tx pgx.Tx, original *fileIntentRecord) ([]SourceSnapshot, error) {
	groups := [][]SourceSnapshot{}
	if original.RecoveryRecord != nil {
		groups = append(groups, original.RecoveryRecord.Body.Sources)
	}
	rows, err := tx.Query(ctx, `SELECT source_snapshots FROM conversation_messages WHERE org_id=$1 AND run_id=$2 AND role='user'
 UNION ALL SELECT source_snapshots FROM agent_tool_receipts WHERE org_id=$1 AND run_id=$2`, original.OrgID, original.RunID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var raw []byte
		var group []SourceSnapshot
		if err = rows.Scan(&raw); err == nil {
			err = json.Unmarshal(raw, &group)
		}
		if err != nil {
			rows.Close()
			return nil, err
		}
		filtered := []SourceSnapshot{}
		for _, src := range group {
			if src.Type == fileIntentSourceType(original) && src.ID == original.RunID {
				if src.SHA256 != original.IntentSHA256 || src.ResourceVersion != 1 {
					rows.Close()
					return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
				}
				continue
			}
			filtered = append(filtered, src)
		}
		groups = append(groups, filtered)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return mergeExecutionSources(groups...)
}

func (s *Service) prepareFileRecoveryIntent(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, runID string, in FileRecoveryInput, members []string, request string) (*fileIntentRecord, error) {
	if !core.ValidID(in.SourceRunID) || !core.ValidID(in.DraftID) || in.SourceRunID != strings.ToLower(in.SourceRunID) || in.DraftID != strings.ToLower(in.DraftID) || !oaDigestPattern.MatchString(in.ContentSHA256) || !in.Reviewed {
		return nil, core.Invalid("file_recovery", "请选择准确的已保存草稿，核对内容摘要并明确确认恢复")
	}
	if err := identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return nil, err
	}
	if c.Type != "private" || c.CreatorID != a.UserID || len(members) != 1 || members[0] != a.UserID {
		return nil, core.Missing()
	}
	old, err := runRecord(ctx, tx, a.OrgID, in.SourceRunID, true)
	if err != nil {
		return nil, err
	}
	if old.ActorID != a.UserID || old.ConversationID != c.ID || !slices.Equal(old.audience, members) {
		return nil, core.Missing()
	}
	if !slices.Contains([]string{"succeeded", "failed", "cancelled"}, old.State) {
		return nil, core.Fail(409, "FILE_RECOVERY_RUN_UNSETTLED", "原运行尚未明确结束，请先核对实际提交结果")
	}
	var sameContext bool
	if err = tx.QueryRow(ctx, "SELECT conversation_revision_id=$3 AND context_security_version=$4 FROM agent_runs WHERE org_id=$1 AND id=$2", a.OrgID, old.ID, c.CurrentRevisionID, c.ContextSecurityVersion).Scan(&sameContext); err != nil {
		return nil, err
	}
	if !sameContext {
		return nil, core.Fail(409, "FILE_RECOVERY_CONTEXT_CHANGED", "原会话范围已变化，请核对资料后发起新的文件请求")
	}
	original, err := fileIntentRecordRead(ctx, tx, a.OrgID, old.ID)
	if err != nil {
		return nil, err
	}
	if original == nil || original.OperationID != nil {
		return nil, core.Fail(409, "FILE_RECOVERY_UNAVAILABLE", "此运行没有可恢复的未提交文件草稿")
	}
	var savedSHA, state, scan string
	err = tx.QueryRow(ctx, `SELECT d.content_sha256,u.state,u.scan_state FROM agent_file_drafts d
 JOIN upload_sessions u ON u.org_id=d.org_id AND u.id=d.id WHERE d.org_id=$1 AND d.run_id=$2 AND d.id=$3`, a.OrgID, old.ID, in.DraftID).Scan(&savedSHA, &state, &scan)
	if err == pgx.ErrNoRows {
		return nil, core.Missing()
	}
	if err != nil {
		return nil, err
	}
	if savedSHA != in.ContentSHA256 || state == "committed" || state == "blocked" || slices.Contains([]string{"quarantined", "blocked", "infected", "unscannable", "waived"}, scan) {
		return nil, core.Fail(409, "FILE_RECOVERY_UNAVAILABLE", "草稿摘要不符、已经提交或有扫描阻断，不能恢复")
	}
	depth := 1
	if original.RecoveryRecord != nil {
		depth = original.RecoveryRecord.Body.Depth + 1
	}
	if depth > 8 {
		return nil, core.Fail(409, "FILE_RECOVERY_DEPTH_LIMIT", "恢复来源已达8层，请核对现行文件后发起新的编辑请求")
	}
	sources, err := fileDraftSources(ctx, tx, original)
	if err != nil {
		return nil, err
	}
	r := &fileRecoveryRecord{Body: fileRecoveryBody{SourceRunID: old.ID, DraftID: in.DraftID, ContentSHA256: savedSHA, SourceIntentSHA256: original.IntentSHA256, SessionID: a.SessionID, Depth: depth, Sources: sources}}
	if err = fileRecoverySources(ctx, tx, a, c, r, true); err != nil {
		return nil, err
	}
	var next *fileIntentRecord
	if original.Mode == "create" {
		next, err = prepareFileCreateIntent(ctx, tx, a, c, runID, FileCreateInput{FolderID: original.Target.FolderID, ExpectedFolderVersion: original.Target.FolderVersion, Name: original.Target.Name, MediaType: original.Target.MediaType}, members, request)
	} else {
		next, err = prepareFileIntent(ctx, tx, a, c, runID, FileUpdateInput{FileID: original.FileID, BaseVersionID: original.BaseVersionID, ExpectedFileVersion: original.FileVersion}, members, request)
	}
	if err != nil {
		return nil, err
	}
	if next.BaseSHA256 != original.BaseSHA256 || next.Target != original.Target {
		return nil, core.Fail(409, "FILE_RECOVERY_SOURCE_CHANGED", "原文件、目录或来源已变更，不能自动套用旧草稿")
	}
	raw, _ := json.Marshal(r.Body)
	r.Raw = string(raw)
	next.RecoveryRecord, next.Recovery = r, r.view()
	var intent map[string]json.RawMessage
	if err = json.Unmarshal([]byte(next.Intent), &intent); err != nil {
		return nil, err
	}
	intent["recovery"], _ = json.Marshal(next.Recovery)
	raw, _ = json.Marshal(intent)
	next.Intent, next.IntentSHA256 = string(raw), core.Digest(raw)
	return next, nil
}

func recoveredFileArguments(ctx context.Context, tx pgx.Tx, i *fileIntentRecord, args json.RawMessage) (fileDraftArguments, error) {
	var in struct {
		RunID          string `json:"run_id"`
		RecoverySHA256 string `json:"recovery_sha256"`
	}
	if err := strictArguments(args, &in); err != nil {
		return fileDraftArguments{}, err
	}
	if i.Recovery == nil || in.RunID != i.RunID || in.RecoverySHA256 != i.Recovery.RecoverySHA256 {
		return fileDraftArguments{}, core.Fail(409, "FILE_RECOVERY_SCOPE_REQUIRED", "恢复工具必须匹配本次明确选定的准确草稿与恢复摘要")
	}
	var edits []byte
	err := tx.QueryRow(ctx, "SELECT edits FROM agent_file_drafts WHERE org_id=$1 AND run_id=$2 AND id=$3 AND content_sha256=$4", i.OrgID, i.Recovery.SourceRunID, i.Recovery.DraftID, i.Recovery.ContentSHA256).Scan(&edits)
	if err != nil {
		return fileDraftArguments{}, err
	}
	var saved []files.AgentTextEdit
	if json.Unmarshal(edits, &saved) != nil || len(saved) == 0 {
		return fileDraftArguments{}, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	if i.Mode == "create" {
		if len(saved) != 1 || saved[0].Start != 0 || saved[0].End != 0 {
			return fileDraftArguments{}, runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
		return fileDraftArguments{FolderID: i.Target.FolderID, Name: i.Target.Name, Content: &saved[0].Text}, nil
	}
	return fileDraftArguments{FileID: i.FileID, BaseVersionID: i.BaseVersionID, Edits: saved}, nil
}
