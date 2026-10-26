package project

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

func deliverableRevision(ctx context.Context, db platform.DBTX, a core.Actor, d Deliverable, p Project, id string, approval bool) (DeliverableRevision, []string, error) {
	var r DeliverableRevision
	ids := []string{}
	if !core.ValidID(id) {
		return r, ids, core.Missing()
	}
	e := db.QueryRow(ctx, `SELECT id,deliverable_id,seq,head_version,project_version,name,acceptance_criteria,owner_id,milestone_id,classification,content_hash,description,reason,submitted_by,submitted_at,file_version_ids
      FROM deliverable_revisions WHERE org_id=$1 AND deliverable_id=$2 AND id=$3 AND protocol_version=1`, a.OrgID, d.ID, id).Scan(&r.ID, &r.DeliverableID, &r.Seq, &r.HeadVersion, &r.ProjectVersion, &r.Name, &r.AcceptanceCriteria, &r.OwnerID, &r.MilestoneID, &r.Classification, &r.ContentHash, &r.Description, &r.Reason, &r.SubmittedBy, &r.SubmittedAt, &ids)
	if e != nil {
		return r, nil, platform.DatabaseError(e)
	}
	// Both current business scope and the exact frozen file versions are checked.
	r.Files, e = deliverableFiles(ctx, db, a, p, ids, approval)
	if e != nil {
		return DeliverableRevision{}, nil, e
	}
	var decision DeliverableDecision
	e = db.QueryRow(ctx, `SELECT id,decision,reason,decided_by,decided_at,head_version,files_verified,criteria_verified FROM acceptance_decisions WHERE org_id=$1 AND revision_id=$2 AND protocol_version=1`, a.OrgID, id).Scan(&decision.ID, &decision.Decision, &decision.Reason, &decision.DecidedBy, &decision.DecidedAt, &decision.HeadVersion, &decision.FilesVerified, &decision.CriteriaVerified)
	if e == nil {
		if decision.Decision == "return" {
			decision.Decision = "request_changes"
		}
		r.Decision = &decision
	} else if e != pgx.ErrNoRows {
		return DeliverableRevision{}, nil, e
	}
	return r, ids, nil
}
func (s *Service) deliverableSnapshot(ctx context.Context, a core.Actor) (pgx.Tx, error) {
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		return nil, e
	}
	var ov, uv int64
	var state string
	e = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&ov)
	if e == nil {
		e = tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, a.UserID).Scan(&uv, &state)
	}
	if e != nil {
		tx.Rollback(ctx)
		return nil, platform.DatabaseError(e)
	}
	if ov != a.OrgAuthVersion || uv != a.AuthVersion || state != "active" {
		tx.Rollback(ctx)
		return nil, core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变化，请重新获取授权")
	}
	return tx, nil
}
func (s *Service) GetDeliverableRevision(ctx context.Context, a core.Actor, id, rid string) (DeliverableRevision, error) {
	tx, e := s.deliverableSnapshot(ctx, a)
	if e != nil {
		return DeliverableRevision{}, e
	}
	defer tx.Rollback(ctx)
	d, p, e := deliverableRead(ctx, tx, a, id, "deliverable.read")
	if e != nil {
		return DeliverableRevision{}, e
	}
	r, _, e := deliverableRevision(ctx, tx, a, d, p, rid, false)
	if e != nil {
		return DeliverableRevision{}, e
	}
	return r, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) ListDeliverableRevisions(ctx context.Context, a core.Actor, id string, f Filter) (core.Page[DeliverableRevision], error) {
	out := core.Page[DeliverableRevision]{Items: []DeliverableRevision{}}
	tx, e := s.deliverableSnapshot(ctx, a)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	d, p, e := deliverableRead(ctx, tx, a, id, "deliverable.read")
	if e != nil {
		return out, e
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.CursorBinding(a, "deliverable-revisions", id)
	var before *time.Time
	var beforeID *string
	if f.Cursor != "" {
		c, e := s.Pager.Decode(f.Cursor, binding)
		if e != nil {
			return out, e
		}
		before = &c.UpdatedAt
		beforeID = &c.ID
	}
	rows, e := tx.Query(ctx, `SELECT id::text FROM deliverable_revisions WHERE org_id=$1 AND deliverable_id=$2 AND protocol_version=1 AND ($3::timestamptz IS NULL OR (submitted_at,id)<($3,$4::uuid)) ORDER BY submitted_at DESC,id DESC LIMIT $5`, a.OrgID, id, before, beforeID, f.Limit+1)
	if e != nil {
		return out, e
	}
	ids := []string{}
	for rows.Next() {
		var rid string
		if e = rows.Scan(&rid); e != nil {
			rows.Close()
			return out, e
		}
		ids = append(ids, rid)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	more := len(ids) > f.Limit
	if more {
		ids = ids[:f.Limit]
	}
	for _, rid := range ids {
		r, _, e := deliverableRevision(ctx, tx, a, d, p, rid, false)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, r)
	}
	if more {
		r := out.Items[len(out.Items)-1]
		next := s.Pager.Encode(platform.Cursor{ID: r.ID, UpdatedAt: r.SubmittedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) DecideDeliverable(ctx context.Context, c core.Command, id string, in DeliverableDecisionInput) (core.Result, error) {
	if !core.ValidID(in.RevisionID) || len(in.ContentHash) != 64 || strings.Trim(in.ContentHash, "0123456789abcdef") != "" ||
		(in.Decision != "accept" && in.Decision != "request_changes") || len([]rune(strings.TrimSpace(in.Reason))) < 1 || len([]rune(in.Reason)) > 20000 ||
		(in.Decision == "accept" && (!in.FilesVerified || !in.CriteriaVerified)) {
		return core.Result{}, core.Invalid("decision", "请固定准确修订、填写决定说明并逐项核实文件及准则")
	}
	d, p, e := deliverableRead(ctx, s.DB, c.Actor, id, "deliverable.decide")
	if e != nil {
		return core.Result{}, e
	}
	_, refs, e := deliverableRevision(ctx, s.DB, c.Actor, d, p, in.RevisionID, false)
	if e != nil {
		return core.Result{}, e
	}
	return s.deliverableMutation(ctx, c, id, "deliverable.decide", in.ExpectedVersion, true, true, refs, func(tx pgx.Tx, p Project, d Deliverable) (core.Result, error) {
		if d.State != "submitted" || d.CurrentRevisionID == nil || *d.CurrentRevisionID != in.RevisionID {
			return core.Result{}, core.Conflict()
		}
		r, _, e := deliverableRevision(ctx, tx, c.Actor, d, p, in.RevisionID, true)
		if e != nil {
			return core.Result{}, e
		}
		if r.ContentHash != in.ContentHash {
			return core.Result{}, core.Conflict()
		}
		if in.Decision == "accept" && r.SubmittedBy == c.Actor.UserID {
			return core.Result{}, core.Fail(403, "DELIVERABLE_SELF_ACCEPTANCE_DENIED", "请由另一名获授权人员核实本次交付")
		}
		decision, state := "accept", "accepted"
		if in.Decision == "request_changes" {
			decision = "return"
			state = "changes_requested"
		}
		_, e = tx.Exec(ctx, `INSERT INTO acceptance_decisions(id,org_id,revision_id,decided_by,decision,reason,protocol_version,session_id,mfa_verified_at,revision_hash,head_version,files_verified,criteria_verified)
          SELECT $1,$2,$3,$4,$5,$6,1,s.id,s.mfa_verified_at,$7,$8,$9,$10 FROM sessions s WHERE s.org_id=$2 AND s.id=$11 AND s.user_id=$4`, core.NewID(), c.Actor.OrgID, in.RevisionID, c.Actor.UserID, decision, strings.TrimSpace(in.Reason), in.ContentHash, d.Version+1, in.FilesVerified, in.CriteriaVerified, c.Actor.SessionID)
		if e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "UPDATE deliverables SET state=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, state); e != nil {
			return core.Result{}, e
		}
		d, e = deliverableHead(ctx, tx, c.Actor.OrgID, id, false)
		result := deliverableResult(d, "deliverable.decided")
		result.Event.Payload["decision"] = in.Decision
		result.Event.Payload["reason"] = in.Reason
		return result, e
	})
}
