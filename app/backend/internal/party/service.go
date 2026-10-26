package party

import (
	"context"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const columns = `p.id,p.version,p.project_id,p.creator_id,p.classification,p.state,p.current_revision_id,p.created_at,p.updated_at,
    r.name,r.kind,r.relationships,r.contact_name,r.email,r.phone,r.notes`
const currentJoin = ` FROM parties p JOIN party_revisions r ON r.org_id=p.org_id AND r.party_id=p.id AND r.id=p.current_revision_id `

func scan(row pgx.Row) (Party, error) {
	var p Party
	err := row.Scan(&p.ID, &p.Version, &p.ProjectID, &p.CreatorID, &p.Classification, &p.State, &p.CurrentRevisionID, &p.CreatedAt, &p.UpdatedAt,
		&p.Name, &p.Kind, &p.Relationships, &p.ContactName, &p.Email, &p.Phone, &p.Notes)
	return p, platform.DatabaseError(err)
}
func resource(p Party) authz.Resource {
	return authz.Resource{Type: "party", ID: p.ID, ProjectID: p.ProjectID, OwnerID: &p.CreatorID, Classification: p.Classification}
}
func metadata(ctx context.Context, db platform.DBTX, org, id string) (Party, error) {
	if !core.ValidID(id) {
		return Party{}, core.Missing()
	}
	return scan(db.QueryRow(ctx, "SELECT "+columns+currentJoin+"WHERE p.org_id=$1 AND p.id=$2", org, id))
}
func projectWritable(ctx context.Context, db platform.DBTX, org string, project *string, class string, lock bool) error {
	if project == nil {
		return nil
	}
	query := "SELECT state,classification_rank(classification)<=classification_rank($3) FROM projects WHERE org_id=$1 AND id=$2"
	if lock {
		query += " FOR UPDATE"
	}
	var state string
	var adequate bool
	if err := db.QueryRow(ctx, query, org, *project, class).Scan(&state, &adequate); err != nil {
		return platform.DatabaseError(err)
	}
	if !adequate {
		return core.Invalid("classification", "往来资料分类不能低于所属项目")
	}
	if slices.Contains([]string{"closing", "archived", "cancelled"}, state) {
		return core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许修改往来资料")
	}
	return nil
}
func present(ctx context.Context, db platform.DBTX, a core.Actor, p Party) (Party, error) {
	if err := authz.Require(ctx, db, a, "party.read", resource(p)); err != nil {
		return Party{}, err
	}
	writable := projectWritable(ctx, db, a.OrgID, p.ProjectID, p.Classification, false) == nil
	p.CanEdit = p.State == "active" && writable && authz.Require(ctx, db, a, "party.update", resource(p)) == nil
	p.CanArchive = writable && authz.Require(ctx, db, a, "party.archive", resource(p)) == nil
	p.CanReadHistory = authz.Require(ctx, db, a, "party.update", resource(p)) == nil
	return p, nil
}
func (s *Service) Get(ctx context.Context, a core.Actor, id string) (Party, error) {
	p, err := metadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return Party{}, err
	}
	return present(ctx, s.DB, a, p)
}
func binding(a core.Actor, scope string) string {
	return core.Digest([]byte(fmt.Sprint(a.OrgID, "/", a.UserID, "/", a.AuthVersion, "/", a.OrgAuthVersion, "/", a.Channel, "/", a.RunID, "/party/", scope)))
}
func (s *Service) List(ctx context.Context, a core.Actor, f Filter) (core.Page[Party], error) {
	out := core.Page[Party]{Items: []Party{}}
	if len([]rune(f.Query)) > 200 || !slices.Contains([]string{"", "active", "archived"}, f.State) || !slices.Contains([]string{"", "supplier", "customer", "other"}, f.Relationship) || (f.ProjectID != nil && !core.ValidID(*f.ProjectID)) {
		return out, core.Invalid("filter", "往来资料筛选无效")
	}
	if f.CompanyOnly && f.ProjectID != nil {
		return out, core.Invalid("company_only", "公司资料与项目筛选不能同时指定")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	scope := ""
	if f.ProjectID != nil {
		scope = *f.ProjectID
	}
	bound := binding(a, fmt.Sprint(f.Query, "/", f.State, "/", f.Relationship, "/", scope, "/", f.CompanyOnly))
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
	rows, err := s.DB.Query(ctx, "SELECT "+columns+currentJoin+`WHERE p.org_id=$1 AND ($2='' OR p.state=$2)
        AND ($3='' OR position(lower($3) in lower(r.name))>0) AND ($4='' OR $4=ANY(r.relationships))
        AND (NOT $12 OR p.project_id IS NULL) AND ($5::uuid IS NULL OR p.project_id=$5) AND can_access(p.org_id,$6,'party.read','party',p.id,p.project_id,p.creator_id,p.classification,$7,$8)
        AND ($9::timestamptz IS NULL OR (p.updated_at,p.id)<($9,$10::uuid)) ORDER BY p.updated_at DESC,p.id DESC LIMIT $11`,
		a.OrgID, f.State, f.Query, f.Relationship, f.ProjectID, a.UserID, a.Channel, a.RunID != "", at, id, f.Limit+1, f.CompanyOnly)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		p, e := scan(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for i, p := range out.Items {
		out.Items[i], err = present(ctx, s.DB, a, p)
		if err != nil {
			return core.Page[Party]{Items: []Party{}}, err
		}
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		p := out.Items[f.Limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: p.ID, UpdatedAt: p.UpdatedAt, Binding: bound})
		out.NextCursor = &c
	}
	return out, nil
}
func validate(content Content, reason string) error {
	if core.Name(content.Name) != nil || core.Name(reason) != nil || !slices.Contains([]string{"organization", "person"}, content.Kind) || len(content.Relationships) < 1 || len(content.Relationships) > 3 || len([]rune(content.ContactName)) > 200 || len(content.Email) > 254 || len([]rune(content.Phone)) > 100 || len([]rune(content.Notes)) > 10000 {
		return core.Invalid("party", "请核对名称、类型、关系和联系资料长度")
	}
	seen := map[string]bool{}
	for _, v := range content.Relationships {
		if !slices.Contains([]string{"supplier", "customer", "other"}, v) || seen[v] {
			return core.Invalid("relationships", "往来关系无效或重复")
		}
		seen[v] = true
	}
	if content.Email != "" {
		address, err := mail.ParseAddress(content.Email)
		if err != nil || address.Address != content.Email || strings.ContainsAny(content.Email, "\r\n") {
			return core.Invalid("email", "请输入单一有效邮箱地址")
		}
	}
	return nil
}
func insertRevision(ctx context.Context, tx pgx.Tx, a core.Actor, id, revision string, seq int64, base *string, content Content, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO party_revisions(id,org_id,party_id,seq,base_revision_id,name,kind,relationships,contact_name,email,phone,notes,author_id,reason)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, revision, a.OrgID, id, seq, base, strings.TrimSpace(content.Name), content.Kind, content.Relationships, content.ContactName, content.Email, content.Phone, content.Notes, a.UserID, reason)
	return err
}
func (s *Service) Create(ctx context.Context, c core.Command, in Create) (core.Result, error) {
	if err := validate(in.Content, in.Reason); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if !core.Classification(in.Classification) || (in.ProjectID != nil && !core.ValidID(*in.ProjectID)) || (in.Classification == "project_restricted" && in.ProjectID == nil) {
			return core.Invalid("classification", "请明确有效项目与资料分类")
		}
		if in.Classification == "confidential" {
			return core.Fail(422, "EXPLICIT_GRANT_REQUIRED", "严格受限资料须使用独立分类与对象授权流程")
		}
		r := authz.Resource{Type: "party", ProjectID: in.ProjectID, OwnerID: &c.Actor.UserID, Classification: in.Classification}
		if err := authz.Require(ctx, tx, c.Actor, "party.create", r); err != nil {
			return err
		}
		return authz.Require(ctx, tx, c.Actor, "party.read", r)
	}, func(tx pgx.Tx) (core.Result, error) {
		if err := projectWritable(ctx, tx, c.Actor.OrgID, in.ProjectID, in.Classification, true); err != nil {
			return core.Result{}, err
		}
		id, revision := core.NewID(), core.NewID()
		if _, err := tx.Exec(ctx, "INSERT INTO parties(id,org_id,project_id,creator_id,classification,current_revision_id) VALUES($1,$2,$3,$4,$5,$6)", id, c.Actor.OrgID, in.ProjectID, c.Actor.UserID, in.Classification, revision); err != nil {
			return core.Result{}, err
		}
		if err := insertRevision(ctx, tx, c.Actor, id, revision, 1, nil, in.Content, in.Reason); err != nil {
			return core.Result{}, err
		}
		p, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		p, err = present(ctx, tx, c.Actor, p)
		return result(p, "party.created", 201, in.Reason), err
	})
}
func result(p Party, event string, status int, reason string) core.Result {
	return core.Result{Status: status, Data: p, Event: core.Event{Type: event, ResourceType: "party", ResourceID: p.ID, Version: p.Version, Payload: map[string]any{"revision_id": p.CurrentRevisionID, "reason": reason}}}
}
