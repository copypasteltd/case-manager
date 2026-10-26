package workflow

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

const stepColumns = `s.id,s.version,s.request_id,r.version,r.title,s.revision_id,s.instance_id,s.step_no,s.title,s.assigned_user_id,
    s.assigned_user_name,u.status='active',s.requires_mfa,s.state,d.decision,d.reason,d.created_at,s.updated_at`
const stepJoin = ` FROM approval_steps s JOIN requests r ON r.org_id=s.org_id AND r.id=s.request_id
    JOIN users u ON u.org_id=s.org_id AND u.id=s.assigned_user_id
    LEFT JOIN approval_decisions d ON d.org_id=s.org_id AND d.step_id=s.id `
const businessHeads = ` JOIN (
    SELECT org_id,id,'contract'::text AS kind,project_id,creator_id,classification,NULL::uuid AS purchase_id FROM contracts
    UNION ALL SELECT org_id,id,kind,project_id,creator_id,classification,NULL::uuid FROM spend_documents
    UNION ALL SELECT org_id,id,kind,project_id,creator_id,classification,purchase_id FROM purchase_variances
 UNION ALL SELECT org_id,id,kind,project_id,creator_id,classification,NULL::uuid FROM change_requests WHERE protocol_version=1
) b ON b.org_id=r.org_id AND b.kind=r.kind AND b.id=r.business_id `

