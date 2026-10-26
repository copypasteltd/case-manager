package project

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
)

// Archive is rare for the three-user initial deployment. Lock the organization's
// project heads in UUID order, before any cost/cash heads, to serialize shared
// allocations without taking a target-project lock and then an earlier sibling.
// Financial facts remain unchanged; the immutable source vectors are rechecked.
func lockCloseoutSources(ctx context.Context, tx pgx.Tx, org, id string) error {
	if _, e := tx.Exec(ctx, "SELECT id FROM projects WHERE org_id=$1 ORDER BY id FOR UPDATE", org); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, "SELECT id FROM actual_cost_entries WHERE org_id=$1 AND id IN(SELECT id FROM project_closeout_cost_ids($1,$2)) ORDER BY id FOR UPDATE", org, id); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, "SELECT id FROM payment_records WHERE org_id=$1 AND id IN(SELECT id FROM project_closeout_cash_ids($1,$2)) ORDER BY id FOR UPDATE", org, id)
	return e
}
func (s *Service) ArchiveProject(ctx context.Context, c core.Command, id string, in ArchiveInput) (core.Result, error) {
	if e := core.RequireVersion(in.ExpectedVersion); e != nil {
		return core.Result{}, e
	}
	if len(in.ExpectedFingerprint) != 32 || strings.Trim(in.ExpectedFingerprint, "0123456789abcdef") != "" {
		return core.Result{}, core.Invalid("expected_fingerprint", "请先读取完整、最新的结项核查")
	}
	in.Summary = strings.TrimSpace(in.Summary)
	in.LessonsLearned = strings.TrimSpace(in.LessonsLearned)
	in.FollowUp = strings.TrimSpace(in.FollowUp)
	in.Reason = strings.TrimSpace(in.Reason)
	for _, field := range []struct{ name, value string }{{"summary", in.Summary}, {"lessons_learned", in.LessonsLearned}, {"follow_up", in.FollowUp}} {
		if n := len([]rune(field.value)); n < 1 || n > 20000 {
			return core.Result{}, core.Invalid(field.name, "请填写1至20000字的复盘内容；无遗留跟进时也请明确说明")
		}
	}
	if core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请填写1至200字的归档原因")
	}
	if !in.BusinessVerified || !in.FinancialVerified {
		return core.Result{}, core.Invalid("attestations", "请逐项确认业务闭环与财务、合同及订阅处置")
	}
	if len(in.FileVersionIDs) > 20 {
		return core.Result{}, core.Invalid("file_version_ids", "最多附上20个固定文件版本")
	}
	seen := []string{}
	for _, vid := range in.FileVersionIDs {
		if !core.ValidID(vid) || slices.Contains(seen, vid) {
			return core.Result{}, core.Invalid("file_version_ids", "附件必须为不同的固定文件版本")
		}
		seen = append(seen, vid)
	}
	check := func(tx pgx.Tx) error {
		if e := authorizeProject(ctx, tx, c.Actor, id, "project.archive"); e != nil {
			return e
		}
		if e := identity.RequireInteractiveSession(ctx, tx, c.Actor, true); e != nil {
			return e
		}
		_, all, e := closeoutVisibility(ctx, tx, c.Actor, id)
		if e != nil {
			return e
		}
		if !all {
			return core.Fail(403, "PROJECT_CLOSEOUT_RESTRICTED", "归档需要完整查阅各类结项依据的当前权限")
		}
		return nil
	}
	out, e := s.Commands.Do(ctx, c, check, func(tx pgx.Tx) (core.Result, error) {
		if e := lockCloseoutSources(ctx, tx, c.Actor.OrgID, id); e != nil {
			return core.Result{}, e
		}
		p, e := getProject(ctx, tx, c.Actor.OrgID, id, false)
		if e != nil {
			return core.Result{}, e
		}
		if p.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if p.State != "closing" {
			return core.Result{}, core.UnexpectedState(p.State, "archive")
		}
		if e = check(tx); e != nil {
			return core.Result{}, e
		}
		b, raw, fp, e := closeoutSnapshot(ctx, tx, c.Actor.OrgID, id)
		if e != nil {
			return core.Result{}, e
		}
		if fp != in.ExpectedFingerprint {
			return core.Result{}, core.Fail(409, "PROJECT_CLOSEOUT_BASIS_CHANGED", "核查依据已变化，请保留复盘内容并重新核对；系统不会自动改用新依据")
		}
		if closeoutBlocked(b.Items) {
			v, _, e := closeoutVisibility(ctx, tx, c.Actor, id)
			if e != nil {
				return core.Result{}, e
			}
			return core.Result{}, closeoutBlockedError(b.Items, v)
		}
		if _, e = deliverableFiles(ctx, tx, c.Actor, p, in.FileVersionIDs, true); e != nil {
			return core.Result{}, e
		}
		archiveID := core.NewID()
		tag, e := tx.Exec(ctx, `INSERT INTO project_archives(id,org_id,project_id,project_version,basis,fingerprint,summary,lessons_learned,follow_up,business_verified,financial_verified,evidence_count,actor_id,session_id,mfa_verified_at,reason)
 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,s.id,s.mfa_verified_at,$14 FROM sessions s WHERE s.org_id=$2 AND s.id=$15 AND s.user_id=$13`, archiveID, c.Actor.OrgID, id, p.Version+1, raw, fp, in.Summary, in.LessonsLearned, in.FollowUp, in.BusinessVerified, in.FinancialVerified, len(in.FileVersionIDs), c.Actor.UserID, in.Reason, c.Actor.SessionID)
		if e != nil {
			return core.Result{}, e
		}
		if tag.RowsAffected() != 1 {
			return core.Result{}, core.Forbidden()
		}
		for _, vid := range in.FileVersionIDs {
			if _, e = tx.Exec(ctx, "INSERT INTO project_archive_files(org_id,archive_id,version_id) VALUES($1,$2,$3)", c.Actor.OrgID, archiveID, vid); e != nil {
				return core.Result{}, e
			}
		}
		if _, e = tx.Exec(ctx, "UPDATE projects SET state='archived',version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id); e != nil {
			return core.Result{}, e
		}
		p, e = getProject(ctx, tx, c.Actor.OrgID, id, false)
		if e != nil {
			return core.Result{}, e
		}
		a, e := readArchive(ctx, tx, c.Actor, id, archiveID)
		if e != nil {
			return core.Result{}, e
		}
		return core.Result{Status: 201, Data: ArchiveReceipt{Project: p, Archive: a}, Event: core.Event{Type: "project.archived", ResourceType: "project", ResourceID: id, Version: p.Version, Payload: map[string]any{"archive_id": archiveID, "fingerprint": fp, "reason": in.Reason}}}, nil
	})
	if e != nil || !out.Replayed {
		return out, e
	}
	encoded, e := json.Marshal(out.Data)
	if e != nil {
		return core.Result{}, e
	}
	var prior ArchiveReceipt
	if e = json.Unmarshal(encoded, &prior); e != nil {
		return core.Result{}, e
	}
	prior.Archive, e = readArchive(ctx, s.DB, c.Actor, id, prior.Archive.ID)
	if e != nil {
		return core.Result{}, e
	}
	out.Data = prior
	return out, nil
}
