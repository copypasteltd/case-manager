package project

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

var closeoutKinds = []string{"tasks", "milestones", "deliverables", "risks", "issues", "changes", "budgets", "contracts", "purchases", "claims", "costs", "payments", "subscriptions"}

type closeoutBasis struct {
	Project Project        `json:"project"`
	Items   []CloseoutItem `json:"items"`
}

func closeoutVisibility(ctx context.Context, db platform.DBTX, a core.Actor, id string) (map[string]bool, bool, error) {
	rows, e := db.Query(ctx, "SELECT kind,readable FROM project_closeout_readable_groups($1,$2,$3,$4,$5)", a.OrgID, a.UserID, id, a.Channel, a.RunID != "")
	if e != nil {
		return nil, false, e
	}
	defer rows.Close()
	v := map[string]bool{}
	all := true
	for rows.Next() {
		var k string
		var ok bool
		if e = rows.Scan(&k, &ok); e != nil {
			return nil, false, e
		}
		v[k] = ok
		all = all && ok
	}
	return v, all && len(v) == len(closeoutKinds), rows.Err()
}
func closeoutSnapshot(ctx context.Context, db platform.DBTX, org, id string) (closeoutBasis, []byte, string, error) {
	var b closeoutBasis
	var raw []byte
	var fingerprint string
	e := db.QueryRow(ctx, "SELECT b,md5(b::text) FROM project_closeout_snapshot($1,$2)b", org, id).Scan(&raw, &fingerprint)
	if e != nil {
		return b, nil, "", platform.DatabaseError(e)
	}
	if e = json.Unmarshal(raw, &b); e != nil {
		return b, nil, "", e
	}
	if len(b.Items) > 5000 {
		return b, nil, "", core.Fail(422, "PROJECT_CLOSEOUT_LIMIT", "结项事项超过单次完整核查上限，请先进行专项整理；系统不会截断后归档")
	}
	return b, raw, fingerprint, nil
}
func closeoutGroups(items []CloseoutItem, visible map[string]bool) []CloseoutGroup {
	out := []CloseoutGroup{}
	for _, k := range closeoutKinds {
		g := CloseoutGroup{Kind: k, Visibility: "restricted", Items: []CloseoutItem{}}
		if visible[k] {
			count, open := 0, 0
			g.Visibility = "complete"
			for _, i := range items {
				if i.Kind == k {
					g.Items = append(g.Items, i)
					count++
					if i.Blocking {
						open++
					}
				}
			}
			g.Count = &count
			g.OpenCount = &open
		}
		out = append(out, g)
	}
	return out
}
func closeoutBlocked(items []CloseoutItem) bool {
	for _, i := range items {
		if i.Blocking {
			return true
		}
	}
	return false
}
func closeoutBlockedError(items []CloseoutItem, v map[string]bool) error {
	e := core.Fail(422, "CLOSE_BLOCKED", "项目仍有未闭环事项，请查看结项核查并先恢复项目处理")
	for _, g := range closeoutGroups(items, v) {
		if g.OpenCount != nil && *g.OpenCount > 0 {
			e.Details = append(e.Details, core.Detail{Field: g.Kind, Code: "OPEN_ITEMS", SafeMessage: "该类别仍有未闭环事项"})
		}
	}
	return e
}
func (s *Service) Closeout(ctx context.Context, a core.Actor, id string) (CloseoutPreview, error) {
	out := CloseoutPreview{AsOf: time.Now().UTC(), Visibility: "restricted", Groups: []CloseoutGroup{}}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	if e = authorizeProject(ctx, tx, a, id, "project.read"); e != nil {
		return out, e
	}
	p, e := getProject(ctx, tx, a.OrgID, id, false)
	if e != nil {
		return out, e
	}
	out.Project = p
	v, all, e := closeoutVisibility(ctx, tx, a, id)
	if e != nil {
		return out, e
	}
	b, _, fp, e := closeoutSnapshot(ctx, tx, a.OrgID, id)
	if e != nil {
		return out, e
	}
	out.Groups = closeoutGroups(b.Items, v)
	if all {
		out.Visibility = "complete"
		out.Fingerprint = &fp
		out.CanClose = !closeoutBlocked(b.Items) && (p.State == "active" || p.State == "on_hold") && authz.Require(ctx, tx, a, "project.update", resource(p)) == nil
		out.CanArchive = !closeoutBlocked(b.Items) && p.State == "closing" && a.RunID == "" && authz.Require(ctx, tx, a, "project.archive", resource(p)) == nil
	}
	return out, tx.Commit(ctx)
}

