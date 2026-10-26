package project

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"time"
)

const deliverableColumns = `d.id,d.org_id,d.project_id,d.creator_id,d.name,d.acceptance_criteria,d.owner_id,d.milestone_id,d.current_revision_id,d.state,d.protocol_version,d.version,d.created_at,d.updated_at`

func scanDeliverable(row pgx.Row) (Deliverable, error) {
	var d Deliverable
	var org string
	err := row.Scan(&d.ID, &org, &d.ProjectID, &d.CreatorID, &d.Name, &d.AcceptanceCriteria, &d.OwnerID, &d.MilestoneID, &d.CurrentRevisionID, &d.State, &d.ProtocolVersion, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	return d, platform.DatabaseError(err)
}
func deliverableHead(ctx context.Context, db platform.DBTX, org, id string, lock bool) (Deliverable, error) {
	if !core.ValidID(id) {
		return Deliverable{}, core.Missing()
	}
	q := "SELECT " + deliverableColumns + " FROM deliverables d WHERE d.org_id=$1 AND d.id=$2"
	if lock {
		q += " FOR UPDATE"
	}
	return scanDeliverable(db.QueryRow(ctx, q, org, id))
}
func deliverableRead(ctx context.Context, db platform.DBTX, a core.Actor, id, action string) (Deliverable, Project, error) {
	d, e := deliverableHead(ctx, db, a.OrgID, id, false)
	if e != nil {
		return d, Project{}, e
	}
	p, e := getProject(ctx, db, a.OrgID, d.ProjectID, false)
	if e != nil {
		return d, p, e
	}
	e = authz.Require(ctx, db, a, action, authz.Resource{Type: "deliverable", ID: id, ProjectID: &p.ID, OwnerID: d.CreatorID, Classification: p.Classification})
	return d, p, e
}
func deliverableCapabilities(ctx context.Context, db platform.DBTX, a core.Actor, p Project, d *Deliverable) error {
	if d.ProtocolVersion != 1 || projectWritable(p) != nil {
		return nil
	}
	if d.MilestoneID != nil && validMilestone(ctx, db, a.OrgID, p.ID, d.MilestoneID) != nil {
		return nil
	}
	r := authz.Resource{Type: "deliverable", ID: d.ID, ProjectID: &p.ID, OwnerID: d.CreatorID, Classification: p.Classification}
	if slices.Contains([]string{"draft", "changes_requested"}, d.State) {
		d.CanEdit = authz.Require(ctx, db, a, "deliverable.create", r) == nil
	}
	if slices.Contains([]string{"draft", "changes_requested", "accepted"}, d.State) {
		d.CanSubmit = authz.Require(ctx, db, a, "deliverable.submit", r) == nil
	}
	if d.State == "submitted" {
		d.CanDecide = authz.Require(ctx, db, a, "deliverable.decide", r) == nil
	}
	if a.RunID != "" || !slices.Contains([]string{"web", "android", "wechat", "douyin"}, a.Channel) {
		d.CanSubmit = false
		d.CanDecide = false
	}
	return nil
}
func (s *Service) GetDeliverable(ctx context.Context, a core.Actor, id string) (Deliverable, error) {
	d, p, e := deliverableRead(ctx, s.DB, a, id, "deliverable.read")
	if e != nil {
		return Deliverable{}, e
	}
	e = deliverableCapabilities(ctx, s.DB, a, p, &d)
	return d, e
}
func (s *Service) ListDeliverables(ctx context.Context, a core.Actor, id string, f Filter) (core.Page[Deliverable], error) {
	out := core.Page[Deliverable]{Items: []Deliverable{}}
	if e := authorizeProject(ctx, s.DB, a, id, "project.read"); e != nil {
		return out, e
	}
	if f.State != "" && !slices.Contains([]string{"draft", "submitted", "accepted", "changes_requested"}, f.State) {
		return out, core.Invalid("state", "无效交付状态")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.CursorBinding(a, "deliverables", id, f.State, f.Query)
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
	rows, e := s.DB.Query(ctx, "SELECT "+deliverableColumns+` FROM deliverables d JOIN projects p ON p.org_id=d.org_id AND p.id=d.project_id
      WHERE d.org_id=$1 AND d.project_id=$2 AND can_access(d.org_id,$3,'deliverable.read','deliverable',d.id,p.id,d.creator_id,p.classification,$4,$10)
      AND ($5='' OR d.state=$5) AND ($6='' OR position(lower($6) IN lower(d.name))>0)
      AND ($7::timestamptz IS NULL OR (d.updated_at,d.id)<($7,$8::uuid)) ORDER BY d.updated_at DESC,d.id DESC LIMIT $9`, a.OrgID, id, a.UserID, a.Channel, f.State, f.Query, before, beforeID, f.Limit+1, a.RunID != "")
	if e != nil {
		return out, e
	}
	for rows.Next() {
		d, e := scanDeliverable(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, d)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[len(out.Items)-1]
		c := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: binding})
		out.NextCursor = &c
	}
	p, e := getProject(ctx, s.DB, a.OrgID, id, false)
	if e != nil {
		return out, e
	}
	for i := range out.Items {
		if e = deliverableCapabilities(ctx, s.DB, a, p, &out.Items[i]); e != nil {
			return out, e
		}
	}
	return out, nil
}
func deliverableInput(in DeliverableCreate) error {
	if core.Name(in.Name) != nil || len([]rune(strings.TrimSpace(in.AcceptanceCriteria))) < 1 || len([]rune(in.AcceptanceCriteria)) > 20000 || !core.ValidID(in.OwnerID) || core.Name(in.Reason) != nil {
		return core.Invalid("deliverable", "请填写完整交付名称、验收准则、负责人及操作原因")
	}
	return nil
}
func deliverableResult(d Deliverable, event string) core.Result {
	status := 200
	if event == "deliverable.created" {
		status = 201
	}
	return core.Result{Status: status, Data: DeliverableReceipt{ID: d.ID, Version: d.Version, RevisionID: d.CurrentRevisionID}, Event: core.Event{Type: event, ResourceType: "deliverable", ResourceID: d.ID, Version: d.Version, Payload: map[string]any{"project_id": d.ProjectID, "state": d.State, "revision_id": d.CurrentRevisionID}}}
}
func (s *Service) CreateDeliverable(ctx context.Context, c core.Command, id string, in DeliverableCreate) (core.Result, error) {
	if e := deliverableInput(in); e != nil {
		return core.Result{}, e
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return authorizeProject(ctx, tx, c.Actor, id, "deliverable.create") }, func(tx pgx.Tx) (core.Result, error) {
		p, e := getProject(ctx, tx, c.Actor.OrgID, id, true)
		if e != nil {
			return core.Result{}, e
		}
		if e = projectWritable(p); e != nil {
			return core.Result{}, e
		}
		if e = validAssignee(ctx, tx, c.Actor.OrgID, id, &in.OwnerID); e != nil {
			return core.Result{}, e
		}
		if e = validMilestone(ctx, tx, c.Actor.OrgID, id, in.MilestoneID); e != nil {
			return core.Result{}, e
		}
		did := core.NewID()
		_, e = tx.Exec(ctx, `INSERT INTO deliverables(id,org_id,project_id,name,state,acceptance_criteria,owner_id,creator_id,milestone_id,protocol_version) VALUES($1,$2,$3,$4,'draft',$5,$6,$7,$8,1)`, did, c.Actor.OrgID, id, strings.TrimSpace(in.Name), strings.TrimSpace(in.AcceptanceCriteria), in.OwnerID, c.Actor.UserID, in.MilestoneID)
		if e != nil {
			return core.Result{}, e
		}
		d, e := deliverableHead(ctx, tx, c.Actor.OrgID, did, false)
		return deliverableResult(d, "deliverable.created"), e
	})
}
func (s *Service) deliverableMutation(ctx context.Context, c core.Command, id, action string, version int64, human, mfa bool, references []string, apply func(pgx.Tx, Project, Deliverable) (core.Result, error)) (core.Result, error) {
	check := func(tx pgx.Tx) (Deliverable, Project, error) {
		d, p, e := deliverableRead(ctx, tx, c.Actor, id, action)
		if e == nil && human {
			e = identity.RequireInteractiveSession(ctx, tx, c.Actor, mfa)
		}
		if e == nil && len(references) > 0 {
			p, e = getProject(ctx, tx, c.Actor.OrgID, p.ID, true)
		}
		if e == nil {
			_, e = deliverableFiles(ctx, tx, c.Actor, p, references, false)
		}
		return d, p, e
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, _, e := check(tx); return e }, func(tx pgx.Tx) (core.Result, error) {
		d, p, e := check(tx)
		if e != nil {
			return core.Result{}, e
		}
		p, e = getProject(ctx, tx, c.Actor.OrgID, p.ID, true)
		if e != nil {
			return core.Result{}, e
		}
		d, e = deliverableHead(ctx, tx, c.Actor.OrgID, id, true)
		if e != nil {
			return core.Result{}, e
		}
		if _, _, e = check(tx); e != nil {
			return core.Result{}, e
		}
		if d.ProtocolVersion != 1 {
			return core.Result{}, core.Fail(409, "DELIVERABLE_LEGACY_EVIDENCE_REQUIRED", "旧交付缺少完整来源，须单独核实迁移")
		}
		if e = projectWritable(p); e != nil {
			return core.Result{}, e
		}
		if e = validMilestone(ctx, tx, c.Actor.OrgID, p.ID, d.MilestoneID); e != nil {
			return core.Result{}, e
		}
		if d.Version != version {
			return core.Result{}, core.Conflict()
		}
		return apply(tx, p, d)
	})
}
func (s *Service) UpdateDeliverable(ctx context.Context, c core.Command, id string, in DeliverablePatch) (core.Result, error) {
	if e := deliverableInput(in.DeliverableCreate); e != nil {
		return core.Result{}, e
	}
	return s.deliverableMutation(ctx, c, id, "deliverable.create", in.ExpectedVersion, false, false, nil, func(tx pgx.Tx, p Project, d Deliverable) (core.Result, error) {
		if !slices.Contains([]string{"draft", "changes_requested"}, d.State) {
			return core.Result{}, core.Fail(409, "DELIVERABLE_READ_ONLY", "仅草稿或退回交付可以修改资料")
		}
		if e := validAssignee(ctx, tx, c.Actor.OrgID, p.ID, &in.OwnerID); e != nil {
			return core.Result{}, e
		}
		if e := validMilestone(ctx, tx, c.Actor.OrgID, p.ID, in.MilestoneID); e != nil {
			return core.Result{}, e
		}
		_, e := tx.Exec(ctx, "UPDATE deliverables SET name=$3,acceptance_criteria=$4,owner_id=$5,milestone_id=$6,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, strings.TrimSpace(in.Name), strings.TrimSpace(in.AcceptanceCriteria), in.OwnerID, in.MilestoneID)
		if e != nil {
			return core.Result{}, e
		}
		d, e = deliverableHead(ctx, tx, c.Actor.OrgID, id, false)
		r := deliverableResult(d, "deliverable.updated")
		r.Event.Payload["reason"] = in.Reason
		return r, e
	})
}
func deliverableFiles(ctx context.Context, db platform.DBTX, a core.Actor, p Project, ids []string, approval bool) ([]fileref.Evidence, error) {
	out := []fileref.Evidence{}
	for _, id := range ids {
		var v fileref.Evidence
		var e error
		if approval {
			v, e = fileref.ForApproval(ctx, db, a, id, &p.ID, p.Classification)
		} else {
			v, e = fileref.Resolve(ctx, db, a, id, &p.ID, p.Classification)
		}
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) SubmitDeliverable(ctx context.Context, c core.Command, id string, in DeliverableSubmit) (core.Result, error) {
	if len(in.FileVersionIDs) < 1 || len(in.FileVersionIDs) > 20 || len([]rune(strings.TrimSpace(in.Description))) < 1 || len([]rune(in.Description)) > 20000 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("submission", "请固定1至20个正式文件版本，并填写交付说明和原因")
	}
	seen := map[string]bool{}
	for _, v := range in.FileVersionIDs {
		if !core.ValidID(v) || seen[v] {
			return core.Result{}, core.Invalid("file_version_ids", "文件版本必须准确且不能重复")
		}
		seen[v] = true
	}
	// Replays must still be authorized for every exact file in this request.
	if _, p, e := deliverableRead(ctx, s.DB, c.Actor, id, "deliverable.submit"); e != nil {
		return core.Result{}, e
	} else if _, e = deliverableFiles(ctx, s.DB, c.Actor, p, in.FileVersionIDs, false); e != nil {
		return core.Result{}, e
	}
	return s.deliverableMutation(ctx, c, id, "deliverable.submit", in.ExpectedVersion, true, false, in.FileVersionIDs, func(tx pgx.Tx, p Project, d Deliverable) (core.Result, error) {
		if !slices.Contains([]string{"draft", "changes_requested", "accepted"}, d.State) {
			return core.Result{}, core.Fail(409, "DELIVERABLE_NOT_SUBMITTABLE", "交付待验收，不能替换其固定依据")
		}
		if (in.ExpectedRevisionID == nil) != (d.CurrentRevisionID == nil) || (in.ExpectedRevisionID != nil && *in.ExpectedRevisionID != *d.CurrentRevisionID) {
			return core.Result{}, core.Conflict()
		}
		if _, e := deliverableFiles(ctx, tx, c.Actor, p, in.FileVersionIDs, true); e != nil {
			return core.Result{}, e
		}
		rid := core.NewID()
		_, e := tx.Exec(ctx, `INSERT INTO deliverable_revisions(id,org_id,deliverable_id,seq,file_version_ids,description,submitted_by,protocol_version,name,acceptance_criteria,owner_id,milestone_id,classification,project_version,head_version,session_id,reason)
            VALUES($1,$2,$3,(SELECT COALESCE(max(seq),0)+1 FROM deliverable_revisions WHERE org_id=$2 AND deliverable_id=$3),$4,$5,$6,1,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, rid, c.Actor.OrgID, id, in.FileVersionIDs, strings.TrimSpace(in.Description), c.Actor.UserID, d.Name, d.AcceptanceCriteria, d.OwnerID, d.MilestoneID, p.Classification, p.Version, d.Version+1, c.Actor.SessionID, in.Reason)
		if e != nil {
			return core.Result{}, e
		}
		for i, v := range in.FileVersionIDs {
			if _, e = tx.Exec(ctx, "INSERT INTO deliverable_revision_files(org_id,revision_id,version_id,ordinal) VALUES($1,$2,$3,$4)", c.Actor.OrgID, rid, v, i+1); e != nil {
				return core.Result{}, e
			}
		}
		if _, e = tx.Exec(ctx, "UPDATE deliverables SET current_revision_id=$3,state='submitted',version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, rid); e != nil {
			return core.Result{}, e
		}
		d, e = deliverableHead(ctx, tx, c.Actor.OrgID, id, false)
		r := deliverableResult(d, "deliverable.submitted")
		r.Event.Payload["reason"] = in.Reason
		return r, e
	})
}