func steps(ctx context.Context, db platform.DBTX, a core.Actor, r Request, revision string) ([]ApprovalStep, error) {
	out := []ApprovalStep{}
	rows, err := db.Query(ctx, "SELECT "+stepColumns+stepJoin+"WHERE s.org_id=$1 AND s.request_id=$2 AND s.revision_id=$3 ORDER BY s.step_no", a.OrgID, r.ID, revision)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v ApprovalStep
		if err = rows.Scan(&v.ID, &v.Version, &v.RequestID, &v.RequestVersion, &v.RequestTitle, &v.RevisionID, &v.InstanceID, &v.Seq, &v.Title, &v.AssignedUserID, &v.AssignedUserName, &v.AssignedUserActive, &v.RequiresMFA, &v.State, &v.Decision, &v.DecisionReason, &v.DecidedAt, &v.UpdatedAt); err != nil {
			rows.Close()
			return out, err
		}
		out = append(out, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	allowed := authz.Require(ctx, db, a, "approval.decide", requestResource(r)) == nil
	for i := range out {
		v := &out[i]
		v.CanDecide = allowed && a.RunID == "" && slices.Contains([]string{"web", "android", "wechat", "douyin"}, a.Channel) && v.AssignedUserID == a.UserID && v.State == "active" && r.State == "in_review" && revision == r.CurrentRevisionID
	}
	return out, nil
}
func presentRequest(ctx context.Context, db platform.DBTX, a core.Actor, r Request) (Request, error) {
	if err := requireRequest(ctx, db, a, r, "request.read"); err != nil {
		return Request{}, err
	}
	var err error
	if r.Payload, err = redactSnapshot(ctx, db, a, r.Payload); err != nil {
		return Request{}, err
	}
	if r.Steps, err = steps(ctx, db, a, r, r.CurrentRevisionID); err != nil {
		return Request{}, err
	}
	if r.ApplicantID == a.UserID && draftState(r.State) {
		d, e := subject(ctx, db, a, r.Kind, r.BusinessID, false, false)
		if e == nil {
			r.CanRevise = authz.Require(ctx, db, a, "request.update", requestResource(r)) == nil
			r.CanSubmit = d.CurrentRevisionID == r.Payload.BusinessRevisionID && d.ContentHash == r.Payload.BusinessContentHash && authz.Require(ctx, db, a, "request.submit", requestResource(r)) == nil
		} else if !hiddenBusinessLink(e) {
			return Request{}, e
		}
	}
	r.CanWithdraw = r.ApplicantID == a.UserID && r.State == "in_review" && r.Workflow != nil && r.Workflow.WithdrawalAllowed && authz.Require(ctx, db, a, "request.withdraw", requestResource(r)) == nil
	return r, nil
}
func (s *Service) Request(ctx context.Context, a core.Actor, id string) (Request, error) {
	r, err := requestMetadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return r, err
	}
	return presentRequest(ctx, s.DB, a, r)
}
func (s *Service) Requests(ctx context.Context, a core.Actor, f RequestFilter) (core.Page[Request], error) {
	out := core.Page[Request]{Items: []Request{}}
	if len([]rune(f.Query)) > 200 || (f.Kind != "" && !validKind(f.Kind)) || !slices.Contains([]string{"", "draft", "in_review", "approved", "returned", "rejected", "withdrawn"}, f.State) {
		return out, core.Invalid("filter", "申请筛选无效")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	bound := requestBinding(a, fmt.Sprint(f.Kind, "/", f.State, "/", f.Query))
	var at *time.Time
	var id *string
	if f.Cursor != "" {
		c, e := s.Pager.Decode(f.Cursor, bound)
		if e != nil {
			return out, e
		}
		at = &c.UpdatedAt
		id = &c.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+requestColumns+requestJoin+businessHeads+`
        WHERE r.org_id=$1 AND ($2='' OR r.kind=$2) AND ($3='' OR r.state=$3) AND ($4='' OR position(lower($4) IN lower(r.title))>0)
        AND can_access(r.org_id,$5,'request.read','request',r.id,r.project_id,r.applicant_id,r.classification,$6,$7)
        AND can_access(b.org_id,$5,b.kind||'.read',b.kind,b.id,b.project_id,b.creator_id,b.classification,$6,$7)
        AND (b.kind<>'change' OR EXISTS(SELECT 1 FROM projects cp WHERE cp.org_id=b.org_id AND cp.id=b.project_id AND can_access(cp.org_id,$5,'budget.read','project',cp.id,cp.id,cp.creator_id,cp.classification,$6,$7)))
        AND (b.kind<>'purchase_variance' OR EXISTS(SELECT 1 FROM spend_documents p WHERE p.org_id=b.org_id AND p.id=b.purchase_id AND can_access(p.org_id,$5,'purchase.read','purchase',p.id,p.project_id,p.creator_id,p.classification,$6,$7)))
        AND ($8::timestamptz IS NULL OR (r.updated_at,r.id)<($8,$9::uuid)) ORDER BY r.updated_at DESC,r.id DESC LIMIT $10`,
		a.OrgID, f.Kind, f.State, f.Query, a.UserID, a.Channel, a.RunID != "", at, id, f.Limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, e := scanRequest(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		v := out.Items[f.Limit-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.UpdatedAt, Binding: bound})
		out.NextCursor = &cursor
	}
	for i, v := range out.Items {
		out.Items[i], err = presentRequest(ctx, s.DB, a, v)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

const revisionColumns = `v.id,v.request_id,v.seq,v.base_revision_id,v.payload_schema_version,v.payload,v.workflow_snapshot,v.content_hash,v.author_id,v.reason,v.created_at`

func scanRevision(row interface{ Scan(...any) error }) (Revision, error) {
	var v Revision
	var payload, flow []byte
	err := row.Scan(&v.ID, &v.RequestID, &v.Seq, &v.BaseRevisionID, &v.PayloadSchemaVersion, &payload, &flow, &v.ContentHash, &v.AuthorID, &v.Reason, &v.CreatedAt)
	if err != nil {
		return v, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(payload, &v.Payload); err != nil {
		return v, err
	}
	if len(flow) > 0 {
		err = json.Unmarshal(flow, &v.Workflow)
	}
	v.Steps = []ApprovalStep{}
	return v, err
}
func revisionMetadata(ctx context.Context, db platform.DBTX, org, request, id string) (Revision, error) {
	if !core.ValidID(id) {
		return Revision{}, core.Missing()
	}
	return scanRevision(db.QueryRow(ctx, "SELECT "+revisionColumns+" FROM request_revisions v WHERE v.org_id=$1 AND v.request_id=$2 AND v.id=$3", org, request, id))
}
func presentRevision(ctx context.Context, db platform.DBTX, a core.Actor, r Request, v Revision) (Revision, error) {
	var err error
	if v.Payload, err = redactSnapshot(ctx, db, a, v.Payload); err != nil {
		return v, err
	}
	v.Steps, err = steps(ctx, db, a, r, v.ID)
	return v, err
}
func (s *Service) RequestRevision(ctx context.Context, a core.Actor, id, revision string) (Revision, error) {
	r, err := requestMetadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return Revision{}, err
	}
	if err = requireRequest(ctx, s.DB, a, r, "request.read"); err != nil {
		return Revision{}, err
	}
	v, err := revisionMetadata(ctx, s.DB, a.OrgID, id, revision)
	if err != nil {
		return v, err
	}
	return presentRevision(ctx, s.DB, a, r, v)
}
func (s *Service) RequestHistory(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[Revision], error) {
	out := core.Page[Revision]{Items: []Revision{}}
	r, err := requestMetadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return out, err
	}
	if err = requireRequest(ctx, s.DB, a, r, "request.read"); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := requestBinding(a, id+"/history")
	var before *int64
	if cursor != "" {
		c, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		var seq int64
		if e = s.DB.QueryRow(ctx, "SELECT seq FROM request_revisions WHERE org_id=$1 AND request_id=$2 AND id=$3", a.OrgID, id, c.ID).Scan(&seq); e != nil {
			return out, e
		}
		before = &seq
	}
	rows, err := s.DB.Query(ctx, "SELECT "+revisionColumns+" FROM request_revisions v WHERE v.org_id=$1 AND v.request_id=$2 AND ($3::bigint IS NULL OR v.seq<$3) ORDER BY v.seq DESC LIMIT $4", a.OrgID, id, before, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, e := scanRevision(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		v := out.Items[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.CreatedAt, Binding: bound})
		out.NextCursor = &next
	}
	for i, v := range out.Items {
		out.Items[i], err = presentRevision(ctx, s.DB, a, r, v)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
func (s *Service) ApprovalTasks(ctx context.Context, a core.Actor, cursor string, limit int) (core.Page[ApprovalStep], error) {
	out := core.Page[ApprovalStep]{Items: []ApprovalStep{}}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := requestBinding(a, "my-active-approval-tasks")
	var at *time.Time
	var id *string
	if cursor != "" {
		c, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &c.UpdatedAt
		id = &c.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT s.id,s.request_id,s.updated_at FROM approval_steps s JOIN requests r ON r.org_id=s.org_id AND r.id=s.request_id
        `+businessHeads+`
        WHERE s.org_id=$1 AND s.assigned_user_id=$2 AND s.state='active' AND r.state='in_review' AND r.current_revision_id=s.revision_id AND r.active_instance_id=s.instance_id
        AND can_access(r.org_id,$2,'request.read','request',r.id,r.project_id,r.applicant_id,r.classification,$3,$4)
        AND can_access(r.org_id,$2,'approval.decide','request',r.id,r.project_id,r.applicant_id,r.classification,$3,$4)
        AND can_access(b.org_id,$2,b.kind||'.read',b.kind,b.id,b.project_id,b.creator_id,b.classification,$3,$4)
        AND (b.kind<>'change' OR EXISTS(SELECT 1 FROM projects cp WHERE cp.org_id=b.org_id AND cp.id=b.project_id AND can_access(cp.org_id,$2,'budget.read','project',cp.id,cp.id,cp.creator_id,cp.classification,$3,$4)))
        AND (b.kind<>'purchase_variance' OR EXISTS(SELECT 1 FROM spend_documents p WHERE p.org_id=b.org_id AND p.id=b.purchase_id AND can_access(p.org_id,$2,'purchase.read','purchase',p.id,p.project_id,p.creator_id,p.classification,$3,$4)))
        AND ($5::timestamptz IS NULL OR (s.updated_at,s.id)<($5,$6::uuid)) ORDER BY s.updated_at DESC,s.id DESC LIMIT $7`, a.OrgID, a.UserID, a.Channel, a.RunID != "", at, id, limit+1)
	if err != nil {
		return out, err
	}
	type entry struct {
		id, request string
		at          time.Time
	}
	entries := []entry{}
	for rows.Next() {
		var v entry
		if err = rows.Scan(&v.id, &v.request, &v.at); err != nil {
			rows.Close()
			return out, err
		}
		entries = append(entries, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(entries) > limit {
		entries = entries[:limit]
		v := entries[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: v.id, UpdatedAt: v.at, Binding: bound})
		out.NextCursor = &next
	}
	for _, v := range entries {
		r, e := requestMetadata(ctx, s.DB, a.OrgID, v.request)
		if e != nil {
			return out, e
		}
		if e = requireRequest(ctx, s.DB, a, r, "request.read"); e != nil {
			return out, e
		}
		list, e := steps(ctx, s.DB, a, r, r.CurrentRevisionID)
		if e != nil {
			return out, e
		}
		for _, task := range list {
			if task.ID == v.id {
				out.Items = append(out.Items, task)
			}
		}
	}
	return out, nil
}