func archiveHistoryReadable(ctx context.Context, db platform.DBTX, a core.Actor, raw []byte) error {
	// Recheck the historical resource classification and project allocations as
	// well as current permissions; a narrowed role cannot replay old snapshots.
	var ok bool
	e := db.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb->'items')i WHERE
 NOT can_access($1,$2,CASE i->>'kind' WHEN 'tasks' THEN 'task.read' WHEN 'milestones' THEN 'milestone.read' WHEN 'deliverables' THEN 'deliverable.read' WHEN 'risks' THEN 'risk.read' WHEN 'issues' THEN 'issue.read' WHEN 'changes' THEN 'change.read' WHEN 'budgets' THEN 'budget.read' WHEN 'contracts' THEN 'contract.read' WHEN 'purchases' THEN 'purchase.read' WHEN 'claims' THEN 'claim.read' WHEN 'costs' THEN 'cost.read' WHEN 'payments' THEN 'payment.read' WHEN 'subscriptions' THEN 'subscription.read' END,
 CASE i->>'kind' WHEN 'tasks' THEN 'task' WHEN 'milestones' THEN 'milestone' WHEN 'deliverables' THEN 'deliverable' WHEN 'risks' THEN 'risk' WHEN 'issues' THEN 'issue' WHEN 'changes' THEN 'change' WHEN 'budgets' THEN 'project' WHEN 'contracts' THEN 'contract' WHEN 'purchases' THEN 'purchase' WHEN 'claims' THEN 'claim' WHEN 'costs' THEN 'cost' WHEN 'payments' THEN 'payment' WHEN 'subscriptions' THEN 'subscription' END,
 CASE WHEN i->>'kind'='budgets' THEN ($3::jsonb->'project'->>'id')::uuid ELSE (i->>'id')::uuid END,($3::jsonb->'project'->>'id')::uuid,COALESCE((i->'facts'->'head'->>'creator_id')::uuid,(i->'facts'->>'creator_id')::uuid,($3::jsonb->'project'->>'creator_id')::uuid),COALESCE(i->'facts'->'head'->>'classification',i->'facts'->>'classification',$3::jsonb->'project'->>'classification'),$4,$5)
 OR i->>'kind'='costs' AND EXISTS(SELECT 1 FROM jsonb_array_elements(i->'facts'->'revision'->'payload'->'allocations')alloc JOIN projects p ON p.org_id=$1 AND p.id=(alloc->>'project_id')::uuid WHERE NOT can_access($1,$2,'project.read','project',p.id,p.id,p.creator_id,p.classification,$4,$5)))`, a.OrgID, a.UserID, raw, a.Channel, a.RunID != "").Scan(&ok)
	if e != nil {
		return e
	}
	if !ok {
		return core.Forbidden()
	}
	return nil
}
func readArchive(ctx context.Context, db platform.DBTX, a core.Actor, projectID, id string) (ProjectArchive, error) {
	out := ProjectArchive{FileVersionIDs: []string{}, Attachments: nil, Groups: []CloseoutGroup{}}
	p, e := getProject(ctx, db, a.OrgID, projectID, false)
	if e != nil {
		return out, e
	}
	if e = authorizeProject(ctx, db, a, projectID, "project.read"); e != nil {
		return out, e
	}
	v, all, e := closeoutVisibility(ctx, db, a, projectID)
	if e != nil {
		return out, e
	}
	if !all {
		return out, core.Forbidden()
	}
	var raw []byte
	var current bool
	e = db.QueryRow(ctx, `SELECT a.id,a.project_id,a.project_version,a.basis->'project'->>'name',a.fingerprint,a.summary,a.lessons_learned,a.follow_up,a.business_verified,a.financial_verified,a.actor_id,(SELECT display_name FROM users WHERE org_id=a.org_id AND id=a.actor_id),a.created_at,a.reason,a.basis,
 ARRAY(SELECT f.version_id::text FROM project_archive_files f WHERE f.org_id=a.org_id AND f.archive_id=a.id ORDER BY f.version_id),
 p.state='archived' AND p.version=a.project_version AND (to_jsonb(p)-'state'-'version'-'updated_at')=((a.basis->'project')-'state'-'version'-'updated_at') AND (project_closeout_snapshot(a.org_id,p.id)->'items')=a.basis->'items'
 FROM project_archives a JOIN projects p ON p.org_id=a.org_id AND p.id=a.project_id WHERE a.org_id=$1 AND a.project_id=$2 AND a.id=$3`, a.OrgID, projectID, id).Scan(&out.ID, &out.ProjectID, &out.ProjectVersion, &out.ProjectName, &out.Fingerprint, &out.Summary, &out.LessonsLearned, &out.FollowUp, &out.BusinessVerified, &out.FinancialVerified, &out.ActorID, &out.ActorName, &out.CreatedAt, &out.Reason, &raw, &out.FileVersionIDs, &current)
	if e != nil {
		return out, platform.DatabaseError(e)
	}
	if e = archiveHistoryReadable(ctx, db, a, raw); e != nil {
		return ProjectArchive{}, e
	}
	var b closeoutBasis
	if e = json.Unmarshal(raw, &b); e != nil {
		return out, e
	}
	out.Groups = closeoutGroups(b.Items, v)
	out.CurrentBasisState = "changed"
	if current {
		out.CurrentBasisState = "current"
	}
	out.Attachments, e = deliverableFiles(ctx, db, a, p, out.FileVersionIDs, false)
	return out, e
}
func (s *Service) Archives(ctx context.Context, a core.Actor, id string, f Filter) (ArchiveList, error) {
	out := ArchiveList{Visibility: "restricted", Items: []ProjectArchive{}}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	if e = authorizeProject(ctx, tx, a, id, "project.read"); e != nil {
		return out, e
	}
	_, all, e := closeoutVisibility(ctx, tx, a, id)
	if e != nil {
		return out, e
	}
	if !all {
		return out, tx.Commit(ctx)
	}
	// Bound history before pagination without letting restricted records affect
	// cursors/counts. Project archive records cannot be updated or deleted.
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.CursorBinding(a, "project-archives", id)
	var before *platform.Cursor
	if f.Cursor != "" {
		cursor, e := s.Pager.Decode(f.Cursor, binding)
		if e != nil {
			return out, e
		}
		before = &cursor
	}
	rows, e := tx.Query(ctx, "SELECT id::text FROM project_archives WHERE org_id=$1 AND project_id=$2 ORDER BY created_at DESC,id DESC LIMIT 501", a.OrgID, id)
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
	if len(ids) > 500 {
		return out, core.Fail(422, "PROJECT_ARCHIVE_HISTORY_LIMIT", "归档历史超过单次完整授权核查上限，请安排专项整理")
	}
	for _, rid := range ids {
		r, e := readArchive(ctx, tx, a, id, rid)
		if e != nil {
			var apiErr *core.Error
			if errors.As(e, &apiErr) && (apiErr.Status == 403 || apiErr.Status == 404) {
				return ArchiveList{Visibility: "restricted", Items: []ProjectArchive{}}, tx.Commit(ctx)
			}
			return out, e
		}
		if before != nil && (r.CreatedAt.After(before.UpdatedAt) || r.CreatedAt.Equal(before.UpdatedAt) && r.ID >= before.ID) {
			continue
		}
		out.Items = append(out.Items, r)
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[len(out.Items)-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &cursor
	}
	out.Visibility = "complete"
	return out, tx.Commit(ctx)
}
