package agent

import (
	"context"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/identity"
	"github.com/jackc/pgx/v5"
)

type FileAbandonment struct {
	ID            string    `json:"id"`
	RunID         string    `json:"run_id"`
	DraftID       string    `json:"draft_id"`
	ContentSHA256 string    `json:"content_sha256"`
	CreatedAt     time.Time `json:"created_at"`
}
type FileAbandonmentInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	ContentSHA256   string `json:"content_sha256"`
	Reviewed        bool   `json:"reviewed"`
	Reason          string `json:"reason"`
}

func (s *Service) AbandonFileRecovery(ctx context.Context, c core.Command, runID, draftID string, in FileAbandonmentInput) (core.Result, error) {
	if !core.ValidID(runID) || !core.ValidID(draftID) {
		return core.Result{}, core.Missing()
	}
	if core.RequireVersion(in.ExpectedVersion) != nil || !oaDigestPattern.MatchString(in.ContentSHA256) || !in.Reviewed || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("abandonment", "请核对准确草稿、运行版本、内容摘要和放弃原因")
	}
	c.ExclusiveOrganizationLock = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return identity.RequireInteractiveSession(ctx, tx, c.Actor, true) }, func(tx pgx.Tx) (core.Result, error) {
		r, err := runRecord(ctx, tx, c.Actor.OrgID, runID, true)
		if err != nil {
			return core.Result{}, err
		}
		if r.ActorID != c.Actor.UserID {
			return core.Result{}, core.Missing()
		}
		conversation, err := metadata(ctx, tx, c.Actor.OrgID, r.ConversationID, false)
		if err != nil {
			return core.Result{}, err
		}
		if conversation.Type != "private" || conversation.CreatorID != c.Actor.UserID {
			return core.Result{}, core.Missing()
		}
		if err = require(ctx, tx, c.Actor, conversation, "conversation.read"); err != nil {
			return core.Result{}, err
		}
		if r.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if !slices.Contains([]string{"succeeded", "failed", "cancelled"}, r.State) {
			return core.Result{}, core.Fail(409, "FILE_RECOVERY_RUN_UNSETTLED", "请先停止并核对原运行实际结果")
		}
		u, err := files.AuthorizeAgentDraftAbandonment(ctx, tx, c.Actor, draftID)
		if err != nil {
			return core.Result{}, err
		}
		var sha string
		if err = tx.QueryRow(ctx, "SELECT content_sha256 FROM agent_file_drafts WHERE org_id=$1 AND run_id=$2 AND id=$3", c.Actor.OrgID, runID, draftID).Scan(&sha); err != nil {
			return core.Result{}, core.Missing()
		}
		if sha != in.ContentSHA256 {
			return core.Result{}, core.Conflict()
		}
		var active bool
		if err = tx.QueryRow(ctx, `WITH RECURSIVE descendants AS (SELECT run_id FROM agent_file_recoveries WHERE org_id=$1 AND source_draft_id=$2
 UNION SELECT c.run_id FROM agent_file_recoveries c JOIN descendants p ON p.run_id=c.source_run_id WHERE c.org_id=$1)
 SELECT EXISTS(SELECT 1 FROM descendants d JOIN agent_runs r ON r.org_id=$1 AND r.id=d.run_id WHERE r.state IN ('queued','running','awaiting_approval','committing'))`, c.Actor.OrgID, draftID).Scan(&active); err != nil {
			return core.Result{}, err
		}
		if active {
			return core.Result{}, core.Fail(409, "FILE_RECOVERY_SOURCE_IN_USE", "后续恢复运行仍在使用此来源，请先核对并停止相关运行")
		}
		out := FileAbandonment{ID: core.NewID(), RunID: runID, DraftID: draftID, ContentSHA256: sha}
		err = tx.QueryRow(ctx, `INSERT INTO agent_file_abandonments(id,org_id,run_id,draft_id,run_version,upload_version,content_sha256,actor_id,session_id,reviewed,reason)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,true,$10) RETURNING created_at`, out.ID, c.Actor.OrgID, runID, draftID, r.Version, u.Version, sha, c.Actor.UserID, c.Actor.SessionID, strings.TrimSpace(in.Reason)).Scan(&out.CreatedAt)
		return core.Result{Data: out, Event: core.Event{Type: "agent.file_recovery_abandoned", ResourceType: "agent_run", ResourceID: runID, Version: r.Version, Payload: map[string]any{"draft_id": draftID, "content_sha256": sha, "abandonment_id": out.ID, "deleted": false}}}, err
	})
}
