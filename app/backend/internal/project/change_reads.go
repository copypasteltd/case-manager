package project

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
)

func changeHead(ctx context.Context, db platform.DBTX, org, id string, lock bool) (Change, error) {
	var d Change
	if !core.ValidID(id) {
		return d, core.Missing()
	}
	q := `SELECT id,project_id,creator_id,classification,state,lifecycle,approval_state,current_revision_id,approved_revision_id,request_id,version,created_at,updated_at FROM change_requests WHERE org_id=$1 AND id=$2 AND protocol_version=1`
	if lock {
		q += " FOR UPDATE"
	}
	e := db.QueryRow(ctx, q, org, id).Scan(&d.ID, &d.ProjectID, &d.CreatorID, &d.Classification, &d.State, &d.Lifecycle, &d.ApprovalState, &d.CurrentRevisionID, &d.ApprovedRevisionID, &d.RequestID, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	return d, platform.DatabaseError(e)
}
func changeResource(d Change) authz.Resource {
	return authz.Resource{Type: "change", ID: d.ID, ProjectID: &d.ProjectID, OwnerID: &d.CreatorID, Classification: d.Classification}
}
func changeRead(ctx context.Context, db platform.DBTX, a core.Actor, id, action string) (Change, Project, error) {
	d, e := changeHead(ctx, db, a.OrgID, id, false)
	if e != nil {
		return d, Project{}, e
	}
	p, e := getProject(ctx, db, a.OrgID, d.ProjectID, false)
	if e != nil {
		return d, p, e
	}
	if e = authz.Require(ctx, db, a, action, changeResource(d)); e != nil {
		return Change{}, p, e
	}
	if e = authz.Require(ctx, db, a, "budget.read", resource(p)); e != nil {
		return Change{}, p, e
	}
	return d, p, nil
}
func readChangeRevision(ctx context.Context, db platform.DBTX, a core.Actor, d Change, p Project, id string, complete bool) (ChangeRevision, error) {
	var r ChangeRevision
	if !core.ValidID(id) {
		return r, core.Missing()
	}
	var content, basis []byte
	e := db.QueryRow(ctx, `SELECT id,change_id,seq,base_revision_id,payload_schema_version,payload,basis,content_hash,approval_amount::text,budget_delta::text,target_budget_id,author_id,reason,created_at FROM project_change_revisions WHERE org_id=$1 AND change_id=$2 AND id=$3`, a.OrgID, d.ID, id).Scan(&r.ID, &r.ChangeID, &r.Seq, &r.BaseRevisionID, &r.PayloadSchemaVersion, &content, &basis, &r.ContentHash, &r.ApprovalAmount, &r.BudgetDelta, &r.TargetBudgetID, &r.AuthorID, &r.Reason, &r.CreatedAt)
	if e != nil {
		return r, platform.DatabaseError(e)
	}
	if e = json.Unmarshal(content, &r.Content); e != nil {
		return r, e
	}
	if e = json.Unmarshal(basis, &r.Basis); e != nil {
		return r, e
	}
	r.Attachments, e = deliverableFiles(ctx, db, a, p, r.Content.FileVersionIDs, complete)
	if e != nil {
		return ChangeRevision{}, e
	}
	return r, nil
}
func readChangeImplementation(ctx context.Context, db platform.DBTX, a core.Actor, id string) (*ChangeImplementation, error) {
	var r ChangeImplementation
	e := db.QueryRow(ctx, `SELECT id,change_id,revision_id,request_revision_id,project_id,before_project_version,after_project_version,change_version,actor_id,mfa_verified_at,scope_verified,budget_verified,reason,created_at FROM project_change_implementations WHERE org_id=$1 AND change_id=$2`, a.OrgID, id).Scan(&r.ID, &r.ChangeID, &r.RevisionID, &r.RequestRevisionID, &r.ProjectID, &r.BeforeProjectVersion, &r.AfterProjectVersion, &r.ChangeVersion, &r.ActorID, &r.MFAVerifiedAt, &r.ScopeVerified, &r.BudgetVerified, &r.Reason, &r.CreatedAt)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	return &r, platform.DatabaseError(e)
}
func currentChangeBasis(ctx context.Context, db platform.DBTX, org, pid, currency string) (ChangeBasis, error) {
	var b ChangeBasis
	var raw []byte
	e := db.QueryRow(ctx, "SELECT project_change_basis($1,$2,$3)", org, pid, currency).Scan(&raw)
	if e != nil {
		return b, platform.DatabaseError(e)
	}
	e = json.Unmarshal(raw, &b)
	return b, e
}
func changeBasisCurrent(ctx context.Context, db platform.DBTX, org string, r ChangeRevision) (bool, error) {
	var same bool
	e := db.QueryRow(ctx, `SELECT basis=project_change_basis(org_id,$3,payload->>'currency') FROM project_change_revisions WHERE org_id=$1 AND id=$2`, org, r.ID, r.Basis.ProjectID).Scan(&same)
	return same, platform.DatabaseError(e)
}
func presentChange(ctx context.Context, db platform.DBTX, a core.Actor, d Change, p Project) (Change, error) {
	var e error
	d.Revision, e = readChangeRevision(ctx, db, a, d, p, d.CurrentRevisionID, false)
	if e != nil {
		return Change{}, e
	}
	d.Implementation, e = readChangeImplementation(ctx, db, a, d.ID)
	if e != nil {
		return Change{}, e
	}
	writable := projectWritable(p) == nil && d.Classification == p.Classification && d.Lifecycle == "draft" && !slices.Contains([]string{"in_review", "approved"}, d.ApprovalState)
	d.CanEdit = writable && authz.Require(ctx, db, a, "change.update", changeResource(d)) == nil
	d.CanPrepareRequest = writable && authz.Require(ctx, db, a, "change.submit", changeResource(d)) == nil && authz.Require(ctx, db, a, "change.update", changeResource(d)) == nil
	if d.CanPrepareRequest && d.RequestID != nil {
		var state string
		if e = db.QueryRow(ctx, "SELECT state FROM requests WHERE org_id=$1 AND id=$2", a.OrgID, *d.RequestID).Scan(&state); e != nil {
			return Change{}, platform.DatabaseError(e)
		}
		d.CanPrepareRequest = state == "rejected"
	}
	same, e := changeBasisCurrent(ctx, db, a.OrgID, d.Revision)
	if e != nil {
		return Change{}, e
	}
	d.CanImplement = d.ApprovalState == "approved" && d.Lifecycle == "requested" && same && projectWritable(p) == nil && a.RunID == "" && slices.Contains([]string{"web", "android", "wechat", "douyin"}, a.Channel) && authz.Require(ctx, db, a, "change.implement", changeResource(d)) == nil && authz.Require(ctx, db, a, "project.update", resource(p)) == nil
	return d, nil
}
func (s *Service) GetChange(ctx context.Context, a core.Actor, id string) (Change, error) {
	tx, e := s.deliverableSnapshot(ctx, a)
	if e != nil {
		return Change{}, e
	}
	defer tx.Rollback(ctx)
	d, p, e := changeRead(ctx, tx, a, id, "change.read")
	if e != nil {
		return Change{}, e
	}
	d, e = presentChange(ctx, tx, a, d, p)
	if e != nil {
		return Change{}, e
	}
	return d, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) GetChangeRevision(ctx context.Context, a core.Actor, id, rid string) (ChangeRevision, error) {
	tx, e := s.deliverableSnapshot(ctx, a)
	if e != nil {
		return ChangeRevision{}, e
	}
	defer tx.Rollback(ctx)
	d, p, e := changeRead(ctx, tx, a, id, "change.read")
	if e != nil {
		return ChangeRevision{}, e
	}
	r, e := readChangeRevision(ctx, tx, a, d, p, rid, false)
	if e != nil {
		return r, e
	}
	return r, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) ListChanges(ctx context.Context, a core.Actor, pid string, f Filter) (core.Page[Change], error) {
	out := core.Page[Change]{Items: []Change{}}
	tx, e := s.deliverableSnapshot(ctx, a)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	p, e := getProject(ctx, tx, a.OrgID, pid, false)
	if e != nil {
		return out, e
	}
	if e = authz.Require(ctx, tx, a, "project.read", resource(p)); e != nil {
		return out, e
	}
	if e = authz.Require(ctx, tx, a, "budget.read", resource(p)); e != nil {
		return out, e
	}
	if f.State != "" && !slices.Contains([]string{"draft", "in_review", "approved", "rejected", "withdrawn", "implemented"}, f.State) {
		return out, core.Invalid("state", "变更状态无效")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.CursorBinding(a, "project-changes", pid, f.State, f.Query)
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
	rows, e := tx.Query(ctx, `SELECT d.id::text FROM change_requests d WHERE d.org_id=$1 AND d.project_id=$2 AND d.protocol_version=1 AND can_access(d.org_id,$3,'change.read','change',d.id,d.project_id,d.creator_id,d.classification,$4,$10) AND ($5='' OR d.state=$5) AND ($6='' OR position(lower($6) IN lower(d.title))>0) AND ($7::timestamptz IS NULL OR (d.updated_at,d.id)<($7,$8::uuid)) ORDER BY d.updated_at DESC,d.id DESC LIMIT $9`, a.OrgID, pid, a.UserID, a.Channel, f.State, f.Query, before, beforeID, f.Limit+1, a.RunID != "")
	if e != nil {
		return out, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return out, e
		}
		ids = append(ids, id)
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
	for _, id := range ids {
		d, e := changeHead(ctx, tx, a.OrgID, id, false)
		if e != nil {
			return out, e
		}
		d, e = presentChange(ctx, tx, a, d, p)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, d)
	}
	if more {
		last := out.Items[len(out.Items)-1]
		c := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: binding})
		out.NextCursor = &c
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) ListChangeRevisions(ctx context.Context, a core.Actor, id string, f Filter) (core.Page[ChangeRevision], error) {
	out := core.Page[ChangeRevision]{Items: []ChangeRevision{}}
	tx, e := s.deliverableSnapshot(ctx, a)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	d, p, e := changeRead(ctx, tx, a, id, "change.read")
	if e != nil {
		return out, e
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.CursorBinding(a, "project-change-revisions", id)
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
	rows, e := tx.Query(ctx, `SELECT id::text FROM project_change_revisions WHERE org_id=$1 AND change_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, before, beforeID, f.Limit+1)
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
		r, e := readChangeRevision(ctx, tx, a, d, p, rid, false)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, r)
	}
	if more {
		last := out.Items[len(out.Items)-1]
		c := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &c
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
