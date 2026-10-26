package project

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type TemplateTask struct {
	Title          string `json:"title"`
	Body           string `json:"body"`
	Priority       string `json:"priority"`
	ReviewRequired bool   `json:"review_required"`
}
type TemplateMilestone struct {
	Title string `json:"title"`
}
type TemplateSchema struct {
	Folders    []string            `json:"folders"`
	Tasks      []TemplateTask      `json:"tasks"`
	Milestones []TemplateMilestone `json:"milestones"`
}
type Template struct {
	ID          string         `json:"id"`
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Revision    int64          `json:"revision"`
	Version     int64          `json:"version"`
	State       string         `json:"state"`
	Schema      TemplateSchema `json:"schema"`
	ContentHash string         `json:"content_hash"`
	CreatorID   *string        `json:"creator_id"`
	BasedOnID   *string        `json:"based_on_id"`
	PublishedAt *time.Time     `json:"published_at"`
	PublishedBy *string        `json:"published_by"`
	RetiredAt   *time.Time     `json:"retired_at"`
	RetiredBy   *string        `json:"retired_by"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}
type TemplateContent struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      TemplateSchema `json:"schema"`
}
type TemplateCreate struct {
	TemplateContent
	Key    string `json:"key"`
	Reason string `json:"reason"`
}
type TemplatePatch struct {
	TemplateContent
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}
type TemplateAction struct {
	ExpectedVersion int64  `json:"expected_version"`
	ContentHash     string `json:"content_hash"`
	Reason          string `json:"reason"`
}
type TemplateEvent struct {
	ID          string    `json:"id"`
	TemplateID  string    `json:"template_id"`
	Version     int64     `json:"version"`
	Kind        string    `json:"kind"`
	ActorID     string    `json:"actor_id"`
	Reason      string    `json:"reason"`
	ContentHash string    `json:"content_hash"`
	Snapshot    Template  `json:"snapshot"`
	CreatedAt   time.Time `json:"created_at"`
}

var templateKey = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func validTemplateSchema(in TemplateSchema) (TemplateSchema, error) {
	if len(in.Folders) > 100 || len(in.Tasks) > 200 || len(in.Milestones) > 100 {
		return in, core.Invalid("schema", "模板最多包含100个目录、200项任务及100个里程碑")
	}
	if in.Folders == nil {
		in.Folders = []string{}
	}
	if in.Tasks == nil {
		in.Tasks = []TemplateTask{}
	}
	if in.Milestones == nil {
		in.Milestones = []TemplateMilestone{}
	}
	names := map[string]bool{}
	for _, name := range in.Folders {
		key, err := core.StorageNameKey(name)
		if err != nil {
			return in, err
		}
		if names[key] {
			return in, core.Invalid("schema.folders", "目录名称经标准化后不能重复")
		}
		names[key] = true
	}
	for i := range in.Tasks {
		t := &in.Tasks[i]
		t.Title = strings.TrimSpace(t.Title)
		if err := core.Name(t.Title); err != nil {
			return in, err
		}
		if len([]rune(t.Body)) > 20000 {
			return in, core.Invalid("schema.tasks", "任务说明过长")
		}
		if t.Priority == "" {
			t.Priority = "normal"
		}
		if t.Priority != "low" && t.Priority != "normal" && t.Priority != "high" && t.Priority != "urgent" {
			return in, core.Invalid("schema.tasks", "任务优先级无效")
		}
	}
	for i := range in.Milestones {
		in.Milestones[i].Title = strings.TrimSpace(in.Milestones[i].Title)
		if err := core.Name(in.Milestones[i].Title); err != nil {
			return in, err
		}
	}
	body, _ := json.Marshal(in)
	if len(body) > 262144 {
		return in, core.Invalid("schema", "模板内容最多256KiB")
	}
	return in, nil
}
func validTemplateContent(in TemplateContent) (TemplateContent, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := core.Name(in.Name); err != nil {
		return in, err
	}
	if len([]rune(in.Description)) > 2000 {
		return in, core.Invalid("description", "模板说明最多2000字符")
	}
	schema, err := validTemplateSchema(in.Schema)
	in.Schema = schema
	return in, err
}
func templateDigest(t Template) string {
	b, _ := json.Marshal(struct {
		Key string `json:"key"`
		TemplateContent
	}{t.Key, TemplateContent{t.Name, t.Description, t.Schema}})
	canonical, _ := core.CanonicalJSON(b)
	return core.Digest(canonical)
}

const templateColumns = `id,key,name,description,revision,version,state,task_and_folder_schema,creator_id,based_on_id,published_at,published_by,retired_at,retired_by,created_at,updated_at`

func scanTemplate(row pgx.Row) (Template, error) {
	var t Template
	var raw []byte
	err := row.Scan(&t.ID, &t.Key, &t.Name, &t.Description, &t.Revision, &t.Version, &t.State, &raw, &t.CreatorID, &t.BasedOnID, &t.PublishedAt, &t.PublishedBy, &t.RetiredAt, &t.RetiredBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return t, platform.DatabaseError(err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&t.Schema); err != nil {
		return t, core.Fail(409, "TEMPLATE_SCHEMA_INVALID", "模板结构无效，请由配置管理员核对")
	}
	if t.Schema.Folders == nil {
		t.Schema.Folders = []string{}
	}
	if t.Schema.Tasks == nil {
		t.Schema.Tasks = []TemplateTask{}
	}
	if t.Schema.Milestones == nil {
		t.Schema.Milestones = []TemplateMilestone{}
	}
	t.ContentHash = templateDigest(t)
	return t, nil
}
func getTemplate(ctx context.Context, db platform.DBTX, org, id, lock string) (Template, error) {
	if !core.ValidID(id) {
		return Template{}, core.Missing()
	}
	return scanTemplate(db.QueryRow(ctx, "SELECT "+templateColumns+" FROM project_templates WHERE org_id=$1 AND id=$2"+lock, org, id))
}
func templateReader(ctx context.Context, db platform.DBTX, a core.Actor) (bool, error) {
	if a.RunID != "" || a.Channel == "telegram" {
		return false, core.Forbidden()
	}
	var manager bool
	if err := db.QueryRow(ctx, "SELECT can_access($1,$2,'config.manage','organization',$1,NULL,NULL,'internal',$3,false)", a.OrgID, a.UserID, a.Channel).Scan(&manager); err != nil {
		return false, err
	}
	if !manager {
		if err := authz.RequireOrganization(ctx, db, a, "project.create"); err != nil {
			return false, err
		}
	}
	return manager, nil
}
func templateAuthority(ctx context.Context, tx pgx.Tx, a core.Actor) error {
	if err := authz.RequireOrganization(ctx, tx, a, "config.manage"); err != nil {
		return err
	}
	return identity.RequireInteractiveSession(ctx, tx, a, true)
}
func (s *Service) Templates(ctx context.Context, a core.Actor, f Filter, key string) (core.Page[Template], error) {
	out := core.Page[Template]{Items: []Template{}}
	manager, err := templateReader(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	if f.State != "" && f.State != "draft" && f.State != "published" && f.State != "archived" {
		return out, core.Invalid("state", "模板状态无效")
	}
	if !manager && f.State != "" && f.State != "published" {
		return out, core.Missing()
	}
	if key != "" && !templateKey.MatchString(key) {
		return out, core.Invalid("key", "模板系列标识无效")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.CursorBinding(a, "project-templates", f.State, f.Query, key)
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
	rows, err := s.DB.Query(ctx, "SELECT "+templateColumns+` FROM project_templates WHERE org_id=$1 AND ($2 OR state='published')
 AND ($3='' OR state=$3) AND ($4='' OR position(lower($4) IN lower(name||' '||key||' '||description))>0)
 AND ($5='' OR key=$5) AND ($6::timestamptz IS NULL OR (updated_at,id)<($6,$7::uuid))
 ORDER BY updated_at DESC,id DESC LIMIT $8`, a.OrgID, manager, f.State, f.Query, key, before, beforeID, f.Limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		t, e := scanTemplate(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, t)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		t := out.Items[f.Limit-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: t.ID, UpdatedAt: t.UpdatedAt, Binding: binding})
		out.NextCursor = &cursor
	}
	return out, nil
}
func (s *Service) GetTemplate(ctx context.Context, a core.Actor, id string) (Template, error) {
	manager, err := templateReader(ctx, s.DB, a)
	if err != nil {
		return Template{}, err
	}
	t, err := getTemplate(ctx, s.DB, a.OrgID, id, "")
	if err == nil && !manager && t.State != "published" {
		return Template{}, core.Missing()
	}
	return t, err
}
func appendTemplateEvent(ctx context.Context, tx pgx.Tx, a core.Actor, t Template, kind, reason string) error {
	snapshot, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO project_template_events(id,org_id,template_id,version,kind,actor_id,reason,content_hash,snapshot)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, core.NewID(), a.OrgID, t.ID, t.Version, kind, a.UserID, reason, t.ContentHash, snapshot)
	return err
}
func templateResult(t Template, status int, kind string) core.Result {
	return core.Result{Status: status, Data: t, Event: core.Event{Type: "project_template." + kind, ResourceType: "project_template", ResourceID: t.ID, Version: t.Version, Payload: map[string]any{"template_id": t.ID, "revision": t.Revision, "content_hash": t.ContentHash}}}
}
func (s *Service) CreateTemplate(ctx context.Context, c core.Command, in TemplateCreate) (core.Result, error) {
	if !templateKey.MatchString(in.Key) {
		return core.Result{}, core.Invalid("key", "系列标识需为小写字母开头的1至64位字母、数字或连字符")
	}
	content, err := validTemplateContent(in.TemplateContent)
	if err != nil {
		return core.Result{}, err
	}
	if err = core.Name(in.Reason); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return templateAuthority(ctx, tx, c.Actor) }, func(tx pgx.Tx) (core.Result, error) {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,64))", c.Actor.OrgID+"/"+in.Key); err != nil {
			return core.Result{}, err
		}
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM project_templates WHERE org_id=$1 AND key=$2)", c.Actor.OrgID, in.Key).Scan(&exists); err != nil {
			return core.Result{}, err
		}
		if exists {
			return core.Result{}, core.Fail(409, "TEMPLATE_KEY_EXISTS", "模板系列已存在，请从其固定修订建立新草稿")
		}
		id := core.NewID()
		body, _ := json.Marshal(content.Schema)
		_, err := tx.Exec(ctx, `INSERT INTO project_templates(id,org_id,key,name,description,revision,state,task_and_folder_schema,creator_id)
  VALUES($1,$2,$3,$4,$5,1,'draft',$6,$7)`, id, c.Actor.OrgID, in.Key, content.Name, content.Description, body, c.Actor.UserID)
		if err != nil {
			return core.Result{}, err
		}
		t, err := getTemplate(ctx, tx, c.Actor.OrgID, id, "")
		if err != nil {
			return core.Result{}, err
		}
		if err = appendTemplateEvent(ctx, tx, c.Actor, t, "created", in.Reason); err != nil {
			return core.Result{}, err
		}
		return templateResult(t, 201, "created"), nil
	})
}
func (s *Service) UpdateTemplate(ctx context.Context, c core.Command, id string, in TemplatePatch) (core.Result, error) {
	content, err := validTemplateContent(in.TemplateContent)
	if err != nil {
		return core.Result{}, err
	}
	if err = core.RequireVersion(in.ExpectedVersion); err != nil {
		return core.Result{}, err
	}
	if err = core.Name(in.Reason); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return templateAuthority(ctx, tx, c.Actor) }, func(tx pgx.Tx) (core.Result, error) {
		t, err := getTemplate(ctx, tx, c.Actor.OrgID, id, " FOR UPDATE")
		if err != nil {
			return core.Result{}, err
		}
		if t.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if t.State != "draft" {
			return core.Result{}, core.Fail(409, "TEMPLATE_IMMUTABLE", "发布或停用的模板内容不可改写，请建立新修订")
		}
		body, _ := json.Marshal(content.Schema)
		_, err = tx.Exec(ctx, "UPDATE project_templates SET name=$3,description=$4,task_and_folder_schema=$5,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, content.Name, content.Description, body)
		if err != nil {
			return core.Result{}, err
		}
		t, err = getTemplate(ctx, tx, c.Actor.OrgID, id, "")
		if err != nil {
			return core.Result{}, err
		}
		if err = appendTemplateEvent(ctx, tx, c.Actor, t, "updated", in.Reason); err != nil {
			return core.Result{}, err
		}
		return templateResult(t, 200, "updated"), nil
	})
}
func (s *Service) ActTemplate(ctx context.Context, c core.Command, id, action string, in TemplateAction) (core.Result, error) {
	if err := core.RequireVersion(in.ExpectedVersion); err != nil {
		return core.Result{}, err
	}
	if err := core.Name(in.Reason); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return templateAuthority(ctx, tx, c.Actor) }, func(tx pgx.Tx) (core.Result, error) {
		t, err := getTemplate(ctx, tx, c.Actor.OrgID, id, " FOR UPDATE")
		if err != nil {
			return core.Result{}, err
		}
		if t.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if in.ContentHash != t.ContentHash {
			return core.Result{}, core.Fail(409, "TEMPLATE_CONTENT_CHANGED", "模板内容已变化，请重新读取并核对")
		}
		switch action {
		case "publish":
			if t.State != "draft" {
				return core.Result{}, core.UnexpectedState(t.State, "published")
			}
			if _, err = validTemplateContent(TemplateContent{t.Name, t.Description, t.Schema}); err != nil {
				return core.Result{}, err
			}
			_, err = tx.Exec(ctx, "UPDATE project_templates SET state='published',published_at=clock_timestamp(),published_by=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, c.Actor.UserID)
		case "retire":
			if t.State == "archived" {
				return core.Result{}, core.UnexpectedState(t.State, "archived")
			}
			_, err = tx.Exec(ctx, "UPDATE project_templates SET state='archived',retired_at=clock_timestamp(),retired_by=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, c.Actor.UserID)
		case "revise":
			if t.State == "draft" {
				return core.Result{}, core.Fail(409, "TEMPLATE_DRAFT_EXISTS", "请先完成或停用现有草稿")
			}
			if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,64))", c.Actor.OrgID+"/"+t.Key); err != nil {
				return core.Result{}, err
			}
			var revision int64
			var draft bool
			if err = tx.QueryRow(ctx, "SELECT COALESCE(max(revision),0)+1,COALESCE(bool_or(state='draft'),false) FROM project_templates WHERE org_id=$1 AND key=$2", c.Actor.OrgID, t.Key).Scan(&revision, &draft); err != nil {
				return core.Result{}, err
			}
			if draft {
				return core.Result{}, core.Fail(409, "TEMPLATE_DRAFT_EXISTS", "该系列已有草稿，请先继续编辑或停用")
			}
			newID := core.NewID()
			body, _ := json.Marshal(t.Schema)
			_, err = tx.Exec(ctx, `INSERT INTO project_templates(id,org_id,key,name,description,revision,state,task_and_folder_schema,creator_id,based_on_id)
    VALUES($1,$2,$3,$4,$5,$6,'draft',$7,$8,$9)`, newID, c.Actor.OrgID, t.Key, t.Name, t.Description, revision, body, c.Actor.UserID, t.ID)
			if err != nil {
				return core.Result{}, err
			}
			created, e := getTemplate(ctx, tx, c.Actor.OrgID, newID, "")
			if e != nil {
				return core.Result{}, e
			}
			if err = appendTemplateEvent(ctx, tx, c.Actor, created, "revised", in.Reason); err != nil {
				return core.Result{}, err
			}
			return templateResult(created, 201, "revised"), nil
		default:
			return core.Result{}, core.Invalid("action", "模板操作无效")
		}
		if err != nil {
			return core.Result{}, err
		}
		t, err = getTemplate(ctx, tx, c.Actor.OrgID, id, "")
		if err != nil {
			return core.Result{}, err
		}
		kind := map[string]string{"publish": "published", "retire": "retired"}[action]
		if err = appendTemplateEvent(ctx, tx, c.Actor, t, kind, in.Reason); err != nil {
			return core.Result{}, err
		}
		return templateResult(t, 200, kind), nil
	})
}
func (s *Service) TemplateHistory(ctx context.Context, a core.Actor, id string, f Filter) (core.Page[TemplateEvent], error) {
	out := core.Page[TemplateEvent]{Items: []TemplateEvent{}}
	if err := authz.RequireOrganization(ctx, s.DB, a, "config.manage"); err != nil {
		return out, err
	}
	if a.RunID != "" || a.Channel == "telegram" {
		return out, core.Forbidden()
	}
	if _, err := getTemplate(ctx, s.DB, a.OrgID, id, ""); err != nil {
		return out, err
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	binding := core.CursorBinding(a, "project-template-history", id)
	var before *time.Time
	var beforeID *string
	if f.Cursor != "" {
		c, err := s.Pager.Decode(f.Cursor, binding)
		if err != nil {
			return out, err
		}
		before = &c.UpdatedAt
		beforeID = &c.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT id,template_id,version,kind,actor_id,reason,content_hash,snapshot,created_at FROM project_template_events
 WHERE org_id=$1 AND template_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, before, beforeID, f.Limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var e TemplateEvent
		var raw []byte
		if err = rows.Scan(&e.ID, &e.TemplateID, &e.Version, &e.Kind, &e.ActorID, &e.Reason, &e.ContentHash, &raw, &e.CreatedAt); err != nil {
			return out, err
		}
		if err = json.Unmarshal(raw, &e.Snapshot); err != nil {
			return out, err
		}
		out.Items = append(out.Items, e)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		e := out.Items[f.Limit-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: e.ID, UpdatedAt: e.CreatedAt, Binding: binding})
		out.NextCursor = &cursor
	}
	return out, nil
}
