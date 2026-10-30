package servicestatus

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"time"
)

const serviceColumns = `s.id,s.project_id,s.creator_id,s.classification,s.name,s.owner_id,s.host_label,s.endpoint_label,s.description,s.lifecycle,s.version,s.created_at,s.updated_at`

func scanService(row pgx.Row) (Record, error) {
	var d Record
	err := row.Scan(&d.ID, &d.ProjectID, &d.CreatorID, &d.Classification, &d.Name, &d.OwnerID, &d.HostLabel, &d.EndpointLabel, &d.Description, &d.Lifecycle, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	return d, platform.DatabaseError(err)
}
func metadata(ctx context.Context, db platform.DBTX, org, id string) (Record, error) {
	if !core.ValidID(id) {
		return Record{}, core.Missing()
	}
	return scanService(db.QueryRow(ctx, "SELECT "+serviceColumns+" FROM services s WHERE s.org_id=$1 AND s.id=$2", org, id))
}
func resource(d Record) authz.Resource {
	return authz.Resource{Type: "service", ID: d.ID, ProjectID: d.ProjectID, OwnerID: &d.CreatorID, Classification: d.Classification}
}
func projectWritable(ctx context.Context, db platform.DBTX, org string, project *string, class string, lock bool) error {
	if project == nil {
		return nil
	}
	q := "SELECT state,classification_rank(classification)<=classification_rank($3) FROM projects WHERE org_id=$1 AND id=$2"
	if lock {
		q += " FOR UPDATE"
	}
	var state string
	var adequate bool
	if err := db.QueryRow(ctx, q, org, *project, class).Scan(&state, &adequate); err != nil {
		return platform.DatabaseError(err)
	}
	if !adequate {
		return core.Invalid("classification", "服务分类不能低于所属项目")
	}
	if slices.Contains([]string{"closing", "archived", "cancelled"}, state) {
		return core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许修改服务配置")
	}
	return nil
}
func ownerAllowed(ctx context.Context, db platform.DBTX, org, id string, project *string) error {
	var ok bool
	if !core.ValidID(id) {
		return core.Invalid("owner_id", "请选择有效负责人")
	}
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users u WHERE u.org_id=$1 AND u.id=$2 AND u.status='active'
 AND ($3::uuid IS NULL OR EXISTS(SELECT 1 FROM project_members m WHERE m.org_id=u.org_id AND m.project_id=$3 AND m.user_id=u.id)))`, org, id, project).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return core.Invalid("owner_id", "负责人必须是当前范围内的有效成员")
	}
	return nil
}
func validateContent(d Content, reason string) error {
	if core.Name(d.Name) != nil || !core.ValidID(d.OwnerID) || core.Name(reason) != nil || len([]rune(d.HostLabel)) > 200 || len([]rune(d.EndpointLabel)) > 500 || len([]rune(d.Description)) > 10000 {
		return core.Invalid("service", "请核对服务名称、负责人、标签和说明长度")
	}
	// Labels are not probe instructions. Refuse embedded control characters, which
	// could otherwise confuse monitoring exports and operator logs.
	if strings.ContainsAny(d.Name+d.HostLabel+d.EndpointLabel, "\r\n\x00") {
		return core.Invalid("service", "名称与标签不能包含控制字符")
	}
	return nil
}
func (s *Service) present(ctx context.Context, db platform.DBTX, a core.Actor, d Record) (Record, error) {
	if err := authz.Require(ctx, db, a, "service.read", resource(d)); err != nil {
		return Record{}, err
	}
	if err := db.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&d.EvaluatedAt); err != nil {
		return Record{}, err
	}
	var err error
	d.Sources, err = readSources(ctx, db, a.OrgID, d.ID, d.EvaluatedAt)
	if err != nil {
		return Record{}, err
	}
	states := make([]State, 0, len(d.Sources))
	d.RecheckAt = d.EvaluatedAt.Add(10 * time.Second)
	beforeDeadline := func(at time.Time) {
		if at.After(d.EvaluatedAt) && at.Before(d.RecheckAt) {
			d.RecheckAt = at
		}
	}
	for _, source := range d.Sources {
		states = append(states, source.State)
		if source.State != Unknown && source.Projection != nil {
			stale := time.Duration(source.StaleAfter) * time.Second
			beforeDeadline(source.Projection.ObservedAt.Add(stale))
			beforeDeadline(source.Projection.ReceivedAt.Add(stale))
		}
	}
	rows, err := db.Query(ctx, `SELECT w.id,w.start_at,w.end_at FROM maintenance_windows w WHERE w.org_id=$1 AND w.service_id=$2
 AND w.end_at>$3 AND NOT EXISTS(SELECT 1 FROM maintenance_cancellations c WHERE c.org_id=w.org_id AND c.window_id=w.id) ORDER BY w.start_at,w.id`, a.OrgID, d.ID, d.EvaluatedAt)
	if err != nil {
		return Record{}, err
	}
	windows := []Window{}
	for rows.Next() {
		var w Window
		if err = rows.Scan(&w.ID, &w.Start, &w.End); err != nil {
			rows.Close()
			return Record{}, err
		}
		windows = append(windows, w)
		beforeDeadline(w.Start)
		beforeDeadline(w.End)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Record{}, err
	}
	d.Summary = Summarize(states, windows, d.EvaluatedAt)
	writable := projectWritable(ctx, db, a.OrgID, d.ProjectID, d.Classification, false) == nil
	d.CanManage = writable && authz.Require(ctx, db, a, "service.manage", resource(d)) == nil && a.RunID == "" && a.Channel != "telegram"
	d.CanUpdateIncident = authz.Require(ctx, db, a, "incident.update", resource(d)) == nil && a.RunID == "" && a.Channel != "telegram"
	return d, nil
}
func (s *Service) Get(ctx context.Context, a core.Actor, id string) (Record, error) {
	d, err := metadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return Record{}, err
	}
	return s.present(ctx, s.DB, a, d)
}
func (s *Service) List(ctx context.Context, a core.Actor, f Filter) (core.Page[Record], error) {
	out := core.Page[Record]{Items: []Record{}}
	if len([]rune(f.Query)) > 200 || !slices.Contains([]string{"", "active", "archived"}, f.Lifecycle) || (f.ProjectID != nil && !core.ValidID(*f.ProjectID)) {
		return out, core.Invalid("filter", "服务筛选无效")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	project := ""
	if f.ProjectID != nil {
		project = *f.ProjectID
	}
	bound := core.CursorBinding(a, "services", f.Query, f.Lifecycle, project)
	var at *time.Time
	var id *string
	if f.Cursor != "" {
		c, err := s.Pager.Decode(f.Cursor, bound)
		if err != nil {
			return out, err
		}
		at = &c.UpdatedAt
		id = &c.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+serviceColumns+` FROM services s WHERE s.org_id=$1 AND ($2='' OR s.lifecycle=$2)
 AND ($3='' OR position(lower($3) in lower(s.name))>0) AND ($4::uuid IS NULL OR s.project_id=$4)
 AND can_access(s.org_id,$5,'service.read','service',s.id,s.project_id,s.creator_id,s.classification,$6,$7)
 AND ($8::timestamptz IS NULL OR (s.updated_at,s.id)<($8,$9::uuid)) ORDER BY s.updated_at DESC,s.id DESC LIMIT $10`, a.OrgID, f.Lifecycle, f.Query, f.ProjectID, a.UserID, a.Channel, a.RunID != "", at, id, f.Limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		d, e := scanService(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[f.Limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: bound})
		out.NextCursor = &c
	}
	for i, d := range out.Items {
		out.Items[i], err = s.present(ctx, s.DB, a, d)
		if err != nil {
			return core.Page[Record]{Items: []Record{}}, err
		}
	}
	return out, nil
}
func (s *Service) refresh(ctx context.Context, a core.Actor, out core.Result, err error) (core.Result, error) {
	if err != nil || !out.Replayed {
		return out, err
	}
	raw, e := json.Marshal(out.Data)
	if e != nil {
		return core.Result{}, e
	}
	var d Record
	if e = json.Unmarshal(raw, &d); e != nil {
		return core.Result{}, e
	}
	out.Data, e = s.Get(ctx, a, d.ID)
	return out, e
}
func (s *Service) Create(ctx context.Context, c core.Command, in Create) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.refresh(ctx, c.Actor, out, commandErr) }()
	if err := validateContent(in.Content, in.Reason); err != nil {
		return out, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if !core.Classification(in.Classification) || (in.ProjectID != nil && !core.ValidID(*in.ProjectID)) || (in.Classification == "project_restricted" && in.ProjectID == nil) {
			return core.Invalid("classification", "请明确服务范围与分类")
		}
		if in.Classification == "confidential" {
			return core.Fail(422, "EXPLICIT_GRANT_REQUIRED", "严格受限服务须使用独立对象授权流程")
		}
		r := authz.Resource{Type: "service", ProjectID: in.ProjectID, OwnerID: &c.Actor.UserID, Classification: in.Classification}
		for _, action := range []string{"service.read", "service.manage"} {
			if e := authz.Require(ctx, tx, c.Actor, action, r); e != nil {
				return e
			}
		}
		return identity.RequireInteractiveSession(ctx, tx, c.Actor, true)
	}, func(tx pgx.Tx) (core.Result, error) {
		if err := projectWritable(ctx, tx, c.Actor.OrgID, in.ProjectID, in.Classification, true); err != nil {
			return core.Result{}, err
		}
		if err := ownerAllowed(ctx, tx, c.Actor.OrgID, in.OwnerID, in.ProjectID); err != nil {
			return core.Result{}, err
		}
		id := core.NewID()
		_, err := tx.Exec(ctx, `INSERT INTO services(id,org_id,project_id,creator_id,owner_id,classification,name,host_label,endpoint_label,description)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, c.Actor.OrgID, in.ProjectID, c.Actor.UserID, in.OwnerID, in.Classification, strings.TrimSpace(in.Name), in.HostLabel, in.EndpointLabel, in.Description)
		if err != nil {
			return core.Result{}, err
		}
		d, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		d, err = s.present(ctx, tx, c.Actor, d)
		return serviceResult(d, "service.created", 201, in.Reason), err
	})
}
func serviceResult(d Record, event string, status int, reason string) core.Result {
	return core.Result{Status: status, Data: d, Event: core.Event{Type: event, ResourceType: "service", ResourceID: d.ID, Version: d.Version, Payload: map[string]any{"reason": reason}}}
}
func (s *Service) mutate(ctx context.Context, c core.Command, id string, version int64, fn func(pgx.Tx, Record) (core.Result, error)) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.refresh(ctx, c.Actor, out, commandErr) }()
	if err := core.RequireVersion(version); err != nil {
		return out, err
	}
	check := func(tx pgx.Tx) (Record, error) {
		d, e := metadata(ctx, tx, c.Actor.OrgID, id)
		if e != nil {
			return d, e
		}
		for _, action := range []string{"service.read", "service.manage"} {
			if e = authz.Require(ctx, tx, c.Actor, action, resource(d)); e != nil {
				return d, e
			}
		}
		return d, identity.RequireInteractiveSession(ctx, tx, c.Actor, true)
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := check(tx); return e }, func(tx pgx.Tx) (core.Result, error) {
		d, e := check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if e = projectWritable(ctx, tx, c.Actor.OrgID, d.ProjectID, d.Classification, true); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "SELECT id FROM services WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id); e != nil {
			return core.Result{}, e
		}
		d, e = check(tx)
		if e != nil {
			return core.Result{}, e
		}
		if d.Version != version {
			return core.Result{}, core.Conflict()
		}
		raw, _ := json.Marshal(d)
		result, e := fn(tx, d)
		result.Event.BeforeDigest = core.Digest(raw)
		return result, e
	})
}
func (s *Service) Update(ctx context.Context, c core.Command, id string, in Patch) (core.Result, error) {
	if e := validateContent(in.Content, in.Reason); e != nil {
		return core.Result{}, e
	}
	if !slices.Contains([]string{"active", "archived"}, in.Lifecycle) {
		return core.Result{}, core.Invalid("lifecycle", "服务目录状态无效")
	}
	return s.mutate(ctx, c, id, in.ExpectedVersion, func(tx pgx.Tx, d Record) (core.Result, error) {
		if e := ownerAllowed(ctx, tx, c.Actor.OrgID, in.OwnerID, d.ProjectID); e != nil {
			return core.Result{}, e
		}
		_, e := tx.Exec(ctx, `UPDATE services SET name=$3,owner_id=$4,host_label=$5,endpoint_label=$6,description=$7,lifecycle=$8,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, c.Actor.OrgID, id, strings.TrimSpace(in.Name), in.OwnerID, in.HostLabel, in.EndpointLabel, in.Description, in.Lifecycle)
		if e != nil {
			return core.Result{}, e
		}
		if e = projectServiceResource(ctx, tx, c.Actor.OrgID, id); e != nil {
			return core.Result{}, e
		}
		return s.changed(ctx, tx, c.Actor, id, "service.updated", in.Reason)
	})
}
func (s *Service) changed(ctx context.Context, tx pgx.Tx, a core.Actor, id, event, reason string) (core.Result, error) {
	d, e := metadata(ctx, tx, a.OrgID, id)
	if e != nil {
		return core.Result{}, e
	}
	d, e = s.present(ctx, tx, a, d)
	return serviceResult(d, event, 200, reason), e
}
